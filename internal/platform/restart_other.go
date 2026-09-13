//go:build !windows
// +build !windows

package platform

import "errors"

// RestartSelf — самоперезапуск нужен только Windows: там смена рендерера
// требует нового процесса (OPENGL32.dll в таблице импорта exe, SPEC 125 §6.1).
// Вне Windows ни Mesa3D, ни кнопки переключения нет, и звать эту функцию
// некому.
func RestartSelf() error {
	return errors.New("restart is not supported on this platform")
}
