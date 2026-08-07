package subscription

import (
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// helper: разбор тела с автоклассификацией.
func parseSingboxBodyForTest(t *testing.T, body string) *SingboxImportResult {
	t.Helper()
	kind := ClassifySubscriptionBody(body)
	if !kind.IsSingbox() {
		t.Fatalf("body classified as %v, expected a sing-box form", kind)
	}
	res, err := ParseSingboxBody(body, kind, nil)
	if err != nil {
		t.Fatalf("ParseSingboxBody() error: %v", err)
	}
	return res
}

func tagsOf(res *SingboxImportResult) []string {
	out := make([]string, 0, len(res.Nodes))
	for _, n := range res.Nodes {
		out = append(out, n.Tag)
	}
	return out
}

// Критерий 1: одиночный outbound даёт одну ноду.
func TestSingboxImportSingleOutbound(t *testing.T) {
	body := `{"type":"vless","tag":"node-a","server":"example.com","server_port":443,"uuid":"11111111-1111-1111-1111-111111111111"}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 (tags: %v)", len(res.Nodes), tagsOf(res))
	}
	n := res.Nodes[0]
	if n.Tag != "node-a" || n.Scheme != "vless" || n.Server != "example.com" || n.Port != 443 {
		t.Fatalf("unexpected node: tag=%q scheme=%q server=%q port=%d", n.Tag, n.Scheme, n.Server, n.Port)
	}
	if n.UUID != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("UUID not lifted from map: %q", n.UUID)
	}
}

// Критерий 2: массив outbound'ов даёт по ноде на элемент.
func TestSingboxImportOutboundArray(t *testing.T) {
	body := `[
	  {"type":"vless","tag":"a","server":"e1.com","server_port":443,"uuid":"u1"},
	  {"type":"trojan","tag":"b","server":"e2.com","server_port":8443,"password":"p2"}
	]`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2 (tags: %v)", len(res.Nodes), tagsOf(res))
	}
	if res.Nodes[0].Scheme != "vless" || res.Nodes[1].Scheme != "trojan" {
		t.Fatalf("unexpected schemes: %q, %q", res.Nodes[0].Scheme, res.Nodes[1].Scheme)
	}
	// trojan: пароль поднимается в UUID (там его читает GenerateNodeJSON).
	if res.Nodes[1].UUID != "p2" {
		t.Errorf("trojan password not lifted: %q", res.Nodes[1].UUID)
	}
}

// Критерий 3: целый конфиг — только outbounds; route/dns/inbounds отмечены как игнор.
func TestSingboxImportWholeConfigIgnoresNonOutboundSections(t *testing.T) {
	body := `{
	  "log":{"level":"info"},
	  "dns":{"servers":[{"address":"8.8.8.8"}]},
	  "inbounds":[{"type":"tun","tag":"tun-in"}],
	  "route":{"rules":[{"outbound":"direct"}]},
	  "outbounds":[
	    {"type":"vless","tag":"a","server":"e1.com","server_port":443,"uuid":"u1"},
	    {"type":"direct","tag":"direct"},
	    {"type":"block","tag":"block"}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 (tags: %v)", len(res.Nodes), tagsOf(res))
	}
	if res.Nodes[0].Tag != "a" {
		t.Errorf("unexpected node tag %q", res.Nodes[0].Tag)
	}

	want := []string{"route", "dns", "inbounds"}
	if len(res.IgnoredSections) != len(want) {
		t.Fatalf("ignored sections = %v, want %v", res.IgnoredSections, want)
	}
	for i, section := range want {
		if res.IgnoredSections[i] != section {
			t.Fatalf("ignored sections = %v, want %v", res.IgnoredSections, want)
		}
	}
}

// Критерий 4: endpoints разбираются наравне с outbounds.
func TestSingboxImportEndpointsSection(t *testing.T) {
	body := `{
	  "endpoints":[
	    {"type":"wireguard","tag":"wg","address":["10.0.0.2/32"],"private_key":"key",
	     "peers":[{"address":"1.2.3.4","port":51820,"public_key":"pub","allowed_ips":["0.0.0.0/0"]}]}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 (tags: %v)", len(res.Nodes), tagsOf(res))
	}
	if res.Nodes[0].Scheme != "wireguard" {
		t.Fatalf("scheme = %q, want wireguard", res.Nodes[0].Scheme)
	}
	// wireguard не имеет server/server_port на верхнем уровне — общая
	// проверка адреса к нему не применяется.
	if res.Nodes[0].Tag != "wg" {
		t.Errorf("tag = %q, want wg", res.Nodes[0].Tag)
	}
}

// Критерий 5: urltest приезжает ОБЫЧНЫМ узлом списка, а не записью вкладки
// Outbounds. Четыре ноды: три сервера плюс узел-группа.
func TestSingboxImportGroupBecomesOrdinaryNode(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"a","server":"e1.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"b","server":"e2.com","server_port":443,"uuid":"u2"},
	    {"type":"vless","tag":"c","server":"e3.com","server_port":443,"uuid":"u3"},
	    {"type":"urltest","tag":"auto","outbounds":["a","b","c"],"url":"https://example.com","interval":"5m"}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 4 {
		t.Fatalf("got %d nodes, want 4 (tags: %v)", len(res.Nodes), tagsOf(res))
	}

	groups := groupNodesOf(res.Nodes)
	if len(groups) != 1 {
		t.Fatalf("got %d group nodes, want 1", len(groups))
	}
	g := groups[0]
	if g.Tag != "auto" {
		t.Fatalf("group tag = %q, want auto", g.Tag)
	}
	if g.Scheme != configtypes.SchemeGroup {
		t.Fatalf("group scheme = %q, want %q", g.Scheme, configtypes.SchemeGroup)
	}
	if g.Outbound["type"] != "urltest" {
		t.Fatalf("group type = %v, want urltest", g.Outbound["type"])
	}
	if members := groupMembersOf(g); len(members) != 3 {
		t.Fatalf("group members = %v, want 3", members)
	}
	if g.Outbound["url"] != "https://example.com" || g.Outbound["interval"] != "5m" {
		t.Errorf("group options not carried over: %v", g.Outbound)
	}
	// Узел-группа идёт последней: группы эмитятся после узлов (A7).
	if res.Nodes[3].Tag != "auto" {
		t.Errorf("group must come after plain nodes, got order %v", tagsOf(res))
	}
}

// Критерий 6: группа без разрешимых членов не создаётся (пустой urltest роняет ядро).
func TestSingboxImportGroupWithNoResolvableMembersSkipped(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"a","server":"e1.com","server_port":443,"uuid":"u1"},
	    {"type":"urltest","tag":"auto","outbounds":["ghost-1","ghost-2"]}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if got := len(groupNodesOf(res.Nodes)); got != 0 {
		t.Fatalf("got %d group nodes, want 0", got)
	}
	if len(res.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(res.Nodes))
	}
}

// Селектор со ссылками на служебные типы теряет их, но живёт на оставшихся.
func TestSingboxImportSelectorDropsServiceMembers(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"a","server":"e1.com","server_port":443,"uuid":"u1"},
	    {"type":"direct","tag":"direct"},
	    {"type":"selector","tag":"select","outbounds":["a","direct"],"default":"a"}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	groups := groupNodesOf(res.Nodes)
	if len(groups) != 1 {
		t.Fatalf("got %d group nodes, want 1", len(groups))
	}
	g := groups[0]
	if members := groupMembersOf(g); len(members) != 1 || members[0] != "a" {
		t.Fatalf("group members = %v, want [a]", members)
	}
	if g.Outbound["default"] != "a" {
		t.Errorf("valid default should be carried over, got %v", g.Outbound["default"])
	}
}

// default, не входящий в состав, не переносится: ядро отвергло бы такой селектор.
func TestSingboxImportSelectorDropsDanglingDefault(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"a","server":"e1.com","server_port":443,"uuid":"u1"},
	    {"type":"selector","tag":"select","outbounds":["a"],"default":"ghost"}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	groups := groupNodesOf(res.Nodes)
	if len(groups) != 1 {
		t.Fatalf("got %d group nodes, want 1", len(groups))
	}
	if _, present := groups[0].Outbound["default"]; present {
		t.Errorf("dangling default must not be carried over: %v", groups[0].Outbound)
	}
}

// Критерий 7: один битый outbound из пяти не роняет остальные.
func TestSingboxImportBrokenEntryDoesNotKillSiblings(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"a","server":"e1.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"broken-no-server","server_port":443,"uuid":"u2"},
	    {"type":"vless","tag":"b","server":"e2.com","server_port":443,"uuid":"u3"},
	    {"type":"quantum-teleport","tag":"unknown-proto","server":"e3.com","server_port":443},
	    {"type":"trojan","tag":"c","server":"e4.com","server_port":443,"password":"p"}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	got := tagsOf(res)
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("tags = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tags = %v, want %v", got, want)
		}
	}

	// Неподдержанный тип не исчезает молча.
	if len(res.UnsupportedTypes) == 0 {
		t.Fatal("unsupported types must be reported")
	}
	found := false
	for _, tp := range res.UnsupportedTypes {
		if tp == "quantum-teleport" {
			found = true
		}
	}
	if !found {
		t.Errorf("unsupported types = %v, want to include quantum-teleport", res.UnsupportedTypes)
	}
}

// Массив конфигов: узлы всех элементов, в порядке появления (A7).
func TestSingboxImportConfigArrayPreservesOrder(t *testing.T) {
	body := `[
	  {"outbounds":[{"type":"vless","tag":"first","server":"e1.com","server_port":443,"uuid":"u1"}]},
	  {"outbounds":[{"type":"trojan","tag":"second","server":"e2.com","server_port":443,"password":"p"}]}
	]`
	res := parseSingboxBodyForTest(t, body)

	got := tagsOf(res)
	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("tags = %v, want [first second]", got)
	}
}

// Узел без тега получает синтетический, а не теряется.
func TestSingboxImportUntaggedEntryGetsSyntheticTag(t *testing.T) {
	body := `{"outbounds":[{"type":"vless","server":"e1.com","server_port":443,"uuid":"u1"}]}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(res.Nodes))
	}
	if res.Nodes[0].Tag == "" {
		t.Fatal("node must receive a synthetic tag")
	}
	if !strings.HasPrefix(res.Nodes[0].Tag, "vless-") {
		t.Errorf("synthetic tag = %q, want vless-* prefix", res.Nodes[0].Tag)
	}
}

// skip-фильтры работают над импортированными узлами так же, как над URI.
func TestSingboxImportRespectsSkipFilters(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"keep","server":"good.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"drop-me","server":"bad.com","server_port":443,"uuid":"u2"}
	  ]
	}`
	kind := ClassifySubscriptionBody(body)
	res, err := ParseSingboxBody(body, kind, []map[string]string{{"tag": "drop-me"}})
	if err != nil {
		t.Fatalf("ParseSingboxBody() error: %v", err)
	}

	got := tagsOf(res)
	if len(got) != 1 || got[0] != "keep" {
		t.Fatalf("tags = %v, want [keep]", got)
	}
}
