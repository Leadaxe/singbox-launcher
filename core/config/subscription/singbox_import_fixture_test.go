package subscription

import (
	_ "embed"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// Реальный по форме sing-box конфиг: группы, цепочка, endpoints,
// секции route/dns/inbounds, REALITY с Xray-псевдонимом fingerprint.
//
//go:embed testdata/singbox_full_config.json
var singboxFullConfigFixture string

// SPEC 094 A — сквозной разбор целого конфига (критерии 3–5, 7).
func TestSingboxImportFullConfigFixture(t *testing.T) {
	res := parseSingboxBodyForTest(t, singboxFullConfigFixture)

	t.Run("service types and detour targets are not standalone nodes", func(t *testing.T) {
		// Узлы в порядке появления, затем узлы-группы (A7).
		got := tagsOf(res)
		want := []string{
			"reality-node", "ws-node", "chained-node", "hy2-node", "wg-node",
			"auto-group", "manual-group",
		}

		if len(got) != len(want) {
			t.Fatalf("tags = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("tags = %v, want %v", got, want)
			}
		}
	})

	t.Run("ignored sections are reported", func(t *testing.T) {
		want := map[string]bool{"route": true, "dns": true, "inbounds": true}
		if len(res.IgnoredSections) != len(want) {
			t.Fatalf("ignored = %v, want %v", res.IgnoredSections, want)
		}
		for _, section := range res.IgnoredSections {
			if !want[section] {
				t.Fatalf("unexpected ignored section %q (%v)", section, res.IgnoredSections)
			}
		}
	})

	t.Run("both groups arrive as ordinary nodes with resolvable members", func(t *testing.T) {
		groups := groupNodesOf(res.Nodes)
		if len(groups) != 2 {
			t.Fatalf("got %d group nodes, want 2", len(groups))
		}

		byTag := map[string]*configtypes.ParsedNode{}
		for _, g := range groups {
			byTag[g.Tag] = g
		}

		auto, ok := byTag["auto-group"]
		if !ok {
			t.Fatalf("auto-group missing (%v)", tagsOf(res))
		}
		if auto.Outbound["type"] != "urltest" {
			t.Errorf("auto-group type = %v, want urltest", auto.Outbound["type"])
		}
		if got := groupMembersOf(auto); len(got) != 3 {
			t.Errorf("auto-group members = %v, want 3", got)
		}
		if auto.Outbound["url"] != "https://www.gstatic.com/generate_204" {
			t.Errorf("auto-group url not carried: %v", auto.Outbound["url"])
		}
		// Группа — не соединение: адреса у неё быть не должно.
		if auto.Server != "" || auto.Port != 0 {
			t.Errorf("group node must have no server/port, got %q:%d", auto.Server, auto.Port)
		}

		manual, ok := byTag["manual-group"]
		if !ok {
			t.Fatalf("manual-group missing (%v)", tagsOf(res))
		}
		if manual.Outbound["type"] != "selector" {
			t.Errorf("manual-group type = %v, want selector", manual.Outbound["type"])
		}
		// "direct" — служебный тип, узлом не стал и в состав не попадает.
		if got := groupMembersOf(manual); len(got) != 2 {
			t.Errorf("manual-group members = %v, want 2 (direct dropped)", got)
		}
		if manual.Outbound["default"] != "reality-node" {
			t.Errorf("manual-group default = %v, want reality-node", manual.Outbound["default"])
		}
	})

	t.Run("detour chain is attached to the chained node", func(t *testing.T) {
		node := findNodeByTag(res, "chained-node")
		if node == nil {
			t.Fatal("chained-node missing")
		}
		got := chainTagsOf(node)
		if len(got) != 1 || got[0] != "jump-socks" {
			t.Fatalf("chain = %v, want [jump-socks]", got)
		}
	})

	t.Run("xray utls alias is canonicalized during import", func(t *testing.T) {
		node := findNodeByTag(res, "reality-node")
		if node == nil {
			t.Fatal("reality-node missing")
		}
		tls, ok := node.Outbound["tls"].(map[string]interface{})
		if !ok {
			t.Fatal("reality-node lost its tls block")
		}
		utls, ok := tls["utls"].(map[string]interface{})
		if !ok {
			t.Fatal("reality-node lost its utls block")
		}
		if utls["fingerprint"] != "chrome" {
			t.Errorf("fingerprint = %v, want chrome", utls["fingerprint"])
		}
		if _, ok := tls["reality"]; !ok {
			t.Error("valid REALITY block must survive")
		}
	})

	t.Run("endpoint node keeps its wireguard shape", func(t *testing.T) {
		node := findNodeByTag(res, "wg-node")
		if node == nil {
			t.Fatal("wg-node missing")
		}
		if node.Scheme != "wireguard" {
			t.Fatalf("scheme = %q, want wireguard", node.Scheme)
		}
		if _, ok := node.Outbound["peers"].([]interface{}); !ok {
			t.Error("wireguard peers must survive the import")
		}
	})

	t.Run("hysteria2 obfs survives with a supported type", func(t *testing.T) {
		node := findNodeByTag(res, "hy2-node")
		if node == nil {
			t.Fatal("hy2-node missing")
		}
		if _, ok := node.Outbound["obfs"].(map[string]interface{}); !ok {
			t.Error("salamander obfs must survive")
		}
	})
}

// Импорт не мутирует исходную map конфига: повторный разбор даёт то же самое.
func TestSingboxImportIsRepeatable(t *testing.T) {
	first := parseSingboxBodyForTest(t, singboxFullConfigFixture)
	second := parseSingboxBodyForTest(t, singboxFullConfigFixture)

	firstTags := tagsOf(first)
	secondTags := tagsOf(second)
	if len(firstTags) != len(secondTags) {
		t.Fatalf("node counts differ: %v vs %v", firstTags, secondTags)
	}
	for i := range firstTags {
		if firstTags[i] != secondTags[i] {
			t.Fatalf("tags differ: %v vs %v", firstTags, secondTags)
		}
	}
	if len(groupNodesOf(first.Nodes)) != len(groupNodesOf(second.Nodes)) {
		t.Fatalf("group node counts differ: %d vs %d",
			len(groupNodesOf(first.Nodes)), len(groupNodesOf(second.Nodes)))
	}
}
