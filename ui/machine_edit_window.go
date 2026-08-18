package ui

import (
	"fmt"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/ui/components"
)

// Окно правки удалённой машины (SPEC 098 §2.1).
//
// Отдельное окно, а не модальный диалог: кроме четырёх полей паспорта здесь
// живут два действия, у каждого своё поле ввода и своя строка статуса, — а
// высокий модальный попап в Fyne раздувается на весь экран (см. память
// проекта: Application.NewWindow для высоких форм).
//
// Три вещи в одном окне не случайно: все они про ОДНУ запись реестра и
// отвечают на три вопроса, которые возникают подряд.
//
//	Паспорт   — «как эту машину зовут и куда стучаться».
//	Re-pair   — «канал сломался; как починить, не потеряв настройки».
//	Copy from — «настройки уже собраны на соседней машине; как не делать
//	             это второй раз руками».

// OpenEditMachineWindow открывает окно правки машины.
// onChanged зовётся после любой применённой правки (перечитать список).
func OpenEditMachineWindow(ac *core.AppController, registry *services.RemoteRegistry,
	d services.RemoteDaemon, onChanged func()) {
	if ac == nil || ac.UIService == nil || ac.UIService.Application == nil {
		return
	}
	win := ac.UIService.Application.NewWindow(
		locale.Tf("remote.edit.window_title", d.Name))

	reload := func() {
		if onChanged != nil {
			onChanged()
		}
	}

	body := container.NewVBox(
		machineEditPassport(win, registry, d, reload),
		widget.NewSeparator(),
		machineEditRePair(win, registry, d, reload),
		widget.NewSeparator(),
		machineEditCopyProfile(ac, win, registry, d, reload),
	)
	win.SetContent(container.NewPadded(components.WrapInScrollWithGutter(body)))
	win.Resize(fyne.NewSize(560, 640))
	win.CenterOnScreen()
	win.Show()
}

// machineEditPassport — имя, адрес и платформа записи.
//
// Save правит ТОЛЬКО реестр: ключи и пин лежат отдельно и переживают
// переименование машины и смену её адреса — ровно поэтому смена адреса не
// требует пере-сопряжения (Update их не трогает).
func machineEditPassport(win fyne.Window, registry *services.RemoteRegistry,
	d services.RemoteDaemon, reload func()) fyne.CanvasObject {
	nameEntry := widget.NewEntry()
	nameEntry.SetText(d.Name)
	addrEntry := widget.NewEntry()
	addrEntry.SetText(d.Addr)

	tgt := d.Target()
	goosSelect := widget.NewSelect([]string{"linux", "darwin", "windows"}, nil)
	goosSelect.SetSelected(tgt.GOOS)
	goarchSelect := widget.NewSelect([]string{"amd64", "arm64", "arm", "386", "mips", "mipsle"}, nil)
	goarchSelect.SetSelected(tgt.GOARCH)

	form := widget.NewForm(
		widget.NewFormItem(locale.T("remote.machines.field_name"), nameEntry),
		widget.NewFormItem(locale.T("remote.machines.field_addr"), addrEntry),
		widget.NewFormItem(locale.T("remote.machines.field_platform"), goosSelect),
		widget.NewFormItem(locale.T("remote.machines.field_arch"), goarchSelect),
	)

	saveBtn := widget.NewButton(locale.T("dialog.button_save"), func() {
		if err := registry.Update(d.ID, nameEntry.Text, addrEntry.Text); err != nil {
			dialog.ShowError(err, win)
			return
		}
		if err := registry.SetPlatform(d.ID, goosSelect.Selected, goarchSelect.Selected); err != nil {
			dialog.ShowError(err, win)
			return
		}
		reload()
		win.Close()
	})
	saveBtn.Importance = widget.HighImportance
	cancelBtn := widget.NewButton(locale.T("dialog.button_cancel"), func() { win.Close() })

	return container.NewVBox(
		form,
		container.NewBorder(nil, nil, nil, container.NewHBox(cancelBtn, saveBtn)),
	)
}

