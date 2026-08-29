package business

import (
	"encoding/json"
	"testing"

	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 116 этап 3, W2 — перенос узла между контейнерами.
//
// Критерии: A3 (copy сохраняет subUrl; move переносит enabled и detour;
// ссылки на прежнюю идентичность собраны в список переписи) и A7 (вынос
// узлов папки в корень не теряет ни узла, порядок и enabled целы).
//
// Текст диалогов не проверяем (правило no-ui-format-tests) — проверяем
// состав модели и список задетых источников.

func moveTestNode(tag string, enabled bool, subURL string) corestate.Node {
	return corestate.Node{
		Kind:    corestate.SourceKindServer,
		Tag:     tag,
		Enabled: enabled,
		Origin:  &corestate.Origin{Kind: corestate.OriginKindURI, Raw: "vless://" + tag, SubURL: subURL},
		Body:    json.RawMessage(`{"type":"vless","server":"1.2.3.4"}`),
	}
}

func moveTestFolder(id, name string, nodes ...corestate.Node) corestate.Source {
	return corestate.Source{
		Node:  corestate.Node{Kind: corestate.SourceKindFolder, Enabled: true},
		ID:    id,
		Name:  name,
		Nodes: nodes,
	}
}

// findFolder возвращает папку по ULID (индексы плывут после выноса в корень).
func findFolder(t *testing.T, m *wizardmodels.WizardModel, id string) *corestate.Source {
	t.Helper()
	for i := range m.Sources {
		if m.Sources[i].ID == id && m.Sources[i].Kind == corestate.SourceKindFolder {
			return &m.Sources[i]
		}
	}
	t.Fatalf("папка %q не найдена", id)
	return nil
}

// A3: copy из подписки в папку — оригинал на месте, у копии subUrl сохранён.
func TestCopyNodeToFolder_KeepsSubURLAndOriginal(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		{
			Node:  corestate.Node{Kind: corestate.SourceKindSubscription, Enabled: true},
			ID:    "01SUB",
			Name:  "Proton",
			URL:   "https://example.com/sub",
			Nodes: []corestate.Node{moveTestNode("NL-1", false, "https://example.com/sub")},
		},
		moveTestFolder("01FOLDER", "Manual"),
	}}

	affected, err := CopyNodeToFolder(m, 0, "NL-1", "01FOLDER")
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if len(affected) != 0 {
		t.Fatalf("copy не двигает существующие ссылки, список переписи = %v", affected)
	}
	if len(m.Sources[0].Nodes) != 1 {
		t.Fatalf("оригинал в подписке пропал: %d узлов", len(m.Sources[0].Nodes))
	}
	folder := findFolder(t, m, "01FOLDER")
	if len(folder.Nodes) != 1 {
		t.Fatalf("копия не легла в папку: %d узлов", len(folder.Nodes))
	}
	cp := folder.Nodes[0]
	if cp.Origin == nil || cp.Origin.SubURL != "https://example.com/sub" {
		t.Fatalf("copy обязана сохранять origin.subUrl, получено %+v", cp.Origin)
	}
	if cp.Enabled {
		t.Fatalf("copy обязана нести enabled оригинала (false)")
	}
	// Копия владеет своим origin: разыменование копии не должно трогать оригинал.
	if !DereferenceNodeOrigin(&folder.Nodes[0]) {
		t.Fatalf("разыменование копии не сработало")
	}
	if m.Sources[0].Nodes[0].Origin.SubURL == "" {
		t.Fatalf("разыменование копии отвязало ОРИГИНАЛ подписки — origin разделён указателем")
	}
}

