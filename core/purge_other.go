//go:build !darwin

package core

// daemonUninstallHint — служба демона есть только на macOS.
func (ac *AppController) daemonUninstallHint() string { return "" }

// daemonUninstallHintFor — служба демона есть только на macOS.
func daemonUninstallHintFor(corePath string) string {
	_ = corePath
	return ""
}
