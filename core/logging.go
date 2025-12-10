// Package core provides core application logic including process management,
// configuration parsing, and service orchestration.
package core

import (
	"log"
	"os"
)

const (
	// maxLogFileSize is the maximum log file size before rotation (10 MB)
	maxLogFileSize = 10 * 1024 * 1024
)

// checkAndRotateLogFile checks log file size and rotates if it exceeds maxLogFileSize.
// Rotates by renaming current file to .old and removing old backup if exists.
func checkAndRotateLogFile(logPath string) {
	info, err := os.Stat(logPath)
	if err != nil {
		return // File doesn't exist yet, nothing to rotate
	}

	if info.Size() > maxLogFileSize {
		// Rotate: rename current file to .old
		oldPath := logPath + ".old"
		_ = os.Remove(oldPath) // Remove old backup if exists
		if err := os.Rename(logPath, oldPath); err != nil {
			log.Printf("checkAndRotateLogFile: Failed to rotate log file %s: %v", logPath, err)
		} else {
			log.Printf("checkAndRotateLogFile: Rotated log file %s (size: %d bytes)", logPath, info.Size())
		}
	}
}

// openLogFileWithRotation opens a log file and rotates it if it exceeds maxLogFileSize.
func openLogFileWithRotation(logPath string) (*os.File, error) {
	checkAndRotateLogFile(logPath)

	// Open file in append mode (not truncate) to preserve recent logs
	// But if file was rotated, it will be a new file
	return os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
}
