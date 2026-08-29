package state

import (
	"encoding/json"
	"testing"
)

// SPEC 094 D4 + SPEC 112 — отметки о выключенных нодах переживают запись
// state.json.
//
// Проверяется именно сериализация: отметка бесполезна, если теряется при
// перезапуске лаунчера. Ключ карты для этого слоя непрозрачен — с SPEC 112 это
// идентичность узла (тег в рамках источника), а legacy-хеши доживают до первой
// миграции в парсере, и оба вида обязаны переживать round-trip одинаково.

func TestDisabledNodesSurviveJSONRoundTrip(t *testing.T) {
	src := Source{
		Node: Node{Kind: SourceKindSubscription, Enabled: true},
		URL:  "https://example.invalid/sub",
		DisabledNodes: map[string]int64{
			// Тег-идентичность (SPEC 112) — с эмодзи и пробелами: ключ карты
			// не обязан быть hex, и JSON это переживает.
			"🇳🇱 Amsterdam": 1754400000,
			// И legacy-хеш, который доживёт до миграции в парсере.
			"aaaabbbbccccddddaaaabbbbccccddddaaaabbbbccccddddaaaabbbbccccdddd": 1754500000,
		},
	}

	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored Source
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(restored.DisabledNodes) != 2 {
		t.Fatalf("got %d marks, want 2 (%v)", len(restored.DisabledNodes), restored.DisabledNodes)
	}
	if restored.DisabledNodes["🇳🇱 Amsterdam"] != 1754400000 {
		t.Errorf("timestamp lost: %v", restored.DisabledNodes)
	}

	// И дальше — в ProxySource, который читает парсер.
	ps := restored.ToProxySourceV4()
	if len(ps.DisabledNodes) != 2 {
		t.Fatalf("marks lost on the way to ProxySource: %v", ps.DisabledNodes)
	}
}

// Источник без отметок не пишет поле в state.json: omitempty бережёт файл от
// пустых карт у каждой подписки.
func TestNoDisabledNodesOmittedFromJSON(t *testing.T) {
	src := Source{Node: Node{Kind: SourceKindSubscription, Enabled: true}, URL: "https://example.invalid/sub"}

	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var probe map[string]interface{}
	if err := json.Unmarshal(data, &probe); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := probe["disabled_nodes"]; present {
		t.Fatalf("empty mark map must be omitted, got %s", data)
	}
}

// Старый state.json без поля читается без ошибок — нода просто включена.
func TestLegacyStateWithoutDisabledNodes(t *testing.T) {
	const legacy = `{"type":"subscription","enabled":true,"url":"https://example.invalid/sub"}`

	var src Source
	if err := json.Unmarshal([]byte(legacy), &src); err != nil {
		t.Fatalf("legacy state must stay readable: %v", err)
	}
	if src.DisabledNodes != nil {
		t.Fatalf("legacy state must yield no marks, got %v", src.DisabledNodes)
	}
}
