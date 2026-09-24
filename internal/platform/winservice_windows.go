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
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// Служба sing-box-lxd и её защищённая копия на Windows (SPEC 141 §3, §6):
// всё читается без прав — раскладка по KnownFolder, конфигурация и
// состояние службы у SCM, DACL службы, владелец и DACL звеньев копии по
// списку разрешённых SID (то же правило, что у ядра: SPEC 103 §2.2 форка,
// lxd/aclcheck.go), ключ файла для кэшей sha и версии.

// privilegedDirName — каталог копии в <ProgramFiles> и данных в <ProgramData>.
const privilegedDirName = "sing-box-lxd"

// knownFolder — известная папка; запасной путь — переменная окружения,
// затем стандартный путь (как у ядра).
func knownFolder(folder *windows.KNOWNFOLDERID, variable, fallback string) string {
	if path, err := windows.KnownFolderPath(folder, 0); err == nil && path != "" {
		return path
	}
	if path := os.Getenv(variable); path != "" {
		return path
	}
	return fallback
}

// PrivilegedCopyDir — <ProgramFiles>\sing-box-lxd: копия ядра службы и
// повышенного classic (SPEC 141 §3 п. 2).
func PrivilegedCopyDir() string {
	return filepath.Join(knownFolder(windows.FOLDERID_ProgramFiles, "ProgramFiles", `C:\Program Files`), privilegedDirName)
}

// PrivilegedDataDir — <ProgramData>\sing-box-lxd: state\ и logs\ службы.
func PrivilegedDataDir() string {
	return filepath.Join(knownFolder(windows.FOLDERID_ProgramData, "ProgramData", `C:\ProgramData`), privilegedDirName)
}

// --- SCM ---------------------------------------------------------------

// ServiceInfo — что служба сообщает без прав (SPEC 141 §6.2).
type ServiceInfo struct {
	// Exists — служба есть у SCM (OpenService не ответил 1060).
	Exists bool
	// ConfigErr — конфигурация не прочиталась (в т. ч. отказ в доступе).
	ConfigErr  error
	BinaryPath string
	State      uint32
	StatusErr  error
	// DACLErr — DACL службы не прочитался или даёт чужому SID лишнее.
	DACLErr error
}

// QueryService читает службу name с правами обычного пользователя:
// mgr.Connect просит SC_MANAGER_ALL_ACCESS и без прав падает, поэтому
// дескрипторы открываются здесь (как `--service=status` ядра). Ошибка —
// не открылся сам SCM.
func QueryService(name string) (ServiceInfo, error) {
	var info ServiceInfo
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return info, fmt.Errorf("open the service manager: %w", err)
	}
	defer func() { _ = windows.CloseServiceHandle(manager) }()
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return info, err
	}
	withDACL := true
	handle, err := windows.OpenService(manager, namePtr, windows.SERVICE_QUERY_CONFIG|windows.SERVICE_QUERY_STATUS|windows.READ_CONTROL)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		withDACL = false
		handle, err = windows.OpenService(manager, namePtr, windows.SERVICE_QUERY_CONFIG|windows.SERVICE_QUERY_STATUS)
	}
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return info, nil
	}
	info.Exists = true
	if err != nil {
		info.ConfigErr = err
		return info, nil
	}
	defer func() { _ = windows.CloseServiceHandle(handle) }()
	service := &mgr.Service{Name: name, Handle: handle}
	// Только QueryServiceConfig: из конфигурации нужен BinaryPathName, а
	// mgr.Config дочитывает ещё три QueryServiceConfig2.
	if binaryPath, err := serviceBinaryPath(handle); err != nil {
		info.ConfigErr = err
	} else {
		info.BinaryPath = binaryPath
	}
	if status, err := service.Query(); err != nil {
		info.StatusErr = err
	} else {
		info.State = uint32(status.State)
	}
	if !withDACL {
		info.DACLErr = errors.New("READ_CONTROL on the service is denied")
		return info, nil
	}
	sd, err := windows.GetSecurityInfo(handle, windows.SE_SERVICE, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		info.DACLErr = err
		return info, nil
	}
	facts, err := descriptorFacts(sd, securityFacts{})
	if err != nil {
		info.DACLErr = err
		return info, nil
	}
	info.DACLErr = serviceACLViolation(facts)
	return info, nil
}

// serviceBinaryPath — BinaryPathName из QueryServiceConfig.
func serviceBinaryPath(handle windows.Handle) (string, error) {
	n := uint32(1024)
	for {
		buf := make([]byte, n)
		cfg := (*windows.QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0]))
		err := windows.QueryServiceConfig(handle, cfg, n, &n)
		if err == nil {
			return windows.UTF16PtrToString(cfg.BinaryPathName), nil
		}
		if !errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER) || n <= uint32(len(buf)) {
			return "", err
		}
	}
}

