package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/locale"
)

// Строка шага окна Service: «N  заголовок», поле команды, ⧉ и ▶ (SPEC 161
// §3.4, PLAN §6.3). ⧉ копирует команду как есть — без ssh-обёртки, чтобы её
// можно было вставить в уже открытую ssh-сессию; ▶ открывает Terminal на
// этом компьютере, для удалённой машины — с `ssh <цель> '<команда>'`.

const (
	serviceScmNoRunText     = "Run it in an elevated PowerShell on the machine."
	serviceInteractiveText  = "Runs in the foreground: stop it with Ctrl-C, then start the service again."
	serviceDefaultPathText  = "default path — the daemon has not reported it yet"
	serviceConfirmRunText   = "This changes the service on %s. Run it?"
	servicePlaceholderText  = "Replace the <…> part before running it."
	serviceRunOverSSHTip    = "Run in Terminal over ssh"
	serviceCommandMaxRows   = 6
	serviceCommandRowLength = 64
)

// serviceRunner решает, как запускать шаги в этом окне.
type serviceRunner struct {
	win fyne.Window
	// remote — команды выполняются на машине по ssh (ssh — её цель).
	remote bool
	ssh    services.SSHTarget
	init   core.ServiceInit
	// title — имя машины для подтверждения опасных шагов.
	title string
}

// terminalCommand — что ▶ отдаёт Terminal; ok=false — ▶ не предлагать, note —
// почему (пусто — без подписи).
func (r *serviceRunner) terminalCommand(step core.ServiceStep) (cmd string, ok bool, note string) {
	if step.Placeholder {
		return "", false, locale.T(servicePlaceholderText)
	}
	if r.remote && !step.RunsLocally && !r.init.Posix() {
		// PowerShell через ssh в cmd.exe-шелл машины не переносится.
		return "", false, locale.T(serviceScmNoRunText)
	}
	if openTerminal == nil {
		return "", false, ""
	}
	if r.remote && !step.RunsLocally {
		return core.WrapSSH(r.ssh, step.Command, strings.Contains(step.Command, "sudo")), true, ""
	}
	return step.Command, true, ""
}

// serviceStepRow — строка шага. title "" — без заголовка; hint — подпись под
// полем («если шапка позеленела — дальше не надо»); danger — ▶ спрашивает
// подтверждение.
func serviceStepRow(r *serviceRunner, title, hint string, step core.ServiceStep, danger bool) fyne.CanvasObject {
	box := container.NewVBox()
	if title != "" {
		box.Add(serviceStepTitle(title))
	}

	entry := widget.NewMultiLineEntry()
	entry.Wrapping = fyne.TextWrapWord
	entry.SetText(step.Command)
	rows := len(step.Command)/serviceCommandRowLength + 1
	if rows > serviceCommandMaxRows {
		rows = serviceCommandMaxRows
	}
	entry.SetMinRowsVisible(rows)

	// ⧉ — текст поля: у шага с плейсхолдером (<name>) его правят руками.
	copyBtn := NewCopyButton("Copy the command", func() (string, bool) {
		return entry.Text, entry.Text != ""
	})
	buttons := container.NewHBox(copyBtn)

	cmd, canRun, note := r.terminalCommand(step)
	if canRun {
		tip := locale.T("Run in Terminal")
		if r.remote && !step.RunsLocally {
			tip = locale.T(serviceRunOverSSHTip)
		}
		runBtn := ttwidget.NewButtonWithIcon("", theme.ComputerIcon(), func() {
			run := func() {
				if err := openTerminal(cmd); err != nil {
					ShowError(r.win, err)
				}
			}
			if !danger {
				run()
				return
			}
			ShowConfirm(r.win, locale.T("Run the command"), locale.Tf(serviceConfirmRunText, r.title), func(ok bool) {
				if ok {
					run()
				}
			})
		})
		runBtn.SetToolTip(tip)
		buttons.Add(runBtn)
	}
	box.Add(container.NewBorder(nil, nil, nil, container.NewVBox(buttons), entry))

	var notes []string
	if hint != "" {
		notes = append(notes, hint)
	}
	if step.Interactive {
		notes = append(notes, locale.T(serviceInteractiveText))
	}
	if note != "" {
		notes = append(notes, note)
	}
	for _, n := range notes {
		box.Add(serviceNoteLabel(n, widget.MediumImportance))
	}
	if step.UsesDefault {
		box.Add(serviceNoteLabel(locale.T(serviceDefaultPathText), widget.LowImportance))
	}
	return box
}

// serviceStepTitle — жирный заголовок шага с переносом.
func serviceStepTitle(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.Wrapping = fyne.TextWrapWord
	return l
}

// serviceNoteLabel — строка пояснения с переносом (без Wrapping длинный
// текст раздувал бы минимальную ширину окна).
func serviceNoteLabel(text string, importance widget.Importance) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	l.Importance = importance
	return l
}
