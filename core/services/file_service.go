package services

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// FileService manages file paths and log file handles.
// It encapsulates file-related operations to reduce AppController complexity.
type FileService struct {
	// File paths
	ExecDir     string
	ConfigPath  string
	SingboxPath string
	WintunPath  string

	// Log files
	MainLogFile  *os.File
	ChildLogFile *os.File
	ApiLogFile   *os.File
}

// NewFileService creates and initializes a new FileService instance.
func NewFileService() (*FileService, error) {
	fs := &FileService{}

	ex, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("NewFileService: cannot determine executable path: %w", err)
	}
	fs.ExecDir = filepath.Dir(ex)

	// Use platform-specific functions
	if err := platform.EnsureDirectories(fs.ExecDir); err != nil {
		return nil, fmt.Errorf("NewFileService: cannot create directories: %w", err)
	}

	fs.ConfigPath = platform.GetConfigPath(fs.ExecDir)
	singboxName := platform.GetExecutableNames()
	fs.SingboxPath = filepath.Join(fs.ExecDir, "bin", singboxName)
	fs.WintunPath = platform.GetWintunPath(fs.ExecDir)

	return fs, nil
}

// OpenLogFiles opens all log files with rotation support.
func (fs *FileService) OpenLogFiles(logFileName, childLogFileName, apiLogFileName string) error {
	// Open main log file
	logFile, err := fs.OpenLogFileWithRotation(filepath.Join(fs.ExecDir, logFileName))
	if err != nil {
		return fmt.Errorf("OpenLogFiles: cannot open main log file: %w", err)
	}
	log.SetOutput(logFile)
	fs.MainLogFile = logFile

	// Open child log file
	childLogFile, err := fs.OpenLogFileWithRotation(filepath.Join(fs.ExecDir, childLogFileName))
	if err != nil {
		log.Printf("OpenLogFiles: failed to open sing-box child log file: %v", err)
		fs.ChildLogFile = nil
	} else {
		fs.ChildLogFile = childLogFile
	}

	// Open API log file
	apiLogFile, err := fs.OpenLogFileWithRotation(filepath.Join(fs.ExecDir, apiLogFileName))
	if err != nil {
		log.Printf("OpenLogFiles: failed to open API log file: %v", err)
		fs.ApiLogFile = nil
	} else {
		fs.ApiLogFile = apiLogFile
	}

	return nil
}

// CloseLogFiles closes all log files.
func (fs *FileService) CloseLogFiles() {
	if fs.MainLogFile != nil {
		debuglog.CloseWithLog("CloseLogFiles: main log file", fs.MainLogFile)
		fs.MainLogFile = nil
	}
	if fs.ChildLogFile != nil {
		debuglog.CloseWithLog("CloseLogFiles: child log file", fs.ChildLogFile)
		fs.ChildLogFile = nil
	}
	if fs.ApiLogFile != nil {
		debuglog.CloseWithLog("CloseLogFiles: API log file", fs.ApiLogFile)
		fs.ApiLogFile = nil
	}
}

// OpenLogFileWithRotation opens a log file with rotation support.
func (fs *FileService) OpenLogFileWithRotation(logPath string) (*os.File, error) {
	fs.CheckAndRotateLogFile(logPath)
	// Open file in append mode (not truncate) to preserve recent logs
	// But if file was rotated, it will be a new file
	return os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
}

// CheckAndRotateLogFile checks log file size and rotates if it exceeds maxLogFileSize.
const maxLogFileSize = 2 * 1024 * 1024 // 2 MB

func (fs *FileService) CheckAndRotateLogFile(logPath string) {
	info, err := os.Stat(logPath)
	if err != nil {
		return // File doesn't exist yet, nothing to rotate
	}

	if info.Size() > maxLogFileSize {
		// Rotate: rename current file to .old
		oldPath := logPath + ".old"
		_ = os.Remove(oldPath) // Remove old backup if exists
		if err := os.Rename(logPath, oldPath); err != nil {
			log.Printf("CheckAndRotateLogFile: Failed to rotate log file %s: %v", logPath, err)
		} else {
			log.Printf("CheckAndRotateLogFile: Rotated log file %s (size: %d bytes)", logPath, info.Size())
		}
	}
}
