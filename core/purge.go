package core

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/process"
)

// Удаление с очисткой (SPEC 135 §4.3): план и выполнение для диалога в
// Settings → Storage и для флага -purge-data. Строки stdout — английские без
// locale: при -purge-data локали ещё не загружены, а после ExecutePurge
// лог уже закрыт.

// errPurgeCoreRunning — очистка при запущенном ядре не начинается. Уходит
// в диалог, поэтому через locale (в отличие от строк stdout ниже).
func errPurgeCoreRunning() error { return errors.New(locale.T("Stop the VPN first")) }

// PurgePlan — что удалит очистка для текущей раскладки.
func (ac *AppController) PurgePlan() paths.PurgePlan {
	exe, err := paths.Executable()
	if err != nil {
		exe = "" // без exe не распознаётся только macOS-бандл: план беднее, не опаснее
	}
	return paths.BuildPurgePlan(ac.FileService.Layout, exe, os.Getenv, runtime.GOOS, paths.ProbeWritable)
}

// NetworkCleanupNeedsAdmin — сетевая очистка (адаптеры, NLA в HKLM, правила
// брандмауэра) недоступна: Windows без прав администратора (SPEC 139 §6
// п. 5). Диалог очистки снимает пункт с подсказкой.
func (ac *AppController) NetworkCleanupNeedsAdmin() bool {
	return windowsNotElevated()
}

// errNetworkCleanupNeedsAdmin — сетевая очистка пропущена: нет прав.
var errNetworkCleanupNeedsAdmin = errors.New("network cleanup needs administrator rights")

// networkCleanup — общая часть для диалога и -purge-data. Без прав на
// Windows — пропуск (errNetworkCleanupNeedsAdmin), ничего не трогается.
//
// Режим адаптеров — Aggressive: ядро к этому моменту не работает, а
// адаптеры после Stop/taskkill часто остаются без CM_PROB_PHANTOM, и
// PhantomOnly их пропустил бы. Защита от чужих адаптеров — фильтр по службе
// Wintun; активные (DN_STARTED) не трогаются в обоих режимах.
func networkCleanup() (adapters, rules int, err error) {
	if runtime.GOOS != "windows" {
		return 0, 0, nil
	}
	if windowsNotElevated() {
		return 0, 0, errNetworkCleanupNeedsAdmin
	}
	adapters, errA := platform.CleanupGhostSingboxTunAdapters(platform.GhostTunCleanupAggressive)
	rules, errR := platform.CleanupOrphanSingTunFirewallRules()
	var msgs []string
	if errA != nil {
		msgs = append(msgs, "adapters: "+errA.Error())
	}
	if errR != nil {
		msgs = append(msgs, "firewall rules: "+errR.Error())
	}
	if len(msgs) > 0 {
		err = errors.New(strings.Join(msgs, "; "))
	}
	return adapters, rules, err
}

// networkCleanupText — строка итога сетевой очистки для stdout. Пропуск без
// прав — с командой, которая доделает очистку из консоли администратора
// (SPEC 139 §6 п. 5).
func networkCleanupText(exe string, adapters, rules int, err error) string {
	if errors.Is(err, errNetworkCleanupNeedsAdmin) {
		return fmt.Sprintf("Network cleanup: skipped (needs administrator). To finish, run from an administrator command prompt: \"%s\" -purge-data -yes\n", exe)
	}
	s := fmt.Sprintf("Network cleanup: %d adapter(s), %d firewall rule(s) removed", adapters, rules)
	if err != nil {
		s += " (errors: " + err.Error() + ")"
	}
	return s + "\n"
}

// DaemonUninstallHint — команда удаления службы демона, если она
// установлена (macOS, plist в /Library/LaunchDaemons), иначе "". Очистка
// службу не трогает. survivesPurge — команда идёт через root-owned копию
// службы (SPEC 136), её можно выполнить и после удаления данных; иначе —
// через ядро лаунчера, и выполнять её нужно до удаления, пока ядро на месте.
func (ac *AppController) DaemonUninstallHint() (command string, survivesPurge bool) {
	return ac.daemonUninstallHint()
}

// ExecutePurgeAndExit выполняет план и завершает процесс без перезапуска:
// закрыть логи → удалить (data → leftover → logs) → значение автозапуска
// (autostart, SPEC 139 §8) → сетевая очистка (network, только Windows) →
// итог в stdout → os.Exit(0). Ошибка возвращается только до начала удаления
// (ядро запущено).
func (ac *AppController) ExecutePurgeAndExit(p paths.PurgePlan, network, autostart bool) error {
	if ac.RunningState != nil && ac.RunningState.IsRunning() {
		return errPurgeCoreRunning()
	}
	exe, exeErr := paths.Executable()
	ac.FileService.CloseLogFiles()
	// crash.log и native-stderr.log держит не FileService: без этого на
	// Windows LogDir не удалить до выхода процесса.
	debuglog.ReleaseLogFiles()
	rep := paths.ExecutePurge(p)
	out := rep.Text()
	if autostart && exeErr == nil {
		out += removeAutostartText(exe)
	}
	if network && runtime.GOOS == "windows" {
		adapters, rules, err := networkCleanup()
		out += networkCleanupText(exe, adapters, rules, err)
	}
	fmt.Print(out)
	os.Exit(0)
	return nil
}