// machineEditRePair — повторное сопряжение существующей записи.
//
// Нужен, когда канал сломался не по нашей вине: демона переустановили, его
// state-каталог вычистили, клиента отозвали или у демона сменился серверный
// сертификат и пин перестал сходиться. Раньше это лечилось «удалить и завести
// заново», а Remove сносит папку машины со ВСЕМИ настройками — платить
// настройками за починку канала незачем.
//
// Свёрнут по умолчанию: в штатной жизни машины сюда не заходят.
func machineEditRePair(win fyne.Window, registry *services.RemoteRegistry,
	d services.RemoteDaemon, reload func()) fyne.CanvasObject {
	hint := widget.NewLabel(locale.T("remote.repair.hint"))
	hint.Wrapping = fyne.TextWrapWord

	// Команду выполняет пользователь НА САМОЙ МАШИНЕ — как и при добавлении,
	// поэтому только копирование: терминал открылся бы не на той машине.
	cmdRow := CommandRow(win, "remote.add.client_add_label", func() (string, error) {
		return "sudo sing-box lxd client add", nil
	}, false)

	inviteEntry := widget.NewMultiLineEntry()
	inviteEntry.SetPlaceHolder(locale.T("remote.add.invite_placeholder"))
	inviteEntry.Wrapping = fyne.TextWrapBreak
	inviteEntry.SetMinRowsVisible(3)

	// Адрес и секрет — те же два узких случая, что при добавлении: listen
	// 0.0.0.0 в приглашении и plain-h2c демон. Пусто = взять из приглашения,
	// а для адреса — оставить нынешний.
	addrEntry := widget.NewEntry()
	addrEntry.SetPlaceHolder(locale.T("remote.repair.addr_placeholder"))
	secretEntry := widget.NewPasswordEntry()
	secretEntry.SetPlaceHolder(locale.T("remote.add.secret_placeholder"))

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord

	repairBtn := widget.NewButton(locale.T("remote.repair.submit"), nil)
	repairBtn.Importance = widget.HighImportance
	repairBtn.OnTapped = func() {
		invite := strings.TrimSpace(inviteEntry.Text)
		if invite == "" {
			status.SetText(locale.T("remote.add.error_empty_invite"))
			return
		}
		// Подтверждение обязательно: успешный re-pair перевыпускает клиентский
		// ключ, и прежний мандат этого лаунчера на машине становится мусором.
		// Отменить это нечем — новый ключ старого не восстанавливает.
		dialog.ShowConfirm(locale.T("remote.repair.confirm_title"),
			locale.Tf("remote.repair.confirm_body", d.Name),
			func(ok bool) {
				if !ok {
					return
				}
				repairBtn.Disable()
				status.SetText(locale.T("remote.repair.pairing"))
				// Enroll — блокирующий сетевой вызов: недоступная машина
				// отвечает по таймауту REST-клиента.
				go func() {
					entry, err := registry.RePair(d.ID, invite,
						strings.TrimSpace(addrEntry.Text), strings.TrimSpace(secretEntry.Text))
					fyne.Do(func() {
						repairBtn.Enable()
						if err != nil {
							debuglog.WarnLog("edit machine: re-pair %q: %v", d.ID, err)
							status.SetText(locale.Tf("remote.add.error_pair", err))
							return
						}
						// Канал стал другим: прежнее соединение и его окна
						// разговаривают по отозванному пину. Рвём здесь, а не
						// оставляем пользователю — иначе строка показывала бы
						// «connected» на мандате, которого больше нет.
						if id, _, ok := GetLxdRemoteOverride(); ok && id == d.ID {
							CloseMachineProfiler(d.ID)
							CloseMachineHostWindow(d.ID)
						}
						debuglog.InfoLog("edit machine: re-paired %q at %s", entry.Name, entry.Addr)
						status.SetText(locale.Tf("remote.repair.done", entry.Addr))
						inviteEntry.SetText("")
						reload()
					})
				}()
			}, win)
	}

	advanced := widget.NewForm(
		widget.NewFormItem(locale.T("remote.add.field_addr"), addrEntry),
		widget.NewFormItem(locale.T("remote.add.field_secret"), secretEntry),
	)

	inner := container.NewVBox(
		hint,
		cmdRow,
		widget.NewForm(widget.NewFormItem(locale.T("remote.add.field_invite"), inviteEntry)),
		widget.NewAccordion(
			widget.NewAccordionItem(locale.T("remote.add.advanced"), advanced),
		),
		status,
		container.NewBorder(nil, nil, nil, repairBtn),
	)
	return widget.NewAccordion(
		widget.NewAccordionItem(locale.T("remote.repair.section"), inner),
	)
}

