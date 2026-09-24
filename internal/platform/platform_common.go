package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/paths"
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

// Хелперы путей SPEC 135: каждый принимает именованный корень. Состояние,
// кэши и скачанное — paths.DataDir; поставляемое (только чтение) —
// paths.AppDir. Передать AppDir в пишущий хелпер не даст компилятор.

// GetConfigPath returns the path to config.json: <DataDir>/bin/config.json.
func GetConfigPath(d paths.DataDir) string {
	return filepath.Join(d.Bin(), constants.ConfigFileName)
}

// GetRemoteMachineDir returns the directory holding EVERYTHING owned by one
// remote machine: <DataDir>/bin/wizard_states/remote/<id>/ (SPEC 098 §2.3).
//
// Its state file, named snapshots, built config, .srs and subscription bodies
// all live here, so deleting a machine is deleting this directory plus its
// key directory — no hunting through bin/ for stragglers.
//
// An empty id falls back to the flat remote/ directory: that is the pre-098
// singleton layout, kept readable so a launcher upgrade before migration
// doesn't lose sight of the one configured machine.
func GetRemoteMachineDir(d paths.DataDir, id string) string {
	base := filepath.Join(GetWizardStatesDir(d), constants.ConfigTargetRemote)
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
func GetRemoteConfigPathFor(d paths.DataDir, id string) string {
	return filepath.Join(GetRemoteMachineDir(d, id), constants.ConfigFileName)
}

// GetRuleSetsDir returns the path to <DataDir>/bin/rule-sets (локальные SRS файлы).
func GetRuleSetsDir(d paths.DataDir) string {
	return filepath.Join(d.Bin(), constants.RuleSetsDirName)
}

// GetRuleSetPath returns the local .srs file of one rule set:
// <DataDir>/bin/rule-sets/<tag>.srs. The only sanctioned way to compose it —
// the path is emitted into config.json as rule_set[].path.
func GetRuleSetPath(d paths.DataDir, tag string) string {
	return filepath.Join(GetRuleSetsDir(d), tag+".srs")
}

// GetTailscaleStateDir returns the root of tailnet state directories:
// <DataDir>/bin/tailscale (SPEC 122).
func GetTailscaleStateDir(d paths.DataDir) string {
	return filepath.Join(d.Bin(), constants.TailscaleDirName)
}

// GetTempDir returns the scratch directory for core / wintun downloads:
// <DataDir>/temp.
func GetTempDir(d paths.DataDir) string {
	return filepath.Join(string(d), constants.TempDirName)
}

// GetDaemonIdentityDir returns the local daemon pairing identity directory:
// <DataDir>/bin/daemon.
func GetDaemonIdentityDir(d paths.DataDir) string {
	return filepath.Join(d.Bin(), constants.DaemonIdentityDirName)
}

// GetRemoteDaemonIdentityDir returns the pairing identity directory of one
// remote daemon: <DataDir>/bin/remote-daemons/<id>.
func GetRemoteDaemonIdentityDir(d paths.DataDir, id string) string {
	return filepath.Join(d.Bin(), constants.RemoteDaemonsDirName, id)
}

// GetRuleSetsDirFor returns the .srs directory for a config target
// (SPEC 098 §2.3).
//
//	local  → <DataDir>/bin/rule-sets/                       (unchanged)
//	remote → <DataDir>/bin/wizard_states/remote/<id>/srs/
//
// Machines do not share rule sets even when the tags coincide. Sharing would
// mean orphan GC had to compute its live set as the union over every machine's
// states — one global calculation that a rebuild of any single machine could
// get wrong — and would make deleting a machine a search rather than an rmdir.
func GetRuleSetsDirFor(d paths.DataDir, target, id string) string {
	if stateTargetSlug(target) == "" {
		return GetRuleSetsDir(d)
	}
	return filepath.Join(GetRemoteMachineDir(d, id), constants.RemoteRuleSetsDirName)
}

// GetSubscriptionsDirFor returns the raw-subscription-body directory for a
// config target (SPEC 098 §2.3).
//
//	local  → <DataDir>/bin/subscriptions/                            (unchanged)
//	remote → <DataDir>/bin/wizard_states/remote/<id>/subscriptions/
//
// Same isolation rule as GetRuleSetsDirFor, and for the same reason: the
// pre-098 shared directory forced collectAllStageSourceIDs to union Source.IDs
// across every state on disk so that refreshing one machine wouldn't delete a
// body owned only by another.
func GetSubscriptionsDirFor(d paths.DataDir, target, id string) string {
	if stateTargetSlug(target) == "" {
		return GetSubscriptionsDir(d)
	}
	return filepath.Join(GetRemoteMachineDir(d, id), constants.SubscriptionsDirName)
}

// GetWizardTemplatePath returns the downloaded / working copy of the wizard
// template: <DataDir>/bin/wizard_template.json — the download target. Which
// file is READ (this one or the shipped one) is decided by
// template.ResolveTemplate (SPEC 135 §3.3); readers go through it. Do NOT
// compose the path from string literals.
func GetWizardTemplatePath(d paths.DataDir) string {
	return filepath.Join(d.Bin(), constants.WizardTemplateFileName)
}

// GetShippedTemplatePath returns the template shipped with the binary:
// <AppDir>/bin/wizard_template.json (read-only seed, SPEC 135 §3.3).
func GetShippedTemplatePath(a paths.AppDir) string {
	return filepath.Join(a.Bin(), constants.WizardTemplateFileName)
}

// GetWizardStatesDir returns the directory holding all wizard states:
// <DataDir>/bin/wizard_states/. The "current" state file (state.json) lives
// inside this directory; named state snapshots also live here.
func GetWizardStatesDir(d paths.DataDir) string {
	return filepath.Join(d.Bin(), constants.WizardStatesDirName)
}

// GetWizardStatePath returns the canonical path of the current wizard state:
// <DataDir>/bin/wizard_states/state.json. The only sanctioned way to locate
// state.json — do NOT compose from string literals.
//
// This is the LOCAL target's state. For remote targets see
// GetWizardStatesDirFor / GetWizardStatePathFor (SPEC 097).
func GetWizardStatePath(d paths.DataDir) string {
	return filepath.Join(GetWizardStatesDir(d), constants.WizardStateFileName)
}

// GetWizardStatesDirFor returns the states directory for a config target
// (SPEC 097, machine-scoped in SPEC 098).
//
//	local  → <DataDir>/bin/wizard_states/            (unchanged; no migration)
//	remote → <DataDir>/bin/wizard_states/remote/<id>/
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
func GetWizardStatesDirFor(d paths.DataDir, target, id string) string {
	if stateTargetSlug(target) == "" {
		return GetWizardStatesDir(d)
	}
	return GetRemoteMachineDir(d, id)
}

// GetWizardStatePathFor returns the current state file for a config target:
// <states-dir-for-target>/state.json (SPEC 097). The only sanctioned way to
// locate a non-local state file — do NOT compose from string literals.
func GetWizardStatePathFor(d paths.DataDir, target, id string) string {
	return filepath.Join(GetWizardStatesDirFor(d, target, id), constants.WizardStateFileName)
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
// <DataDir>/bin/outbounds.cache.json. SPEC 045 phase 5.1. The only sanctioned
// way to locate the cache — do NOT compose from string literals.
func GetOutboundsCachePath(d paths.DataDir) string {
	return filepath.Join(d.Bin(), constants.OutboundsCacheFileName)
}

// GetSubscriptionsDir returns the directory for raw subscription bodies:
// <DataDir>/bin/subscriptions/. One file per Source(id) — see SPEC 052.
// The only sanctioned way to locate this dir — do NOT compose from string
// literals.
func GetSubscriptionsDir(d paths.DataDir) string {
	return filepath.Join(d.Bin(), constants.SubscriptionsDirName)
}

// EnsureDirectories creates the writable directories of the layout:
// Data/bin, Data/bin/rule-sets and Logs. AppDir is never created or touched.
func EnsureDirectories(l paths.Layout) error {
	dirs := []string{
		string(l.Logs),
		l.Data.Bin(),
		GetRuleSetsDir(l.Data),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, DefaultDirMode); err != nil {
			return err
		}
	}
	return nil
}
