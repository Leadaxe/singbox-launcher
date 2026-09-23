package core

import (
	"errors"
	"os"
	"runtime"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
)

// PortableToggleAvailable — показывается ли переключатель Portable вообще
// (SPEC 135 §4.2): на macOS его нет ни для .app (режима нет), ни для голого
// бинаря (он и так portable).
func PortableToggleAvailable() bool {
	return runtime.GOOS != "darwin"
}

// PortableToggleState — состояние чекбокса Portable в разделе Storage:
// отмечен в режимах Portable и Legacy; недоступен (enabled=false) с причиной
// для подсказки, когда раскладку задают переменные окружения, каталог
// программы не пишется или ядро запущено.
func (ac *AppController) PortableToggleState() (checked bool, enabled bool, reason string) {
	if ac == nil || ac.FileService == nil {
		return false, false, ""
	}
	l := ac.FileService.Layout
	checked = l.Mode == paths.ModePortable || l.Mode == paths.ModeLegacy
	if err := ac.portableSwitchBlocker(l); err != nil {
		return checked, false, err.Error()
	}
	return checked, true, ""
}

// portableSwitchBlocker — почему переключать нельзя прямо сейчас; nil — можно.
// Текст ошибки локализован: он же причина в подсказке чекбокса.
func (ac *AppController) portableSwitchBlocker(l paths.Layout) error {
	switch {
	case !PortableToggleAvailable():
		// UI прячет переключатель на macOS; сюда доходит только прямой вызов.
		return errors.New("portable mode switch is not available on this platform")
	case l.Mode == paths.ModeEnv:
		return errors.New(locale.T("Paths are set by environment variables"))
	case !paths.ProbeWritable(string(l.App)):
		return errors.New(locale.T("The program folder is read-only"))
	case ac.RunningState != nil && ac.RunningState.IsRunning():
		return errors.New(locale.T("Stop the VPN first"))
	}
	return nil
}

// PortableSwitchTarget — куда уедут данные при переключении: для включения —
// AppDir/bin, для выключения — bin в системном DataDir (§3.1).
func (ac *AppController) PortableSwitchTarget(on bool) (string, error) {
	if ac == nil || ac.FileService == nil {
		return "", errors.New("file service is not initialized")
	}
	l := ac.FileService.Layout
	if on {
		return l.App.Bin(), nil
	}
	target, err := systemDefaultLayout()
	if err != nil {
		return "", err
	}
	return target.Data.Bin(), nil
}

// systemDefaultLayout — системная раскладка для текущего бинаря.
func systemDefaultLayout() (paths.Layout, error) {
	exe, err := paths.Executable()
	if err != nil {
		return paths.Layout{}, err
	}
	return paths.SystemDefault(exe, os.Getenv, runtime.GOOS, paths.ProbeWritable)
}

// SwitchPortable переносит данные между AppDir и системным DataDir (SPEC 135
// §4.2). Условия проверяются заново: между показом диалога и подтверждением
// пользователь мог запустить ядро. Раскладка процесса не меняется — её
// применит перезапуск, который делает вызывающий.
func (ac *AppController) SwitchPortable(on bool) (paths.SwitchReport, error) {
	if ac == nil || ac.FileService == nil {
		return paths.SwitchReport{}, errors.New("file service is not initialized")
	}
	l := ac.FileService.Layout
	if err := ac.portableSwitchBlocker(l); err != nil {
		return paths.SwitchReport{}, err
	}

	var (
		rep paths.SwitchReport
		err error
	)
	if on {
		rep, err = paths.SwitchToPortable(l)
	} else {
		var target paths.Layout
		target, err = systemDefaultLayout()
		if err == nil {
			rep, err = paths.SwitchToSystem(l, target)
		}
	}
	if err != nil {
		debuglog.ErrorLog("storage: portable switch (on=%v) failed: %v; %s", on, err, rep.Summary())
		return rep, err
	}
	debuglog.WarnLog("storage: %s", rep.Summary())
	return rep, nil
}
