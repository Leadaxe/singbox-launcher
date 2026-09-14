package business

// SPEC 116 этап 3, W7 — заливка подписки в папку (сценарий С5, критерий A5).
//
// Проверяется ровно то, что принадлежит ЭТОМУ слою: откуда берётся материал
// (уже материализованные nodes[], без повторного разбора тела), как в merge
// попадает Truncated, что делает пустая подписка и что заливка не привязывает
// узлы папки к живым узлам подписки общими указателями. Сама механика merge
// покрыта core/state/folder_merge_test.go — дублировать её здесь незачем.
//
// Текст диалогов не проверяем (правило no-ui-format-tests).

import (
	"encoding/json"
	"errors"
	"testing"

	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

const fillTestSubURL = "https://provider.example/sub"

func fillTestSub(nodes ...corestate.Node) corestate.Source {
	return corestate.Source{
		Node:  corestate.Node{Kind: corestate.SourceKindSubscription, Enabled: true},
		ID:    "01SUBFILL",
		Name:  "Proton",
		URL:   fillTestSubURL,
		Nodes: nodes,
	}
}

// fillTestSubNode — узел, каким его положил fetch-конвейер: subUrl внутри
// своей подписки пуст (контейнер и есть связь, sources_v7.go:47).
func fillTestSubNode(tag string, enabled bool) corestate.Node {
	return corestate.Node{
		Kind:    corestate.SourceKindServer,
		Tag:     tag,
		Enabled: enabled,
		Origin:  &corestate.Origin{Kind: corestate.OriginKindURI, Raw: "vless://" + tag},
		Body:    json.RawMessage(`{"type":"vless","server":"1.2.3.4"}`),
	}
}

// Заливка берёт узлы подписки как есть и штампует им subUrl; при повторном
// вызове ничего не меняется (отдельного «обновить папку» нет — сценарий С5).
//
// detour владельца BYPASS на свой релей ведёт после заливки на КОПИЮ релея в
// папке (NODE_LINK.md §9.3 п. 8): через узел подписки копия выпала бы из
// конфига вместе с выключенной подпиской. Релей без копии в папке (тег занят
// ручным узлом) оставляет ссылку на подписке — подменить цель нельзя.
func TestFillFolderFromSubscription_FillsAndIsIdempotent(t *testing.T) {
	owner := fillTestSubNode("NL-1", true)
	owner.Detour = &corestate.NodeLink{FolderID: "01SUBFILL", Tag: "NL-1 · relay"}
	relay := fillTestSubNode("NL-1 · relay", true)
	relay.Service = true
	blocked := fillTestSubNode("DE-1", false)
	blocked.Detour = &corestate.NodeLink{FolderID: "01SUBFILL", Tag: "DE-1 · relay"}
	manual := moveTestNode("DE-1 · relay", true, "")
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		fillTestSub(owner, relay, blocked, fillTestSubNode("DE-1 · relay", true)),
		moveTestFolder("01FOLDER", "My folder", manual),
	}}

	res, err := FillFolderFromSubscription(m, "01FOLDER", "01SUBFILL")
	if err != nil {
		t.Fatalf("заливка отказала: %v", err)
	}
	// Одно предупреждение ожидаемо: релей DE-1 отбит ручным узлом с тем же
	// тегом.
	if !res.Changed || len(res.Warnings) != 1 {
		t.Fatalf("первая заливка: changed=%v warns=%v", res.Changed, res.Warnings)
	}
	folder := findFolder(t, m, "01FOLDER")
	if len(folder.Nodes) != 4 {
		t.Fatalf("узлы не легли в папку: %+v", folder.Nodes)
	}
	for i := range folder.Nodes[1:] {
		n := &folder.Nodes[i+1]
		if n.Origin == nil || n.Origin.SubURL != fillTestSubURL {
			t.Fatalf("узлу папки не проставлен subUrl: %+v", n.Origin)
		}
		if !n.Enabled {
			t.Fatalf("новый узел обязан родиться включённым: %+v", n)
		}
	}
	if d := folder.Nodes[1].Detour; d == nil || d.FolderID != "01FOLDER" || d.Tag != "NL-1 · relay" {
		t.Fatalf("detour копии не переуказан на копию релея: %+v", d)
	}
	if d := folder.Nodes[3].Detour; d == nil || d.FolderID != "01SUBFILL" || d.Tag != "DE-1 · relay" {
		t.Fatalf("detour без копии релея в папке обязан остаться на подписке: %+v", d)
	}
	if d := m.Sources[0].Nodes[0].Detour; d == nil || d.FolderID != "01SUBFILL" {
		t.Fatalf("заливка изменила detour узла подписки: %+v", d)
	}

	res2, err := FillFolderFromSubscription(m, "01FOLDER", "01SUBFILL")
	if err != nil {
		t.Fatalf("повторная заливка отказала: %v", err)
	}
	if res2.Changed || len(res2.Warnings) != 1 {
		t.Fatalf("повторная заливка не идемпотентна: changed=%v warns=%v", res2.Changed, res2.Warnings)
	}
}

