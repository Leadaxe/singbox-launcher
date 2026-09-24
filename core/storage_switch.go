package core

import (
	"errors"
	"os"
	"runtime"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
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
// программы не пишется пользователем (SPEC 139 §7), экземпляр повышен
// через UAC или ядро запущено.
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
	case platform.ElevatedViaUAC():
		// SPEC 139 §6 п. 8: повышенный через UAC экземпляр пишет туда, куда
		// обычный не может, — переезд данных только при обычном запуске. Без
		// UAC (выключен, встроенный Administrator) обычного запуска нет, и
		// решает предикат ниже.
		return errors.New(locale.T("Change this in a normal start, not as administrator"))
	case !paths.AppDirUserWritable(string(l.App), os.Getenv, runtime.GOOS, paths.ProbeWritable):
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

// StorageLeftover — остаток прошлого переезда из settings.json
// (storage_leftover), если путь ещё существует; иначе "".
func (ac *AppController) StorageLeftover() string {
	if ac == nil || ac.FileService == nil {
		return ""
	}
	p := locale.LoadSettings(ac.FileService.Layout.Data.Bin()).StorageLeftover
	if p == "" {
		return ""
	}
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return p
}

// IsStorageSwitching — идёт переезд данных переключателем Portable: старт
// ядра и автообновление подписок в это время отказывают.
func (ac *AppController) IsStorageSwitching() bool {
	return ac != nil && ac.storageSwitching.Load()
}

// SwitchPortable переносит данные между AppDir и системным DataDir (SPEC 135
// §4.2) и сразу же перезапускает лаунчер (RequestRestartAfterExit +
// GracefulExit): раскладка процесса старая, и любая запись после переезда
// ушла бы на старое место. Условия проверяются заново: между показом
// диалога и подтверждением пользователь мог запустить ядро.
//
// На время копирования поднят storageSwitching: старт ядра и автообновление
// подписок отказывают. При ошибке флаг снимается, раскладки целы.
//
// Итог и остаток (Leftover) пишутся в лог, остаток — ещё и в settings.json
// нового места (storage_leftover): раздел Storage покажет его после
// перезапуска, очистка удалит.
//
// Выключение, когда в системном каталоге уже лежат данные, а в AppDir/bin
// их нет (paths.ErrTargetHasData), — не ошибка: переезжать нечему, удаляется
// только portable.txt, и перезапуск подхватит найденные данные.
func (ac *AppController) SwitchPortable(on bool) (paths.SwitchReport, error) {
	if ac == nil || ac.FileService == nil {
		return paths.SwitchReport{}, errors.New("file service is not initialized")
	}
	l := ac.FileService.Layout
	if err := ac.portableSwitchBlocker(l); err != nil {
		return paths.SwitchReport{}, err
	}
	if !ac.storageSwitching.CompareAndSwap(false, true) {
		return paths.SwitchReport{}, errors.New(locale.T("Data move in progress"))
	}

	var (
		rep    paths.SwitchReport
		err    error
		target paths.Layout
	)
	if on {
		rep, err = paths.SwitchToPortable(l)
	} else {
		target, err = systemDefaultLayout()
		if err == nil {
			rep, err = paths.SwitchToSystem(l, target)
		}
	}
	if !on && errors.Is(err, paths.ErrTargetHasData) {
		if rmErr := paths.RemovePortableMarker(l.App); rmErr != nil {
			ac.storageSwitching.Store(false)
			debuglog.ErrorLog("storage: %v", rmErr)
			return rep, rmErr
		}
		debuglog.WarnLog("storage: %s already holds settings; portable.txt removed instead of moving", target.Data)
		ac.restartForNewLayout()
		return rep, nil
	}
	if err != nil {
		ac.storageSwitching.Store(false)
		debuglog.ErrorLog("storage: portable switch (on=%v) failed: %v; %s", on, err, rep.Summary())
		return rep, err
	}
	debuglog.WarnLog("storage: %s", rep.Summary())
	if rep.Leftover != "" {
		debuglog.WarnLog("storage: left over from the move (shown in Storage, removed by Remove all data): %s", rep.Leftover)
	}
	// Всегда: пустая строка стирает запись, приехавшую в settings.json
	// вместе с данными от прошлого переезда.
	if err := locale.MarkStorageLeftover(rep.To, rep.Leftover); err != nil {
		debuglog.WarnLog("storage: persist leftover: %v", err)
	}
	ac.restartForNewLayout()
	return rep, nil
}

// restartForNewLayout — перезапуск тем же путём, что у переключения Mesa:
// RequestRestartAfterExit + GracefulExit, сам RestartSelf — в конце main(),
// когда ядро остановлено и логи закрыты.
func (ac *AppController) restartForNewLayout() {
	debuglog.WarnLog("storage: restarting to apply the new data layout")
	platform.RequestRestartAfterExit()
	ac.GracefulExit()
}
