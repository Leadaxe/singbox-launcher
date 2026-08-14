package services

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// Миграция singleton-профиля удалённой машины в её собственную папку
// (SPEC 098 §2.3).
//
// До SPEC 098 на все удалённые машины было по одному файлу:
// bin/wizard_states/remote/state.json и bin/remote-config.json. Вторая
// добавленная машина молча затирала настройки первой. Теперь у каждой машины
// своя директория, и эти два файла надо отдать их законному владельцу.
//
// Владелец определяется однозначно ТОЛЬКО когда в реестре ровно одна запись:
// файлы могли быть настроены лишь для неё. При нуле или нескольких записях
// угадывать нельзя, а потерять настроенный конфиг недопустимо — оставляем
// файлы на месте и пишем warning.

// MigrateLegacyRemoteProfile переносит singleton-состояние и singleton-конфиг
// удалённой машины в директорию единственной записи реестра.
//
// Идемпотентна: повторный запуск после успешного переезда не находит исходных
// файлов и не делает ничего. Ошибки не фатальны — лаунчер продолжает работу,
// просто машина откроется с пустым состоянием, а старые файлы останутся на
// диске нетронутыми (потерять их хуже, чем не мигрировать).
func MigrateLegacyRemoteProfile(execDir string, registry *RemoteRegistry) error {
	if registry == nil {
		return nil
	}

	legacyDir := platform.GetRemoteMachineDir(execDir, "")
	legacyState := filepath.Join(legacyDir, constants.WizardStateFileName)
	legacyConfig := filepath.Join(platform.GetBinDir(execDir), constants.LegacyRemoteConfigFileName)

	stateExists := fileExists(legacyState)
	configExists := fileExists(legacyConfig)
	snapshots := legacyRemoteSnapshots(legacyDir)

	if !stateExists && !configExists && len(snapshots) == 0 {
		return nil // нечего мигрировать — обычный путь после первого запуска
	}

	list, err := registry.List()
	if err != nil {
		return fmt.Errorf("remote migration: read registry: %w", err)
	}
	if len(list) != 1 {
		debuglog.WarnLog("remote migration: found legacy remote profile (state=%v config=%v snapshots=%d), "+
			"but registry has %d machines — cannot tell whose it is; files left in place",
			stateExists, configExists, len(snapshots), len(list))
		return nil
	}

	id := strings.TrimSpace(list[0].ID)
	if id == "" {
		debuglog.WarnLog("remote migration: single registry entry has empty id; files left in place")
		return nil
	}
	dstDir := platform.GetRemoteMachineDir(execDir, id)
	if err := os.MkdirAll(dstDir, platform.DefaultDirMode); err != nil {
		return fmt.Errorf("remote migration: mkdir %s: %w", dstDir, err)
	}

	moved := 0
	if stateExists {
		if moveFile(legacyState, filepath.Join(dstDir, constants.WizardStateFileName)) {
			moved++
		}
	}
	for _, snap := range snapshots {
		if moveFile(snap, filepath.Join(dstDir, filepath.Base(snap))) {
			moved++
		}
	}
	// Конфиг переезжает под каноническим для машины именем config.json —
	// singleton-имя remote-config.json больше не существует.
	if configExists {
		if moveFile(legacyConfig, platform.GetRemoteConfigPathFor(execDir, id)) {
			moved++
		}
	}

	debuglog.InfoLog("remote migration: moved %d legacy file(s) into %s (machine %q)", moved, dstDir, list[0].Name)
	return nil
}

// legacyRemoteSnapshots возвращает именованные снапшоты, лежащие ПЛОСКО в
// remote/ — то есть принадлежащие singleton-профилю.
//
// Поддиректории пропускаются: это уже директории машин, их снапшоты на месте.
func legacyRemoteSnapshots(legacyDir string) []string {
	entries, err := os.ReadDir(legacyDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if e.Name() == constants.WizardStateFileName {
			continue // текущее состояние переносится отдельно
		}
		out = append(out, filepath.Join(legacyDir, e.Name()))
	}
	return out
}

// moveFile переносит файл, не перезаписывая существующий назначения.
//
// Отказ при существующем dst — не перестраховка: единственный сценарий, при
// котором он уже есть, это частично прошедшая предыдущая миграция или
// пользователь, успевший настроить машину заново. В обоих случаях новый файл
// свежее старого singleton'а.
func moveFile(src, dst string) bool {
	if fileExists(dst) {
		debuglog.WarnLog("remote migration: %s already exists — keeping it, leaving %s in place", dst, src)
		return false
	}
	if err := os.Rename(src, dst); err == nil {
		return true
	}
	// Rename не работает через границу файловых систем; копируем и удаляем.
	raw, err := os.ReadFile(src)
	if err != nil {
		debuglog.WarnLog("remote migration: read %s: %v", src, err)
		return false
	}
	if err := os.WriteFile(dst, raw, platform.DefaultFileMode); err != nil {
		debuglog.WarnLog("remote migration: write %s: %v", dst, err)
		return false
	}
	if err := os.Remove(src); err != nil {
		debuglog.WarnLog("remote migration: remove %s after copy: %v", src, err)
	}
	return true
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
