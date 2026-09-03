package backup

// Валидация экспорта против нормативной схемы (SPEC 103, фаза 4).
//
// Схема contract/schema/backup.schema.json — договор между приложениями:
// файл, не проходящий её, LxBox имеет право не принять. Проверять глазами
// такое нельзя, поэтому экспорт валидируется структурно на каждом прогоне.
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
	"testing"
	"time"
)

var identityHashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

func exportSample(t *testing.T) map[string]any {
	t.Helper()
	b, _, err := Export(mkState(), ExportOptions{
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
	schemaPath := filepath.Join("..", "..", "contract", "schema", "backup.schema.json")
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
	schemaPath := filepath.Join("..", "..", "contract", "schema", "backup.schema.json")
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

	b, _, err := Export(richState(), ExportOptions{AppVersion: "test", Now: time.Unix(1750000000, 0)})
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

	backupData, err := os.ReadFile(filepath.Join("..", "..", "contract", "schema", "backup.schema.json"))
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
