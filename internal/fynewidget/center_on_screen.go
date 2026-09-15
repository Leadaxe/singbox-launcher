package fynewidget

import (
	"sync"

	"fyne.io/fyne/v2"

	"singbox-launcher/internal/debuglog"
)

// CenterOnScreen центрирует окно, только если есть экран, на который его можно
// поставить. Во всём лаунчере окна центрируются через него, а не через
// w.CenterOnScreen() напрямую.
//
// Зачем обёртка. В Fyne 2.8.1 doCenterOnScreen
// (internal/driver/glfw/window_desktop.go:202-203) берёт getMonitorForWindow()
// и сразу зовёт у результата GetVideoMode() — без проверки на nil. Монитор
// оказывается nil, когда GLFW не видит ни одного экрана: GetPrimaryMonitor()
// возвращает nil, и go-gl разыменовывает nil-указатель. У ещё не показанного окна
// центрирование откладывается до создания, поэтому паника прилетает из первого
// Show(), а не из этого вызова. Остальные пути Fyne, которые трогают монитор при
// создании и показе окна (detectScale, размещение рядом с соседним окном),
// nil проверяют; полноэкранный режим на macOS обходится без монитора.
//
// Без центрирования окно не теряется: на macOS GLFW сам ставит новое окно
// в центр экрана ([NSWindow center]), а Fyne лишь подгоняет позицию под
// итоговый размер.
func CenterOnScreen(w fyne.Window) {
	if w == nil {
		return
	}
	if reason := centerBlocker(); reason != "" {
		logCenterSkip(reason)
		return
	}
	w.CenterOnScreen()
}

var (
	centerSkipMu     sync.Mutex
	centerSkipLogged = map[string]bool{}
)

// logCenterSkip пишет пропуск центрирования один раз на причину: окна в таком
// состоянии открываются пачками, а суть видна из первой строки.
func logCenterSkip(reason string) {
	centerSkipMu.Lock()
	first := !centerSkipLogged[reason]
	centerSkipLogged[reason] = true
	centerSkipMu.Unlock()
	if first {
		debuglog.WarnLog("CenterOnScreen skipped: %s", reason)
	}
}
