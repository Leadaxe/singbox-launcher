// Package services содержит сервисы приложения, инкапсулирующие специфическую функциональность.
//
// FileService управляет файловыми путями и лог-файлами приложения.
//
// Ответственности:
//   - Раскладка данных (Layout: AppDir/DataDir/LogDir, SPEC 135) и пути от неё (ConfigPath, SingboxPath, SingboxBundledPath)
//   - Миграция унаследованных данных App/bin → Data/bin при старте (SPEC 135 §3.4)
//   - Создание writable-директорий (Data/bin, Logs) при старте
//   - Управление жизненным циклом лог-файлов (открытие, закрытие)
//   - Ротация логов при превышении размера (максимум 1 старый файл на каждый лог)
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
package services

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
)

// maxLogFileSize — порог ротации лог-файлов (2 MB).
// При превышении текущий лог переименовывается в .old, старый .old удаляется.
const maxLogFileSize = 2 * 1024 * 1024 // 2 MB

// FileService управляет файловыми путями и лог-файлами приложения.
// Создаётся один раз при старте через NewFileService и хранится в AppController.
type FileService struct {
	// Layout — раскладка данных (SPEC 135), решается один раз в main():
	// App — поставляемое (только чтение), Data — состояние и кэши, Logs — логи.
	Layout paths.Layout

	// ConfigPath — полный путь к config.json (платформозависимый).
	ConfigPath string

	// SingboxBundledPath — <Data>/bin/sing-box (или .exe): цель установки ядра из лаунчера (Core → Download).
	SingboxBundledPath string

	// SingboxPath — путь для запуска sing-box, проверки версии и capabilities:
	// итог цепочки SINGBOX_LAUNCHER_CORE → Data/bin → App/bin → PATH
	// (SPEC 135 §3.3, platform.ResolveSingboxExecPath). Пересчитывает ResolveCore.
	SingboxPath string

	// CoreSource — откуда взят SingboxPath: env / data / app / path; "" — ядро
	// не найдено (SingboxPath тогда = SingboxBundledPath, цель скачивания).
	CoreSource string

	// ShadowedCorePath — второе найденное ядро, затенённое выбранным (Data над
	// App, env над Data/App); пусто, если затенять нечего. Только для лога.
	ShadowedCorePath string

	// WintunPath — wintun.dll рядом с выбранным ядром: Dir(SingboxPath)/wintun.dll
	// (только Windows, пустая строка на других платформах).
	WintunPath string

	// MainLogFile — лог приложения (singbox-launcher.log).
	// Используется как вывод стандартного log пакета.
	MainLogFile *os.File

	// ChildLogFile — лог дочернего процесса sing-box (sing-box.log).
	// Stdout/Stderr sing-box перенаправляются в этот файл.
	ChildLogFile *os.File

	// ApiLogFile — лог API-запросов (api.log).
	ApiLogFile *os.File

	// ChildLogPath — абсолютный путь лога sing-box: <Logs>/sing-box.log.
	ChildLogPath string

	// Migration — итог переноса унаследованных данных App/bin → Data/bin
	// (SPEC 135 §3.4), выполненного в NewFileService. Читает main: строка в
	// лог после открытия логов и решение об одноразовом уведомлении.
	Migration paths.MigrationResult

	// MigrationErr — сбой миграции. Старт не прерывается: приложение
	// поднимается с пустым DataDir, а state.json в Data так и не появился,
	// поэтому следующий старт повторит попытку.
	MigrationErr error
}

// NewFileService создаёт и инициализирует FileService от раскладки layout.
// Переносит унаследованные данные (SPEC 135 §3.4), определяет все пути и
// создаёт writable-директории (Data/bin, Logs).
// Вызывается один раз при создании AppController.
func NewFileService(layout paths.Layout) (*FileService, error) {
	fs := &FileService{Layout: layout}

	// SPEC 135 §3.4: до EnsureDirectories и до любого чтения settings/state
	// из Data — копия должна лечь раньше, чем кто-то увидит пустой Data/bin.
	// Логов ещё нет (они открываются следом, в OpenLogFiles), и debuglog
	// ранние строки не буферизует — итог пишет main после открытия логов.
	fs.Migration, fs.MigrationErr = paths.MigrateLegacyData(layout, nil)
	if fs.MigrationErr == nil && fs.Migration.Migrated {
		stampPreMigrationDataRoot(layout.Data, fs.Migration.Source)
	}

	if err := platform.EnsureDirectories(layout); err != nil {
		return nil, fmt.Errorf("NewFileService: cannot create directories: %w", err)
	}

	fs.ConfigPath = platform.GetConfigPath(layout.Data)
	fs.SingboxBundledPath = filepath.Join(layout.Data.Bin(), platform.GetExecutableNames())
	fs.ResolveCore()
	fs.ChildLogPath = filepath.Join(string(layout.Logs), constants.ChildLogFileName)

	return fs, nil
}

// stampPreMigrationDataRoot записывает в settings.json нового DataDir СТАРЫЙ
// корень данных — тот, под которым собран перенесённый config.json (SPEC 135
// §3.4, §3.5). Флаг Migration.Migrated живёт только в памяти первого старта:
// перезапуск лаунчера до пересборки терял его, и config.json с путями .srs и
// tailscale на старый корень уходил в ядро как есть. Несовпадение штампа с
// текущим DataDir переживает перезапуск (core.RefreshTemplateIfStale →
// пересборка); успешная сборка перепишет штамп текущим корнем.
//
// source — App/bin; корень — без хвоста bin, в той же форме, что
// core.configDataRoot (filepath.Clean). Штамп, уже принесённый из старого
// settings.json, не трогается. Не записалось — WARN (логов ещё нет, строка
// уходит в stderr); миграция при этом состоялась.
func stampPreMigrationDataRoot(data paths.DataDir, source string) {
	bin := data.Bin()
	if locale.LoadSettings(bin).ConfigDataRoot != "" {
		return
	}
	oldRoot := filepath.Clean(filepath.Dir(source))
	if err := locale.MarkConfigDataRoot(bin, oldRoot); err != nil {
		debuglog.WarnLog("migration: cannot record previous data root %s: %v", oldRoot, err)
	}
}

