package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"singbox-launcher/core/template"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
)

// templateRefreshTimeout bounds the startup template refresh. The first core
// start waits for it (awaitTemplateRefresh), so on a dead or throttled link
// this is the longest a start can be held back — and only on launches of a new
// launcher version until one download succeeds.
const templateRefreshTimeout = 15 * time.Second

// TemplateRefreshResult reports what RefreshTemplateIfStale did.
type TemplateRefreshResult struct {
	// RebuildConfig — config.json on disk must not be reused: the next core
	// start rebuilds it from state. Set when this is the first launch of a
	// launcher version that did not build it (whenever state.json exists,
	// whatever happened to the download), and when the data root changed
	// since the last build (SPEC 135 §3.5: config.json holds absolute paths).
	RebuildConfig bool
	// Downloaded — the template pinned for this version was fetched and now
	// sits in Data/bin (replacing the stale file or restoring a missing one).
	Downloaded bool
}

// RefreshTemplateIfStale brings the wizard template up to the one pinned for
// this launcher version when the file in use was installed by an older one
// (SPEC 046: the template format shifts between versions).
//
// Two folders (SPEC 135 §3.3): the shipped template and its marker live in
// App/bin, the downloaded one and the stamp in Data/bin. Which file is read
// is decided by template.ResolveTemplate; this function only reads the
// marker from App and writes (stamp, download) into Data. In portable/legacy
// layouts App == Data and the behaviour is the one from before SPEC 135.
//
// The stale template is REPLACED, never deleted: it stays in place until the
// new one is downloaded and parsed (template.DownloadTemplate swaps the file
// atomically). Earlier this function removed the file on startup and the
// launcher lived without a template until someone pressed Download: the
// pre-start rebuild failed with "no such file", and the core silently came up
// on the old config.json.
//
// Marker (LastTemplateLauncherVersion in settings.json), at most one
// successful refresh per version:
//   - download succeeded → stamped by DownloadTemplate;
//   - download failed → NOT stamped: nothing was lost (the old file is still
//     there), so the next launch simply tries again;
//   - no template and no state.json (a pristine install that never downloaded
//     one) → stamped without touching the network: the Download button on the
//     Local tab stays the way in, as before;
//   - template bundled by the installer for exactly this version (resolver
//     picked the shipped one with a current marker) → kept and stamped, no
//     network. A downloaded copy left in Data by the previous version is
//     removed: it is superseded by the shipped one, and after the stamp the
//     resolver would otherwise go back to it.
//
// A template the user placed by hand survives launches on the same version
// (the stamp short-circuits the check); on the next upgrade it is replaced by
// the pinned one — never removed without a replacement.
//
// Skipped for dev builds: AppVersion of the form "v-local-test" or
// "unnamed-dev" doesn't compare meaningfully against semver, so the policy
// would either always or never fire. Both are annoying during inner-loop
// development; we leave the local template alone in those cases. The data
// root check (config_data_root) runs on dev builds too: it has nothing to do
// with versions.
//
// NETWORK: call off the UI thread. l and fetch are parameters so tests
// run without an AppController and without a network.
func RefreshTemplateIfStale(ctx context.Context, l paths.Layout, fetch template.URLFetcher) (TemplateRefreshResult, error) {
	var res TemplateRefreshResult
	binDir := l.Data.Bin()
	settings := locale.LoadSettings(binDir)

	// SPEC 135 §3.5: config.json holds absolute paths (.srs, tailscale) under
	// the data root it was built with. Moved root → those paths lie.
	if old := settings.ConfigDataRoot; old != "" && old != configDataRoot(l) {
		debuglog.WarnLog("config: data root changed %s -> %s, rebuilding config.json", old, configDataRoot(l))
		res.RebuildConfig = true
	}

	if isDevAppVersion(constants.AppVersion) {
		debuglog.DebugLog("template: skipping stale-check on dev build %q", constants.AppVersion)
		return res, nil
	}

	last := settings.LastTemplateLauncherVersion
	if last != "" && CompareVersions(last, constants.AppVersion) >= 0 {
		// Same launcher (or downgrade — leave the file, user knows what
		// they're doing).
		return res, nil
	}

	// config.json on disk was built by another launcher version, maybe from
	// another template: whatever happens below, the first start rebuilds it.
	_, stateErr := os.Stat(platform.GetWizardStatePath(l.Data))
	hasState := stateErr == nil
	res.RebuildConfig = res.RebuildConfig || hasState

	templatePath := platform.GetWizardTemplatePath(l.Data)
	_, statErr := os.Stat(templatePath)

	// Шаблон, положенный установщиком под ЭТУ версию (архив win64-full,
	// install-macos.sh), несёт рядом маркер с версией лаунчера. Совпал — шаблон
	// свежий, и перекачивать тот же файл незачем: весь смысл вложения в том,
	// чтобы первый запуск не ходил в сеть. Маркер только читается; штамп в
	// settings ставится, как при любом другом исходе без скачивания.
	if keptShippedTemplate(l, statErr == nil) {
		debuglog.InfoLog("template: bundled by the installer for %q — kept", constants.AppVersion)
		stampTemplateCheck(binDir)
		return res, nil
	}

	switch {
	case statErr == nil:
		debuglog.InfoLog("template: installed by %q, launcher is %q — downloading the pinned template; the current file stays until it arrives",
			last, constants.AppVersion)
	case os.IsNotExist(statErr):
		if !hasState {
			// Nothing on disk builds from a template — leave the install
			// as the user has it, but record the check so it doesn't repeat.
			debuglog.DebugLog("template: no file and no state; recording stale-check for %q", constants.AppVersion)
			stampTemplateCheck(binDir)
			return res, nil
		}
		// state.json builds config.json from a template that is gone (an
		// older launcher deleted it on upgrade): fetch it back.
		debuglog.InfoLog("template: missing while state.json needs it — downloading the pinned template for %q", constants.AppVersion)
	default:
		return res, fmt.Errorf("template refresh: stat %s: %w", templatePath, statErr)
	}

	if _, err := template.DownloadTemplate(ctx, l.Data, fetch); err != nil {
		return res, fmt.Errorf("template refresh: %w", err)
	}
	res.Downloaded = true
	return res, nil
}

