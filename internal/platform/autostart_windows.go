//go:build windows

package platform

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"

	"singbox-launcher/internal/debuglog"
)

// autostartRunKey — ключ Run текущего пользователя. Записи HKCU Windows
// запускает при входе с обычными правами; процесс с манифестом
// requireAdministrator оттуда не стартует — поэтому автозапуск появился
// только с asInvoker (SPEC 139 §1).
const autostartRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// ReadAutostart читает значение автозапуска; found=false — значения нет.
// StartupApproved (отключение в Диспетчере задач) не учитывается.
func ReadAutostart() (entry AutostartEntry, found bool, err error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartRunKey, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return AutostartEntry{}, false, nil
		}
		return AutostartEntry{}, false, fmt.Errorf("open HKCU\\%s: %w", autostartRunKey, err)
	}
	defer debuglog.RunAndLog("ReadAutostart: close key", k.Close)
	v, _, err := k.GetStringValue(AutostartValueName)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return AutostartEntry{}, false, nil
		}
		return AutostartEntry{}, false, fmt.Errorf("read %s: %w", AutostartLocation, err)
	}
	return ParseAutostartCommand(v), true, nil
}

// WriteAutostart записывает (или перезаписывает) значение автозапуска.
func WriteAutostart(e AutostartEntry) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, autostartRunKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open HKCU\\%s: %w", autostartRunKey, err)
	}
	defer debuglog.RunAndLog("WriteAutostart: close key", k.Close)
	if err := k.SetStringValue(AutostartValueName, e.Command()); err != nil {
		return fmt.Errorf("write %s: %w", AutostartLocation, err)
	}
	return nil
}

// DeleteAutostart удаляет значение автозапуска; отсутствие — не ошибка.
// Чьё это значение, проверяет вызывающий.
func DeleteAutostart() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, autostartRunKey, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("open HKCU\\%s: %w", autostartRunKey, err)
	}
	defer debuglog.RunAndLog("DeleteAutostart: close key", k.Close)
	if err := k.DeleteValue(AutostartValueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("delete %s: %w", AutostartLocation, err)
	}
	return nil
}
