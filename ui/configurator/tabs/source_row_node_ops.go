// File source_row_node_ops.go — правый клик по строке ВЕРХНЕГО узла в списке
// Sources: полное меню узла (SPEC 116 W13, обкатка; заход 2 — пункт 5).
//
// # Почему точка входа отдельная, а механика — общая
//
// Механика переноса готова с W2 (business.MoveNodeToFolder /
// CopyNodeToFolder) и с W5 подключена к строке узла КОНТЕЙНЕРА (вкладка
// Preview окна источника, preview_node_ops.go). У верхнего узла (server /
// chain / auto) контейнера нет — он сам себе Source, и вкладки Preview с его
// строкой не существует: единственная его строка живёт здесь, в списке
// Sources. Поэтому не хватало ровно UI-точки, а не операции.
//
// # Принцип «меню = кнопки» (заход 2)
//
// Меню строки и кнопки справа у неё — ОДИН набор действий и ОДНА реализация.
// Заход 1 показывал только Move/Copy, и получалось, что правый клик знает про
// узел меньше, чем ряд иконок в той же строке. Теперь пункты ведут туда же,
// куда кнопки:
//
//   - «Node info…» / «Rename…» → `showSourceEditWindow` — окно источника, то
//     же, что открывает карандаш. Оно и есть форма правки верхнего узла: тег
//     правится полем `nodeTagEntry`, а на Save отрабатывает
//     `resetRefsAfterNodeRename` (гашение ссылок на прежнюю идентичность) и
//     `staleSelectionAfterEdit`. Своего диалога переименования здесь заводить
//     нельзя: у формы есть сброс ссылок на Save, у второго пути его не было бы.
//   - «Delete» → `showSourceRowDeleteDialog` — тот же путь, что у корзины
//     строки, вместе с веткой непустой папки (сценарий С7).
//   - «Copy JSON» / «Copy tag» — то же, что у одноимённых пунктов строки узла
//     контейнера: тело, которое уедет в конфиг, и имя, которым узел зовут
//     правила.
//   - «Copy to folder…» / «Move to folder…» → `previewNodeOps`
//     (showMoveOrCopyDialog / applyMoveOrCopy) — тот же диалог, тот же
//     showStaleSelectionDialog и тот же showDetourRefsResetDialog, что в
//     Preview. Второй набор диалогов разъехался бы с первым текстами и
//     проверками, а операция под ними одна.
//
// Контекст операций для строки списка отличается от оконного ровно двумя
// вещами: владелец диалогов — главное окно визарда (окна источника здесь нет),
// а reloadScratch/refreshPreview пусты — рабочей копии, которую надо было бы
// перечитать после немедленной мутации, у списка нет вовсе; список целиком
// перестраивает applySourceMutation.
package tabs

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// sourceRowNodeOpsAllowed — у этого вида источника есть ОДИН собственный узел,
// который можно перенести в папку.
//
// Только server / chain / auto. Контейнеры (папка, подписка) исключены по
// разным причинам, и обе не про запрет операции, а про её адрес: у них не один
// узел, а состав, и переносится из них конкретный узел — строкой состава
// (drill-down списка или вкладка Preview, W5), а не строкой контейнера.
// Неразобранная запись (unsupported) в корне не живёт вовсе
// (normalizeSourceShape её отвергает).
func sourceRowNodeOpsAllowed(kind corestate.SourceKind) bool {
	switch kind {
	case corestate.SourceKindServer, corestate.SourceKindChain, corestate.SourceKindAuto:
		return true
	default:
		return false
	}
}

