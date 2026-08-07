package state

import (
	"encoding/json"
	"testing"
)

// SPEC 094 D4 — отметки о выключенных нодах переживают запись state.json.
//
// Проверяется именно сериализация: отметка бесполезна, если теряется при
// перезапуске лаунчера — ровно тот сценарий, ради которого она привязана к
// хешу, а не к тегу.

func TestDisabledNodesSurviveJSONRoundTrip(t *testing.T) {
	src := Source{
		Type:    SourceTypeSubscription,
		Enabled: true,
		URL:     "https://example.invalid/sub",
		DisabledNodes: map[string]int64{
			"aaaabbbbccccdddd": 1754400000,
			"eeeeffff00001111": 1754500000,
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
	if restored.DisabledNodes["aaaabbbbccccdddd"] != 1754400000 {
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
	src := Source{Type: SourceTypeSubscription, Enabled: true, URL: "https://example.invalid/sub"}

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
