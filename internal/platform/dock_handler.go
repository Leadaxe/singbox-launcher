//go:build darwin
// +build darwin

package platform

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework Foundation

#import <Cocoa/Cocoa.h>
#import <Foundation/Foundation.h>
#import <objc/runtime.h>

// Forward declaration
extern void callGoDockCallback(void);

// Global flag to track if delegate is set
static int dockHandlerInstalled = 0;
static id dockDelegate = nil;

// Function to handle applicationShouldHandleReopen
static BOOL handleApplicationShouldHandleReopen(id self, SEL _cmd, NSApplication *sender, BOOL flag) {
    // If there are no visible windows, call the Go callback to show the window
    if (!flag) {
        callGoDockCallback();
    }
    return YES;
}

// SetupDockReopenHandler sets up the NSApplicationDelegate to handle Dock icon clicks
static void setupDockReopenHandlerImpl(void) {
    if (dockHandlerInstalled) {
        return;
    }

    // Get NSObject class
    Class nsObjectClass = objc_getClass("NSObject");
    if (nsObjectClass == nil) {
        return;
    }

    // Create a new class dynamically
    Class delegateClass = objc_allocateClassPair(nsObjectClass, "DockReopenHandler", 0);
    if (delegateClass == nil) {
        // Class might already exist, try to get it
        delegateClass = objc_getClass("DockReopenHandler");
        if (delegateClass == nil) {
            return;
        }
    } else {
        // Add the method to the class
        SEL selector = sel_registerName("applicationShouldHandleReopen:hasVisibleWindows:");
        // Method signature: c@:@c where:
        // c = BOOL (return type)
        // @ = id (self)
        // : = SEL (_cmd)
        // @ = NSApplication* (sender)
        // c = BOOL (flag)
        class_addMethod(delegateClass, selector, (IMP)handleApplicationShouldHandleReopen, "c@:@c");
        objc_registerClassPair(delegateClass);
    }

    // Create instance and set as delegate
    dockDelegate = [[delegateClass alloc] init];
    [NSApp setDelegate:dockDelegate];

    dockHandlerInstalled = 1;
}

// CleanupDockReopenHandler cleans up the delegate
static void cleanupDockReopenHandlerImpl(void) {
    if (!dockHandlerInstalled) {
        return;
    }
    if (dockDelegate != nil) {
        [NSApp setDelegate:nil];
        [dockDelegate release];
        dockDelegate = nil;
    }
    dockHandlerInstalled = 0;
}

// hideDockIconImpl hides the Dock icon by setting activation policy to Accessory
// NSApplicationActivationPolicyAccessory makes the app run without showing in Dock
// This is used for tray-only mode when user wants to hide the app from Dock
static void hideDockIconImpl(void) {
    [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
}

// Export functions for Go - using inline to avoid duplicate symbols
// CGO compiles code multiple times, so we use inline to avoid symbol duplication
static inline void singboxLauncherSetupDockReopenHandler(void) {
    setupDockReopenHandlerImpl();
}

static inline void singboxLauncherCleanupDockReopenHandler(void) {
    cleanupDockReopenHandlerImpl();
}

static inline void singboxLauncherHideDockIcon(void) {
    hideDockIconImpl();
}

// Non-inline wrappers for Go to call (inline functions can't be called from Go)
// Using weak linkage to allow multiple definitions - CGO compiles code multiple times
// and weak symbols allow the linker to choose one definition without errors
__attribute__((weak)) void callSetupDockReopenHandler(void) {
    singboxLauncherSetupDockReopenHandler();
}

__attribute__((weak)) void callCleanupDockReopenHandler(void) {
    singboxLauncherCleanupDockReopenHandler();
}

__attribute__((weak)) void callHideDockIcon(void) {
    singboxLauncherHideDockIcon();
}
*/
import "C"

import (
	"log"
	"runtime"
)

var dockReopenCallback func()

//export callGoDockCallback
func callGoDockCallback() {
	if dockReopenCallback != nil {
		dockReopenCallback()
	}
}

// SetupDockReopenHandler sets up macOS Dock icon click handler
// When user clicks Dock icon and there are no visible windows,
// the provided callback will be called to show the window
func SetupDockReopenHandler(showWindowCallback func()) {
	if runtime.GOOS != "darwin" {
		return
	}
	if showWindowCallback == nil {
		log.Println("SetupDockReopenHandler: callback is nil, skipping setup")
		return
	}

	// Store callback in Go variable
	dockReopenCallback = showWindowCallback

	// Setup the handler
	C.callSetupDockReopenHandler()
	log.Println("SetupDockReopenHandler: Dock reopen handler registered for macOS (using runtime API)")
}

// CleanupDockReopenHandler cleans up the Dock reopen handler
func CleanupDockReopenHandler() {
	if runtime.GOOS != "darwin" {
		return
	}
	C.callCleanupDockReopenHandler()
	dockReopenCallback = nil
	log.Println("CleanupDockReopenHandler: Dock reopen handler cleaned up")
}

// HideDockIcon hides the Dock icon on macOS (tray-only mode)
func HideDockIcon() {
	if runtime.GOOS != "darwin" {
		return
	}
	C.callHideDockIcon()
	log.Println("HideDockIcon: Dock icon hidden (NSApplicationActivationPolicyAccessory)")
}
