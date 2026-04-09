package dialogs

import (
	"fmt"
	"strings"
	"time"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

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
		msgLabel := widget.NewLabel(locale.T("dialog.download_failed"))
		msgLabel.Wrapping = fyne.TextWrapWord
		mainContent.Add(msgLabel)
		hintLabel := widget.NewLabel(locale.T("dialog.download_failed_manual_hint"))
		hintLabel.Wrapping = fyne.TextWrapWord
		mainContent.Add(hintLabel)

		if downloadURL != "" {
			link := widget.NewHyperlink(locale.T("dialog.open_download_page"), nil)
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
				fyne.CurrentApp().Clipboard().SetContent(downloadURL)
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
			openFolderBtn := widget.NewButton(locale.T("dialog.open_folder"), func() {
				if err := platform.OpenFolder(targetDir); err != nil {
					ShowError(window, fmt.Errorf("failed to open folder: %w", err))
				}
			})
			buttons = openFolderBtn
		}

		d := NewCustom(title, mainContent, buttons, locale.T("dialog.close"), window)
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

// ShowLinuxCapabilitiesRequired shows a dialog for the Linux capabilities message
// with the setcap command in a selectable entry and a Copy button (issue #34).
// title is the dialog title (e.g. "Error" or "Linux Capabilities"); message is the full
// text (warning + explanation); command is the single line to copy (e.g. sudo setcap ...).
func ShowLinuxCapabilitiesRequired(window fyne.Window, title, message, command string) {
	fyne.Do(func() {
		mainContent := container.NewVBox()

		// Use selectable multiline text so users can copy the full error details.
		msgEntry := widget.NewMultiLineEntry()
		msgEntry.SetText(message)
		msgEntry.Disable()
		msgEntry.Wrapping = fyne.TextWrapWord
		msgEntry.SetMinRowsVisible(10)
		msgScroll := container.NewScroll(msgEntry)
		msgScroll.SetMinSize(fyne.NewSize(520, 220))
		mainContent.Add(msgScroll)

		// Selectable command line and Copy button
		entry := widget.NewEntry()
		entry.SetText(command)
		entry.Disable()
		entry.Wrapping = fyne.TextWrapOff
		entry.SetMinRowsVisible(1)
		copyBtn := widget.NewButtonWithIcon(locale.T("dialog.copy"), theme.ContentCopyIcon(), func() {
			fullText := message
			if command != "" && fullText != "" && !strings.Contains(fullText, command) {
				fullText += "\n\n" + command
			} else if fullText == "" {
				fullText = command
			}
			if fullText != "" {
				fyne.CurrentApp().Clipboard().SetContent(fullText)
			}
		})
		copyBtn.Importance = widget.LowImportance
		cmdRow := container.NewVBox(
			entry,
			container.NewHBox(layout.NewSpacer(), copyBtn),
		)
		mainContent.Add(cmdRow)
		// Reserve extra vertical space so the bottom dialog bar never overlaps the command row.
		bottomSpacer := canvas.NewRectangle(color.Transparent)
		bottomSpacer.SetMinSize(fyne.NewSize(1, 8))
		mainContent.Add(bottomSpacer)

		d := dialog.NewCustom(title, locale.T("dialog.ok"), mainContent, window)
		d.Show()
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
		killButton := widget.NewButton(locale.T("dialog.kill_process"), nil)
		closeButton := widget.NewButton(locale.T("dialog.close_warning"), nil)
		content := container.NewVBox(
			widget.NewLabel(locale.T("dialog.process_already_running")),
			killButton,
			closeButton,
		)
		d = dialog.NewCustomWithoutButtons(locale.T("dialog.warning"), content, window)
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
