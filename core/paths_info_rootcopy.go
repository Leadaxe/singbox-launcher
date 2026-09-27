//go:build darwin || (windows && !386)

package core

import "singbox-launcher/internal/paths"

// fillRootCopyStatus заполняет блок защищённой root-owned копии (SPEC 143).
//
// Проверка та же, что у гейта привилегированного старта
// (checkPrivilegedCoreCopy): цепочка владения от корня раскладки и sha256
// против ядра лаунчера. Нужна, чтобы UI показал состояние копии заранее, а
// не только диалогом при отказе старта: «копия не текущее ядро —
// синхронизируйте» должно быть видно до нажатия Start с TUN.
func (ac *AppController) fillRootCopyStatus(info *paths.PathsInfo) {
	if ac == nil || ac.FileService == nil {
		return
	}
	l := systemDaemonServiceLayout()
	if l.CorePath == "" {
		return
	}
	info.RootCopyPath = l.CorePath
	c := checkPrivilegedCoreCopy(l, ac.FileService.SingboxPath, &daemonServiceHashes)
	info.RootCopyState = string(c.State)
	info.RootCopyDetail = c.Detail
}
