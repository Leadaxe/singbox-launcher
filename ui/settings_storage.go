package ui

import (
	"image/color"
	"path/filepath"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
)

// buildStorageSection — раздел Storage вкладки Settings (SPEC 135 §4.1): где
// лежат программа, данные, логи, какое ядро и какой шаблон выбраны.
//
// Таблица — FormLayout: колонка ключей выравнивается по самому длинному,
// значения тянутся. Пути длинные, поэтому каждое значение — Label с
// TextWrapBreak: Label без Wrapping держит минимальную ширину по всей строке
// и раздувает окно. Значения Selectable — путь можно выделить и скопировать
// по частям; целиком блок копирует Copy paths.
//
// Второе значение — refresh: перечитывает блок (ядро меняется после
// скачивания, версия ядра становится известна после первой проверки).
// Зовётся при выборе вкладки Settings, фоновых опросов нет.
func buildStorageSection(ac *core.AppController) (fyne.CanvasObject, func()) {
	title := widget.NewLabelWithStyle(locale.T("Storage"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	hint := widget.NewLabel(locale.T("Where the launcher keeps its files. Attach “Copy paths” to bug reports."))
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance

	value := func() *widget.Label {
		l := widget.NewLabel("")
		l.Wrapping = fyne.TextWrapBreak
		l.Selectable = true
		return l
	}
	modeVal, appVal, dataVal, logsVal := value(), value(), value(), value()
	coreVal, coreMeta, tmplVal, tmplMeta := value(), value(), value(), value()
	coreMeta.Importance = widget.LowImportance
	tmplMeta.Importance = widget.LowImportance

	// Текущий блок — открывающие кнопки читают пути отсюда, а не из
	// замыкания на момент постройки.
	var info paths.PathsInfo

	openBtn := func(dir func() string) fyne.CanvasObject {
		btn := ttwidget.NewButtonWithIcon("", theme.FolderOpenIcon(), func() {
			d := dir()
			if d == "" {
				return
			}
			if err := platform.OpenFolder(d); err != nil {
				debuglog.ErrorLog("settings.storage: open folder %s: %v", d, err)
				ShowError(ac.UIService.MainWindow, err)
			}
		})
		btn.SetToolTip(locale.T("Open folder"))
		// VBox держит кнопку в её высоте у верхнего края строки: Border
		// растянул бы её на все строки перенесённого пути.
		return container.NewVBox(btn)
	}
	cell := func(v fyne.CanvasObject, btn fyne.CanvasObject) fyne.CanvasObject {
		return container.NewBorder(nil, nil, nil, btn, v)
	}
	key := func(text string) fyne.CanvasObject { return widget.NewLabel(text) }

	rows := []fyne.CanvasObject{
		key(locale.T("Mode")), modeVal,
		key(locale.T("Program")), cell(appVal, openBtn(func() string { return string(info.Layout.App) })),
		key(locale.T("Data")), cell(dataVal, openBtn(func() string { return string(info.Layout.Data) })),
		key(locale.T("Logs")), cell(logsVal, openBtn(func() string { return string(info.Layout.Logs) })),
		key(locale.T("Core")), cell(container.NewVBox(coreVal, coreMeta), openBtn(func() string {
			if info.CorePath == "" {
				return ""
			}
			return filepath.Dir(info.CorePath)
		})),
		key(locale.T("Template")), container.NewVBox(tmplVal, tmplMeta),
	}
	var wintunVal *widget.Label
	if runtime.GOOS == "windows" {
		wintunVal = value()
		rows = append(rows, widget.NewLabel("wintun"), wintunVal) // l10n-exempt: product name
	}
	table := container.New(layout.NewFormLayout(), rows...)

	orUnknown := func(s string) string {
		if s == "" {
			return locale.T("unknown")
		}
		return s
	}
	render := func() {
		modeVal.SetText(storageModeText(info.Layout))
		appVal.SetText(string(info.Layout.App))
		dataVal.SetText(string(info.Layout.Data))
		logsVal.SetText(string(info.Layout.Logs))

		coreVal.SetText(info.CorePath)
		meta := locale.Tf("Version: %s, source: %s", orUnknown(info.CoreVersion), orUnknown(info.CoreSource))
		if info.CoreSource == "" {
			meta = locale.T("Not found: the core will be downloaded to this path")
		}
		if info.ShadowedCore != "" {
			meta += "\n" + locale.Tf("Shadows: %s", info.ShadowedCore)
		}
		coreMeta.SetText(meta)

		tmplVal.SetText(info.TemplatePath)
		if info.TemplateSource == "" {
			tmplMeta.SetText(locale.T("Not found: the template will be downloaded to this path"))
		} else {
			tmplMeta.SetText(locale.Tf("Source: %s", info.TemplateSource))
		}

		if wintunVal != nil {
			state := locale.T("not found")
			if info.WintunFound {
				state = locale.T("found")
			}
			wintunVal.SetText(info.WintunPath + " (" + state + ")")
		}
	}

	// Остаток прошлого переезда (storage_leftover): что переключатель
	// Portable не смог стереть. Строка видна, пока путь существует.
	leftoverVal := widget.NewLabel("")
	leftoverVal.Wrapping = fyne.TextWrapBreak
	leftoverVal.Importance = widget.WarningImportance
	leftoverVal.Hide()
	renderLeftover := func() {
		p := ac.StorageLeftover()
		if p == "" {
			leftoverVal.Hide()
			return
		}
		leftoverVal.SetText(locale.Tf("Left over from the move: %s", p))
		leftoverVal.Show()
	}

	// Место этапов 8–9 SPEC 135 под таблицей путей: чекбокс Portable (§4.2)
	// и кнопка «Remove all data…» (§4.3).
	extra := container.NewVBox(leftoverVal)
	refreshPortable := func() {}
	if core.PortableToggleAvailable() {
		var portable fyne.CanvasObject
		portable, refreshPortable = buildPortableToggle(ac)
		extra.Add(portable)
	}
	extra.Add(buildPurgeButton(ac))

	// Версия ядра — только из сессионного кэша контроллера. Если её ещё
	// никто не спрашивал, спрашиваем один раз в фоне: результат кэшируется,
	// и следующие вызовы бинарь уже не запускают.
	refresh := func() {
		refreshPortable()
		renderLeftover()
		info = ac.PathsInfo()
		render()
		if info.CoreVersion != "" || info.CoreSource == "" {
			return
		}
		go func() {
			if _, err := ac.GetInstalledCoreVersion(); err != nil {
				return
			}
			fyne.Do(func() {
				info = ac.PathsInfo()
				render()
			})
		}()
	}
	refresh()

	copyBtn := widget.NewButtonWithIcon(locale.T("Copy paths"), theme.ContentCopyIcon(), func() {
		ac.UIService.Application.Clipboard().SetContent(ac.PathsInfo().Text())
		dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow,
			locale.T("Paths copied"), locale.T("Data, logs, core and template paths copied. Paste them into the bug report."))
	})

	return container.NewVBox(
		title,
		hint,
		table,
		container.NewHBox(copyBtn),
		extra,
	), refresh
}

// storageModeText — строка Mode: режим раскладки и, для Env, какие
// переменные окружения сработали.
func storageModeText(l paths.Layout) string {
	switch l.Mode {
	case paths.ModePortable:
		return locale.T("Portable")
	case paths.ModeLegacy:
		return locale.T("Legacy")
	case paths.ModeSystem:
		return locale.T("System")
	case paths.ModeEnv:
		s := locale.T("Environment")
		if len(l.EnvSource) > 0 {
			s += " (" + strings.Join(l.EnvSource, ", ") + ")"
		}
		return s
	}
	return string(l.Mode)
}

// buildPortableToggle — чекбокс Portable раздела Storage (SPEC 135 §4.2) и
// серая строка под ним: куда уедут данные или почему переключать нельзя.
//
// Клик не меняет режим сам: чекбокс сразу возвращается в прежнее состояние,
// а переезд идёт только после подтверждения и заканчивается перезапуском —
// раскладка считается один раз при старте. Второе значение — refresh,
// зовётся вместе с остальными строками Storage при выборе вкладки.
func buildPortableToggle(ac *core.AppController) (fyne.CanvasObject, func()) {
	check := ttwidget.NewCheck(locale.T("Portable mode (keep data next to the program)"), nil)
	note := widget.NewLabel("")
	note.Wrapping = fyne.TextWrapBreak
	note.Importance = widget.LowImportance

	var onChanged func(bool)
	setChecked := func(v bool) {
		check.OnChanged = nil
		check.SetChecked(v)
		check.OnChanged = onChanged
	}

	refresh := func() {
		checked, enabled, reason := ac.PortableToggleState()
		setChecked(checked)
		if !enabled {
			check.Disable()
			check.SetToolTip(reason)
			note.SetText(reason)
			return
		}
		check.Enable()
		check.SetToolTip("")
		to, err := ac.PortableSwitchTarget(!checked)
		if err != nil {
			debuglog.WarnLog("settings.storage: portable target: %v", err)
			note.SetText(err.Error())
			return
		}
		note.SetText(locale.Tf("Switching moves the data to: %s", to))
	}

	onChanged = func(on bool) {
		setChecked(!on)
		confirmPortableSwitch(ac, on, refresh)
	}
	check.OnChanged = onChanged
	refresh()

	return container.NewVBox(check, note), refresh
}

// confirmPortableSwitch — подтверждение «откуда → куда» и переезд в фоне под
// модальным прогрессом (он же не даёт нажать Start в окне; трей и Debug API
// останавливает флаг переезда в контроллере). Перезапуск делает сам
// SwitchPortable сразу после переезда.
func confirmPortableSwitch(ac *core.AppController, on bool, refresh func()) {
	win := ac.UIService.MainWindow
	from := ac.FileService.Layout.Data.Bin()
	to, err := ac.PortableSwitchTarget(on)
	if err != nil {
		ShowError(win, err)
		return
	}
	confirm := dialog.NewConfirm(
		locale.T("Move launcher data"),
		locale.Tf("Data will be moved from:\n%s\nto:\n%s", from, to)+
			"\n\n"+locale.T("The launcher will restart immediately after the move."),
		func(yes bool) {
			if !yes {
				return
			}
			progress := showPortableSwitchProgress(win)
			go func() {
				// Успех перезапускает лаунчер изнутри SwitchPortable (прогресс
				// остаётся до выхода); итог и остаток — в логе и в Storage
				// после перезапуска. Сюда возвращается только ошибка.
				if _, err := ac.SwitchPortable(on); err != nil {
					fyne.Do(func() {
						progress.Hide()
						dialog.ShowError(err, win)
						refresh()
					})
				}
			}()
		}, win)
	confirm.SetConfirmText(locale.T("Yes"))
	confirm.SetDismissText(locale.T("Cancel"))
	confirm.Show()
}

// showPortableSwitchProgress — модальный прогресс на время копирования.
func showPortableSwitchProgress(win fyne.Window) dialog.Dialog {
	label := widget.NewLabel(locale.T("Moving data…"))
	// Wrapping: Label без переноса раздувает диалог по всей строке;
	// ширину задаёт распорка.
	label.Wrapping = fyne.TextWrapWord
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(280, 1))
	d := dialog.NewCustomWithoutButtons(locale.T("Move launcher data"),
		container.NewVBox(label, widget.NewProgressBarInfinite(), spacer), win)
	d.Show()
	return d
}
