// File node_rename_delete_test.go — реестр ссылок при переименовании и
// удалении узла контейнера (SPEC 116 этап 3, W5; дыра Д7, критерий A3).
//
// Данные, не тексты: проверяется состав ссылок после операции и список
// задетых источников. Подписи диалогов не покрываем (no-ui-format-tests).
package business

import (
	"testing"

	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// renameModel — папка с узлом NL-1, на который ссылаются цепочка (хопом),
// Auto-группа (членом и умолчанием) и сосед по папке (детуром), плюс ЧУЖАЯ
// ссылка на одноимённый тег в другой папке — она трогаться не должна.
func renameModel() *wizardmodels.WizardModel {
	neighbour := moveTestNode("NL-2", true, "")
	neighbour.Detour = &corestate.NodeLink{FolderID: "01SRC", Tag: "NL-1"}

	return &wizardmodels.WizardModel{Sources: []corestate.Source{
		moveTestFolder("01SRC", "Source folder", moveTestNode("NL-1", true, ""), neighbour),
		{
			ID:    "01CHAIN",
			Label: "Chain to NL",
			Node: corestate.Node{
				Kind: corestate.SourceKindChain, Tag: "chain-nl", Enabled: true,
				Hops: []corestate.NodeLink{{FolderID: "01SRC", Tag: "NL-1"}},
			},
		},
		{
			ID:    "01AUTO",
			Label: "Auto NL",
			Node: corestate.Node{
				Kind: corestate.SourceKindAuto, Tag: "auto-nl", Enabled: true,
				Group: &corestate.AutoGroup{
					GroupType: corestate.AutoGroupSelector,
					Default:   "NL-1",
					Members: []corestate.NodeLink{
						{FolderID: "01SRC", Tag: "NL-1"},
						{FolderID: "01SRC", Tag: "NL-2"},
					},
				},
			},
		},
		{
			ID:    "01OTHER",
			Label: "Other chain",
			Node: corestate.Node{
				Kind: corestate.SourceKindChain, Tag: "chain-other", Enabled: true,
				Hops: []corestate.NodeLink{{FolderID: "01ELSE", Tag: "NL-1"}},
			},
		},
	}}
}

// A3: переименование узла контейнера переписывает ВСЕ виды ссылок на него и
// не задевает одноимённый тег чужой папки.
func TestRepointContainerNodeLinks_RenameInPlace(t *testing.T) {
	m := renameModel()

	affected := RepointContainerNodeLinks(m, "01SRC", "NL-1", "NL-1-renamed")

	if d := m.Sources[0].Nodes[1].Detour; d == nil || d.FolderID != "01SRC" || d.Tag != "NL-1-renamed" {
		t.Fatalf("детур соседа по папке не переписан: %+v", d)
	}
	if h := m.Sources[1].Hops[0]; h.FolderID != "01SRC" || h.Tag != "NL-1-renamed" {
		t.Fatalf("хоп цепочки не переписан: %+v", h)
	}
	g := m.Sources[2].Group
	if g.Members[0].Tag != "NL-1-renamed" {
		t.Fatalf("член группы не переписан: %+v", g.Members[0])
	}
	if g.Default != "NL-1-renamed" {
		t.Fatalf("умолчание селектора разъехалось с составом: %q", g.Default)
	}
	if h := m.Sources[3].Hops[0]; h.FolderID != "01ELSE" || h.Tag != "NL-1" {
		t.Fatalf("переписана ЧУЖАЯ ссылка одноимённого тега: %+v", h)
	}

	names := map[string]bool{}
	for _, n := range affected {
		names[n] = true
	}
	if !names["Source folder"] || !names["Chain to NL"] || !names["Auto NL"] {
		t.Fatalf("список переписи = %v, ожидались три источника", affected)
	}
	if names["Other chain"] {
		t.Fatalf("чужой источник попал в список переписи: %v", affected)
	}
}

// A3: удаление узла гасит ссылки на него — перенаправлять не на что, и
// подставлять соседа вместо удалённого нельзя.
func TestClearContainerNodeLinks_DropsRefsToDeletedNode(t *testing.T) {
	m := renameModel()

	affected := ClearContainerNodeLinks(m, "01SRC", "NL-1")

	if d := m.Sources[0].Nodes[1].Detour; d != nil {
		t.Fatalf("детур на удалённый узел не погашен: %+v", d)
	}
	if len(m.Sources[1].Hops) != 0 {
		t.Fatalf("хоп на удалённый узел остался: %+v", m.Sources[1].Hops)
	}
	g := m.Sources[2].Group
	if len(g.Members) != 1 || g.Members[0].Tag != "NL-2" {
		t.Fatalf("состав группы не почищен: %+v", g.Members)
	}
	if g.Default != "" {
		t.Fatalf("умолчание осталось на выбывшем члене: %q", g.Default)
	}
	if h := m.Sources[3].Hops[0]; h.FolderID != "01ELSE" || h.Tag != "NL-1" {
		t.Fatalf("погашена ЧУЖАЯ ссылка одноимённого тега: %+v", h)
	}

	names := map[string]bool{}
	for _, n := range affected {
		names[n] = true
	}
	if !names["Source folder"] || !names["Chain to NL"] || !names["Auto NL"] {
		t.Fatalf("список задетых = %v, ожидались три источника", affected)
	}
	if names["Other chain"] {
		t.Fatalf("чужой источник попал в список задетых: %v", affected)
	}
}

// Переименование В ТОТ ЖЕ тег — no-op: ни правок, ни ложного списка переписи
// (иначе пользователю показали бы предупреждение на пустом месте).
func TestRepointContainerNodeLinks_SameTagIsNoop(t *testing.T) {
	m := renameModel()
	if affected := RepointContainerNodeLinks(m, "01SRC", "NL-1", "NL-1"); len(affected) != 0 {
		t.Fatalf("переименование в то же имя дало список переписи: %v", affected)
	}
	if h := m.Sources[1].Hops[0]; h.Tag != "NL-1" {
		t.Fatalf("ссылка изменилась при no-op: %+v", h)
	}
}
