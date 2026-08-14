package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
)

// copyFeedbackDelay — сколько держится галочка после копирования.
const copyFeedbackDelay = 1200 * time.Millisecond

// setClipboard — короткое имя для fynewidget.SetClipboard: копирование зовут
// из десятка мест пакета, и полное имя там только шумит.
func setClipboard(text string) { fynewidget.SetClipboard(text) }

// NewCopyButton — кнопка «скопировать» с тихим фидбеком: иконка на секунду
// становится галочкой и возвращается обратно.
//
// text вычисляется в момент нажатия, а не при сборке: команды и превью
// пересчитываются от состояния формы, и захват строки заранее копировал бы
// устаревшее значение. ok=false — копирования не было (текст не удалось
// собрать), и галочку не показываем: она означала бы «в буфере то, что нужно».
//
// Единая реализация на все места копирования: раньше их было три, и фидбек в
// них уже начал разъезжаться.
func NewCopyButton(tooltipKey string, text func() (string, bool)) *ttwidget.Button {
	var btn *ttwidget.Button
	btn = ttwidget.NewButtonWithIcon("", theme.ContentCopyIcon(), func() {
		s, ok := text()
		if !ok {
			return
		}
		setClipboard(s)
		btn.SetIcon(theme.ConfirmIcon())
		go func() {
			time.Sleep(copyFeedbackDelay)
			fyne.Do(func() { btn.SetIcon(theme.ContentCopyIcon()) })
		}()
	})
	if tooltipKey != "" {
		btn.SetToolTip(locale.T(tooltipKey))
	}
	return btn
}

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

	// Пересчёт команды на нажатии: ошибку показываем и не копируем — прежний
	// текст в буфере читался бы как «скопировалась актуальная команда».
	copyBtn := NewCopyButton("conn.cmd_copy_tooltip", func() (string, bool) {
		text, err := command()
		if err != nil {
			ShowError(win, err)
			return "", false
		}
		entry.SetText(text)
		return text, true
	})

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
