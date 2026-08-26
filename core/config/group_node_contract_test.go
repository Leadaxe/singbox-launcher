package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 094 A5.1 — сохранность существующих контрактов при вводе схемы "group".
//
// Узел-группа проходит через весь конвейер, поэтому каждое место, которое
// ветвится по node.Scheme либо пишет узел на диск, обязано вести себя
// предсказуемо. Здесь проверяется именно это, а не сам импорт.

// groupNode строит узел-группу так, как его отдаёт парсер.
func groupNode(tag string, members ...string) *ParsedNode {
	list := make([]interface{}, 0, len(members))
	for _, m := range members {
		list = append(list, m)
	}
	return &ParsedNode{
		Tag:    tag,
		Scheme: configtypes.SchemeGroup,
		Label:  tag,
		Outbound: map[string]interface{}{
			"tag":                       tag,
			"type":                      "urltest",
			configtypes.GroupMembersKey: list,
			"url":                       "https://www.gstatic.com/generate_204",
			"interval":                  "5m",
		},
		SourceIndex: configtypes.UnsetSourceIndex,
	}
}

func plainNode(tag, server string) *ParsedNode {
	return &ParsedNode{
		Tag:    tag,
		Scheme: "vless",
		Server: server,
		Port:   443,
		UUID:   "b831381d-6324-4d53-ad4f-8cda48b30811",
		Outbound: map[string]interface{}{
			"type":        "vless",
			"tag":         tag,
			"server":      server,
			"server_port": 443,
			"uuid":        "b831381d-6324-4d53-ad4f-8cda48b30811",
		},
		SourceIndex: configtypes.UnsetSourceIndex,
	}
}

// decodeEmitted снимает обрамление генератора и декодирует объект.
func decodeEmitted(t *testing.T, emitted string) map[string]interface{} {
	t.Helper()
	start := strings.Index(emitted, "{")
	if start < 0 {
		t.Fatalf("emitted fragment has no object: %s", emitted)
	}
	body := strings.TrimSuffix(strings.TrimSpace(emitted[start:]), ",")

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		t.Fatalf("emitted fragment is not valid JSON: %v\n%s", err, emitted)
	}
	return obj
}

// Узел-группа эмитится настоящим selector/urltest без server/server_port.
func TestGroupNodeEmitsAsSingboxGroup(t *testing.T) {
	emitted, err := GenerateNodeJSON(groupNode("auto", "n1", "n2"))
	if err != nil {
		t.Fatalf("GenerateNodeJSON() error: %v", err)
	}

	obj := decodeEmitted(t, emitted)

	if obj["type"] != "urltest" {
		t.Errorf("type = %v, want urltest", obj["type"])
	}
	if obj["tag"] != "auto" {
		t.Errorf("tag = %v, want auto", obj["tag"])
	}
	// Группа не соединение: этих полей быть не должно, иначе ядро отвергнет.
	if _, present := obj["server"]; present {
		t.Error("group must not emit a server field")
	}
	if _, present := obj["server_port"]; present {
		t.Error("group must not emit a server_port field")
	}

	members, ok := obj["outbounds"].([]interface{})
	if !ok || len(members) != 2 {
		t.Fatalf("outbounds = %v, want two members", obj["outbounds"])
	}
	if members[0] != "n1" || members[1] != "n2" {
		t.Errorf("member order changed: %v", members)
	}
	if obj["url"] != "https://www.gstatic.com/generate_204" || obj["interval"] != "5m" {
		t.Errorf("group options not emitted: %v", obj)
	}
}

// Пустая группа не эмитится: sing-box отказывается стартовать на urltest без
// членов. Последний рубеж — парсер отбрасывает такие группы раньше.
func TestGroupNodeWithoutMembersIsRejected(t *testing.T) {
	if _, err := GenerateNodeJSON(groupNode("auto")); err == nil {
		t.Fatal("emitting a memberless group must fail")
	}

	broken := groupNode("auto", "n1")
	delete(broken.Outbound, "type")
	if _, err := GenerateNodeJSON(broken); err == nil {
		t.Fatal("emitting a typeless group must fail")
	}
}

