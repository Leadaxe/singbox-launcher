// File node_rule_view_window.go — просмотр правила, которым владеет узел.
//
// Правила из секций узла (SPEC 121 §10.4) в списке Rules не правятся: их
// источник — тело узла, и вторая точка правки разошлась бы с первой. Но
// «не правится» не обязано значить «не видно»: строка списка несёт только
// имя и тег, а чем правило ловит трафик — не видно нигде, кроме вкладки JSON
// окна источника, куда за этим надо идти отдельно.
//
// Поэтому клик по строке открывает тело на просмотр — тот же приём, что у
// собранного конфига (final_tab.showConfigWindow): своё окно, моноширинный
// текст, выделение и копирование есть, ввод откатывается.
package tabs

import (
	"encoding/json"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
)

// showNodeRuleViewWindow показывает тело правила узла.
//
// rec == nil — узел или позиция уехали (узел отредактировали, правил стало
// меньше): молча ничего не открываем, ошибки здесь нет.
func showNodeRuleViewWindow(rec *corestate.Rule, title, nodeTag string) {
	if rec == nil {
		return
	}
	app := fyne.CurrentApp()
	if app == nil {
		return
	}

	// Тело печатается с отступами — его читают, а не машина. Ошибку разбора
	// показываем как текст: окно просмотра не то место, где стоит молчать о
	// том, что запись нечитаема.
	text := ""
	if body, err := rec.BodyMap(); err == nil {
		if pretty, perr := json.MarshalIndent(body, "", "  "); perr == nil {
			text = string(pretty)
		} else {
			text = perr.Error()
		}
	} else {
		text = err.Error()
	}

	w := app.NewWindow(title)

	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapOff
	entry.SetText(text)
	// Откат вместо молчаливого игнора: пустой хэндлер оставлял бы
	// напечатанное на экране, и правка выглядела бы принятой (тот же приём,
	// что у jsonEntry в source_edit_window.go).
	entry.OnChanged = func(s string) {
		if s != text {
			entry.SetText(text)
		}
	}

	note := widget.NewLabel(locale.Tf("This rule belongs to node %q — edit it in that source's JSON tab.", nodeTag))
	note.Wrapping = fyne.TextWrapWord

	closeBtn := widget.NewButton(locale.T("Cancel"), func() { w.Close() })
	w.SetContent(container.NewBorder(
		note,
		container.NewHBox(layout.NewSpacer(), closeBtn),
		nil, nil,
		container.NewVScroll(entry),
	))
	w.Resize(fyne.NewSize(620, 480))
	fynewidget.CenterOnScreen(w)
	w.Show()
}
