//go:build !windows
// +build !windows

package platform

// RegisterInstance — метки экземпляра и событие Quit для установщика
// Windows (SPEC 140 §4). Вне Windows установщика нет.
func RegisterInstance(onQuit func()) {}
