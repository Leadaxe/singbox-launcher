//go:build !darwin

package core

// daemonUninstallHint — служба демона есть только на macOS.
func (ac *AppController) daemonUninstallHint() (string, bool) { return "", false }

// daemonUninstallHintFor — служба демона есть только на macOS.
func daemonUninstallHintFor(corePath string) (string, bool) {
	_ = corePath
	return "", false
}
