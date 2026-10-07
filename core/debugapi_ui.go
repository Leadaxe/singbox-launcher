package core

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

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
	// MinSize — минимум содержимого канвы: ниже него окно не сжимается.
	MinSize [2]int `json:"min_size"`
	// MinTree — кто этот минимум складывает: поддерево узлов, чья
	// минимальная высота не меньше uiMinTreeThreshold (остальные опущены —
	// иначе снимок Мастера был бы на тысячи строк).
	MinTree *uiMinNode `json:"min_tree,omitempty"`
}

// uiMinNode — узел дерева минимальных размеров (триаж «окно не сжимается»:
// AppTabs берёт максимум по всем вкладкам, и держатель минимума может
// лежать на невидимой вкладке).
type uiMinNode struct {
	Type     string       `json:"type"`
	Hidden   bool         `json:"hidden,omitempty"`
	Size     [2]int       `json:"size"`
	Min      [2]int       `json:"min"`
	Children []*uiMinNode `json:"children,omitempty"`
}

// uiMinTreeThreshold — минимальная высота узла, с которой он попадает в
// дерево; uiMinTreeDepth — предел глубины обхода.
const (
	uiMinTreeThreshold float32 = 60
	uiMinTreeDepth             = 16
)

// uiMinTree — обход известных контейнеров: *fyne.Container, Scroll, AppTabs,
// Split. Прочие виджеты — листья: их внутренности доступны только через
// внутренний кэш рендереров, а для поиска держателя минимума хватает
// контейнерного уровня.
func uiMinTree(o fyne.CanvasObject, depth int) *uiMinNode {
	if o == nil {
		return nil
	}
	min := o.MinSize()
	if min.Height < uiMinTreeThreshold && depth > 0 {
		return nil
	}
	n := &uiMinNode{Type: fmt.Sprintf("%T", o), Hidden: !o.Visible(), Size: sizeOf(o.Size()), Min: sizeOf(min)}
	if depth >= uiMinTreeDepth {
		return n
	}
	var kids []fyne.CanvasObject
	switch v := o.(type) {
	case *fyne.Container:
		kids = v.Objects
	case *container.Scroll:
		kids = []fyne.CanvasObject{v.Content}
	case *container.AppTabs:
		for _, it := range v.Items {
			kids = append(kids, it.Content)
		}
	case *container.DocTabs:
		for _, it := range v.Items {
			kids = append(kids, it.Content)
		}
	case *container.Split:
		kids = []fyne.CanvasObject{v.Leading, v.Trailing}
	}
	for _, k := range kids {
		if child := uiMinTree(k, depth+1); child != nil {
			n.Children = append(n.Children, child)
		}
	}
	return n
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
				info.MinSize = sizeOf(c.Content().MinSize())
				info.MinTree = uiMinTree(c.Content(), 0)
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
