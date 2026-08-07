package subscription

import (
	"fmt"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 094 фаза B — detour-цепочки при импорте sing-box конфига.

func chainTagsOf(node *configtypes.ParsedNode) []string {
	out := make([]string, 0, len(node.Chain))
	for _, hop := range node.Chain {
		out = append(out, hop.Tag)
	}
	return out
}

func findNodeByTag(res *SingboxImportResult, tag string) *configtypes.ParsedNode {
	for _, n := range res.Nodes {
		if n.Tag == tag {
			return n
		}
	}
	return nil
}

// Критерий 9: цепочка A→B→C даёт один узел с двумя хопами.
func TestDetourChainTwoHops(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"A","server":"a.com","server_port":443,"uuid":"u1","detour":"B"},
	    {"type":"vless","tag":"B","server":"b.com","server_port":443,"uuid":"u2","detour":"C"},
	    {"type":"socks","tag":"C","server":"c.com","server_port":1080}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	// B и C — цели чужого detour, самостоятельными узлами не становятся.
	if len(res.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 (tags: %v)", len(res.Nodes), tagsOf(res))
	}
	node := res.Nodes[0]
	if node.Tag != "A" {
		t.Fatalf("node tag = %q, want A", node.Tag)
	}

	got := chainTagsOf(node)
	if len(got) != 2 || got[0] != "B" || got[1] != "C" {
		t.Fatalf("chain = %v, want [B C]", got)
	}

	// Deprecated Jump синхронизирован с первым хопом.
	if node.Jump == nil || node.Jump.Tag != "B" {
		t.Fatalf("Jump must mirror Chain[0], got %+v", node.Jump)
	}
}

// Критерий 10: цепочка глубже предела усекается, узел остаётся рабочим.
func TestDetourChainDepthLimit(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`{"outbounds":[`)
	const total = 12
	for i := 0; i < total; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		detour := ""
		if i < total-1 {
			detour = fmt.Sprintf(`,"detour":"h%d"`, i+1)
		}
		fmt.Fprintf(&sb, `{"type":"socks","tag":"h%d","server":"h%d.com","server_port":1080%s}`, i, i, detour)
	}
	sb.WriteString(`]}`)

	res := parseSingboxBodyForTest(t, sb.String())

	if len(res.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 (tags: %v)", len(res.Nodes), tagsOf(res))
	}
	node := res.Nodes[0]
	if len(node.Chain) != maxDetourChainDepth {
		t.Fatalf("chain length = %d, want %d", len(node.Chain), maxDetourChainDepth)
	}
	if node.Tag != "h0" {
		t.Fatalf("head node = %q, want h0", node.Tag)
	}
}

// Критерий 11: кольцо A→B→A даёт ОБЕ ноды; ни одна не теряется.
//
// Это главный кейс порядка вычислений (SPEC B3): если сначала собрать цели
// detour, а кольца искать после, узел внутри кольца окажется целью и исчезнет.
func TestDetourChainCycleKeepsBothNodes(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"A","server":"a.com","server_port":443,"uuid":"u1","detour":"B"},
	    {"type":"vless","tag":"B","server":"b.com","server_port":443,"uuid":"u2","detour":"A"}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2 (tags: %v)", len(res.Nodes), tagsOf(res))
	}

	// Кольцо разорвано: суммарно цепочек не больше одной ссылки,
	// и ни один узел не ссылается сам на себя транзитивно бесконечно.
	for _, n := range res.Nodes {
		for _, hop := range n.Chain {
			if hop.Tag == n.Tag {
				t.Fatalf("node %q has itself in its chain: %v", n.Tag, chainTagsOf(n))
			}
		}
	}
}

// Самоссылка detour → узел живёт без цепочки.
func TestDetourChainSelfReference(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"A","server":"a.com","server_port":443,"uuid":"u1","detour":"A"}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 (tags: %v)", len(res.Nodes), tagsOf(res))
	}
	if len(res.Nodes[0].Chain) != 0 {
		t.Fatalf("self-detour must not build a chain, got %v", chainTagsOf(res.Nodes[0]))
	}
}

// Критерий 12: detour на несуществующий тег — узел живёт, дозванивается напрямую.
func TestDetourChainDanglingTarget(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"A","server":"a.com","server_port":443,"uuid":"u1","detour":"ghost"}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1", len(res.Nodes))
	}
	if len(res.Nodes[0].Chain) != 0 {
		t.Fatalf("dangling detour must not build a chain, got %v", chainTagsOf(res.Nodes[0]))
	}
}

