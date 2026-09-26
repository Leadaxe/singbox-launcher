//go:build windows && !386

package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"singbox-launcher/internal/debuglog"
)

// Classic-ядро под правами администратора на Windows (SPEC 141 §8):
// повышенный лаунчер исполняет только защищённую копию
// <ProgramFiles>\sing-box-lxd\sing-box-lxd.exe, вывод ядра — в
// <ProgramData>\sing-box-lxd\logs\classic.log с явным DACL. Скриптов,
// pid-файлов и AEWP (macOS) здесь нет — старт обычным exec из повышенного
// процесса, Stop/Kill — по PID.

// errPrivilegedNotSupported — AEWP-путь macOS на Windows не существует.
var errPrivilegedNotSupported = errors.New("privileged execution not supported on this platform")

// ErrPrivilegedLogDirMissing — каталога logs\ службы нет: его создают
// `--service=install` и `--service=copy` (SPEC 141 §3 п. 8).
var ErrPrivilegedLogDirMissing = errors.New("the protected logs folder is missing")

const (
	PrivilegedStartName        = ""
	PrivilegedLegacyScriptName = ""
	PrivilegedPidFileName      = ""
	// PrivilegedCopyName — главный файл набора копии; так же зовётся процесс
	// службы и повышенного classic.
	PrivilegedCopyName     = "sing-box-lxd.exe"
	PrivilegedPkillPattern = ""
	// PrivilegedCronetName — библиотека naive, член набора, если лежит рядом
	// с ядром лаунчера (wintun.dll ядро несёт go:embed).
	PrivilegedCronetName = "libcronet.dll"
	// PrivilegedSidecarName — сайдкар установки в каталоге копии.
	PrivilegedSidecarName = "sing-box-lxd.install.json"
	// privilegedLogRotateBytes — ротация classic.log в classic.log.old.
	privilegedLogRotateBytes = 2 << 20
)

// IsPrivilegedCoreProcessName — имя процесса защищённой копии.
func IsPrivilegedCoreProcessName(name string) bool {
	return strings.EqualFold(name, PrivilegedCopyName)
}

// RunWithPrivileges — только macOS.
func RunWithPrivileges(toolPath string, args []string) (scriptPID, singboxPID int, err error) {
	_, _ = toolPath, args
	return 0, 0, errPrivilegedNotSupported
}

// StartPrivilegedCore — только macOS: на Windows повышенный лаунчер сам
// запускает копию.
func StartPrivilegedCore(corePath, binDir, configName string) (shellPID, corePID int, err error) {
	_, _, _ = corePath, binDir, configName
	return 0, 0, errPrivilegedNotSupported
}

// PrivilegedCoreLogPath — classic.log повышенного classic; "" без прав:
// обычный лаунчер пишет лог ядра в свой каталог.
func PrivilegedCoreLogPath() string {
	if !IsElevated() {
		return ""
	}
	return privilegedCoreLogFile()
}

func privilegedCoreLogFile() string {
	return filepath.Join(PrivilegedDataDir(), "logs", "classic.log")
}

// OpenPrivilegedCoreLog открывает classic.log для вывода повышенного ядра
// (SPEC 141 §8): каталог logs\ и цепочка <ProgramData>\sing-box-lxd — по
// инварианту §6.1 (нет каталога — ErrPrivilegedLogDirMissing); > 2 МиБ —
// rename в classic.log.old (не вышло — старт не валится, ротация ждёт
// следующего старта); файл открывается без следования ссылкам
// (reparse point или каталог на его месте — отказ) на дозапись, и на
// каждом старте ему заново ставится явный DACL: SYSTEM и Administrators —
// полный доступ, пользователь лаунчера — чтение (install/copy снимают его).
func OpenPrivilegedCoreLog() (*os.File, error) {
	root := PrivilegedDataDir()
	dir := filepath.Join(root, "logs")
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrPrivilegedLogDirMissing, dir)
	}
	if err := CheckProtectedCopy(root, nil, nil); err != nil {
		return nil, err
	}
	if err := checkProtectedPath(dir, true); err != nil {
		return nil, err
	}
	path := privilegedCoreLogFile()
	if fi, err := os.Lstat(path); err == nil && fi.Mode().IsRegular() && fi.Size() > privilegedLogRotateBytes {
		old := path + ".old"
		_ = os.Remove(old)
		if err := os.Rename(path, old); err != nil {
			// Файл держит чужой дескриптор без FILE_SHARE_DELETE (открытый
			// в редакторе лог, антивирус): ротация откладывается до
			// следующего старта, ядро пишет дальше в тот же файл.
			debuglog.WarnLog("OpenPrivilegedCoreLog: rotate %s skipped: %v", path, err)
		}
	}
	sddl, err := privilegedLogSDDL()
	if err != nil {
		return nil, err
	}
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return nil, err
	}
	sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(name,
		windows.FILE_APPEND_DATA|windows.FILE_READ_ATTRIBUTES|windows.READ_CONTROL|windows.WRITE_DAC|windows.WRITE_OWNER|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, sa, windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	var fi windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &fi); err != nil {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if fi.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("%s is not a regular file (reparse point or directory); remove it", path)
	}
	owner, _, err := sd.Owner()
	if err == nil {
		var dacl *windows.ACL
		if dacl, _, err = sd.DACL(); err == nil {
			err = windows.SetSecurityInfo(h, windows.SE_FILE_OBJECT,
				windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
				owner, nil, dacl, nil)
		}
	}
	if err != nil {
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("set the access list of %s: %w", path, err)
	}
	return os.NewFile(uintptr(h), path), nil
}

// privilegedLogSDDL — владелец Administrators; SYSTEM и Administrators —
// полный доступ, пользователь токена лаунчера — чтение.
func privilegedLogSDDL() (string, error) {
	tok, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return "", fmt.Errorf("open process token: %w", err)
	}
	defer func() { _ = tok.Close() }()
	user, err := tok.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("token user: %w", err)
	}
	return "O:BAD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FR;;;" + user.User.Sid.String() + ")", nil
}

// KillPrivilegedProcess — no-op: повышенный classic снимается по PID
// (KillProcessByPID), не AEWP.
func KillPrivilegedProcess(scriptPID, singboxPID int, pidFile string) error {
	_, _, _ = scriptPID, singboxPID, pidFile
	return nil
}

// KillPrivilegedByPattern — только macOS; `taskkill /IM sing-box-lxd.exe`
// запрещён — так же зовётся служба (SPEC 141 §8).
func KillPrivilegedByPattern() error {
	return errPrivilegedNotSupported
}

// WaitForPrivilegedExit — no-op вне macOS.
func WaitForPrivilegedExit(pid int) {
	_ = pid
}

// FreePrivilegedAuthorization — no-op вне macOS.
func FreePrivilegedAuthorization() {}
