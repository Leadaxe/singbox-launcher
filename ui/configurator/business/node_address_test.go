package business

import (
	"testing"

	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

func addrModel() *wizardmodels.WizardModel {
	return &wizardmodels.WizardModel{
		Sources: []corestate.Source{
			{Node: corestate.Node{Kind: corestate.SourceKindServer, Tag: "root-a", Enabled: true}},
			{
				ID:   "F1",
				Node: corestate.Node{Kind: corestate.SourceKindFolder, Enabled: true},
				Nodes: []corestate.Node{
					{Kind: corestate.SourceKindServer, Tag: "in-folder", Enabled: true},
					// Тот же тег, что у корневого узла: адрес обязан их различать.
					{Kind: corestate.SourceKindServer, Tag: "root-a", Enabled: true},
				},
			},
			{ID: "F2", Node: corestate.Node{Kind: corestate.SourceKindSubscription, Enabled: true},
				Nodes: []corestate.Node{{Kind: corestate.SourceKindServer, Tag: "sub-node", Enabled: true}}},
		},
	}
}

// TestNodeByLinkDistinguishesSpaces — корневой и папочный узлы с ОДНИМ тегом
// это разные узлы. Пустой FolderID адресует корень, заданный — состав папки;
// перепутать их значит применить правку не к тому узлу.
func TestNodeByLinkDistinguishesSpaces(t *testing.T) {
	m := addrModel()

	root := NodeByLink(m, corestate.NodeLink{Tag: "root-a"})
	if root == nil {
		t.Fatal("корневой узел не найден по пустому FolderID")
	}
	if root != &m.Sources[0].Node {
		t.Fatal("корневой адрес указал не на встроенный Node строки корня")
	}

	inFolder := NodeByLink(m, corestate.NodeLink{FolderID: "F1", Tag: "root-a"})
	if inFolder == nil {
		t.Fatal("одноимённый узел папки не найден")
	}
	if inFolder == root {
		t.Fatal("адреса корня и папки разрешились в ОДИН узел — правка ушла бы не туда")
	}
	if inFolder != &m.Sources[1].Nodes[1] {
		t.Fatal("папочный адрес указал не на тот узел состава")
	}
}

// TestNodeByLinkMutatesInPlace — адресат возвращается указателем: правка через
// него обязана попадать в модель, иначе окно правило бы копию.
func TestNodeByLinkMutatesInPlace(t *testing.T) {
	m := addrModel()
	n := NodeByLink(m, corestate.NodeLink{FolderID: "F1", Tag: "in-folder"})
	if n == nil {
		t.Fatal("узел не найден")
	}
	n.Enabled = false
	if m.Sources[1].Nodes[0].Enabled {
		t.Fatal("правка через адрес не дошла до модели")
	}
}

// TestNodeByLinkMissing — исчезнувший узел даёт nil, а не панику и не соседа.
func TestNodeByLinkMissing(t *testing.T) {
	m := addrModel()
	cases := []corestate.NodeLink{
		{Tag: "ghost"},
		{FolderID: "F1", Tag: "ghost"},
		{FolderID: "NOPE", Tag: "in-folder"},
		{Tag: ""},
		{FolderID: "F1", Tag: ""},
	}
	for _, link := range cases {
		if got := NodeByLink(m, link); got != nil {
			t.Fatalf("%+v разрешился в узел %q, ожидался nil", link, got.Tag)
		}
	}
}

// TestNodeByLinkSkipsContainers — корневой адрес не должен поймать папку или
// подписку: у них своё имя, а узлом они не являются.
func TestNodeByLinkSkipsContainers(t *testing.T) {
	m := addrModel()
	m.Sources[1].Tag = "F1-tag"
	if got := NodeByLink(m, corestate.NodeLink{Tag: "F1-tag"}); got != nil {
		t.Fatal("корневой адрес поймал контейнер — он не узел")
	}
}

// TestSourceIndexByLink — индекс ВЫВОДИТСЯ из адреса и переживает
// перестановку строк, в отличие от запомненного номера.
func TestSourceIndexByLink(t *testing.T) {
	m := addrModel()
	if got := SourceIndexByLink(m, corestate.NodeLink{FolderID: "F1", Tag: "in-folder"}); got != 1 {
		t.Fatalf("индекс контейнера = %d, ожидался 1", got)
	}
	if got := SourceIndexByLink(m, corestate.NodeLink{Tag: "root-a"}); got != 0 {
		t.Fatalf("индекс корневого узла = %d, ожидался 0", got)
	}
	// Перестановка: адрес прежний, индекс новый.
	m.Sources[0], m.Sources[1] = m.Sources[1], m.Sources[0]
	if got := SourceIndexByLink(m, corestate.NodeLink{FolderID: "F1", Tag: "in-folder"}); got != 0 {
		t.Fatalf("после перестановки индекс = %d, ожидался 0", got)
	}
	if got := SourceIndexByLink(m, corestate.NodeLink{Tag: "ghost"}); got != -1 {
		t.Fatalf("несуществующий адрес дал индекс %d, ожидался -1", got)
	}
}

// TestSourceByLink — контейнер узла; у корневого его нет.
func TestSourceByLink(t *testing.T) {
	m := addrModel()
	if got := SourceByLink(m, corestate.NodeLink{FolderID: "F2", Tag: "sub-node"}); got == nil || got.ID != "F2" {
		t.Fatalf("контейнер узла подписки не найден: %+v", got)
	}
	if got := SourceByLink(m, corestate.NodeLink{Tag: "root-a"}); got != nil {
		t.Fatal("у корневого узла контейнера нет")
	}
}
