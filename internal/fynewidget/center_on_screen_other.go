//go:build !darwin

package fynewidget

// centerBlocker на Windows и Linux ничего не запрещает: центрирование как было.
// Паника на nil-мониторе там возможна только без единого подключённого экрана:
// спящий дисплей GLFW там из списка мониторов не выбрасывает, в отличие от
// macOS (CGDisplayIsAsleep в _glfwPollMonitorsCocoa).
func centerBlocker() string { return "" }
