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

// rejectedCodesOf — коды отбракованных записей импорта в порядке файла.
func rejectedCodesOf(res *SingboxImportResult) []string {
	out := make([]string, 0)
	for _, r := range res.RejectedRecords() {
		out = append(out, r.Code)
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

// Критерий 11 (контракт 1.1.118, fail-closed): кольцо A→B→A — ОБЕ ноды
// отбраковываются записями с кодом detour_cycle_broken; ни одна не исчезает
// молча и ни одна не остаётся рабочей «напрямую».
//
// Порядок вычислений (SPEC B3) по-прежнему важен: если сначала собрать цели
// detour, а кольца искать после, узел внутри кольца окажется чужим джампом и
// пропадёт без записи.
func TestDetourChainCycleDropsBothNodes(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"A","server":"a.com","server_port":443,"uuid":"u1","detour":"B"},
	    {"type":"vless","tag":"B","server":"b.com","server_port":443,"uuid":"u2","detour":"A"}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 0 {
		t.Fatalf("got %d nodes, want 0 (tags: %v)", len(res.Nodes), tagsOf(res))
	}
	codes := rejectedCodesOf(res)
	if len(codes) != 2 || codes[0] != WarnDetourCycleBroken || codes[1] != WarnDetourCycleBroken {
		t.Fatalf("rejected codes = %v, want two %s", codes, WarnDetourCycleBroken)
	}
}

// Кольцо из трёх узлов A→B→C→A (и X, чья цепь в него ведёт): КАЖДЫЙ участник
// отбраковывается своей записью — ни один не прячется как цель чужой цепи
// (расхождение с LxBox, замеченное на синке 1.1.118: лаунчер терял B молча).
func TestDetourChainCycleOfThreeDropsEveryParticipant(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"X","server":"x.com","server_port":443,"uuid":"u0","detour":"A"},
	    {"type":"vless","tag":"A","server":"a.com","server_port":443,"uuid":"u1","detour":"B"},
	    {"type":"vless","tag":"B","server":"b.com","server_port":443,"uuid":"u2","detour":"C"},
	    {"type":"vless","tag":"C","server":"c.com","server_port":443,"uuid":"u3","detour":"A"}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 0 {
		t.Fatalf("got %d nodes, want 0 (tags: %v)", len(res.Nodes), tagsOf(res))
	}
	codes := rejectedCodesOf(res)
	if len(codes) != 4 {
		t.Fatalf("rejected codes = %v, want four %s (one per node)", codes, WarnDetourCycleBroken)
	}
	for _, c := range codes {
		if c != WarnDetourCycleBroken {
			t.Fatalf("rejected codes = %v, want all %s", codes, WarnDetourCycleBroken)
		}
	}
}

// Самоссылка detour → узел отбраковывается как кольцо (fail-closed).
func TestDetourChainSelfReference(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"A","server":"a.com","server_port":443,"uuid":"u1","detour":"A"}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 0 {
		t.Fatalf("got %d nodes, want 0 (tags: %v)", len(res.Nodes), tagsOf(res))
	}
	if codes := rejectedCodesOf(res); len(codes) != 1 || codes[0] != WarnDetourCycleBroken {
		t.Fatalf("rejected codes = %v, want [%s]", codes, WarnDetourCycleBroken)
	}
}

// Критерий 12 (контракт 1.1.118, fail-closed): detour на несуществующий тег —
// узел отбраковывается с кодом detour_target_missing, а не дозванивается
// напрямую.
func TestDetourChainDanglingTarget(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"A","server":"a.com","server_port":443,"uuid":"u1","detour":"ghost"},
	    {"type":"vless","tag":"B","server":"b.com","server_port":443,"uuid":"u2"}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 1 || res.Nodes[0].Tag != "B" {
		t.Fatalf("got nodes %v, want only B", tagsOf(res))
	}
	if codes := rejectedCodesOf(res); len(codes) != 1 || codes[0] != WarnDetourTargetMissing {
		t.Fatalf("rejected codes = %v, want [%s]", codes, WarnDetourTargetMissing)
	}
}

// Хоп, который узлом не стал (неподдержанный тип), — та же недоступная цель:
// носитель отбраковывается с detour_target_missing, а не едет с укороченной
// цепочкой молча.
func TestDetourChainUnusableHop(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"A","server":"a.com","server_port":443,"uuid":"u1","detour":"H"},
	    {"type":"no-such-type","tag":"H","server":"h.com","server_port":1}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if len(res.Nodes) != 0 {
		t.Fatalf("got %d nodes, want 0 (tags: %v)", len(res.Nodes), tagsOf(res))
	}
	if codes := rejectedCodesOf(res); len(codes) != 1 || codes[0] != WarnDetourTargetMissing {
		t.Fatalf("rejected codes = %v, want [%s]", codes, WarnDetourTargetMissing)
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

// detour на группу (контракт 1.1.118, fail-closed): группа хопом быть не может,
// носитель отбраковывается с detour_to_group; член группы и сама группа живут.
func TestDetourChainToGroup(t *testing.T) {
	body := `{
	  "outbounds":[
	    {"type":"vless","tag":"A","server":"a.com","server_port":443,"uuid":"u1","detour":"auto"},
	    {"type":"vless","tag":"B","server":"b.com","server_port":443,"uuid":"u2"},
	    {"type":"urltest","tag":"auto","outbounds":["B"]}
	  ]
	}`
	res := parseSingboxBodyForTest(t, body)

	if findNodeByTag(res, "A") != nil {
		t.Fatalf("node A must be dropped (tags: %v)", tagsOf(res))
	}
	if findNodeByTag(res, "B") == nil || findNodeByTag(res, "auto") == nil {
		t.Fatalf("B and the group must survive (tags: %v)", tagsOf(res))
	}
	if codes := rejectedCodesOf(res); len(codes) != 1 || codes[0] != WarnDetourToGroup {
		t.Fatalf("rejected codes = %v, want [%s]", codes, WarnDetourToGroup)
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
