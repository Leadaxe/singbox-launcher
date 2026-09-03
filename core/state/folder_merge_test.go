package state

// Тесты merge-заливки подписки в ПАПКУ (SPEC 116 W1, критерий A5).
// Пять случаев таблицы: совпал / новый / исчез / исчез при truncated /
// недостоверный ответ. Проверяются enabled, detour, origin.subUrl и
// порядок nodes[].
//
// Политика папки отличается от подписочной ровно двумя вещами: участвуют
// только узлы этой заливки (origin.subUrl == url), и исчезнувший узел
// разыменовывается, а не удаляется.

import (
	"encoding/json"
	"testing"
)

const folderTestSubURL = "https://provider.example/sub"

func folderTestFolder(nodes ...Node) *Source {
	return &Source{
		Node:  Node{Kind: SourceKindFolder, Enabled: true},
		ID:    "01FOLDERMERGE0000000000000",
		Name:  "folder",
		Nodes: nodes,
	}
}

// subNode — узел папки, залитый подпиской url.
func subNode(tag, body, url string) Node {
	n := serverNode(tag, body)
	n.Origin = &Origin{Kind: OriginKindURI, Raw: "vless://old/" + tag, SubURL: url}
	return n
}

// freshNode — узел из свежего материала: парсер subUrl не проставляет.
func freshNode(tag, body string) Node {
	n := serverNode(tag, body)
	n.Origin = &Origin{Kind: OriginKindURI, Raw: "vless://fresh/" + tag}
	return n
}

func folderNodeByTag(t *testing.T, f *Source, tag string) *Node {
	t.Helper()
	for i := range f.Nodes {
		if f.Nodes[i].Tag == tag {
			return &f.Nodes[i]
		}
	}
	t.Fatalf("узел %q пропал из папки: %v", tag, mergedTags(f))
	return nil
}

// Случай 5 (первым — он же гард всей функции): недостоверный ответ не
// трогает nodes[] папки вообще; чужой kind и пустой url — тоже no-op.
func TestFolderMergeUntrustedIsNoop(t *testing.T) {
	f := folderTestFolder(subNode("A", `{"v":1}`, folderTestSubURL))
	fresh := &SubFetchMaterial{Nodes: []Node{freshNode("B", `{"v":2}`)}}

	if changed, _ := MergeFolderNodesFromSubscription(f, folderTestSubURL, fresh, false); changed {
		t.Fatal("недостоверный ответ обязан быть no-op")
	}
	if changed, _ := MergeFolderNodesFromSubscription(f, folderTestSubURL, nil, true); changed {
		t.Fatal("nil-материал обязан быть no-op")
	}
	if changed, _ := MergeFolderNodesFromSubscription(f, "", fresh, true); changed {
		t.Fatal("пустой subURL обязан быть no-op")
	}
	sub := mergeTestSub(serverNode("A", `{"v":1}`))
	if changed, _ := MergeFolderNodesFromSubscription(sub, folderTestSubURL, fresh, true); changed {
		t.Fatal("подписка не контейнер этой функции")
	}
	if !tagsEqual(mergedTags(f), "A") || string(f.Nodes[0].Body) != `{"v":1}` {
		t.Fatalf("nodes[] папки тронуты: %v", mergedTags(f))
	}
}

