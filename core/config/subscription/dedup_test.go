package subscription

import (
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 094 D3 — дедупликация внутри источника.
//
// Хук NodeIdentityHashFunc в тестах пакета subscription не установлен
// приложением, поэтому подставляется здесь. Это же проверяет контракт «парсер
// работоспособен в изоляции»: без хука дедуп просто не выполняется.

// withIdentityHash подставляет хук на время теста.
//
// Тестовая реализация намеренно НЕ повторяет продовую (sha256 от эмиссии):
// здесь важно поведение дедупа, а не конкретная функция. Ключ строится из
// полей, описывающих подключение, включая SNI — как и продовый хеш.
func withIdentityHash(t *testing.T) {
	t.Helper()
	prev := NodeIdentityHashFunc
	NodeIdentityHashFunc = func(node *configtypes.ParsedNode) string {
		if node == nil {
			return ""
		}
		parts := []string{node.Scheme, node.Server, node.UUID}
		if node.Outbound != nil {
			if tls, ok := node.Outbound["tls"].(map[string]interface{}); ok {
				if sni, ok := tls["server_name"].(string); ok {
					parts = append(parts, sni)
				}
			}
		}
		return strings.Join(parts, "|")
	}
	t.Cleanup(func() { NodeIdentityHashFunc = prev })
}

// Критерий 19: один сервер, повторённый трижды с разными ремарками, даёт одну ноду.
func TestDedupCollapsesRepeatedServerInURIList(t *testing.T) {
	withIdentityHash(t)

	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com"
	body := strings.Join([]string{
		uri + "#🇳🇱 NL-1",
		uri + "#🇳🇱 Amsterdam",
		uri + "#🇳🇱 Fast",
	}, "\n")

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	if len(res.Nodes) != 1 {
		got := make([]string, 0, len(res.Nodes))
		for _, n := range res.Nodes {
			got = append(got, n.Tag)
		}
		t.Fatalf("got %d nodes, want 1 (tags: %v)", len(res.Nodes), got)
	}
	// Выживает первый по порядку — его имя пользователь и увидит.
	if res.Nodes[0].Tag != "🇳🇱 NL-1" {
		t.Errorf("surviving node = %q, want the first one", res.Nodes[0].Tag)
	}
}

// Разные серверы не схлопываются.
func TestDedupKeepsDistinctServers(t *testing.T) {
	withIdentityHash(t)

	body := strings.Join([]string{
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@a.com:443?security=tls&sni=a.com#A",
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@b.com:443?security=tls&sni=b.com#B",
	}, "\n")

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	if len(res.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2", len(res.Nodes))
	}
}

// Ключевое отличие от LxBox: один сервер с разными SNI — РАЗНЫЕ ноды.
// Это запасные варианты обхода блокировки, схлопывать их нельзя.
func TestDedupKeepsSameServerWithDifferentSNI(t *testing.T) {
	withIdentityHash(t)

	body := strings.Join([]string{
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=www.microsoft.com#MS",
		"vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=www.cloudflare.com#CF",
	}, "\n")

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	if len(res.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2 — different SNI are different nodes", len(res.Nodes))
	}
}

// Дедуп работает и для импортированных sing-box конфигов.
func TestDedupCollapsesDuplicatesInSingboxImport(t *testing.T) {
	withIdentityHash(t)

	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"first","server":"e.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"second","server":"e.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"other","server":"x.com","server_port":443,"uuid":"u1"}
	  ]
	}`
	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	if len(res.Nodes) != 2 {
		got := make([]string, 0, len(res.Nodes))
		for _, n := range res.Nodes {
			got = append(got, n.Tag)
		}
		t.Fatalf("got %d nodes, want 2 (tags: %v)", len(res.Nodes), got)
	}
	if res.Nodes[0].Tag != "first" {
		t.Errorf("surviving node = %q, want first", res.Nodes[0].Tag)
	}
}

// Дедуп не должен ломать состав узла-группы: если её член схлопнулся,
// группа обязана указывать на выжившего.
func TestDedupKeepsGroupMembershipConsistent(t *testing.T) {
	withIdentityHash(t)

	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"a","server":"e.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"a-dup","server":"e.com","server_port":443,"uuid":"u1"},
	    {"type":"vless","tag":"b","server":"x.com","server_port":443,"uuid":"u1"},
	    {"type":"urltest","tag":"auto","outbounds":["a","a-dup","b"]}
	  ]
	}`
	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	groups := groupNodesOf(res.Nodes)
	if len(groups) != 1 {
		t.Fatalf("got %d group nodes, want 1", len(groups))
	}

	finalTags := map[string]bool{}
	for _, n := range res.Nodes {
		finalTags[n.Tag] = true
	}
	members := groupMembersOf(groups[0])
	if len(members) == 0 {
		t.Fatal("group lost all members after dedup")
	}
	for _, m := range members {
		if !finalTags[m] {
			t.Fatalf("group references %q, which is not an emitted node (members: %v)", m, members)
		}
	}
}

// Без хука дедуп не выполняется: парсер обязан работать в изоляции.
func TestDedupIsNoopWithoutHook(t *testing.T) {
	prev := NodeIdentityHashFunc
	NodeIdentityHashFunc = nil
	t.Cleanup(func() { NodeIdentityHashFunc = prev })

	const uri = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com"
	body := uri + "#one\n" + uri + "#two"

	res := loadFromInlineBody(t, body, configtypes.ProxySource{})

	if len(res.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2 — dedup must be inert without the hook", len(res.Nodes))
	}
}

// Узел без вычислимой идентичности не должен схлопываться с другими такими же.
func TestDedupKeepsNodesWithoutIdentity(t *testing.T) {
	prev := NodeIdentityHashFunc
	NodeIdentityHashFunc = func(*configtypes.ParsedNode) string { return "" }
	t.Cleanup(func() { NodeIdentityHashFunc = prev })

	nodes := []*configtypes.ParsedNode{
		{Tag: "a", Scheme: "vless"},
		{Tag: "b", Scheme: "vless"},
	}
	if got := dedupNodesByIdentity(nodes); len(got) != 2 {
		t.Fatalf("got %d nodes, want 2 — empty identity must not collapse nodes", len(got))
	}
}
