//go:build darwin

package core

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// Классификатор состояния launchd-службы демона (SPEC 136).
//
// Ядро lx.11+ на `lxd --service=install` копирует себя в root-owned каталог
// службы и переписывает plist на копию (SPEC 100 форка). Лаунчер ничего не
// копирует: он только читает plist, проверяет цепочку владения копии и
// сверяет sha256 копии с ядром лаунчера и с тем, что отвечает работающий
// демон. Всё читается без root: plist и сайдкар 0644, копия 0755.
//
// Раскладка (решение владельца 24.09.2026, ядро lx.12): копия — плоский
// файл /Library/PrivilegedHelperTools/sing-box-lxd, сайдкар —
// <копия>.install.json; каталога службы нет. Цепочка владения: /Library →
// PrivilegedHelperTools → файл. Ранние сборки lx.11 клали копию в
// /Library/PrivilegedHelperTools/<label>/sing-box или в плоский
// /Library/PrivilegedHelperTools/<label> — это legacy: убрать `sudo rm -rf`.
//
// Здесь — платформенная часть классификатора (общая — daemon_service_state.go):
// раскладка /Library, чтение plist, цепочка владения по uid (Stat_t), ключ
// кэша по (dev, inode), состояние у launchd и legacy-раскладка lx.11.

const (
	// daemonServiceChainRoot — верх цепочки владения копии: от него вниз до
	// файла каждое звено обязано быть root-owned без g/o-записи.
	daemonServiceChainRoot = "/Library"
	// daemonServiceHelperToolsDir — каталог привилегированных помощников
	// macOS (root:wheel 1755); копия лежит в нём плоским файлом
	// platform.PrivilegedCopyName (`sing-box-lxd` — так же зовётся процесс).
	daemonServiceHelperToolsDir = "/Library/PrivilegedHelperTools"
	// daemonServiceLegacyCopyPath — раскладка ранних сборок lx.11: каталог
	// <label>/ с sing-box внутри или плоский файл <label>. Не используется;
	// лаунчер советует её удалить.
	daemonServiceLegacyCopyPath = daemonServiceHelperToolsDir + "/" + daemonLaunchdLabel
	// daemonServiceSidecarSuffix — сайдкар установки рядом с копией,
	// <копия>.install.json (root:wheel 0644):
	// {source, sha256, version, installed_at, plist_path, label}.
	daemonServiceSidecarSuffix = ".install.json"
	// launchctlTool / launchctlTimeout — чтение состояния службы у launchd
	// (`launchctl print system/<label>`, без sudo); exit 113 — службы в
	// домене нет (не загружена).
	launchctlTool           = "/bin/launchctl"
	launchctlTimeout        = 2 * time.Second
	launchctlNotFoundStatus = 113
	// launchdRunningState — `state = running` в выводе launchctl print.
	launchdRunningState = "running"
	// launchdNotLoaded — LaunchdState, когда launchd службу не знает.
	launchdNotLoaded = "not loaded"
)

// daemonServiceCorePath — каноническая root-owned копия ядра службы.
func daemonServiceCorePath() string {
	return filepath.Join(daemonServiceHelperToolsDir, platform.PrivilegedCopyName)
}

// isLegacyCopyPath — path — копия ранней раскладки lx.11 (legacy — сам
// файл или что-то внутри каталога <label>/).
func isLegacyCopyPath(path, legacy string) bool {
	if legacy == "" {
		return false
	}
	path = filepath.Clean(path)
	legacy = filepath.Clean(legacy)
	return path == legacy || strings.HasPrefix(path, legacy+string(filepath.Separator))
}

// legacyCopyRemoveCommand — команда удаления остатков ранней раскладки:
// сам путь (файл или каталог) и сайдкар плоского варианта.
func legacyCopyRemoveCommand(legacy string) string {
	return "sudo rm -rf " + shellQuote(legacy) + " " + shellQuote(daemonServiceSidecarPath(legacy))
}

// daemonServiceSidecarPath — сайдкар установки копии corePath.
func daemonServiceSidecarPath(corePath string) string {
	return corePath + daemonServiceSidecarSuffix
}

func systemDaemonServiceLayout() daemonServiceLayout {
	return daemonServiceLayout{
		PlistPath:  daemonSystemPlistPath(),
		CorePath:   daemonServiceCorePath(),
		LegacyPath: daemonServiceLegacyCopyPath,
		ChainRoot:  daemonServiceChainRoot,
		OwnerUID:   0,
	}
}