// Случай 1 (совпал) + 2 (новый): body/origin.raw освежены, enabled/detour и
// ПОЗИЦИЯ целы; новый добавлен включённым в хвост с проставленным subUrl.
func TestFolderMergeRefreshAndAdd(t *testing.T) {
	kept := subNode("A", `{"v":1}`, folderTestSubURL)
	kept.Enabled = false
	kept.Detour = &NodeLink{Tag: "up"}

	f := folderTestFolder(kept, subNode("B", `{"v":1}`, folderTestSubURL))

	// Порядок тела провайдера обратный — папка обязана сохранить свой.
	fresh := &SubFetchMaterial{Nodes: []Node{
		freshNode("B", `{"v":2}`),
		freshNode("A", `{"v":2}`),
		freshNode("C", `{"v":2}`),
	}}
	changed, warns := MergeFolderNodesFromSubscription(f, folderTestSubURL, fresh, true)
	if !changed {
		t.Fatal("merge не отметил изменение")
	}
	if len(warns) != 0 {
		t.Fatalf("неожиданные warnings: %v", warns)
	}
	if !tagsEqual(mergedTags(f), "A", "B", "C") {
		t.Fatalf("порядок папки не сохранён: %v", mergedTags(f))
	}

	a := folderNodeByTag(t, f, "A")
	if a.Enabled {
		t.Fatal("enabled=false не пережил заливку")
	}
	if a.Detour == nil || a.Detour.Tag != "up" {
		t.Fatalf("detour не пережил заливку: %+v", a.Detour)
	}
	if string(a.Body) != `{"v":2}` {
		t.Fatalf("body не освежён: %s", a.Body)
	}
	if a.Origin == nil || a.Origin.Raw != "vless://fresh/A" {
		t.Fatalf("origin.raw не освежён: %+v", a.Origin)
	}
	if nodeSubURL(a) != folderTestSubURL {
		t.Fatalf("subUrl совпавшего потерян: %q", nodeSubURL(a))
	}

	c := folderNodeByTag(t, f, "C")
	if !c.Enabled {
		t.Fatal("новый узел обязан родиться включённым")
	}
	if nodeSubURL(c) != folderTestSubURL {
		t.Fatalf("новому узлу не проставлен subUrl: %q", nodeSubURL(c))
	}
}

// Случай 3 (исчез): узел остаётся в папке на своём месте, origin.subUrl
// обнулён, warning выдан. Чужие узлы (другая подписка / ручной) не
// участвуют вовсе.
func TestFolderMergeDisappearedIsDereferencedNotDeleted(t *testing.T) {
	manual := serverNode("M", `{"v":1}`)
	other := subNode("O", `{"v":1}`, "https://other.example/sub")

	f := folderTestFolder(
		subNode("A", `{"v":1}`, folderTestSubURL),
		manual,
		subNode("GONE", `{"v":1}`, folderTestSubURL),
		other,
	)

	fresh := &SubFetchMaterial{Nodes: []Node{freshNode("A", `{"v":2}`)}}
	changed, warns := MergeFolderNodesFromSubscription(f, folderTestSubURL, fresh, true)
	if !changed {
		t.Fatal("merge не отметил изменение")
	}
	if !tagsEqual(mergedTags(f), "A", "M", "GONE", "O") {
		t.Fatalf("состав/порядок папки нарушен: %v", mergedTags(f))
	}

	gone := folderNodeByTag(t, f, "GONE")
	if nodeSubURL(gone) != "" {
		t.Fatalf("исчезнувший узел не разыменован: %q", nodeSubURL(gone))
	}
	if string(gone.Body) != `{"v":1}` || gone.Origin == nil || gone.Origin.Raw != "vless://old/GONE" {
		t.Fatalf("исчезнувший узел обязан остаться как был: %+v", gone)
	}
	if len(warns) != 1 {
		t.Fatalf("разыменование обязано дать ровно один warning: %v", warns)
	}

	if nodeSubURL(folderNodeByTag(t, f, "O")) != "https://other.example/sub" {
		t.Fatal("узел чужой подписки тронут заливкой")
	}
	if m := folderNodeByTag(t, f, "M"); nodeSubURL(m) != "" || string(m.Body) != `{"v":1}` {
		t.Fatalf("ручной узел тронут заливкой: %+v", m)
	}
}

