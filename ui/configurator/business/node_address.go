// File node_address.go — адресация узла в модели по NodeLink.
//
// NodeLink — не «похожий на адрес» тип, а САМ адрес узла: пара «id папки +
// тег», где пустой FolderID означает корневое пространство. На нём уже живут
// detour, позиции цепочек и члены групп; заводить для окон вторую форму
// адресации («индекс в Sources» + «индекс в Nodes») значило бы держать две
// системы координат на одну модель — и разъехаться на первом же переносе узла
// между папками.
package business

import (
	"strings"

	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// NodeByLink возвращает УКАЗАТЕЛЬ на узел модели по его адресу.
//
// Указатель, а не копия: адресат нужен вызывающему для правки на месте
// (переименование, detour, тело). nil означает «такого узла больше нет» —
// законный исход: узел могли удалить между открытием окна и действием в нём.
//
// Корневой узел (FolderID == "") — это Source с встроенным Node, поэтому
// возвращается именно &Source.Node: у корневого пространства тег-политики
// нет, и сырой тег равен финальному.
func NodeByLink(model *wizardmodels.WizardModel, link corestate.NodeLink) *corestate.Node {
	if model == nil {
		return nil
	}
	tag := strings.TrimSpace(link.Tag)
	if tag == "" {
		return nil
	}
	if link.FolderID == "" {
		for i := range model.Sources {
			s := &model.Sources[i]
			if !sourceKindIsNode(s.Kind) {
				continue
			}
			if strings.TrimSpace(s.NodeTagOrLabel()) == tag {
				return &s.Node
			}
		}
		return nil
	}
	folder := findSourceByID(model, link.FolderID)
	if folder == nil {
		return nil
	}
	for i := range folder.Nodes {
		if folder.Nodes[i].Tag == tag {
			return &folder.Nodes[i]
		}
	}
	return nil
}

// SourceByLink возвращает КОНТЕЙНЕР узла (nil для корневого узла).
//
// Нужен тем, кто правит узел в составе: тег-политика, порядок и merge живут
// на контейнере, а не на узле.
func SourceByLink(model *wizardmodels.WizardModel, link corestate.NodeLink) *corestate.Source {
	if model == nil || link.FolderID == "" {
		return nil
	}
	return findSourceByID(model, link.FolderID)
}

// SourceIndexByLink — позиция контейнера (или самого корневого узла) в
// model.Sources; -1, если адрес не разрешается.
//
// Индекс здесь ВЫВОДИТСЯ из адреса, а не хранится параллельно ему: место в
// слайсе меняется от перетаскивания и удаления соседей, и запомненный индекс
// протухает молча, тогда как адрес переживает перестановку.
func SourceIndexByLink(model *wizardmodels.WizardModel, link corestate.NodeLink) int {
	if model == nil {
		return -1
	}
	if link.FolderID != "" {
		for i := range model.Sources {
			if model.Sources[i].ID == link.FolderID {
				return i
			}
		}
		return -1
	}
	tag := strings.TrimSpace(link.Tag)
	if tag == "" {
		return -1
	}
	for i := range model.Sources {
		s := &model.Sources[i]
		if !sourceKindIsNode(s.Kind) {
			continue
		}
		if strings.TrimSpace(s.NodeTagOrLabel()) == tag {
			return i
		}
	}
	return -1
}

// LinkOfRootSource — адрес корневой строки-узла.
func LinkOfRootSource(src *corestate.Source) corestate.NodeLink {
	if src == nil {
		return corestate.NodeLink{}
	}
	return corestate.NodeLink{Tag: strings.TrimSpace(src.NodeTagOrLabel())}
}

// LinkOfContainerNode — адрес узла внутри контейнера.
func LinkOfContainerNode(folder *corestate.Source, rawTag string) corestate.NodeLink {
	link := corestate.NodeLink{Tag: strings.TrimSpace(rawTag)}
	if folder != nil {
		link.FolderID = folder.ID
	}
	return link
}

// sourceKindIsNode — строка сама себе узел (состава нет).
//
// Тот же вопрос, что различает виды в окне источника; здесь он нужен, чтобы
// корневой адрес не поймал контейнер с совпавшим именем.
func sourceKindIsNode(kind corestate.SourceKind) bool {
	switch kind {
	case wizardmodels.SourceKindServer,
		wizardmodels.SourceKindChain,
		wizardmodels.SourceKindAuto,
		wizardmodels.SourceKindUnsupported:
		return true
	}
	return false
}
