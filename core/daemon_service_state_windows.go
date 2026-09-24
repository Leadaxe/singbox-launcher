//go:build windows && !386

package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// Платформенная часть классификатора службы на Windows (SPEC 141 §6):
// раскладка <ProgramFiles>\sing-box-lxd (windows.KnownFolderPath, как у
// ядра), определение службы у SCM (BinaryPathName, DACL службы), инвариант
// копии — владелец и DACL звеньев по списку разрешённых SID, набор файлов
// (sing-box-lxd.exe + libcronet.dll) и сайдкар sing-box-lxd.install.json,
// ключ файла — GetFileInformationByHandle, состояние — QueryServiceStatus.
// Всё без прав: DACL службы даёт Authenticated Users чтение конфигурации и
// статуса (SPEC 141 §3 п. 1a).

// minCoreForRootOwnedService — первое ядро со службой Windows (SPEC 103
// форка, линия v1.14.2): `--service=install|copy`, --invite-out, набор с
// libcronet.dll. Пре-релиз порогового релиза (rc.1) гейт проходит: разбор
// сравнивает базу и lx.N, пре-релиз ядра лаунчера не понижает.
const minCoreForRootOwnedService = "1.14.2-lx.2-rc.1"

// daemonServiceCorePath — главный файл набора защищённой копии.
func daemonServiceCorePath() string {
	return filepath.Join(platform.PrivilegedCopyDir(), platform.PrivilegedCopyName)
}

// daemonServiceSidecarPath — сайдкар установки в каталоге копии.
func daemonServiceSidecarPath(corePath string) string {
	return filepath.Join(filepath.Dir(corePath), platform.PrivilegedSidecarName)
}

func systemDaemonServiceLayout() daemonServiceLayout {
	return daemonServiceLayout{CorePath: daemonServiceCorePath()}
}

