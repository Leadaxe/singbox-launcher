package build

import (
	"encoding/json"
	"testing"

	corestate "singbox-launcher/core/state"
)

// SPEC 105: DNS-группа — сервер типа `group` со списком членов.
//
// Ядро (форк lx) такой тип принимает: проверено `sing-box check` — конфиг с
// {"type":"group","servers":[...]} проходит, а с выдуманным типом падает
// «unknown transport type».
//
// Группа даёт то, чего одиночный сервер не может: несколько путей до одного
// набора зон с выбором по скорости. Одна мёртвая нода не вешает резолв.
func TestDNSGroupServerSurvivesEmission(t *testing.T) {
	body := map[string]interface{}{
		"type":      "group",
		"servers":   []interface{}{"udp-a", "doh-b"},
		"mode":      "fastest",
		"error_ttl": float64(30),
		// Поля визарда обязаны быть вычищены — ядро их не знает.
		"description": "Быстрый российский DNS",
		"enabled":     true,
		"_ui_hint":    "internal",
	}

	got := stripDNSWizardOnlyFields(body)

	if got["type"] != "group" {
		t.Fatalf("тип группы потерян: %v", got["type"])
	}
	members, ok := got["servers"].([]interface{})
	if !ok || len(members) != 2 {
		t.Fatalf("состав группы потерян: %v", got["servers"])
	}
	if got["mode"] != "fastest" {
		t.Errorf("режим выбора потерян: %v", got["mode"])
	}
	if got["error_ttl"] != float64(30) {
		t.Errorf("error_ttl потерян: %v", got["error_ttl"])
	}
	for _, wizardOnly := range []string{"description", "enabled", "_ui_hint"} {
		if _, present := got[wizardOnly]; present {
			t.Errorf("поле визарда %q уехало в конфиг — ядро его не знает", wizardOnly)
		}
	}
}

// Группа хранится как обычный пользовательский DNS-сервер: отдельного вида
// записи не нужно, у kind=user тело произвольное.
func TestDNSGroupRoundTripsThroughState(t *testing.T) {
	srv := corestate.DNSServer{
		Kind:    corestate.DNSServerKindUser,
		Tag:     "dns-ru",
		Enabled: true,
		Body: map[string]interface{}{
			"type":    "group",
			"servers": []interface{}{"udp-a", "doh-b"},
			"mode":    "fastest",
		},
	}

	raw, err := json.Marshal(srv)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var back corestate.DNSServer
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if back.Body["type"] != "group" {
		t.Fatalf("после round-trip тип = %v; JSON: %s", back.Body["type"], raw)
	}
	members, _ := back.Body["servers"].([]interface{})
	if len(members) != 2 {
		t.Errorf("состав группы после round-trip: %v", back.Body["servers"])
	}
}

// Ключевые слова движка шаблонов не должны уезжать в конфиг ядра.
//
// Легаси-написание (`if`, `if_or`) вычищалось с самого начала, а канон
// SPEC 107 (`#enable`, `#if`) — нет: гейт оставался в теле DNS-сервера и
// уходил ядру как неизвестное поле.
func TestDNSStripsTemplateDirectives(t *testing.T) {
	got := stripDNSWizardOnlyFields(map[string]interface{}{
		"type":     "udp",
		"server":   "1.1.1.1",
		"#enable":  "@ipv6_enabled",
		"#if":      []interface{}{"@tun"},
		"if":       []interface{}{"@legacy"},
		"_scratch": 1,
	})

	for _, directive := range []string{"#enable", "#if", "if", "_scratch"} {
		if _, present := got[directive]; present {
			t.Errorf("директива %q уехала в конфиг ядра", directive)
		}
	}
	if got["type"] != "udp" || got["server"] != "1.1.1.1" {
		t.Errorf("настоящие поля потеряны: %v", got)
	}
}
