package ui

import (
	"fmt"
	"os"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	"singbox-launcher/ui/configurator"
)

// Правая колонка вкладки Remote — список удалённых машин (SPEC 098 §2.1).
//
// Заменяет собой три разбросанных места: дропдаун выбора машины в шапке
// Servers, блок питания там же и область Remote в окне подключения. Смысл
// сведения — не эстетика: пока выбор машины и таргет конфига были разными
// переключателями, можно было собрать конфиг для роутера и задеплоить его на
// VPS. Здесь у каждой строки свои Configure и Deploy, и промахнуться нечем.

// machineListPanel — состояние правой колонки.
type machineListPanel struct {
	ac        *core.AppController
	registry  *services.RemoteRegistry
	list      *fyne.Container
	container *fyne.Container
	// proxies — панель списка узлов слева. Панель машин ею управляет: до
	// Connect она скрыта, после — показывается и перечитывается.
	proxies *ProxyListPanel
	// health — последний известный статус каждой машины, чтобы перерисовка
	// строки не ходила в сеть заново.
	health map[string]services.RemoteHealth
}

// CreateMachineListPanel строит правую колонку вкладки Remote.
func CreateMachineListPanel(ac *core.AppController, proxies *ProxyListPanel) fyne.CanvasObject {
	p := &machineListPanel{
		ac:       ac,
		registry: services.NewRemoteRegistry(ac.FileService.ExecDir),
		proxies:  proxies,
		health:   make(map[string]services.RemoteHealth),
	}
	p.list = container.NewVBox()

	addBtn := widget.NewButton(locale.T("remote.machines.add"), func() {
		OpenAddMachineWindow(ac, func() {
			// Только что добавленная машина ещё не подключена — узлов у неё
			// для нас нет, и Refresh ушёл бы с пустой группой.
			p.Reload()
		})
	})
	addBtn.Importance = widget.MediumImportance

	header := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle(locale.T("remote.machines.title"), fyne.TextAlignLeading,
			fyne.TextStyle{Bold: true}),
		addBtn,
	)

	scroll := container.NewVScroll(p.list)
	p.container = container.NewBorder(container.NewVBox(header, widget.NewSeparator()), nil, nil, nil, scroll)
	p.Reload()

	// Активная машина меняется и снаружи панели: переход на вкладку Local
	// снимает транспорт (SPEC 098 — Local всегда про своё ядро). Без этой
	// подписки маркер ● оставался бы висеть на машине, с которой разговор
	// уже не идёт.
	OnOverrideChanged(func() {
		fyne.Do(p.redrawRows)
	})
	return p.container
}

// Reload перечитывает реестр и перерисовывает строки.
func (p *machineListPanel) Reload() {
	list, err := p.registry.List()
	if err != nil {
		debuglog.WarnLog("machine list: read registry: %v", err)
	}
	p.list.RemoveAll()
	if len(list) == 0 {
		// Пустой список — не ошибка: пользователь ещё не сопрягался ни с чем.
		// Говорим, что делать, вместо пустого места.
		hint := widget.NewLabel(locale.T("remote.machines.empty"))
		hint.Wrapping = fyne.TextWrapWord
		p.list.Add(hint)
		p.list.Refresh()
		return
	}
	activeID, _, _ := GetLxdRemoteOverride()
	for _, d := range list {
		p.list.Add(p.buildRow(d, d.ID == activeID))
	}
	p.list.Refresh()
	// Никаких сетевых опросов здесь: показ списка — не повод стучаться к
	// чужим хостам. Состояние машины появляется только после явного Connect.
}

