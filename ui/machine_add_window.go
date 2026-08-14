package ui

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/ui/components"
)

// Окно добавления удалённой машины (SPEC 098).
//
// Отдельное окно, а не диалог: полей пять, приглашение длинное, а модальный
// попап в Fyne на высокой форме раздувается на весь экран (см. память
// проекта — Application.NewWindow вместо dialog для высоких форм).
//
// Раньше сопряжение жило в окне подключения вперемешку с настройками СВОЕГО
// демона. Оттуда росла путаница: Pair там затирал адрес локального демона,
// потому что поля были общие. Здесь окно занимается ровно одним — заводит
// новую запись в реестре машин.

// OpenAddMachineWindow открывает окно добавления машины.
// onAdded зовётся после успешного сопряжения (обновить список).
func OpenAddMachineWindow(ac *core.AppController, onAdded func()) {
	if ac == nil || ac.UIService == nil || ac.UIService.Application == nil {
		return
	}
	win := ac.UIService.Application.NewWindow(locale.T("remote.add.window_title"))

	// Приглашение печатает демон на самой машине: `адрес#отпечаток#код`.
	// Многострочное поле — строка длинная, и в однострочном её не видно
	// целиком, из-за чего легко вставить обрезанное.
	inviteEntry := widget.NewMultiLineEntry()
	inviteEntry.SetPlaceHolder(locale.T("remote.add.invite_placeholder"))
	inviteEntry.Wrapping = fyne.TextWrapBreak
	inviteEntry.SetMinRowsVisible(3)

	nameEntry := widget.NewEntry()
	nameEntry.SetPlaceHolder(locale.T("remote.add.name_placeholder"))

	// Адрес подключения. Пусто = взять из приглашения.
	//
	// Нужен, потому что демон печатает в приглашении СВОЙ listen-адрес: при
	// listen 0.0.0.0 оттуда приезжает нерабочее значение, а при listen на
	// LAN-интерфейсе — адрес, по которому мы можем быть недоступны.
	addrEntry := widget.NewEntry()
	addrEntry.SetPlaceHolder(locale.T("remote.add.addr_placeholder"))

	// Платформа машины — свойство записи реестра (§2.4): под неё собирается
	// её конфиг, и она же видна в строке списка. Спрашиваем сразу, чтобы не
	// пришлось искать это в правке после первого же Deploy.
	goosSelect := widget.NewSelect([]string{"linux", "darwin", "windows"}, nil)
	goosSelect.SetSelected("linux")
	goarchSelect := widget.NewSelect([]string{"amd64", "arm64", "arm", "386", "mips", "mipsle"}, nil)
	goarchSelect.SetSelected("amd64")

	// Секрет нужен только plain-h2c демону (dev на loopback); при mTLS
	// мандатом служит клиентский сертификат.
	secretEntry := widget.NewPasswordEntry()
	secretEntry.SetPlaceHolder(locale.T("remote.add.secret_placeholder"))

	// Обычный путь — одно приглашение: в нём уже есть и адрес, и отпечаток
	// сервера, а мандатом служит клиентский сертификат (mTLS).
	form := widget.NewForm(
		widget.NewFormItem(locale.T("remote.add.field_name"), nameEntry),
		widget.NewFormItem(locale.T("remote.add.field_invite"), inviteEntry),
		widget.NewFormItem(locale.T("remote.machines.field_platform"), goosSelect),
		widget.NewFormItem(locale.T("remote.machines.field_arch"), goarchSelect),
	)

	// Адрес и секрет нужны в двух узких случаях, поэтому спрятаны:
	//   - демон слушает 0.0.0.0 — в приглашении приезжает нерабочий адрес;
	//   - демон поднят на plain-h2c (dev на loopback) — там мандат Bearer.
	// Держать их в основной форме значило бы спрашивать при каждом
	// добавлении то, что почти всегда берётся из приглашения.
	advancedForm := widget.NewForm(
		widget.NewFormItem(locale.T("remote.add.field_addr"), addrEntry),
		widget.NewFormItem(locale.T("remote.add.field_secret"), secretEntry),
	)
	advanced := widget.NewAccordion(
		widget.NewAccordionItem(locale.T("remote.add.advanced"), advancedForm),
	)

	hint := widget.NewLabel(locale.T("remote.add.hint"))
	hint.Wrapping = fyne.TextWrapWord

	// Команду выполняет пользователь НА САМОЙ МАШИНЕ (там свой путь к бинарю
	// и свой sudo), поэтому кнопки «открыть в терминале» тут нет — только
	// копирование: терминал открылся бы на этой машине, а нужен на той.
	inviteCmdRow := CommandRow(win, "remote.add.client_add_label", func() (string, error) {
		return "sudo sing-box lxd client add", nil
	}, false)

	docsLink := widget.NewHyperlink(locale.T("remote.add.docs_link"), nil)
	_ = docsLink.SetURLFromString(constants.RemoteDaemonDocsURL)
	docsLink.OnTapped = func() {
		if err := platform.OpenURL(constants.RemoteDaemonDocsURL); err != nil {
			debuglog.ErrorLog("add machine: open docs link: %v", err)
			ShowError(win, err)
		}
	}

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord

	cancelBtn := widget.NewButton(locale.T("dialog.button_cancel"), func() { win.Close() })
	addBtn := widget.NewButton(locale.T("remote.add.submit"), nil)
	addBtn.Importance = widget.HighImportance

	addBtn.OnTapped = func() {
		invite := strings.TrimSpace(inviteEntry.Text)
		if invite == "" {
			status.SetText(locale.T("remote.add.error_empty_invite"))
			return
		}
		addBtn.Disable()
		status.SetText(locale.T("remote.add.pairing"))

		// Сопряжение — блокирующий сетевой вызов (enroll по mTLS), поэтому в
		// горутине: недоступная машина отвечает по таймауту.
		go func() {
			registry := services.NewRemoteRegistry(ac.FileService.ExecDir)
			entry, err := registry.PairWithAddr(invite, nameEntry.Text,
				strings.TrimSpace(addrEntry.Text), strings.TrimSpace(secretEntry.Text))
			if err == nil {
				// Платформа — отдельным шагом: PairWithAddr её не знает, а
				// генерация конфига машины берёт GOOS/GOARCH именно отсюда.
				if setErr := registry.SetPlatform(entry.ID, goosSelect.Selected, goarchSelect.Selected); setErr != nil {
					debuglog.WarnLog("add machine: set platform for %q: %v", entry.ID, setErr)
				}
			}
			fyne.Do(func() {
				addBtn.Enable()
				if err != nil {
					debuglog.WarnLog("add machine: pair failed: %v", err)
					// Код приглашения одноразовый и сгорает после первой
					// попытки — говорим об этом прямо, иначе пользователь
					// будет жать «Add» с тем же кодом.
					status.SetText(locale.Tf("remote.add.error_pair", err))
					return
				}
				debuglog.InfoLog("add machine: paired %q (%s)", entry.Name, entry.Addr)
				if onAdded != nil {
					onAdded()
				}
				win.Close()
			})
		}()
	}

	buttons := container.NewBorder(nil, nil, nil,
		container.NewHBox(cancelBtn, addBtn))

	body := container.NewVBox(
		hint,
		inviteCmdRow,
		docsLink,
		widget.NewSeparator(),
		form,
		advanced,
		status,
		buttons,
	)
	// Резервируем место под полосу прокрутки: с раскрытым Advanced форма
	// заведомо длиннее окна, а без gutter полоса ложится поверх правого края
	// полей ввода. Канонический хелпер проекта — тот же, что в окне
	// подключения и настройках.
	win.SetContent(container.NewPadded(components.WrapInScrollWithGutter(body)))
	// Высота с запасом под раскрытый Advanced: при 520 форма упиралась в низ
	// окна и появлялась прокрутка на ровном месте.
	win.Resize(fyne.NewSize(520, 570))
	win.CenterOnScreen()
	win.Show()
}