// Случай 4 (исчез при truncated): ни один узел не разыменован — «исчез»
// неотличим от «остался за капом»; обновление и добавление работают.
func TestFolderMergeTruncatedKeepsLinks(t *testing.T) {
	f := folderTestFolder(
		subNode("A", `{"v":1}`, folderTestSubURL),
		subNode("BEYOND", `{"v":1}`, folderTestSubURL),
	)
	fresh := &SubFetchMaterial{
		Nodes:     []Node{freshNode("A", `{"v":2}`), freshNode("NEW", `{"v":2}`)},
		Truncated: true,
	}
	changed, warns := MergeFolderNodesFromSubscription(f, folderTestSubURL, fresh, true)
	if !changed {
		t.Fatal("merge не отметил изменение")
	}
	if len(warns) != 0 {
		t.Fatalf("при truncated разыменования нет — warnings лишние: %v", warns)
	}
	if !tagsEqual(mergedTags(f), "A", "BEYOND", "NEW") {
		t.Fatalf("состав при truncated: %v", mergedTags(f))
	}
	if nodeSubURL(folderNodeByTag(t, f, "BEYOND")) != folderTestSubURL {
		t.Fatal("при truncated разыменование запрещено")
	}
	if string(folderNodeByTag(t, f, "A").Body) != `{"v":2}` {
		t.Fatal("обновление совпавшего при truncated обязано работать")
	}
}

// Занятый сырой тег: узел свежего тела не подменяет чужой узел папки и не
// рождает второй с тем же тегом — деградирует с warning.
func TestFolderMergeForeignTagCollisionDegrades(t *testing.T) {
	manual := serverNode("A", `{"mine":true}`)
	f := folderTestFolder(manual)

	fresh := &SubFetchMaterial{Nodes: []Node{freshNode("A", `{"v":2}`)}}
	_, warns := MergeFolderNodesFromSubscription(f, folderTestSubURL, fresh, true)
	if !tagsEqual(mergedTags(f), "A") {
		t.Fatalf("второй узел с занятым тегом добавлен: %v", mergedTags(f))
	}
	if string(f.Nodes[0].Body) != `{"mine":true}` || nodeSubURL(&f.Nodes[0]) != "" {
		t.Fatalf("ручной узел подменён заливкой: %+v", f.Nodes[0])
	}
	if len(warns) != 1 {
		t.Fatalf("коллизия тега обязана дать warning: %v", warns)
	}
}

// Смена вида узла у провайдера: enabled живёт, detour (только у Server)
// теряется с warning — общая половина ядра работает и на папке.
func TestFolderMergeKindChangeDropsDetourWithWarning(t *testing.T) {
	old := subNode("A", `{}`, folderTestSubURL)
	old.Enabled = false
	old.Detour = &NodeLink{Tag: "up"}
	// Узел B лежит в папке заранее: члены группы разбираются отдельным тестом,
	// а здесь состав обязан резолвиться, чтобы warning остался ровно один — про
	// потерянный detour.
	f := folderTestFolder(old, subNode("B", `{}`, folderTestSubURL))

	auto := Node{
		Kind:    SourceKindAuto,
		Tag:     "A",
		Enabled: true,
		Group:   &AutoGroup{GroupType: AutoGroupURLTest, Members: []NodeLink{{FolderID: "01SUBSCRIPTION000000000000", Tag: "B"}}},
	}
	_, warns := MergeFolderNodesFromSubscription(f, folderTestSubURL,
		&SubFetchMaterial{Nodes: []Node{auto, freshNode("B", `{}`)}}, true)
	got := folderNodeByTag(t, f, "A")
	if got.Kind != SourceKindAuto || got.Enabled {
		t.Fatalf("узел A после смены вида: %+v", got)
	}
	if got.Detour != nil {
		t.Fatal("detour у auto не существует типом")
	}
	if nodeSubURL(got) != folderTestSubURL {
		t.Fatalf("узел без origin обязан получить origin с subUrl: %+v", got.Origin)
	}
	if len(warns) != 1 {
		t.Fatalf("потеря detour обязана дать warning: %v", warns)
	}
}

