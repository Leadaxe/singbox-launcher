package build

import (
	"encoding/json"
	"strings"
	"testing"
)

func sanitizeHelper(t *testing.T, outbounds []string, templateTags ...string) (map[string]map[string]interface{}, map[string]bool) {
	t.Helper()
	cache := &ParsedCache{}
	finalTags := make(map[string]bool)
	for _, tt := range templateTags {
		finalTags[tt] = true
	}
	for _, o := range outbounds {
		cache.Outbounds = append(cache.Outbounds, json.RawMessage(o))
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(o), &m); err != nil {
			t.Fatalf("bad fixture: %v", err)
		}
		finalTags[m["tag"].(string)] = true
	}
	out := sanitizeOutboundGraph(cache, finalTags)
	got := make(map[string]map[string]interface{})
	for _, raw := range out.Outbounds {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("emitted entry is not JSON: %s", raw)
		}
		got[m["tag"].(string)] = m
	}
	return got, finalTags
}

// TestSanitizeChainWithVanishedHop — цепочка с позицией, которой нет в
// финальном конфиге (источник хопа дропнул hash-detour ПОСЛЕ разрешения
// цепочки), удаляется целиком, и ссылающаяся на неё группа чистится следом.
func TestSanitizeChainWithVanishedHop(t *testing.T) {
	got, tags := sanitizeHelper(t, []string{
		`{"tag":"n1","type":"vless","server":"a"}`,
		`{"tag":"c1","type":"chain","outbounds":["ghost","n1"]}`,
		`{"tag":"g1","type":"selector","outbounds":["c1","n1"],"default":"c1"}`,
	})
	if _, ok := got["c1"]; ok {
		t.Error("chain with vanished hop must be dropped")
	}
	if tags["c1"] {
		t.Error("dropped chain must leave finalTags")
	}
	g1 := got["g1"]
	members, _ := g1["outbounds"].([]interface{})
	if len(members) != 1 || members[0] != "n1" {
		t.Errorf("group must lose the dropped chain: %v", members)
	}
	if g1["default"] != "n1" {
		t.Errorf("default must be repaired: %v", g1["default"])
	}
}

// TestSanitizeEmptyGroupCascade — группа, опустевшая после удаления
// участника, удаляется, и группа уровнем выше теряет её из состава.
func TestSanitizeEmptyGroupCascade(t *testing.T) {
	got, _ := sanitizeHelper(t, []string{
		`{"tag":"n1","type":"vless","server":"a"}`,
		`{"tag":"inner","type":"urltest","outbounds":["ghost"]}`,
		`{"tag":"outer","type":"selector","outbounds":["inner","n1"]}`,
	})
	if _, ok := got["inner"]; ok {
		t.Error("emptied group must be dropped")
	}
	members, _ := got["outer"]["outbounds"].([]interface{})
	if len(members) != 1 || members[0] != "n1" {
		t.Errorf("outer group must lose the emptied inner group: %v", members)
	}
}

// TestSanitizeCrossEdgeCycle — кольцо через рёбра РАЗНЫХ видов: узел входит
// в группу, через которую сам ходит транзитом (n1.detour → n2, n2.detour →
// группа, группа ∋ n1). Ни одна из прежних частных проверок это не ловила.
func TestSanitizeCrossEdgeCycle(t *testing.T) {
	got, _ := sanitizeHelper(t, []string{
		`{"tag":"n1","type":"vless","server":"a","detour":"n2"}`,
		`{"tag":"n2","type":"vless","server":"b","detour":"g1"}`,
		`{"tag":"g1","type":"selector","outbounds":["n1"]}`,
	})
	// Кольцо обязано быть разорвано: конфиг без кольца по любым рёбрам.
	seen := map[string][]string{}
	for tag, m := range got {
		var refs []string
		if d, _ := m["detour"].(string); d != "" {
			refs = append(refs, d)
		}
		if arr, ok := m["outbounds"].([]interface{}); ok {
			for _, x := range arr {
				refs = append(refs, x.(string))
			}
		}
		seen[tag] = refs
	}
	var walk func(tag string, stack map[string]bool) bool
	walk = func(tag string, stack map[string]bool) bool {
		if stack[tag] {
			return true
		}
		stack[tag] = true
		for _, r := range seen[tag] {
			if walk(r, stack) {
				return true
			}
		}
		delete(stack, tag)
		return false
	}
	for tag := range seen {
		if walk(tag, map[string]bool{}) {
			t.Fatalf("cycle survived sanitization: %v", seen)
		}
	}
}

