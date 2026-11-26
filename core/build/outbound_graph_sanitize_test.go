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
	out, _ := sanitizeOutboundGraph(cache, finalTags, false)
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
	out, _ := sanitizeOutboundGraph(cache, finalTags, false)
	if len(out.Outbounds) != len(in) {
		t.Fatalf("healthy graph lost entries: %d != %d", len(out.Outbounds), len(in))
	}
	for i, raw := range out.Outbounds {
		if strings.TrimSpace(string(raw)) != in[i] {
			t.Errorf("healthy entry rewritten: %s -> %s", in[i], raw)
		}
	}
}

// Снятые хопы группируются по паре источник × цель и несут узлы: у подписки
// на полсотни узлов одна пропавшая цель иначе дала бы полсотни строк отчёта.
func TestSanitizeGroupsStrippedHopsPerSourceAndTarget(t *testing.T) {
	origins := map[string]NodeOrigin{
		"Big 1":   {SourceID: "01BIG", SourceLabel: "Big Sub"},
		"Big 2":   {SourceID: "01BIG", SourceLabel: "Big Sub"},
		"Big 3":   {SourceID: "01BIG", SourceLabel: "Big Sub"},
		"Small 1": {SourceID: "01SML", SourceLabel: "Small Sub"},
	}
	_, losses := sanitizeWithOrigins(t, []string{
		`{"tag":"Big 1","type":"vless","server":"a","detour":"gone"}`,
		`{"tag":"Big 2","type":"vless","server":"b","detour":"gone"}`,
		`{"tag":"Big 3","type":"vless","server":"c","detour":"other-gone"}`,
		`{"tag":"Small 1","type":"vless","server":"d","detour":"gone"}`,
	}, origins)

	if len(losses.stripped) != 3 {
		t.Fatalf("записей %d, ожидалось 3 (Big×gone, Big×other-gone, Small×gone): %+v", len(losses.stripped), losses.stripped)
	}
	count := map[string]int{}
	for _, e := range losses.stripped {
		count[e.SourceID+"/"+e.Target] = len(e.Nodes)
	}
	if count["01BIG/gone"] != 2 || count["01BIG/other-gone"] != 1 || count["01SML/gone"] != 1 {
		t.Errorf("группировка по паре источник × цель неверна: %v", count)
	}
}

// TestSanitizeChainDetourRewrite — правило 7 (SPEC 165 ядра, SPEC 189; сверено с
// ядром d9fc75468): на ядре, которое переписывает цепочку в detour, группа,
// достижимая из двух цепочек, снимает позднюю цепочку (chain_detour_conflict);
// кольцо `[A, Группа{A, X}]` снимает цепочку (chain_detour_ring); локальный
// порт узла позиции ≥ 1 снимает цепочку (chain_hop_local_port); поля сокета
// узла не трогаются. Для старого ядра конфиг законен и остаётся как есть.
func TestSanitizeChainDetourRewrite(t *testing.T) {
	outbounds := []string{
		`{"tag":"entry","type":"vless","server":"a"}`,
		`{"tag":"entry2","type":"vless","server":"a"}`,
		`{"tag":"x","type":"vless","server":"a","tcp_fast_open":true}`,
		`{"tag":"pool","type":"urltest","outbounds":["x"]}`,
		`{"tag":"c1","type":"chain","outbounds":["entry","pool"]}`,
		`{"tag":"c2","type":"chain","outbounds":["entry2","pool"]}`,
		`{"tag":"r","type":"vless","server":"a"}`,
		`{"tag":"y","type":"vless","server":"a"}`,
		`{"tag":"rg","type":"selector","outbounds":["r","y"]}`,
		`{"tag":"ring","type":"chain","outbounds":["r","rg"]}`,
		`{"tag":"e3","type":"vless","server":"a"}`,
		`{"tag":"wg","type":"wireguard","listen_port":51820}`,
		`{"tag":"port","type":"chain","outbounds":["e3","wg"]}`,
	}
	run := func(rewrite bool) (map[string]map[string]interface{}, graphLosses) {
		cache := &ParsedCache{}
		finalTags := map[string]bool{}
		for _, o := range outbounds {
			cache.Outbounds = append(cache.Outbounds, json.RawMessage(o))
			var m map[string]interface{}
			if err := json.Unmarshal([]byte(o), &m); err != nil {
				t.Fatalf("bad fixture: %v", err)
			}
			finalTags[m["tag"].(string)] = true
		}
		out, losses := sanitizeOutboundGraph(cache, finalTags, rewrite)
		got := map[string]map[string]interface{}{}
		for _, raw := range out.Outbounds {
			var m map[string]interface{}
			if err := json.Unmarshal(raw, &m); err != nil {
				t.Fatalf("emitted entry is not JSON: %s", raw)
			}
			got[m["tag"].(string)] = m
		}
		return got, losses
	}

	got, losses := run(true)
	for _, dropped := range []string{"c2", "ring", "port"} {
		if _, ok := got[dropped]; ok {
			t.Errorf("chain %s must be dropped", dropped)
		}
	}
	if _, ok := got["c1"]; !ok {
		t.Fatal("the earlier chain must stay")
	}
	if got["x"]["tcp_fast_open"] != true {
		t.Error("socket fields of a node inside a chain are not an error and must stay")
	}
	if _, ok := got["wg"]["listen_port"]; !ok {
		t.Error("listen_port is not removed: the chain is dropped instead")
	}
	codes := map[string]map[string]string{}
	for _, w := range losses.warnings {
		codes[w.Code] = w.Params
	}
	if p := codes["chain_detour_conflict"]; p["chain"] != "c2" || p["node"] != "pool" || p["other"] != "c1" {
		t.Errorf("chain_detour_conflict params = %v", p)
	}
	if p := codes["chain_detour_ring"]; p["chain"] != "ring" || p["node"] != "r" {
		t.Errorf("chain_detour_ring params = %v", p)
	}
	if p := codes["chain_hop_local_port"]; p["chain"] != "port" || p["node"] != "wg" || p["fields"] != "listen_port" {
		t.Errorf("chain_hop_local_port params = %v", p)
	}

	got, losses = run(false)
	for _, kept := range []string{"c1", "c2", "ring", "port"} {
		if _, ok := got[kept]; !ok {
			t.Errorf("an older core clones position nodes: chain %s is valid", kept)
		}
	}
	if len(losses.warnings) != 0 {
		t.Errorf("no rule-7 warnings for an older core: %v", losses.warnings)
	}
}
