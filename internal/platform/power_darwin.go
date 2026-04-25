//go:build darwin
// +build darwin

package platform

// macOS power-event hook: register with the IOPowerManagement system via
// IORegisterForSystemPower and dispatch sleep / wake callbacks. Closes the
// macOS half of SPEC 011 (Linux + Windows previously).
//
// References:
//   https://developer.apple.com/documentation/iokit/iopmlib_h
//   https://developer.apple.com/library/archive/qa/qa1340/_index.html
//
// Threading: CFRunLoop must run on the OS thread that added the source. We
// pin a goroutine via runtime.LockOSThread, install the run-loop source on
// that thread, then call CFRunLoopRun (blocks until CFRunLoopStop).
//
// Sleep flow: kIOMessageCanSystemSleep (system asks if we can sleep) and
// kIOMessageSystemWillSleep (sleep is imminent) both require an
// IOAllowPowerChange reply or the system delays a few seconds. We always
// allow — the launcher never has a reason to veto. kIOMessageSystemHasPoweredOn
// is the resume edge.
//
// Robustness:
//   - If IORegisterForSystemPower fails (sandbox / unusual env), the listener
//     never starts and callbacks never fire — same observable behavior as the
//     stub on minimal Linux.
//   - Dispatch happens off the run-loop thread to keep the system happy
//     (callbacks should be quick); user callbacks may take their time.

/*
#cgo darwin LDFLAGS: -framework IOKit -framework CoreFoundation
#include <stdlib.h>
#include <IOKit/IOKitLib.h>
#include <IOKit/IOMessage.h>
#include <IOKit/pwr_mgt/IOPMLib.h>
#include <CoreFoundation/CoreFoundation.h>

extern void goPowerCallback(unsigned int messageType);

// Holds the system-power root port. Captured during startPowerListener so the
// callback can call IOAllowPowerChange. Single-listener model: we never
// register twice in one process lifetime.
static io_connect_t g_pmRootPort = 0;

static void cPowerCallback(void *refcon, io_service_t service, natural_t messageType, void *messageArgument) {
    // Forward the message into Go before replying so the resume edge isn't
    // delayed by user callbacks (Go side dispatches asynchronously).
    goPowerCallback((unsigned int)messageType);

    // Sleep transitions require an explicit IOAllowPowerChange reply
    // (or the system stalls 30s waiting for one). We always allow.
    if (messageType == kIOMessageCanSystemSleep ||
        messageType == kIOMessageSystemWillSleep) {
        IOAllowPowerChange(g_pmRootPort, (long)messageArgument);
    }
}

// startPowerListener registers with the system-power notification port and
// returns the IONotificationPortRef. Returns NULL on failure.
static IONotificationPortRef startPowerListener() {
    IONotificationPortRef port = NULL;
    io_object_t notifier = IO_OBJECT_NULL;
    g_pmRootPort = IORegisterForSystemPower(NULL, &port, cPowerCallback, &notifier);
    if (g_pmRootPort == MACH_PORT_NULL) {
        return NULL;
    }
    return port;
}

// runPowerLoop installs the notification source on the current thread's
// CFRunLoop and blocks until CFRunLoopStop is called. Caller MUST be on a
// locked OS thread.
static void runPowerLoop(IONotificationPortRef port) {
    CFRunLoopAddSource(CFRunLoopGetCurrent(),
                       IONotificationPortGetRunLoopSource(port),
                       kCFRunLoopDefaultMode);
    CFRunLoopRun();
}

static void stopPowerLoop() {
    CFRunLoopStop(CFRunLoopGetMain());
}

// Constant accessors — using a static function avoids tying every Go-side
// call to the cgo macro expansion of these IOKit message identifiers.
static unsigned int kMsgCanSystemSleep()     { return kIOMessageCanSystemSleep; }
static unsigned int kMsgSystemWillSleep()    { return kIOMessageSystemWillSleep; }
static unsigned int kMsgSystemWillNotSleep() { return kIOMessageSystemWillNotSleep; }
static unsigned int kMsgSystemHasPoweredOn() { return kIOMessageSystemHasPoweredOn; }
*/
import "C"

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"

	"singbox-launcher/internal/debuglog"
)

