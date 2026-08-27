package config

import (
	"strings"
	"testing"
)

// SPEC 113-B — единая строгость detour: недоступная цель ВСЕГДА выбрасывает
// носителя перехода, выпадение каскадирует по графу источников, кольцо
// fail-closed для всех участников.
//
// Формулировки причин тестами не фиксируются побуквенно: проверяется, что
// причина называет тех, кого пользователю нужно узнать (SPEC 112-A).

// cascadeNode — узел источника i с заданным identity-тегом и финальным тегом.
func cascadeNode(t *testing.T, uri, identity, final string, sourceIndex int) *ParsedNode {
	t.Helper()
	n := parseNodeForTagDetour(t, uri)
	n.IdentityTag = identity
	n.Tag = final
	if n.Outbound != nil {
		n.Outbound["tag"] = final
	}
	n.SourceIndex = sourceIndex
	return n
}

func excludedByID(list []SourceExclusion, id string) (SourceExclusion, bool) {
	for _, e := range list {
		if e.SourceID == id {
			return e, true
		}
	}
	return SourceExclusion{}, false
}

func hasDetour(n *ParsedNode) bool {
	if n == nil || n.Outbound == nil {
		return false
	}
	d, _ := n.Outbound["detour"].(string)
	return strings.TrimSpace(d) != ""
}

// Тест 1 (SPEC-B): A битый (его хоп не нашёлся), B ходит через узел из A.
// Выпадают ОБА, в реестре две записи с причинами, узлов B в конфиге нет, и
// нигде не снят detour — молчаливого direct-дозвона не появилось.
//
// На старом коде B оставался: его ссылка разрешалась по карте, построенной до
// выпадения A, узлы B ехали с detour на исчезнувший тег, а спасал их
// граф-санитайзер снятием detour.
func TestResolveNodeDetours_CascadeThroughDroppedSource(t *testing.T) {
	hopA := cascadeNode(t, tagDetourHopURI, "hop-a", "hop-a", 0)
	nodeB := cascadeNode(t, tagDetourChainedURI, "node-b", "node-b", 1)

	pc := tagDetourParserConfig(t,
		// A сам задетурен на узел, которого нет → A выпадает.
		ProxySource{ID: "01A", Label: "Alpha", Connections: []string{tagDetourHopURI},
			DetourNodeSourceID: "01GONE", DetourNodeTag: "исчезнувший хоп"},
		// B ходит через узел источника A.
		ProxySource{ID: "01B", Label: "Bravo", Connections: []string{"..."},
			DetourNodeSourceID: "01A", DetourNodeTag: "hop-a"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {hopA}, 1: {nodeB}}
	all, excluded := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{hopA, nodeB})

	if len(all) != 0 {
		tags := make([]string, 0, len(all))
		for _, n := range all {
			tags = append(tags, n.Tag)
		}
		t.Fatalf("каскад не сработал: в конфиге остались узлы %v", tags)
	}
	if len(excluded) != 2 {
		t.Fatalf("в реестре %d записей, ожидались две (Alpha и Bravo): %+v", len(excluded), excluded)
	}
	if _, ok := excludedByID(excluded, "01A"); !ok {
		t.Error("источник Alpha не попал в реестр")
	}
	eB, ok := excludedByID(excluded, "01B")
	if !ok {
		t.Fatal("зависимый источник Bravo не попал в реестр")
	}
	// Причина каскада обязана назвать промежуточный источник — иначе выпадение
	// Bravo выглядит беспричинным.
	if !strings.Contains(eB.Reason, "Alpha") {
		t.Errorf("причина каскада %q не называет источник, из-за которого исчез хоп", eB.Reason)
	}
	if hasDetour(nodeB) {
		t.Error("detour не имеет права остаться на узле выпавшего источника")
	}
	if hasDetour(hopA) {
		t.Error("detour не имеет права быть снятым/проставленным на узле выпавшего источника")
	}
}

