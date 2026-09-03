package tabs

// Окно «Отчёт импорта» — что применилось и что НЕ смогло приехать.
//
// Импорт правит состояние, и его потери — не мелочь на полях:
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

	// Заголовок списка потерь экспорта. У импорта потери — «применено не
	// полностью» (запись приехала, но иначе); у экспорта — «в файл не
	// попало»: разные события, и одним словом их называть нельзя.
	settingsBackupExportLostTitleText = "Did not make it into the file:"
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
	openBackupReportWindow(
		locale.T("Import report"),
		importReportHead(res),
		locale.T("Not applied as-is:"),
		importReportText(res, warns),
		warns)
}

// openBackupReportWindow — окно списка потерь, общее для импорта и экспорта.
//
// Экспорт получил его в SPEC 116 W9 (§O1=А): раньше он показывал потери
// модалкой с обрезкой на 10 строках, и хвост «… +N» отсылал в ОТЧЁТ ИМПОРТА,
// которого при экспорте не бывает. Пользователь, создавший папку, видел
// «Копия сохранена» и обрубок списка — то самое молчание, которое запрещает
// критерий A9.
func openBackupReportWindow(title, headText, warnTitleText, copyText string, warns []backup.Warning) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	w := app.NewWindow(title)

	head := widget.NewLabelWithStyle(
		headText, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	head.Wrapping = fyne.TextWrapWord

	warnTitle := widget.NewLabel(warnTitleText)
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
		fynewidget.SetClipboard(copyText)
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

// showExportReport показывает итог экспорта (SPEC 116 W9, критерий A9).
//
// Без потерь — короткий диалог «сохранено, храните надёжно», как было.
// С потерями — то же окно, что у импорта: полный НЕОБРЕЗАННЫЙ список того,
// чего в файле нет. Обрезка здесь была прямым нарушением A9 — папка,
// оказавшаяся одиннадцатой, выпадала под «… +N».
func showExportReport(win fyne.Window, path string, warns []backup.Warning) {
	head := fmt.Sprintf(locale.T(settingsBackupExportDoneText), path)
	if len(warns) == 0 {
		dialog.ShowInformation(locale.T("Backup saved"), head, win)
		return
	}
	openBackupReportWindow(
		locale.T("Export report"),
		head,
		locale.T(settingsBackupExportLostTitleText),
		exportReportText(path, warns),
		warns)
}

// exportReportText — весь отчёт экспорта одним текстом для буфера обмена.
func exportReportText(path string, warns []backup.Warning) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, locale.T(settingsBackupExportDoneText), path)
	if len(warns) > 0 {
		sb.WriteString("\n\n")
		sb.WriteString(locale.T(settingsBackupExportLostTitleText))
		for _, w := range warns {
			sb.WriteString("\n• ")
			sb.WriteString(warnText(w))
		}
	}
	return sb.String()
}

// importReportHead — шапка отчёта: что применилось.
//
// После перехода на слияние (D-095) одного числа «применено» мало: «добавлено
// 3» и «обновлено 3» — разные новости, а «пропущено» вообще не потеря, а «у
// тебя уже есть». Поэтому строки раскладки идут отдельно и ТОЛЬКО когда есть
// что сказать: в обычном импорте на чистую машину обновлять и пропускать
// нечего, и нули в отчёте были бы шумом.
func importReportHead(res *backup.ImportResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, locale.T("Applied: %d source(s), %d rule(s)."),
		res.AppliedSources, res.AppliedRules)

	added := res.AddedSubscriptions + res.AddedServers + res.AddedFolders + res.AddedChains
	if added > 0 {
		sb.WriteString("\n")
		fmt.Fprintf(&sb, locale.T("Added: %d"), added)
	}
	if res.UpdatedSubscriptions > 0 {
		sb.WriteString("\n")
		fmt.Fprintf(&sb, locale.T("Updated subscriptions: %d"), res.UpdatedSubscriptions)
	}
	if res.SkippedServers > 0 {
		sb.WriteString("\n")
		fmt.Fprintf(&sb, locale.T("Already here, skipped: %d server(s)"), res.SkippedServers)
	}
	return sb.String()
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
