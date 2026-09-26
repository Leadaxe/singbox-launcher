package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// Ручной config_json (Source.ConfigJSON) — passthrough-путь: объект уходит в
// конфиг как есть, включая типы и поля, которых per-scheme эмиттер не знает.
// Это контракт всей фичи: пользователь собирает JSON руками именно потому,
// что у лаунчера нет парсера/конвертера для его протокола, и молчаливое
// урезание до {tag,type,server,server_port} (см. emitter-parser-pairing)
// обесценило бы правку.

// manualBuild проводит ручной config_json тем же путём, что приложение:
// MaterializeServerNode (единственная точка «URI или ручной JSON → тело узла»)
// → корневой server-узел канона под тегом модели → сборка. Вторым корнем
// стоит цель detour: ссылка у ручного узла — такая же, как у любого.
func manualBuild(t *testing.T, uri string, raw json.RawMessage, tag string) *OutboundGenerationResult {
	t.Helper()
	mat, err := MaterializeServerNode(uri, raw)
	if err != nil {
		t.Fatalf("MaterializeServerNode: %v", err)
	}
	node := configtypes.CanonicalNode{
		Kind:       "server",
		Enabled:    true,
		Body:       mat.Body,
		OriginKind: mat.OriginKind,
		OriginRaw:  mat.OriginRaw,
		Detour:     &configtypes.NodeLink{Tag: "warp-out"},
	}
	return runCanonicalBuild(t, []ProxySource{
		canonRoot("S1", tag, node),
		canonRoot("S2", "warp-out", canonServerNode("warp-out", "warp-out", "warp.example", 443)),
	}, nil)
}

func TestManualConfigJSON_UnknownTypeAndFieldsSurvive(t *testing.T) {
	// URI намеренно мусорный: при заданном config_json он игнорируется —
	// протокол может вообще не иметь URI-схемы.
	res := manualBuild(t, "someproto://not-parseable", json.RawMessage(`{
			"type": "someproto",
			"tag": "hand-written",
			"server": "10.0.0.1",
			"server_port": 8443,
			"experimental_option": {"nested": true},
			"multiplex": {"enabled": true, "max_streams": 8}
		}`), "my-node")

	var got string
	for _, line := range res.OutboundsJSON {
		if strings.Contains(line, `"tag":"my-node"`) {
			got = line
		}
	}
	if got == "" {
		t.Fatalf("manual node not emitted: %v (warnings %v)", emittedTags(res), res.EmissionWarnings)
	}

	for _, want := range []string{
		`"tag":"my-node"`, // тег модели, не "hand-written"
		`"type":"someproto"`,
		`"experimental_option":{"nested":true}`,
		`"multiplex":{"enabled":true,"max_streams":8}`,
		`"detour":"warp-out"`, // detour-ссылка модели стампится и на ручную ноду
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %s in emitted JSON:\n%s", want, got)
		}
	}
	if strings.Contains(got, "hand-written") {
		t.Errorf("tag из ручного JSON уехал в конфиг:\n%s", got)
	}

	// Эмитированная объектная строка (последняя; выше может быть коммент
	// "// label") обязана быть валидным JSON.
	lines := strings.Split(strings.TrimSpace(got), "\n")
	line := strings.TrimSuffix(strings.TrimSpace(lines[len(lines)-1]), ",")
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("emitted line must be valid JSON: %v\n%s", err, line)
	}
}

func TestManualConfigJSON_WireguardGoesToEndpoints(t *testing.T) {
	res := manualBuild(t, "", json.RawMessage(`{
			"type": "wireguard",
			"address": ["10.2.0.2/32"],
			"private_key": "yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=",
			"peers": [{"address": "185.107.80.114", "port": 51820, "public_key": "xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg="}]
		}`), "wg-manual")

	if hasTag(emittedTags(res), "wg-manual") {
		t.Fatalf("wireguard config_json must not emit an outbound: %v", emittedTags(res))
	}
	var epJSON string
	for _, ep := range res.EndpointsJSON {
		if strings.Contains(ep, `"wg-manual"`) {
			epJSON = ep
		}
	}
	if epJSON == "" {
		t.Fatalf("wireguard config_json must emit an endpoint, got endpoints=%v (warnings %v)", res.EndpointsJSON, res.EmissionWarnings)
	}
	for _, want := range []string{`"wg-manual"`, `"yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk="`} {
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
			if mat, err := MaterializeServerNode("", json.RawMessage(raw)); err == nil {
				t.Fatalf("expected an error for %s, got body %s", name, mat.Body)
			}
		})
	}
}