// inspectDaemonServiceDefinition — первый шаг классификатора без хэшей:
// NotInstalled / Unsafe / Stale (копии нет) / OK (определение службы
// безопасно). Дёшев: чтение plist и Lstat цепочки.
func inspectDaemonServiceDefinition(l daemonServiceLayout) DaemonServiceCheck {
	if _, err := os.Lstat(l.PlistPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DaemonServiceCheck{State: DaemonServiceNotInstalled}
		}
		return DaemonServiceCheck{State: DaemonServiceUnsafe,
			Detail: fmt.Sprintf("cannot stat the service plist %s: %v", l.PlistPath, err)}
	}
	servicePath, err := readPlistProgramPath(l.PlistPath)
	if err != nil {
		return DaemonServiceCheck{State: DaemonServiceUnsafe,
			Detail: fmt.Sprintf("cannot read ProgramArguments[0] from %s: %v", l.PlistPath, err)}
	}
	c := DaemonServiceCheck{State: DaemonServiceOK, ServicePath: servicePath}
	if filepath.Clean(servicePath) != filepath.Clean(l.CorePath) {
		c.State = DaemonServiceUnsafe
		c.Detail = fmt.Sprintf("the service runs %s, not the root-owned copy %s", servicePath, l.CorePath)
		if isLegacyCopyPath(servicePath, l.LegacyPath) {
			c.Detail = fmt.Sprintf("the service runs %s, a copy in the legacy layout of early lx.11 builds: run Install or update service, then remove it (%s)",
				servicePath, legacyCopyRemoveCommand(l.LegacyPath))
		}
		return c
	}
	if err := checkRootOwnedChain(l.CorePath, l.ChainRoot, l.OwnerUID); err != nil {
		if errors.Is(err, errDaemonCopyMissing) {
			c.State = DaemonServiceStale
			c.CopyMissing = true
		} else {
			c.State = DaemonServiceUnsafe
		}
		c.Detail = err.Error()
	}
	return c
}

func checkRootOwnedEntry(path string, ownerUID uint32, wantDir bool) error {
	fi, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", errDaemonCopyMissing, path)
		}
		return fmt.Errorf("%s: %v", path, err)
	}
	mode := fi.Mode()
	switch {
	case mode&os.ModeSymlink != 0:
		return fmt.Errorf("%s is a symlink", path)
	case wantDir && !fi.IsDir():
		return fmt.Errorf("%s is not a directory", path)
	case !wantDir && fi.IsDir():
		return fmt.Errorf("%s is a directory, not the copy file: remove it (sudo rm -rf %s) and run the command again",
			path, shellQuote(path))
	case !wantDir && !mode.IsRegular():
		return fmt.Errorf("%s is not a regular file", path)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%s: no owner information", path)
	}
	if st.Uid != ownerUID {
		return fmt.Errorf("%s is owned by uid %d, want %d", path, st.Uid, ownerUID)
	}
	if mode.Perm()&0o022 != 0 {
		return fmt.Errorf("%s is writable by group or others (%04o)", path, mode.Perm())
	}
	return nil
}

// compareDaemonServiceRunning — шаг классификатора после файлов: что о
// службе говорит менеджер служб ОС (на macOS — launchd).
func compareDaemonServiceRunning(c *DaemonServiceCheck) {
	compareDaemonServiceLaunchd(c, queryLaunchdJob(daemonLaunchdLabel))
}

// launchdJob — что launchd знает о службе. Known=false — спросить не
// удалось (нет launchctl, таймаут, непонятный вывод): вердикт не
// выносится. Loaded=false — службы в домене system нет.
type launchdJob struct {
	Known  bool
	Loaded bool
	State  string
}

// queryLaunchdJob — `launchctl print system/<label>` без sudo, с таймаутом;
// кэша нет: состояние меняется от любой команды пользователя.
func queryLaunchdJob(label string) launchdJob {
	ctx, cancel := context.WithTimeout(context.Background(), launchctlTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, launchctlTool, "print", "system/"+label).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == launchctlNotFoundStatus {
			return launchdJob{Known: true}
		}
		debuglog.DebugLog("daemon service: launchctl print system/%s: %v", label, err)
		return launchdJob{}
	}
	state := parseLaunchctlPrintState(out)
	if state == "" {
		debuglog.DebugLog("daemon service: launchctl print system/%s: no state line", label)
		return launchdJob{}
	}
	return launchdJob{Known: true, Loaded: true, State: state}
}

// parseLaunchctlPrintState — `state = …` самой службы: строка верхнего
// уровня блока (один таб), а не вложенных (endpoints и т.п.).
func parseLaunchctlPrintState(out []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "\tstate = ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "\tstate = "))
		}
	}
	return ""
}

// compareDaemonServiceLaunchd — шаг после файлов: служба на месте, но
// launchd её не держит — NotRunning. Работает только поверх OK; launchd не
// ответил — вердикт не выносится.
func compareDaemonServiceLaunchd(c *DaemonServiceCheck, job launchdJob) {
	if !job.Known {
		return
	}
	c.LaunchdState = job.State
	if !job.Loaded {
		c.LaunchdState = launchdNotLoaded
	}
	if c.State != DaemonServiceOK {
		return
	}
	switch {
	case !job.Loaded:
		c.State = DaemonServiceNotRunning
		c.Detail = "the service is not loaded in launchd"
	case job.State != launchdRunningState:
		c.State = DaemonServiceNotRunning
		c.Detail = fmt.Sprintf("launchd reports the service state %q", job.State)
	}
}

func statHashKey(path string) (fileHashKey, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return fileHashKey{}, err
	}
	if !fi.Mode().IsRegular() {
		return fileHashKey{}, fmt.Errorf("%s is not a regular file", path)
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fileHashKey{}, fmt.Errorf("%s: no inode information", path)
	}
	return fileHashKey{dev: uint64(st.Dev), ino: st.Ino, size: fi.Size(), mtime: fi.ModTime().UnixNano()}, nil
}
