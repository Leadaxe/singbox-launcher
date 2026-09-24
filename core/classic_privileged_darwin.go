//go:build darwin

package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"singbox-launcher/internal/debuglog"
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

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	privilegedCopyMissingText     = "TUN mode starts the sing-box core as root. For safety the launcher runs only a root-owned copy of the core that your user account cannot modify, and there is no such copy yet."
	privilegedCopyOutdatedText    = "TUN mode starts the sing-box core as root from a root-owned copy, and the copy is not the launcher's current core (copy %s, launcher core %s) — for example after a core update."
	privilegedCopyUnsafeText      = "TUN mode starts the sing-box core as root only from a root-owned copy, and the copy's location failed the ownership check:\n%s\nIf the command below reports the same problem, fix the ownership of that path."
	privilegedCopyServiceNoteText = "The daemon service is installed on this Mac, so the command is its Install or update command: it refreshes the same copy and restarts the service."
	privilegedCopyInstructionText = "Run this command in Terminal (it asks for your sudo password), then click Retry:"
)

// privilegedCopyState — вердикт гейта (SPEC 137 §4).
type privilegedCopyState string

const (
	// privilegedCopyNoCore — ядро лаунчера не найдено или не читается:
	// сравнивать не с чем, и команде копирования нечего копировать.
	privilegedCopyNoCore privilegedCopyState = "no_core"
	// privilegedCopyMissing — копии (или её каталога) нет, а всё, что выше,
	// root-owned.
	privilegedCopyMissing privilegedCopyState = "missing"
	// privilegedCopyUnsafe — цепочка владения копии нарушена или копия не
	// прочиталась: запускать её под root нельзя.
	privilegedCopyUnsafe privilegedCopyState = "unsafe"
	// privilegedCopyOutdated — копия цела, но это не ядро лаунчера (sha).
	privilegedCopyOutdated privilegedCopyState = "outdated"
	// privilegedCopyOK — копия root-owned и совпадает с ядром лаунчера.
	privilegedCopyOK privilegedCopyState = "ok"
)

// privilegedCopyCheck — вердикт гейта и его основания. Detail — английская
// причина для лога и диалога.
type privilegedCopyCheck struct {
	State privilegedCopyState
	// CorePath — копия, которую исполнит root.
	CorePath string
	// LauncherCore — ядро лаунчера после EvalSymlinks.
	LauncherCore string
	Detail       string
	// CopySHA256 / LauncherSHA256 — hex sha256; пусто, если не считались.
	CopySHA256     string
	LauncherSHA256 string
}

// checkPrivilegedCoreCopy — гейт перед стартом с привилегиями. В отличие
// от классификатора службы закрыт по умолчанию: не посчитался хэш —
// старта нет. Порядок: ядро лаунчера → цепочка владения копии → sha.
func checkPrivilegedCoreCopy(l daemonServiceLayout, launcherCore string, hashes *fileHashCache) privilegedCopyCheck {
	c := privilegedCopyCheck{State: privilegedCopyOK, CorePath: l.CorePath}
	if launcherCore == "" {
		c.State = privilegedCopyNoCore
		c.Detail = "the launcher has no sing-box core"
		return c
	}
	resolved, err := filepath.EvalSymlinks(launcherCore)
	if err != nil {
		c.State = privilegedCopyNoCore
		c.Detail = fmt.Sprintf("sing-box core %s: %v", launcherCore, err)
		return c
	}
	c.LauncherCore = resolved
	launcherSum, err := hashes.sum(resolved)
	if err != nil {
		c.State = privilegedCopyNoCore
		c.Detail = fmt.Sprintf("cannot read sing-box core %s: %v", resolved, err)
		return c
	}
	c.LauncherSHA256 = launcherSum

	if err := checkRootOwnedChain(l.CorePath, l.ChainRoot, l.OwnerUID); err != nil {
		if errors.Is(err, errDaemonCopyMissing) {
			c.State = privilegedCopyMissing
		} else {
			c.State = privilegedCopyUnsafe
		}
		c.Detail = err.Error()
		if _, lerr := os.Lstat(l.LegacyPath); l.LegacyPath != "" && lerr == nil {
			c.Detail += fmt.Sprintf("; %s is a copy in the legacy layout of early lx.11 builds, not used: remove it (%s)",
				l.LegacyPath, legacyCopyRemoveCommand(l.LegacyPath))
		}
		return c
	}
	copySum, err := hashes.sum(l.CorePath)
	if err != nil {
		c.State = privilegedCopyUnsafe
		c.Detail = fmt.Sprintf("cannot read the root-owned copy %s: %v", l.CorePath, err)
		return c
	}
	c.CopySHA256 = copySum
	if copySum != launcherSum {
		c.State = privilegedCopyOutdated
		c.Detail = fmt.Sprintf("the root-owned copy %s (sha256 %s) is not the launcher core %s (sha256 %s)",
			l.CorePath, shortSHA(copySum), resolved, shortSHA(launcherSum))
	}
	return c
}

