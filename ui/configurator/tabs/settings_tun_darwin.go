//go:build darwin

package tabs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	settingsTunOffCoreRunningText     = "Stop the sing-box core first (Core tab -> Stop), then turn off TUN.\n\nAfter TUN ran with administrator rights, the experimental cache under bin/ and core log files (logs/sing-box.log) may be owned by root. They are removed automatically when you turn TUN off, but only while the core is stopped."
	settingsTunOffSingboxOnSystemText = "sing-box is still running on the system (detected PID %d). Use Core -> Stop (enter your password if asked) until the process exits, or end it in Activity Monitor, then turn off TUN.\n\nIf Stop was cancelled earlier, the launcher could think the core was stopped while sing-box was still running — that leaves the TUN interface and ports (e.g. mixed proxy) busy."
)

// pathUnderRoot returns true if target is inside root (after Clean), not escaping with "..".
func pathUnderRoot(root, target string) bool {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// removeTunLeftover удаляет target своим uid (SPEC 137.1): target лежит под
// root лексически и после разрешения симлинков родителя, сам не симлинк.
func removeTunLeftover(root, target string) error {
	if !pathUnderRoot(root, target) {
		return fmt.Errorf("%s is outside %s, left in place", target, root)
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fmt.Errorf("%s: %v", root, err)
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return fmt.Errorf("%s: %v", filepath.Dir(target), err)
	}
	if !pathUnderRoot(realRoot, realParent) {
		return fmt.Errorf("%s resolves outside %s, left in place", target, root)
	}
	fi, err := os.Lstat(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("%s: %v", target, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symbolic link, left in place", target)
	}
	if err := os.RemoveAll(target); err != nil {
		return fmt.Errorf("cannot remove %s: %v", target, err)
	}
	return nil
}

// maybeTunOffDarwin при снятии TUN на macOS: не даёт выключить, пока ядро запущено;
// после остановки удаляет experimental.cache_file.path под bin/ (если есть),
// а также логи ядра logs/sing-box.log и logs/sing-box.log.old, если существуют
// (ядро под root могло оставить их root-owned).
//
// SPEC 137.1: без root. Файлы лежат в каталогах пользователя, а право удаления
// даёт каталог, а не владелец файла — root-owned остатки лаунчер удаляет своим
// uid (removeTunLeftover). Интерфейс и маршруты ядро снимает само при выходе:
// привилегированных вызовов здесь нет.
// Возвращает true, если снятие галки отменено (чекбокс возвращён в true).
func maybeTunOffDarwin(presenter *wizardpresentation.WizardPresenter, model *wizardmodels.WizardModel, td *wizardtemplate.TemplateData, varName string, chk *widget.Check) bool {
	if varName != "tun" || presenter == nil || model == nil || td == nil || chk == nil {
		return false
	}

	st := model.SettingsVars
	vars := td.Vars
	raw := td.RawTemplate
	v, overridden := model.SettingsVars[varName]
	prevTrue := strings.TrimSpace(wizardtemplate.DisplaySettingValueFor(vars, st, raw, varName, model.Target.Normalized())) == "true"
	if overridden {
		prevTrue = v == "true"
	}
	if !prevTrue {
		return false
	}

	ac := presenter.Controller()
	if ac == nil || ac.RunningState == nil {
		return false
	}
	if ac.RunningState.IsRunning() {
		dialog.ShowError(errors.New(locale.T(settingsTunOffCoreRunningText)), presenter.DialogParent())
		chk.SetChecked(true)
		return true
	}

	// RunningState may be false while sing-box still runs (e.g. privileged Stop failed or was cancelled).
	if ac.ProcessService != nil {
		if alive, pid := ac.ProcessService.IsSingBoxProcessRunningOnSystem(); alive {
			dialog.ShowError(fmt.Errorf("%s", locale.Tf(settingsTunOffSingboxOnSystemText, pid)), presenter.DialogParent())
			chk.SetChecked(true)
			return true
		}
	}

	if ac.FileService == nil {
		return false
	}

	var targets []string
	binDir := filepath.Clean(ac.FileService.Layout.Data.Bin())
	logsDir := filepath.Clean(string(ac.FileService.Layout.Logs))

	expRaw, expOK, expErr := wizardbusiness.EffectiveConfigSection(model, "experimental")
	if expErr != nil {
		debuglog.WarnLog("maybeTunOffDarwin: EffectiveConfigSection: %v", expErr)
	}
	if expErr == nil && expOK {
		if shouldRm, relPath := config.ExperimentalCacheFileFromSection(expRaw); shouldRm && relPath != "" {
			var cacheAbs string
			if filepath.IsAbs(relPath) {
				cacheAbs = filepath.Clean(relPath)
			} else {
				cacheAbs = filepath.Join(binDir, filepath.Clean(relPath))
			}
			if pathUnderRoot(binDir, cacheAbs) {
				if _, err := os.Lstat(cacheAbs); err == nil {
					targets = append(targets, cacheAbs)
				}
			} else {
				debuglog.WarnLog("maybeTunOffDarwin: cache path outside bin, skip: %q", cacheAbs)
			}
		}
	}

	logPath := ac.FileService.ChildLogPath
	var removedCoreLogs bool
	for _, p := range []string{logPath, logPath + ".old"} {
		if !pathUnderRoot(logsDir, p) {
			continue
		}
		if _, err := os.Lstat(p); err == nil {
			targets = append(targets, p)
			removedCoreLogs = true
		}
	}

	if len(targets) == 0 {
		return false
	}

	var failed []string
	for _, p := range targets {
		root := binDir
		if pathUnderRoot(logsDir, p) {
			root = logsDir
		}
		if err := removeTunLeftover(root, p); err != nil {
			debuglog.WarnLog("maybeTunOffDarwin: %v", err)
			failed = append(failed, err.Error())
			continue
		}
		debuglog.InfoLog("maybeTunOffDarwin: removed %s", p)
	}
	if len(failed) > 0 {
		dialog.ShowError(errors.New(strings.Join(failed, "\n")), presenter.DialogParent())
		return false
	}
	if removedCoreLogs {
		if rerr := ac.FileService.ReopenChildLogFile(); rerr != nil {
			debuglog.WarnLog("maybeTunOffDarwin: ReopenChildLogFile: %v", rerr)
		}
	}
	return false
}
