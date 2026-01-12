//go:build !darwin
// +build !darwin

package platform

import "log"

// HideDockIcon is a no-op on non-macOS platforms
func HideDockIcon() {
	log.Printf("platform: HideDockIcon is not implemented on non-darwin platforms")
}

// RestoreDockIcon is a no-op on non-macOS platforms
func RestoreDockIcon() {
	log.Printf("platform: RestoreDockIcon is not implemented on non-darwin platforms")
}
