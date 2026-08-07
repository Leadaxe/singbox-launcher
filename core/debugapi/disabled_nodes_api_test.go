package debugapi

import (
	"encoding/json"
	"net/http"
	"testing"

	"singbox-launcher/core/state"
)

// SPEC 094 D4 — отметки о выключенных нодах видны через Debug API.
//
// Проверка идёт по настоящему HTTP: сервер поднимается на свободном порту,
// запрос уходит с bearer-токеном. Это тот же путь, которым пользователь
// смотрит состояние снаружи, не открывая GUI.

func TestStateFullExposesDisabledNodes(t *testing.T) {
	st := state.New()
	st.Connections.Sources = []state.Source{{
		ID:      "sub-1",
		Type:    state.SourceTypeSubscription,
		Enabled: true,
		URL:     "https://example.invalid/sub",
		DisabledNodes: map[string]int64{
			"74954ec683a0aaaabbbbccccddddeeee": 1754400000,
		},
	}}

	ff := &fakeFacade{stateValue: st}
	base, _ := newTestServer(t, ff)

	resp, err := http.DefaultClient.Do(authedReq(t, "GET", base+"/state/full", nil))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var got state.State
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(got.Connections.Sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(got.Connections.Sources))
	}
	marks := got.Connections.Sources[0].DisabledNodes
	if len(marks) != 1 {
		t.Fatalf("disabled_nodes = %v, want one mark", marks)
	}
	if marks["74954ec683a0aaaabbbbccccddddeeee"] != 1754400000 {
		t.Fatalf("mark timestamp lost: %v", marks)
	}
}

// Источник без отметок не отдаёт пустое поле — оно omitempty на всём пути.
func TestStateFullOmitsEmptyDisabledNodes(t *testing.T) {
	st := state.New()
	st.Connections.Sources = []state.Source{{
		ID:      "sub-1",
		Type:    state.SourceTypeSubscription,
		Enabled: true,
		URL:     "https://example.invalid/sub",
	}}

	ff := &fakeFacade{stateValue: st}
	base, _ := newTestServer(t, ff)

	resp, err := http.DefaultClient.Do(authedReq(t, "GET", base+"/state/full", nil))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var raw map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// У поля Connections нет json-тега, поэтому ключ — имя поля как есть.
	conns, _ := raw["Connections"].(map[string]interface{})
	sources, _ := conns["sources"].([]interface{})
	if len(sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(sources))
	}
	src, _ := sources[0].(map[string]interface{})
	if _, present := src["disabled_nodes"]; present {
		t.Fatalf("empty mark map must be omitted from the API response: %v", src)
	}
}
