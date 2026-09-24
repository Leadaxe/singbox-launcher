//go:build !darwin && (!windows || 386)

package core

// daemonUninstallHint — службы демона вне daemon-платформ (Linux, Win7) нет.
func (ac *AppController) daemonUninstallHint() (string, bool) { return "", false }

// daemonUninstallHintFor — службы демона вне daemon-платформ (Linux, Win7) нет.
func daemonUninstallHintFor(corePath string) (string, bool) {
	_ = corePath
	return "", false
}