// keptShippedTemplate — ветка «положен установщиком под эту версию»: шаблон
// оставляется как есть, сеть не нужна. dataExists — есть ли Data/bin/…json.
//
// Маркер читается из App. Нет маркера в App — из Data: у унаследованных
// установок и после миграции маркер лежит рядом со скачанным шаблоном (в
// portable это один и тот же файл).
//
// Поставляемый выбран резолвером, а в Data лежит копия прошлой версии — она
// удаляется: штамп сейчас станет равен AppVersion, и по правилу резолвера
// следующее чтение вернулось бы к ней. Не удалилась — false: штамп не
// ставится, поставляемый продолжает побеждать, проверка повторится.
func keptShippedTemplate(l paths.Layout, dataExists bool) bool {
	tr := template.ResolveTemplate(l)
	dataPath := platform.GetWizardTemplatePath(l.Data)
	if tr.Source == template.TemplateSourceApp && tr.ShippedCurrent {
		if dataExists && filepath.Clean(tr.Path) != filepath.Clean(dataPath) {
			if err := os.Remove(dataPath); err != nil && !os.IsNotExist(err) {
				debuglog.WarnLog("template: cannot remove %s superseded by the shipped template: %v", dataPath, err)
				return false
			}
			debuglog.InfoLog("template: removed %s — superseded by the shipped %s", dataPath, tr.Path)
		}
		return true
	}
	if !dataExists {
		return false
	}
	if l.App != "" {
		if _, ok := template.ReadTemplateMarker(l.App.Bin()); ok {
			return false // маркер App есть и не совпал — решил резолвер
		}
	}
	marker, ok := template.ReadTemplateMarker(l.Data.Bin())
	return ok && marker == constants.AppVersion
}

// configDataRoot — значение штампа config_data_root для раскладки l.
func configDataRoot(l paths.Layout) string {
	return filepath.Clean(string(l.Data))
}

// stampConfigDataRoot записывает DataDir, с которым только что собран
// config.json (SPEC 135 §3.5). Best effort: не записалось — на следующем
// старте сравнение просто не сработает или сработает лишний раз.
func stampConfigDataRoot(l paths.Layout) {
	if err := locale.MarkConfigDataRoot(l.Data.Bin(), configDataRoot(l)); err != nil {
		debuglog.WarnLog("config: failed to record data root: %v", err)
	}
}

