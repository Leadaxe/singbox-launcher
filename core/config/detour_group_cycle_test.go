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
	if len(dropped) != 1 || dropped[0].Tag != "Proton" {
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

// Кольцо detour→группа едет в отчёт сборки: одна запись на пару (источник,
// группа) с кодом реестра и адресом источника — иначе выброс виден только в
// логе, а в Мастере читается как «фильтр Направления не сработал» (живой
// случай: подписка Liberty с detour на «Jump server» и фильтр (🔥|🗽) у
// самого «Jump server»).
func TestDetourCycle_WarningsGroupedBySourceWithAddress(t *testing.T) {
	proxies := []ProxySource{
		{ID: "01LIBERTY", Label: "Liberty VPN 🗽"},
		{ID: "01TAILSCALE", Label: "Tailscale LexNet"},
	}
	cycles := []DetourCycle{
		{Node: "🗽🇭🇺-Венгрия", Group: "Jump server", SourceIndex: 0},
		{Node: "🗽🇵🇱-Польша", Group: "Jump server", SourceIndex: 0},
		{Node: "Tailscale LexNet", Group: "Jump server", SourceIndex: 1},
		{Node: "manual-out", Group: "Jump server", SourceIndex: UnsetSourceIndex},
	}
	ws := detourCycleWarnings(cycles, proxies)
	if len(ws) != 3 {
		t.Fatalf("записей %d, ожидали 3 (Liberty, Tailscale, без источника): %+v", len(ws), ws)
	}
	lib := ws[0]
	if lib.Code != codeNodeDetourThroughGroup || lib.SourceID != "01LIBERTY" || lib.SourceLabel != "Liberty VPN 🗽" {
		t.Errorf("запись Liberty без кода или адреса: %+v", lib)
	}
	if lib.DirectionTag != "Jump server" || lib.Params["group"] != "Jump server" || lib.Params["count"] != "2" {
		t.Errorf("запись Liberty не называет группу и число узлов: %+v", lib)
	}
	if lib.Text == "" || lib.Text == codeNodeDetourThroughGroup {
		t.Errorf("текст записи пуст: %+v", lib)
	}
	if ws[1].SourceID != "01TAILSCALE" || ws[1].Params["count"] != "1" {
		t.Errorf("запись Tailscale: %+v", ws[1])
	}
	if ws[2].SourceID != "" || ws[2].Params["count"] != "1" {
		t.Errorf("узел без источника обязан дать запись без адреса: %+v", ws[2])
	}
}
