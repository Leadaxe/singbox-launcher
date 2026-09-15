//go:build !darwin

package fynewidget

// centerBlocker на Windows и Linux ничего не запрещает: центрирование как было.
// Паника на nil-мониторе там возможна только без единого подключённого экрана,
// а список мониторов GLFW обновляется по системным событиям и не застревает
// в состоянии на момент запуска, как на macOS.
func centerBlocker() string { return "" }
