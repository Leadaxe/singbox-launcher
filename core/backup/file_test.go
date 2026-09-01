package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Файл переживает запись и чтение без изменений — иначе бэкап бесполезен.
func TestWriteReadFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lx-backup.json")

	b, _, err := Export(mkState(), ExportOptions{AppVersion: "1.4.2", Now: time.Unix(1750000000, 0)})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if err := WriteFile(path, b); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, warns, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(warns) != 0 {
		t.Errorf("предупреждения на своём же файле: %v", warns)
	}
	if got.LxBackup != b.LxBackup || got.ExportedAt != b.ExportedAt {
		t.Errorf("шапка изменилась: %+v против %+v", got.ExportedBy, b.ExportedBy)
	}
	if len(got.Subscriptions) != len(b.Subscriptions) || len(got.Rules) != len(b.Rules) {
		t.Errorf("состав изменился: подписок %d/%d, правил %d/%d",
			len(got.Subscriptions), len(b.Subscriptions), len(got.Rules), len(b.Rules))
	}
}

// Временный файл не остаётся рядом с результатом.
func TestWriteFileLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b.json")
	if err := WriteFile(path, &Backup{LxBackup: FormatVersion}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("остался временный файл %s", e.Name())
		}
	}
}

// Чужой файл не притворяется бэкапом: без lx_backup — понятная ошибка,
// а не половина импортированного состояния.
func TestParseRejectsNonBackup(t *testing.T) {
	_, _, err := Parse([]byte(`{"outbounds":[{"type":"vless"}]}`))
	if err == nil {
		t.Fatal("конфиг sing-box принят за бэкап")
	}
}

// Неизвестный ключ корня не отвергает файл (минорная версия обязана
// читаться), но и не проходит молча.
func TestParseReportsUnknownRootKeys(t *testing.T) {
	raw := []byte(`{"lx_backup":1,"exported_by":{"app":"lxbox"},"exported_at":"2026-08-22T00:00:00Z","channels":[{"id":1}]}`)
	b, warns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if b.LxBackup != 1 {
		t.Errorf("версия не прочитана: %d", b.LxBackup)
	}
	if !hasWarn(warns, WarnBackupUnknownField) {
		t.Errorf("неизвестный ключ не назван: %v", warns)
	}
}

// П3/П6: неизвестный ключ ВНУТРИ записи называется вместе с сущностью, в
// которой он встретился. «unknown field: packages» без имени правила
// пользователю ничего не говорит — он не знает, где это искать.
func TestParseNamesEntityOfUnknownKey(t *testing.T) {
	raw := []byte(`{"lx_backup":1,"exported_by":{"app":"lxbox","version":"3.0"},` +
		`"exported_at":"2026-08-22T00:00:00Z",` +
		`"subscriptions":[{"url":"https://example.com/sub","identity_override":"by-server"}],` +
		`"rules":[{"kind":"inline","name":"Keep","packages":["com.example"]}]}`)
	_, warns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := map[string]bool{
		"subscriptions[https://example.com/sub].identity_override": false,
		"rules[Keep].packages": false,
	}
	for _, w := range warns {
		if w.Code != WarnBackupUnknownField {
			continue
		}
		if _, ok := want[w.Detail]; ok {
			want[w.Detail] = true
		}
	}
	for detail, seen := range want {
		if !seen {
			t.Errorf("не названо %q; получено: %v", detail, warns)
		}
	}
}

