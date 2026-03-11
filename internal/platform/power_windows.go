//go:build windows
// +build windows

package platform

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"singbox-launcher/internal/debuglog"

	"golang.org/x/sys/windows"
)

const (
	wmPowerBroadcast    = 0x0218
	pbtApmSuspend       = 4   // system entering sleep/hibernation
	pbtApmResumeSuspend = 7   // user-triggered resume
	pbtApmResumeAuto    = 18  // system resume (sleep/hibernation)
	wmQuit              = 0x0012
	// Defer resume callback so WM_POWERBROADCAST handler stays minimal (Windows recommendation).
	powerResumeCallbackDelay = 100 * time.Millisecond
)

var (
	powerResumeCallbackMu sync.Mutex
	powerResumeCallback   func()
	powerSleepCallbackMu  sync.Mutex
	powerSleepCallback    func()
	powerResumeThreadId   uint32
	powerResumeReady      = make(chan struct{})
	powerResumeStarted    int32
	powerSleeping         int32
	powerCtxMu            sync.RWMutex
	powerCtx              context.Context
	powerCancel           context.CancelFunc
)

// powerWndProc is the window procedure for the hidden power-notification window.
// It runs on the Windows message-loop thread; callbacks must be quick.
func powerWndProc(hwnd windows.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	if msg != wmPowerBroadcast {
		return defWindowProcW(hwnd, msg, wParam, lParam)
	}
	switch wParam {
	case pbtApmSuspend:
		// System entering sleep/hibernation: cancel in-flight work, set status, notify subscribers.
		debuglog.InfoLog("power: system entering sleep/hibernation")
		powerCtxMu.Lock()
		if powerCancel != nil {
			powerCancel()
		}
		powerCtxMu.Unlock()
		atomic.StoreInt32(&powerSleeping, 1)
		powerSleepCallbackMu.Lock()
		fn := powerSleepCallback
		powerSleepCallbackMu.Unlock()
		if fn != nil {
			fn()
		}
		return 1
	case pbtApmResumeSuspend, pbtApmResumeAuto:
		// System resumed: new context, clear status; run subscriber callback after a short delay so this handler stays minimal.
		debuglog.InfoLog("power: system resumed from sleep/hibernation (wParam=%d)", wParam)
		atomic.StoreInt32(&powerSleeping, 0)
		powerCtxMu.Lock()
		powerCtx, powerCancel = context.WithCancel(context.Background())
		powerCtxMu.Unlock()
		powerResumeCallbackMu.Lock()
		fn := powerResumeCallback
		powerResumeCallbackMu.Unlock()
		if fn != nil {
			time.AfterFunc(powerResumeCallbackDelay, fn)
		}
		return 1
	}
	return defWindowProcW(hwnd, msg, wParam, lParam)
}