// PurgeCLI — флаг -purge-data (SPEC 135 §4.3, решение Е). Без yes печатает
// план и выходит; с yes удаляет и печатает итог. Без прав на Windows сеть и
// остатки в защищённом AppDir пропускаются с подсказкой (SPEC 139 §6 п. 5–6):
// это не ошибка, код выхода от них не зависит. Вызывается из main до
// контроллера и GUI: логи ещё не открыты, закрывать нечего. Служба демона
// не трогается — печатается команда. Возвращает код выхода процесса: 1,
// если работает другой экземпляр лаунчера или ядро из каталога данных
// (purgeBlockingProcess), ядро живо по pid-файлу привилегированного запуска
// (процессы не трогаем), или часть удалить не вышло.
func PurgeCLI(l paths.Layout, exe string, yes bool, out io.Writer) int {
	plan := paths.BuildPurgePlan(l, exe, os.Getenv, runtime.GOOS, paths.ProbeWritable)
	hint, hintSurvives := daemonUninstallHintFor(platform.ResolveSingboxExecPath(l, os.Getenv).Path)
	// SPEC 139 §8: значение автозапуска удаляется, только если указывает на
	// этот exe.
	autostart := runtime.GOOS == "windows" && autostartOwned(exe)

	fmt.Fprint(out, plan.Text())
	if autostart {
		fmt.Fprintf(out, "[autostart] %s\n", platform.AutostartLocation)
	}
	switch {
	case hint != "" && hintSurvives:
		fmt.Fprintf(out, "The daemon service is installed and is not removed by this command; it runs from its own root-owned copy of the core, which the data removal does not touch. To remove the service as well:\n  %s\n", hint)
	case hint != "":
		fmt.Fprintf(out, "The daemon service is installed and is not removed by this command. Remove it first, while the core binary still exists:\n  %s\n", hint)
	}
	if !yes {
		fmt.Fprintln(out, "Dry run. Add -yes to remove.")
		return 0
	}

	if msg := purgeBlockingProcess(l, exe); msg != "" {
		fmt.Fprintf(out, "%s. Quit it first; nothing was removed.\n", msg)
		return 1
	}
	if pid, alive := purgeCoreAliveByPidFile(l.Data); alive {
		fmt.Fprintf(out, "sing-box is running (pid %d). Stop the VPN first; nothing was removed.\n", pid)
		return 1
	}

	rep := paths.ExecutePurge(plan)
	fmt.Fprint(out, rep.Text())
	if autostart {
		fmt.Fprint(out, removeAutostartText(exe))
	}
	if runtime.GOOS == "windows" {
		adapters, rules, err := networkCleanup()
		fmt.Fprint(out, networkCleanupText(exe, adapters, rules, err))
	}
	if len(rep.Failed) > 0 {
		return 1
	}
	return 0
}

// purgeBlockingProcess — что мешает очистке без окна: другой процесс
// лаунчера (то же имя исполняемого файла, как в
// CheckIfLauncherAlreadyRunningUtil) или sing-box, запущенный из каталога
// данных или программы. Чужой sing-box (путь известен и лежит в другом
// месте) не мешает; путь неизвестен — считаем своим. Процессы только
// читаются, ничего не убивается. "" — мешающих нет (или список процессов не
// получить: очистку это не блокирует, пишется предупреждение).
func purgeBlockingProcess(l paths.Layout, exe string) string {
	procs, err := process.GetProcesses()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot list processes: %v\n", err)
		return ""
	}
	self := os.Getpid()
	launcher := strings.ToLower(filepath.Base(exe))
	coreName := strings.ToLower(platform.GetExecutableNames())
	corePath := platform.ResolveSingboxExecPath(l, os.Getenv).Path

	var pathByPID map[int]string // лениво: ListProcesses дороже go-ps
	pathOf := func(pid int) string {
		if pathByPID == nil {
			pathByPID = map[int]string{}
			if list, err := platform.ListProcesses(); err == nil {
				for _, e := range list {
					pathByPID[e.PID] = e.Path
				}
			}
		}
		return pathByPID[pid]
	}
	ours := func(p string) bool {
		if p == "" {
			return true
		}
		for _, dir := range []string{l.Data.Bin(), l.App.Bin()} {
			if rel, err := filepath.Rel(dir, p); err == nil && !strings.HasPrefix(rel, "..") {
				return true
			}
		}
		return corePath != "" && strings.EqualFold(filepath.Clean(p), filepath.Clean(corePath))
	}

	for _, p := range procs {
		if p.PID == self {
			continue
		}
		name := strings.ToLower(p.Name)
		if launcher != "" && name == launcher {
			return fmt.Sprintf("The launcher is running (pid %d)", p.PID)
		}
		if name == coreName && ours(pathOf(p.PID)) {
			return fmt.Sprintf("sing-box is running (pid %d)", p.PID)
		}
	}
	return ""
}

// purgeCoreAliveByPidFile — жив ли процесс из pid-файла привилегированного
// запуска (<Data>/bin/singbox.pid: PID скрипта и PID ядра по строке; только
// macOS, на других ОС имя файла пустое). Проверка — по списку процессов,
// без сигналов: чужой процесс не трогаем. Переиспользованный PID отсекается
// по имени (sing-box, root-owned копия ядра — SPEC 137 — или шелл запуска).
func purgeCoreAliveByPidFile(d paths.DataDir) (int, bool) {
	if platform.PrivilegedPidFileName == "" {
		return 0, false
	}
	b, err := os.ReadFile(filepath.Join(d.Bin(), platform.PrivilegedPidFileName))
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(b), "\n") {
		pid, err := strconv.Atoi(strings.TrimSpace(line))
		if err != nil || pid <= 0 {
			continue
		}
		info, found, err := process.FindProcess(pid)
		if err != nil || !found {
			continue
		}
		name := strings.ToLower(info.Name)
		if strings.Contains(name, "sing-box") || platform.IsPrivilegedCoreProcessName(info.Name) || name == "sh" {
			return pid, true
		}
	}
	return 0, false
}
