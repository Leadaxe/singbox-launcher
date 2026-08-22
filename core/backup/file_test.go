package backup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Файл переживает запись и чтение без изменений — иначе бэкап бесполезен.
func TestWriteReadFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lx-backup.json")

	b, err := Export(mkState(), ExportOptions{AppVersion: "1.4.2", Now: time.Unix(1750000000, 0)})
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
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("права файла %o, ожидалось 600 — файл содержит секреты открытым текстом", perm)
	}
}

// Экспорт читаем глазами: файл правят руками, и JSON в одну строку это
// исключает.
func TestWriteFileIsIndented(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b.json")
	b, _ := Export(mkState(), ExportOptions{})
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
