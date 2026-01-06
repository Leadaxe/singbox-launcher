// Package presentation содержит слой представления визарда конфигурации.
//
// Файл utils.go содержит утилиты для работы с GUI в контексте визарда:
//   - SafeFyneDo - безопасный вызов fyne.Do из других горутин
//
// SafeFyneDo гарантирует, что обновление GUI происходит в UI потоке Fyne,
// что необходимо при работе из горутин (например, при асинхронных операциях).
package presentation

import (
	"fyne.io/fyne/v2"
)

// SafeFyneDo безопасно выполняет функцию в UI потоке Fyne.
// Проверяет, что window не nil, перед вызовом fyne.Do.
func SafeFyneDo(window fyne.Window, fn func()) {
	if window != nil {
		fyne.Do(fn)
	}
}