// runPowerResumeListener creates a hidden window and runs the message loop on a locked OS thread.
// Closes powerResumeReady when the window is created and the loop is about to run, or on setup failure so the caller does not block.
func runPowerResumeListener() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tid := windows.GetCurrentThreadId()
	powerResumeThreadId = tid
	powerCtxMu.Lock()
	powerCtx, powerCancel = context.WithCancel(context.Background())
	powerCtxMu.Unlock()
	atomic.StoreInt32(&powerSleeping, 0)

	user32 := windows.NewLazySystemDLL("user32.dll")
	getModuleHandleW := windows.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
	registerClassExW := user32.NewProc("RegisterClassExW")
	createWindowExW := user32.NewProc("CreateWindowExW")
	getMessageW := user32.NewProc("GetMessageW")
		translateMessage := user32.NewProc("TranslateMessage")
		dispatchMessageW := user32.NewProc("DispatchMessageW")
	defWindowProcWProc := user32.NewProc("DefWindowProcW")

	defWindowProcW = func(hwnd windows.HWND, msg uint32, wParam, lParam uintptr) uintptr {
		r, _, _ := defWindowProcWProc.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return r
	}

	className, _ := windows.UTF16PtrFromString("SingboxPowerNotify")
	mod, _, _ := getModuleHandleW.Call(0)
	if mod == 0 {
		debuglog.ErrorLog("power: GetModuleHandleW failed")
		close(powerResumeReady)
		return
	}

	wndProc := windows.NewCallback(powerWndProc)
	wc := struct {
		Size, Style                       uint32
		WndProc                           uintptr
		ClsExtra, WndExtra                int32
		Instance, Icon, Cursor, Background uintptr
		MenuName, ClassName               *uint16
		IconSm                            uintptr
	}{
		Size:     uint32(unsafe.Sizeof(struct {
			Size, Style                       uint32
			WndProc                           uintptr
			ClsExtra, WndExtra                int32
			Instance, Icon, Cursor, Background uintptr
			MenuName, ClassName               *uint16
			IconSm                            uintptr
		}{})),
		WndProc:   wndProc,
		Instance:  mod,
		ClassName: className,
	}
	cls, _, err := registerClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if cls == 0 {
		debuglog.ErrorLog("power: RegisterClassExW failed: %v", err)
		close(powerResumeReady)
		return
	}

	hwnd, _, err := createWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("SingboxPowerNotify"))),
		0x80CF0000, // WS_OVERLAPPEDWINDOW
		0, 0, 0, 0,
		0, 0, mod, 0,
	)
	if hwnd == 0 {
		debuglog.ErrorLog("power: CreateWindowExW failed: %v", err)
		close(powerResumeReady)
		return
	}
	_ = windows.HWND(hwnd)

	close(powerResumeReady)
	debuglog.InfoLog("power: resume listener started (thread id %d)", tid)

	for {
		var msg struct {
			Hwnd    windows.HWND
			Message uint32
			WParam  uintptr
			LParam  uintptr
			Time    uint32
			Pt      struct{ X, Y int32 }
		}
		r, _, _ := getMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 {
			break
		}
		if int32(r) == -1 {
			debuglog.ErrorLog("power: GetMessage failed")
			break
		}
		if _, _, err := translateMessage.Call(uintptr(unsafe.Pointer(&msg))); err != nil && err != windows.ERROR_SUCCESS {
			debuglog.WarnLog("power: TranslateMessage failed: %v", err)
		}
		if _, _, err := dispatchMessageW.Call(uintptr(unsafe.Pointer(&msg))); err != nil && err != windows.ERROR_SUCCESS {
			debuglog.WarnLog("power: DispatchMessageW failed: %v", err)
		}
	}
}

var defWindowProcW func(hwnd windows.HWND, msg uint32, wParam, lParam uintptr) uintptr

// IsSleeping returns true when the system has entered sleep/hibernation and has not yet resumed.
// Subscribers should not start new work when true; in-flight work is cancelled via PowerContext().
func IsSleeping() bool {
	return atomic.LoadInt32(&powerSleeping) == 1
}

// PowerContext returns the current power context. It is cancelled when the system enters sleep; a new one is created on resume. Use for HTTP requests and timers so they abort on sleep.
func PowerContext() context.Context {
	powerCtxMu.RLock()
	defer powerCtxMu.RUnlock()
	if powerCtx != nil {
		return powerCtx
	}
	return context.Background()
}

// RegisterSleepCallback registers fn to be called when the system is entering sleep/hibernation.
func RegisterSleepCallback(fn func()) {
	powerSleepCallbackMu.Lock()
	powerSleepCallback = fn
	powerSleepCallbackMu.Unlock()
}

// RegisterPowerResumeCallback registers fn to be called when the system resumes from sleep or hibernation.
// Starts the power listener if needed. Call StopPowerResumeListener before process exit.
func RegisterPowerResumeCallback(fn func()) {
	powerResumeCallbackMu.Lock()
	powerResumeCallback = fn
	powerResumeCallbackMu.Unlock()

	if atomic.CompareAndSwapInt32(&powerResumeStarted, 0, 1) {
		powerResumeReady = make(chan struct{})
		go runPowerResumeListener()
		<-powerResumeReady
	}
}

// StopPowerResumeListener stops the power-resume listener. Safe to call if RegisterPowerResumeCallback was never called.
func StopPowerResumeListener() {
	if atomic.LoadInt32(&powerResumeStarted) == 0 {
		return
	}
	postThreadMessageW := windows.NewLazySystemDLL("user32.dll").NewProc("PostThreadMessageW")
	tid := powerResumeThreadId
	if tid != 0 {
		if _, _, err := postThreadMessageW.Call(uintptr(tid), wmQuit, 0, 0); err != nil && err != windows.ERROR_SUCCESS {
			debuglog.WarnLog("power: PostThreadMessageW failed for thread %d: %v", tid, err)
		} else {
			debuglog.DebugLog("power: posted WM_QUIT to thread %d", tid)
		}
	}
}
