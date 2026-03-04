package platform

import (
	"os"
	"path/filepath"

	"singbox-launcher/internal/constants"
)

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
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}