// stampTemplateCheck records that this launcher version has run its template
// check. Best effort: if persisting fails, the check just runs again next
// launch — the same cosmetic nuisance MarkTemplateInstalled already tolerates.
func stampTemplateCheck(binDir string) {
	if err := locale.MarkTemplateInstalled(binDir, constants.AppVersion); err != nil {
		debuglog.WarnLog("template: failed to record stale-check version: %v", err)
	}
}

// StartTemplateRefresh runs RefreshTemplateIfStale in the background and arms
// the gate config builds wait on (awaitTemplateRefresh): the first core start
// after an upgrade must be built from the refreshed template instead of racing
// the download with the old one — and without holding the window back while
// the network answers.
//
// Call once from main, before anything can start the core (autostart, UI,
// debug API).
func (ac *AppController) StartTemplateRefresh() {
	if ac == nil || ac.FileService == nil {
		return
	}
	done := make(chan struct{})
	ac.templateRefreshDone.Store(&done)
	layout := ac.FileService.Layout
	parent := ac.ctx
	if parent == nil {
		parent = context.Background()
	}

	// SPEC 132: отметки версий — СВОЕЙ горутиной, а не внутри шаблонной.
	// Ворота сборки (awaitTemplateRefresh) ждут именно докачку шаблона, и
	// вешать на них ещё и `sing-box version` значило бы задерживать первый
	// старт ради записи в лог.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				debuglog.WarnLog("version marks: recovered from panic: %v", r)
			}
		}()
		ac.logCoreResolution()
		ac.CheckVersionMarks()
	}()

	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				debuglog.WarnLog("template: refresh recovered from panic: %v", r)
			}
		}()
		ctx, cancel := context.WithTimeout(parent, templateRefreshTimeout)
		defer cancel()

		res, err := RefreshTemplateIfStale(ctx, layout, ac.GetURLBytes)
		if err != nil {
			debuglog.WarnLog("template: %v — the installed template stays in use", err)
		} else if res.Downloaded {
			debuglog.WarnLog("template: refreshed for launcher %s", constants.AppVersion)
		}
		// Before the gate opens: a start waiting on it must already see the
		// marker, or its rebuild takes the no-op path and runs the old file.
		//
		// SPEC 135 §3.4: after a migration the copied config.json still points
		// at the old root. NewFileService stamps that old root into the new
		// settings.json when it carried none, so the data-root check above
		// fires on this start and on any restart before the rebuild. The
		// in-memory migration flag stays as the fallback for a stamp that
		// could not be written.
		migrated := ac.FileService != nil && ac.FileService.Migration.Migrated
		if (res.RebuildConfig || migrated) && ac.StateService != nil {
			if migrated && !res.RebuildConfig {
				debuglog.WarnLog("config: data migrated from %s, rebuilding config.json", ac.FileService.Migration.Source)
			}
			ac.StateService.MarkConfigStale()
		}
		if res.Downloaded && ac.UIService != nil && ac.UIService.UpdateConfigStatusFunc != nil {
			ac.UIService.UpdateConfigStatusFunc()
		}
	}()
}

// awaitTemplateRefresh blocks until the startup template refresh has finished.
// No-op when none was started (tests, tools). The refresh is bounded by its
// own timeout; the extra margin here only guards against a hang outside the
// network call, so a start can never block forever.
func (ac *AppController) awaitTemplateRefresh() {
	if ac == nil {
		return
	}
	p := ac.templateRefreshDone.Load()
	if p == nil {
		return
	}
	select {
	case <-*p:
		return
	default:
	}
	debuglog.InfoLog("template: waiting for the startup template refresh before building config.json")
	select {
	case <-*p:
	case <-time.After(templateRefreshTimeout + 5*time.Second):
		debuglog.WarnLog("template: startup refresh still running after %s — building with the installed template",
			templateRefreshTimeout+5*time.Second)
	}
}

// isDevAppVersion reports whether AppVersion is a non-release shape: the
// hard-coded default (`v-local-test`), the build-script default
// (`unnamed-dev`), or `git describe`-with-`-dirty`.
//
// Dev shapes don't follow semver, so CompareVersions against them is
// undefined; we sidestep the whole ladder for them.
func isDevAppVersion(v string) bool {
	if v == "" {
		return true
	}
	if strings.HasPrefix(v, "v-local-test") || strings.Contains(v, "unnamed-dev") {
		return true
	}
	if strings.HasSuffix(v, "-dirty") {
		return true
	}
	return false
}