// showSourceRowNodeContextMenu — меню строки верхнего узла по правому клику.
//
// rawTag — адрес узла в корневом пространстве. У корня тег-политики нет,
// поэтому финальный тег там равен сырому (см. containerRefOf в node_move.go), и
// отдельной идентичности, как у узла папки, выводить не из чего.
//
// shortLabel — как строка зовёт источник: тем же именем его назовёт диалог
// удаления, чтобы подтверждение говорило про то, на что человек смотрит.
func showSourceRowNodeContextMenu(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	sourceIndex int,
	kind corestate.SourceKind,
	rawTag string,
	shortLabel string,
	pe *fyne.PointEvent,
) {
	if presenter == nil || guiState == nil || guiState.Window == nil || pe == nil {
		return
	}
	if !sourceRowNodeOpsAllowed(kind) || strings.TrimSpace(rawTag) == "" {
		return
	}

	ops := &previewNodeOps{
		presenter:   presenter,
		guiState:    guiState,
		win:         guiState.Window,
		sourceIndex: sourceIndex,
		kind:        kind,
		// reloadScratch/refreshPreview — nil: окна источника здесь нет, а
		// список перестраивает applySourceMutation внутри applyMoveOrCopy.
	}

	openEdit := func() {
		presenter.MergeGUIToModel()
		m := presenter.Model()
		if m == nil || sourceIndex < 0 || sourceIndex >= len(m.Sources) {
			return
		}
		showSourceEditWindow(presenter, guiState, guiState.Window, sourceIndex, shortLabel)
	}

	items := []*fyne.MenuItem{
		// Первым — то же, что делает карандаш строки: посмотреть и поправить.
		fyne.NewMenuItem(locale.T("Node info…"), openEdit),
		fyne.NewMenuItem(locale.T("Copy JSON"), func() {
			fynewidget.SetClipboard(sourceRowNodeJSON(presenter, sourceIndex, rawTag))
		}),
		fyne.NewMenuItem(locale.T("Copy tag"), func() {
			fynewidget.SetClipboard(rawTag)
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(locale.T("Copy to folder…"), func() {
			ops.showMoveOrCopyDialog(rawTag, false)
		}),
		fyne.NewMenuItem(locale.T("Move to folder…"), func() {
			ops.showMoveOrCopyDialog(rawTag, true)
		}),
		// Переименование верхнего узла живёт полем формы: только там на Save
		// отрабатывает сброс ссылок на прежнюю идентичность.
		fyne.NewMenuItem(locale.T("Rename…"), openEdit),
		fyne.NewMenuItem(locale.T("Delete"), func() {
			showSourceRowDeleteDialog(presenter, guiState, sourceIndex, sourceID(presenter, sourceIndex),
				shortLabel, 0)
		}),
	}
	widget.ShowPopUpMenuAtPosition(
		fyne.NewMenu("", items...), guiState.Window.Canvas(), pe.AbsolutePosition)
}

// sourceRowNodeJSON — тело верхнего узла как оно уедет в конфиг.
//
// Эмиссия ТА ЖЕ, что у списка и у сборки (`config.EmitCanonicalSource` со
// СВОИМ пустым `tagCounts`) — второй сборки JSON не заводим: у пункта «Copy
// JSON» на вкладке Preview он берётся ровно оттуда же
// (`previewNodeJSON(node)`). Пусто = узел не собрался; отдавать в буфер
// «{}» честнее, чем молчать.
func sourceRowNodeJSON(
	presenter *wizardpresentation.WizardPresenter,
	sourceIndex int,
	rawTag string,
) string {
	m := presenter.Model()
	if m == nil || sourceIndex < 0 || sourceIndex >= len(m.Sources) {
		return "{}"
	}
	emitted := config.EmitCanonicalSource(
		m.Sources[sourceIndex].ToProxySourceV4(), sourceIndex, map[string]int{})
	for _, n := range emitted.Nodes {
		if n == nil {
			continue
		}
		if config.NodeIdentity(n) == rawTag || n.Tag == rawTag {
			return previewNodeJSON(n)
		}
	}
	// Узел один — если идентичность не совпала (тег-машина его переименовала),
	// берём единственный выпущенный: промахнуться тут не в кого.
	if len(emitted.Nodes) == 1 {
		return previewNodeJSON(emitted.Nodes[0])
	}
	return "{}"
}

// sourceID — ULID источника по его позиции; пусто, если позиции больше нет.
func sourceID(presenter *wizardpresentation.WizardPresenter, sourceIndex int) string {
	m := presenter.Model()
	if m == nil || sourceIndex < 0 || sourceIndex >= len(m.Sources) {
		return ""
	}
	return m.Sources[sourceIndex].ID
}
