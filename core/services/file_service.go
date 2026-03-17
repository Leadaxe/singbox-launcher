// Package services содержит сервисы приложения, инкапсулирующие специфическую функциональность.
//
// FileService управляет файловыми путями и лог-файлами приложения.
//
// Ответственности:
//   - Определение путей к исполняемым файлам и конфигурации (ExecDir, ConfigPath, SingboxPath)
//   - Создание необходимых директорий (logs/, bin/) при старте
//   - Управление жизненным циклом лог-файлов (открытие, закрытие)
//   - Ротация логов при превышении размера (максимум 1 старый файл на каждый лог)
//   - Создание резервных копий файлов (BackupFile, BackupPath)
//
// Ротация логов:
//   - Порог: 2 MB (maxLogFileSize)
//   - При превышении: file.log → file.log.old (старый .old удаляется)
//   - Хранится максимум 1 резервная копия каждого лог-файла
//   - Ротация вызывается при открытии лога и перед каждым запуском sing-box
//
// Используется в:
//   - controller.go — инициализация при старте, RunHidden для логирования дочерних процессов
//   - process_service.go — пути к sing-box, лог-файл дочернего процесса
//   - wintun_downloader.go — путь к wintun.dll
//   - ui/wizard/business/saver.go — путь к config.json через FileServiceInterface
package services

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// maxLogFileSize — порог ротации лог-файлов (2 MB).
// При превышении текущий лог переименовывается в .old, старый .old удаляется.
const maxLogFileSize = 2 * 1024 * 1024 // 2 MB

// FileService управляет файловыми путями и лог-файлами приложения.
// Создаётся один раз при старте через NewFileService и хранится в AppController.
type FileService struct {
	// ExecDir — директория, в которой находится исполняемый файл приложения.
	// Все относительные пути (bin/, logs/, config.json) строятся от неё.
	ExecDir string

	// ConfigPath — полный путь к config.json (платформозависимый).
	ConfigPath string

	// SingboxPath — полный путь к исполняемому файлу sing-box (bin/sing-box или bin/sing-box.exe).
	SingboxPath string

	// WintunPath — полный путь к wintun.dll (только Windows, пустая строка на других платформах).
	WintunPath string

	// MainLogFile — лог приложения (singbox-launcher.log).
	// Используется как вывод стандартного log пакета.
	MainLogFile *os.File

	// ChildLogFile — лог дочернего процесса sing-box (sing-box.log).
	// Stdout/Stderr sing-box перенаправляются в этот файл.
	ChildLogFile *os.File

	// ApiLogFile — лог API-запросов (api.log).
	ApiLogFile *os.File

	// ChildLogRelativePath is the relative path to the sing-box log file (e.g. "logs/sing-box.log").
	// Set when OpenLogFiles is called; used by the log viewer to build the full path without duplicating the constant.
	ChildLogRelativePath string
}

// NewFileService создаёт и инициализирует FileService.
// Определяет все пути и создаёт необходимые директории (logs/, bin/).
// Вызывается один раз при создании AppController.
func NewFileService() (*FileService, error) {
	fs := &FileService{}

	ex, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("NewFileService: cannot determine executable path: %w", err)
	}
	fs.ExecDir = filepath.Dir(ex)

	if err := platform.EnsureDirectories(fs.ExecDir); err != nil {
		return nil, fmt.Errorf("NewFileService: cannot create directories: %w", err)
	}

	fs.ConfigPath = platform.GetConfigPath(fs.ExecDir)
	singboxName := platform.GetExecutableNames()
	fs.SingboxPath = filepath.Join(fs.ExecDir, "bin", singboxName)
	fs.WintunPath = platform.GetWintunPath(fs.ExecDir)

	return fs, nil
}

// OpenLogFiles открывает все лог-файлы приложения с ротацией.
// Основной лог (MainLogFile) устанавливается как вывод стандартного log пакета.
// Ошибки открытия ChildLogFile и ApiLogFile не являются критическими —
// приложение продолжает работу без них.
func (fs *FileService) OpenLogFiles(logFileName, childLogFileName, apiLogFileName string) error {
	fs.ChildLogRelativePath = childLogFileName
	logFile, err := fs.OpenLogFileWithRotation(filepath.Join(fs.ExecDir, logFileName))
	if err != nil {
		return fmt.Errorf("OpenLogFiles: cannot open main log file: %w", err)
	}
	log.SetOutput(logFile)
	fs.MainLogFile = logFile

	childLogFile, err := fs.OpenLogFileWithRotation(filepath.Join(fs.ExecDir, childLogFileName))
	if err != nil {
		debuglog.WarnLog("OpenLogFiles: failed to open sing-box child log file: %v", err)
		fs.ChildLogFile = nil
	} else {
		fs.ChildLogFile = childLogFile
	}

	apiLogFile, err := fs.OpenLogFileWithRotation(filepath.Join(fs.ExecDir, apiLogFileName))
	if err != nil {
		debuglog.WarnLog("OpenLogFiles: failed to open API log file: %v", err)
		fs.ApiLogFile = nil
	} else {
		fs.ApiLogFile = apiLogFile
	}

	return nil
}