// buildRow — одна строка машины: имя, платформа, адрес, статус и кнопки.
func (p *machineListPanel) buildRow(d services.RemoteDaemon, active bool) fyne.CanvasObject {
	marker := "○"
	if active {
		marker = "●"
	}
	name := widget.NewLabelWithStyle(marker+" "+d.Name, fyne.TextAlignLeading,
		fyne.TextStyle{Bold: active})

	tgt := d.Target()
	meta := widget.NewLabel(fmt.Sprintf("%s/%s   %s", tgt.GOOS, tgt.GOARCH, d.Addr))

	// Соединения с машиной ещё не было — про её ядро мы не знаем НИЧЕГО, и
	// узнать можем только сходив по сети, чего без спроса делать нельзя.
	health, connected := p.health[d.ID]

	editBtn := ttwidget.NewButton("✎", func() {
		p.editMachine(d)
	})
	editBtn.SetToolTip(locale.T("remote.machines.edit_tooltip"))
	editBtn.Importance = widget.LowImportance

	removeBtn := ttwidget.NewButton("✕", func() {
		p.removeMachine(d)
	})
	removeBtn.SetToolTip(locale.T("remote.machines.remove_tooltip"))
	removeBtn.Importance = widget.LowImportance

	// Правка и удаление — напротив ИМЕНИ: это операции над самой записью,
	// а не над её ядром, поэтому доступны и без соединения.
	nameRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(editBtn, removeBtn), name)

	rows := []fyne.CanvasObject{nameRow, meta}

	// Configure правит профиль машины на диске — сеть для этого не нужна,
	// поэтому кнопка живёт до соединения (§2.4).
	configureBtn := widget.NewButton(locale.T("remote.machines.configure"), func() {
		configurator.ShowConfigWizardForMachine(p.ac.UIService.MainWindow, d)
	})

	if !connected {
		// До Connect строка показывает только паспорт машины и Connect.
		// Ни статуса, ни Start/Stop, ни Deploy: всё это требует ответа от
		// демона, а выдумывать его состояние — врать пользователю.
		connectBtn := widget.NewButton(locale.T("remote.machines.connect"), func() {
			p.connectMachine(d)
		})
		connectBtn.Importance = widget.HighImportance
		rows = append(rows, container.NewHBox(configureBtn, connectBtn), widget.NewSeparator())
		return container.NewVBox(rows...)
	}

	// Соединились: показываем настоящий статус ядра и открываем управление.
	//
	// Статусы демона: idle | started | fatal (RemoteHealth.CoreStatus).
	statusText := locale.T("remote.machines.reachable")
	switch {
	case health.Err != "":
		statusText = locale.Tf("remote.machines.unreachable", health.Err)
	case health.CoreStatus != "":
		statusText = health.CoreStatus
	}
	// Версия ядра идёт вместе со статусом: без неё непонятно, поддерживает ли
	// та сторона то, что мы деплоим, — а это первый вопрос при разборе сбоя.
	if health.Version != "" {
		statusText = health.Version + " · " + statusText
	}
	status := widget.NewLabel(statusText)
	status.Wrapping = fyne.TextWrapWord

	running := health.CoreStatus == "started"
	powerLabel := locale.T("servers.power.start")
	if running {
		powerLabel = locale.T("servers.power.stop")
	}
	powerBtn := widget.NewButton(powerLabel, func() {
		p.togglePower(d, running)
	})
	if health.Err != "" {
		// Машина не отвечает — управлять её ядром нечем.
		powerBtn.Disable()
	}

	// (i) — всё, что демон сообщил о себе: хеши конфигов, последняя ошибка,
	// state-dir. В строку это не влезает, а при разборе «почему на машине не
	// то» нужно целиком.
	infoBtn := ttwidget.NewButton("ⓘ", func() {
		p.showHealthDetails(d, health)
	})
	infoBtn.SetToolTip(locale.T("remote.machines.info_tooltip"))
	infoBtn.Importance = widget.LowImportance

	deployBtn := widget.NewButton(locale.T("servers.power.deploy"), func() {
		p.deployTo(d)
	})
	if health.Err != "" {
		deployBtn.Disable()
	}

	disconnectBtn := widget.NewButton(locale.T("remote.machines.disconnect"), func() {
		p.disconnectMachine()
	})

	// Start/Stop — напротив СТАТУСА, который он меняет: кнопка стоит там,
	// где виден её результат.
	statusRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(infoBtn, powerBtn), status)
	rows = append(rows,
		statusRow,
		container.NewHBox(configureBtn, deployBtn, disconnectBtn),
		widget.NewSeparator(),
	)
	return container.NewVBox(rows...)
}

