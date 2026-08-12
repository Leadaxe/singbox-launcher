package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/internal/locale"
)

// CommandRow — строка с командой, которую пользователь должен выполнить сам:
// поле с текстом плюс кнопки «скопировать» и «открыть в терминале».
//
// Команды с sudo лаунчер не запускает: привилегированное действие выполняет
// пользователь в своём терминале, видя полный вывод. Наше дело — отдать точный
// текст, чтобы его не пришлось набирать руками с риском опечатки.
//
// Вынесено из окна подключения (connection_local_daemon_darwin.go), чтобы
// окном добавления машины использовалась та же строка: две реализации
// копирования разъезжались бы в поведении фидбека.
// withTerminal=false убирает кнопку «открыть в терминале»: она открыла бы
// терминал на ЭТОЙ машине, тогда как команду надо выполнить на удалённой.
func CommandRow(win fyne.Window, labelKey string, command func() (string, error), withTerminal bool) fyne.CanvasObject {
	entry := widget.NewEntry()
	entry.Wrapping = fyne.TextWrapOff
	if text, err := command(); err == nil {
		entry.SetText(text)
	}

	var copyBtn *ttwidget.Button
	copyBtn = ttwidget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		text, err := command()
		if err != nil {
			ShowError(win, err)
			return
		}
		entry.SetText(text)
		win.Clipboard().SetContent(text)
		// Тихий фидбек: иконка на секунду становится галочкой. Никаких
		// системных нотификаций и модалок ради копирования строки.
		copyBtn.SetIcon(theme.ConfirmIcon())
		go func() {
			time.Sleep(1200 * time.Millisecond)
			fyne.Do(func() { copyBtn.SetIcon(theme.ContentCopyIcon()) })
		}()
	})
	copyBtn.SetToolTip(locale.T("conn.cmd_copy_tooltip"))

	buttons := container.NewHBox(copyBtn)
	// Кнопка терминала — только там, где лаунчер умеет его открыть (macOS).
	// На прочих платформах остаётся копирование: обещать открытие терминала
	// и ничего не делать хуже, чем его не предлагать.
	if withTerminal && openTerminal != nil {
		termBtn := ttwidget.NewButtonWithIcon("", theme.ComputerIcon(), func() {
			text, err := command()
			if err != nil {
				ShowError(win, err)
				return
			}
			entry.SetText(text)
			if err := openTerminal(text); err != nil {
				ShowError(win, err)
			}
		})
		termBtn.SetToolTip(locale.T("conn.cmd_terminal_tooltip"))
		buttons.Add(termBtn)
	}

	rowLabel := widget.NewLabel(locale.T(labelKey))
	rowLabel.Wrapping = fyne.TextWrapWord
	return container.NewVBox(
		rowLabel,
		container.NewBorder(nil, nil, nil, buttons, entry),
	)
}

// openTerminal — платформенный хук открытия терминала с командой.
// nil там, где лаунчер этого не умеет (см. command_row_*.go).
var openTerminal func(string) error
