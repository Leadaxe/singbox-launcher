//go:build darwin

package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"singbox-launcher/internal/debuglog"
)

// Гейт привилегированного старта classic-движка (SPEC 137).
//
// Ядро с TUN стартует под root через AEWP, и root исполняет только
// root-owned копию ядра — ту же, что запускает служба демона (SPEC 136,
// раскладка lx.11). Перед стартом лаунчер без root проверяет цепочку
// владения копии и сверяет её sha256 с ядром лаунчера; не прошло — старта
// с привилегиями нет, пользователь получает одну sudo-команду, которая
// создаёт или обновляет копию. Лаунчер ничего не копирует сам.

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
// Бинарь — ядро лаунчера: ядро копирует себя само.
func privilegedCopyCommandFor(l daemonServiceLayout, launcherCore string) (command string, viaService bool) {
	if _, err := os.Lstat(l.PlistPath); err == nil {
		return daemonServiceCommand(launcherCore, "lxd", "--service=install"), true
	}
	return daemonServiceCommand(launcherCore, "lxd", "--service=copy"), false
}

// privilegedCoreCopyGate — гейт перед AEWP: путь копии для старта или
// ошибка. Отказ пишется WARN с обоими sha.
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
	command, _ := privilegedCopyCommandFor(l, ac.FileService.SingboxPath)
	debuglog.WarnLog("startSingBox: privileged start refused, core copy %s: %s (copy sha256 %q, launcher core sha256 %q); run: %s",
		c.State, c.Detail, c.CopySHA256, c.LauncherSHA256, command)
	return "", fmt.Errorf("the root-owned core copy for the privileged start is %s: %s. Run in Terminal: %s", c.State, c.Detail, command)
}
