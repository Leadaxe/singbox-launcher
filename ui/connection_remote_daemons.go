package ui

import (
	"fmt"
	"net"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// SPEC 097 — панель «Удалённые машины» в окне подключения.
//
// Полный жизненный цикл подключения к чужому демону `sing-box lxd` (роутер,
// VPS, другой mac): добавить (имя + адрес + приглашение), подключиться,
// поправить адрес, удалить. Переключение между машинами — здесь и в
// дропдауне шапки Servers, обе точки работают с одним реестром.
//
// Кросс-платформенно, в отличие от buildRemoteDaemonPanel (darwin-only): та
// панель настраивает демон, которым лаунчер САМ управляет на этой машине —
// это macOS-фича. Здесь мы только клиент к чужому процессу, и лаунчер на
// Windows точно так же может рулить linux-роутером.
func buildRemoteDaemonsPanel(ac *core.AppController, win fyne.Window, onChanged func()) fyne.CanvasObject {
	if ac == nil || ac.FileService == nil {
		return widget.NewLabel(locale.T("conn.remotes.unavailable"))
	}
	registry := services.NewRemoteRegistry(ac.FileService.ExecDir)

	list := container.NewVBox()
	var refresh func()

	// notify — перерисовать саму панель и уведомить вкладку Servers, чтобы
	// она перечитала прокси с нового источника.
	notify := func() {
		refresh()
		if onChanged != nil {
			onChanged()
		}
	}

	connectTo := func(entry services.RemoteDaemon) {
		if err := SetLxdRemoteOverride(ac, entry.ID); err != nil {
			dialog.ShowError(err, win)
			return
		}
		notify()
	}

	disconnect := func() {
		ClearLxdRemoteOverride()
		notify()
	}

	remove := func(entry services.RemoteDaemon) {
		dialog.ShowConfirm(
			locale.T("conn.remotes.remove_title"),
			locale.Tf("conn.remotes.remove_body", entry.Name),
			func(ok bool) {
				if !ok {
					return
				}
				// Если удаляем ту машину, которую сейчас смотрим — сначала
				// отцепляемся, иначе Servers остался бы с транспортом к
				// подключению, которого уже нет в реестре.
				if id, _, active := GetLxdRemoteOverride(); active && id == entry.ID {
					ClearLxdRemoteOverride()
					if onChanged != nil {
						onChanged()
					}
				}
				if err := registry.Remove(entry.ID); err != nil {
					dialog.ShowError(err, win)
					return
				}
				notify()
			}, win)
	}

	editEntry := func(entry services.RemoteDaemon) {
		nameEntry := widget.NewEntry()
		nameEntry.SetText(entry.Name)
		addrEntry := widget.NewEntry()
		addrEntry.SetText(entry.Addr)
		addrEntry.SetPlaceHolder("192.168.10.1:9091")

		form := widget.NewForm(
			widget.NewFormItem(locale.T("conn.remotes.name"), nameEntry),
			widget.NewFormItem(locale.T("conn.remotes.addr"), addrEntry),
		)
		hint := wrappedInfoLabel(locale.T("conn.remotes.addr_hint"))
		hint.Importance = widget.LowImportance

		d := dialog.NewCustomConfirm(
			locale.T("conn.remotes.edit_title"), locale.T("diag.save"), locale.T("diag.cancel"),
			container.NewVBox(form, hint),
			func(ok bool) {
				if !ok {
					return
				}
				if err := registry.Update(entry.ID, nameEntry.Text, addrEntry.Text); err != nil {
					dialog.ShowError(err, win)
					return
				}
				// Адрес мог смениться — активный транспорт смотрит на старый.
				if id, _, active := GetLxdRemoteOverride(); active && id == entry.ID {
					if err := SetLxdRemoteOverride(ac, entry.ID); err != nil {
						debuglog.WarnLog("remotes: reconnect after edit: %v", err)
					}
				}
				notify()
			}, win)
		d.Resize(fyne.NewSize(520, 260))
		d.Show()
	}

	// addRemote — добавление машины: имя, адрес, приглашение.
	//
	// Адрес отдельным полем, хотя он есть и в приглашении: демон печатает
	// в приглашении свой listen-адрес, и при listen 0.0.0.0 оттуда приезжает
	// нерабочее значение. Пустое поле = взять адрес из приглашения.
	addRemote := func() {
		nameEntry := widget.NewEntry()
		nameEntry.SetPlaceHolder(locale.T("conn.remotes.name_placeholder"))
		addrEntry := widget.NewEntry()
		addrEntry.SetPlaceHolder("192.168.10.1:9091")
		inviteEntry := widget.NewEntry()
		inviteEntry.SetPlaceHolder("address#fingerprint#code")
		secretEntry := widget.NewPasswordEntry()
		secretEntry.SetPlaceHolder(locale.T("conn.remotes.secret_placeholder"))

		// Команду отдельным полем, а не в тексте: из Label её не скопировать.
		cmdField := widget.NewEntry()
		cmdField.SetText("sudo sing-box lxd client add")
		cmdField.Wrapping = fyne.TextWrapOff

		form := widget.NewForm(
			widget.NewFormItem(locale.T("conn.remotes.name"), nameEntry),
			widget.NewFormItem(locale.T("conn.remotes.addr"), addrEntry),
			widget.NewFormItem(locale.T("conn.remotes.invite"), inviteEntry),
			widget.NewFormItem(locale.T("conn.remotes.secret"), secretEntry),
		)
		body := container.NewVBox(
			wrappedInfoLabel(locale.T("conn.remotes.add_hint")),
			cmdField,
			widget.NewSeparator(),
			form,
		)

		d := dialog.NewCustomConfirm(
			locale.T("conn.remotes.add_title"), locale.T("conn.remotes.add_action"), locale.T("diag.cancel"),
			body,
			func(ok bool) {
				if !ok {
					return
				}
				// Enroll ходит по сети — не блокируем UI-поток.
				go func() {
					entry, err := registry.PairWithAddr(
						inviteEntry.Text, nameEntry.Text, addrEntry.Text, secretEntry.Text)
					fyne.Do(func() {
						if err != nil {
							dialog.ShowError(err, win)
							return
						}
						notify()
						dialog.ShowInformation(
							locale.T("conn.remotes.added_title"),
							locale.Tf("conn.remotes.added_body", entry.Name, entry.Addr), win)
					})
				}()
			}, win)
		d.Resize(fyne.NewSize(640, 420))
		d.Show()
	}

	// importPaired — забрать в реестр подключение, сопряжённое панелью выше
	// (settings.json). Нужно для машин, сопряжённых до появления реестра:
	// повторный enroll им не требуется, ключ уже доверен демоном.
	importPaired := func() {
		st := locale.LoadSettings(platform.GetBinDir(ac.FileService.ExecDir))
		addr := strings.TrimSpace(st.DaemonAddress)
		if addr == "" {
			dialog.ShowInformation(locale.T("conn.remotes.import_title"),
				locale.T("conn.remotes.import_nothing"), win)
			return
		}
		if isLoopbackAddr(addr) {
			dialog.ShowInformation(locale.T("conn.remotes.import_title"),
				locale.T("conn.remotes.import_local"), win)
			return
		}
		nameEntry := widget.NewEntry()
		nameEntry.SetPlaceHolder(locale.T("conn.remotes.name_placeholder"))
		nameEntry.SetText(addr)
		body := container.NewVBox(
			wrappedInfoLabel(locale.Tf("conn.remotes.import_body", addr)),
			widget.NewForm(widget.NewFormItem(locale.T("conn.remotes.name"), nameEntry)),
		)
		d := dialog.NewCustomConfirm(locale.T("conn.remotes.import_title"),
			locale.T("conn.remotes.import_action"), locale.T("diag.cancel"), body,
			func(ok bool) {
				if !ok {
					return
				}
				entry, err := registry.ImportPairedDaemon(
					nameEntry.Text, addr, st.DaemonServerFingerprint, st.DaemonSecret,
					core.DaemonIdentityDir(ac.FileService.ExecDir))
				if err != nil {
					dialog.ShowError(err, win)
					return
				}
				notify()
				dialog.ShowInformation(locale.T("conn.remotes.import_title"),
					locale.Tf("conn.remotes.import_done", entry.Name, entry.Addr), win)
			}, win)
		d.Resize(fyne.NewSize(560, 260))
		d.Show()
	}

	refresh = func() {
		list.RemoveAll()
		entries, err := registry.List()
		if err != nil {
			lbl := wrappedInfoLabel(fmt.Sprintf("%v", err))
			lbl.Importance = widget.DangerImportance
			list.Add(lbl)
			list.Refresh()
			return
		}
		activeID, _, active := GetLxdRemoteOverride()
		if len(entries) == 0 {
			empty := wrappedInfoLabel(locale.T("conn.remotes.empty"))
			empty.Importance = widget.LowImportance
			list.Add(empty)
		}
		for _, e := range entries {
			entry := e
			title := entry.Name
			if active && entry.ID == activeID {
				title = "● " + title
			}
			nameLbl := widget.NewLabel(title)
			addrLbl := widget.NewLabel(entry.Addr)
			addrLbl.Importance = widget.LowImportance

			var actionBtn *widget.Button
			if active && entry.ID == activeID {
				actionBtn = widget.NewButton(locale.T("conn.remotes.disconnect"), disconnect)
			} else {
				actionBtn = widget.NewButton(locale.T("conn.remotes.connect"), func() { connectTo(entry) })
				actionBtn.Importance = widget.HighImportance
			}
			editBtn := widget.NewButton(locale.T("conn.remotes.edit"), func() { editEntry(entry) })
			delBtn := widget.NewButton(locale.T("conn.remotes.remove"), func() { remove(entry) })
			delBtn.Importance = widget.DangerImportance

			list.Add(container.NewBorder(nil, nil,
				container.NewVBox(nameLbl, addrLbl),
				container.NewHBox(actionBtn, editBtn, delBtn),
			))
			list.Add(widget.NewSeparator())
		}
		list.Refresh()
	}
	refresh()

	addBtn := widget.NewButton(locale.T("conn.remotes.add_action"), addRemote)
	addBtn.Importance = widget.HighImportance
	importBtn := widget.NewButton(locale.T("conn.remotes.import_action"), importPaired)

	return container.NewVBox(
		wrappedInfoLabel(locale.T("conn.remotes.intro")),
		container.NewHBox(addBtn, importBtn),
		widget.NewSeparator(),
		list,
	)
}

// wrappedInfoLabel — пояснение с переносом по словам.
func wrappedInfoLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}

// isLoopbackAddr — указывает ли host:port на эту же машину.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
