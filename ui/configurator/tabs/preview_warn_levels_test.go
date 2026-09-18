// File preview_warn_levels_test.go — счёт по УРОВНЯМ и живость членов группы
// у выключенного источника (SPEC 131 §6, обкатка на живом состоянии).
//
// Данные-критично здесь два числа, и оба зовут пользователя чинить: «⚠ N» в
// шапке контейнера и «⚠ N node(s) with warnings» в списке источников. Каждое
// врало по своему поводу — первое считало выключенные группы сломанными,
// второе считало «к сведению» предупреждением, — и оба показывали тревогу
// там, где чинить нечего. Тексты не проверяются (память `no-ui-format-tests`):
// только выбор и счёт.
package tabs

import (
	"testing"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/nodewarn"
)

// warnedNode — узел состава с заданными кодами.
func warnedNode(rawTag string, codes ...string) corestate.Node {
	n := stateServer(rawTag)
	for _, c := range codes {
		n.Warnings = append(n.Warnings, corestate.NodeWarning{Code: c})
	}
	return n
}

// Коды берутся из реестра — тот же довод, что в internal/nodewarn: зашитый
// список разъехался бы с ним на первом же понижении уровня.
const (
	testWarnCode = "transport_unsupported" // severity=warning
	testInfoCode = "reality_fp_not_chrome" // severity=info
)

// Узел, у которого единственный код — info, не красится, не считается в «⚠»
// и не попадает в сводку проблем: он попадает в СВОЮ группу «к сведению».
func TestPreviewRowsCountWarningsByLevel(t *testing.T) {
	// Тот самый расклад живого состояния: двенадцать узлов с одним info,
	// один с настоящим предупреждением, один чистый.
	stateNodes := []corestate.Node{stateServer("clean")}
	emitted := []*config.ParsedNode{emittedNode("clean", "clean")}
	for i := 0; i < 12; i++ {
		tag := "info" + string(rune('a'+i))
		stateNodes = append(stateNodes, warnedNode(tag, testInfoCode))
		emitted = append(emitted, emittedNode(tag, tag))
	}
	stateNodes = append(stateNodes, warnedNode("bad", testWarnCode))
	emitted = append(emitted, emittedNode("bad", "bad"))

	rows := buildPreviewRows(stateNodes, emitted)
	if len(rows) != 14 {
		t.Fatalf("строк = %d, ожидали 14", len(rows))
	}

	if got := previewRowsWarned(rows); got != 1 {
		t.Errorf("⚠-счётчик = %d, ожидался 1 (двенадцать info — не предупреждения)", got)
	}
	if got := previewRowsInfoOnly(rows); got != 12 {
		t.Errorf("узлов «к сведению» = %d, ожидалось 12", got)
	}
	if got := previewRowsBroken(rows); got != 0 {
		t.Errorf("сломанных = %d, ожидался 0 — ни одна запись не отбракована", got)
	}

	// Выделение строки — тот же предикат: info-узел показывает свой обычный
	// состав, и оранжевый цвет рядом с ним означал бы «чини меня».
	for i := range rows {
		want := rows[i].RawTag == "bad"
		if got := previewRowWarn(rows[i]); got != want {
			t.Errorf("строка %q: выделение = %v, ожидалось %v", rows[i].RawTag, got, want)
		}
	}

	// Сводка под составом: проблемы и «к сведению» — ДВЕ разные строки, и
	// обе непусты на этом наборе.
	if previewWarningsSummary(rows) == "" {
		t.Error("сводка проблем пуста, хотя один узел с warning есть")
	}
	if previewInfoSummary(rows) == "" {
		t.Error("группа «к сведению» пуста, хотя двенадцать info-узлов есть")
	}

	// А без единой проблемы ⚠-сводки не должно быть вовсе: строка «⚠ …» над
	// одними info обещала бы поломку.
	infoOnlyRows := make([]previewRow, 0, 12)
	for i := range rows {
		if nodewarn.InfoOnly(rows[i].Warnings) {
			infoOnlyRows = append(infoOnlyRows, rows[i])
		}
	}
	if s := previewWarningsSummary(infoOnlyRows); s != "" {
		t.Errorf("сводка проблем на одних info = %q, ожидалась пустая", s)
	}
	if previewInfoSummary(infoOnlyRows) == "" {
		t.Error("группа «к сведению» пропала на наборе из одних info")
	}
	if previewWarningsBlock(infoOnlyRows) == nil {
		t.Error("блок сводки пропал: про info рассказать всё равно негде больше")
	}
}

// sourceWarnedNodes — то же правило на СОСТАВЕ источника: это число рисует
// строку списка источников, которую читают, ничего не открывая.
func TestSourceWarnedNodesIgnoresInfo(t *testing.T) {
	src := corestate.Source{
		Node: corestate.Node{Kind: corestate.SourceKindSubscription, Enabled: true},
		ID:   "src-1",
		Nodes: []corestate.Node{
			warnedNode("a", testInfoCode),
			warnedNode("b", testInfoCode),
			stateServer("c"),
		},
	}
	if got := sourceWarnedNodes(&src); got != 0 {
		t.Errorf("узлов с предупреждениями = %d, ожидался 0 — в составе одни info", got)
	}
	src.Nodes = append(src.Nodes, warnedNode("d", testWarnCode))
	if got := sourceWarnedNodes(&src); got != 1 {
		t.Errorf("узлов с предупреждениями = %d, ожидался 1", got)
	}
	// Узловой источник (состава нет): коды живут у самого источника.
	node := corestate.Source{
		Node: corestate.Node{Kind: corestate.SourceKindServer, Enabled: true},
		ID:   "src-2",
	}
	node.Warnings = []corestate.NodeWarning{{Code: testInfoCode}}
	if got := sourceWarnedNodes(&node); got != 0 {
		t.Errorf("узловой источник с одним info сосчитан как %d", got)
	}
	node.Warnings = append(node.Warnings, corestate.NodeWarning{Code: testWarnCode})
	if got := sourceWarnedNodes(&node); got != 1 {
		t.Errorf("узловой источник с warning сосчитан как %d", got)
	}
}

