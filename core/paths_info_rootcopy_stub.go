//go:build !(darwin || (windows && !386))

package core

import "singbox-launcher/internal/paths"

// fillRootCopyStatus — на платформах без привилегированного root-owned
// запуска (Linux и т. п.) защищённой копии нет: блок остаётся пустым, и UI
// его не показывает.
func (ac *AppController) fillRootCopyStatus(info *paths.PathsInfo) {}