// machineEditCopyProfile — перенос настроек с ДРУГОЙ машины на эту.
//
// Отвечает на вопрос «у меня уже настроен роутер, как не собирать то же самое
// на VPS руками». Копируется состояние визарда (источники, правила, DNS,
// переменные) — то, что человек собирал сам.
//
// Сопряжение при этом не копируется, и говорить об этом надо прямо: у машины
// свой ключ, и общий на две означал бы, что отзыв доступа на одной отзывает
// его на обеих. Канал приёмника остаётся его собственным.
func machineEditCopyProfile(ac *core.AppController, win fyne.Window,
	registry *services.RemoteRegistry, d services.RemoteDaemon, reload func()) fyne.CanvasObject {
	hint := widget.NewLabel(locale.T("remote.copy.hint"))
	hint.Wrapping = fyne.TextWrapWord

	list, err := registry.List()
	if err != nil {
		debuglog.WarnLog("edit machine: read registry: %v", err)
	}

	// В источники попадают только машины, у которых ЕСТЬ что копировать:
	// предлагать пустую машину значило бы предлагать действие, которое
	// заведомо кончится отказом «нечего копировать».
	labels := make([]string, 0, len(list))
	byLabel := make(map[string]string, len(list))
	for _, other := range list {
		if other.ID == d.ID {
			continue
		}
		if !machineHasProfile(ac, other.ID) {
			continue
		}
		t := other.Target()
		label := fmt.Sprintf("%s  (%s/%s)", other.Name, t.GOOS, t.GOARCH)
		labels = append(labels, label)
		byLabel[label] = other.ID
	}

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord

	srcSelect := widget.NewSelect(labels, nil)
	srcSelect.PlaceHolder = locale.T("remote.copy.placeholder")

	copyBtn := widget.NewButton(locale.T("remote.copy.submit"), nil)
	copyBtn.OnTapped = func() {
		srcID, ok := byLabel[srcSelect.Selected]
		if !ok {
			status.SetText(locale.T("remote.copy.error_no_source"))
			return
		}
		apply := func() {
			if err := registry.CopyProfileFrom(srcID, d.ID); err != nil {
				debuglog.WarnLog("edit machine: copy profile %q → %q: %v", srcID, d.ID, err)
				status.SetText(locale.Tf("remote.copy.error", err))
				return
			}
			debuglog.InfoLog("edit machine: profile copied %q → %q", srcID, d.ID)
			status.SetText(locale.Tf("remote.copy.done", srcSelect.Selected))
			reload()
		}
		// Приёмник уже настроен — перезапись сотрёт его настройки, и вернуть
		// их будет неоткуда. Спрашиваем; на пустой машине спрашивать не о чем.
		if machineHasProfile(ac, d.ID) {
			dialog.ShowConfirm(locale.T("remote.copy.confirm_title"),
				locale.Tf("remote.copy.confirm_body", d.Name),
				func(ok bool) {
					if ok {
						apply()
					}
				}, win)
			return
		}
		apply()
	}

	if len(labels) == 0 {
		// Копировать неоткуда: либо машина одна, либо соседние ещё не
		// настраивали. Говорим это прямо вместо пустого выпадающего списка.
		srcSelect.Disable()
		copyBtn.Disable()
		status.SetText(locale.T("remote.copy.no_sources"))
	}

	inner := container.NewVBox(
		hint,
		container.NewBorder(nil, nil, nil, copyBtn, srcSelect),
		status,
	)
	return widget.NewAccordion(
		widget.NewAccordionItem(locale.T("remote.copy.section"), inner),
	)
}

// machineHasProfile — есть ли у машины сохранённое состояние визарда.
//
// Проверяется файл, а не «подключались ли когда-нибудь»: копировать можно
// ровно то, что лежит на диске, и связь машины с сетью тут ни при чём.
func machineHasProfile(ac *core.AppController, id string) bool {
	if ac == nil || ac.FileService == nil {
		return false
	}
	path := platform.GetWizardStatePathFor(ac.FileService.ExecDir, constants.ConfigTargetRemote, id)
	_, err := os.Stat(path)
	return err == nil
}
