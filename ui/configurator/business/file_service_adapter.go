// Package business содержит бизнес-логику визарда конфигурации.
//
// FileServiceAdapter адаптирует core/services.FileService для использования в бизнес-логике.
// Реализует интерфейс FileServiceInterface (interfaces.go). Определён в файле без build tag,
// чтобы wizard.go и другие вызывающие пакеты собирались и при сборке без cgo (например, линтер).
package business

import (
	"singbox-launcher/core/services"
)

// FileServiceAdapter адаптирует services.FileService для использования в бизнес-логике.
// Реализует интерфейс FileServiceInterface, определенный в interfaces.go.
type FileServiceAdapter struct {
	FileService *services.FileService
}

func (a *FileServiceAdapter) ConfigPath() string {
	return a.FileService.ConfigPath
}

func (a *FileServiceAdapter) ExecDir() string {
	return a.FileService.ExecDir
}

func (a *FileServiceAdapter) SingboxPath() string {
	return a.FileService.SingboxPath
}
