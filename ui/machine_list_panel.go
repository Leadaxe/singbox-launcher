package ui

import (
	"fmt"
	"os"

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
	// onSelectionChanged — перезагрузка списка прокси слева после смены
	// активной машины.
	onSelectionChanged func()
	// health — последний известный статус каждой машины, чтобы перерисовка
	// строки не ходила в сеть заново.
	health map[string]services.RemoteHealth
}

// CreateMachineListPanel строит правую колонку вкладки Remote.
func CreateMachineListPanel(ac *core.AppController, onSelectionChanged func()) fyne.CanvasObject {
	p := &machineListPanel{
		ac:                 ac,
		registry:           services.NewRemoteRegistry(ac.FileService.ExecDir),
		onSelectionChanged: onSelectionChanged,
		health:             make(map[string]services.RemoteHealth),
	}
	p.list = container.NewVBox()

	addBtn := widget.NewButton(locale.T("remote.machines.add"), func() {
		// Сопряжение живёт в окне подключения — там уже есть разбор
		// приглашения и обработка ошибок enroll'а. Дублировать его здесь
		// значило бы иметь две реализации одноразового кода.
		OpenConnectionWindow(ac, func() {
			p.Reload()
			if p.onSelectionChanged != nil {
				p.onSelectionChanged()
			}
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
	p.refreshHealthAsync(list)
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

	// Статус ядра: то, что уже успел вернуть фоновой health-опрос. Пока не
	// вернул — честное «проверяем», а не выдуманное «остановлено».
	statusText := locale.T("remote.machines.checking")
	if h, ok := p.health[d.ID]; ok {
		switch {
		case h.Err != "":
			statusText = locale.Tf("remote.machines.unreachable", h.Err)
		case h.CoreStatus != "":
			statusText = h.CoreStatus
		default:
			statusText = locale.T("remote.machines.reachable")
		}
	}
	status := widget.NewLabel(statusText)
	status.Wrapping = fyne.TextWrapWord

	// Configure — визард, корневой на профиле ЭТОЙ машины (§2.4). Именно
	// здесь исчезает выбор таргета: он задан тем, из какой строки открыли.
	configureBtn := widget.NewButton(locale.T("remote.machines.configure"), func() {
		configurator.ShowConfigWizardForMachine(p.ac.UIService.MainWindow, d)
	})

	selectBtn := widget.NewButton(locale.T("remote.machines.select"), func() {
		p.selectMachine(d)
	})
	if active {
		selectBtn.Disable()
	}

	// Питание ядра машины. Надпись зависит от последнего известного статуса;
	// пока health не вернулся — Start, потому что «запустить уже запущенное»
	// демон отвергает безобидно, а «остановить» вслепую рвёт чужой VPN.
	//
	// Статусы демона: idle | started | fatal (RemoteHealth.CoreStatus).
	// Сравнение с "running" не совпадало ни с одним из них, поэтому кнопка
	// всегда показывала Start и остановить ядро было нечем.
	running := false
	if h, ok := p.health[d.ID]; ok && h.CoreStatus == "started" {
		running = true
	}
	powerLabel := locale.T("servers.power.start")
	if running {
		powerLabel = locale.T("servers.power.stop")
	}
	powerBtn := widget.NewButton(powerLabel, func() {
		p.togglePower(d, running)
	})

	deployBtn := widget.NewButton(locale.T("servers.power.deploy"), func() {
		p.deployTo(d)
	})

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
	// а не над её ядром. Start/Stop — напротив СТАТУСА, который он меняет:
	// кнопка стоит там, где виден её результат.
	nameRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(editBtn, removeBtn), name)
	statusRow := container.NewBorder(nil, nil, nil,
		container.NewHBox(powerBtn), status)

	card := container.NewVBox(
		nameRow,
		meta,
		statusRow,
		container.NewHBox(configureBtn, deployBtn, selectBtn),
		widget.NewSeparator(),
	)
	return card
}

// selectMachine делает машину активной: список прокси слева начинает
// показывать её узлы.
//
// Выбор эфемерный (SPEC 097 §4.3) — не переживает перезапуск, чтобы лаунчер
// всегда стартовал со своим ядром.
func (p *machineListPanel) selectMachine(d services.RemoteDaemon) {
	if err := SetLxdRemoteOverride(p.ac, d.ID); err != nil {
		debuglog.WarnLog("machine list: select %q: %v", d.ID, err)
		dialog.ShowError(err, p.ac.UIService.MainWindow)
		return
	}
	p.Reload()
	if p.onSelectionChanged != nil {
		p.onSelectionChanged()
	}
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
			if activeID == d.ID && p.onSelectionChanged != nil {
				p.onSelectionChanged()
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
			fyne.Do(func() {
				if err != nil {
					debuglog.WarnLog("machine list: power %q: %v", d.ID, err)
					dialog.ShowError(err, p.ac.UIService.MainWindow)
				}
				p.Reload()
				if p.onSelectionChanged != nil {
					p.onSelectionChanged()
				}
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

// refreshHealthAsync опрашивает машины в фоне и перерисовывает строки.
//
// Строго вне UI-потока: недоступный роутер отвечает по таймауту REST-клиента,
// и синхронный опрос подвесил бы окно на всё это время.
func (p *machineListPanel) refreshHealthAsync(list []services.RemoteDaemon) {
	for _, d := range list {
		go func(d services.RemoteDaemon) {
			h := p.registry.Health(d.ID)
			fyne.Do(func() {
				p.health[d.ID] = h
				p.redrawRows()
			})
		}(d)
	}
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