// privilegedCopyCommandFor — одна sudo-команда, которая создаёт или
// обновляет копию (SPEC 137 §5). Установлена служба демона — её команда
// install из SPEC 136: она обновляет ту же копию и перезапускает службу.
// Иначе — `lxd --service=copy` (lx.11): только копия и сайдкар, без plist.
// Бинарь — ядро лаунчера версии launcherVersion: ядро копирует себя само.
// Ядро, не умеющее копию (serviceCoreGate), — команды нет, ошибка
// *serviceCoreTooOldError: сначала обновить ядро.
func privilegedCopyCommandFor(l daemonServiceLayout, launcherCore, launcherVersion string) (command string, viaService bool, err error) {
	_, statErr := os.Lstat(l.PlistPath)
	viaService = statErr == nil
	if err := serviceCoreGate(launcherVersion); err != nil {
		return "", viaService, err
	}
	if viaService {
		return daemonServiceCommand(launcherCore, "lxd", "--service=install"), true, nil
	}
	return daemonServiceCommand(launcherCore, "lxd", "--service=copy"), false, nil
}

// privilegedCoreCopyGate — гейт перед AEWP: путь копии для старта или
// ошибка. Отказ по копии (missing / unsafe / outdated) пишется WARN с
// обоими sha и показывается диалогом с одной sudo-командой — или, если
// ядро лаунчера копию не умеет, с подсказкой сначала обновить ядро; тогда
// возвращается errPrivilegedCopyNotReady, и Start не добавляет «Failed to
// start sing-box». Нет ядра лаунчера — обычная ошибка старта.
func (ac *AppController) privilegedCoreCopyGate() (string, error) {
	l := systemDaemonServiceLayout()
	c := checkPrivilegedCoreCopy(l, ac.FileService.SingboxPath, &daemonServiceHashes)
	switch c.State {
	case privilegedCopyOK:
		debuglog.DebugLog("startSingBox: privileged start from the root-owned copy %s (sha256 %s)", c.CorePath, shortSHA(c.CopySHA256))
		return c.CorePath, nil
	case privilegedCopyNoCore:
		return "", errors.New(c.Detail)
	}
	version := ac.launcherCoreVersion()
	command, viaService, cmdErr := privilegedCopyCommandFor(l, ac.FileService.SingboxPath, version)
	var coreHint string
	if cmdErr != nil {
		coreHint = DaemonServiceCoreHint(version)
		debuglog.WarnLog("startSingBox: privileged start refused, core copy %s: %s (copy sha256 %s, launcher core sha256 %s); no command: %v",
			c.State, c.Detail, orUnknown(c.CopySHA256), orUnknown(c.LauncherSHA256), cmdErr)
	} else {
		debuglog.WarnLog("startSingBox: privileged start refused, core copy %s: %s (copy sha256 %s, launcher core sha256 %s); command: %s",
			c.State, c.Detail, orUnknown(c.CopySHA256), orUnknown(c.LauncherSHA256), command)
	}
	ac.showPrivilegedCopyDialog(c, command, viaService, coreHint)
	return "", errPrivilegedCopyNotReady
}

// orUnknown — sha для лога: пустое значение (не считалось) видно как «-».
func orUnknown(sum string) string {
	if sum == "" {
		return "-"
	}
	return sum
}

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

// notifyPrivilegedCopyAfterCoreUpdate — после скачивания ядра (SPEC 137
// §7) копия для старта с TUN отстаёт от нового ядра. При установленной
// службе диалог install уже показал notifyDaemonServiceAfterCoreUpdate: он
// обновляет ту же копию. Иначе — WARN с обоими sha, а ближайший старт с TUN
// не пройдёт гейт и покажет диалог с командой. Копии нет — молчим: её
// попросит первый старт с TUN.
func (ac *AppController) notifyPrivilegedCopyAfterCoreUpdate() {
	l := systemDaemonServiceLayout()
	if _, err := os.Lstat(l.PlistPath); err == nil {
		return
	}
	c := checkPrivilegedCoreCopy(l, ac.FileService.SingboxPath, &daemonServiceHashes)
	if c.State != privilegedCopyOutdated {
		return
	}
	command, _, err := privilegedCopyCommandFor(l, ac.FileService.SingboxPath, ac.launcherCoreVersion())
	if err != nil {
		debuglog.WarnLog("core updated: the root-owned copy for the privileged (TUN) start is outdated (copy sha256 %s, launcher core sha256 %s); no command: %v",
			c.CopySHA256, c.LauncherSHA256, err)
		return
	}
	debuglog.WarnLog("core updated: the root-owned copy for the privileged (TUN) start is outdated (copy sha256 %s, launcher core sha256 %s); the next TUN start asks to run: %s",
		c.CopySHA256, c.LauncherSHA256, command)
}