// detour на служебный тип (direct) завершает цепочку молча.
func TestDetourChainToServiceType(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"A","server":"a.com","server_port":443,"uuid":"u1","detour":"direct"},
	    {"type":"direct","tag":"direct"}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 (tags: %v)", len(res.Nodes), tagsOf(res))
	}
	if len(res.Nodes[0].Chain) != 0 {
		t.Fatalf("detour to a service type must not build a chain, got %v", chainTagsOf(res.Nodes[0]))
	}
}

// detour на группу цепочку не строит: развёрнутая группа приехала бы без членов.
func TestDetourChainToGroup(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"A","server":"a.com","server_port":443,"uuid":"u1","detour":"auto"},
	    {"type":"vless","tag":"B","server":"b.com","server_port":443,"uuid":"u2"},
	    {"type":"urltest","tag":"auto","outbounds":["B"]}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	nodeA := findNodeByTag(res, "A")
	if nodeA == nil {
		t.Fatalf("node A must survive (tags: %v)", tagsOf(res))
	}
	if len(nodeA.Chain) != 0 {
		t.Fatalf("detour to a group must not build a chain, got %v", chainTagsOf(nodeA))
	}
}

// Два узла законно ссылаются на один и тот же джамп.
func TestDetourChainSharedHop(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"A","server":"a.com","server_port":443,"uuid":"u1","detour":"J"},
	    {"type":"vless","tag":"B","server":"b.com","server_port":443,"uuid":"u2","detour":"J"},
	    {"type":"socks","tag":"J","server":"j.com","server_port":1080}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 2 {
		t.Fatalf("got %d nodes, want 2 (tags: %v)", len(res.Nodes), tagsOf(res))
	}
	for _, n := range res.Nodes {
		got := chainTagsOf(n)
		if len(got) != 1 || got[0] != "J" {
			t.Fatalf("node %q chain = %v, want [J]", n.Tag, got)
		}
	}
}

// Критерий 13: старая форма state.json (только Jump) не теряет хоп.
func TestAdoptLegacyJumpMigration(t *testing.T) {
	node := &configtypes.ParsedNode{
		Tag:    "main",
		Scheme: "vless",
		Jump: &configtypes.ParsedJump{
			Tag:      "legacy-hop",
			Scheme:   "socks",
			Server:   "hop.com",
			Port:     1080,
			Outbound: map[string]interface{}{"version": "5"},
		},
	}

	node.AdoptLegacyJump()

	if len(node.Chain) != 1 {
		t.Fatalf("chain length = %d, want 1", len(node.Chain))
	}
	hop := node.Chain[0]
	if hop.Tag != "legacy-hop" || hop.Scheme != "socks" || hop.Server != "hop.com" || hop.Port != 1080 {
		t.Fatalf("migrated hop = %+v", hop)
	}
}

func TestAdoptLegacyJumpIsNoopWhenChainPresent(t *testing.T) {
	node := &configtypes.ParsedNode{
		Tag:   "main",
		Chain: []*configtypes.ParsedNode{{Tag: "real-hop"}},
		Jump:  &configtypes.ParsedJump{Tag: "stale"},
	}

	node.AdoptLegacyJump()

	if len(node.Chain) != 1 || node.Chain[0].Tag != "real-hop" {
		t.Fatalf("existing chain must win, got %v", chainTagsOf(node))
	}
}

func TestSyncJumpFromChain(t *testing.T) {
	node := &configtypes.ParsedNode{
		Tag: "main",
		Chain: []*configtypes.ParsedNode{
			{Tag: "hop1", Scheme: "socks", Server: "h1.com", Port: 1080},
			{Tag: "hop2", Scheme: "vless", Server: "h2.com", Port: 443},
		},
	}

	node.SyncJumpFromChain()

	if node.Jump == nil || node.Jump.Tag != "hop1" || node.Jump.Port != 1080 {
		t.Fatalf("Jump must mirror Chain[0], got %+v", node.Jump)
	}

	node.Chain = nil
	node.SyncJumpFromChain()
	if node.Jump != nil {
		t.Fatalf("empty chain must clear Jump, got %+v", node.Jump)
	}
}
