//go:build darwin

package tabs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
	wizardtemplate "singbox-launcher/core/template"
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

// maybeTunOffDarwin при снятии TUN на macOS: не даёт выключить, пока ядро запущено;
// после остановки — привилегированно удаляет experimental.cache_file.path под bin/ (если есть),
// а также логи ядра logs/sing-box.log и logs/sing-box.log.old, если существуют (после TUN под root они могут быть недоступны обычному процессу).
// Возвращает true, если снятие галки отменено (чекбокс возвращён в true).
func maybeTunOffDarwin(presenter *wizardpresentation.WizardPresenter, model *wizardmodels.WizardModel, td *wizardtemplate.TemplateData, varName string, chk *widget.Check) bool {
	if varName != "tun" || presenter == nil || model == nil || td == nil || chk == nil {
		return false
	}

	st := model.SettingsVars
	vars := td.Vars
	raw := td.RawTemplate
	v, overridden := model.SettingsVars[varName]
	prevTrue := strings.TrimSpace(wizardtemplate.DisplaySettingValue(vars, st, raw, varName)) == "true"
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
		dialog.ShowError(errors.New(locale.T("wizard.settings.tun_off_core_running")), presenter.DialogParent())
		chk.SetChecked(true)
		return true
	}

	// RunningState may be false while sing-box still runs (e.g. privileged Stop failed or was cancelled).
	if ac.ProcessService != nil {
		if alive, pid := ac.ProcessService.IsSingBoxProcessRunningOnSystem(); alive {
			dialog.ShowError(fmt.Errorf("%s", locale.Tf("wizard.settings.tun_off_singbox_on_system", pid)), presenter.DialogParent())
			chk.SetChecked(true)
			return true
		}
	}

	if ac.FileService == nil {
		return false
	}

	var targets []string
	binDir := filepath.Clean(platform.GetBinDir(ac.FileService.ExecDir))
	execDir := filepath.Clean(ac.FileService.ExecDir)

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

	logPath := filepath.Join(execDir, constants.LogsDirName, constants.ChildLogFileName)
	var removedCoreLogs bool
	for _, p := range []string{logPath, logPath + ".old"} {
		if !pathUnderRoot(execDir, p) {
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

	var quoted []string
	for _, p := range targets {
		quoted = append(quoted, strconv.Quote(p))
	}
	shell := "rm -rf " + strings.Join(quoted, " ")
	_, _, err := platform.RunWithPrivileges("/bin/sh", []string{"-c", shell})
	if err != nil {
		debuglog.WarnLog("maybeTunOffDarwin: privileged rm: %v", err)
		dialog.ShowError(err, presenter.DialogParent())
		return false
	}
	if removedCoreLogs {
		if rerr := ac.FileService.ReopenChildLogFile(); rerr != nil {
			debuglog.WarnLog("maybeTunOffDarwin: ReopenChildLogFile: %v", rerr)
		}
	}
	return false
}