// ServiceRunning — состояние SERVICE_RUNNING.
func ServiceRunning(state uint32) bool { return svc.State(state) == svc.Running }

// ServiceStateName — CurrentState SCM для показа и Debug API.
func ServiceStateName(state uint32) string {
	switch svc.State(state) {
	case svc.Stopped:
		return "stopped"
	case svc.StartPending:
		return "start_pending"
	case svc.StopPending:
		return "stop_pending"
	case svc.Running:
		return "running"
	case svc.ContinuePending:
		return "continue_pending"
	case svc.PausePending:
		return "pause_pending"
	case svc.Paused:
		return "paused"
	}
	return fmt.Sprintf("state_%d", state)
}

// ServiceExecutablePath — argv[0] из BinaryPathName (SPEC 141 §6.2):
// разбор DecomposeCommandLine, префикс `\??\` снимается, переменные
// окружения (%SystemRoot%) раскрываются. unquotedSpace — путь с пробелом
// без кавычек (SCM сначала попробует C:\Program.exe).
func ServiceExecutablePath(binaryPath string) (path string, unquotedSpace bool, err error) {
	args, err := windows.DecomposeCommandLine(binaryPath)
	if err != nil {
		return "", false, err
	}
	if len(args) == 0 || args[0] == "" {
		return "", false, errors.New("empty command line")
	}
	trimmed := strings.TrimSpace(binaryPath)
	if !strings.HasPrefix(trimmed, `"`) {
		exe := trimmed
		if i := strings.Index(strings.ToLower(trimmed), ".exe"); i >= 0 {
			exe = trimmed[:i]
		}
		unquotedSpace = strings.ContainsAny(exe, " \t")
	}
	path = strings.TrimPrefix(args[0], `\??\`)
	if strings.Contains(path, "%") {
		path = expandEnv(path)
	}
	return path, unquotedSpace, nil
}

func expandEnv(s string) string {
	src, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return s
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n, err := windows.ExpandEnvironmentStrings(src, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || int(n) > len(buf) {
		return s
	}
	return windows.UTF16ToString(buf[:n])
}

// SamePathFold — пути Windows равны так, как их сравнивает файловая
// система: после Clean, без учёта регистра.
func SamePathFold(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// --- Инвариант защищённого пути (SPEC 141 §6.1 = SPEC 103 §2.2 форка) ---

// Разрешённые SID и права — как lxd/aclcheck.go ядра.
const (
	sidSystem           = "S-1-5-18"
	sidAdministrators   = "S-1-5-32-544"
	sidTrustedInstaller = "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"

	aceAccessAllowed              = 0x0
	aceAccessDenied               = 0x1
	aceAccessDeniedObject         = 0x6
	aceAccessAllowedCallback      = 0x9
	aceAccessDeniedCallback       = 0xA
	aceAccessDeniedCallbackObject = 0xC
	aceInheritOnly                = 0x08

	accessFileWriteData       = 0x00000002
	accessFileAppendData      = 0x00000004
	accessFileWriteEA         = 0x00000010
	accessFileDeleteChild     = 0x00000040
	accessFileWriteAttributes = 0x00000100
	accessDelete              = 0x00010000
	accessWriteDAC            = 0x00040000
	accessWriteOwner          = 0x00080000
	accessGenericAll          = 0x10000000
	accessGenericWrite        = 0x40000000
	accessServiceChangeConfig = 0x0002

	ancestorForbidden  = accessDelete | accessWriteDAC | accessWriteOwner | accessGenericWrite | accessGenericAll | accessFileDeleteChild
	protectedForbidden = ancestorForbidden | accessFileWriteData | accessFileAppendData | accessFileWriteEA | accessFileWriteAttributes
	serviceForbidden   = accessServiceChangeConfig | accessWriteDAC | accessWriteOwner | accessDelete | accessGenericWrite | accessGenericAll
)

// ErrProtectedPathMissing — звена копии нет при целых предках: не дыра, а
// отсутствующая копия (Stale / missing).
var ErrProtectedPathMissing = errors.New("the protected copy is missing")

type aclEntry struct {
	Type  uint8
	Flags uint8
	Mask  uint32
	SID   *windows.SID
}

type securityFacts struct {
	Owner        *windows.SID
	DACL         []aclEntry
	NullDACL     bool
	ReparsePoint bool
	Directory    bool
}

func administrativeSID(sid *windows.SID) bool {
	if sid == nil {
		return false
	}
	s := sid.String()
	return s == sidSystem || s == sidAdministrators || s == sidTrustedInstaller
}

// principalName — «имя (SID)» для причины вердикта.
func principalName(sid *windows.SID) string {
	if sid == nil {
		return "(no SID)"
	}
	s := sid.String()
	if account, domain, _, err := sid.LookupAccount(""); err == nil && account != "" {
		if domain != "" && !strings.EqualFold(domain, "BUILTIN") && !strings.EqualFold(domain, "NT AUTHORITY") && !strings.EqualFold(domain, "NT SERVICE") {
			account = domain + `\` + account
		}
		return account + " (" + s + ")"
	}
	return s
}

func descriptorFacts(sd *windows.SECURITY_DESCRIPTOR, facts securityFacts) (securityFacts, error) {
	if owner, _, err := sd.Owner(); err == nil && owner != nil {
		facts.Owner = owner
	}
	dacl, _, err := sd.DACL()
	if errors.Is(err, windows.ERROR_OBJECT_NOT_FOUND) || (err == nil && dacl == nil) {
		facts.NullDACL = true
		return facts, nil
	}
	if err != nil {
		return facts, err
	}
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return facts, err
		}
		e := aclEntry{Type: ace.Header.AceType, Flags: ace.Header.AceFlags, Mask: uint32(ace.Mask)}
		switch e.Type {
		case aceAccessAllowed, aceAccessDenied, aceAccessAllowedCallback, aceAccessDeniedCallback:
			e.SID = (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		}
		facts.DACL = append(facts.DACL, e)
	}
	return facts, nil
}

// readSecurityFacts — владелец, DACL и тип звена по дескриптору, открытому
// без следования ссылкам.
func readSecurityFacts(path string) (securityFacts, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return securityFacts{}, err
	}
	h, err := windows.CreateFile(name, windows.READ_CONTROL|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return securityFacts{}, &os.PathError{Op: "open", Path: path, Err: err}
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var facts securityFacts
	var fi windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &fi); err != nil {
		return facts, fmt.Errorf("%s: %w", path, err)
	}
	facts.ReparsePoint = fi.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
	facts.Directory = fi.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0
	sd, err := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return facts, fmt.Errorf("read the security of %s: %w", path, err)
	}
	return descriptorFacts(sd, facts)
}

// aclViolation — инвариант одного звена: предок (ancestor) — владелец из
// {SYSTEM, Administrators, TrustedInstaller}, чужому SID нельзя заменить
// то, что ниже; каталог копии и набор — владелец SYSTEM или Administrators,
// чужому SID нельзя писать вовсе. Reparse point, NULL DACL и
// неразбираемые разрешающие ACE — отказ.
func aclViolation(path string, facts securityFacts, ancestor bool) error {
	if facts.ReparsePoint {
		return fmt.Errorf("%s is a reparse point, must be a real file or directory", path)
	}
	if ancestor {
		if !administrativeSID(facts.Owner) {
			return fmt.Errorf("%s: owner %s is not SYSTEM, Administrators or TrustedInstaller", path, principalName(facts.Owner))
		}
	} else if s := facts.Owner; s == nil || (s.String() != sidSystem && s.String() != sidAdministrators) {
		return fmt.Errorf("%s: owner %s is not SYSTEM or Administrators", path, principalName(facts.Owner))
	}
	if facts.NullDACL {
		return fmt.Errorf("%s has a NULL DACL (full access for everyone)", path)
	}
	forbidden := uint32(protectedForbidden)
	if ancestor {
		forbidden = ancestorForbidden
	}
	for _, e := range facts.DACL {
		if e.Flags&aceInheritOnly != 0 {
			continue
		}
		switch e.Type {
		case aceAccessDenied, aceAccessDeniedObject, aceAccessDeniedCallback, aceAccessDeniedCallbackObject:
			continue
		case aceAccessAllowed:
		default:
			return fmt.Errorf("%s: %s holds an access entry of type %d that cannot be evaluated", path, principalName(e.SID), e.Type)
		}
		if administrativeSID(e.SID) {
			continue
		}
		if granted := e.Mask & forbidden; granted != 0 {
			return fmt.Errorf("%s: %s is granted 0x%x, must not be writable by a non-administrative account", path, principalName(e.SID), granted)
		}
	}
	return nil
}

// serviceACLViolation — DACL службы: чужому SID ни CHANGE_CONFIG, ни
// WRITE_DAC/WRITE_OWNER/DELETE/GENERIC_WRITE|ALL (SPEC 141 §6.1).
func serviceACLViolation(facts securityFacts) error {
	if facts.NullDACL {
		return errors.New("the service has a NULL DACL (full access for everyone)")
	}
	for _, e := range facts.DACL {
		if e.Flags&aceInheritOnly != 0 {
			continue
		}
		switch e.Type {
		case aceAccessDenied, aceAccessDeniedObject, aceAccessDeniedCallback, aceAccessDeniedCallbackObject:
			continue
		case aceAccessAllowed:
		default:
			return fmt.Errorf("the service DACL gives %s an access entry of type %d that cannot be evaluated", principalName(e.SID), e.Type)
		}
		if administrativeSID(e.SID) {
			continue
		}
		if granted := e.Mask & serviceForbidden; granted != 0 {
			return fmt.Errorf("the service DACL grants %s 0x%x, which lets it point the service at another binary", principalName(e.SID), granted)
		}
	}
	return nil
}

// fixedNTFSVolume — корень тома path; не локальный NTFS — отказ.
func fixedNTFSVolume(path string) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	if err := windows.GetVolumePathName(name, &buf[0], uint32(len(buf))); err != nil {
		return "", fmt.Errorf("resolve the volume of %s: %w", path, err)
	}
	root := windows.UTF16ToString(buf)
	rootName, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return "", err
	}
	fs := make([]uint16, 64)
	if windows.GetDriveType(rootName) != windows.DRIVE_FIXED ||
		windows.GetVolumeInformation(rootName, nil, 0, nil, nil, nil, &fs[0], uint32(len(fs))) != nil ||
		!strings.EqualFold(windows.UTF16ToString(fs), "NTFS") {
		return "", fmt.Errorf("%s is not on a fixed NTFS volume", path)
	}
	return root, nil
}

// checkProtectedAncestors — правило предков от корня тома до родителя dir.
func checkProtectedAncestors(dir string) error {
	parent := filepath.Dir(filepath.Clean(dir))
	root, err := fixedNTFSVolume(parent)
	if err != nil {
		return err
	}
	root = filepath.Clean(root)
	rel, err := filepath.Rel(root, parent)
	if err != nil || rel == ".." || strings.HasPrefix(rel, `..\`) {
		return fmt.Errorf("%s is not below its volume root %s", parent, root)
	}
	chain := []string{root}
	if rel != "." {
		cur := root
		for _, part := range strings.Split(rel, `\`) {
			cur = filepath.Join(cur, part)
			chain = append(chain, cur)
		}
	}
	for _, component := range chain {
		facts, err := readSecurityFacts(component)
		if err != nil {
			return err
		}
		if err := aclViolation(component, facts, true); err != nil {
			return err
		}
		if !facts.Directory {
			return fmt.Errorf("%s is not a directory", component)
		}
	}
	return nil
}

// checkProtectedPath — правило копии для каталога или файла набора.
func checkProtectedPath(path string, dir bool) error {
	facts, err := readSecurityFacts(path)
	if err != nil {
		return err
	}
	if err := aclViolation(path, facts, false); err != nil {
		return err
	}
	switch {
	case dir && !facts.Directory:
		return fmt.Errorf("%s is not a directory", path)
	case !dir && facts.Directory:
		return fmt.Errorf("%s is a directory, not a file of the copy", path)
	}
	return nil
}

// CheckProtectedCopy — инвариант SPEC 141 §6.1 для каталога dir: предки от
// корня тома, сам каталог и файлы. required — члены, без которых копии нет
// (ErrProtectedPathMissing); optional проверяются, если лежат.
func CheckProtectedCopy(dir string, required, optional []string) error {
	if err := checkProtectedAncestors(dir); err != nil {
		return err
	}
	if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s", ErrProtectedPathMissing, dir)
	}
	if err := checkProtectedPath(dir, true); err != nil {
		return err
	}
	for _, name := range required {
		p := filepath.Join(dir, name)
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrProtectedPathMissing, p)
		}
		if err := checkProtectedPath(p, false); err != nil {
			return err
		}
	}
	for _, name := range optional {
		p := filepath.Join(dir, name)
		if _, err := os.Lstat(p); err != nil {
			continue
		}
		if err := checkProtectedPath(p, false); err != nil {
			return err
		}
	}
	return nil
}

// --- Ключ файла ----------------------------------------------------------

// FileKey — идентичность содержимого без чтения (SPEC 141 §6.2): замена
// файла (новый индекс) или запись (size/mtime) меняют ключ.
type FileKey struct {
	Volume  uint32
	Index   uint64
	Size    int64
	ModTime int64
}

// StatFileKey — ключ обычного файла path (ссылки разыменовываются, как у
// os.Stat).
func StatFileKey(path string) (FileKey, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return FileKey{}, err
	}
	h, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return FileKey{}, &os.PathError{Op: "open", Path: path, Err: err}
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var fi windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &fi); err != nil {
		return FileKey{}, fmt.Errorf("%s: %w", path, err)
	}
	if fi.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return FileKey{}, fmt.Errorf("%s is not a regular file", path)
	}
	return FileKey{
		Volume:  fi.VolumeSerialNumber,
		Index:   uint64(fi.FileIndexHigh)<<32 | uint64(fi.FileIndexLow),
		Size:    int64(fi.FileSizeHigh)<<32 | int64(fi.FileSizeLow),
		ModTime: fi.LastWriteTime.Nanoseconds(),
	}, nil
}
