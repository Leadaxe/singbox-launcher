// File preview_row_view.go — как строка списка Preview выглядит и что
// показывает по клику (SPEC 116 W11).
//
// Строк два вида (preview_rows.go): за одной стоит собравшийся узел, за
// другой — неразобранная запись тела. Тексты и обработчики развилку знают
// ЗДЕСЬ, одним местом; список их только вызывает.
package tabs

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/textnorm"
)

// previewUnsupportedMark — знак неразобранной записи в подстроке строки.
//
// «⚠» U+26A0 — тот же знак, которым отмечены недоступные цели detour и
// потери бэкапа: новых глифов волна не заводит.
const previewUnsupportedMark = "⚠"

// previewRowTitle — имя строки.
//
// У собравшегося узла это его ФИНАЛЬНЫЙ тег (то, чем узел зовётся в
// конфиге), у неразобранной записи — её сырой тег: финального у неё нет и
// быть не может, тег-машину она не проходит вовсе.
func previewRowTitle(r previewRow) string {
	if r.Unsupported {
		return textnorm.NormalizeProxyDisplay(r.RawTag)
	}
	if r.Node == nil {
		return textnorm.NormalizeProxyDisplay(r.RawTag)
	}
	return nodeDisplayLine(r.Node)
}

// previewRowSubtitle — вторая строка: «протокол·транспорт·security» у узла,
// «⚠ причина» у неразобранной записи.
//
// Причина занимает место протокола не случайно: подстрока отвечает на вопрос
// «что это такое», и у записи, которую не разобрали, честный ответ —
// «вот почему её нет».
func previewRowSubtitle(r previewRow) string {
	if r.Unsupported {
		reason := r.Reason
		if reason == "" {
			reason = locale.T("could not be parsed")
		}
		return previewUnsupportedMark + " " + reason
	}
	if r.Node == nil {
		// Узел в составе есть, эмиссия его не выпустила: он выключен.
		return locale.T("off")
	}
	return previewNodeSubtitle(r.Node)
}

// previewRowToolTip — полный текст под курсором.
//
// У неразобранной записи это причина целиком плюс её исходник: в подстроке
// причина обрезается шириной окна, а исходник не показывается вовсе, и без
// тултипа строка «⚠ vless outbound rejected: empty…» была бы тупиком.
// У собравшегося узла тултипа нет: его подстрока помещается целиком.
func previewRowToolTip(r previewRow) string {
	if !r.Unsupported {
		return ""
	}
	tip := r.Reason
	if tip == "" {
		tip = locale.T("could not be parsed")
	}
	if r.OriginRaw != "" {
		tip += "\n" + r.OriginRaw
	}
	return tip
}

// previewAnnounceBlock — сообщение провайдера над списком; nil, когда
// провайдер молчал.
//
// Чужой текст, показанный КАК ДАННЫЕ (то же правило, что в Overview):
// провайдер объясняет, почему состав такой, — интерпретировать его за него мы
// не беремся. Wrapping обязателен: Label без него отдаёт всю строку как
// min-width и раздувает окно на весь экран (fyne-ловушка).
func previewAnnounceBlock(msg string) fyne.CanvasObject {
	if msg == "" {
		return nil
	}
	lbl := widget.NewLabel(locale.Tf("provider says: %s", msg))
	lbl.Wrapping = fyne.TextWrapWord
	lbl.Importance = widget.WarningImportance
	return lbl
}

// showPreviewRowInfoWindow — клик по строке: как узел зовут и из чего он
// сделан (SPEC 116 W11).
//
// Два поля, и оба нужны обоим видам строк: имя — то, под которым узел
// известен пользователю, исходник (`origin.raw`) — то, из чего он собран
// (или из чего собраться не смог). Полное окно разбора («Node info…»)
// осталось в контекстном меню: у неразобранной записи разбирать нечего, а у
// узла оно длинное и по клику всплывать не должно.
func showPreviewRowInfoWindow(r previewRow) {
	origin := r.OriginRaw
	if origin == "" && !r.Unsupported {
		// Узел, собранный руками с нуля, исходника не имеет — показывать
		// пустое окно незачем, у него есть полноценное «Node info…».
		showPreviewNodeInfoWindow(r.Node)
		return
	}

	win := fyne.CurrentApp().NewWindow(locale.Tf("Node: %s", previewRowTitle(r)))

	body := container.NewVBox(
		previewInfoRow(locale.T("Tag"), previewRowTitle(r)),
	)
	if r.Unsupported {
		reason := r.Reason
		if reason == "" {
			reason = locale.T("could not be parsed")
		}
		body.Add(previewInfoRow(locale.T("Reason"), reason))
	}
	body.Add(widget.NewSeparator())
	body.Add(previewSectionHeader(locale.T("Origin")))

	// MultiLineEntry, а не Label: исходник длинный, его выделяют и копируют.
	// Ввод откатывается — тот же приём, что у read-only полей Overview
	// (Disable() на macOS красит текст цветом фона).
	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapBreak
	entry.SetText(origin)
	entry.OnChanged = func(s string) {
		if s != origin {
			entry.SetText(origin)
		}
	}
	body.Add(entry)
	body.Add(widget.NewButton(locale.T("Copy"), func() {
		fynewidget.SetClipboard(origin)
	}))

	win.SetContent(previewWithScrollGutter(body))
	win.Resize(fyne.NewSize(560, 360))
	win.CenterOnScreen()
	win.Show()
}

// showPreviewRowContextMenu — меню строки правым кликом.
//
// У неразобранной записи из всего меню осмысленны два пункта: посмотреть её
// исходник и скопировать его (починить строку можно только вставив её
// исправленной — руками, в папку). Пункты про JSON, тег и операции над узлом
// ей не подходят: узла за ней нет.
func showPreviewRowContextMenu(
	win fyne.Window,
	r previewRow,
	rawTag string,
	ops *previewNodeOps,
	pe *fyne.PointEvent,
) {
	if win == nil || pe == nil {
		return
	}
	if !r.Unsupported {
		// Узел может быть nil — выключенный узел эмиссию не проходит, но
		// строкой остаётся, и операции над ним (перенести, переименовать,
		// удалить) обязаны работать: он в составе, просто не в конфиге.
		showPreviewNodeContextMenu(win, r.Node, rawTag, ops, pe)
		return
	}
	origin := r.OriginRaw
	items := []*fyne.MenuItem{
		fyne.NewMenuItem(locale.T("Node info…"), func() {
			showPreviewRowInfoWindow(r)
		}),
		fyne.NewMenuItem(locale.T("Copy source line"), func() {
			fynewidget.SetClipboard(origin)
		}),
	}
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), win.Canvas(), pe.AbsolutePosition)
}
