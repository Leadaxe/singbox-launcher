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
	b, err := Export(mkState(), ExportOptions{
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

// Экспорт не должен нести ключей, которых схема не знает: она закрыта
// (additionalProperties:false), и лишнее поле сделает файл невалидным для
// принимающей стороны.
func TestExportHasNoUnknownRootKeys(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "contract", "schema", "backup.schema.json")
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Skipf("схема недоступна: %v", err)
	}
	var schema struct {
		Properties           map[string]any `json:"properties"`
		AdditionalProperties *bool          `json:"additionalProperties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("разбор схемы: %v", err)
	}
	if schema.AdditionalProperties != nil && *schema.AdditionalProperties {
		t.Skip("схема допускает произвольные ключи — проверка неприменима")
	}
	doc := exportSample(t)
	for key := range doc {
		if _, ok := schema.Properties[key]; !ok {
			t.Errorf("экспорт несёт ключ %q, которого нет в схеме", key)
		}
	}
}
