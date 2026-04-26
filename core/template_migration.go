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

// templateFileName mirrors ui/wizard/template.TemplateFileName. Duplicated
// here to keep core/ free of a downstream dependency on ui/.
const templateFileName = "wizard_template.json"

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

	templatePath := filepath.Join(binDir, templateFileName)
	if _, err := os.Stat(templatePath); err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to invalidate
		}
		return fmt.Errorf("template invalidation: stat %s: %w", templatePath, err)
	}

	if err := os.Remove(templatePath); err != nil {
		return fmt.Errorf("template invalidation: remove %s: %w", templatePath, err)
	}
	debuglog.InfoLog("template: invalidated by launcher upgrade (was installed by %q, now %q)", last, constants.AppVersion)
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
