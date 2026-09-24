package core

// Права по требованию на Windows (SPEC 139). Лаунчер собран с манифестом
// asInvoker: proxy-only работает без прав, TUN — только после явного
// перезапуска с повышением. Здесь гейт TUN в ProcessService.Start, диалог
// «TUN без прав», перезапуск от имени администратора (новый экземпляр
// первым, старый выходит после успеха ShellExecuteExW), переход в режим
// прокси и сообщение о ядре, которое без прав не снять.

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"

	"fyne.io/fyne/v2"

	"singbox-launcher/core/config"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
)

// Флаги командной строки, которые собирает перезапуск (main их объявляет).
const (
	// HandoffFlagName — раскладка родителя для повышенного экземпляра:
	// -handoff=<PID>|<Mode>|<DataDir>|<LogDir> (paths.Layout.Handoff).
	HandoffFlagName = "handoff"
	// AutostartFlagName — -autostart=on|off для установщика (SPEC 139 §8).
	AutostartFlagName = "autostart"
	startFlagName     = "start"
	trayFlagName      = "tray"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	tunNeedsAdminText    = "TUN mode creates a network adapter and changes routes, which Windows allows only to administrators. The launcher is running without administrator rights."
	proxyModeNoRightText = "Proxy mode needs no rights: programs that use the system proxy (browsers and most apps) go through the local proxy on port %s, the rest connect directly."
	otherAccountText     = "Windows will ask for an administrator account, and the launcher will run under that account."
	killNeedsAdminText   = "sing-box was started with administrator rights, and this launcher cannot stop it without them. Restart the launcher as administrator to stop it."
)

// Переменные шаблона, которые переключает «Switch to proxy mode» (SPEC 139 §4).
const (
	varTun                = "tun"
	varEnableProxyIn      = "enable_proxy_in"
	varProxyInSystemProxy = "proxy_in_set_system_proxy"
	varProxyInListenPort  = "proxy_in_listen_port"
	// defaultProxyInPort — порт proxy-in, если его нет ни в state, ни в шаблоне.
	defaultProxyInPort = "7890"
)

// windowsNotElevated — Windows и процесс без прав администратора: всё, что
// требует админа, пропускается или ведёт в диалог (SPEC 139 §6). Вне
// Windows гейтов нет.
func windowsNotElevated() bool {
	return runtime.GOOS == "windows" && !platform.IsElevated()
}

// logSkippedAdminCleanups — одна строка INFO о пропущенных без прав
// очистках старта (SPEC 139 §2 п. 4, §6 п. 2–3) вместо WARN на каждое место.
func logSkippedAdminCleanups() {
	debuglog.InfoLog("elevation: not elevated, skipped: %s; they run on the next start as administrator",
		strings.Join(platform.AdminCleanupTasks(), ", "))
}

// tunNeedsElevation — гейт TUN (SPEC 139 §4): Windows, процесс без прав, в
// собранном config.json есть TUN. Ошибка чтения конфига — как на darwin:
// WARN и «TUN нет» (ядро само сообщит о проблеме).
func (ac *AppController) tunNeedsElevation() bool {
	if !windowsNotElevated() {
		return false
	}
	hasTun, err := config.ConfigHasTun(ac.FileService.ConfigPath)
	if err != nil {
		debuglog.WarnLog("startSingBox: could not check TUN in config: %v; assuming no TUN", err)
		return false
	}
	return hasTun
}

// showTunElevationDialog — диалог «TUN без прав» (SPEC 139 §4) вместо старта
// ядра. Окно скрыто (трей, -tray) — сначала показывается. Без UI — WARN.
//
// Порядок кнопок: [Install service — SPEC 141, Windows x64/arm64] ·
// Restart as administrator · Switch to proxy mode · Cancel.
func (ac *AppController) showTunElevationDialog() {
	if !ac.hasUI() {
		debuglog.WarnLog("startSingBox: TUN needs administrator rights and the launcher is not elevated; sing-box not started")
		return
	}
	debuglog.WarnLog("startSingBox: TUN needs administrator rights; showing the elevation dialog")

	message := locale.T(tunNeedsAdminText) + "\n\n" + locale.Tf(proxyModeNoRightText, ac.proxyInListenPort())
	if platform.ElevationAsksOtherAccount() {
		message += "\n\n" + locale.T(otherAccountText)
	}

	actions := []dialogs.Action{
		{
			Label:     locale.T("Restart as administrator"),
			Important: true,
			Run:       func(d *dialogs.ActionsDialog) { ac.runRestartAsAdministrator(d, true) },
		},
		ac.switchToProxyAction(),
	}
	// SPEC 141 §9: Install service — первой (служба ставится одним окном UAC,
	// лаунчер переходит в daemon-режим и стартует без прав).
	if install, ok := ac.tunInstallServiceAction(); ok {
		actions = append([]dialogs.Action{install}, actions...)
	}

	ac.UIService.ShowMainWindowOrFocusWizard()
	win := ac.UIService.MainWindow
	fyne.Do(win.RequestFocus)
	dialogs.ShowActions(win, locale.T("TUN needs administrator rights"), message, actions, locale.T("Cancel"))
}

