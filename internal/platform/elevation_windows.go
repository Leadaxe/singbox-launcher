//go:build windows

package platform

// Права по требованию (SPEC 139): лаунчер собран с манифестом asInvoker и
// повышается только по явному действию пользователя. Здесь — проверка токена,
// запуск процесса через «runas» (ShellExecuteExW) и ожидание выхода
// родителя при перезапуске с повышением. RunElevated — общий примитив и для
// команд службы sing-box-lxd (SPEC 141 §5.2).

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"singbox-launcher/internal/debuglog"
)

var (
	elevatedOnce sync.Once
	elevated     bool
)

// IsElevated — процесс работает с правами администратора (SPEC 139 §2 п. 2).
// Считается один раз: токен процесса не меняется. Повышенным считается токен
// с TokenElevation или с действующим членством в BUILTIN\Administrators —
// второе покрывает машины с выключенным UAC, где у администратора полный
// токен без признака повышения.
func IsElevated() bool {
	elevatedOnce.Do(func() {
		elevated = tokenElevated()
	})
	return elevated
}

func tokenElevated() bool {
	tok, err := windows.OpenCurrentProcessToken()
	if err != nil {
		debuglog.WarnLog("elevation: open process token: %v", err)
		return false
	}
	defer debuglog.RunAndLog("elevation: close process token", tok.Close)
	if tok.IsElevated() {
		return true
	}
	sid, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		debuglog.WarnLog("elevation: administrators SID: %v", err)
		return false
	}
	// Token(0) — токен текущего потока или процесса: CheckTokenMembership
	// требует токен олицетворения и сам делает его из процессного. У
	// администратора под UAC группа в фильтрованном токене deny-only — false.
	member, err := windows.Token(0).IsMember(sid)
	if err != nil {
		debuglog.WarnLog("elevation: administrators membership: %v", err)
		return false
	}
	return member
}

// tokenElevationTypeDefault — TokenElevationTypeDefault: у токена нет
// связанного (обычный пользователь или выключенный UAC).
const tokenElevationTypeDefault = 1

// ElevationAsksOtherAccount — повышение спросит учётные данные другой
// учётной записи: процесс не повышен, а у токена нет связанного полного
// (обычный пользователь, SPEC 139 §4). Администратор под UAC получает
// окно подтверждения, а не пароль.
func ElevationAsksOtherAccount() bool {
	if IsElevated() {
		return false
	}
	tok, err := windows.OpenCurrentProcessToken()
	if err != nil {
		debuglog.WarnLog("elevation: open process token: %v", err)
		return false
	}
	defer debuglog.RunAndLog("elevation: close process token", tok.Close)
	var t, n uint32
	if err := windows.GetTokenInformation(tok, windows.TokenElevationType, (*byte)(unsafe.Pointer(&t)), uint32(unsafe.Sizeof(t)), &n); err != nil {
		debuglog.WarnLog("elevation: token elevation type: %v", err)
		return false
	}
	return t == tokenElevationTypeDefault
}

// AdminCleanupTasks — очистки старта, которым нужны права администратора
// (SPEC 139 §6 п. 2–3): запись NLA-профилей в HKLM, удаление правил
// брандмауэра и, только на Win7, DIF_REMOVE призрачных адаптеров. Имена —
// для одной строки INFO о пропуске.
func AdminCleanupTasks() []string {
	tasks := []string{"NLA profile cleanup", "orphan firewall rule cleanup"}
	if isWindows7() {
		tasks = append(tasks, "ghost adapter cleanup (Win7)")
	}
	return tasks
}

// ShellExecuteExW нет в x/sys/windows (ни v0.25.0 для Win7, ни v0.47.0):
// там только ShellExecute, без дескриптора процесса.
var (
	modShell32          = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW = modShell32.NewProc("ShellExecuteExW")
)

// Флаги SHELLEXECUTEINFOW.fMask.
const (
	seeMaskNoCloseProcess = 0x00000040 // вернуть hProcess
	seeMaskNoAsync        = 0x00000100 // дождаться запуска: поток сразу после вызова может завершиться
	seeMaskFlagNoUI       = 0x00000400 // без собственного окна ошибки: её показывает вызывающий
)

// sFalse — S_FALSE от CoInitializeEx: COM на потоке уже инициализирован в
// том же режиме. Успех, как и S_OK (nil), и требует парного CoUninitialize.
const sFalse = syscall.Errno(1)

// shellExecuteInfo — SHELLEXECUTEINFOW. Выравнивание полей Go совпадает с C
// и на amd64, и на 386.
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         windows.HWND
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     windows.Handle
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    windows.Handle
	dwHotKey     uint32
	hIconMonitor windows.Handle
	hProcess     windows.Handle
}

// ElevatedProcess — процесс, запущенный RunElevated. Дескриптор держится до
// Close: по нему Wait ждёт выхода и читает код (SPEC 141 §5.2).
type ElevatedProcess struct {
	Pid    int
	handle windows.Handle
}