// selectMachine делает машину активной: список прокси слева начинает
// показывать её узлы.
//
// Выбор эфемерный (SPEC 097 §4.3) — не переживает перезапуск, чтобы лаунчер
// всегда стартовал со своим ядром.
func (p *machineListPanel) connectMachine(d services.RemoteDaemon) {
	if err := SetLxdRemoteOverride(p.ac, d.ID); err != nil {
		debuglog.WarnLog("machine list: connect %q: %v", d.ID, err)
		dialog.ShowError(err, p.ac.UIService.MainWindow)
		return
	}
	// Спрашиваем состояние ядра — первый и единственный сетевой поход,
	// сделанный по явной команде пользователя. Блокирующий вызов, поэтому
	// в горутине: недоступная машина отвечает по таймауту REST-клиента.
	go func() {
		h := p.registry.Health(d.ID)
		fyne.Do(func() {
			p.health[d.ID] = h
			p.redrawRows()

			if h.Err != "" {
				debuglog.WarnLog("machine list: %q unreachable: %s", d.ID, h.Err)
				return
			}
			// Узлы читаем ТОЛЬКО если ядро на машине запущено. При idle их
			// физически нет: демон ответит «service is not started», и
			// пользователь получил бы ошибку там, где достаточно нажать Start.
			if h.CoreStatus != "started" {
				debuglog.InfoLog("machine list: %q connected, core is %q — press Start to load nodes",
					d.ID, h.CoreStatus)
				return
			}
			p.proxies.SetEnabled(true)
			p.loadNodes()
		})
	}()
}

// showHealthDetails показывает всё, что демон сообщил о себе.
//
// Отдельное окно, а не диалог: строк много и они длинные (хеши по 64 символа,
// путь к state-dir), а высокий модальный попап Fyne раздувает на весь экран.
//
// Значения показываются как есть, без «причёсывания»: это диагностика, и
// подмена пустого поля прочерком или домысленным текстом здесь стоила бы
// дороже, чем пустая строка.
func (p *machineListPanel) showHealthDetails(d services.RemoteDaemon, h services.RemoteHealth) {
	rows := [][2]string{
		{locale.T("remote.info.machine"), d.Name},
		{locale.T("remote.info.addr"), d.Addr},
		{locale.T("remote.info.platform"), fmt.Sprintf("%s/%s", d.Target().GOOS, d.Target().GOARCH)},
		{locale.T("remote.info.daemon_version"), h.Version},
		{locale.T("remote.info.core_status"), h.CoreStatus},
		{locale.T("remote.info.state_dir"), h.StateDir},
		{locale.T("remote.info.active_sha"), h.ActiveSHA},
		{locale.T("remote.info.last_good_sha"), h.LastGoodSHA},
	}
	if h.InterruptedApply {
		rows = append(rows, [2]string{locale.T("remote.info.interrupted"), locale.T("remote.info.interrupted_yes")})
	}
	if h.LastError != "" {
		rows = append(rows, [2]string{locale.T("remote.info.last_error"), h.LastError})
	}
	if h.Err != "" {
		rows = append(rows, [2]string{locale.T("remote.info.unreachable"), h.Err})
	}

	items := make([]*widget.FormItem, 0, len(rows))
	var plain strings.Builder
	for _, r := range rows {
		// Значение — не Label, а поле только для чтения: хеш и путь нужно
		// уметь выделить и скопировать, иначе диагностику не перенести в тикет.
		v := widget.NewEntry()
		v.SetText(r[1])
		v.Wrapping = fyne.TextWrapOff
		items = append(items, widget.NewFormItem(r[0], v))
		fmt.Fprintf(&plain, "%s: %s\n", r[0], r[1])
	}

	win := p.ac.UIService.Application.NewWindow(locale.Tf("remote.info.window_title", d.Name))
	copyBtn := widget.NewButton(locale.T("dialog.copy"), func() {
		win.Clipboard().SetContent(plain.String())
	})
	closeBtn := widget.NewButton(locale.T("dialog.close"), func() { win.Close() })
	closeBtn.Importance = widget.HighImportance

	body := container.NewVBox(
		widget.NewForm(items...),
		container.NewBorder(nil, nil, copyBtn, closeBtn),
	)
	win.SetContent(container.NewPadded(container.NewVScroll(body)))
	win.Resize(fyne.NewSize(560, 420))
	win.CenterOnScreen()
	win.Show()
}

