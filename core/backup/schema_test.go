package backup

// Валидация экспорта против нормативных схем (SPEC 103 фаза 4; SPEC 127 W3.3).
//
// Схема — договор между приложениями: файл, не проходящий её, LxBox имеет
// право не принять. Проверять глазами такое нельзя, поэтому каждый писатель
// валидируется структурно на каждом прогоне, и ПИСАТЕЛЕЙ ДВА (окно
// совместимости, docs/BACKUP.md §1):
//
//   - Export012 → contract/schema/backup-0.12.schema.json (lx_backup: 1);
//   - Export10  → contract/schema/backup.schema.json      (lx_backup: 2).
//
// Обе проверки идут на богатом состоянии: бедное состояние проходит любую
// схему, потому что почти все поля необязательные.
//
// Валидатор здесь минимальный и намеренно проверяет ровно то, что схема
// объявляет строгим: обязательные поля, закрытые множества (enum),
// формат ключей disabled и запрет неизвестных ключей там, где стоит
// additionalProperties:false. Полноценный JSON-Schema-движок ради этого в
// зависимости не тянется.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Два адреса схем: 0.12 — legacy-писателя, backup.schema.json — действующего
// формата 1.0. Имена вынесены в функции, потому что перепутать их в тесте
// значит проверять писателя чужой схемой и не заметить расхождения.
func schemaPath012() string {
	return filepath.Join("..", "..", "contract", "schema", "backup-0.12.schema.json")
}

func schemaPath10() string {
	return filepath.Join("..", "..", "contract", "schema", "backup.schema.json")
}

var identityHashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

func exportSample(t *testing.T) map[string]any {
	t.Helper()
	b, _, err := Export012(mkState(), ExportOptions{
		AppVersion: "1.4.2", Platform: "darwin", Now: time.Unix(1750000000, 0),
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return doc
}

// Обязательные поля корня — без них файл не опознать как бэкап.
func TestExportHasRequiredRootFields(t *testing.T) {
	doc := exportSample(t)
	for _, key := range []string{"lx_backup", "exported_by", "exported_at"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("нет обязательного поля %q", key)
		}
	}
	if v, _ := doc["lx_backup"].(float64); int(v) != FormatVersion {
		t.Errorf("lx_backup = %v, ожидалось %d", doc["lx_backup"], FormatVersion)
	}
	by, _ := doc["exported_by"].(map[string]any)
	if by["app"] != AppLauncher {
		t.Errorf("exported_by.app = %v", by["app"])
	}
}

// Ключи disabled — identity-хеши (64 hex). Короткий ключ означает, что
// отметка не сопоставится ни с одной нодой на другой стороне.
func TestExportDisabledKeysAreIdentityHashes(t *testing.T) {
	doc := exportSample(t)
	subs, _ := doc["subscriptions"].([]any)
	for _, s := range subs {
		sub, _ := s.(map[string]any)
		disabled, ok := sub["disabled"].(map[string]any)
		if !ok {
			continue
		}
		for hash := range disabled {
			if !identityHashRe.MatchString(hash) {
				t.Errorf("ключ disabled %q не является identity-хешем (64 hex)", hash)
			}
		}
	}
}

// kind правила — из закрытого множества схемы.
func TestExportRuleKindsAreKnown(t *testing.T) {
	allowed := map[string]bool{"inline": true, "srs": true, "preset": true, "json": true}
	doc := exportSample(t)
	rules, _ := doc["rules"].([]any)
	if len(rules) == 0 {
		t.Fatal("в образце нет правил — тест бессмыслен")
	}
	for _, r := range rules {
		rule, _ := r.(map[string]any)
		kind, _ := rule["kind"].(string)
		if !allowed[kind] {
			t.Errorf("kind %q вне множества схемы", kind)
		}
	}
}

// Экспорт не должен нести ключей, которых схема не объявляет.
//
// Схема с 0.11.0 открыта (additionalProperties:true) намеренно — чужой файл с
// лишним полем обязан проходить валидацию (П3). Но это послабление для
// ЧТЕНИЯ, а не для письма: собственный экспорт, вышедший за объявленные
// properties, означал бы поле, которого нет в таблице BACKUP.md §2, — то есть
// ровно тайный груз, ради сноса которого механизм extensions и убран.
func TestExportHasNoUndeclaredRootKeys(t *testing.T) {
	schemaPath := schemaPath012()
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Skipf("схема недоступна: %v", err)
	}
	var schema struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("разбор схемы: %v", err)
	}
	doc := exportSample(t)
	for key := range doc {
		if _, ok := schema.Properties[key]; !ok {
			t.Errorf("экспорт несёт ключ %q, которого нет в схеме", key)
		}
	}
}