// П4: файл 0.10.x с extensions на нескольких уровнях даёт ОДИН warning на
// файл, перечисляющий затронутые записи. Строка на каждый карман утопила бы
// пользователя в списке вместо объяснения, а молчание нарушило бы П6.
func TestParseAggregatesExtensionsIntoSingleWarning(t *testing.T) {
	raw := []byte(`{"lx_backup":1,"exported_by":{"app":"launcher","version":"1.5.3"},` +
		`"exported_at":"2026-08-22T00:00:00Z","extensions":{"lxbox":{"folders":[]}},` +
		`"subscriptions":[{"url":"https://a.example/sub","extensions":{"launcher":{"id":"x"}}},` +
		`{"url":"https://b.example/sub","extensions":{"launcher":{"id":"y"}}}],` +
		`"chains":[{"tag":"relay","chain":{"hops":["a"]},"extensions":{"launcher":{"id":"z"}}}]}`)
	_, warns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var got []Warning
	for _, w := range warns {
		if w.Code == WarnBackupExtensionsDropped {
			got = append(got, w)
		}
	}
	if len(got) != 1 {
		t.Fatalf("warning'ов об extensions %d, ожидался ровно один: %v", len(got), warns)
	}
	for _, place := range []string{
		"<file root>",
		"subscriptions[https://a.example/sub]",
		"subscriptions[https://b.example/sub]",
		"chains[relay]",
	} {
		if !strings.Contains(got[0].Detail, place) {
			t.Errorf("перечень затронутых записей не называет %q: %q", place, got[0].Detail)
		}
	}
}

// Единственная коллизия ТИПОВ между 0.10.x и 0.11.0: `subscriptions[].skip` —
// boolean у LxBox, список фильтров отсева у launcher. Строгий разбор ронял бы
// весь импорт из-за одного поля, то есть терял бы всё остальное молча.
// Требование BACKUP.md §10: булев skip отбрасывается с warning, импорт идёт.
func TestParseTolerantToBooleanSkip(t *testing.T) {
	raw := []byte(`{"lx_backup":1,"exported_by":{"app":"lxbox","version":"3.0"},` +
		`"exported_at":"2026-08-22T00:00:00Z",` +
		`"subscriptions":[{"url":"https://example.com/sub","label":"Main","skip":true}],` +
		`"rules":[{"kind":"inline","name":"Keep"}]}`)
	b, warns, err := Parse(raw)
	if err != nil {
		t.Fatalf("булев skip уронил разбор: %v", err)
	}
	if len(b.Subscriptions) != 1 || b.Subscriptions[0].Label != "Main" {
		t.Fatalf("остальные поля записи потеряны: %+v", b.Subscriptions)
	}
	if len(b.Subscriptions[0].Skip) != 0 {
		t.Errorf("несовпавшее по типу поле применено: %+v", b.Subscriptions[0].Skip)
	}
	if len(b.Rules) != 1 {
		t.Errorf("остальные секции потеряны: %+v", b.Rules)
	}

	found := false
	for _, w := range warns {
		if w.Code == WarnBackupFieldTypeMismatch &&
			w.Detail == "subscriptions[https://example.com/sub].skip" {
			found = true
		}
	}
	if !found {
		t.Errorf("отброшенное поле не названо warning'ом: %v", warns)
	}
}

// Несовпадение типа не должно съедать соседнюю запись, где поле в порядке:
// вырезается ключ, а не секция.
func TestParseTolerantKeepsWellTypedNeighbour(t *testing.T) {
	raw := []byte(`{"lx_backup":1,"exported_by":{"app":"lxbox","version":"3.0"},` +
		`"exported_at":"2026-08-22T00:00:00Z","subscriptions":[` +
		`{"url":"https://a.example/sub","skip":true},` +
		`{"url":"https://b.example/sub","skip":[{"field":"tag","contains":"trial"}]}]}`)
	b, _, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(b.Subscriptions) != 2 {
		t.Fatalf("подписок %d, ожидалось 2", len(b.Subscriptions))
	}
	// Ключ снимается у всей секции: тип поля у стороны один на весь файл,
	// и индексов путь из UnmarshalTypeError не содержит. Важно, что соседняя
	// запись СОХРАНИЛАСЬ и названа — а не то, что её skip уцелел.
	if b.Subscriptions[1].URL != "https://b.example/sub" {
		t.Errorf("вторая запись потеряна: %+v", b.Subscriptions[1])
	}
}