// loadNodes перечитывает группы и список узлов активной машины.
//
// Сброс идёт первым: он перечитывает группы У ЭТОЙ машины и чистит прежний
// список. Без него запрос уходил бы с группой предыдущего источника —
// отсюда «Daemon: group "" not found».
func (p *machineListPanel) loadNodes() {
	if p.ac.UIService != nil && p.ac.UIService.ResetAPIStateFunc != nil {
		p.ac.UIService.ResetAPIStateFunc()
	}
	p.proxies.Refresh()
}

// disconnectMachine рвёт связь с машиной: строка сворачивается обратно к
// паспорту с кнопкой Connect, список слева пустеет.
//
// Список именно ОЧИЩАЕТСЯ, а не перезагружается: Refresh пошёл бы за узлами,
// когда транспорта уже нет и группа пуста, — и пользователь получал бы
// «Daemon: group "" not found» в ответ на штатное отключение.
func (p *machineListPanel) disconnectMachine() {
	p.proxies.SetEnabled(false)
	ClearLxdRemoteOverride(p.ac)
	p.health = make(map[string]services.RemoteHealth)
	p.Reload()
	p.proxies.Clear()
}

// editMachine — правка имени и адреса. Платформа правится там же: это
// свойство машины, и менять его из визарда нельзя (§2.4).
func (p *machineListPanel) editMachine(d services.RemoteDaemon) {
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

	dlg := dialog.NewCustomConfirm(locale.T("remote.machines.edit_title"),
		locale.T("dialog.button_save"), locale.T("dialog.button_cancel"), form,
		func(ok bool) {
			if !ok {
				return
			}
			if err := p.registry.Update(d.ID, nameEntry.Text, addrEntry.Text); err != nil {
				dialog.ShowError(err, p.ac.UIService.MainWindow)
				return
			}
			if err := p.registry.SetPlatform(d.ID, goosSelect.Selected, goarchSelect.Selected); err != nil {
				dialog.ShowError(err, p.ac.UIService.MainWindow)
				return
			}
			p.Reload()
		}, p.ac.UIService.MainWindow)
	dlg.Resize(fyne.NewSize(460, 300))
	dlg.Show()
}

// removeMachine удаляет машину со всем её имуществом (§3.1.9).
//
// Предупреждение про отзыв доступа обязательно: мы забываем ключ у себя, но
// регистрация на СТОРОНЕ демона остаётся, и снять её может только он сам.
// Промолчать здесь значило бы дать пользователю ложное чувство, что доступ
// отозван.
func (p *machineListPanel) removeMachine(d services.RemoteDaemon) {
	dialog.ShowConfirm(
		locale.T("remote.machines.remove_title"),
		locale.Tf("remote.machines.remove_body", d.Name),
		func(ok bool) {
			if !ok {
				return
			}
			activeID, _, _ := GetLxdRemoteOverride()
			if activeID == d.ID {
				// Снимаем выбор до удаления: иначе левая колонка осталась бы
				// с транспортом на машину, которой уже нет.
				ClearLxdRemoteOverride(p.ac)
			}
			if err := p.registry.Remove(d.ID); err != nil {
				dialog.ShowError(err, p.ac.UIService.MainWindow)
				return
			}
			p.Reload()
			if activeID == d.ID {
				p.proxies.SetEnabled(false)
				p.proxies.Clear()
			}
		}, p.ac.UIService.MainWindow)
}

