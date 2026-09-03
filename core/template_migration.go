package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// InvalidateTemplateIfStale removes bin/wizard_template.json when it was last
// installed by an older launcher version. Idempotent and called once on
// startup, before any UI is built.
//
// Why this exists (SPEC 046): the template format can shift between launcher
// versions (new vars, renamed params, schema upgrades). A template installed
// by v0.8.7 can silently misbehave under v0.8.8 — the wizard may still load
// it, but the generated config drifts from what was tested. Forcing a
// redownload on upgrade is cheap (one click for the user) and deterministic.
//
// Invalidate AT MOST ONCE per launcher version. After a stale-check (whether we
// actually removed a file or there was nothing to remove) we stamp
// LastTemplateLauncherVersion = current, so the next startup on the same version
// short-circuits. Without this, the marker was only ever written by the UI
// Download button — so a template placed back by hand (or shipped as a bundled
// seed) stayed "stale" forever and got wiped on *every* launch, making it
// impossible to install one manually. Stamping here means the user can drop a
// template file in by hand and it survives. (See bug: junk re-deletion loop.)
//
// Skipped for dev builds: AppVersion of the form "v-local-test" or
// "unnamed-dev" doesn't compare meaningfully against semver, so the policy
// below would either always-invalidate or never-invalidate. Both are
// annoying during inner-loop development; we just leave the local template
// alone in those cases.
//
// Caller-provided execDir keeps the function pure-function-pluggable for
// tests (no AppController dependency).
func InvalidateTemplateIfStale(execDir string) error {
	if isDevAppVersion(constants.AppVersion) {
		debuglog.DebugLog("template: skipping stale-check on dev build %q", constants.AppVersion)
		return nil
	}

	binDir := platform.GetBinDir(execDir)
	settings := locale.LoadSettings(binDir)
	last := settings.LastTemplateLauncherVersion

	if last != "" && CompareVersions(last, constants.AppVersion) >= 0 {
		// Same launcher (or downgrade — leave the file, user knows what
		// they're doing).
		return nil
	}

	templatePath := filepath.Join(binDir, constants.WizardTemplateFileName)

	// Шаблон, положенный установщиком под ЭТУ версию (архив win64-full,
	// install-macos.sh), несёт рядом маркер с версией лаунчера. Совпал — шаблон
	// свежий, и сносить его ради перекачки того же файла незачем: весь смысл
	// вложения в том, чтобы первый запуск не ходил в сеть. Маркер только
	// читается; штамп в settings ставится, как при любом другом исходе.
	if raw, rerr := os.ReadFile(filepath.Join(binDir, constants.WizardTemplateVersionFileName)); rerr == nil &&
		strings.TrimSpace(string(raw)) == constants.AppVersion {
		if _, serr := os.Stat(templatePath); serr == nil {
			debuglog.InfoLog("template: bundled by the installer for %q — kept", constants.AppVersion)
			if err := locale.MarkTemplateInstalled(binDir, constants.AppVersion); err != nil {
				debuglog.WarnLog("template: failed to record stale-check version: %v", err)
			}
			return nil
		}
	}

	switch _, err := os.Stat(templatePath); {
	case err == nil:
		if rmErr := os.Remove(templatePath); rmErr != nil {
			// Don't stamp the marker if the removal failed — we want to retry
			// the invalidation next startup rather than silently leave a stale
			// template in place.
			return fmt.Errorf("template invalidation: remove %s: %w", templatePath, rmErr)
		}
		debuglog.InfoLog("template: invalidated by launcher upgrade (was installed by %q, now %q)", last, constants.AppVersion)
	case os.IsNotExist(err):
		// Nothing to remove — but still record that this version has run its
		// stale-check, so a manually-placed template isn't wiped on next launch.
		debuglog.DebugLog("template: no file to invalidate; recording stale-check for %q", constants.AppVersion)
	default:
		return fmt.Errorf("template invalidation: stat %s: %w", templatePath, err)
	}

	// Mark the stale-check done for this version (at-most-once invalidation).
	// Best effort: if persisting fails we just re-invalidate next upgrade, the
	// same cosmetic nuisance MarkTemplateInstalled already tolerates.
	if err := locale.MarkTemplateInstalled(binDir, constants.AppVersion); err != nil {
		debuglog.WarnLog("template: failed to record stale-check version: %v", err)
	}
	return nil
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
