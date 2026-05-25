package traffic

import (
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	tprof "singbox-launcher/internal/traffic"
)

// liveView is the system-wide stream tab. Maintains an in-memory ring of
// the most recent events, plus client-side filters (kind chips + search
// box + per-process filter). All filtering is done on display — the
// profiler streams everything to us.
type liveView struct {
	Content fyne.CanvasObject

	mu      sync.Mutex
	events  []tprof.TrafficEvent
	filter  liveFilter
	list    *widget.List
	statusL *widget.Label
	unsub   func()
}

// liveFilter is the user's current filter state. Defaults: everything on,
// no search.
type liveFilter struct {
	ShowDNS     bool
	ShowDNSFail bool
	ShowTCP     bool
	ShowTCPClose bool
	ShowUDP     bool
	Search      string
	Process     string // empty = all processes
}

func defaultLiveFilter() liveFilter {
	return liveFilter{
		ShowDNS:      true,
		ShowDNSFail:  true,
		ShowTCP:      true,
		ShowTCPClose: true,
		ShowUDP:      true,
	}
}

// liveViewRingSize caps the live view's in-memory list. Without a cap a
// chatty target could grow it forever; 5000 fits comfortably in a Fyne
// List and ~10 MB of RAM.
const liveViewRingSize = 5000

func buildLiveView(deps WindowDeps) *liveView {
	v := &liveView{filter: defaultLiveFilter()}

	v.statusL = widget.NewLabel("")
	updateStatus := func() {
		v.mu.Lock()
		n := len(v.events)
		v.mu.Unlock()
		v.statusL.SetText("Events in buffer: " + itoa(n) + "  (newest first)")
	}

	// Backfill from rolling buffer so user sees something immediately.
	snap := deps.Profiler.Snapshot(60 * time.Second)
	v.events = append(v.events, snap...)

	v.list = widget.NewList(
		func() int {
			v.mu.Lock()
			defer v.mu.Unlock()
			return len(v.filteredIndices())
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("...")
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			v.mu.Lock()
			idxs := v.filteredIndices()
			if i < 0 || i >= len(idxs) {
				v.mu.Unlock()
				return
			}
			e := v.events[len(v.events)-1-idxs[i]] // newest first
			v.mu.Unlock()
			label := o.(*widget.Label)
			line := formatEventRow(e)
			if e.ProcessName != "" {
				line += "   [" + e.ProcessName + "]"
			} else if e.ProcessPath != "" {
				line += "   [" + shortPath(e.ProcessPath) + "]"
			}
			label.SetText(line)
		},
	)

	// Filter controls.
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Search domain / IP / process…")
	searchEntry.OnChanged = func(s string) {
		v.mu.Lock()
		v.filter.Search = strings.ToLower(strings.TrimSpace(s))
		v.mu.Unlock()
		v.list.Refresh()
	}

	mkCheck := func(label string, get func(*liveFilter) *bool) *widget.Check {
		c := widget.NewCheck(label, func(b bool) {
			v.mu.Lock()
			*get(&v.filter) = b
			v.mu.Unlock()
			v.list.Refresh()
		})
		c.SetChecked(true)
		return c
	}
	chipDNS := mkCheck("DNS", func(f *liveFilter) *bool { return &f.ShowDNS })
	chipDNSx := mkCheck("DNS×", func(f *liveFilter) *bool { return &f.ShowDNSFail })
	chipTCP := mkCheck("TCP", func(f *liveFilter) *bool { return &f.ShowTCP })
	chipTCPc := mkCheck("TCP·", func(f *liveFilter) *bool { return &f.ShowTCPClose })
	chipUDP := mkCheck("UDP", func(f *liveFilter) *bool { return &f.ShowUDP })

	filterRow := container.NewHBox(
		chipDNS, chipDNSx, chipTCP, chipTCPc, chipUDP,
	)

	top := container.NewVBox(
		searchEntry,
		filterRow,
		v.statusL,
	)

	bannerVBox := container.NewVBox()
	rebuildBanners := func() {
		bannerVBox.Objects = nil
		if deps.SingBoxRunning != nil && !deps.SingBoxRunning() {
			bannerVBox.Add(buildBanner("Sing-box is not running — live events will appear after Start."))
		}
		if deps.FindProcessEnabled != nil && !deps.FindProcessEnabled() {
			bannerVBox.Add(buildBanner("Process detection disabled in template — events will lack process attribution. Enable route.find_process and Save in the wizard."))
		}
		bannerVBox.Refresh()
	}
	rebuildBanners()

	body := container.NewBorder(
		container.NewVBox(bannerVBox, top),
		nil, nil, nil,
		v.list,
	)
	v.Content = body
	updateStatus()

	// Subscribe and pump events. fyne.Do to marshal onto UI thread.
	ch, unsub := deps.Profiler.Subscribe()
	v.unsub = unsub
	go func() {
		for e := range ch {
			ee := e
			v.mu.Lock()
			v.events = append(v.events, ee)
			if len(v.events) > liveViewRingSize {
				v.events = v.events[len(v.events)-liveViewRingSize:]
			}
			v.mu.Unlock()
			fyne.Do(func() {
				v.list.Refresh()
				updateStatus()
				rebuildBanners()
			})
		}
	}()

	_ = layout.NewSpacer() // keep package import in case future toolbar
	return v
}

// Stop unsubscribes the view from the profiler. Called when the window
// closes.
func (v *liveView) Stop() {
	if v.unsub != nil {
		v.unsub()
		v.unsub = nil
	}
}

// filteredIndices returns indices into v.events (oldest→newest order)
// that pass the current filter. The list widget reverses for display.
// Caller must hold v.mu.
func (v *liveView) filteredIndices() []int {
	out := make([]int, 0, len(v.events))
	for i, e := range v.events {
		if !v.passes(e) {
			continue
		}
		out = append(out, i)
	}
	return out
}

func (v *liveView) passes(e tprof.TrafficEvent) bool {
	switch e.Kind {
	case tprof.EventDNSResolve:
		if !v.filter.ShowDNS {
			return false
		}
	case tprof.EventDNSFail:
		if !v.filter.ShowDNSFail {
			return false
		}
	case tprof.EventTCPOpen:
		if !v.filter.ShowTCP {
			return false
		}
	case tprof.EventTCPClose:
		if !v.filter.ShowTCPClose {
			return false
		}
	case tprof.EventUDPOpen, tprof.EventUDPClose:
		if !v.filter.ShowUDP {
			return false
		}
	}
	if v.filter.Process != "" && e.ProcessPath != v.filter.Process {
		return false
	}
	if s := v.filter.Search; s != "" {
		hay := strings.ToLower(e.Domain + " " + e.IP + " " + e.ProcessPath + " " + e.ProcessName)
		if !strings.Contains(hay, s) {
			return false
		}
	}
	return true
}

func buildBanner(text string) fyne.CanvasObject {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return container.NewVBox(l, widget.NewSeparator())
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	// avoid strconv import bloat — small custom impl
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

func shortPath(p string) string {
	// trim to basename for display
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	if i := strings.LastIndex(p, "\\"); i >= 0 {
		return p[i+1:]
	}
	return p
}