// inspectDaemonServiceDefinition — первый шаг классификатора без хэшей
// (SPEC 141 §6.2): NotInstalled / Unsafe / Stale (копии нет) / OK. SCM не
// ответил — вердикт NotInstalled с причиной (судить не о чем).
func inspectDaemonServiceDefinition(l daemonServiceLayout) DaemonServiceCheck {
	info, err := platform.QueryService(daemonServiceName)
	if err != nil {
		debuglog.DebugLog("daemon service: %v", err)
		return DaemonServiceCheck{State: DaemonServiceNotInstalled, Detail: err.Error()}
	}
	if !info.Exists {
		return DaemonServiceCheck{State: DaemonServiceNotInstalled}
	}
	if info.ConfigErr != nil {
		return DaemonServiceCheck{State: DaemonServiceUnsafe,
			Detail: fmt.Sprintf("cannot read the configuration of service %s: %v", daemonServiceName, info.ConfigErr)}
	}
	servicePath, unquoted, err := platform.ServiceExecutablePath(info.BinaryPath)
	if err != nil {
		return DaemonServiceCheck{State: DaemonServiceUnsafe,
			Detail: fmt.Sprintf("cannot parse the service command line %q: %v", info.BinaryPath, err)}
	}
	c := DaemonServiceCheck{State: DaemonServiceOK, ServicePath: servicePath}
	if info.StatusErr == nil {
		c.LaunchdState = platform.ServiceStateName(info.State)
	}
	switch {
	case unquoted:
		c.State = DaemonServiceUnsafe
		c.Detail = fmt.Sprintf("the service command line is not quoted and its path contains a space (%s)", info.BinaryPath)
		return c
	case !platform.SamePathFold(servicePath, l.CorePath):
		c.State = DaemonServiceUnsafe
		c.Detail = fmt.Sprintf("the service runs %s, not the protected copy %s", servicePath, l.CorePath)
		return c
	case info.DACLErr != nil:
		c.State = DaemonServiceUnsafe
		c.Detail = fmt.Sprintf("service %s: %v", daemonServiceName, info.DACLErr)
		return c
	}
	if err := checkDaemonCopyChain(l); err != nil {
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

// checkDaemonCopyChain — инвариант SPEC 141 §6.1 для копии раскладки l:
// предки от корня тома, каталог копии, главный файл (обязателен) и
// libcronet.dll (если лежит). Нет каталога или файла — errDaemonCopyMissing.
func checkDaemonCopyChain(l daemonServiceLayout) error {
	err := platform.CheckProtectedCopy(filepath.Dir(l.CorePath),
		[]string{filepath.Base(l.CorePath)}, []string{platform.PrivilegedCronetName})
	if errors.Is(err, platform.ErrProtectedPathMissing) {
		return fmt.Errorf("%w: %v", errDaemonCopyMissing, err)
	}
	return err
}

// checkRootOwnedEntry — звено цепочки по uid (macOS); на Windows цепочку
// проверяет checkDaemonCopyChain по DACL.
func checkRootOwnedEntry(path string, _ uint32, _ bool) error {
	return fmt.Errorf("%s: unix ownership is not available on Windows", path)
}

// daemonServiceDefined — служба есть у SCM.
func daemonServiceDefined(_ daemonServiceLayout) bool {
	info, err := platform.QueryService(daemonServiceName)
	return err == nil && info.Exists
}

// sameServicePath — пути Windows без учёта регистра после Clean (§6.2).
func sameServicePath(a, b string) bool { return platform.SamePathFold(a, b) }

// daemonSetMismatch — члены набора кроме главного файла и лишние файлы в
// каталоге копии (SPEC 141 §6.2): libcronet.dll — то же присутствие и sha,
// что у ядра лаунчера (launcherCore — после EvalSymlinks); лишнее — любое
// имя, кроме членов набора, сайдкара и остатков замены образа
// (<член>.old, .<член>.tmp-<hex>). err — хэш не посчитался.
func daemonSetMismatch(corePath, launcherCore string, hashes *fileHashCache) (name string, extra bool, detail string, err error) {
	copyDir := filepath.Dir(corePath)
	lib := platform.PrivilegedCronetName
	copyLib := filepath.Join(copyDir, lib)
	srcLib := filepath.Join(filepath.Dir(launcherCore), lib)
	_, copyErr := os.Lstat(copyLib)
	_, srcErr := os.Stat(srcLib)
	copyHas, srcHas := copyErr == nil, srcErr == nil
	switch {
	case srcHas && !copyHas:
		return lib, false, fmt.Sprintf("the protected copy has no %s, the launcher core %s has one", lib, launcherCore), nil
	case copyHas && !srcHas:
		return lib, false, fmt.Sprintf("the protected copy has %s, the launcher core %s has none", lib, launcherCore), nil
	case copyHas:
		copySum, err := hashes.sum(copyLib)
		if err != nil {
			return "", false, "", fmt.Errorf("cannot read %s: %w", copyLib, err)
		}
		srcSum, err := hashes.sum(srcLib)
		if err != nil {
			return "", false, "", fmt.Errorf("cannot read %s: %w", srcLib, err)
		}
		if copySum != srcSum {
			return lib, false, fmt.Sprintf("%s in the protected copy (sha256 %s) is not the launcher's %s (sha256 %s)",
				lib, shortSHA(copySum), srcLib, shortSHA(srcSum)), nil
		}
	}
	entries, err := os.ReadDir(copyDir)
	if err != nil {
		return "", false, "", fmt.Errorf("list %s: %w", copyDir, err)
	}
	known := []string{strings.ToLower(filepath.Base(corePath)), strings.ToLower(lib), strings.ToLower(platform.PrivilegedSidecarName)}
	for _, e := range entries {
		if !daemonCopyKnownEntry(e.Name(), known) {
			return e.Name(), true, fmt.Sprintf("unknown file %s in the protected copy folder",
				filepath.Join(copyDir, e.Name())), nil
		}
	}
	return "", false, "", nil
}

// daemonCopyKnownEntry — член набора, сайдкар или остаток замены образа
// (SPEC 141 §3 п. 3a) — не лишний файл.
func daemonCopyKnownEntry(name string, known []string) bool {
	lower := strings.ToLower(name)
	for _, k := range known {
		if lower == k || lower == k+".old" || strings.HasPrefix(lower, "."+k+".tmp-") {
			return true
		}
	}
	return false
}

// fileHashKey — (VolumeSerialNumber, FileIndex, size, mtime) из
// GetFileInformationByHandle (SPEC 141 §6.2).
type fileHashKey platform.FileKey

func statHashKey(path string) (fileHashKey, error) {
	k, err := platform.StatFileKey(path)
	return fileHashKey(k), err
}

// compareDaemonServiceRunning — шаг после файлов: SCM без прав
// (QueryServiceStatus). CurrentState ≠ SERVICE_RUNNING — NotRunning (как
// exit 5 у ядра); START_PENDING до следующего Refresh — тоже. SCM не
// ответил — вердикт не выносится.
func compareDaemonServiceRunning(c *DaemonServiceCheck) {
	info, err := platform.QueryService(daemonServiceName)
	if err != nil || !info.Exists || info.StatusErr != nil {
		return
	}
	c.LaunchdState = platform.ServiceStateName(info.State)
	if c.State != DaemonServiceOK || platform.ServiceRunning(info.State) {
		return
	}
	c.State = DaemonServiceNotRunning
	c.Detail = fmt.Sprintf("the service is installed but not running (SCM state %s)", c.LaunchdState)
}

// readDaemonServiceSidecarWarnings — warnings последнего install/copy из
// сайдкара (SPEC 141 §3 п. 3, §5.3); нет поля или сайдкара — нет.
func readDaemonServiceSidecarWarnings(corePath string) []DaemonServiceWarning {
	data, err := os.ReadFile(daemonServiceSidecarPath(corePath))
	if err != nil {
		return nil
	}
	var sidecar struct {
		Warnings []DaemonServiceWarning `json:"warnings"`
	}
	if err := json.Unmarshal(data, &sidecar); err != nil {
		debuglog.DebugLog("daemon service: sidecar: %v", err)
		return nil
	}
	return sidecar.Warnings
}

// legacyCopyRemoveCommand — ранней раскладки lx.11 на Windows не было:
// LegacyPath пуст, и сюда не доходят.
func legacyCopyRemoveCommand(_ string) string { return "" }