// togglePower запускает или останавливает ядро на машине.
//
// Останов подтверждается, старт — нет: остановка рвёт VPN у всех, кто ходит
// через эту машину, а запуск безобиден.
func (p *machineListPanel) togglePower(d services.RemoteDaemon, running bool) {
	run := func() {
		go func() {
			var err error
			if running {
				err = p.registry.StopCore(d.ID)
			} else {
				err = p.registry.StartCore(d.ID)
			}
			// Перечитываем состояние: оно только что изменилось, и рисовать
			// строку по домыслу («нажали Start — значит started») нельзя —
			// ядро могло не подняться.
			h := p.registry.Health(d.ID)
			fyne.Do(func() {
				if err != nil {
					debuglog.WarnLog("machine list: power %q: %v", d.ID, err)
					dialog.ShowError(err, p.ac.UIService.MainWindow)
				}
				p.health[d.ID] = h
				p.redrawRows()

				if h.CoreStatus == "started" {
					// Ядро поднялось — вот теперь у машины есть группы и узлы.
					p.loadNodes()
					return
				}
				// Ядро остановлено: список слева должен опустеть, а не
				// показывать узлы, которых на машине уже нет.
				if p.ac.APIService != nil {
					p.ac.APIService.ResetScope(services.ScopeRemote)
				}
				p.proxies.Clear()
			})
		}()
	}
	if running {
		dialog.ShowConfirm(
			locale.T("servers.power.stop_title"),
			locale.Tf("servers.power.stop_body", d.Name),
			func(ok bool) {
				if ok {
					run()
				}
			}, p.ac.UIService.MainWindow)
		return
	}
	run()
}

// deployTo отправляет машине ЕЁ СОБСТВЕННЫЙ конфиг (§2.4).
//
// Путь считается от ID той же строки, из которой нажали, — поэтому промах
// «собрал для одной, задеплоил на другую» тут невозможен по конструкции, а не
// по проверке.
func (p *machineListPanel) deployTo(d services.RemoteDaemon) {
	path := platform.GetRemoteConfigPathFor(p.ac.FileService.ExecDir, d.ID)
	config, err := os.ReadFile(path)
	if err != nil {
		// Конфиг ещё не собирали. Говорим, что делать, вместо сырой ошибки
		// чтения — и указываем на Configure ИМЕННО этой машины.
		dialog.ShowInformation(locale.T("servers.power.deploy_title"),
			locale.Tf("remote.machines.deploy_missing", d.Name), p.ac.UIService.MainWindow)
		return
	}
	dialog.ShowConfirm(
		locale.T("servers.power.deploy_title"),
		locale.Tf("servers.power.deploy_body", d.Name, len(config)),
		func(ok bool) {
			if !ok {
				return
			}
			go func() {
				applyErr := p.registry.ApplyConfig(d.ID, config)
				fyne.Do(func() {
					if applyErr != nil {
						debuglog.WarnLog("machine list: deploy %q: %v", d.ID, applyErr)
						dialog.ShowError(applyErr, p.ac.UIService.MainWindow)
						return
					}
					dialog.ShowInformation(locale.T("servers.power.deploy_title"),
						locale.Tf("servers.power.deploy_done", d.Name), p.ac.UIService.MainWindow)
					p.Reload()
				})
			}()
		}, p.ac.UIService.MainWindow)
}

// redrawRows перерисовывает строки из кеша health, не трогая реестр.
func (p *machineListPanel) redrawRows() {
	list, err := p.registry.List()
	if err != nil {
		return
	}
	activeID, _, _ := GetLxdRemoteOverride()
	p.list.RemoveAll()
	for _, d := range list {
		p.list.Add(p.buildRow(d, d.ID == activeID))
	}
	p.list.Refresh()
}