var (
	powerCallbacksMu sync.Mutex
	sleepCallbacks   []func()
	resumeCallbacks  []func()
	listenerStarted  bool

	sleepingFlag atomic.Bool

	powerCtx       context.Context = context.Background()
	powerCtxCancel context.CancelFunc
)

// IsSleeping reports whether macOS has signalled an imminent sleep but not
// yet a resume. True between kIOMessageSystemWillSleep and
// kIOMessageSystemHasPoweredOn.
func IsSleeping() bool {
	return sleepingFlag.Load()
}

// PowerContext returns a context that callers can use for outgoing work;
// cancelled at each suspend transition and replaced on resume so stale
// requests started before sleep don't linger on dead sockets.
func PowerContext() context.Context {
	powerCallbacksMu.Lock()
	defer powerCallbacksMu.Unlock()
	return powerCtx
}

// RegisterSleepCallback registers fn to run when the system is about to sleep.
func RegisterSleepCallback(fn func()) {
	if fn == nil {
		return
	}
	powerCallbacksMu.Lock()
	sleepCallbacks = append(sleepCallbacks, fn)
	startListenerLocked()
	powerCallbacksMu.Unlock()
}

// RegisterPowerResumeCallback registers fn to run when the system resumes.
func RegisterPowerResumeCallback(fn func()) {
	if fn == nil {
		return
	}
	powerCallbacksMu.Lock()
	resumeCallbacks = append(resumeCallbacks, fn)
	startListenerLocked()
	powerCallbacksMu.Unlock()
}

// StopPowerResumeListener tears the listener down — optional, idempotent.
// Note: the underlying CFRunLoop thread isn't always reachable through
// CFRunLoopGetMain (we don't run on the main thread); a clean stop would
// require capturing the run-loop reference at start. For a launcher process
// that lives until quit, this is acceptable.
func StopPowerResumeListener() {
	powerCallbacksMu.Lock()
	defer powerCallbacksMu.Unlock()
	if powerCtxCancel != nil {
		powerCtxCancel()
	}
	listenerStarted = false
	powerCtx = context.Background()
	powerCtxCancel = nil
}

// startListenerLocked fires up the IOKit registration and CFRunLoop goroutine
// exactly once. Must be called with powerCallbacksMu held.
func startListenerLocked() {
	if listenerStarted {
		return
	}

	port := C.startPowerListener()
	if port == nil {
		debuglog.WarnLog("platform/power_darwin: IORegisterForSystemPower failed — power hooks inactive")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	powerCtx = ctx
	powerCtxCancel = cancel
	listenerStarted = true

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		C.runPowerLoop(port)
	}()
	debuglog.InfoLog("platform/power_darwin: subscribed to IOKit system-power notifications")
}

//export goPowerCallback
func goPowerCallback(messageType C.uint) {
	switch uint32(messageType) {
	case uint32(C.kMsgSystemWillSleep()):
		// Sleep edge: cancel power context, fire sleep-callbacks.
		sleepingFlag.Store(true)
		powerCallbacksMu.Lock()
		if powerCtxCancel != nil {
			powerCtxCancel()
		}
		powerCtx = context.Background()
		powerCtxCancel = nil
		cbs := append([]func(){}, sleepCallbacks...)
		powerCallbacksMu.Unlock()
		// Run callbacks asynchronously so the IOKit reply path
		// (IOAllowPowerChange in the C callback) isn't delayed.
		go func() {
			for _, cb := range cbs {
				cb()
			}
		}()
	case uint32(C.kMsgSystemHasPoweredOn()):
		// Resume edge: fresh context, fire resume-callbacks.
		sleepingFlag.Store(false)
		powerCallbacksMu.Lock()
		ctx, cancel := context.WithCancel(context.Background())
		powerCtx = ctx
		powerCtxCancel = cancel
		cbs := append([]func(){}, resumeCallbacks...)
		powerCallbacksMu.Unlock()
		go func() {
			for _, cb := range cbs {
				cb()
			}
		}()
	default:
		// kIOMessageCanSystemSleep / kIOMessageSystemWillNotSleep / others:
		// no Go-side action, the C side handles IOAllowPowerChange.
		_ = C.kMsgCanSystemSleep()
		_ = C.kMsgSystemWillNotSleep()
	}
}
