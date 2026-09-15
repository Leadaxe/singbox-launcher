//go:build !darwin
// +build !darwin

package platform

import (
	"time"

	"singbox-launcher/internal/debuglog"
)

// HideDockIcon is a no-op on non-macOS platforms
func HideDockIcon() {
	debuglog.DebugLog("platform: HideDockIcon is not implemented on non-darwin platforms")
}

// RestoreDockIcon is a no-op on non-macOS platforms
func RestoreDockIcon() {
	debuglog.DebugLog("platform: RestoreDockIcon is not implemented on non-darwin platforms")
}

// SetQuitRequestHandler is a no-op on non-macOS platforms: the quit request
// it handles comes from the AppKit application delegate.
func SetQuitRequestHandler(func(), time.Duration) {}
