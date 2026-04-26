package platform

import (
	"os"
	"path/filepath"
	"runtime"

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

// GetBinDir returns the path to bin directory
func GetBinDir(execDir string) string {
	return filepath.Join(execDir, constants.BinDirName)
}

// GetRuleSetsDir returns the path to bin/rule-sets directory (локальные SRS файлы)
func GetRuleSetsDir(execDir string) string {
	return filepath.Join(execDir, constants.BinDirName, constants.RuleSetsDirName)
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
func GetWizardStatePath(execDir string) string {
	return filepath.Join(GetWizardStatesDir(execDir), constants.WizardStateFileName)
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