// TestSanitizeChainLeafUnderGroup — группа, стоящая позицией ≥ 1 цепочки,
// не должна содержать цепочек в листьях: ядро отвергает такой конфиг на
// старте («nested chain is only allowed at position 0»), а check молчит.
func TestSanitizeChainLeafUnderGroup(t *testing.T) {
	got, _ := sanitizeHelper(t, []string{
		`{"tag":"n1","type":"vless","server":"a"}`,
		`{"tag":"n2","type":"vless","server":"b"}`,
		`{"tag":"inner-chain","type":"chain","outbounds":["n1","n2"]}`,
		`{"tag":"dir","type":"selector","outbounds":["inner-chain","n2"]}`,
		`{"tag":"c-main","type":"chain","outbounds":["n1","dir"]}`,
	})
	members, _ := got["dir"]["outbounds"].([]interface{})
	for _, m := range members {
		if m == "inner-chain" {
			t.Error("chain leaf must be pruned from a group standing at position >= 1")
		}
	}
	if _, ok := got["c-main"]; !ok {
		t.Error("main chain must survive once the leaf is pruned")
	}
	if _, ok := got["inner-chain"]; !ok {
		t.Error("inner chain itself stays in config (usable at position 0 elsewhere)")
	}
}

// TestSanitizeDirectChainAtPositionOne — прямая ссылка цепочки на цепочку
// позицией ≥ 1 недопустима: ссылающаяся цепочка удаляется.
func TestSanitizeDirectChainAtPositionOne(t *testing.T) {
	got, _ := sanitizeHelper(t, []string{
		`{"tag":"n1","type":"vless","server":"a"}`,
		`{"tag":"c-inner","type":"chain","outbounds":["n1","n1"]}`,
		`{"tag":"c-bad","type":"chain","outbounds":["n1","c-inner"]}`,
	})
	if _, ok := got["c-bad"]; ok {
		t.Error("chain referencing a chain at position >= 1 must be dropped")
	}
	if _, ok := got["c-inner"]; !ok {
		t.Error("inner chain must survive")
	}
}

// TestSanitizeKeepsHealthyGraph — здоровый граф проходит байт-в-байт (записи
// не dirty — исходные строки не переписываются).
func TestSanitizeKeepsHealthyGraph(t *testing.T) {
	in := []string{
		`{"tag":"n1","type":"vless","server":"a","detour":"direct-out"}`,
		`{"tag":"c1","type":"chain","outbounds":["n1","n1"]}`,
		`{"tag":"g1","type":"selector","outbounds":["c1","n1"],"default":"n1"}`,
	}
	cache := &ParsedCache{}
	finalTags := map[string]bool{"direct-out": true}
	for _, o := range in {
		cache.Outbounds = append(cache.Outbounds, json.RawMessage(o))
		var m map[string]interface{}
		_ = json.Unmarshal([]byte(o), &m)
		finalTags[m["tag"].(string)] = true
	}
	out := sanitizeOutboundGraph(cache, finalTags)
	if len(out.Outbounds) != len(in) {
		t.Fatalf("healthy graph lost entries: %d != %d", len(out.Outbounds), len(in))
	}
	for i, raw := range out.Outbounds {
		if strings.TrimSpace(string(raw)) != in[i] {
			t.Errorf("healthy entry rewritten: %s -> %s", in[i], raw)
		}
	}
}
