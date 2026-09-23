package platform

// Канал «на машине появилось аппаратное OpenGL» между фоновой пробой гейта и
// UI (SPEC 125 §2.5). Без build-тегов: вне Windows фоновая проба не
// запускается, и notifyHardwareGLAvailable никто не зовёт.

import (
	"sync"

	"singbox-launcher/internal/debuglog"
)

var (
	hwGLMu sync.Mutex
	// hwGLCallback читается только в notifyHardwareGLAvailable, который зовёт
	// один windows-файл, — вне Windows вся цепочка выглядит мёртвой.
	hwGLCallback func(renderer string) //nolint:unused // см. notifyHardwareGLAvailable
	// hwGLPending — результат, пришедший раньше регистрации обработчика.
	// Гонка реальна: фоновая проба стартует в гейте (до NewWindow), а
	// подписка — в SetOnStarted. Потерянный результат означал бы, что
	// предложение вернуться на железо не приходит до следующего старта.
	hwGLPending string
)

// SetOnHardwareGLAvailable регистрирует обработчик «железо доступно».
// Если проба уже успела ответить, обработчик вызывается сразу же.
func SetOnHardwareGLAvailable(fn func(renderer string)) {
	hwGLMu.Lock()
	hwGLCallback = fn
	pending := hwGLPending
	hwGLPending = ""
	hwGLMu.Unlock()

	if fn != nil && pending != "" {
		fn(pending)
	}
}

// notifyHardwareGLAvailable вызывается фоновой пробой гейта, когда лаунчер
// рисует через Mesa, а железо ответило >= 2.1.
func notifyHardwareGLAvailable(renderer string) { //nolint:unused // вызывается только из glprobe_windows.go (//go:build windows)
	hwGLMu.Lock()
	fn := hwGLCallback
	if fn == nil {
		hwGLPending = renderer
	}
	hwGLMu.Unlock()

	debuglog.WarnLog("gl: hardware OpenGL is available while rendering via Mesa3D (renderer=%q)", renderer)
	if fn != nil {
		fn(renderer)
	}
}