// То же для записей сущностей: поле, вышедшее из-под объявленных properties,
// не попадёт в таблицу поддержки и станет невидимым для другой стороны.
func TestExportEntityKeysAreDeclared(t *testing.T) {
	schemaPath := schemaPath012()
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Skipf("схема недоступна: %v", err)
	}
	var schema struct {
		Properties map[string]struct {
			Items struct {
				Properties map[string]any `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("разбор схемы: %v", err)
	}

	b, _, err := Export012(richState(), ExportOptions{AppVersion: "test", Now: time.Unix(1750000000, 0)})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	for _, section := range []string{"subscriptions", "servers", "chains", "rules"} {
		declared := schema.Properties[section].Items.Properties
		if len(declared) == 0 {
			t.Fatalf("схема не объявляет полей секции %s — сверять нечем", section)
		}
		var items []map[string]json.RawMessage
		if err := json.Unmarshal(doc[section], &items); err != nil {
			t.Fatalf("%s: %v", section, err)
		}
		if len(items) == 0 {
			t.Fatalf("в образце нет записей %s — тест бессмыслен", section)
		}
		for _, item := range items {
			for key := range item {
				if _, ok := declared[key]; !ok {
					t.Errorf("%s[]: экспорт несёт необъявленный ключ %q", section, key)
				}
			}
		}
	}
}

// ── Писатель 1.0 против backup.schema.json ───────────────────────────────
//
// Отдельным набором, а не параметризацией 0.12-тестов: формы файлов разные
// (одна sources[] против четырёх плоских секций), и общий тест свёлся бы к
// двум ветвям в каждом проверяющем.

func export10Sample(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(fixedExport10(t, richState10()), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return doc
}

func readSchema10(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(schemaPath10())
	if err != nil {
		t.Skipf("схема недоступна: %v", err)
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("разбор схемы 1.0: %v", err)
	}
	return schema
}

// Корень файла 1.0: обязательные ключи на месте, маркер формата — 2, и ни
// одного ключа мимо объявленных properties.
//
// Открытость схемы (additionalProperties: true) — послабление для ЧТЕНИЯ
// чужого файла, а не для письма: собственный экспорт, вышедший за объявленные
// properties, означал бы поле, которого нет в таблице BACKUP.md §2, то есть
// тайный груз.
func TestExport10RootKeysAreDeclared(t *testing.T) {
	doc := export10Sample(t)
	for _, key := range []string{"lx_backup", "exported_by", "exported_at"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("нет обязательного поля %q", key)
		}
	}
	var marker int
	if err := json.Unmarshal(doc["lx_backup"], &marker); err != nil || marker != FormatVersion10 {
		t.Errorf("lx_backup = %s, ожидалось %d", doc["lx_backup"], FormatVersion10)
	}

	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	data, err := os.ReadFile(schemaPath10())
	if err != nil {
		t.Skipf("схема недоступна: %v", err)
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("разбор схемы: %v", err)
	}
	for key := range doc {
		if _, ok := schema.Properties[key]; !ok {
			t.Errorf("экспорт 1.0 несёт ключ %q, которого нет в схеме", key)
		}
	}
	// Плоских секций 0.12 в файле 1.0 быть не должно, и схема их не знает:
	// проверка «ключ объявлен» поймала бы это и сама, но явная формулировка
	// объясняет, ЧТО именно сломалось.
	for _, key := range []string{"subscriptions", "servers", "chains"} {
		if _, ok := schema.Properties[key]; ok {
			t.Errorf("схема 1.0 всё ещё объявляет секцию 0.12 %q", key)
		}
	}
}

// Записи файла 1.0 — против $defs соответствующего вида.
//
// Проверяется то же, что у 0.12: ни одного ключа мимо объявленных. Адрес
// определения выбирается по kind записи — ровно как это делает if/then в
// схеме, и расхождение между «что пишет Go» и «что объявляет схема» вылезает
// на том виде, где оно есть, а не общим «где-то в sources[]».
func TestExport10EntityKeysAreDeclared(t *testing.T) {
	schema := readSchema10(t)
	var defs map[string]struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema["$defs"], &defs); err != nil {
		t.Fatalf("разбор $defs: %v", err)
	}

	declared := func(name string) map[string]json.RawMessage {
		d := defs[name].Properties
		if len(d) == 0 {
			t.Fatalf("$defs/%s не объявляет полей — сверять нечем", name)
		}
		return d
	}
	check := func(where, def string, rec map[string]json.RawMessage) {
		for key := range rec {
			if _, ok := declared(def)[key]; !ok {
				t.Errorf("%s: экспорт несёт ключ %q, не объявленный в $defs/%s", where, key, def)
			}
		}
	}

	doc := export10Sample(t)
	var sources []map[string]json.RawMessage
	if err := json.Unmarshal(doc["sources"], &sources); err != nil {
		t.Fatalf("sources: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("в образце нет источников — тест бессмыслен")
	}
	seen := map[string]bool{}
	for _, rec := range sources {
		var kind string
		if err := json.Unmarshal(rec["kind"], &kind); err != nil {
			t.Fatalf("kind: %v", err)
		}
		seen[kind] = true
		switch kind {
		case "folder":
			check("sources[] folder", "sourceFolder", rec)
			var nodes []map[string]json.RawMessage
			if err := json.Unmarshal(rec["nodes"], &nodes); err != nil {
				t.Fatalf("nodes: %v", err)
			}
			for _, n := range nodes {
				check("sources[].nodes[]", "node", n)
			}
		case "subscription":
			check("sources[] subscription", "sourceSubscription", rec)
		default:
			check("sources[] "+kind, "sourceServer", rec)
		}
	}
	// Образец обязан покрывать все виды: иначе тест зелен потому, что вида в
	// нём нет, а не потому, что он описан.
	for _, kind := range []string{"server", "chain", "folder", "subscription"} {
		if !seen[kind] {
			t.Errorf("в образце нет источника вида %q — этот вид не проверен", kind)
		}
	}

	var rules []map[string]json.RawMessage
	if err := json.Unmarshal(doc["rules"], &rules); err != nil {
		t.Fatalf("rules: %v", err)
	}
	if len(rules) == 0 {
		t.Fatal("в образце нет правил — тест бессмыслен")
	}
	for _, rec := range rules {
		check("rules[]", "rule", rec)
	}

	var dns struct {
		Servers []map[string]json.RawMessage `json:"servers"`
		Rules   []map[string]json.RawMessage `json:"rules"`
	}
	if err := json.Unmarshal(doc["dns"], &dns); err != nil {
		t.Fatalf("dns: %v", err)
	}
	if len(dns.Servers) == 0 || len(dns.Rules) == 0 {
		t.Fatal("в образце нет DNS-записей — тест бессмыслен")
	}
	for _, rec := range dns.Servers {
		check("dns.servers[]", "dnsServer", rec)
	}
	for _, rec := range dns.Rules {
		check("dns.rules[]", "dnsRule", rec)
	}
}

// Секции узла: закрытое множество ключей (единственное место схемы 1.0, где
// additionalProperties:false), и записи внутри — В КОРНЕВОЙ форме.
//
// Ровно это и есть «одно пространство имён»: узловое правило отличается от
// корневого только тем, где оно лежит. Второй набор ключей у тех же
// сущностей был бы возвратом к маппингу, ради сноса которого и затеян 1.0.
func TestExport10SectionsUseRootRecordForm(t *testing.T) {
	doc := export10Sample(t)
	var sources []map[string]json.RawMessage
	if err := json.Unmarshal(doc["sources"], &sources); err != nil {
		t.Fatalf("sources: %v", err)
	}
	var sections struct {
		Rules []map[string]json.RawMessage `json:"rules"`
		DNS   struct {
			Servers []map[string]json.RawMessage `json:"servers"`
			Rules   []map[string]json.RawMessage `json:"rules"`
		} `json:"dns"`
	}
	found := false
	for _, rec := range sources {
		raw, ok := rec["sections"]
		if !ok {
			continue
		}
		found = true
		if err := json.Unmarshal(raw, &sections); err != nil {
			t.Fatalf("sections: %v", err)
		}
	}
	if !found {
		t.Fatal("в образце нет узла с секциями — тест бессмыслен")
	}
	if len(sections.Rules) == 0 || len(sections.DNS.Servers) == 0 || len(sections.DNS.Rules) == 0 {
		t.Fatalf("секции образца неполны: %+v", sections)
	}
	// Форма 0.12 (match/outbound у правила, name/value у DNS) внутри секций
	// означала бы, что «одно пространство имён» не доехало.
	for _, rec := range sections.Rules {
		for _, forbidden := range []string{"match", "outbound"} {
			if _, ok := rec[forbidden]; ok {
				t.Errorf("правило секции несёт ключ формы 0.12 %q", forbidden)
			}
		}
		if _, ok := rec["body"]; !ok {
			t.Errorf("у правила секции нет body: %v", rec)
		}
	}
	for _, rec := range append(sections.DNS.Servers, sections.DNS.Rules...) {
		for _, forbidden := range []string{"name", "value"} {
			if _, ok := rec[forbidden]; ok {
				t.Errorf("DNS-запись секции несёт ключ формы 0.12 %q", forbidden)
			}
		}
	}
}

// TestBackupDirectionDefMirrorsCanon — копия канона Направления в схеме
// бэкапа не должна разъезжаться с оригиналом.
//
// contract/schema/backup.schema.json несёт $defs/direction и $defs/directionAuto
// РУЧНОЙ копией contract/schema/direction.schema.json: JSON Schema не умеет
// ссылаться на внешний файл так, чтобы это работало у обеих сторон без
// резолвера, и копия — сознательная цена. Цена копии — дрейф: поле, добавленное
// в канон, молча не доедет до бэкапа, и Направление приедет урезанным.
// Проверяется именно множество полей, а не тексты описаний: описания в копии
// имеют право быть короче, а вот состав обязан совпадать РОВНО — «шире» тоже
// расхождение, оно означает поле, которого в каноне нет.
//
// required сверяется тем же правилом: у Направления ровно одно обязательное
// поле — tag, и разъехавшийся список обязательных полей делает файл,
// проходящий одну схему и не проходящий другую.
func TestBackupDirectionDefMirrorsCanon(t *testing.T) {
	type schemaNode struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
		Defs       map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}

	read := func(name string) schemaNode {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("..", "..", "contract", "schema", name))
		if err != nil {
			t.Skipf("схема %s недоступна: %v", name, err)
		}
		var n schemaNode
		if err := json.Unmarshal(data, &n); err != nil {
			t.Fatalf("разбор %s: %v", name, err)
		}
		return n
	}

	canon := read("direction.schema.json")

	backupData, err := os.ReadFile(schemaPath10())
	if err != nil {
		t.Skipf("схема бэкапа недоступна: %v", err)
	}
	var backupSchema struct {
		Defs map[string]schemaNode `json:"$defs"`
	}
	if err := json.Unmarshal(backupData, &backupSchema); err != nil {
		t.Fatalf("разбор backup.schema.json: %v", err)
	}

	mirror := backupSchema.Defs["direction"]
	if len(mirror.Properties) == 0 {
		t.Fatal("$defs/direction в схеме бэкапа не объявляет полей — сверять нечем")
	}
	if len(canon.Properties) == 0 {
		t.Fatal("direction.schema.json не объявляет полей — сверять нечем")
	}
	comparePropertySets(t, "$defs/direction", canon.Properties, mirror.Properties)

	if !equalStringSets(canon.Required, mirror.Required) {
		t.Errorf("$defs/direction: required %v, в каноне %v", mirror.Required, canon.Required)
	}

	// Вложенная группа автовыбора скопирована тем же способом и дрейфует так же.
	canonAuto := canon.Defs["auto"].Properties
	mirrorAuto := backupSchema.Defs["directionAuto"].Properties
	if len(canonAuto) == 0 || len(mirrorAuto) == 0 {
		t.Fatal("группа auto не объявлена в одной из схем — сверять нечем")
	}
	comparePropertySets(t, "$defs/directionAuto", canonAuto, mirrorAuto)
}

func comparePropertySets(t *testing.T, where string, canon, mirror map[string]json.RawMessage) {
	t.Helper()
	for key := range canon {
		if _, ok := mirror[key]; !ok {
			t.Errorf("%s: канон объявляет %q, копия в схеме бэкапа — нет: Направление приедет урезанным", where, key)
		}
	}
	for key := range mirror {
		if _, ok := canon[key]; !ok {
			t.Errorf("%s: копия объявляет %q, которого нет в каноне", where, key)
		}
	}
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, v := range a {
		set[v] = true
	}
	for _, v := range b {
		if !set[v] {
			return false
		}
	}
	return true
}

// Словарь кодов бэкапа — нормативный источник (contract/registry/backup_warnings.json).
//
// Нормативность без проверки — просто текст: код в Go заводят, реестр
// остаётся, и вторая сторона (LxBox) о деградации не узнаёт. Так уже уезжал
// словарь разбора подписок (`ws_early_data_converted` прожил весь цикл вне
// реестра), и ровно от этого там стоит registry_sync_test.
//
// Проверяется в обе стороны: код из Go обязан быть объявлен, а объявленный
// код — либо ставиться лаунчером, либо быть помечен чужой стороной (LxBox
// эмитирует свои два, и требовать их от Go значило бы требовать чужой код).
// Список констант вычитывается ИЗ ИСХОДНИКА, а не пишется здесь руками:
// список в тесте разъехался бы точно так же, как разъезжается реестр.
func TestBackupWarningCodesDeclaredInRegistry(t *testing.T) {
	reg := loadBackupWarningsRegistry(t)
	for name, code := range goBackupWarningConstants(t) {
		entry, ok := reg[code]
		if !ok {
			t.Errorf("код %q (%s) есть в Go, но отсутствует в contract/registry/backup_warnings.json", code, name)
			continue
		}
		if entry.Severity == "" || entry.Side == "" || entry.Desc == "" {
			t.Errorf("код %q объявлен неполно: severity=%q side=%q desc=%d символов",
				code, entry.Severity, entry.Side, len(entry.Desc))
		}
	}
}

// Объявленный код, который лаунчер не ставит, обязан быть чужим (LxBox).
// Иначе это обещанная, но не выдаваемая диагностика: пользователь о потере
// не узнает, а вторая сторона будет ждать кода, которого нет.
func TestBackupWarningCodesAreActuallySet(t *testing.T) {
	// Коды, которые эмитирует ТОЛЬКО LxBox (contract/README.md, 0.12.2).
	foreign := map[string]bool{
		"backup_dns_entry_skipped": true,
		"backup_warp_skipped":      true,
	}
	consts := goBackupWarningConstants(t)
	byCode := map[string]string{}
	for name, code := range consts {
		byCode[code] = name
	}
	used := backupConstantsUsedInPackage(t)
	for code := range loadBackupWarningsRegistry(t) {
		if foreign[code] {
			continue
		}
		name, ok := byCode[code]
		if !ok {
			t.Errorf("код %q объявлен в реестре, но в Go его нет — либо заводить, либо помечать стороной", code)
			continue
		}
		if !used[name] {
			t.Errorf("константа %s (%q) объявлена, но нигде не ставится: обещанная диагностика, которой не будет", name, code)
		}
	}
}

type backupWarningEntry struct {
	Severity string   `json:"severity"`
	Params   []string `json:"params"`
	Side     string   `json:"side"`
	Desc     string   `json:"desc"`
}

func loadBackupWarningsRegistry(t *testing.T) map[string]backupWarningEntry {
	t.Helper()
	path := filepath.Join("..", "..", "contract", "registry", "backup_warnings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("реестр не найден (%s) — контракт не синхронизирован", path)
	}
	var f struct {
		Warnings map[string]backupWarningEntry `json:"warnings"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("разбор %s: %v", path, err)
	}
	if len(f.Warnings) == 0 {
		t.Fatalf("%s: словарь пуст", path)
	}
	return f.Warnings
}

// goBackupWarningConstants — коды из import.go, вычитанные из исходника.
func goBackupWarningConstants(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile("import.go")
	if err != nil {
		t.Fatalf("import.go: %v", err)
	}
	re := regexp.MustCompile(`(WarnBackup\w+)\s*=\s*"([^"]+)"`)
	out := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(string(data), -1) {
		out[m[1]] = m[2]
	}
	if len(out) == 0 {
		t.Fatal("в import.go не найдено ни одной константы кода")
	}
	return out
}

// backupConstantsUsedInPackage — какие константы реально СТАВЯТСЯ в коде
// пакета.
//
// Использованием считается упоминание в литерале Warning'а (`Code:` или
// `Warning{Code: …}`), а не объявление константы: строка `WarnX = "x"` в
// блоке констант упоминает имя, но диагностики не даёт. Поэтому строки
// объявлений отсеиваются, а не целый файл: коды и ставятся, и объявляются в
// import.go, и отсев по имени файла объявил бы «не ставится» половину
// словаря.
func backupConstantsUsedInPackage(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("чтение пакета: %v", err)
	}
	out := map[string]bool{}
	use := regexp.MustCompile(`WarnBackup\w+`)
	decl := regexp.MustCompile(`^\s*(WarnBackup\w+)\s*=\s*"`)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if decl.MatchString(line) {
				continue
			}
			for _, m := range use.FindAllString(line, -1) {
				out[m] = true
			}
		}
	}
	return out
}