// Тест 2 (SPEC-B): цепочка A←B←C — выпадают все три, причины честные:
// у корня (A) — про его собственную ссылку, у B и C — про источник выше.
func TestResolveNodeDetours_CascadeChainOfThree(t *testing.T) {
	a := cascadeNode(t, tagDetourHopURI, "hop-a", "hop-a", 0)
	b := cascadeNode(t, tagDetourChainedURI, "node-b", "node-b", 1)
	c := cascadeNode(t, tagDetourChainedURI, "node-c", "node-c", 2)

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01A", Label: "Alpha", Connections: []string{tagDetourHopURI},
			DetourNodeSourceID: "01GONE", DetourNodeTag: "исчезнувший хоп"},
		ProxySource{ID: "01B", Label: "Bravo", Connections: []string{"..."},
			DetourNodeSourceID: "01A", DetourNodeTag: "hop-a"},
		ProxySource{ID: "01C", Label: "Charlie", Connections: []string{"..."},
			DetourNodeSourceID: "01B", DetourNodeTag: "node-b"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {a}, 1: {b}, 2: {c}}
	all, excluded := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{a, b, c})

	if len(all) != 0 {
		t.Fatalf("цепочка выпадений оборвалась: осталось %d узлов", len(all))
	}
	if len(excluded) != 3 {
		t.Fatalf("в реестре %d записей, ожидались три: %+v", len(excluded), excluded)
	}
	eB, okB := excludedByID(excluded, "01B")
	eC, okC := excludedByID(excluded, "01C")
	if !okB || !okC {
		t.Fatalf("в реестре нет Bravo и/или Charlie: %+v", excluded)
	}
	if !strings.Contains(eB.Reason, "Alpha") {
		t.Errorf("причина Bravo %q не называет Alpha", eB.Reason)
	}
	// Причина Charlie обязана указывать на СВОЮ цель — Bravo, а не на корень:
	// чинить пользователю ближайшее звено.
	if !strings.Contains(eC.Reason, "Bravo") {
		t.Errorf("причина Charlie %q не называет Bravo — ближайшее сломанное звено", eC.Reason)
	}
}

// Тест 3 (SPEC-B): кольцо A↔B — оба fail-closed, причина говорит про цикл.
// На старом коде оба источника оставались, их узлы ехали с взаимными detour,
// а кольцо разрывал граф-санитайзер снятием detour у одного из них.
func TestResolveNodeDetours_RingIsFailClosedForEveryone(t *testing.T) {
	a := cascadeNode(t, tagDetourHopURI, "hop-a", "hop-a", 0)
	b := cascadeNode(t, tagDetourChainedURI, "node-b", "node-b", 1)

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01A", Label: "Alpha", Connections: []string{tagDetourHopURI},
			DetourNodeSourceID: "01B", DetourNodeTag: "node-b"},
		ProxySource{ID: "01B", Label: "Bravo", Connections: []string{"..."},
			DetourNodeSourceID: "01A", DetourNodeTag: "hop-a"},
	)
	nodesBySource := map[int][]*ParsedNode{0: {a}, 1: {b}}
	all, excluded := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{a, b})

	if len(all) != 0 {
		t.Fatalf("участники кольца остались в конфиге: %d узлов", len(all))
	}
	if len(excluded) != 2 {
		t.Fatalf("в реестре %d записей, ожидались две: %+v", len(excluded), excluded)
	}
	for _, id := range []string{"01A", "01B"} {
		e, ok := excludedByID(excluded, id)
		if !ok {
			t.Fatalf("участник кольца %s не попал в реестр", id)
		}
		if !strings.Contains(e.Reason, "цикл") {
			t.Errorf("причина %q не говорит про цикл", e.Reason)
		}
	}
	if hasDetour(a) || hasDetour(b) {
		t.Error("кольцо не имеет права быть разорванным простановкой/снятием detour")
	}
}

// Здоровая цепочка A←B←C собирается целиком: топологический порядок не должен
// ломать штамповку, даже когда источник-цель стоит НИЖЕ ссылающегося.
func TestResolveNodeDetours_TopologicalOrderStampsReverseListing(t *testing.T) {
	// B (индекс 0) ссылается на A (индекс 1) — цель стоит ниже в списке.
	b := cascadeNode(t, tagDetourChainedURI, "node-b", "node-b", 0)
	a := cascadeNode(t, tagDetourHopURI, "hop-a", "hop-a", 1)

	pc := tagDetourParserConfig(t,
		ProxySource{ID: "01B", Label: "Bravo", Connections: []string{"..."},
			DetourNodeSourceID: "01A", DetourNodeTag: "hop-a"},
		ProxySource{ID: "01A", Label: "Alpha", Connections: []string{tagDetourHopURI}},
	)
	nodesBySource := map[int][]*ParsedNode{0: {b}, 1: {a}}
	all, excluded := resolveNodeDetours(pc, nodesBySource, []*ParsedNode{b, a})

	if len(all) != 2 || len(excluded) != 0 {
		t.Fatalf("здоровая связка распалась: узлов %d, исключений %d", len(all), len(excluded))
	}
	if got, _ := b.Outbound["detour"].(string); got != "hop-a" {
		t.Errorf("detour = %q, ожидался финальный тег хопа", got)
	}
}
