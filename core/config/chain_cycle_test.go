// File chain_cycle_test.go — цикл группа↔цепочка (SPEC 110, T9).
package config

import "testing"

func chainNode(tag string, hops ...string) *ParsedNode {
	raw := make([]interface{}, 0, len(hops))
	for _, h := range hops {
		raw = append(raw, h)
	}
	return &ParsedNode{Tag: tag, Scheme: "chain", Outbound: map[string]interface{}{
		"tag": tag, "type": "chain", "outbounds": raw,
	}}
}

// Самый частый сценарий: цепочка [proxy-out, exit], а proxy-out фильтром
// ловит всё — включая саму цепочку.
func TestChainCycle_DirectSelfReference(t *testing.T) {
	nodes := []*ParsedNode{
		{Tag: "🇩🇪 Frankfurt", Scheme: "socks"},
		chainNode("via-de", "proxy-out", "🇳🇱 Amsterdam"),
	}
	hops := chainHopsByTag(nodes)

	kept, dropped := dropChainsThroughDirection(nodes, "proxy-out", hops)
	if len(dropped) != 1 || dropped[0] != "via-de" {
		t.Fatalf("выброшено %v, ожидали [via-de]", dropped)
	}
	for _, n := range kept {
		if n.Tag == "via-de" {
			t.Error("цепочка осталась в группе, через которую проходит")
		}
	}

	// В чужой группе та же цепочка — законный участник.
	kept, dropped = dropChainsThroughDirection(nodes, "other", hops)
	if len(dropped) != 0 {
		t.Errorf("цепочка выброшена из чужой группы: %v", dropped)
	}
	if len(kept) != len(nodes) {
		t.Errorf("состав чужой группы урезан: %d из %d", len(kept), len(nodes))
	}
}

// Транзитивно: цепочка через цепочку, которая идёт через группу.
func TestChainCycle_Transitive(t *testing.T) {
	nodes := []*ParsedNode{
		chainNode("inner", "proxy-out", "hop-b"),
		chainNode("outer", "inner", "hop-c"),
	}
	hops := chainHopsByTag(nodes)
	_, dropped := dropChainsThroughDirection(nodes, "proxy-out", hops)
	if len(dropped) != 2 {
		t.Fatalf("выброшено %v, ожидали обе цепочки: outer идёт через inner, а тот через proxy-out", dropped)
	}
}

// Без цепочек и без циклов вход возвращается как есть — конфиги без
// цепочек обязаны собираться байт-в-байт как раньше.
func TestChainCycle_NoChainsIsNoOp(t *testing.T) {
	nodes := []*ParsedNode{{Tag: "a", Scheme: "socks"}, {Tag: "b", Scheme: "socks"}}
	kept, dropped := dropChainsThroughDirection(nodes, "proxy-out", chainHopsByTag(nodes))
	if dropped != nil {
		t.Errorf("выброшено %v на пуле без цепочек", dropped)
	}
	if len(kept) != 2 {
		t.Errorf("состав изменён: %d из 2", len(kept))
	}
}

// Испорченные данные не должны подвешивать сборку: взаимная ссылка цепочек
// в пуле невозможна через ResolveChainSources, но функция не обязана
// полагаться на чужие инварианты.
func TestChainCycle_SelfReferencingDataTerminates(t *testing.T) {
	nodes := []*ParsedNode{
		chainNode("a", "b", "x"),
		chainNode("b", "a", "y"),
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		dropChainsThroughDirection(nodes, "proxy-out", chainHopsByTag(nodes))
	}()
	<-done
}
