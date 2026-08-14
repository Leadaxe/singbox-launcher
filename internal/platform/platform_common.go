package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"singbox-launcher/internal/constants"
)

// ShortcutModifierLabel returns the human-visible label for the platform's
// keyboard shortcut modifier — "⌘" on macOS, "Ctrl" on Windows/Linux. Used in
// tooltips and similar surface text. Mirrors what fyne.KeyModifierShortcutDefault
// resolves to per platform.
func ShortcutModifierLabel() string {
	if runtime.GOOS == "darwin" {
		return "⌘"
	}
	return "Ctrl"
}

// DefaultDirMode — права по умолчанию для создания директорий (rwxr-xr-x).
// На Windows значение игнорируется ОС, но Go требует параметр в os.MkdirAll.
const DefaultDirMode os.FileMode = 0755

// DefaultFileMode — права по умолчанию для создания/записи файлов (rw-r--r--).
// На Windows Go смотрит только на бит 0200 (owner write) для read-only флага.
const DefaultFileMode os.FileMode = 0644

// GetConfigPath returns the path to config.json
func GetConfigPath(execDir string) string {
	return filepath.Join(execDir, constants.BinDirName, constants.ConfigFileName)
}

// GetRemoteMachineDir returns the directory holding EVERYTHING owned by one
// remote machine: <execDir>/bin/wizard_states/remote/<id>/ (SPEC 098 §2.3).
//
// Its state file, named snapshots, built config, .srs and subscription bodies
// all live here, so deleting a machine is deleting this directory plus its
// key directory — no hunting through bin/ for stragglers.
//
// An empty id falls back to the flat remote/ directory: that is the pre-098
// singleton layout, kept readable so a launcher upgrade before migration
// doesn't lose sight of the one configured machine.
func GetRemoteMachineDir(execDir, id string) string {
	base := filepath.Join(GetWizardStatesDir(execDir), constants.ConfigTargetRemote)
	if id = strings.TrimSpace(id); id != "" {
		return filepath.Join(base, id)
	}
	return base
}

// GetRemoteConfigPathFor returns the built config of one remote machine:
// <machine-dir>/config.json (SPEC 098 §2.3).
//
// Deliberately NOT the local bin/config.json: that file belongs to the local
// core and is rewritten by Update/Rebuild, so a remote config placed there
// would either be clobbered or — worse — picked up and run locally.
//
// Per-machine rather than the pre-098 singleton bin/remote-config.json: with
// one file for every machine the second machine silently overwrote the first,
// and Deploy could send a config built for a different platform entirely.
func GetRemoteConfigPathFor(execDir, id string) string {
	return filepath.Join(GetRemoteMachineDir(execDir, id), constants.ConfigFileName)
}

// GetBinDir returns the path to bin directory
func GetBinDir(execDir string) string {
	return filepath.Join(execDir, constants.BinDirName)
}

// GetRuleSetsDir returns the path to bin/rule-sets directory (локальные SRS файлы)
func GetRuleSetsDir(execDir string) string {
	return filepath.Join(execDir, constants.BinDirName, constants.RuleSetsDirName)
}

// GetRuleSetsDirFor returns the .srs directory for a config target
// (SPEC 098 §2.3).
//
//	local  → <execDir>/bin/rule-sets/                       (unchanged)
//	remote → <execDir>/bin/wizard_states/remote/<id>/srs/
//
// Machines do not share rule sets even when the tags coincide. Sharing would
// mean orphan GC had to compute its live set as the union over every machine's
// states — one global calculation that a rebuild of any single machine could
// get wrong — and would make deleting a machine a search rather than an rmdir.
func GetRuleSetsDirFor(execDir, target, id string) string {
	if stateTargetSlug(target) == "" {
		return GetRuleSetsDir(execDir)
	}
	return filepath.Join(GetRemoteMachineDir(execDir, id), constants.RemoteRuleSetsDirName)
}

// GetSubscriptionsDirFor returns the raw-subscription-body directory for a
// config target (SPEC 098 §2.3).
//
//	local  → <execDir>/bin/subscriptions/                            (unchanged)
//	remote → <execDir>/bin/wizard_states/remote/<id>/subscriptions/
//
// Same isolation rule as GetRuleSetsDirFor, and for the same reason: the
// pre-098 shared directory forced collectAllStageSourceIDs to union Source.IDs
// across every state on disk so that refreshing one machine wouldn't delete a
// body owned only by another.
func GetSubscriptionsDirFor(execDir, target, id string) string {
	if stateTargetSlug(target) == "" {
		return GetSubscriptionsDir(execDir)
	}
	return filepath.Join(GetRemoteMachineDir(execDir, id), constants.SubscriptionsDirName)
}