// Пользовательские пометки узла папки переживают заливку, а сама подписка от
// заливки не меняется ни одним полем (её узлы — не материал на выброс, а
// живое состояние).
func TestFillFolderFromSubscription_KeepsUserMarksAndDoesNotTouchSubscription(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		fillTestSub(fillTestSubNode("NL-1", true)),
		moveTestFolder("01FOLDER", "My folder", moveTestNode("NL-1", false, fillTestSubURL)),
	}}
	m.Sources[1].Nodes[0].Detour = &corestate.NodeLink{Tag: "hop"}
	subBefore, _ := json.Marshal(m.Sources[0])

	if _, err := FillFolderFromSubscription(m, "01FOLDER", "01SUBFILL"); err != nil {
		t.Fatalf("заливка отказала: %v", err)
	}

	got := findFolder(t, m, "01FOLDER").Nodes[0]
	if got.Enabled {
		t.Fatal("enabled=false не пережил заливку")
	}
	if got.Detour == nil || got.Detour.Tag != "hop" {
		t.Fatalf("detour не пережил заливку: %+v", got.Detour)
	}
	subAfter, _ := json.Marshal(m.Sources[0])
	if string(subBefore) != string(subAfter) {
		t.Fatalf("заливка изменила подписку-донора:\nдо:    %s\nпосле: %s", subBefore, subAfter)
	}
	// Общих указателей между узлом папки и узлом подписки не остаётся: правка
	// origin копии не должна доставать до провайдерского узла.
	if got.Origin == m.Sources[0].Nodes[0].Origin {
		t.Fatal("узел папки делит Origin с узлом подписки")
	}
}

// Truncated берётся из UpdateStatus и доезжает до merge: исчезнувший у
// провайдера узел при обрезке НЕ разыменовывается (SPEC 113-A).
func TestFillFolderFromSubscription_TruncatedBlocksDereference(t *testing.T) {
	sub := fillTestSub(fillTestSubNode("NL-1", true))
	sub.UpdateStatus = &corestate.SubUpdateStatus{Truncated: true}
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		sub,
		moveTestFolder("01FOLDER", "My folder", moveTestNode("BEYOND", true, fillTestSubURL)),
	}}

	res, err := FillFolderFromSubscription(m, "01FOLDER", "01SUBFILL")
	if err != nil {
		t.Fatalf("заливка отказала: %v", err)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("при truncated разыменования нет — warnings лишние: %v", res.Warnings)
	}
	folder := findFolder(t, m, "01FOLDER")
	if folder.Nodes[0].Origin == nil || folder.Nodes[0].Origin.SubURL != fillTestSubURL {
		t.Fatalf("при truncated разыменование запрещено: %+v", folder.Nodes[0].Origin)
	}

	// Тот же материал без флага обрезки — узел разыменован и остался в папке.
	m.Sources[0].UpdateStatus = nil
	res, err = FillFolderFromSubscription(m, "01FOLDER", "01SUBFILL")
	if err != nil {
		t.Fatalf("заливка отказала: %v", err)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("исчезнувший узел обязан дать ровно один warning: %v", res.Warnings)
	}
	folder = findFolder(t, m, "01FOLDER")
	if len(folder.Nodes) != 2 {
		t.Fatalf("исчезнувший узел удалён из папки: %+v", folder.Nodes)
	}
	if folder.Nodes[0].Tag != "BEYOND" || folder.Nodes[0].Origin.SubURL != "" {
		t.Fatalf("исчезнувший узел не разыменован на месте: %+v", folder.Nodes[0])
	}
}

// Подписку ни разу не обновляли: заливать нечего — сентинел, по которому UI
// предложит обновление. Своего fetch'а на этом слое нет.
func TestFillFolderFromSubscription_NotFetchedIsSentinel(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		fillTestSub(),
		moveTestFolder("01FOLDER", "My folder"),
	}}
	_, err := FillFolderFromSubscription(m, "01FOLDER", "01SUBFILL")
	if !errors.Is(err, ErrSubscriptionNotFetched) {
		t.Fatalf("пустая подписка обязана дать сентинел, получено: %v", err)
	}
	if len(findFolder(t, m, "01FOLDER").Nodes) != 0 {
		t.Fatal("отказ заливки не вправе трогать папку")
	}
}

// Список доноров — только подписки, в порядке Sources, со счётчиком узлов.
func TestFolderFillSubscriptions_ListsSubscriptionsOnly(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		moveTestFolder("01FOLDER", "My folder"),
		fillTestSub(fillTestSubNode("NL-1", true)),
		{Node: corestate.Node{Kind: corestate.SourceKindServer, Tag: "single", Enabled: true}, ID: "01SRV"},
	}}
	got := FolderFillSubscriptions(m)
	if len(got) != 1 {
		t.Fatalf("в списке доноров обязана быть одна подписка: %+v", got)
	}
	if got[0].ID != "01SUBFILL" || got[0].URL != fillTestSubURL || got[0].NodeCount != 1 {
		t.Fatalf("строка донора: %+v", got[0])
	}
}
