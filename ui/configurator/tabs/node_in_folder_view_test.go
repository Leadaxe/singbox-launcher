package tabs

import (
	"testing"

	corestate "singbox-launcher/core/state"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// Окно, открытое на УЗЕЛ внутри папки, обязано жить видом УЗЛА, а не папки.
//
// Регрессия обкатки (заход 3): sourceIndex у такого окна указывает на
// контейнер, и всякий, кто пересчитывал вид по нему, показывал форму папки
// («Folder name», тег-политика, «Fold this subscription») над узлом WireGuard.
// Заголовок брал имя оттуда же — «Source — Folder 1» вместо имени узла.
func TestNodeInFolderUsesNodeViewNotFolderView(t *testing.T) {
	folder := corestate.NewFolderSource("Folder 1")
	folder.Nodes = []corestate.Node{{
		Kind:    corestate.SourceKindServer,
		Tag:     "185.107.56.130",
		Enabled: true,
		Origin:  &corestate.Origin{Kind: corestate.OriginKindWGIni, Raw: "[Interface]\nPrivateKey = x\n"},
	}}
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{folder}}
	link := corestate.NodeLink{FolderID: folder.ID, Tag: "185.107.56.130"}

	// Адрес разрешается в индекс КОНТЕЙНЕРА — это норма, состав живёт в нём.
	idx := wizardbusiness.SourceIndexByLink(m, link)
	if idx != 0 {
		t.Fatalf("SourceIndexByLink = %d, ожидался 0", idx)
	}

	// Вид обязан считаться по узлу, а не по строке под этим индексом.
	node := wizardbusiness.NodeByLink(m, link)
	if node == nil {
		t.Fatal("NodeByLink не нашёл узел")
	}
	nodeView := sourceViewOf(node.Kind)
	if !nodeView.isServer {
		t.Errorf("вид узла: isServer=false (kind=%q)", node.Kind)
	}
	if nodeView.isFolder || nodeView.isContainer {
		t.Errorf("вид узла считает его контейнером: isFolder=%v isContainer=%v",
			nodeView.isFolder, nodeView.isContainer)
	}

	// Контрольная проверка: вид строки под тем же индексом — папка. Именно
	// эту подмену и допускал баг.
	rowView := sourceViewOf(m.Sources[idx].Kind)
	if !rowView.isFolder {
		t.Fatalf("строка под индексом %d перестала быть папкой", idx)
	}
}