// Wait ждёт выхода процесса не дольше timeout (отрицательный — без
// ограничения). exited=false — таймаут, процесс жив и не трогается.
func (p *ElevatedProcess) Wait(timeout time.Duration) (exitCode uint32, exited bool, err error) {
	if p == nil || p.handle == 0 {
		return 0, false, errors.New("no process handle")
	}
	ev, err := windows.WaitForSingleObject(p.handle, waitMillis(timeout))
	if err != nil {
		return 0, false, fmt.Errorf("wait for pid %d: %w", p.Pid, err)
	}
	if ev == uint32(windows.WAIT_TIMEOUT) {
		return 0, false, nil
	}
	var code uint32
	if err := windows.GetExitCodeProcess(p.handle, &code); err != nil {
		return 0, true, fmt.Errorf("exit code of pid %d: %w", p.Pid, err)
	}
	return code, true, nil
}

// Close закрывает дескриптор процесса; сам процесс не трогается.
func (p *ElevatedProcess) Close() error {
	if p == nil || p.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(p.handle)
	p.handle = 0
	return err
}

// RunElevated запускает exe с аргументами args от имени администратора:
// ShellExecuteExW с глаголом «runas» (окно UAC). Блокирует до ответа
// пользователя — звать из отдельной горутины, не из UI-потока. Отказ в UAC —
// ErrElevationCancelled.
//
// Аргументы собираются в строку по правилам CommandLineToArgvW
// (windows.ComposeCommandLine). dir — рабочий каталог процесса, show —
// ElevatedShowNormal или ElevatedShowHidden. Владелец окна UAC — окно на
// переднем плане в момент вызова: иначе запрос может открыться за окном
// лаунчера (SPEC 139 §5 п. 3).
//
// Поток закрепляется (LockOSThread) и инициализирует COM: ShellExecuteEx
// может отдать запуск расширениям оболочки, которым нужен STA-апартамент.
func RunElevated(exe string, args []string, dir string, show int) (*ElevatedProcess, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err == nil || err == sFalse {
		defer windows.CoUninitialize()
	} else {
		// RPC_E_CHANGED_MODE и т.п.: COM на потоке уже в другом режиме —
		// ShellExecuteEx работает и так, парный CoUninitialize не нужен.
		debuglog.DebugLog("RunElevated: CoInitializeEx: %v", err)
	}

	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return nil, err
	}
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return nil, fmt.Errorf("executable %q: %w", exe, err)
	}
	params, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(args))
	if err != nil {
		return nil, fmt.Errorf("arguments: %w", err)
	}
	var directory *uint16
	if dir != "" {
		if directory, err = windows.UTF16PtrFromString(dir); err != nil {
			return nil, fmt.Errorf("directory %q: %w", dir, err)
		}
	}

	info := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess | seeMaskNoAsync | seeMaskFlagNoUI,
		hwnd:         windows.GetForegroundWindow(),
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: params,
		lpDirectory:  directory,
		nShow:        int32(show),
	}
	info.cbSize = uint32(unsafe.Sizeof(info))

	debuglog.DebugLog("RunElevated: runas %s %s", exe, windows.ComposeCommandLine(args))
	ok, _, callErr := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return nil, ErrElevationCancelled
		}
		return nil, fmt.Errorf("run as administrator %s: %w", filepath.Base(exe), callErr)
	}
	p := &ElevatedProcess{handle: info.hProcess}
	if info.hProcess != 0 {
		if pid, err := windows.GetProcessId(info.hProcess); err == nil {
			p.Pid = int(pid)
		}
	}
	debuglog.DebugLog("RunElevated: started pid %d", p.Pid)
	return p, nil
}

// WaitForProcessExit ждёт выхода процесса pid, запущенного из того же exe,
// не дольше timeout (SPEC 139 §5 п. 6): новый повышенный экземпляр ждёт
// родителя, прежде чем трогать логи, порт Debug API и трей.
//
//   - процесса нет — exited=true сразу;
//   - образ процесса — не exe (PID уже занят другим процессом) — не ждать,
//     exited=true;
//   - дескриптор не открыть (родитель под другой учётной записью) — опрос
//     списка процессов раз в 250 мс.
//
// exited=false — таймаут: вызывающий продолжает старт с WARN.
func WaitForProcessExit(pid int, exe string, timeout time.Duration) (exited bool, err error) {
	if pid <= 0 {
		return true, nil
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return true, nil // такого PID уже нет
		}
		debuglog.DebugLog("WaitForProcessExit: OpenProcess(%d): %v; polling the process list", pid, err)
		return pollProcessGone(pid, timeout)
	}
	defer debuglog.RunAndLog("WaitForProcessExit: close process handle", func() error { return windows.CloseHandle(h) })

	if image, err := processImageOf(h); err == nil && !strings.EqualFold(filepath.Clean(image), filepath.Clean(exe)) {
		debuglog.DebugLog("WaitForProcessExit: pid %d is %s, not the launcher; not waiting", pid, image)
		return true, nil
	}
	ev, err := windows.WaitForSingleObject(h, waitMillis(timeout))
	if err != nil {
		return false, fmt.Errorf("wait for pid %d: %w", pid, err)
	}
	return ev != uint32(windows.WAIT_TIMEOUT), nil
}

// processImageOf — полный путь образа процесса по открытому дескриптору.
func processImageOf(h windows.Handle) (string, error) {
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:n]), nil
}

// waitMillis — таймаут для WaitForSingleObject; отрицательный — INFINITE.
func waitMillis(timeout time.Duration) uint32 {
	if timeout < 0 {
		return windows.INFINITE
	}
	ms := timeout.Milliseconds()
	if ms >= int64(windows.INFINITE) {
		return windows.INFINITE - 1
	}
	return uint32(ms)
}
