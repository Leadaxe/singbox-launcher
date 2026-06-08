package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// ShowError shows an error dialog to the user
func ShowError(window fyne.Window, err error) {
	fyne.Do(func() {
		dialog.ShowError(err, window)
	})
}

// ShowErrorText shows an error dialog with a text message
func ShowErrorText(window fyne.Window, title, message string) {
	fyne.Do(func() {
		dialog.ShowError(fmt.Errorf("%s: %s", title, message), window)
	})
}

// ShowInfo shows an information dialog to the user
func ShowInfo(window fyne.Window, title, message string) {
	fyne.Do(func() {
		dialog.ShowInformation(title, message, window)
	})
}

// ShowCustom shows a custom dialog with custom content
func ShowCustom(window fyne.Window, title, dismiss string, content fyne.CanvasObject) {
	fyne.Do(func() {
		dialog.ShowCustom(title, dismiss, content, window)
	})
}

// ShowConfirm shows a confirmation dialog
func ShowConfirm(window fyne.Window, title, message string, onConfirm func(bool)) {
	fyne.Do(func() {
		dialog.ShowConfirm(title, message, onConfirm, window)
	})
}
