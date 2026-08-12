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
// Управляет реестром сохранённых демонов `sing-box lxd` (роутер, VPS, другой
// mac) и переключает вкладку Servers на выбранный.
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

	connectTo := func(entry services.RemoteDaemon) {
		if err := SetLxdRemoteOverride(ac, entry.ID); err != nil {
			dialog.ShowError(err, win)
			return
		}
		refresh()
		if onChanged != nil {
			onChanged()
		}
	}

	disconnect := func() {
		ClearLxdRemoteOverride()
		refresh()
		if onChanged != nil {
			onChanged()
		}
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
				refresh()
			}, win)
	}

	editAddr := func(entry services.RemoteDaemon) {
		addrEntry := widget.NewEntry()
		addrEntry.SetText(entry.Addr)
		addrEntry.SetPlaceHolder("192.168.10.1:9500")
		form := widget.NewForm(widget.NewFormItem(locale.T("conn.remotes.addr"), addrEntry))
		hint := widget.NewLabel(locale.T("conn.remotes.addr_hint"))
		hint.Wrapping = fyne.TextWrapWord
		hint.Importance = widget.LowImportance
		d := dialog.NewCustomConfirm(
			locale.T("conn.remotes.edit_addr_title"), locale.T("diag.save"), locale.T("diag.cancel"),
			container.NewVBox(form, hint),
			func(ok bool) {
				if !ok {
					return
				}
				if err := registry.SetAddr(entry.ID, addrEntry.Text); err != nil {
					dialog.ShowError(err, win)
					return
				}
				// Адрес сменился — активный транспорт смотрит на старый.
				if id, _, active := GetLxdRemoteOverride(); active && id == entry.ID {
					if err := SetLxdRemoteOverride(ac, entry.ID); err != nil {
						debuglog.WarnLog("remotes: reconnect after addr change: %v", err)
					}
					if onChanged != nil {
						onChanged()
					}
				}
				refresh()
			}, win)
		d.Resize(fyne.NewSize(460, 200))
		d.Show()
	}

	// Импорт подключения, сопряжённого через панель выше (settings.json).
	//
	// Зачем это вместо второго Pair: сопряжение пишет адрес и пин в
	// ЕДИНСТВЕННЫЙ набор полей settings.json. Сопряглись с роутером — и
	// подключение к своему демону затёрто (именно так оно и ломалось).
	// Импорт забирает чужую машину в реестр вместе с копией ключей, после
	// чего settings.json можно вернуть локальному демону.
	importPaired := func() {
		st := locale.LoadSettings(platform.GetBinDir(ac.FileService.ExecDir))
		addr := strings.TrimSpace(st.DaemonAddress)
		if addr == "" {
			dialog.ShowInformation(locale.T("conn.remotes.import_title"),
				locale.T("conn.remotes.import_nothing"), win)
			return
		}
		if isLoopbackAddr(addr) {
			// Свой демон импортировать незачем: им управляет панель выше,
			// и он и так доступен движку.
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
				refresh()
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
			lbl := widget.NewLabel(fmt.Sprintf("%v", err))
			lbl.Importance = widget.DangerImportance
			lbl.Wrapping = fyne.TextWrapWord
			list.Add(lbl)
			list.Refresh()
			return
		}
		activeID, _, active := GetLxdRemoteOverride()
		if len(entries) == 0 {
			empty := widget.NewLabel(locale.T("conn.remotes.empty"))
			empty.Wrapping = fyne.TextWrapWord
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
			editBtn := widget.NewButton(locale.T("conn.remotes.edit_addr"), func() { editAddr(entry) })
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

	intro := widget.NewLabel(locale.T("conn.remotes.intro"))
	intro.Wrapping = fyne.TextWrapWord

	addBtn := widget.NewButton(locale.T("conn.remotes.import_action"), importPaired)
	addBtn.Importance = widget.HighImportance

	return container.NewVBox(
		intro,
		container.NewHBox(addBtn),
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
