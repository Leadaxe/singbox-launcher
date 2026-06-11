package config

import "testing"

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

// SPEC 077 phase 2: self-reference is dropped, the node keeps dialing directly.
func TestSanitizeNodeDetours_SelfReference(t *testing.T) {
	a := nodeWithDetour("A", "A")
	sanitizeNodeDetours([]*ParsedNode{a})
	if _, ok := detourOf(a); ok {
		t.Error("self-referential detour must be dropped")
	}
}

// A→B where B has no detour is a valid one-hop chain — nothing is dropped.
func TestSanitizeNodeDetours_ValidChainKept(t *testing.T) {
	a := nodeWithDetour("A", "B")
	b := nodeWithDetour("B", "")
	sanitizeNodeDetours([]*ParsedNode{a, b})
	if d, ok := detourOf(a); !ok || d != "B" {
		t.Errorf("valid A→B detour must be kept, got %q (present=%v)", d, ok)
	}
}

// Detour onto a tag that is not a node (template/preset group, built-in) is
// left untouched — it cannot be validated here.
func TestSanitizeNodeDetours_ExternalTargetKept(t *testing.T) {
	a := nodeWithDetour("A", "proxy-group") // group lives in template, not in node set
	sanitizeNodeDetours([]*ParsedNode{a})
	if d, ok := detourOf(a); !ok || d != "proxy-group" {
		t.Errorf("detour onto external tag must be kept, got %q (present=%v)", d, ok)
	}
}

// A↔B mutual detour is a cycle; exactly one edge is dropped so sing-box won't
// reject the config, and at least one direction survives.
func TestSanitizeNodeDetours_TwoCycleBroken(t *testing.T) {
	a := nodeWithDetour("A", "B")
	b := nodeWithDetour("B", "A")
	sanitizeNodeDetours([]*ParsedNode{a, b})
	_, aHas := detourOf(a)
	_, bHas := detourOf(b)
	if aHas && bHas {
		t.Error("2-cycle must have at least one edge dropped")
	}
	if !aHas && !bHas {
		t.Error("2-cycle must not drop both edges (over-pruning)")
	}
}

// A→B→C→A 3-cycle: the ring must be broken (not all three survive).
func TestSanitizeNodeDetours_ThreeCycleBroken(t *testing.T) {
	a := nodeWithDetour("A", "B")
	b := nodeWithDetour("B", "C")
	c := nodeWithDetour("C", "A")
	sanitizeNodeDetours([]*ParsedNode{a, b, c})
	kept := 0
	for _, n := range []*ParsedNode{a, b, c} {
		if _, ok := detourOf(n); ok {
			kept++
		}
	}
	if kept != 2 {
		t.Errorf("3-cycle must drop exactly one edge (kept=%d, want 2)", kept)
	}
}
