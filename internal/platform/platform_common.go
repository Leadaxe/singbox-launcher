package platform

import (
	"os"
	"path/filepath"

	"singbox-launcher/internal/constants"
)

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