// ResolveCore пересчитывает путь ядра и его спутников (SPEC 135 §3.3, §5):
// сначала ядро по цепочке, затем wintun.dll от каталога выбранного ядра —
// загрузчик ОС ищет спутники рядом с sing-box, а не рядом с лаунчером.
//
// Зовётся из NewFileService и после успешного скачивания ядра (оно ложится
// в Data/bin и должно сменить поставляемое или системное).
func (fs *FileService) ResolveCore() {
	r := platform.ResolveSingboxExecPath(fs.Layout, os.Getenv)
	wintun := platform.GetWintunPathFor(filepath.Dir(r.Path))
	// Поля читают другие горутины без блокировки; пишем только изменившиеся,
	// чтобы повторный вызов с тем же итогом (обычный случай после скачивания
	// в Data/bin) ничего не трогал.
	if fs.SingboxPath != r.Path {
		fs.SingboxPath = r.Path
	}
	if fs.CoreSource != r.Source {
		fs.CoreSource = r.Source
	}
	if fs.ShadowedCorePath != r.Shadowed {
		fs.ShadowedCorePath = r.Shadowed
	}
	if fs.WintunPath != wintun {
		fs.WintunPath = wintun
	}
}

// OpenLogFiles открывает все лог-файлы приложения с ротацией.
// Основной лог (MainLogFile) устанавливается как вывод стандартного log пакета.
// Ошибки открытия ChildLogFile и ApiLogFile не являются критическими —
// приложение продолжает работу без них.
func (fs *FileService) OpenLogFiles() error {
	logFile, err := fs.OpenLogFileWithRotation(filepath.Join(string(fs.Layout.Logs), constants.MainLogFileName))
	if err != nil {
		return fmt.Errorf("OpenLogFiles: cannot open main log file: %w", err)
	}
	log.SetOutput(logFile)
	fs.MainLogFile = logFile

	childLogFile, err := fs.OpenLogFileWithRotation(fs.ChildLogPath)
	if err != nil {
		debuglog.WarnLog("OpenLogFiles: failed to open sing-box child log file: %v", err)
		fs.ChildLogFile = nil
	} else {
		fs.ChildLogFile = childLogFile
	}

	apiLogFile, err := fs.OpenLogFileWithRotation(filepath.Join(string(fs.Layout.Logs), constants.APILogFileName))
	if err != nil {
		debuglog.WarnLog("OpenLogFiles: failed to open API log file: %v", err)
		fs.ApiLogFile = nil
	} else {
		fs.ApiLogFile = apiLogFile
	}

	return nil
}

// ReopenChildLogFile закрывает дескриптор лога sing-box и заново открывает файл по пути
// ChildLogPath (создаёт при отсутствии). Нужно после внешнего удаления файла (например
// привилегированный rm): иначе старый fd указывает на снятый с каталога inode, а путь для
// просмотра логов и следующий Start не совпадают с реальной записью.
func (fs *FileService) ReopenChildLogFile() error {
	if fs == nil {
		return nil
	}
	full := fs.ChildLogPath
	if fs.ChildLogFile != nil {
		debuglog.RunAndLog("ReopenChildLogFile: close stale child log", fs.ChildLogFile.Close)
		fs.ChildLogFile = nil
	}
	f, err := fs.OpenLogFileWithRotation(full)
	if err != nil {
		debuglog.WarnLog("ReopenChildLogFile: cannot open %s: %v", full, err)
		return err
	}
	fs.ChildLogFile = f
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
//
// Если ротируется child-лог (sing-box.log), открытый ChildLogFile указывает
// на переименованный файл — без переоткрытия весь дальнейший вывод sing-box
// уходил бы в .old, который больше никогда не ротируется. Поэтому дескриптор
// закрывается перед rename (иначе на Windows rename падает) и открывается
// заново после.
func (fs *FileService) CheckAndRotateLogFile(logPath string) {
	info, err := os.Stat(logPath)
	if err != nil {
		return // File doesn't exist yet, nothing to rotate
	}
	if info.Size() <= maxLogFileSize {
		return
	}

	isChildLog := fs.ChildLogFile != nil && fs.ChildLogPath != "" && logPath == fs.ChildLogPath
	if isChildLog {
		if err := fs.ChildLogFile.Close(); err != nil {
			debuglog.WarnLog("CheckAndRotateLogFile: failed to close child log before rotation: %v", err)
		}
		fs.ChildLogFile = nil
	}

	oldPath := logPath + ".old"
	_ = os.Remove(oldPath) // Remove old backup if exists
	if err := os.Rename(logPath, oldPath); err != nil {
		debuglog.WarnLog("CheckAndRotateLogFile: Failed to rotate log file %s: %v", logPath, err)
	} else {
		debuglog.DebugLog("CheckAndRotateLogFile: Rotated log file %s (size: %d bytes)", logPath, info.Size())
	}

	if isChildLog {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, platform.DefaultFileMode)
		if err != nil {
			debuglog.WarnLog("CheckAndRotateLogFile: failed to reopen child log after rotation: %v", err)
			return
		}
		fs.ChildLogFile = f
	}
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
