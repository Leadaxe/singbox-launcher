// File detour_group_cycle_test.go — цикл узел→группа→узел (SPEC 077 follow-up).
//
// Живой случай: endpoint Proton с `detour: "vpn ②"` попадал фильтром в состав
// самой «vpn ②». Ядро отвергало ВЕСЬ конфиг:
//
//	dependency[Proton] not found for outbound[proxy-out-auto]
package config

import "testing"

func detourNode(tag, detour string) *ParsedNode {
	n := &ParsedNode{Tag: tag, Scheme: "wireguard", Outbound: map[string]interface{}{"tag": tag}}
	if detour != "" {
		n.Outbound["detour"] = detour
	}
	return n
}

func TestDetourCycle_NodeThroughItsOwnGroup(t *testing.T) {
	nodes := []*ParsedNode{
		detourNode("Proton", "vpn ②"),
		detourNode("wg-parnas", ""),
	}

	kept, dropped := dropNodesDetouringThroughGroup(nodes, "vpn ②")
	if len(dropped) != 1 || dropped[0] != "Proton" {
		t.Fatalf("выброшено %v, ожидали [Proton]", dropped)
	}
	for _, n := range kept {
		if n.Tag == "Proton" {
			t.Error("узел остался в группе, через которую ходит — ядро не стартует")
		}
	}

	// Детур сохраняется: пользователь задал его осознанно, и тихо отправить
	// трафик напрямую значит нарушить ровно то, о чём он просил.
	if nodeDetourTarget(nodes[0]) != "vpn ②" {
		t.Error("detour снят — узел молча пошёл бы напрямую")
	}

	// В чужой группе тот же узел — законный участник.
	kept, dropped = dropNodesDetouringThroughGroup(nodes, "vpn ①")
	if len(dropped) != 0 {
		t.Errorf("узел выброшен из чужой группы: %v", dropped)
	}
	if len(kept) != 2 {
		t.Errorf("состав чужой группы урезан: %d из 2", len(kept))
	}
}

// Узлы без detour и группы без таких узлов — вход возвращается как есть:
// конфиги без этой болезни обязаны собираться байт-в-байт как раньше.
func TestDetourCycle_NoDetoursIsNoOp(t *testing.T) {
	nodes := []*ParsedNode{detourNode("a", ""), detourNode("b", "")}
	kept, dropped := dropNodesDetouringThroughGroup(nodes, "vpn ②")
	if dropped != nil {
		t.Errorf("выброшено %v на узлах без detour", dropped)
	}
	if len(kept) != 2 {
		t.Errorf("состав изменён: %d из 2", len(kept))
	}
}

// Детур на ДРУГУЮ группу циклом не является и трогаться не должен.
func TestDetourCycle_DetourToOtherGroupKept(t *testing.T) {
	nodes := []*ParsedNode{detourNode("Proton", "vpn ①")}
	kept, dropped := dropNodesDetouringThroughGroup(nodes, "vpn ②")
	if len(dropped) != 0 || len(kept) != 1 {
		t.Errorf("узел с детуром на чужую группу выброшен: dropped=%v", dropped)
	}
}
