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

// NetworkCleanup — призрачные wintun-адаптеры лаунчера и осиротевшие правила
// брандмауэра sing-tun. Только Windows; на других ОС результат нулевой.
func (ac *AppController) NetworkCleanup() (adapters, rules int, err error) {
	return networkCleanup()
}

// networkCleanup — общая часть для диалога и -purge-data.
//
// Режим адаптеров — Aggressive: ядро к этому моменту не работает, а
// адаптеры после Stop/taskkill часто остаются без CM_PROB_PHANTOM, и
// PhantomOnly их пропустил бы. Защита от чужих адаптеров — фильтр по службе
// Wintun; активные (DN_STARTED) не трогаются в обоих режимах.
func networkCleanup() (adapters, rules int, err error) {
	if runtime.GOOS != "windows" {
		return 0, 0, nil
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

// networkCleanupText — строка итога сетевой очистки для stdout.
func networkCleanupText(adapters, rules int, err error) string {
	s := fmt.Sprintf("Network cleanup: %d adapter(s), %d firewall rule(s) removed", adapters, rules)
	if err != nil {
		s += " (errors: " + err.Error() + ")"
	}
	return s + "\n"
}

// DaemonUninstallHint — команда удаления службы демона, если она
// установлена (macOS, plist в /Library/LaunchDaemons), иначе "". Очистка
// службу не трогает: команду выполняет пользователь до удаления данных,
// пока ядро ещё на месте.
func (ac *AppController) DaemonUninstallHint() string {
	return ac.daemonUninstallHint()
}

// ExecutePurgeAndExit выполняет план и завершает процесс без перезапуска:
// закрыть логи → удалить (data → leftover → logs) → сетевая очистка
// (network, только Windows) → итог в stdout → os.Exit(0). Ошибка
// возвращается только до начала удаления (ядро запущено).
func (ac *AppController) ExecutePurgeAndExit(p paths.PurgePlan, network bool) error {
	if ac.RunningState != nil && ac.RunningState.IsRunning() {
		return errPurgeCoreRunning()
	}
	ac.FileService.CloseLogFiles()
	rep := paths.ExecutePurge(p)
	out := rep.Text()
	if network && runtime.GOOS == "windows" {
		out += networkCleanupText(networkCleanup())
	}
	fmt.Print(out)
	os.Exit(0)
	return nil
}

// PurgeCLI — флаг -purge-data (SPEC 135 §4.3, решение Е). Без yes печатает
// план и выходит; с yes удаляет и печатает итог. Вызывается из main до
// контроллера и GUI: логи ещё не открыты, закрывать нечего. Служба демона
// не трогается — печатается команда. Возвращает код выхода процесса: 1,
// если ядро живо по pid-файлу привилегированного запуска (процесс не
// трогаем) или часть удалить не вышло.
func PurgeCLI(l paths.Layout, exe string, yes bool, out io.Writer) int {
	plan := paths.BuildPurgePlan(l, exe, os.Getenv, runtime.GOOS, paths.ProbeWritable)
	hint := daemonUninstallHintFor(platform.ResolveSingboxExecPath(l, os.Getenv).Path)

	fmt.Fprint(out, plan.Text())
	if hint != "" {
		fmt.Fprintf(out, "The daemon service is installed and is not removed by this command. Remove it first, while the core binary still exists:\n  %s\n", hint)
	}
	if !yes {
		fmt.Fprintln(out, "Dry run. Add -yes to remove.")
		return 0
	}

	if pid, alive := purgeCoreAliveByPidFile(l.Data); alive {
		fmt.Fprintf(out, "sing-box is running (pid %d). Stop the VPN first; nothing was removed.\n", pid)
		return 1
	}

	rep := paths.ExecutePurge(plan)
	fmt.Fprint(out, rep.Text())
	if runtime.GOOS == "windows" {
		fmt.Fprint(out, networkCleanupText(networkCleanup()))
	}
	if len(rep.Failed) > 0 {
		return 1
	}
	return 0
}

// purgeCoreAliveByPidFile — жив ли процесс из pid-файла привилегированного
// запуска (<Data>/bin/singbox.pid: PID скрипта и PID ядра по строке; только
// macOS, на других ОС имя файла пустое). Проверка — по списку процессов,
// без сигналов: чужой процесс не трогаем. Переиспользованный PID отсекается
// по имени (sing-box или sh-скрипт запуска).
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
		if strings.Contains(name, "sing-box") || name == "sh" {
			return pid, true
		}
	}
	return 0, false
}
