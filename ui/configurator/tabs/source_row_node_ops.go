// File source_row_node_ops.go — правый клик по строке ВЕРХНЕГО узла в списке
// Sources: «Move to folder…» / «Copy to folder…» (SPEC 116 W13, обкатка).
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
// Общее берётся целиком, своего не заводится:
//
//   - мутация модели и реестр переписи ссылок → business.MoveNodeToFolder /
//     CopyNodeToFolder (они же перечисляют непереносимые ссылки корня —
//     rootOnlyRefsToTag: цели правил, route.final, detour DNS, addOutbounds);
//   - выбор целевой папки, побочки, предупреждения → previewNodeOps
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

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/locale"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// sourceRowNodeOpsAllowed — у этого вида источника есть ОДИН собственный узел,
// который можно перенести в папку.
//
// Только server / chain / auto. Контейнеры (папка, подписка) исключены по
// разным причинам, и обе не про запрет операции, а про её адрес: у них не один
// узел, а состав, и переносится из них конкретный узел — строкой Preview
// (W5), а не строкой контейнера. Неразобранная запись (unsupported) в корне не
// живёт вовсе (normalizeSourceShape её отвергает).
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
func showSourceRowNodeContextMenu(
	presenter *wizardpresentation.WizardPresenter,
	guiState *wizardpresentation.GUIState,
	sourceIndex int,
	kind corestate.SourceKind,
	rawTag string,
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

	items := []*fyne.MenuItem{
		fyne.NewMenuItem(locale.T("Move to folder…"), func() {
			ops.showMoveOrCopyDialog(rawTag, true)
		}),
		fyne.NewMenuItem(locale.T("Copy to folder…"), func() {
			ops.showMoveOrCopyDialog(rawTag, false)
		}),
	}
	widget.ShowPopUpMenuAtPosition(
		fyne.NewMenu("", items...), guiState.Window.Canvas(), pe.AbsolutePosition)
}
