package tabs

// Окно «Отчёт импорта» — что применилось и что НЕ смогло приехать.
//
// Импорт заменяет состояние целиком, и его потери — не мелочь на полях:
// выключенное правило или пропущенная переменная всплывут через неделю, когда
// связать их с импортом уже некому. Раньше отчёт показывался через
// dialog.ShowInformation, а список потерь резался на 12 строках («… +N») —
// то есть ровно длинный перечень, ради которого отчёт и нужен, пользователь
// не видел.
//
// Application.NewWindow, а не модальный попап: диалог живёт внутри канвы
// родителя и не может быть выше её, а высокий попап со списком Label'ов в
// Fyne раздувает размер окна ([[fyne-label-minwidth-trap]]) — та же причина,
// по которой своё окно есть у собранного конфига (showConfigWindow) и у
// журнала обмена с машиной.

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/backup"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	settingsBackupReportMoreText = "The full list is in the import report shown after the import."
)

// showImportReport показывает итог импорта.
//
// Без потерь — короткий диалог: успех не заслуживает отдельного окна и лишнего
// клика по «Закрыть». С потерями — окно с полным, НЕ обрезанным списком.
func showImportReport(win fyne.Window, res *backup.ImportResult, warns []backup.Warning) {
	if len(warns) == 0 {
		dialog.ShowInformation(
			locale.T("Backup imported"),
			importReportHead(res),
			win)
		return
	}
	openImportReportWindow(res, warns)
}

// openImportReportWindow — собственно окно отчёта.
func openImportReportWindow(res *backup.ImportResult, warns []backup.Warning) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	w := app.NewWindow(locale.T("Import report"))

	head := widget.NewLabelWithStyle(
		importReportHead(res), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	head.Wrapping = fyne.TextWrapWord

	warnTitle := widget.NewLabel(locale.T("Not applied as-is:"))
	warnTitle.Wrapping = fyne.TextWrapWord

	// Каждая потеря — своей строкой, без лимита. Wrapping обязателен: Label без
	// переноса задаёт окну min-width по самой длинной строке, а Detail здесь —
	// теги и адреса произвольной длины ([[fyne-label-minwidth-trap]]).
	rows := container.NewVBox()
	for _, warn := range warns {
		line := widget.NewLabel("• " + warnText(warn))
		line.Wrapping = fyne.TextWrapWord
		rows.Add(line)
	}

	copyBtn := widget.NewButton(locale.T("Copy"), func() {
		fynewidget.SetClipboard(importReportText(res, warns))
	})
	closeBtn := widget.NewButton(locale.T("Close"), func() { w.Close() })
	closeBtn.Importance = widget.HighImportance

	w.SetContent(container.NewPadded(container.NewBorder(
		container.NewVBox(head, warnTitle, widget.NewSeparator()),
		container.NewBorder(nil, nil, copyBtn, container.NewHBox(layout.NewSpacer(), closeBtn)),
		nil, nil,
		container.NewVScroll(rows),
	)))
	w.Resize(fyne.NewSize(560, 420))
	w.CenterOnScreen()
	w.Show()
}

// importReportHead — шапка отчёта: что применилось.
func importReportHead(res *backup.ImportResult) string {
	return fmt.Sprintf(locale.T("Applied: %d source(s), %d rule(s)."),
		res.AppliedSources, res.AppliedRules)
}

// importReportText — весь отчёт одним текстом, без обрезки: то, что уезжает в
// буфер по кнопке «Копировать», должно совпадать с тем, что видно на экране.
func importReportText(res *backup.ImportResult, warns []backup.Warning) string {
	var sb strings.Builder
	sb.WriteString(importReportHead(res))
	if len(warns) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(locale.T("Not applied as-is:"))
		for _, w := range warns {
			sb.WriteString("\n• ")
			sb.WriteString(warnText(w))
		}
	}
	return sb.String()
}
