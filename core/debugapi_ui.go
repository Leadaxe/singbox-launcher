package core

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"

	"singbox-launcher/core/debugapi"
)

// uiCallTimeout — сколько ждать главный цикл Fyne. Живой цикл отвечает за
// миллисекунды; молчание дольше — сам по себе диагноз (цикл заблокирован).
const uiCallTimeout = 5 * time.Second

// fyneUIInspector реализует debugapi.UIInspector поверх fyne.CurrentApp().
// Все обращения к объектам канваса — через fyne.DoAndWait: снаружи главной
// горутины трогать дерево виджетов нельзя.
type fyneUIInspector struct{}

type uiOverlayInfo struct {
	Type    string   `json:"type"`
	Visible bool     `json:"visible"`
	Pos     [2]int   `json:"pos"`
	Size    [2]int   `json:"size"`
	Objects []string `json:"objects,omitempty"`
}

type uiWindowInfo struct {
	Title      string          `json:"title"`
	CanvasSize [2]int          `json:"canvas_size"`
	Content    string          `json:"content"`
	Focused    string          `json:"focused,omitempty"`
	Overlays   []uiOverlayInfo `json:"overlays"`
}

// onMain выполняет fn на главной горутине с таймаутом.
func onMain(fn func()) error {
	done := make(chan struct{})
	go func() {
		fyne.DoAndWait(fn)
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(uiCallTimeout):
		return debugapi.ErrUILoopUnresponsive
	}
}

func (fyneUIInspector) Snapshot() (any, error) {
	var out []uiWindowInfo
	err := onMain(func() {
		app := fyne.CurrentApp()
		if app == nil || app.Driver() == nil {
			return
		}
		for _, w := range app.Driver().AllWindows() {
			c := w.Canvas()
			if c == nil {
				continue
			}
			info := uiWindowInfo{Title: w.Title(), CanvasSize: sizeOf(c.Size()), Overlays: []uiOverlayInfo{}}
			if c.Content() != nil {
				info.Content = fmt.Sprintf("%T", c.Content())
			}
			if f := c.Focused(); f != nil {
				info.Focused = fmt.Sprintf("%T", f)
			}
			for _, o := range c.Overlays().List() {
				info.Overlays = append(info.Overlays, describeOverlay(o))
			}
			out = append(out, info)
		}
	})
	if out == nil {
		out = []uiWindowInfo{}
	}
	return out, err
}

func (fyneUIInspector) ClearOverlays() (int, error) {
	n := 0
	err := onMain(func() {
		app := fyne.CurrentApp()
		if app == nil || app.Driver() == nil {
			return
		}
		for _, w := range app.Driver().AllWindows() {
			c := w.Canvas()
			if c == nil {
				continue
			}
			// Remove(x) снимает x и всё над ним — идём от верхнего вниз, чтобы
			// счётчик отражал реально снятые.
			list := c.Overlays().List()
			for i := len(list) - 1; i >= 0; i-- {
				c.Overlays().Remove(list[i])
				n++
			}
		}
	})
	return n, err
}

func describeOverlay(o fyne.CanvasObject) uiOverlayInfo {
	info := uiOverlayInfo{
		Type:    fmt.Sprintf("%T", o),
		Visible: o.Visible(),
		Pos:     posOf(o.Position()),
		Size:    sizeOf(o.Size()),
	}
	if c, ok := o.(*fyne.Container); ok {
		for i, child := range c.Objects {
			if i >= 8 {
				info.Objects = append(info.Objects, fmt.Sprintf("… +%d", len(c.Objects)-i))
				break
			}
			s := fmt.Sprintf("%T", child)
			if !child.Visible() {
				s += " (hidden)"
			}
			info.Objects = append(info.Objects, s)
		}
	}
	return info
}

func sizeOf(s fyne.Size) [2]int    { return [2]int{int(s.Width), int(s.Height)} }
func posOf(p fyne.Position) [2]int { return [2]int{int(p.X), int(p.Y)} }
