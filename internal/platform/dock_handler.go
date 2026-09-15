//go:build darwin
// +build darwin

package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework Foundation

// Реализация — dock_handler_darwin.m. Здесь только объявления: файл
// экспортирует Go-функции (//export), а такую преамбулу cgo копирует в два
// C-файла, и любое определение попало бы в сборку дважды.
void launcherInstallAppDelegate(void);
void launcherRemoveAppDelegate(void);
void launcherReplyToTerminate(void);
void launcherHideDockIcon(void);
void launcherRestoreDockIcon(void);
*/
import "C"

import (
	"runtime"
	"sync"
	"time"

	"singbox-launcher/internal/debuglog"
)

var dockReopenCallback func()

//export callGoDockCallback
func callGoDockCallback() {
	if dockReopenCallback != nil {
		dockReopenCallback()
	}
}

// SetupDockReopenHandler ставит приложению свой делегат NSApplication: клик
// по иконке в Dock при скрытом окне вызывает showWindowCallback, а запрос
// macOS на завершение идёт в обработчик SetQuitRequestHandler.
//
// # Ловушка: замена делегата
//
// Делегат у NSApp один, и до нас его ставит GLFW (glfwInit, первое окно
// Fyne). У GLFWApplicationDelegate есть методы, без которых GLFW тихо
// ломается:
//
//   - applicationDidChangeScreenParameters: — единственное место, где GLFW
//     после старта перечитывает список мониторов. Без него список навсегда
//     остаётся таким, каким был при запуске: лаунчер, запущенный при спящем
//     дисплее, не видел мониторов и после пробуждения (fynewidget.CenterOnScreen).
//   - applicationDidHide: — возвращает видеорежимы мониторов.
//   - applicationShouldTerminate: — шлёт всем окнам запрос на закрытие и
//     отвечает NSTerminateCancel; окно SystrayMonitor на этот запрос гасит
//     цикл событий Fyne.
//
// Прежний делегат лаунчера этих методов не знал и заменял GLFW целиком. Без
// applicationShouldTerminate: AppKit отвечал NSTerminateNow, и Cmd+Q,
// «Завершить» в Dock и выход из системы завершали процесс сразу, через
// exit(): GracefulExit не выполнялся, и в classic-режиме ядро оставалось
// работать без лаунчера вместе с маршрутами TUN.
//
// Поэтому делегат оборачивает прежний: всё, чего он не реализует сам,
// перенаправляется делегату GLFW. Сам он отвечает только за reopen и
// applicationShouldTerminate:. Ответ GLFW (NSTerminateCancel) для выхода
// из системы означал бы отмену выхода, поэтому здесь его нет.
//
// Вызывать с главного потока после инициализации GLFW, то есть после
// создания первого окна: вызванный раньше, делегат нечего было бы
// оборачивать, а glfwInit потом заменил бы его своим.
func SetupDockReopenHandler(showWindowCallback func()) {
	if runtime.GOOS != "darwin" {
		return
	}
	if showWindowCallback == nil {
		debuglog.DebugLog("SetupDockReopenHandler: callback is nil, skipping setup")
		return
	}

	// Store callback in Go variable
	dockReopenCallback = showWindowCallback

	C.launcherInstallAppDelegate()
	debuglog.InfoLog("SetupDockReopenHandler: app delegate installed (Dock reopen, quit request; other calls go to GLFW's delegate)")
}

// CleanupDockReopenHandler снимает делегат лаунчера.
func CleanupDockReopenHandler() {
	if runtime.GOOS != "darwin" {
		return
	}
	C.launcherRemoveAppDelegate()
	dockReopenCallback = nil
	debuglog.InfoLog("CleanupDockReopenHandler: Dock reopen handler cleaned up")
}

// quitRequest — что выполнить, когда macOS просит приложение завершиться, и
// сколько этого ждать. Пишется из main до цикла событий, читается делегатом
// на главном потоке.
var quitRequest struct {
	sync.Mutex
	handler func()
	budget  time.Duration
	started bool
}

// SetQuitRequestHandler задаёт, что выполняется перед выходом по запросу
// macOS: Cmd+Q, «Завершить» в меню Dock, Apple Event quit, выход из системы,
// перезагрузка, выключение. Все они приходят в делегат как
// applicationShouldTerminate: (SetupDockReopenHandler).
//
// handler выполняется в отдельной горутине, пока AppKit ждёт ответа
// (NSTerminateLater). Главный поток всё это время стоит во вложенном цикле
// AppKit, и цикл событий Fyne не крутится: handler не должен ждать главного
// потока (fyne.DoAndWait), иначе дождётся только бюджета. fyne.Do можно:
// он лишь ставит функцию в очередь, до которой дело уже не дойдёт.
//
// Ответ «можно завершаться» уходит, как только handler вернулся, но не позже
// budget: выход из системы не должен ждать лаунчер. Отменить выход нельзя
// ни при каком исходе. После ответа AppKit сам вызывает exit(), и остаток
// main() после цикла событий не выполняется.
func SetQuitRequestHandler(handler func(), budget time.Duration) {
	quitRequest.Lock()
	defer quitRequest.Unlock()
	quitRequest.handler = handler
	quitRequest.budget = budget
}

// launcherQuitRequested вызывается делегатом на главном потоке. 1 — завершать
// сразу (NSTerminateNow), 0 — ответ придёт позже (NSTerminateLater).
//
//export launcherQuitRequested
func launcherQuitRequested() C.int {
	quitRequest.Lock()
	handler, budget, repeated := quitRequest.handler, quitRequest.budget, quitRequest.started
	if handler != nil {
		quitRequest.started = true
	}
	quitRequest.Unlock()

	if handler == nil {
		return 1
	}
	if repeated {
		// Пока ответ не отправлен, AppKit повторно делегата не спрашивает:
		// второй Apple Event quit получает от него отказ -128, второй
		// terminate: завершает процесс сам. Сюда попадает только запрос после
		// уже отправленного ответа — ждать больше нечего.
		debuglog.WarnLog("quit request: repeated after shutdown started, quitting now")
		return 1
	}
	go finishQuitRequest(handler, budget)
	return 0
}

// finishQuitRequest выполняет handler и отвечает AppKit по его окончании
// или по истечении budget — что наступит раньше.
func finishQuitRequest(handler func(), budget time.Duration) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler()
	}()
	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		debuglog.WarnLog("quit request: shutdown did not finish within %s, quitting anyway (a running core may be left behind)", budget)
	}
	C.launcherReplyToTerminate()
}

// HideDockIcon hides the Dock icon on macOS (tray-only mode)
func HideDockIcon() {
	if runtime.GOOS != "darwin" {
		return
	}
	C.launcherHideDockIcon()
	debuglog.InfoLog("HideDockIcon: Dock icon hidden (NSApplicationActivationPolicyAccessory)")
}

// RestoreDockIcon restores the Dock icon on macOS (makes app appear in Dock again)
func RestoreDockIcon() {
	if runtime.GOOS != "darwin" {
		return
	}
	C.launcherRestoreDockIcon()
	debuglog.InfoLog("RestoreDockIcon: Dock icon restored (NSApplicationActivationPolicyRegular)")
}
