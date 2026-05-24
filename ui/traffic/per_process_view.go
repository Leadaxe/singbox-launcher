package traffic

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// perProcessView is the recording tab. Phase 4 stub — Phase 5 fills in
// process picker, 4 sub-tabs (Live/Domains/IPs/Connections), saved
// sessions list.
type perProcessView struct {
	Content    fyne.CanvasObject
	onRefresh  func()  // called when recording state changes (window title)
	stopFn     func()  // unsubscribe + tear down timers
}

func buildPerProcessView(deps WindowDeps, onRefresh func()) *perProcessView {
	v := &perProcessView{onRefresh: onRefresh}
	placeholder := widget.NewLabel("Per-process recording — see Phase 5")
	v.Content = container.NewCenter(placeholder)
	v.stopFn = func() {}
	_ = deps
	return v
}

func (v *perProcessView) Stop() {
	if v.stopFn != nil {
		v.stopFn()
	}
}
