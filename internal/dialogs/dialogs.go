package dialogs

import (
	"fmt"
	"time"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// downloadFailedMessage — единая фраза в диалоге при любой ошибке загрузки.
const downloadFailedMessage = "Download failed. See the log for details."

// downloadFailedManualHint — подсказка скачать вручную.
const downloadFailedManualHint = "Please download the file manually and place it in the folder below."

// NewCustom создает диалог с упрощенным API: mainContent (центр), buttons (низ), Border.
// Если dismissText не пустой, создается кнопка закрытия слева от buttons; ESC закрывает диалог.
func NewCustom(title string, mainContent fyne.CanvasObject, buttons fyne.CanvasObject, dismissText string, parent fyne.Window) dialog.Dialog {
	var d dialog.Dialog

	// Если buttons пусто, создаем пустой контейнер
	if buttons == nil {
		buttons = container.NewHBox()
	}

	// Если dismissText не пустой, создаем кнопку закрытия и размещаем её слева, buttons справа
	if dismissText != "" {
		closeButton := widget.NewButton(dismissText, func() {
			if d != nil {
				d.Hide()
			}
		})
		// Используем Border для размещения: closeButton слева, buttons справа
		buttons = container.NewBorder(nil, nil, closeButton, buttons, nil)
	}

	// Собираем Border: top=nil, bottom=buttons (с кнопкой dismissText слева, если указан), left=nil, right=nil, center=mainContent
	content := container.NewBorder(
		nil,         // top
		buttons,     // bottom (кнопка с dismissText слева, если указан)
		nil,         // left
		nil,         // right
		mainContent, // center
	)

	d = dialog.NewCustomWithoutButtons(title, content, parent)

	// Если dismissText не пустой, добавляем обработку ESC
	if dismissText != "" {
		originalOnTypedKey := parent.Canvas().OnTypedKey()
		parent.Canvas().SetOnTypedKey(func(key *fyne.KeyEvent) {
			if key.Name == fyne.KeyEscape && d != nil {
				d.Hide()
				// Восстанавливаем оригинальный обработчик
				if originalOnTypedKey != nil {
					parent.Canvas().SetOnTypedKey(originalOnTypedKey)
				} else {
					parent.Canvas().SetOnTypedKey(nil)
				}
				return
			}
			// Пробрасываем другие клавиши оригинальному обработчику
			if originalOnTypedKey != nil {
				originalOnTypedKey(key)
			}
		})

		// Восстанавливаем обработчик при закрытии диалога
		d.SetOnClosed(func() {
			if originalOnTypedKey != nil {
				parent.Canvas().SetOnTypedKey(originalOnTypedKey)
			} else {
				parent.Canvas().SetOnTypedKey(nil)
			}
		})
	}

	return d
}

// ShowDownloadFailedManual shows a unified dialog when a download fails (network or other).
// Always displays the same short message, a link to download manually, and a button to open
// the target folder. downloadURL and targetDir may be empty to hide the link or "Open folder" button.
func ShowDownloadFailedManual(window fyne.Window, title, downloadURL, targetDir string) {
	debuglog.DebugLog("dialogs: ShowDownloadFailedManual start title=%s", title)
	fyne.Do(func() {
		mainContent := container.NewVBox()
		msgLabel := widget.NewLabel(downloadFailedMessage)
		msgLabel.Wrapping = fyne.TextWrapWord
		mainContent.Add(msgLabel)
		hintLabel := widget.NewLabel(downloadFailedManualHint)
		hintLabel.Wrapping = fyne.TextWrapWord
		mainContent.Add(hintLabel)

		if downloadURL != "" {
			link := widget.NewHyperlink("Open download page", nil)
			if err := link.SetURLFromString(downloadURL); err == nil {
				link.OnTapped = func() {
					if err := platform.OpenURL(downloadURL); err != nil {
						debuglog.ErrorLog("dialogs: OpenURL failed: %v", err)
						ShowError(window, fmt.Errorf("failed to open link: %w", err))
						return
					}
				}
			}
			copyBtn := widget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
				window.Clipboard().SetContent(downloadURL)
			})
			copyBtn.Importance = widget.LowImportance
			linkRow := container.NewHBox(link, copyBtn)
			// Reserve minimum height so the link row is not overlapped by the button bar (Hyperlink can report zero height).
			linkWrap := container.NewVBox(linkRow)
			spacer := canvas.NewRectangle(color.Transparent)
			spacer.SetMinSize(fyne.NewSize(1, 24))
			linkWrap.Add(spacer)
			mainContent.Add(linkWrap)
			mainContent.Add(widget.NewLabel(""))
		}

		var buttons fyne.CanvasObject
		if targetDir != "" {
			openFolderBtn := widget.NewButton("Open folder", func() {
				if err := platform.OpenFolder(targetDir); err != nil {
					ShowError(window, fmt.Errorf("failed to open folder: %w", err))
				}
			})
			buttons = openFolderBtn
		}

		d := NewCustom(title, mainContent, buttons, "Close", window)
		d.Show()
		debuglog.DebugLog("dialogs: ShowDownloadFailedManual shown")
	})
}

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

// ShowProcessKillConfirmation shows a dialog asking user if they want to kill a running process.
// onKill is called in a goroutine when user clicks "Kill Process".
func ShowProcessKillConfirmation(window fyne.Window, onKill func()) {
	fyne.Do(func() {
		var d dialog.Dialog
		killButton := widget.NewButton("Kill Process", nil)
		closeButton := widget.NewButton("Close This Warning", nil)
		content := container.NewVBox(
			widget.NewLabel("Sing-Box appears to be already running.\nWould you like to kill the existing process?"),
			killButton,
			closeButton,
		)
		d = dialog.NewCustomWithoutButtons("Warning", content, window)
		killButton.OnTapped = func() {
			go onKill()
			d.Hide()
		}
		closeButton.OnTapped = func() { d.Hide() }
		d.Show()
	})
}

// ShowAutoHideInfo shows a temporary notification and dialog that auto-hides after 2 seconds
func ShowAutoHideInfo(app fyne.App, window fyne.Window, title, message string) {
	app.SendNotification(&fyne.Notification{Title: title, Content: message})
	fyne.Do(func() {
		d := dialog.NewCustomWithoutButtons(title, widget.NewLabel(message), window)
		d.Show()
		go func() {
			<-time.After(2 * time.Second)
			fyne.Do(func() { d.Hide() })
		}()
	})
}
