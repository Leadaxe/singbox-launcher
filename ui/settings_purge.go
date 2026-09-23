package ui

import (
	"runtime"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
)

// buildPurgeButton — кнопка «Remove all data…» раздела Storage (SPEC 135
// §4.3). При запущенном ядре кнопка активна, но диалог очистки не
// открывает: просит сначала остановить VPN. План считается в фоне — обход
// DataDir с кэшами и .srs может занять заметное время.
func buildPurgeButton(ac *core.AppController) fyne.CanvasObject {
	var btn *widget.Button
	btn = widget.NewButtonWithIcon(locale.T("Remove all data…"), theme.DeleteIcon(), func() {
		win := ac.UIService.MainWindow
		if ac.RunningState != nil && ac.RunningState.IsRunning() {
			dialog.ShowInformation(locale.T("Remove all launcher data"), locale.T("Stop the VPN first"), win)
			return
		}
		btn.Disable()
		go func() {
			plan := ac.PurgePlan()
			hint := ac.DaemonUninstallHint()
			fyne.Do(func() {
				btn.Enable()
				showPurgeDialog(ac, plan, hint)
			})
		}()
	})
	btn.Importance = widget.DangerImportance
	return container.NewHBox(btn)
}

// showPurgeDialog — подтверждение очистки: по строке на элемент плана с
// чекбоксом, путём и размером; Data снять нельзя. На Windows — отдельный
// чекбокс сетевой очистки. Если установлена служба демона, над кнопками —
// команда её удаления: после удаления данных лаунчер её собрать не сможет.
//
// Подписи — Label с переносом рядом с пустым чекбоксом: текст Check не
// переносится и длинным путём раздул бы диалог.
func showPurgeDialog(ac *core.AppController, plan paths.PurgePlan, daemonHint string) {
	win := ac.UIService.MainWindow
	if len(plan.Items) == 0 {
		dialog.ShowInformation(locale.T("Remove all launcher data"), locale.T("Nothing to remove."), win)
		return
	}

	intro := widget.NewLabel(locale.T("The following will be deleted. The program folder itself is not touched — delete it yourself afterwards."))
	intro.Wrapping = fyne.TextWrapWord

	list := container.NewVBox()
	checks := make([]*widget.Check, len(plan.Items))
	for i, it := range plan.Items {
		check := widget.NewCheck("", nil)
		check.SetChecked(true)
		if it.Kind == paths.PurgeData {
			check.Disable()
		}
		checks[i] = check

		text := widget.NewLabel(purgeKindText(it.Kind) + ": " + it.Path + " (" +
			locale.Tf("%d files, %s", it.Files, paths.FormatBytes(it.Bytes)) + ")")
		text.Wrapping = fyne.TextWrapBreak
		lines := container.NewVBox(text)
		if it.Note != "" {
			note := widget.NewLabel(purgeNoteText(it.Note))
			note.Wrapping = fyne.TextWrapWord
			note.Importance = widget.LowImportance
			lines.Add(note)
		}
		list.Add(container.NewBorder(nil, nil, container.NewVBox(check), nil, lines))
	}

	var networkCheck *widget.Check
	if runtime.GOOS == "windows" {
		networkCheck = widget.NewCheck("", nil)
		networkCheck.SetChecked(true)
		label := widget.NewLabel(locale.T("Network cleanup: ghost wintun adapters and orphan firewall rules"))
		label.Wrapping = fyne.TextWrapWord
		list.Add(widget.NewSeparator())
		list.Add(container.NewBorder(nil, nil, container.NewVBox(networkCheck), nil, label))
	}

	scroll := container.NewVScroll(list)
	scroll.SetMinSize(fyne.NewSize(520, 240))

	var bottom fyne.CanvasObject
	if daemonHint != "" {
		row := CommandRow(win, "The daemon service is installed. Remove it first with this command, otherwise the launcher will not be able to do it after the data is gone:", func() (string, error) {
			return daemonHint, nil
		}, true)
		bottom = container.NewVBox(widget.NewSeparator(),
			container.NewBorder(nil, nil, container.NewVBox(widget.NewIcon(theme.WarningIcon())), nil, row))
	}
	content := container.NewBorder(intro, bottom, nil, nil, scroll)

	confirm := dialog.NewCustomConfirm(locale.T("Remove all launcher data"), locale.T("Delete"), locale.T("Cancel"), content,
		func(yes bool) {
			if !yes {
				return
			}
			for i, check := range checks {
				plan.Items[i].Selected = check.Checked
			}
			network := networkCheck != nil && networkCheck.Checked
			debuglog.WarnLog("settings.storage: removing launcher data and exiting:\n%s", plan.Text())
			go func() {
				// Успех завершает процесс внутри (os.Exit); ошибка — только до
				// начала удаления.
				if err := ac.ExecutePurgeAndExit(plan, network); err != nil {
					ShowError(win, err)
				}
			}()
		}, win)
	confirm.Resize(fyne.NewSize(600, 460))
	confirm.Show()
}

// purgeKindText — подпись раздела плана.
func purgeKindText(k paths.PurgeKind) string {
	switch k {
	case paths.PurgeData:
		return locale.T("Data")
	case paths.PurgeLogs:
		return locale.T("Logs")
	case paths.PurgeLeftover:
		return locale.T("Unused data")
	}
	return string(k)
}

// purgeNoteText — локализованное пояснение к лишнему (PurgeItem.Note).
func purgeNoteText(note string) string {
	switch note {
	case paths.PurgeNoteMovedAway:
		return locale.T("Left over from a data move")
	case paths.PurgeNoteUnusedSystem:
		return locale.T("Unused system data folder")
	case paths.PurgeNoteOldLogs:
		return locale.T("Old logs next to the program")
	case paths.PurgeNotePreMigration:
		return locale.T("Data from before the migration, inside the app bundle")
	case paths.PurgeNotePreMigrationApp:
		return locale.T("Data from before the migration, next to the program")
	}
	return note
}
