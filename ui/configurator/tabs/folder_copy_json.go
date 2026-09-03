// File folder_copy_json.go — «Copy nodes as JSON» в окне ПАПКИ (SPEC 116
// этап 3, W8; цель 3, сценарий С6, критерий A6).
//
// # Что уезжает в буфер
//
// Документ {"outbounds":[...],"endpoints":[...]} — ровно те записи, которые
// папка добавит в config.json: та же точка эмиссии, что у сборки
// (config.EmitNodeJSONs через unpackNodesDoc), второй эмиссии не заведено.
// Такой документ `sing-box check` принимает как фрагмент конфига (A6).
//
// # Почему теги ФИНАЛЬНЫЕ (§O2, вариант А)
//
// Узлы берутся из config.EmitCanonicalSource — то есть уже прошли тег-машину
// папки: префикс/постфикс политики, {$num}, уникализация. Документ поэтому
// самодостаточен: его можно вставить в чужой конфиг, и он совпадёт с тем, что
// лаунчер собирает сам. Сырые теги (вариант Б) дали бы документ,
// рассинхронизированный с реальным конфигом, и коллизии при вставке.
//
// # Почему кнопка на вкладке JSON, а не в «Add nodes…»
//
// «Add nodes…» — наполнение папки, здесь обратное направление: выгрузка. А на
// вкладке JSON лежит ровно тот текст, который уедет в буфер, и её подсказка
// уже объясняет, что это за outbound'ы. Разница с показом одна: показ
// обрезается лимитом previewNodeCap (MultiLineEntry вешает окно на
// полутысяче outbound'ов), а выгрузка — НЕТ, иначе «взять всю папку» отдало
// бы не всю папку.
package tabs

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// folderCopyNodesJSON собирает документ по ЖИВОЙ записи папки и кладёт его в
// буфер обмена.
//
// Модель перечитывается здесь, а не берётся снимком окна: состав папки правят
// операции над узлом и наполнение, и оба мутируют модель немедленно (см.
// шапку preview_node_ops.go). Снимок отдал бы состав на момент открытия окна.
func folderCopyNodesJSON(presenter *wizardpresentation.WizardPresenter, sourceIndex int, win fyne.Window) {
	if presenter == nil || win == nil {
		return
	}
	m := presenter.Model()
	if m == nil || sourceIndex < 0 || sourceIndex >= len(m.Sources) {
		return
	}
	src := m.Sources[sourceIndex]
	// Вид перечитывается вместе с записью: кнопка родилась у папки, но между
	// её постройкой и кликом список источников мог поехать, и индекс окна
	// указал бы на чужую запись.
	if src.Kind != corestate.SourceKindFolder {
		return
	}
	if len(src.Nodes) == 0 {
		dialogs.ShowAutoHideInfo(fyne.CurrentApp(), win,
			locale.T("Nothing to copy"),
			locale.T("This folder has no nodes yet."))
		return
	}

	// tagCounts свой на выгрузку: документ уникализируется САМ В СЕБЕ, а не
	// в контексте всего конфига — иначе первый же тег приехал бы с «-2»,
	// доставшимся от соседнего источника, которого в документе нет.
	emitted := config.EmitCanonicalSource(src.ToProxySourceV4(), sourceIndex, map[string]int{})
	res := unpackNodesDoc(emitted.Nodes, 0)
	if res.Err != nil {
		dialogs.ShowAutoHideInfo(fyne.CurrentApp(), win,
			locale.T("Nothing to copy"), res.Err.Error())
		return
	}
	if res.Emitted == 0 {
		// Ни одного эмитируемого узла: в буфер уехал бы пустой документ, и
		// человек вставил бы пустоту, думая что забрал папку. Причин ровно
		// две, и они разные: узлы выключены (воля пользователя — их и в
		// config.json нет) либо не собрались (поломка). Одна формулировка на
		// оба случая отправила бы человека чинить то, что он сам выключил.
		reason := locale.T("None of the folder nodes could be built into an outbound.")
		if len(emitted.Nodes) == 0 && len(emitted.Warnings) == 0 {
			reason = locale.T("All nodes of this folder are disabled — they do not reach the config either.")
		}
		dialogs.ShowAutoHideInfo(fyne.CurrentApp(), win, locale.T("Nothing to copy"), reason)
		return
	}

	fynewidget.SetClipboard(res.Text)

	msg := locale.Tf("%d node(s) copied as JSON.", res.Emitted)
	if res.Dropped > 0 {
		// Отброшенное называется вслух: сборка молчит про такие узлы (пишет
		// warn в лог), но здесь пользователь уносит документ с собой и должен
		// знать, что унёс не всё.
		msg += " " + locale.Tf("%d node(s) could not be built and were left out.", res.Dropped)
	}
	dialogs.ShowAutoHideInfo(fyne.CurrentApp(), win, locale.T("Copied"), msg)
}

// folderCopyJSONButton — кнопка выгрузки для вкладки JSON окна папки.
// nil у любого другого вида источника: у подписки состав принадлежит
// провайдеру и забирается её собственным URL, у узла есть «Copy JSON» в его
// меню.
func folderCopyJSONButton(isFolder bool, presenter *wizardpresentation.WizardPresenter, sourceIndex int, win fyne.Window) *widget.Button {
	if !isFolder {
		return nil
	}
	btn := widget.NewButton(locale.T("Copy nodes as JSON"), func() {
		folderCopyNodesJSON(presenter, sourceIndex, win)
	})
	btn.Importance = widget.MediumImportance
	return btn
}
