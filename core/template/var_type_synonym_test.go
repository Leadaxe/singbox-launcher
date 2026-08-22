package template

import (
	"encoding/json"
	"testing"
)

// Шаблон LxBox объявляет picker DNS-тегов как "dns_servers", наш — как
// "dns_server". Одна сущность под двумя именами означала бы, что шаблон с
// той стороны у нас молча теряет переменную: validateVar отвергает
// неизвестный тип и пропускает ВЕСЬ пресет.
func TestVarTypeSynonymDNSServers(t *testing.T) {
	t.Run("PresetVar", func(t *testing.T) {
		for _, raw := range []string{
			`{"name":"r","type":"dns_servers","default_value":"google_udp"}`,
			`{"name":"r","type":"dns_server","default_value":"google_udp"}`,
		} {
			var v PresetVar
			if err := json.Unmarshal([]byte(raw), &v); err != nil {
				t.Fatalf("%s: %v", raw, err)
			}
			if v.Type != "dns_server" {
				t.Errorf("%s → type=%q, ожидался канонический dns_server", raw, v.Type)
			}
		}
	})

	t.Run("TemplateVar", func(t *testing.T) {
		for _, raw := range []string{
			`{"name":"r","type":"dns_servers"}`,
			`{"name":"r","type":"dns_server"}`,
		} {
			var v TemplateVar
			if err := json.Unmarshal([]byte(raw), &v); err != nil {
				t.Fatalf("%s: %v", raw, err)
			}
			if v.Type != "dns_server" {
				t.Errorf("%s → type=%q, ожидался канонический dns_server", raw, v.Type)
			}
		}
	})

	// Чужие типы не трогаются.
	t.Run("другие типы без изменений", func(t *testing.T) {
		for _, typ := range []string{"outbound", "enum", "text", "number", "bool", ""} {
			if got := canonicalVarType(typ); got != typ {
				t.Errorf("canonicalVarType(%q) = %q, ожидалось без изменений", typ, got)
			}
		}
	})
}

// Пресет с типом из шаблона LxBox обязан проходить валидацию, а не
// отбрасываться целиком.
func TestPresetVarSynonymPassesValidation(t *testing.T) {
	var v PresetVar
	if err := json.Unmarshal([]byte(
		`{"name":"dom","type":"dns_servers","default_value":"google_udp"}`), &v); err != nil {
		t.Fatal(err)
	}
	for _, w := range validateVar("p1", &v) {
		if w.Action == "skip" {
			t.Fatalf("пресет отброшен из-за синонима типа: %s", w.Message)
		}
	}
}
