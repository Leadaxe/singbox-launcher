package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/subscription"
)

// Ручной config_json (Source.ConfigJSON) — passthrough-путь: объект уходит в
// конфиг как есть, включая типы и поля, которых per-scheme эмиттер не знает.
// Это контракт всей фичи: пользователь собирает JSON руками именно потому,
// что у лаунчера нет парсера/конвертера для его протокола, и молчаливое
// урезание до {tag,type,server,server_port} (см. emitter-parser-pairing)
// обесценило бы правку.

// manualLoadNodes прогоняет ProxySource через тот же вход, что и сборка.
func manualLoadNodes(t *testing.T, ps configtypes.ProxySource) []*ParsedNode {
	t.Helper()
	res, err := subscription.LoadNodesFromSourceEx(ps, map[string]int{}, nil, 0, 1)
	if err != nil {
		t.Fatalf("LoadNodesFromSourceEx: %v", err)
	}
	if res == nil {
		t.Fatal("LoadNodesFromSourceEx: nil result")
	}
	return res.Nodes
}

func TestManualConfigJSON_UnknownTypeAndFieldsSurvive(t *testing.T) {
	ps := configtypes.ProxySource{
		// URI намеренно мусорный: при заданном config_json он игнорируется —
		// протокол может вообще не иметь URI-схемы.
		Connections: []string{"someproto://not-parseable"},
		TagMask:     "my-node",
		DetourTag:   "warp-out",
		ConfigJSON: json.RawMessage(`{
			"type": "someproto",
			"tag": "hand-written",
			"server": "10.0.0.1",
			"server_port": 8443,
			"experimental_option": {"nested": true},
			"multiplex": {"enabled": true, "max_streams": 8}
		}`),
	}

	nodes := manualLoadNodes(t, ps)
	if len(nodes) != 1 {
		t.Fatalf("expected exactly 1 node from config_json, got %d", len(nodes))
	}
	node := nodes[0]
	if !node.EmitRaw {
		t.Error("manual node must carry EmitRaw")
	}
	if node.Tag != "my-node" {
		t.Errorf("tag must be restamped by TagMask: got %q", node.Tag)
	}

	outJSONs, epJSON, err := EmitNodeJSONs(node)
	if err != nil {
		t.Fatalf("EmitNodeJSONs: %v", err)
	}
	if epJSON != "" || len(outJSONs) != 1 {
		t.Fatalf("expected 1 outbound line, got outbounds=%d endpoint=%q", len(outJSONs), epJSON)
	}
	got := outJSONs[0]

	for _, want := range []string{
		`"tag":"my-node"`, // финальный тег, не "hand-written"
		`"type":"someproto"`,
		`"experimental_option":{"nested":true}`,
		`"multiplex":{"enabled":true,"max_streams":8}`,
		`"detour":"warp-out"`, // source-level detour стампится и на ручную ноду
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %s in emitted JSON:\n%s", want, got)
		}
	}

	// Эмитированная объектная строка (последняя; выше может быть коммент
	// "// label") обязана быть валидным JSON.
	lines := strings.Split(strings.TrimSpace(got), "\n")
	line := strings.TrimSuffix(strings.TrimSpace(lines[len(lines)-1]), ",")
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("emitted line must be valid JSON: %v\n%s", err, line)
	}

	// SPEC 112: идентичность ручной ноды — её тег. Он обязан быть, иначе
	// ни отметку выключения, ни ссылку detour к ней не привязать.
	if NodeIdentity(node) == "" {
		t.Error("у ручной ноды обязана быть идентичность (тег)")
	}
}

func TestManualConfigJSON_WireguardGoesToEndpoints(t *testing.T) {
	ps := configtypes.ProxySource{
		TagMask: "wg-manual",
		ConfigJSON: json.RawMessage(`{
			"type": "wireguard",
			"address": ["10.2.0.2/32"],
			"private_key": "FAKEKEY",
			"peers": [{"address": "185.107.80.114", "port": 51820, "public_key": "FAKEPUB"}]
		}`),
	}

	nodes := manualLoadNodes(t, ps)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}
	outJSONs, epJSON, err := EmitNodeJSONs(nodes[0])
	if err != nil {
		t.Fatalf("EmitNodeJSONs: %v", err)
	}
	if len(outJSONs) != 0 || epJSON == "" {
		t.Fatalf("wireguard config_json must emit an endpoint, got outbounds=%d endpoint=%q", len(outJSONs), epJSON)
	}
	for _, want := range []string{`"tag": "wg-manual"`, `"private_key": "FAKEKEY"`} {
		if !strings.Contains(epJSON, want) {
			t.Errorf("expected %s in endpoint JSON:\n%s", want, epJSON)
		}
	}
}

func TestManualConfigJSON_InvalidInputYieldsNoNodes(t *testing.T) {
	for name, raw := range map[string]string{
		"invalid_json": `{"type": "vless",`,
		"missing_type": `{"server": "10.0.0.1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			ps := configtypes.ProxySource{ConfigJSON: json.RawMessage(raw)}
			if nodes := manualLoadNodes(t, ps); len(nodes) != 0 {
				t.Fatalf("expected 0 nodes for %s, got %d", name, len(nodes))
			}
		})
	}
}