// CloseLogFiles закрывает все открытые лог-файлы.
// Вызывается при завершении приложения (GracefulExit).
func (fs *FileService) CloseLogFiles() {
	if fs.MainLogFile != nil {
		debuglog.RunAndLog("CloseLogFiles: close main log file", fs.MainLogFile.Close)
		fs.MainLogFile = nil
	}
	if fs.ChildLogFile != nil {
		debuglog.RunAndLog("CloseLogFiles: close child log file", fs.ChildLogFile.Close)
		fs.ChildLogFile = nil
	}
	if fs.ApiLogFile != nil {
		debuglog.RunAndLog("CloseLogFiles: close API log file", fs.ApiLogFile.Close)
		fs.ApiLogFile = nil
	}
}

// OpenLogFileWithRotation открывает лог-файл с предварительной ротацией.
// Если файл превышает maxLogFileSize, он будет переименован в .old перед открытием.
// Файл открывается в режиме append (O_APPEND) — новые записи добавляются в конец.
func (fs *FileService) OpenLogFileWithRotation(logPath string) (*os.File, error) {
	fs.CheckAndRotateLogFile(logPath)
	return os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, platform.DefaultFileMode)
}

// CheckAndRotateLogFile проверяет размер лог-файла и выполняет ротацию при необходимости.
// Ротация: file.log → file.log.old (предыдущий .old удаляется).
// Хранится максимум 1 резервная копия.
// Вызывается также из process_service.go перед каждым запуском sing-box.
func (fs *FileService) CheckAndRotateLogFile(logPath string) {
	info, err := os.Stat(logPath)
	if err != nil {
		return // File doesn't exist yet, nothing to rotate
	}

	if info.Size() > maxLogFileSize {
		oldPath := logPath + ".old"
		_ = os.Remove(oldPath) // Remove old backup if exists
		if err := os.Rename(logPath, oldPath); err != nil {
			debuglog.WarnLog("CheckAndRotateLogFile: Failed to rotate log file %s: %v", logPath, err)
		} else {
			debuglog.DebugLog("CheckAndRotateLogFile: Rotated log file %s (size: %d bytes)", logPath, info.Size())
		}
	}
}

// BackupPath генерирует путь для резервной копии файла.
// Формат: file.ext → file-old.ext (например, config.json → config-old.json).
func BackupPath(path string) string {
	dir := filepath.Dir(path)
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)
	return filepath.Join(dir, fmt.Sprintf("%s-old%s", base, ext))
}

// ReadLastLines reads up to maxLines last lines from the file at path.
// Used by the diagnostics log viewer (Core tab) for tail-style display.
// Returns (nil, nil) if the file does not exist; returns (nil, err) on read errors.
func ReadLastLines(path string, maxLines int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			debuglog.WarnLog("ReadLastLines: failed to close file %s: %v", path, cerr)
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("ReadLastLines: %s is a directory", path)
	}
	size := info.Size()
	if size == 0 {
		return []string{}, nil
	}
	// Read backwards in chunks to collect last N lines
	const chunkSize = 4096
	var buf []byte
	pos := size
	for pos > 0 && len(buf) < 2*1024*1024 { // cap total read at 2MB
		readSize := chunkSize
		if int64(readSize) > pos {
			readSize = int(pos)
		}
		pos -= int64(readSize)
		_, err := f.Seek(pos, io.SeekStart)
		if err != nil {
			return nil, err
		}
		chunk := make([]byte, readSize)
		_, err = io.ReadFull(f, chunk)
		if err != nil {
			return nil, err
		}
		buf = append(chunk, buf...)
	}
	// buf is now the last min(2MB, size) of the file
	// Split into lines and take last maxLines
	scanner := bufio.NewScanner(strings.NewReader(string(buf)))
	scanner.Buffer(nil, 512*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(lines) <= maxLines {
		return lines, nil
	}
	return lines[len(lines)-maxLines:], nil
}

// BackupFile создаёт резервную копию файла (file.ext → file-old.ext).
// Предыдущая резервная копия удаляется — хранится максимум 1 бэкап.
// Если файл не существует или является директорией — ничего не делает.
func BackupFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Nothing to backup
		}
		return fmt.Errorf("BackupFile: cannot stat %s: %w", path, err)
	}
	if info.IsDir() {
		return nil
	}

	backup := BackupPath(path)
	_ = os.Remove(backup) // Remove old backup if exists
	if err := os.Rename(path, backup); err != nil {
		return fmt.Errorf("BackupFile: cannot rename %s to %s: %w", path, backup, err)
	}
	debuglog.DebugLog("BackupFile: %s → %s", path, backup)
	return nil
}
