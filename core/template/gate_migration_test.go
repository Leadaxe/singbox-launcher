package template

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// SPEC 107 §15.3 — критерий приёмки миграции: шаблон с легаси `if`/`if_or` и
// тот же шаблон с `#enable` обязаны давать ИДЕНТИЧНЫЙ конфиг.
//
// Это главная страховка перевода 29 употреблений в bin/wizard_template.json:
// если гейт где-то читается иначе, фрагменты молча исчезнут из конфига или,
// наоборот, появятся лишние — и заметить это на глаз невозможно.
func TestGateMigrationProducesIdenticalConfig(t *testing.T) {
	raw, err := os.ReadFile("../../bin/wizard_template.json")
	if err != nil {
		t.Skipf("шаблон недоступен: %v", err)
	}

	// Переведённая копия: "if" → "#enable" на уровне текста (значение той же
	// формы — список имён, канон §5.1 принимает его как сахар and).
	migrated := []byte(strings.ReplaceAll(string(raw), `"if":`, `"#enable":`))
	if len(migrated) != len(raw)+len(`"#enable":`)-len(`"if":`) && !strings.Contains(string(migrated), `"#enable":`) {
		t.Fatal("подмена ключа не сработала — проверить формат шаблона")
	}

	build := func(src []byte) string {
		t.Helper()
		var td struct {
			Vars   []TemplateVar   `json:"vars"`
			Params []TemplateParam `json:"params"`
			Config json.RawMessage `json:"config"`
		}
		if err := json.Unmarshal(src, &td); err != nil {
			t.Fatalf("parse: %v", err)
		}
		out, err := ApplyTemplateWithVarsFor(td.Config, td.Params, td.Vars, nil, src, LocalTarget())
		if err != nil {
			t.Fatalf("apply: %v", err)
		}
		// Нормализуем через round-trip: сравниваем значения, не форматирование.
		var v interface{}
		if err := json.Unmarshal(out, &v); err != nil {
			t.Fatalf("reparse: %v", err)
		}
		// Секреты type:"secret" генерируются случайно на каждую сборку
		// (MaybeGenerateSecrets) — маскируем, иначе сравнивать нечего.
		maskSecrets(v)
		norm, _ := json.Marshal(v)
		return string(norm)
	}

	legacy := build(raw)
	withEnable := build(migrated)
	if legacy != withEnable {
		t.Errorf("конфиг разошёлся после замены if → #enable\n  длина legacy=%d, #enable=%d",
			len(legacy), len(withEnable))
		// Показать первое расхождение — иначе диффать нечитаемо.
		for i := 0; i < len(legacy) && i < len(withEnable); i++ {
			if legacy[i] != withEnable[i] {
				from := i - 120
				if from < 0 {
					from = 0
				}
				t.Errorf("  первое расхождение на позиции %d:\n  legacy:  …%s…\n  enable:  …%s…",
					i, legacy[from:min(i+120, len(legacy))], withEnable[from:min(i+120, len(withEnable))])
				break
			}
		}
	}
}

// maskSecrets заменяет значения полей, которые генерируются случайно.
func maskSecrets(v interface{}) {
	switch x := v.(type) {
	case map[string]interface{}:
		for k, val := range x {
			if k == "secret" || k == "password" || k == "uuid" {
				x[k] = "<generated>"
				continue
			}
			maskSecrets(val)
		}
	case []interface{}:
		for _, e := range x {
			maskSecrets(e)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
