//go:build darwin

package ui

import "singbox-launcher/core"

// На macOS лаунчер умеет открыть Terminal.app с подставленной командой:
// пользователь видит полный вывод и вводит свой sudo сам.
func init() {
	openTerminal = func(cmd string) error {
		ac := core.GetController()
		if ac == nil {
			return nil
		}
		return ac.OpenTerminalWithCommand(cmd)
	}
}

// Подписи и тексты панели daemon-движка (connection_local_daemon.go) на
// macOS: команды службы пользователь выполняет в Terminal под своим sudo.
// Windows — command_row_windows.go.
const (
	// Подписи под заголовками шагов окна Service: где и как исполняется
	// команда (заголовок шага ставит окно).
	daemonInstallRowLabel = "Runs in Terminal with your sudo; a first install prints a pairing invite at the end:" // l10n-key

	daemonOpRowLabel = "Runs in Terminal with your sudo:" // l10n-key

	daemonFreshInviteRowLabel = "Runs in Terminal with your sudo and prints a one-time invite — paste it below:" // l10n-key

	daemonUninstallStepLabel = "2. Remove the service (run in Terminal, your sudo):" // l10n-key

	daemonPairHelpText = "Paste the invite printed by the daemon and click Pair. Where to get one:\n\n- Installing the service prints an invite at the end of its Terminal output (Core tab).\n- For a fresh invite run the command below (copy or open in Terminal), then paste the printed invite into the pairing field.\n\nThe code is one-time: it burns after a successful pairing. The secret field is only for daemons running without TLS." // l10n-key

	daemonServiceUnsafeText = "The service runs a binary your user can modify. Install or update the service to move it to a root-owned copy." // l10n-key

	daemonServiceManagerText = "launchd reports: %s" // l10n-key

	daemonPurgeSurvivesLabel = "The daemon service is installed and is not removed with the data: it runs from its own root-owned copy of the core. To remove the service as well, run this command:" // l10n-key

	daemonPurgeFirstLabel = "The daemon service is installed. Remove it first with this command, otherwise the launcher will not be able to do it after the data is gone:" // l10n-key
)