// Идемпотентность: повторная заливка того же тела ничего не меняет.
func TestFolderMergeIsIdempotent(t *testing.T) {
	f := folderTestFolder(subNode("A", `{"v":2}`, folderTestSubURL))
	fresh := &SubFetchMaterial{Nodes: []Node{freshNode("A", `{"v":2}`)}}
	MergeFolderNodesFromSubscription(f, folderTestSubURL, fresh, true)
	snapshot, _ := json.Marshal(f.Nodes)

	changed, warns := MergeFolderNodesFromSubscription(f, folderTestSubURL, fresh, true)
	after, _ := json.Marshal(f.Nodes)
	if changed || len(warns) != 0 || string(snapshot) != string(after) {
		t.Fatalf("повторная заливка не идемпотентна: changed=%v warns=%v", changed, warns)
	}
}

// SPEC 116 W7 (features/sources.md §Auto): Auto, приехавший заливкой,
// переуказывает members с подписки-источника на ПАПКУ; член, чья копия в папку
// не попала (тег занят чужим узлом), — prune с warning. Умолчание селектора
// снимается вместе с исчезнувшим членом.
func TestFolderMergeRepointsAutoMembers(t *testing.T) {
	const subID = "01SUBSCRIPTION000000000000"
	// C уже занят ручным узлом папки: копия члена C приехать не сможет.
	f := folderTestFolder(serverNode("C", `{"mine":true}`))

	auto := Node{
		Kind:    SourceKindAuto,
		Tag:     "G",
		Enabled: true,
		Group: &AutoGroup{
			GroupType: AutoGroupSelector,
			Default:   "C",
			Members: []NodeLink{
				{FolderID: subID, Tag: "B"},
				{FolderID: subID, Tag: "C"},
			},
		},
	}
	material := &SubFetchMaterial{Nodes: []Node{auto, freshNode("B", `{}`), freshNode("C", `{}`)}}

	_, warns := MergeFolderNodesFromSubscription(f, folderTestSubURL, material, true)

	got := folderNodeByTag(t, f, "G")
	if got.Group == nil || len(got.Group.Members) != 1 {
		t.Fatalf("состав группы после заливки: %+v", got.Group)
	}
	if got.Group.Members[0].FolderID != f.ID || got.Group.Members[0].Tag != "B" {
		t.Fatalf("член не переуказан на папку: %+v", got.Group.Members[0])
	}
	if got.Group.Default != "" {
		t.Fatalf("умолчание, выпавшее из состава, обязано сняться: %q", got.Group.Default)
	}
	// Материал вызывающего (узлы живой подписки) правкой на месте не задет.
	if material.Nodes[0].Group.Members[0].FolderID != subID || len(material.Nodes[0].Group.Members) != 2 {
		t.Fatalf("заливка переписала состав группы САМОЙ подписки: %+v", material.Nodes[0].Group)
	}
	// Ровно два warning'а: prune члена C (тег занят) и снятое умолчание.
	if len(warns) != 3 {
		t.Fatalf("ожидались warning'и о занятом теге, prune члена и снятом умолчании: %v", warns)
	}
}

// Ручной Auto папки заливка не трогает: он не приехал этой подпиской, и его
// состав — решение пользователя.
func TestFolderMergeLeavesForeignAutoMembersAlone(t *testing.T) {
	manual := Node{
		Kind:    SourceKindAuto,
		Tag:     "MyGroup",
		Enabled: true,
		Group: &AutoGroup{
			GroupType: AutoGroupURLTest,
			Members:   []NodeLink{{FolderID: "01OTHERFOLDER0000000000000", Tag: "ghost"}},
		},
	}
	f := folderTestFolder(manual)

	_, warns := MergeFolderNodesFromSubscription(f, folderTestSubURL,
		&SubFetchMaterial{Nodes: []Node{freshNode("A", `{}`)}}, true)

	got := folderNodeByTag(t, f, "MyGroup")
	if len(got.Group.Members) != 1 || got.Group.Members[0].Tag != "ghost" {
		t.Fatalf("ручной Auto тронут заливкой: %+v", got.Group)
	}
	if len(warns) != 0 {
		t.Fatalf("чужой Auto не повод для warning: %v", warns)
	}
}
