//go:build !darwin

package core

import "errors"

// findSingboxRunProcessDarwin — заглушка для не-darwin платформ; ошибка
// уводит caller на общий имя-ориентированный скан.
func findSingboxRunProcessDarwin() (bool, int, error) {
	return false, -1, errors.New("darwin only")
}