// Каналы (вкладка Outbounds) не затронуты: узел-группа не превращается в
// селектор конфигуратора и не меняет ParserConfig.
func TestGroupNodeDoesNotTouchOutboundChannels(t *testing.T) {
	parserConfig := &ParserConfig{}
	parserConfig.ParserConfig.Proxies = []ProxySource{{
		Source: "https://example.invalid/sub",
	}}
	parserConfig.ParserConfig.Outbounds = []Direction{{
		Tag:     "proxy-out",
		Type:    "selector",
		Filters: map[string]interface{}{"tag": "/./"},
	}}

	nodesBefore := len(parserConfig.ParserConfig.Outbounds)
	localBefore := len(parserConfig.ParserConfig.Proxies[0].Outbounds)

	loader := func(ProxySource, map[string]int, func(float64, string), int, int) ([]*ParsedNode, error) {
		return []*ParsedNode{
			plainNode("n1", "a.example.com"),
			plainNode("n2", "b.example.com"),
			groupNode("auto", "n1", "n2"),
		}, nil
	}

	res, err := GenerateOutboundsFromParserConfig(parserConfig, map[string]int{}, nil, loader, DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("GenerateOutboundsFromParserConfig() error: %v", err)
	}

	if got := len(parserConfig.ParserConfig.Outbounds); got != nodesBefore {
		t.Fatalf("global outbounds changed: %d → %d", nodesBefore, got)
	}
	if got := len(parserConfig.ParserConfig.Proxies[0].Outbounds); got != localBefore {
		t.Fatalf("source-local outbounds changed: %d → %d", localBefore, got)
	}

	// Узел-группа попал в outbounds конфига, а не в endpoints.
	joined := strings.Join(res.OutboundsJSON, "\n")
	if !strings.Contains(joined, `"tag":"auto"`) {
		t.Fatalf("group node missing from outbounds section:\n%s", joined)
	}
	if strings.Contains(strings.Join(res.EndpointsJSON, "\n"), `"tag":"auto"`) {
		t.Fatal("group node must not land in the endpoints section")
	}

	// Три ноды: две обычные + группа. Группа считается нодой, не селектором.
	if res.NodesCount != 3 {
		t.Errorf("NodesCount = %d, want 3", res.NodesCount)
	}
}

// Селектор-канал видит узел-группу как обычную ноду пула и может её включить.
func TestGroupNodeIsSelectableByChannelFilter(t *testing.T) {
	parserConfig := &ParserConfig{}
	parserConfig.ParserConfig.Proxies = []ProxySource{{Source: "https://example.invalid/sub"}}
	parserConfig.ParserConfig.Outbounds = []Direction{{
		Tag:     "proxy-out",
		Type:    "selector",
		Filters: map[string]interface{}{"tag": "/auto/i"},
	}}

	loader := func(ProxySource, map[string]int, func(float64, string), int, int) ([]*ParsedNode, error) {
		return []*ParsedNode{
			plainNode("n1", "a.example.com"),
			groupNode("auto", "n1"),
		}, nil
	}

	res, err := GenerateOutboundsFromParserConfig(parserConfig, map[string]int{}, nil, loader, DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("GenerateOutboundsFromParserConfig() error: %v", err)
	}

	joined := strings.Join(res.OutboundsJSON, "\n")
	if !strings.Contains(joined, `"proxy-out"`) {
		t.Fatalf("channel selector missing:\n%s", joined)
	}
	// Канал отобрал группу по фильтру — она рядовая нода пула.
	if !strings.Contains(joined, `"auto"`) {
		t.Fatalf("group node not visible to channel filter:\n%s", joined)
	}
}

// SPEC 112: узел-группа идентичности НЕ имеет. Отметок выключения у групп нет,
// а хопом цепочки группа быть не может — для этого есть DetourTag (SPEC 077).
func TestGroupNodeHasNoIdentity(t *testing.T) {
	if got := NodeIdentity(groupNode("auto", "n1", "n2")); got != "" {
		t.Fatalf("идентичность узла-группы = %q, ожидалась пустая", got)
	}
}

// Round-trip через JSON (как в state.json) не искажает узел-группу.
func TestGroupNodeSurvivesJSONRoundTrip(t *testing.T) {
	original := groupNode("auto", "n1", "n2")

	data, err := json.Marshal(original.Outbound)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var restoredOutbound map[string]interface{}
	if err := json.Unmarshal(data, &restoredOutbound); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	restored := &ParsedNode{
		Tag:      original.Tag,
		Scheme:   original.Scheme,
		Label:    original.Label,
		Outbound: restoredOutbound,
	}

	beforeJSON, err := GenerateNodeJSON(original)
	if err != nil {
		t.Fatalf("emit original: %v", err)
	}
	afterJSON, err := GenerateNodeJSON(restored)
	if err != nil {
		t.Fatalf("emit restored: %v", err)
	}
	if beforeJSON != afterJSON {
		t.Fatalf("round-trip changed the emitted group:\nbefore: %s\nafter:  %s", beforeJSON, afterJSON)
	}

	if original.Tag != restored.Tag {
		t.Fatal("round-trip changed the group tag")
	}
}

// Потребители, ожидающие server/port, не должны падать на узле-группе.
func TestGroupNodeIsSafeForServerlessConsumers(t *testing.T) {
	node := groupNode("auto", "n1")

	if node.Server != "" {
		t.Errorf("group node server = %q, want empty", node.Server)
	}
	if node.Port != 0 {
		t.Errorf("group node port = %d, want 0", node.Port)
	}

	// sanitizeNodeDetours обходит весь список и читает Outbound["detour"].
	sanitizeNodeDetours([]*ParsedNode{node, plainNode("n1", "a.example.com")})

	// chainOfNode на группе без цепочки возвращает пусто, а не паникует.
	if chain := chainOfNode(node); len(chain) != 0 {
		t.Errorf("group node chain = %v, want empty", chain)
	}
}