// Ошибка НЕ типа фатальна: битый файл нечего спасать, и делать вид, что он
// прочитан, хуже отказа.
func TestParseStillRejectsBrokenAfterTolerance(t *testing.T) {
	if _, _, err := Parse([]byte(`{"lx_backup":1,"subscriptions":[{"url":`)); err == nil {
		t.Fatal("обрезанный JSON принят терпимым разбором")
	}
}

// SPEC §3: неизвестные ключи ЛЮБОГО уровня дают warning с полным путём.
// Вложенный уровень — самое удобное место спрятать чужое поле, и раньше
// сканер до него не спускался.
func TestParseNamesUnknownKeysAtNestedLevels(t *testing.T) {
	raw := []byte(`{"lx_backup":1,"exported_by":{"app":"lxbox","version":"3.0"},` +
		`"exported_at":"2026-08-22T00:00:00Z","subscriptions":[{"url":"https://a.example/sub",` +
		`"outbounds":[{"tag":"vpn-1","local_only":1,"auto":{"mode":"least_test","mystery":2}}]}],` +
		`"directions":[{"tag":"vpn-9","auto":{"mode":"round_robin","phantom":3}}],` +
		`"chains":[{"tag":"relay","chain":{"hops":["a"],"weird":true}}],` +
		`"warp":[{"type":"wg","private_key":"k","stowaway":1}]}`)
	_, warns, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := map[string]bool{
		"subscriptions[https://a.example/sub].outbounds[vpn-1].local_only":   false,
		"subscriptions[https://a.example/sub].outbounds[vpn-1].auto.mystery": false,
		"directions[vpn-9].auto.phantom":                                     false,
		"chains[relay].chain.weird":                                          false,
		"warp[wg].stowaway":                                                  false,
	}
	for _, w := range warns {
		if w.Code != WarnBackupUnknownField {
			continue
		}
		if _, ok := want[w.Detail]; ok {
			want[w.Detail] = true
		}
	}
	for detail, seen := range want {
		if !seen {
			t.Errorf("вложенный ключ не назван: %q; получено %v", detail, warns)
		}
	}
}

// Битый JSON — ошибка, а не паника.
func TestParseBrokenJSON(t *testing.T) {
	if _, _, err := Parse([]byte(`{"lx_backup":1,`)); err == nil {
		t.Fatal("обрезанный JSON принят")
	}
}

// Файл больше потолка не читается: импортёр не обязан переваривать
// произвольные данные.
func TestReadFileRejectsHuge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "huge.json")
	big := make([]byte, MaxFileBytes+1)
	for i := range big {
		big[i] = ' '
	}
	if err := os.WriteFile(path, big, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadFile(path); err == nil {
		t.Fatal("файл сверх потолка принят")
	}
}

// Права файла: бэкап несёт секреты открытым текстом (BACKUP.md §5), и
// читать его посторонним ни к чему.
func TestWriteFilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b.json")
	if err := WriteFile(path, &Backup{LxBackup: FormatVersion}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// У Windows нет POSIX-битов: Go отдаёт там 0666 независимо от режима,
	// с которым файл создан. Проверять инвариант можно только там, где права
	// вообще существуют — иначе тест падает на ровном месте и блокирует CI.
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-права не поддерживаются на Windows (os.Stat всегда отдаёт 0666)")
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("права файла %o, ожидалось 600 — файл содержит секреты открытым текстом", perm)
	}
}

// Экспорт читаем глазами: файл правят руками, и JSON в одну строку это
// исключает.
func TestWriteFileIsIndented(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b.json")
	b, _, _ := Export(mkState(), ExportOptions{})
	if err := WriteFile(path, b); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var compact json.RawMessage
	if err := json.Unmarshal(data, &compact); err != nil {
		t.Fatalf("файл не разбирается: %v", err)
	}
	if len(data) > 0 && data[len(data)-1] != '\n' {
		t.Error("файл без завершающего перевода строки")
	}
	if !containsNewlines(data) {
		t.Error("файл записан одной строкой — читать и править его невозможно")
	}
}

func containsNewlines(b []byte) bool {
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n > 3
}