// A3: сырой тег, занятый в целевой папке, разрешается суффиксом.
func TestCopyNodeToFolder_TagCollisionSuffixed(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		{
			Node:  corestate.Node{Kind: corestate.SourceKindSubscription, Enabled: true},
			ID:    "01SUB",
			Nodes: []corestate.Node{moveTestNode("NL-1", true, "https://example.com/sub")},
		},
		moveTestFolder("01FOLDER", "Manual", moveTestNode("NL-1", true, "")),
	}}

	if _, err := CopyNodeToFolder(m, 0, "NL-1", "01FOLDER"); err != nil {
		t.Fatalf("copy: %v", err)
	}
	folder := findFolder(t, m, "01FOLDER")
	if len(folder.Nodes) != 2 {
		t.Fatalf("ожидалось два узла в папке, получено %d", len(folder.Nodes))
	}
	if folder.Nodes[0].Tag != "NL-1" || folder.Nodes[1].Tag != "NL-1-2" {
		t.Fatalf("коллизия сырого тега не разрешена суффиксом: %q, %q",
			folder.Nodes[0].Tag, folder.Nodes[1].Tag)
	}
}

// A3: move из подписки запрещён — состав подписки принадлежит провайдеру.
func TestMoveNodeToFolder_RefusesSubscriptionSource(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		{
			Node:  corestate.Node{Kind: corestate.SourceKindSubscription, Enabled: true},
			ID:    "01SUB",
			Nodes: []corestate.Node{moveTestNode("NL-1", true, "https://example.com/sub")},
		},
		moveTestFolder("01FOLDER", "Manual"),
	}}

	if _, err := MoveNodeToFolder(m, 0, "NL-1", "01FOLDER"); err == nil {
		t.Fatalf("move из подписки обязан отказывать")
	}
	if len(m.Sources[0].Nodes) != 1 {
		t.Fatalf("отказавший move всё-таки унёс узел из подписки")
	}
	if len(m.Sources[1].Nodes) != 0 {
		t.Fatalf("отказавший move всё-таки положил узел в папку")
	}
}

// A3: move между папками — enabled и detour едут, ссылки на прежний адрес
// переписаны, задетые источники названы.
func TestMoveNodeToFolder_CarriesMarksAndRepointsLinks(t *testing.T) {
	moving := moveTestNode("NL-1", false, "https://example.com/sub")
	moving.Detour = &corestate.NodeLink{Tag: "warp"}

	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		moveTestFolder("01SRC", "Source folder", moving),
		moveTestFolder("01DST", "Target folder"),
		// Цепочка в корне ходит через переносимый узел.
		{
			ID:    "01CHAIN",
			Label: "Chain to NL",
			Node: corestate.Node{
				Kind: corestate.SourceKindChain, Tag: "chain-nl", Enabled: true,
				Hops: []corestate.NodeLink{{FolderID: "01SRC", Tag: "NL-1"}},
			},
		},
		// Auto-группа держит его членом и умолчанием.
		{
			ID:    "01AUTO",
			Label: "Auto NL",
			Node: corestate.Node{
				Kind: corestate.SourceKindAuto, Tag: "auto-nl", Enabled: true,
				Group: &corestate.AutoGroup{
					GroupType: corestate.AutoGroupSelector,
					Default:   "NL-1",
					Members:   []corestate.NodeLink{{FolderID: "01SRC", Tag: "NL-1"}},
				},
			},
		},
		// Чужая ссылка на тот же сырой тег в ДРУГОЙ папке — трогать нельзя.
		{
			ID:    "01OTHER",
			Label: "Other chain",
			Node: corestate.Node{
				Kind: corestate.SourceKindChain, Tag: "chain-other", Enabled: true,
				Hops: []corestate.NodeLink{{FolderID: "01ELSE", Tag: "NL-1"}},
			},
		},
	}}

	affected, err := MoveNodeToFolder(m, 0, "NL-1", "01DST")
	if err != nil {
		t.Fatalf("move: %v", err)
	}

	if len(m.Sources[0].Nodes) != 0 {
		t.Fatalf("узел не ушёл из исходной папки")
	}
	dst := findFolder(t, m, "01DST")
	if len(dst.Nodes) != 1 {
		t.Fatalf("узел не приехал в целевую папку")
	}
	got := dst.Nodes[0]
	if got.Enabled {
		t.Fatalf("enabled=false не переехал вместе с узлом")
	}
	if got.Detour == nil || got.Detour.Tag != "warp" {
		t.Fatalf("личный detour не переехал: %+v", got.Detour)
	}
	if got.Origin == nil || got.Origin.SubURL != "https://example.com/sub" {
		t.Fatalf("move не должен разыменовывать узел: %+v", got.Origin)
	}

	if h := m.Sources[2].Hops[0]; h.FolderID != "01DST" || h.Tag != "NL-1" {
		t.Fatalf("хоп цепочки не переписан: %+v", h)
	}
	g := m.Sources[3].Group
	if g.Members[0].FolderID != "01DST" || g.Members[0].Tag != "NL-1" {
		t.Fatalf("член группы не переписан: %+v", g.Members[0])
	}
	if g.Default != "NL-1" {
		t.Fatalf("умолчание селектора разъехалось с составом: %q", g.Default)
	}
	if h := m.Sources[4].Hops[0]; h.FolderID != "01ELSE" || h.Tag != "NL-1" {
		t.Fatalf("переписана ЧУЖАЯ ссылка на одноимённый тег другой папки: %+v", h)
	}

	if len(affected) != 2 {
		t.Fatalf("список переписи = %v, ожидались две записи", affected)
	}
	names := map[string]bool{affected[0]: true, affected[1]: true}
	if !names["Chain to NL"] || !names["Auto NL"] {
		t.Fatalf("список переписи = %v, ожидались Chain to NL и Auto NL", affected)
	}
}

