//go:build !windows && !linux && !darwin
// +build !windows,!linux,!darwin

package platform

import "context"

// IsSleeping reports whether the system is in sleep/hibernation. On this platform it is always false.
func IsSleeping() bool {
	return false
}

// PowerContext returns a context for outgoing requests. On this platform it is never cancelled by power events.
func PowerContext() context.Context {
	return context.Background()
}

// RegisterSleepCallback registers a callback for sleep notification. On this platform it is a no-op.
func RegisterSleepCallback(fn func()) {}

// RegisterPowerResumeCallback registers a callback for resume notification. On this platform it is a no-op.
func RegisterPowerResumeCallback(fn func()) {}

// StopPowerResumeListener stops the power listener. On this platform it is a no-op.
func StopPowerResumeListener() {}
