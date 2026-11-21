package config

import "testing"

// SPEC 077 → SPEC 113-B: самоссылка и кольцо detour — fail-closed, носитель
// выбрасывается. Недоступная цель (контракт 1.1.171) здесь не решается: узел
// с висячим detour остаётся, хоп снимает граф-санитайзер с detour_unavailable.

func nodeWithDetour(tag, detour string) *ParsedNode {
	ob := map[string]interface{}{"type": "vless", "tag": tag}
	if detour != "" {
		ob["detour"] = detour
	}
	return &ParsedNode{Tag: tag, Scheme: "vless", Outbound: ob}
}

func detourOf(n *ParsedNode) (string, bool) {
	v, ok := n.Outbound["detour"].(string)
	return v, ok
}

func tagsOf(nodes []*ParsedNode) map[string]bool {
	out := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		if n != nil {
			out[n.Tag] = true
		}
	}
	return out
}

// Самоссылка: узел выпадает целиком. Снять detour было бы тихим прямым
// дозвоном, а он для detour запрещён.
func TestSanitizeNodeDetours_SelfReferenceDropsNode(t *testing.T) {
	a := nodeWithDetour("A", "A")
	kept := tagsOf(sanitizeNodeDetours([]*ParsedNode{a}))
	if kept["A"] {
		t.Error("самоссылающийся узел обязан выпасть, а не потерять detour")
	}
	if _, ok := detourOf(a); !ok {
		t.Error("detour снят — это тихий прямой дозвон")
	}
}

// A→B, где у B перехода нет, — валидный однохоповый маршрут: не трогаем.
func TestSanitizeNodeDetours_ValidChainKept(t *testing.T) {
	a := nodeWithDetour("A", "B")
	b := nodeWithDetour("B", "")
	kept := tagsOf(sanitizeNodeDetours([]*ParsedNode{a, b}))
	if !kept["A"] || !kept["B"] {
		t.Fatalf("здоровая связка распалась: %v", kept)
	}
	if d, ok := detourOf(a); !ok || d != "B" {
		t.Errorf("рабочий A→B detour обязан уцелеть, получено %q (есть=%v)", d, ok)
	}
}

// Detour на тег, который узлом не является (шаблонная/preset-группа,
// служебный outbound), здесь не решается: полный набор тегов известен только
// на финальной сборке. Узел остаётся, решение — за граф-санитайзером.
func TestSanitizeNodeDetours_ExternalTargetKept(t *testing.T) {
	a := nodeWithDetour("A", "proxy-group") // группа живёт в шаблоне, не среди узлов
	kept := tagsOf(sanitizeNodeDetours([]*ParsedNode{a}))
	if !kept["A"] {
		t.Fatal("узел с внешней целью detour выпадать здесь не должен")
	}
	if d, ok := detourOf(a); !ok || d != "proxy-group" {
		t.Errorf("detour на внешний тег обязан уцелеть, получено %q (есть=%v)", d, ok)
	}
}

// Кольцо A↔B: fail-closed для ОБОИХ. Прежде выпадало одно ребро, и один из
// узлов начинал ходить напрямую — молча.
func TestSanitizeNodeDetours_TwoCycleDropsBoth(t *testing.T) {
	a := nodeWithDetour("A", "B")
	b := nodeWithDetour("B", "A")
	kept := tagsOf(sanitizeNodeDetours([]*ParsedNode{a, b}))
	if kept["A"] || kept["B"] {
		t.Errorf("участники кольца остались в конфиге: %v", kept)
	}
	if _, ok := detourOf(a); !ok {
		t.Error("detour снят у A — тихий прямой дозвон")
	}
	if _, ok := detourOf(b); !ok {
		t.Error("detour снят у B — тихий прямой дозвон")
	}
}

// A→B→C→A: выпадают все трое.
func TestSanitizeNodeDetours_ThreeCycleDropsAll(t *testing.T) {
	a := nodeWithDetour("A", "B")
	b := nodeWithDetour("B", "C")
	c := nodeWithDetour("C", "A")
	kept := tagsOf(sanitizeNodeDetours([]*ParsedNode{a, b, c}))
	if len(kept) != 0 {
		t.Errorf("кольцо из трёх обязано выпасть целиком, осталось %v", kept)
	}
}

// Каскада нет (1.1.171): D ходил через участника кольца — остаётся с висячим
// detour, хоп снимет граф-санитайзер с detour_unavailable.
func TestSanitizeNodeDetours_NoCascadeAfterRing(t *testing.T) {
	a := nodeWithDetour("A", "B")
	b := nodeWithDetour("B", "A")
	d := nodeWithDetour("D", "A")
	e := nodeWithDetour("E", "")
	kept := tagsOf(sanitizeNodeDetours([]*ParsedNode{a, b, d, e}))
	if !kept["D"] {
		t.Error("узел, ходивший через выброшенный, выпадать не должен — цель недоступна, хоп снимет санитайзер")
	}
	if kept["A"] || kept["B"] {
		t.Errorf("участники кольца остались: %v", kept)
	}
	if !kept["E"] {
		t.Error("узел без detour к кольцу отношения не имеет и выпадать не должен")
	}
}