// A3: move ИЗ КОРНЯ в папку — верхний Source исчезает, NodeLink переписан, а
// ссылки классов, которые в папку указать не могут (цель правила,
// route.final), не переписываются молча, а названы в списке предупреждения.
func TestMoveNodeToFolder_FromRootNamesUnrewritableRefs(t *testing.T) {
	m := &wizardmodels.WizardModel{
		Sources: []corestate.Source{
			{ID: "01TOP", Label: "WARP hop", Node: moveTestNode("warp", true, "")},
			moveTestFolder("01DST", "Target folder"),
			{
				ID:    "01CHAIN",
				Label: "Chain via warp",
				Node: corestate.Node{
					Kind: corestate.SourceKindChain, Tag: "chain-warp", Enabled: true,
					Hops: []corestate.NodeLink{{Tag: "warp"}},
				},
			},
		},
		CustomRules: []*wizardmodels.RuleState{
			{SelectedOutbound: "warp", Rule: wizardtemplate.TemplateSelectableRule{Label: "Torrents"}},
			{SelectedOutbound: "direct-out", Rule: wizardtemplate.TemplateSelectableRule{Label: "LAN"}},
		},
		SelectedFinalOutbound: "warp",
	}

	affected, err := MoveNodeToFolder(m, 0, "warp", "01DST")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if len(m.Sources) != 2 {
		t.Fatalf("верхний узел не убран из корня: %d источников", len(m.Sources))
	}
	dst := findFolder(t, m, "01DST")
	if len(dst.Nodes) != 1 || dst.Nodes[0].Tag != "warp" {
		t.Fatalf("узел не приехал в папку: %+v", dst.Nodes)
	}
	chain := m.Sources[1]
	if chain.Hops[0].FolderID != "01DST" || chain.Hops[0].Tag != "warp" {
		t.Fatalf("хоп не переписан на адрес папки: %+v", chain.Hops[0])
	}

	names := map[string]bool{}
	for _, n := range affected {
		names[n] = true
	}
	if !names["Chain via warp"] {
		t.Fatalf("переписанная цепочка не названа: %v", affected)
	}
	if !names["Torrents"] || !names["route.final"] {
		t.Fatalf("ссылки, которые нельзя переписать, не названы: %v", affected)
	}
	if names["LAN"] {
		t.Fatalf("названо чужое правило: %v", affected)
	}
	// Значения не переписаны — их чинит штатный сброс осиротевших целей.
	if m.CustomRules[0].SelectedOutbound != "warp" || m.SelectedFinalOutbound != "warp" {
		t.Fatalf("корневые ссылки молча переписаны в адрес папки")
	}
}

