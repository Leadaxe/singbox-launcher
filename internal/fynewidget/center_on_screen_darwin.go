//go:build darwin

package fynewidget

/*
#cgo LDFLAGS: -framework CoreGraphics
#include <CoreGraphics/CoreGraphics.h>

// launcherHasAwakeDisplay повторяет отбор мониторов GLFW (_glfwPollMonitorsCocoa
// в cocoa_monitor.m): экраном считается online-дисплей, который не спит.
static int launcherHasAwakeDisplay(void) {
	CGDirectDisplayID ids[64];
	uint32_t count = 0;
	if (CGGetOnlineDisplayList(64, ids, &count) != kCGErrorSuccess) {
		return 0;
	}
	for (uint32_t i = 0; i < count; i++) {
		if (!CGDisplayIsAsleep(ids[i])) {
			return 1;
		}
	}
	return 0;
}
*/
import "C"

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"github.com/go-gl/glfw/v3.4/glfw"
)

// centerBlocker возвращает причину не центрировать окно или "".
//
// Проверок две, и нужны обе.
//
// CoreGraphics отвечает, есть ли неспящий дисплей прямо сейчас. GLFW при
// опросе пропускает спящие дисплеи (CGDisplayIsAsleep), поэтому при спящем
// дисплее у него нет ни одного монитора.
//
// Список самого GLFW нужен потому, что он обновляется не сразу. GLFW
// опрашивает мониторы при glfwInit (создание первого окна) и в
// applicationDidChangeScreenParameters своего делегата NSApplication, то есть
// только когда AppKit сообщит о смене экранов. Пока сообщения не было,
// CoreGraphics уже видит проснувшийся экран, а у GLFW список пустой, и Show()
// упал бы. Делегат лаунчера пересылает этот вызов делегату GLFW
// (platform.SetupDockReopenHandler). До 1.6.0 он его терял, и лаунчер,
// запущенный при спящем дисплее (например, с -tray), жил с пустым списком
// до перезапуска.
func centerBlocker() string {
	app := fyne.CurrentApp()
	if app == nil {
		return ""
	}
	if _, ok := app.Driver().(desktop.Driver); !ok {
		// Не GLFW-драйвер (тестовый): его CenterOnScreen мониторов не трогает.
		return ""
	}
	if C.launcherHasAwakeDisplay() == 0 {
		return "no awake display"
	}
	if !glfwHasMonitor() {
		return "GLFW has no monitor (its list is taken at startup, when no display was awake)"
	}
	return ""
}

// glfwHasMonitor сообщает, есть ли у GLFW основной монитор: без него
// getMonitorForWindow в Fyne возвращает nil. После завершения GLFW (выход из
// приложения) go-gl паникует — это тоже «центрировать некуда».
func glfwHasMonitor() (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return glfw.GetPrimaryMonitor() != nil
}
