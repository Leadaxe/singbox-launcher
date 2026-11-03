//go:build !darwin

package platform

// SendNotification has no platform-specific path outside macOS: the caller
// sends through fyne.App.SendNotification.
func SendNotification(title, content string) bool { return false }
