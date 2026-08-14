package fynewidget

import "fyne.io/fyne/v2"

// SetClipboard кладёт текст в буфер обмена приложения.
//
// Идём через App, а не через Window: у окна Clipboard() устарел (Fyne 2.6+),
// а буфер и так один на приложение — привязка к конкретному окну ничего не
// давала. Молча ничего не делает без приложения: копирование — вспомогательное
// действие, ронять из-за него вызывающего незачем.
func SetClipboard(text string) {
	app := fyne.CurrentApp()
	if app == nil {
		return
	}
	if cb := app.Clipboard(); cb != nil {
		cb.SetContent(text)
	}
}
