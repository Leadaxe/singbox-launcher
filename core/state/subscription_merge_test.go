package state

// Тесты merge-правил подписки (SPEC 118 W3, §4.D на уровне модели):
// сохранение пользовательских пометок, удаление исчезнувших, запрет
// удаления при truncated (113-A), одноразовые pending_disabled (O2) и
// согласованность мостовой карты DisabledNodes с каноном.

import (
	"encoding/json"
	"testing"
)

func mergeTestSub(nodes ...Node) *Source {
	return &Source{
		Node:  Node{Kind: SourceKindSubscription, Enabled: true},
		ID:    "01SUBMERGE00000000000000000",
		Name:  "sub",
		Nodes: nodes,
	}
}

func serverNode(tag string, body string) Node {
	return Node{Kind: SourceKindServer, Tag: tag, Enabled: true, Body: json.RawMessage(body)}
}

func mergedTags(sub *Source) []string {
	tags := make([]string, 0, len(sub.Nodes))
	for i := range sub.Nodes {
		tags = append(tags, sub.Nodes[i].Tag)
	}
	return tags
}

func tagsEqual(got []string, want ...string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// Недостоверный результат не трогает nodes[] вообще (SPEC 113-A).
func TestMergeUntrustedIsNoop(t *testing.T) {
	sub := mergeTestSub(serverNode("A", `{"type":"vless"}`))
	changed, _ := MergeSubscriptionNodes(sub, &SubFetchMaterial{Nodes: []Node{serverNode("B", `{}`)}}, false)
	if changed || !tagsEqual(mergedTags(sub), "A") {
		t.Fatalf("untrusted merge изменил nodes: %v", mergedTags(sub))
	}
	changed, _ = MergeSubscriptionNodes(sub, nil, true)
	if changed || !tagsEqual(mergedTags(sub), "A") {
		t.Fatalf("nil-материал изменил nodes: %v", mergedTags(sub))
	}
}

// Совпавший тег: body освежается, enabled/detour живут; новый — включён;
// исчезнувший — удалён; порядок = порядок свежего тела.
func TestMergeRefreshAddRemove(t *testing.T) {
	old := serverNode("A", `{"v":1}`)
	old.Enabled = false
	old.Detour = &NodeLink{Tag: "up"}
	sub := mergeTestSub(old, serverNode("B", `{"v":1}`))

	fresh := &SubFetchMaterial{Nodes: []Node{
		serverNode("C", `{"v":2}`), // новый — впереди, порядок тела побеждает
		serverNode("A", `{"v":2}`),
	}}
	changed, warns := MergeSubscriptionNodes(sub, fresh, true)
	if !changed {
		t.Fatal("merge не отметил изменение")
	}
	if len(warns) != 0 {
		t.Fatalf("неожиданные warnings: %v", warns)
	}
	if !tagsEqual(mergedTags(sub), "C", "A") {
		t.Fatalf("порядок/состав: %v", mergedTags(sub))
	}
	if !sub.Nodes[0].Enabled {
		t.Fatal("новый узел обязан родиться включённым")
	}
	a := sub.Nodes[1]
	if a.Enabled {
		t.Fatal("enabled=false не пережил merge")
	}
	if a.Detour == nil || a.Detour.Tag != "up" {
		t.Fatalf("detour не пережил merge: %+v", a.Detour)
	}
	if string(a.Body) != `{"v":2}` {
		t.Fatalf("body не освежён: %s", a.Body)
	}
}

// Truncated: удаление исчезнувших запрещено — они удерживаются в хвосте;
// обновление и добавление работают.
func TestMergeTruncatedKeepsDisappeared(t *testing.T) {
	sub := mergeTestSub(serverNode("A", `{"v":1}`), serverNode("B", `{"v":1}`))
	fresh := &SubFetchMaterial{
		Nodes:     []Node{serverNode("C", `{"v":2}`), serverNode("A", `{"v":2}`)},
		Truncated: true,
	}
	if changed, _ := MergeSubscriptionNodes(sub, fresh, true); !changed {
		t.Fatal("merge не отметил изменение")
	}
	if !tagsEqual(mergedTags(sub), "C", "A", "B") {
		t.Fatalf("truncated-merge: %v", mergedTags(sub))
	}
	if string(sub.Nodes[1].Body) != `{"v":2}` {
		t.Fatal("обновление совпавшего при truncated обязано работать")
	}
}

// pending_disabled (вердикт O2): применяется по сырым тегам на первом
// достоверном fetch и стирается; несматченный тег — warning.
func TestMergePendingDisabledApplied(t *testing.T) {
	sub := mergeTestSub()
	sub.PendingDisabled = []string{"B", "ghost"}
	fresh := &SubFetchMaterial{Nodes: []Node{serverNode("A", `{}`), serverNode("B", `{}`)}}
	_, warns := MergeSubscriptionNodes(sub, fresh, true)
	if sub.Nodes[0].Tag != "A" || !sub.Nodes[0].Enabled {
		t.Fatalf("узел A: %+v", sub.Nodes[0])
	}
	if sub.Nodes[1].Enabled {
		t.Fatal("pending-отметка не применилась к B")
	}
	if sub.PendingDisabled != nil {
		t.Fatalf("pending_disabled не стёрт: %v", sub.PendingDisabled)
	}
	if len(warns) != 1 {
		t.Fatalf("несматченный ghost обязан дать warning: %v", warns)
	}
}

// pending_disabled при truncated: несматченный тег переживает fetch —
// узел мог остаться за капом.
func TestMergePendingDisabledSurvivesTruncated(t *testing.T) {
	sub := mergeTestSub()
	sub.PendingDisabled = []string{"B", "beyond-cap"}
	fresh := &SubFetchMaterial{Nodes: []Node{serverNode("B", `{}`)}, Truncated: true}
	MergeSubscriptionNodes(sub, fresh, true)
	if sub.Nodes[0].Enabled {
		t.Fatal("pending-отметка не применилась к B")
	}
	if len(sub.PendingDisabled) != 1 || sub.PendingDisabled[0] != "beyond-cap" {
		t.Fatalf("несматченная отметка при truncated обязана выжить: %v", sub.PendingDisabled)
	}
}

// Мостовая карта DisabledNodes согласуется с каноном (TEMPORARY BRIDGE):
// выключенный узел несёт запись (со старым timestamp), включённый — нет,
// ключ исчезнувшего узла выбрасывается на достоверном полном merge и
// удерживается при truncated.
func TestMergeSyncsLegacyDisabledMap(t *testing.T) {
	restore := nowUnixForBridge
	nowUnixForBridge = func() int64 { return 42 }
	defer func() { nowUnixForBridge = restore }()

	off := serverNode("A", `{}`)
	off.Enabled = false
	sub := mergeTestSub(off, serverNode("B", `{}`))
	sub.DisabledNodes = map[string]int64{"A": 7, "gone": 9}

	fresh := &SubFetchMaterial{Nodes: []Node{serverNode("A", `{}`), serverNode("B", `{}`)}}
	MergeSubscriptionNodes(sub, fresh, true)
	if len(sub.DisabledNodes) != 1 || sub.DisabledNodes["A"] != 7 {
		t.Fatalf("карта после полного merge: %v", sub.DisabledNodes)
	}

	// Truncated: ключ без узла удерживается.
	sub.DisabledNodes["stale"] = 9
	MergeSubscriptionNodes(sub, &SubFetchMaterial{Nodes: []Node{serverNode("A", `{}`)}, Truncated: true}, true)
	if sub.DisabledNodes["stale"] != 9 || sub.DisabledNodes["A"] != 7 {
		t.Fatalf("карта после truncated merge: %v", sub.DisabledNodes)
	}

	// pending-отметка без старого timestamp получает мостовой «сейчас».
	sub2 := mergeTestSub()
	sub2.PendingDisabled = []string{"X"}
	MergeSubscriptionNodes(sub2, &SubFetchMaterial{Nodes: []Node{serverNode("X", `{}`)}}, true)
	if sub2.DisabledNodes["X"] != 42 {
		t.Fatalf("мостовая карта не отражает pending-отметку: %v", sub2.DisabledNodes)
	}
}

// Фикс ревью W3 (блокер 1б): отметка, живущая ТОЛЬКО в легаси-карте
// DisabledNodes (старый state.json, легаси-путь UI), втягивается в канон
// ДО перезаписи карты — пользовательская правка любым путём побеждает,
// merge её не «оживляет».
func TestMergePullsLegacyMapIntoCanon(t *testing.T) {
	sub := mergeTestSub(serverNode("A", `{}`), serverNode("B", `{}`))
	// Легаси-путь записал только карту; канонический enabled остался true.
	sub.DisabledNodes = map[string]int64{"B": 7}

	fresh := &SubFetchMaterial{Nodes: []Node{serverNode("A", `{}`), serverNode("B", `{}`)}}
	changed, _ := MergeSubscriptionNodes(sub, fresh, true)
	if !changed {
		t.Fatal("втягивание отметки меняет канон — changed обязан подняться")
	}
	if sub.Nodes[1].Enabled {
		t.Fatal("легаси-отметка карты не опустила канонический enabled — узел ожил")
	}
	if !sub.Nodes[0].Enabled {
		t.Fatal("втягивание зацепило узел без отметки")
	}
	if sub.DisabledNodes["B"] != 7 {
		t.Fatalf("timestamp легаси-отметки потерян: %v", sub.DisabledNodes)
	}
}

// Смена вида узла у провайдера: enabled живёт, detour (только у Server)
// теряется с warning, не молча.
func TestMergeKindChangeDropsDetourWithWarning(t *testing.T) {
	old := serverNode("A", `{}`)
	old.Enabled = false
	old.Detour = &NodeLink{Tag: "up"}
	sub := mergeTestSub(old)

	auto := Node{Kind: SourceKindAuto, Tag: "A", Enabled: true, Group: &AutoGroup{GroupType: AutoGroupURLTest, Members: []NodeLink{{FolderID: sub.ID, Tag: "B"}}}}
	_, warns := MergeSubscriptionNodes(sub, &SubFetchMaterial{Nodes: []Node{serverNode("B", `{}`), auto}}, true)
	got := sub.Nodes[1]
	if got.Kind != SourceKindAuto || got.Enabled {
		t.Fatalf("узел A после смены вида: %+v", got)
	}
	if got.Detour != nil {
		t.Fatal("detour у auto не существует типом")
	}
	if len(warns) != 1 {
		t.Fatalf("потеря detour обязана дать warning: %v", warns)
	}
}