// switchToProxyAction — кнопка «Switch to proxy mode». При открытом
// конфигураторе недоступна: его Save перезаписал бы state и вернул tun=true.
func (ac *AppController) switchToProxyAction() dialogs.Action {
	act := dialogs.Action{Label: locale.T("Switch to proxy mode")}
	if ac.UIService.WizardWindow != nil {
		act.Disabled = true
		act.Hint = locale.T("Close the configurator first")
		return act
	}
	act.Run = func(d *dialogs.ActionsDialog) {
		// Конфигуратор могли открыть, пока диалог висел.
		if ac.UIService.WizardWindow != nil {
			d.SetStatus(locale.T("Close the configurator first"))
			return
		}
		d.Hide()
		go func() {
			if err := ac.SwitchToProxyMode(); err != nil {
				debuglog.ErrorLog("switch to proxy mode: %v", err)
				ac.ShowRebuildError(err)
			}
		}()
	}
	return act
}

// runRestartAsAdministrator — нажатие «Restart as administrator»: запрос UAC
// в отдельной горутине (UI не блокируется). Отмена UAC — строка в диалоге,
// иная ошибка — текст в диалоге; успех — выход этого экземпляра.
func (ac *AppController) runRestartAsAdministrator(d *dialogs.ActionsDialog, withStart bool) {
	d.SetBusy()
	go func() {
		err := ac.RestartAsAdministrator(withStart)
		switch {
		case err == nil:
			d.Hide()
		case errors.Is(err, platform.ErrElevationCancelled):
			debuglog.InfoLog("restart as administrator: the UAC prompt was cancelled")
			d.SetStatus(locale.T("The administrator prompt was cancelled."))
		default:
			debuglog.WarnLog("restart as administrator: %v", err)
			d.SetStatus(locale.Tf("Could not restart as administrator: %s", err.Error()))
		}
	}()
}

// RestartAsAdministrator запускает новый экземпляр лаунчера с повышением и
// после успеха завершает этот (SPEC 139 §5): сначала новый, потом выход
// старого — отказ в UAC ничего не ломает. Новый получает раскладку этого
// экземпляра флагом -handoff и ждёт его выхода, прежде чем трогать логи,
// порт Debug API и трей. withStart — пользователь нажимал Start: новый
// экземпляр сразу поднимет VPN. Блокирует до ответа UAC — звать не из
// UI-потока.
func (ac *AppController) RestartAsAdministrator(withStart bool) error {
	exe, err := paths.Executable()
	if err != nil {
		return fmt.Errorf("executable path: %w", err)
	}
	args := restartArgs(ac.FileService.Layout, withStart)
	debuglog.InfoLog("restart as administrator: asking for elevation (start=%v)", withStart)
	p, err := platform.RunElevated(exe, args, string(ac.FileService.Layout.App), platform.ElevatedShowNormal)
	if err != nil {
		return err
	}
	debuglog.WarnLog("restart as administrator: started pid %d, exiting", p.Pid)
	if err := p.Close(); err != nil {
		debuglog.DebugLog("restart as administrator: close process handle: %v", err)
	}
	// Как Quit в трее: GracefulExit на UI-потоке. RequestRestartAfterExit не
	// взводится — новый экземпляр уже запущен.
	fyne.Do(ac.GracefulExit)
	return nil
}

