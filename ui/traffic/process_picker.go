package traffic

import (
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/platform"
	tprof "singbox-launcher/internal/traffic"
)

// ShowProcessPicker opens a modal that lets the user pick one running
// process. On confirm, callback fires with the chosen process path. The
// list is OS-level (platform.ListProcesses) but augmented with processes
// the profiler has seen recent traffic for — those bubble to the top.
func ShowProcessPicker(parent fyne.Window, profiler *tprof.TrafficProfiler, onPick func(path, displayName string)) {
	osList, err := platform.ListProcesses()
	if err != nil {
		dialog.ShowError(err, parent)
		return
	}
	seen := profiler.SeenProcesses()

	// Build display rows: seen-with-traffic first (sorted by recency),
	// then OS-level (sorted alphabetically) skipping duplicates.
	type row struct {
		Path        string
		DisplayName string
		Hint        string // " (recently seen)" etc.
	}
	rows := make([]row, 0, len(seen)+len(osList))
	seenPaths := make(map[string]struct{})
	for _, ps := range seen {
		rows = append(rows, row{Path: ps.Path, DisplayName: displayOrFallback(ps.DisplayName, ps.Path), Hint: "  (recent traffic)"})
		seenPaths[ps.Path] = struct{}{}
	}
	osRows := make([]row, 0, len(osList))
	for _, p := range osList {
		if _, dup := seenPaths[p.Path]; dup {
			continue
		}
		osRows = append(osRows, row{Path: p.Path, DisplayName: displayOrFallback(p.DisplayName, p.Path)})
	}
	sort.Slice(osRows, func(i, j int) bool {
		return strings.ToLower(osRows[i].DisplayName) < strings.ToLower(osRows[j].DisplayName)
	})
	rows = append(rows, osRows...)

	// Filterable list.
	filter := ""
	filtered := func() []row {
		if filter == "" {
			return rows
		}
		f := strings.ToLower(filter)
		out := make([]row, 0, len(rows))
		for _, r := range rows {
			if strings.Contains(strings.ToLower(r.DisplayName), f) || strings.Contains(strings.ToLower(r.Path), f) {
				out = append(out, r)
			}
		}
		return out
	}

	selectedIdx := -1
	list := widget.NewList(
		func() int { return len(filtered()) },
		func() fyne.CanvasObject { return widget.NewLabel("...") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			items := filtered()
			if i < 0 || i >= len(items) {
				return
			}
			r := items[i]
			o.(*widget.Label).SetText(r.DisplayName + "  —  " + r.Path + r.Hint)
		},
	)
	list.OnSelected = func(id widget.ListItemID) { selectedIdx = id }

	search := widget.NewEntry()
	search.SetPlaceHolder("Filter by name or path…")
	search.OnChanged = func(s string) {
		filter = s
		list.Refresh()
		selectedIdx = -1
		list.UnselectAll()
	}

	body := container.NewBorder(search, nil, nil, nil, list)
	d := dialog.NewCustomConfirm("Pick process to record", "Start", "Cancel", body, func(ok bool) {
		if !ok || selectedIdx < 0 {
			return
		}
		items := filtered()
		if selectedIdx >= len(items) {
			return
		}
		picked := items[selectedIdx]
		onPick(picked.Path, picked.DisplayName)
	}, parent)
	d.Resize(fyne.NewSize(640, 480))
	d.Show()
}

func displayOrFallback(display, path string) string {
	if display != "" {
		return display
	}
	return shortPath(path)
}
