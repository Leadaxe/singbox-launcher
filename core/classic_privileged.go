//go:build darwin || (windows && !386)

package core

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	"singbox-launcher/internal/debuglog"
)

// Гейт привилегированного старта classic-движка (SPEC 137), общая часть.
//
// Ядро с TUN стартует с повышенными правами и исполняет только защищённую
// копию ядра — ту же, что запускает служба демона (SPEC 136). Перед стартом
// лаунчер без прав проверяет цепочку владения копии и сверяет её sha256 с
// ядром лаунчера; не прошло — старта с правами нет, пользователь получает
// одну команду, которая создаёт или обновляет копию. Лаунчер ничего не
// копирует сам. Здесь — вердикты, выбор команды и оркестрация гейта;
// диалог с текстами платформы — classic_privileged_<os>.go.

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

	if err := checkDaemonCopyChain(l); err != nil {
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
		return c
	}
	// Остальные члены набора (Windows, SPEC 141 §8): закрыт по умолчанию —
	// не посчитался хэш — старта нет.
	name, _, detail, err := daemonSetMismatch(l.CorePath, resolved, hashes)
	switch {
	case err != nil:
		c.State = privilegedCopyUnsafe
		c.Detail = err.Error()
	case name != "":
		c.State = privilegedCopyOutdated
		c.Detail = detail
	}
	return c
}

// privilegedCopyCommandFor — одна sudo-команда, которая создаёт или
// обновляет копию (SPEC 137 §5). Установлена служба демона — её команда
// install из SPEC 136: она обновляет ту же копию и перезапускает службу.
//
// Иначе копию надо положить без демона. Способ зависит от того, что это за
// ядро (SPEC 143):
//
//   - ядро форка с `lxd --service=copy` и опознанной версией — копирует
//     себя само: штатный путь SPEC 137;
//   - опознанный релиз форка СТАРШЕ порога (например 1.14.1-lx.8) — команды
//     нет: такой релиз копию ещё не умеет, версия форка обещает свою
//     раскладку, и подменять её копией от лаунчера нельзя. Поведение
//     прежнее: подсказка обновить ядро;
//   - НЕопознанная версия (кастомная сборка без `-lx.N`, например
//     1.15.0-jiejie-masquerade.5) — копию кладёт сам лаунчер одной
//     проверяемой командой. Classic run не требует от ядра поддержки lxd
//     (SPEC 143 §5.2), а версия о способностях не говорит ничего, поэтому
//     решает проба бинаря, а не строка.
//
// Гейт по версии (serviceCoreGate) остаётся для пути СЛУЖБЫ: установка
// службы — это протокол демона, и без lxd она невозможна.
func privilegedCopyCommandFor(l daemonServiceLayout, launcherCore, launcherVersion string) (command string, viaService bool, err error) {
	viaService = daemonServiceDefined(l)
	if viaService {
		// Служба установлена: её install — единственный корректный путь,
		// и он действительно требует lxd у ядра.
		if err := serviceCoreGate(launcherVersion); err != nil {
			return "", true, err
		}
		return daemonServiceCommand(launcherCore, daemonInstallArgs()...), true, nil
	}
	// Опознанный релиз форка не старше порога — штатный путь SPEC 137: ядро
	// копирует себя само (`lxd --service=copy`). Гейт по версии здесь и есть
	// доказательство способности: порог `minCoreForRootOwnedService` введён
	// именно как «первое ядро форка, чей lxd умеет --service=copy».
	if serviceCoreGate(launcherVersion) == nil {
		return daemonServiceCommand(launcherCore, "lxd", "--service=copy"), false, nil
	}
	// Версия опознана, но старше порога: команды нет (прежнее поведение) —
	// версия форка обещает свою раскладку копии.
	if _, recognized := parseCoreBuild(launcherVersion); recognized {
		return "", false, &serviceCoreTooOldError{version: launcherVersion}
	}
	// Версия не опознана — кастомная сборка. Classic TUN не требует lxd, а
	// версия о способностях не говорит: спрашиваем сам бинарь. Умеет
	// копировать себя — его штатная команда; нет — копию кладём сами.
	if coreSupportsServiceCopy(launcherCore) {
		return daemonServiceCommand(launcherCore, "lxd", "--service=copy"), false, nil
	}
	debuglog.DebugLog("privilegedCopyCommandFor: core %q has an unrecognized version %q and cannot copy itself; using the launcher install command", launcherCore, launcherVersion)
	return rootCopyInstallCommand(launcherCore, l.CorePath), false, nil
}

// rootCopyInstallCommand — sudo-команда, которая кладёт копию ядра в
// защищённый каталог от root, не полагаясь на поддержку lxd ядром.
//
// Команда намеренно минимальна и проверяема человеком: install с
// владельцем root:wheel и правами 0755, во временный файл рядом с целью с
// последующим переименованием, чтобы работающий экземпляр никогда не
// наблюдал полузаписанный бинарь. Целевой каталог создаётся при необходимости.
func rootCopyInstallCommand(src, dst string) string {
	dir := path.Dir(dst)
	// cp во временный файл в ТОМ ЖЕ каталоге + chown/chmod + atomic mv.
	// Никаких симлинков: mv по одному каталогу атомарен.
	return "sudo /bin/mkdir -p " + shellQuote(dir) +
		" && sudo /bin/cp -f " + shellQuote(src) + " " + shellQuote(dst+".new") +
		" && sudo /usr/sbin/chown root:wheel " + shellQuote(dst+".new") +
		" && sudo /bin/chmod 0755 " + shellQuote(dst+".new") +
		" && sudo /bin/mv -f " + shellQuote(dst+".new") + " " + shellQuote(dst)
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

// notifyPrivilegedCopyAfterCoreUpdate — после скачивания ядра (SPEC 137
// §7) копия для старта с TUN отстаёт от нового ядра. При установленной
// службе диалог install уже показал notifyDaemonServiceAfterCoreUpdate: он
// обновляет ту же копию. Иначе — WARN с обоими sha, а ближайший старт с TUN
// не пройдёт гейт и покажет диалог с командой. Копии нет — молчим: её
// попросит первый старт с TUN.
func (ac *AppController) notifyPrivilegedCopyAfterCoreUpdate() {
	l := systemDaemonServiceLayout()
	if daemonServiceDefined(l) {
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
