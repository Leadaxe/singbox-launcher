package core

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/state"
)

// SPEC 148 §5: Save choice — поле появляется, меняется, убирается; порядок
// прочих ключей не трогается.
func TestSetBodyTopLevelString(t *testing.T) {
	body := []byte(`{"type":"tailscale","tag":"ts","auth_key":"k","hostname":"mac"}`)

	added, err := SetBodyTopLevelString(body, "exit_node", "100.64.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(added), `{"type":"tailscale","tag":"ts","auth_key":"k","hostname":"mac","exit_node":"100.64.0.1"}`; got != want {
		t.Errorf("появление:\n got %s\nwant %s", got, want)
	}

	changed, err := SetBodyTopLevelString([]byte(`{"type":"tailscale","exit_node":"a","tag":"ts"}`), "exit_node", "b")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(changed), `{"type":"tailscale","exit_node":"b","tag":"ts"}`; got != want {
		t.Errorf("смена на месте:\n got %s\nwant %s", got, want)
	}

	removed, err := SetBodyTopLevelString(added, "exit_node", "")
	if err != nil {
		t.Fatal(err)
	}
	if string(removed) != string(body) {
		t.Errorf("снятие: got %s, want %s", removed, body)
	}

	// Числа и вложенные объекты переживают правку дословно.
	nested, err := SetBodyTopLevelString([]byte(`{"a":1.50,"b":{"x":[1,2]}}`), "exit_node", "r")
	if err != nil || string(nested) != `{"a":1.50,"b":{"x":[1,2]},"exit_node":"r"}` {
		t.Errorf("вложенное: %s, %v", nested, err)
	}

	if _, err := SetBodyTopLevelString([]byte(`[1]`), "exit_node", "r"); err == nil {
		t.Error("не объект обязан давать ошибку")
	}
}

// Save choice пишет только в свой узел: сервер в корне или член папки.
// Узел подписки не находится — кнопка у него скрыта.
func TestFindOwnTailscaleNode(t *testing.T) {
	ts := func(tag string) state.Node {
		return state.Node{Kind: state.SourceKindServer, Tag: tag, Enabled: true,
			Body: json.RawMessage(`{"type":"tailscale","tag":"` + tag + `"}`)}
	}
	vless := state.Node{Kind: state.SourceKindServer, Tag: "v", Body: json.RawMessage(`{"type":"vless","tag":"v"}`)}
	s := &state.State{Sources: []state.Source{
		{Node: ts("root-ts")},
		{Node: vless},
		{Node: state.Node{Kind: state.SourceKindFolder}, ID: "f", Nodes: []state.Node{ts("member-ts")}},
		{Node: state.Node{Kind: state.SourceKindSubscription}, URL: "https://sub", Nodes: []state.Node{ts("sub-ts")}},
	}}
	if n := findOwnTailscaleNode(s, "root-ts"); n == nil || n.Tag != "root-ts" {
		t.Errorf("сервер в корне не найден: %v", n)
	}
	if n := findOwnTailscaleNode(s, "member-ts"); n == nil || n.Tag != "member-ts" {
		t.Errorf("член папки не найден: %v", n)
	}
	if n := findOwnTailscaleNode(s, "sub-ts"); n != nil {
		t.Error("узел подписки не свой: Save choice у него скрыта")
	}
	if n := findOwnTailscaleNode(s, "v"); n != nil {
		t.Error("не tailscale")
	}
}