// restartArgs — аргументы нового экземпляра из разобранных флагов
// (flag.Visit), а не из сырой строки: всё заданное, кроме -tray (действие
// идёт из открытого окна), прежних -start и -handoff; плюс -start, если
// пользователь нажимал Start, и -handoff с раскладкой и PID этого процесса.
func restartArgs(l paths.Layout, withStart bool) []string {
	var args []string
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case trayFlagName, startFlagName, HandoffFlagName:
			return
		}
		args = append(args, "-"+f.Name+"="+f.Value.String())
	})
	if withStart {
		args = append(args, "-"+startFlagName)
	}
	return append(args, "-"+HandoffFlagName+"="+l.Handoff(os.Getpid()))
}

// SwitchToProxyMode — «Switch to proxy mode» (SPEC 139 §4): в state
// локального профиля tun=false, proxy-in и системный прокси включены →
// Save → принудительная пересборка config.json → Start. Режиму прокси права
// не нужны: системный прокси пользователя пишется в HKCU.
func (ac *AppController) SwitchToProxyMode() error {
	debuglog.InfoLog("switch to proxy mode: tun=false, proxy-in with system proxy")
	if err := setLocalStateVars(ac, []state.SettingVar{
		{Name: varTun, Value: "false"},
		{Name: varEnableProxyIn, Value: "true"},
		{Name: varProxyInSystemProxy, Value: "true"},
	}); err != nil {
		return err
	}
	if err := ac.RebuildConfigIfDirty(true); err != nil {
		return fmt.Errorf("rebuild config: %w", err)
	}
	debuglog.WarnLog("switch to proxy mode: config rebuilt without TUN, starting sing-box")
	StartSingBoxProcess()
	return nil
}

// proxyInListenPort — порт proxy-in так, как его видит сборщик: значение из
// state, иначе дефолт шаблона, иначе 7890.
func (ac *AppController) proxyInListenPort() string {
	if s, err := state.Load(platform.GetWizardStatePath(ac.FileService.Layout.Data)); err == nil {
		for _, v := range s.Vars {
			if v.Name == varProxyInListenPort && strings.TrimSpace(v.Value) != "" {
				return strings.TrimSpace(v.Value)
			}
		}
	}
	if td, err := template.LoadTemplateData(ac.FileService.Layout); err == nil && td != nil {
		for _, v := range td.Vars {
			if v.Name == varProxyInListenPort {
				if def := v.DefaultValue.ForTarget(template.LocalTarget()); def != "" {
					return def
				}
			}
		}
	}
	return defaultProxyInPort
}

// KillNeedsElevation — после неудачного taskkill (SPEC 139 §6 п. 7): процесс
// ядра жив, а лаунчер без прав — значит, ядро запущено повышенным
// экземпляром, и снять его может только администратор.
func (ac *AppController) KillNeedsElevation(killErr error) bool {
	if killErr == nil || !windowsNotElevated() || ac.ProcessService == nil {
		return false
	}
	found, pid := ac.ProcessService.isSingBoxProcessRunning()
	if found {
		debuglog.WarnLog("kill sing-box: pid %d survived taskkill (%v); it runs with administrator rights", pid, killErr)
	}
	return found
}

// ShowKillNeedsElevation — сообщение «ядро запущено с правами» с кнопкой
// Restart as administrator: повышенный экземпляр снимет ядро через
// «already running» → Kill.
func (ac *AppController) ShowKillNeedsElevation() {
	if !ac.hasUI() {
		return
	}
	actions := []dialogs.Action{{
		Label:     locale.T("Restart as administrator"),
		Important: true,
		Run:       func(d *dialogs.ActionsDialog) { ac.runRestartAsAdministrator(d, false) },
	}}
	dialogs.ShowActions(ac.UIService.MainWindow, locale.T("Warning"), locale.T(killNeedsAdminText), actions, locale.T("Close"))
}

// setLocalStateVars записывает переменные в state.json локального профиля
// (load → правка vars → Save; новые — в конец списка). Пересборку делает
// вызывающий.
func setLocalStateVars(ac *AppController, vars []state.SettingVar) error {
	if ac == nil || ac.FileService == nil {
		return errors.New("core: no controller")
	}
	statePath := platform.GetWizardStatePath(ac.FileService.Layout.Data)
	s, err := state.Load(statePath)
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}
	for _, v := range vars {
		found := false
		for i := range s.Vars {
			if s.Vars[i].Name == v.Name {
				s.Vars[i].Value = v.Value
				found = true
				break
			}
		}
		if !found {
			s.Vars = append(s.Vars, v)
		}
	}
	if err := s.Save(statePath); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	return nil
}