// GetWizardTemplatePath returns the canonical path of wizard_template.json:
// <execDir>/bin/wizard_template.json. This is the only sanctioned way for the
// rest of the codebase to locate the template file — do NOT compose the path
// from string literals.
func GetWizardTemplatePath(execDir string) string {
	return filepath.Join(execDir, constants.BinDirName, constants.WizardTemplateFileName)
}

// GetWizardStatesDir returns the directory holding all wizard states:
// <execDir>/bin/wizard_states/. The "current" state file (state.json) lives
// inside this directory; named state snapshots also live here.
func GetWizardStatesDir(execDir string) string {
	return filepath.Join(execDir, constants.BinDirName, constants.WizardStatesDirName)
}

// GetWizardStatePath returns the canonical path of the current wizard state:
// <execDir>/bin/wizard_states/state.json. The only sanctioned way to locate
// state.json — do NOT compose from string literals.
//
// This is the LOCAL target's state. For remote targets see
// GetWizardStatesDirFor / GetWizardStatePathFor (SPEC 097).
func GetWizardStatePath(execDir string) string {
	return filepath.Join(GetWizardStatesDir(execDir), constants.WizardStateFileName)
}

// GetWizardStatesDirFor returns the states directory for a config target
// (SPEC 097, machine-scoped in SPEC 098).
//
//	local  → <execDir>/bin/wizard_states/            (unchanged; no migration)
//	remote → <execDir>/bin/wizard_states/remote/<id>/
//
// The local target keeps the historical flat layout, so every existing reader
// (varsubst, snapshot, config_service) and every saved snapshot stays valid;
// the id argument is ignored for it.
//
// Each remote machine gets its own subdirectory, which also isolates its
// named snapshots: StateStore.ListWizardStates skips subdirectories, so a
// store rooted at one machine never lists another machine's snapshots.
//
// An empty id resolves to the flat remote/ directory — the pre-098 singleton
// layout, which migration reads from and then vacates.
//
// An unknown / empty target is treated as local — callers that predate
// targets keep working.
func GetWizardStatesDirFor(execDir, target, id string) string {
	if stateTargetSlug(target) == "" {
		return GetWizardStatesDir(execDir)
	}
	return GetRemoteMachineDir(execDir, id)
}

// GetWizardStatePathFor returns the current state file for a config target:
// <states-dir-for-target>/state.json (SPEC 097). The only sanctioned way to
// locate a non-local state file — do NOT compose from string literals.
func GetWizardStatePathFor(execDir, target, id string) string {
	return filepath.Join(GetWizardStatesDirFor(execDir, target, id), constants.WizardStateFileName)
}

// stateTargetSlug maps a target to its subdirectory name; "" means «no
// subdirectory» (the local target lives directly in wizard_states/).
//
// Unknown values fall back to local rather than to some new directory: a typo
// must not silently strand a state file where no reader looks for it.
func stateTargetSlug(target string) string {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case constants.ConfigTargetRemote:
		return constants.ConfigTargetRemote
	default:
		return ""
	}
}

// GetOutboundsCachePath returns the canonical path of the outbounds cache:
// <execDir>/bin/outbounds.cache.json. SPEC 045 phase 5.1. The only sanctioned
// way to locate the cache — do NOT compose from string literals.
func GetOutboundsCachePath(execDir string) string {
	return filepath.Join(execDir, constants.BinDirName, constants.OutboundsCacheFileName)
}

// GetSubscriptionsDir returns the directory for raw subscription bodies:
// <execDir>/bin/subscriptions/. One file per Source(id) — see SPEC 052.
// The only sanctioned way to locate this dir — do NOT compose from string
// literals.
func GetSubscriptionsDir(execDir string) string {
	return filepath.Join(execDir, constants.BinDirName, constants.SubscriptionsDirName)
}

// GetLogsDir returns the path to logs directory
func GetLogsDir(execDir string) string {
	return filepath.Join(execDir, constants.LogsDirName)
}

// EnsureDirectories creates necessary directories if they don't exist
func EnsureDirectories(execDir string) error {
	dirs := []string{
		GetLogsDir(execDir),
		GetBinDir(execDir),
		GetRuleSetsDir(execDir),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, DefaultDirMode); err != nil {
			return err
		}
	}
	return nil
}
