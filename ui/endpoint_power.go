package ui

import (
	"os"

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

// SPEC 097 — управление ядром ВЫБРАННОЙ машины прямо в шапке Servers.
//
// До этого в шапке стоял статус, который всегда показывал ЛОКАЛЬНОЕ ядро:
// выбрана удалённая машина, а надпись — про своё. Управлять удалённым ядром
// из UI было нечем вовсе: сопряглись, видим прокси, а поднять там ядро
// нельзя.
//
// Теперь один контрол: кнопка Start/Stop и короткий статус той машины,
// которая выбрана в дропдауне рядом.

// endpointPowerControl — кнопка + статус выбранного источника.
type endpointPowerControl struct {
	container *fyne.Container
	button    *widget.Button
	deploy    *widget.Button
	status    *widget.Label

	ac *core.AppController
	// pending гасит кнопку на время сетевой операции: старт/стоп на роутере
	// отвечает не мгновенно, и без блокировки второй клик уедет вдогонку.
	pending bool
}

// newEndpointPowerControl строит контрол. onChanged дёргается после
// успешной операции — вкладка должна перечитать прокси.
func newEndpointPowerControl(ac *core.AppController, onChanged func()) *endpointPowerControl {
	c := &endpointPowerControl{
		ac:     ac,
		status: widget.NewLabel(""),
	}
	c.button = widget.NewButton("", func() { c.toggle(onChanged) })
	// Кнопка деплоя: отправить bin/remote-config.json на выбранную машину.
	// Раньше цикл обрывался — визард собирал конфиг в файл, а доставлять его
	// приходилось руками.
	c.deploy = widget.NewButton(locale.T("servers.power.deploy"), func() { c.deployConfig(onChanged) })
	c.container = container.NewHBox(c.button, c.deploy, c.status)

	c.refresh()
	// Смена источника (дропдаун, окно подключения) — перечитать состояние.
	OnOverrideChanged(func() { fyne.Do(c.refresh) })
	return c
}

// refresh перечитывает состояние выбранного источника.
//
// Для локального — из контроллера (мгновенно). Для удалённого — по сети,
// поэтому опрос уходит в горутину, а кнопка до ответа заблокирована: иначе
// недоступный роутер подвесил бы UI на таймаут.
func (c *endpointPowerControl) refresh() {
	id, _, isRemote := GetLxdRemoteOverride()
	if !isRemote {
		// Локальному ядру деплой не нужен: конфиг для него собирает
		// Update/Rebuild из своего же state.
		c.deploy.Hide()
		c.applyLocalState()
		return
	}
	c.deploy.Show()
	c.status.SetText(locale.T("conn.remotes.health_checking"))
	c.status.Refresh()
	c.button.Disable()
	go func(machineID string) {
		health := services.NewRemoteRegistry(c.ac.FileService.ExecDir).Health(machineID)
		fyne.Do(func() {
			// Пока ходили по сети, пользователь мог переключить источник —
			// не затираем состояние другой машины.
			if curID, _, active := GetLxdRemoteOverride(); !active || curID != machineID {
				return
			}
			c.applyRemoteState(health)
		})
	}(id)
}

func (c *endpointPowerControl) applyLocalState() {
	running := false
	if c.ac != nil {
		running = c.ac.RunningState.IsRunning()
	}
	c.render(running, locale.T("servers.power.local"))
}

func (c *endpointPowerControl) applyRemoteState(h services.RemoteHealth) {
	if !h.Reachable {
		c.status.SetText(locale.T("conn.remotes.health_unreachable"))
		c.status.Importance = widget.DangerImportance
		c.status.Refresh()
		// Кнопка бесполезна, пока машина не отвечает: команда всё равно не
		// дойдёт, а активная кнопка обещала бы обратное.
		c.button.SetText(locale.T("servers.power.start"))
		c.button.Disable()
		return
	}
	c.render(h.CoreStatus == "started", h.CoreStatus)
}

// render — общий вид: кнопка противоположного действия + короткий статус.
func (c *endpointPowerControl) render(running bool, statusText string) {
	if running {
		c.button.SetText(locale.T("servers.power.stop"))
		c.button.Importance = widget.MediumImportance
	} else {
		c.button.SetText(locale.T("servers.power.start"))
		c.button.Importance = widget.HighImportance
	}
	if !c.pending {
		c.button.Enable()
	}
	c.button.Refresh()

	c.status.SetText(statusText)
	c.status.Importance = widget.MediumImportance
	c.status.Refresh()
}

// toggle запускает или останавливает ядро выбранного источника.
func (c *endpointPowerControl) toggle(onChanged func()) {
	id, name, isRemote := GetLxdRemoteOverride()
	if !isRemote {
		// Локальное ядро: те же функции, что у кнопок на вкладке Core.
		if c.ac != nil && c.ac.RunningState.IsRunning() {
			core.StopSingBoxProcess()
		} else {
			core.StartSingBoxProcess()
		}
		c.refresh()
		return
	}

	registry := services.NewRemoteRegistry(c.ac.FileService.ExecDir)
	run := func(stop bool) {
		c.pending = true
		c.button.Disable()
		c.status.SetText(locale.T("conn.remotes.health_checking"))
		c.status.Refresh()
		go func() {
			var err error
			if stop {
				err = registry.StopCore(id)
			} else {
				err = registry.StartCore(id)
			}
			fyne.Do(func() {
				c.pending = false
				if err != nil {
					debuglog.WarnLog("remote power: %v", err)
					dialog.ShowError(err, c.ac.UIService.MainWindow)
				}
				c.refresh()
				if err == nil && onChanged != nil {
					onChanged()
				}
			})
		}()
	}

	// Останов рвёт VPN у всех, кто ходит через эту машину, — спрашиваем.
	// Старт безобиден, его подтверждать не нужно.
	if c.button.Text == locale.T("servers.power.stop") {
		dialog.ShowConfirm(
			locale.T("servers.power.stop_title"),
			locale.Tf("servers.power.stop_body", name),
			func(ok bool) {
				if ok {
					run(true)
				}
			}, c.ac.UIService.MainWindow)
		return
	}
	run(false)
}

// deployConfig отправляет bin/remote-config.json на выбранную машину.
//
// Конфиг берётся из файла, который пишет визард для remote-таргета, — это
// тот же артефакт, что пользователь видит в превью. Собирать его здесь
// заново значило бы иметь два источника правды.
func (c *endpointPowerControl) deployConfig(onChanged func()) {
	id, name, isRemote := GetLxdRemoteOverride()
	if !isRemote {
		return
	}
	path := platform.GetRemoteConfigPath(c.ac.FileService.ExecDir)
	config, err := os.ReadFile(path)
	if err != nil {
		// Файла нет — конфиг для remote ещё не собирали. Говорим что делать,
		// а не показываем сырую ошибку чтения.
		dialog.ShowInformation(locale.T("servers.power.deploy_title"),
			locale.Tf("servers.power.deploy_missing", path), c.ac.UIService.MainWindow)
		return
	}

	dialog.ShowConfirm(
		locale.T("servers.power.deploy_title"),
		locale.Tf("servers.power.deploy_body", name, len(config)),
		func(ok bool) {
			if !ok {
				return
			}
			c.pending = true
			c.deploy.Disable()
			c.button.Disable()
			c.status.SetText(locale.T("servers.power.deploying"))
			c.status.Refresh()
			go func() {
				err := services.NewRemoteRegistry(c.ac.FileService.ExecDir).ApplyConfig(id, config)
				fyne.Do(func() {
					c.pending = false
					c.deploy.Enable()
					if err != nil {
						debuglog.WarnLog("remote deploy: %v", err)
						dialog.ShowError(err, c.ac.UIService.MainWindow)
					} else {
						dialog.ShowInformation(locale.T("servers.power.deploy_title"),
							locale.Tf("servers.power.deploy_done", name), c.ac.UIService.MainWindow)
					}
					c.refresh()
					if err == nil && onChanged != nil {
						onChanged()
					}
				})
			}()
		}, c.ac.UIService.MainWindow)
}
