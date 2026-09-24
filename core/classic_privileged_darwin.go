//go:build darwin

package core

import (
	"errors"
	"os"
	"strings"

	"fyne.io/fyne/v2"

	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
)

// Гейт привилегированного старта classic-движка (SPEC 137).
//
// Ядро с TUN стартует под root через AEWP, и root исполняет только
// root-owned копию ядра — ту же, что запускает служба демона (SPEC 136,
// раскладка lx.11). Перед стартом лаунчер без root проверяет цепочку
// владения копии и сверяет её sha256 с ядром лаунчера; не прошло — старта
// с привилегиями нет, пользователь получает одну sudo-команду, которая
// создаёт или обновляет копию. Лаунчер ничего не копирует сам.
//
// Здесь — платформенная часть (общая — classic_privileged.go): тексты
// диалога («root», Terminal, sudo) и сам диалог отказа гейта.

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	privilegedCopyMissingText     = "TUN mode starts the sing-box core as root. For safety the launcher runs only a root-owned copy of the core that your user account cannot modify, and there is no such copy yet."
	privilegedCopyOutdatedText    = "TUN mode starts the sing-box core as root from a root-owned copy, and the copy is not the launcher's current core (copy %s, launcher core %s) — for example after a core update."
	privilegedCopyUnsafeText      = "TUN mode starts the sing-box core as root only from a root-owned copy, and the copy's location failed the ownership check:\n%s\nIf the command below reports the same problem, fix the ownership of that path."
	privilegedCopyServiceNoteText = "The daemon service is installed on this Mac, so the command is its Install or update command: it refreshes the same copy and restarts the service."
	privilegedCopyInstructionText = "Run this command in Terminal (it asks for your sudo password), then click Retry:"
)

// showPrivilegedCopyDialog — диалог отказа гейта (SPEC 137 §5): причина,
// одна sudo-команда, Copy the command / Run in Terminal / Retry / Close.
// Retry повторяет Start тем же путём, что кнопка Start. command == "" (ядро
// лаунчера копию не умеет) — вместо команды coreHint, кнопка одна: Close.
func (ac *AppController) showPrivilegedCopyDialog(c privilegedCopyCheck, command string, viaService bool, coreHint string) {
	if !ac.hasUI() {
		return
	}
	var title, reason string
	switch c.State {
	case privilegedCopyMissing:
		title = locale.T("Core copy for privileged start is missing")
		reason = locale.T(privilegedCopyMissingText)
	case privilegedCopyOutdated:
		title = locale.T("Core copy for privileged start is outdated")
		reason = locale.Tf(privilegedCopyOutdatedText, shortSHA(c.CopySHA256), shortSHA(c.LauncherSHA256))
	default:
		title = locale.T("Core copy for privileged start is not protected")
		reason = locale.Tf(privilegedCopyUnsafeText, c.Detail)
	}
	parts := []string{reason}
	if command == "" {
		parts = append(parts, coreHint)
		dialogs.ShowCommandRetry(ac.UIService.MainWindow, title, strings.Join(parts, "\n\n"), "", nil, nil)
		return
	}
	if viaService {
		parts = append(parts, locale.T(privilegedCopyServiceNoteText))
	}
	parts = append(parts, locale.T(privilegedCopyInstructionText))
	dialogs.ShowCommandRetry(ac.UIService.MainWindow, title, strings.Join(parts, "\n\n"), command,
		ac.OpenTerminalWithCommand, func() { go StartSingBoxProcess() })
}

// classicElevatedUsesCopy — на macOS привилегированный старт идёт своим
// путём (TUN → AEWP, startSingBoxPrivileged), не через повышенный токен.
func classicElevatedUsesCopy() bool { return false }

// elevatedClassicStart — только Windows (SPEC 141 §8).
func (ac *AppController) elevatedClassicStart() (string, *os.File, error) {
	return "", nil, errors.New("elevated classic start is Windows-only")
}

// tunInstallServiceAction — диалога «TUN без прав» на macOS нет.
func (ac *AppController) tunInstallServiceAction() (dialogs.Action, bool) {
	return dialogs.Action{}, false
}

// showDaemonCoreUpdatedDialog — «Core updated»: sudo-команда install для
// Terminal.
func (ac *AppController) showDaemonCoreUpdatedDialog(command string) {
	dialogs.ShowLinuxCapabilitiesRequired(ac.UIService.MainWindow,
		locale.T("Core updated — update the daemon service"),
		locale.T(daemonCoreUpdatedBodyText),
		command)
}

// ShowDaemonUnsafeNoticeElevated — на macOS модальное предупреждение
// показывает main.go (sudo-команда для Terminal).
func (ac *AppController) ShowDaemonUnsafeNoticeElevated(_ fyne.Window, _, _, _ string) bool {
	return false
}