// A7: вынос узлов папки в корень — ни одного узла не потеряно, порядок и
// enabled сохранены, ссылки переписаны в корневое пространство.
func TestExtractFolderNodesToRoot_KeepsOrderAndMarks(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		moveTestFolder("01FOLDER", "Proton",
			moveTestNode("NL-1", true, ""),
			moveTestNode("NL-2", false, "https://example.com/sub"),
			moveTestNode("NL-3", true, ""),
		),
		{
			ID:    "01CHAIN",
			Label: "Chain to NL-2",
			Node: corestate.Node{
				Kind: corestate.SourceKindChain, Tag: "chain-nl", Enabled: true,
				Hops: []corestate.NodeLink{{FolderID: "01FOLDER", Tag: "NL-2"}},
			},
		},
	}}

	affected := ExtractFolderNodesToRoot(m, 0)

	if len(m.Sources) != 5 {
		t.Fatalf("ожидалось 5 источников (папка + 3 узла + цепочка), получено %d", len(m.Sources))
	}
	folder := findFolder(t, m, "01FOLDER")
	if len(folder.Nodes) != 0 {
		t.Fatalf("папка обязана остаться пустой, в ней %d узлов", len(folder.Nodes))
	}
	wantTags := []string{"NL-1", "NL-2", "NL-3"}
	wantEnabled := []bool{true, false, true}
	for i, want := range wantTags {
		s := m.Sources[i+1]
		if s.Kind != corestate.SourceKindServer {
			t.Fatalf("узел %d вынесен не как server: %q", i, s.Kind)
		}
		if s.Tag != want {
			t.Fatalf("порядок вынесенных узлов нарушен: %d-й = %q, ожидался %q", i, s.Tag, want)
		}
		if s.Enabled != wantEnabled[i] {
			t.Fatalf("enabled узла %q не сохранён", want)
		}
		if s.ID == "" {
			t.Fatalf("вынесенному узлу %q не выдан ULID", want)
		}
	}

	// Ссылка ушла из пространства папки в корневое.
	chain := m.Sources[4]
	if chain.Hops[0].FolderID != "" || chain.Hops[0].Tag != "NL-2" {
		t.Fatalf("хоп не переписан в корневое пространство: %+v", chain.Hops[0])
	}
	if len(affected) != 1 || affected[0] != "Chain to NL-2" {
		t.Fatalf("список переписи = %v, ожидался [Chain to NL-2]", affected)
	}
}

// A7: имя, уже занятое в корне, разрешается суффиксом — иначе два узла
// столкнулись бы одним финальным тегом.
func TestExtractFolderNodesToRoot_UniquifiesAgainstRoot(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		{ID: "01TOP", Node: corestate.Node{Kind: corestate.SourceKindServer, Tag: "NL-1", Enabled: true}},
		moveTestFolder("01FOLDER", "Proton", moveTestNode("NL-1", true, "")),
	}}

	ExtractFolderNodesToRoot(m, 1)

	if len(m.Sources) != 3 {
		t.Fatalf("ожидалось 3 источника, получено %d", len(m.Sources))
	}
	if m.Sources[2].Tag != "NL-1-2" {
		t.Fatalf("коллизия корневого тега не разрешена суффиксом: %q", m.Sources[2].Tag)
	}
}

// Д5: разыменование обнуляет subUrl и только его; узел без связи не меняется.
func TestDereferenceNodeOrigin(t *testing.T) {
	n := moveTestNode("NL-1", true, "https://example.com/sub")
	if !DereferenceNodeOrigin(&n) {
		t.Fatalf("разыменование не сработало")
	}
	if n.Origin.SubURL != "" {
		t.Fatalf("subUrl не обнулён: %q", n.Origin.SubURL)
	}
	if n.Origin.Raw != "vless://NL-1" || n.Origin.Kind != corestate.OriginKindURI {
		t.Fatalf("разыменование задело остальной origin: %+v", n.Origin)
	}
	if DereferenceNodeOrigin(&n) {
		t.Fatalf("повторное разыменование обязано быть no-op")
	}

	manual := moveTestNode("NL-2", true, "")
	if DereferenceNodeOrigin(&manual) {
		t.Fatalf("узел без subUrl менять нечего")
	}
	if DereferenceNodeOrigin(nil) {
		t.Fatalf("nil-узел — no-op")
	}
}
