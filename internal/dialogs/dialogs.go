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
	ShowDownloadFailedManualWithReason(window, title, "", downloadURL, targetDir)
}

// maxReasonLen caps the failure reason shown in the dialog. Wrapped labels grow
// the dialog vertically without bound, and a wall of Go error text helps nobody
// — the full error always goes to the log.
const maxReasonLen = 200

// ShowDownloadFailedManualWithReason is ShowDownloadFailedManual plus a concrete,
// human-readable cause ("GitHub rate limit reached — wait or switch node")
// instead of the generic "see the log". Pass an empty reason for the generic text.
func ShowDownloadFailedManualWithReason(window fyne.Window, title, reason, downloadURL, targetDir string) {
	debuglog.DebugLog("dialogs: ShowDownloadFailedManual start title=%s reason=%q", title, reason)
	fyne.Do(func() {
		mainContent := container.NewVBox()
		message := locale.T("Download failed. See the log for details.")
		if reason != "" {
			message = reason
			// Count runes, not bytes: slicing Cyrillic text by byte offset
			// splits a character in half and renders as a replacement glyph.
			if r := []rune(message); len(r) > maxReasonLen {
				message = string(r[:maxReasonLen]) + "…"
			}
		}
		msgLabel := widget.NewLabel(message)
		msgLabel.Wrapping = fyne.TextWrapWord
		mainContent.Add(msgLabel)
		hintLabel := widget.NewLabel(locale.T("Please download the file manually and place it in the folder below."))
		hintLabel.Wrapping = fyne.TextWrapWord
		mainContent.Add(hintLabel)

		if downloadURL != "" {
			link := widget.NewHyperlink(locale.T("Open download page"), nil)
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
			openFolderBtn := widget.NewButton(locale.T("Open folder"), func() {
				if err := platform.OpenFolder(targetDir); err != nil {
					ShowError(window, fmt.Errorf("failed to open folder: %w", err))
				}
			})
			buttons = openFolderBtn
		}

		d := NewCustom(title, mainContent, buttons, locale.T("Close"), window)
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
// Copy puts only the command on the clipboard: it is pasted into a terminal,
// where the explanation would be run as junk input. An empty command shows the
// message alone (no command row).
func ShowLinuxCapabilitiesRequired(window fyne.Window, title, message, command string) {
	fyne.Do(func() {
		mainContent := container.NewVBox()

		// Обычный Label, а не Disable()'нутый Entry: отключённый Entry в Fyne
		// рисуется цветом DisabledColor — тем же, которым рисуется
		// placeholder, — и объяснение выглядит как незаполненная подсказка,
		// а не как текст, который надо прочесть. Кнопка Copy ниже кладёт в
		// буфер только команду: её вставляют в терминал, и пояснение там
		// стало бы «паразитным текстом».
		msgLabel := widget.NewLabel(message)
		msgLabel.Wrapping = fyne.TextWrapWord

		mainContent.Add(messageScroll(msgLabel, message))

		// Selectable command line and Copy button
		// Команда остаётся Entry — её выделяют и копируют мышью, — но НЕ
		// Disable()'нутым: отключённый Entry рисуется цветом placeholder'а, и
		// команда для терминала выглядит нечитаемой подсказкой. Правки гасятся
		// откатом текста: поле остаётся только для чтения, оставаясь читаемым.
		entry := widget.NewEntry()
		entry.SetText(command)
		entry.Wrapping = fyne.TextWrapOff
		entry.SetMinRowsVisible(1)
		entry.OnChanged = func(s string) {
			if s != command {
				entry.SetText(command)
			}
		}
		copyBtn := widget.NewButtonWithIcon(locale.T("Copy"), theme.ContentCopyIcon(), func() {
			if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
				app.Clipboard().SetContent(command)
			}
		})
		copyBtn.Importance = widget.LowImportance
		cmdRow := container.NewVBox(
			entry,
			container.NewHBox(layout.NewSpacer(), copyBtn),
		)
		if command != "" {
			mainContent.Add(cmdRow)
		}
		// Reserve extra vertical space so the bottom dialog bar never overlaps the command row.
		bottomSpacer := canvas.NewRectangle(color.Transparent)
		bottomSpacer.SetMinSize(fyne.NewSize(1, 8))
		mainContent.Add(bottomSpacer)

		d := dialog.NewCustom(title, locale.T("OK"), mainContent, window)
		d.Show()
	})
}

// messageWidth — ширина текста в диалогах «текст + команда».
const messageWidth = 520

// messageScroll — прокрутка под пояснение диалога с высотой по содержимому
// и потолком. Прежние SetMinRowsVisible(10) и MinSize 520×220 резервировали
// десять строк под сообщение из двух, и половину диалога занимала пустота.
// Высота оценивается по длине текста, а не по Label.MinSize(): до
// размещения в контейнере тот не знает ширину и считает перенос по словам
// как одну строку, занижая высоту в разы на длинном тексте.
func messageScroll(label *widget.Label, message string) *container.Scroll {
	lines := 1 + len([]rune(message))/72 // ~72 символа в строке при 520px
	if n := strings.Count(message, "\n"); n > 0 {
		lines += n
	}
	msgH := float32(lines)*theme.TextSize()*1.5 + 16
	if msgH < 56 {
		msgH = 56
	}
	if msgH > 260 {
		msgH = 260 // длинное сообщение скроллится
	}
	scroll := container.NewVScroll(label)
	scroll.SetMinSize(fyne.NewSize(messageWidth, msgH))
	return scroll
}

// commandCopyFeedback — сколько держится галочка на кнопке копирования.
const commandCopyFeedback = 1200 * time.Millisecond

// ShowCommandRetry — диалог шага, который пользователь выполняет сам в
// терминале (sudo), прежде чем действие можно повторить (SPEC 137):
// пояснение, команда в поле только для чтения, кнопки «Copy the command» и
// «Run in Terminal», внизу Close и Retry. Retry закрывает диалог и зовёт
// onRetry. openTerminal == nil прячет кнопку терминала, onRetry == nil —
// Retry, command == "" — поле и кнопки команды (остаётся пояснение). Сам
// диалог ничего привилегированного не запускает.
func ShowCommandRetry(window fyne.Window, title, message, command string, openTerminal func(string) error, onRetry func()) {
	fyne.Do(func() {
		msgLabel := widget.NewLabel(message)
		msgLabel.Wrapping = fyne.TextWrapWord

		// Поле остаётся читаемым Entry (не Disable — тот рисуется цветом
		// placeholder'а), правки гасятся откатом текста.
		entry := widget.NewEntry()
		entry.SetText(command)
		entry.Wrapping = fyne.TextWrapOff
		entry.OnChanged = func(s string) {
			if s != command {
				entry.SetText(command)
			}
		}

		var copyBtn *widget.Button
		copyBtn = widget.NewButtonWithIcon(locale.T("Copy the command"), theme.ContentCopyIcon(), func() {
			if app := fyne.CurrentApp(); app != nil && app.Clipboard() != nil {
				app.Clipboard().SetContent(command)
			}
			copyBtn.SetIcon(theme.ConfirmIcon())
			go func() {
				time.Sleep(commandCopyFeedback)
				fyne.Do(func() { copyBtn.SetIcon(theme.ContentCopyIcon()) })
			}()
		})
		actions := container.NewHBox(layout.NewSpacer(), copyBtn)
		if openTerminal != nil {
			termBtn := widget.NewButtonWithIcon(locale.T("Run in Terminal"), theme.ComputerIcon(), func() {
				if err := openTerminal(command); err != nil {
					debuglog.WarnLog("dialogs: open Terminal: %v", err)
					dialog.ShowError(err, window)
				}
			})
			actions.Add(termBtn)
		}

		var d dialog.Dialog
		var buttons fyne.CanvasObject
		if onRetry != nil {
			retryBtn := widget.NewButton(locale.T("Retry"), func() {
				d.Hide()
				onRetry()
			})
			retryBtn.Importance = widget.HighImportance
			buttons = container.NewHBox(retryBtn)
		}
		content := container.NewVBox(messageScroll(msgLabel, message))
		if command != "" {
			content.Add(entry)
			content.Add(actions)
		}
		d = NewCustom(title, content, buttons, locale.T("Close"), window)
		d.Show()
		debuglog.DebugLog("dialogs: ShowCommandRetry %q shown", title)
	})
}

// Action — кнопка диалога ShowActions.
type Action struct {
	Label     string
	Important bool // HighImportance: основное действие
	// Disabled — кнопка недоступна; Hint — почему: отдельная серая строка
	// под пояснением, законченной фразой (без префикса с именем кнопки).
	Disabled bool
	Hint     string
	// Run — нажатие, в UI-потоке. Диалог сам не закрывается: действие
	// зовёт Hide или пишет строку статуса (SetStatus).
	Run func(d *ActionsDialog)
}

// ActionsDialog — открытый диалог ShowActions.
type ActionsDialog struct {
	d       dialog.Dialog
	status  *widget.Label
	buttons []*widget.Button
	actions []Action
}

// Hide закрывает диалог. Из любой горутины.
func (a *ActionsDialog) Hide() {
	fyne.Do(func() { a.d.Hide() })
}

// SetStatus пишет строку статуса над кнопками ("" — прячет её) и снова
// включает кнопки после SetBusy. Из любой горутины.
func (a *ActionsDialog) SetStatus(text string) {
	fyne.Do(func() {
		a.status.SetText(text)
		if text == "" {
			a.status.Hide()
		} else {
			a.status.Show()
		}
		a.setEnabled(true)
	})
}

// SetBusy выключает кнопки действий, пока идёт долгое действие (запрос UAC),
// чтобы его не запустили второй раз. Из UI-потока.
func (a *ActionsDialog) SetBusy() {
	a.setEnabled(false)
}

func (a *ActionsDialog) setEnabled(on bool) {
	for i, b := range a.buttons {
		if on && !a.actions[i].Disabled {
			b.Enable()
		} else {
			b.Disable()
		}
	}
}

// ShowActions — диалог с пояснением и набором действий (SPEC 139 §4:
// «TUN без прав»; SPEC 141 добавит в тот же список Install service):
// пояснение, подсказки недоступных действий, строка статуса (отказ в UAC,
// ошибка) и кнопки — dismissText слева, действия справа в порядке списка.
func ShowActions(window fyne.Window, title, message string, actions []Action, dismissText string) {
	fyne.Do(func() {
		msgLabel := widget.NewLabel(message)
		msgLabel.Wrapping = fyne.TextWrapWord
		content := container.NewVBox(messageScroll(msgLabel, message))
		for _, act := range actions {
			if act.Disabled && act.Hint != "" {
				hint := widget.NewLabel(act.Hint)
				hint.Wrapping = fyne.TextWrapWord
				hint.Importance = widget.LowImportance
				content.Add(hint)
			}
		}
		status := widget.NewLabel("")
		status.Wrapping = fyne.TextWrapWord
		status.Importance = widget.WarningImportance
		status.Hide()
		content.Add(status)

		a := &ActionsDialog{status: status, actions: actions}
		row := container.NewHBox()
		for _, act := range actions {
			act := act
			btn := widget.NewButton(act.Label, func() {
				if act.Run != nil {
					act.Run(a)
				}
			})
			if act.Important {
				btn.Importance = widget.HighImportance
			}
			if act.Disabled {
				btn.Disable()
			}
			a.buttons = append(a.buttons, btn)
			row.Add(btn)
		}
		a.d = NewCustom(title, content, row, dismissText, window)
		a.d.Show()
		debuglog.DebugLog("dialogs: ShowActions %q shown", title)
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
		killButton := widget.NewButton(locale.T("Kill Process"), nil)
		closeButton := widget.NewButton(locale.T("Close This Warning"), nil)
		content := container.NewVBox(
			widget.NewLabel(locale.T("Sing-Box appears to be already running.\nWould you like to kill the existing process?")),
			killButton,
			closeButton,
		)
		d = dialog.NewCustomWithoutButtons(locale.T("Warning"), content, window)
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
