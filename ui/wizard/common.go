package wizard

import (
	"fyne.io/fyne/v2"
)

// safeFyneDo safely calls fyne.Do only if window is still valid
func safeFyneDo(window fyne.Window, fn func()) {
	if window != nil {
		fyne.Do(fn)
	}
}