// autoGroupNode — узел-группа, ссылающаяся на членов своего контейнера.
func autoGroupNode(tag, ownerID string, members ...string) corestate.Node {
	g := &corestate.AutoGroup{GroupType: corestate.AutoGroupURLTest}
	for _, m := range members {
		g.Members = append(g.Members, corestate.NodeLink{FolderID: ownerID, Tag: m})
	}
	return corestate.Node{Kind: corestate.SourceKindAuto, Tag: tag, Enabled: true, Group: g}
}

// Выключенный источник: его группы выключены ВМЕСТЕ с членами одним и тем же
// тумблером, и объявлять их сломанными — ложная тревога (владелец увидел её
// на подписке Liberty: «⚠ 8 node error(s)» и «[0] fastest» у каждой группы).
func TestGroupMembersAliveInDisabledOwnSource(t *testing.T) {
	const ownerID = "src-off"
	stateNodes := []corestate.Node{
		stateServer("m1"),
		stateServer("m2"),
		autoGroupNode("auto", ownerID, "m1", "m2"),
	}
	emitted := []*config.ParsedNode{
		emittedNode("m1", "m1"),
		emittedNode("m2", "m2"),
		{Scheme: configtypes.SchemeGroup, Tag: "auto", Outbound: map[string]interface{}{"type": "urltest"}},
	}
	off := corestate.Source{
		Node:  corestate.Node{Kind: corestate.SourceKindSubscription, Enabled: false},
		ID:    ownerID,
		Nodes: stateNodes,
	}

	rows := buildPreviewRows(stateNodes, emitted)
	annotatePreviewGroupRows(rows, stateNodes, []corestate.Source{off}, ownerID)

	if !rows[2].GroupCounted {
		t.Fatal("строка группы не сосчитана")
	}
	if rows[2].GroupAlive != 2 {
		t.Errorf("живых членов = %d, ожидалось 2: источник выключен вместе с ними, а не сломан",
			rows[2].GroupAlive)
	}
	if previewRowsBroken(rows) != 0 {
		t.Errorf("сломанных строк = %d, ожидался 0", previewRowsBroken(rows))
	}
	if previewRowWarn(rows[2]) {
		t.Error("строка группы выключенного источника выделена как сломанная")
	}

	// Тот же источник ВКЛЮЧЁН — счёт не меняется: тумблер источника из счёта
	// членов своего же состава ушёл целиком.
	on := off
	on.Enabled = true
	rowsOn := buildPreviewRows(stateNodes, emitted)
	annotatePreviewGroupRows(rowsOn, stateNodes, []corestate.Source{on}, ownerID)
	if rowsOn[2].GroupAlive != 2 {
		t.Errorf("включённый источник: живых = %d, ожидалось 2", rowsOn[2].GroupAlive)
	}

	// Выключен САМ узел-член — он и правда не живой, в любом источнике.
	half := make([]corestate.Node, len(stateNodes))
	copy(half, stateNodes)
	half[1] = stateServer("m2")
	half[1].Enabled = false
	offHalf := off
	offHalf.Nodes = half
	rowsHalf := buildPreviewRows(half, emitted)
	annotatePreviewGroupRows(rowsHalf, half, []corestate.Source{offHalf}, ownerID)
	if rowsHalf[2].GroupAlive != 1 {
		t.Errorf("выключенный член сосчитан живым: живых = %d, ожидалось 1", rowsHalf[2].GroupAlive)
	}
}

// Группа ссылается на члена ЧУЖОГО выключенного источника — прежнее правило
// остаётся: включи такую группу, и на сборке члена не будет.
func TestGroupMembersFromOtherDisabledSourceStayDead(t *testing.T) {
	const ownerID, otherID = "src-own", "src-other"
	stateNodes := []corestate.Node{autoGroupNode("auto", otherID, "far")}
	emitted := []*config.ParsedNode{
		{Scheme: configtypes.SchemeGroup, Tag: "auto", Outbound: map[string]interface{}{"type": "urltest"}},
	}
	own := corestate.Source{
		Node:  corestate.Node{Kind: corestate.SourceKindFolder, Enabled: true},
		ID:    ownerID,
		Nodes: stateNodes,
	}
	other := corestate.Source{
		Node:  corestate.Node{Kind: corestate.SourceKindSubscription, Enabled: false},
		ID:    otherID,
		Nodes: []corestate.Node{stateServer("far")},
	}

	rows := buildPreviewRows(stateNodes, emitted)
	annotatePreviewGroupRows(rows, stateNodes, []corestate.Source{own, other}, ownerID)

	if rows[0].GroupAlive != 0 {
		t.Errorf("член чужого выключенного источника сосчитан живым: %d", rows[0].GroupAlive)
	}
	if previewRowsBroken(rows) != 1 {
		t.Errorf("группа без живых членов не помечена сломанной: broken = %d", previewRowsBroken(rows))
	}
}
