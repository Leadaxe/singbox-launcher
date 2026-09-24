//go:build windows && !386

package ui

// Подписи и тексты панели daemon-движка (connection_local_daemon.go) на
// Windows (SPEC 141 §9): терминала нет (openTerminal = nil), операцию службы
// запускает кнопка «Run as administrator» — core исполняет команду через
// окно UAC (core.DaemonOpsElevated); команда в поле — для Copy в консоль
// администратора. Kickstart на Windows нет: install и copy сами
// перезапускают службу. macOS — command_row_darwin.go.
const (
	daemonInstallRowLabel = "Install or update the service (Windows asks for administrator rights):" // l10n-key

	// NotRunning: `sc.exe start sing-box-lxd`, подпись несёт кнопка «Start
	// the service».
	daemonStartRowLabel = ""

	daemonInstallStepLabel = "1. Install or update the service (Windows asks for administrator rights; the launcher pairs with it automatically):" // l10n-key

	daemonPairStepLabel = "2. If pairing did not happen automatically, paste an invite (address#fingerprint#code) and pair:" // l10n-key

	daemonUninstallStepLabel = "2. Remove the service (Windows asks for administrator rights):" // l10n-key

	daemonPairHelpText = "Paste the invite printed by the daemon and click Pair. Installing or updating the service pairs the launcher automatically, so an invite is only needed when that did not happen:\n\n- \"Need a fresh invite\" on the Install tab issues a new invite and pairs with it (Windows asks for administrator rights).\n- Or run the command below in a terminal opened as administrator, then paste the printed invite into the pairing field.\n\nThe code is one-time: it burns after a successful pairing. The secret field is only for daemons running without TLS." // l10n-key

	daemonServiceUnsafeText = "The service runs a binary your user can modify. Install or update the service to move it to a protected copy." // l10n-key

	daemonServiceManagerText = "Service Control Manager reports: %s" // l10n-key

	daemonPurgeSurvivesLabel = "The daemon service is installed and is not removed with the data: it runs from its own protected copy of the core. To remove the service as well, run this command as administrator:" // l10n-key

	daemonPurgeFirstLabel = "The daemon service is installed. Remove it first with this command as administrator, otherwise the launcher will not be able to do it after the data is gone:" // l10n-key
)
