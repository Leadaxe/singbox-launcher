package dialogs

// rule_process_picker.go — выбор процессов для поля вида `processes` в окне
// правила (SPEC 188): список запущенных процессов отдельным окном и поле
// выбранных имён с кнопками удаления.

import (
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"image/color"

	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/process"
)

// processListField — поле выбранных процессов: имена по алфавиту, «−» у
// каждого, «Select Processes...» открывает окно со списком.
type processListField struct {
	app      fyne.App
	selected []string
	list     *fyne.Container
	button   *widget.Button
	box      *fyne.Container
	onChange func()
}

func newProcessListField(app fyne.App, initial []string, onChange func()) *processListField {
	f := &processListField{app: app, onChange: onChange, list: container.NewVBox()}
	f.selected = dedupeProcessStrings(initial)
	sortProcessStrings(f.selected)
	f.button = widget.NewButton(locale.T("Select Processes..."), f.openPicker)
	// Поле в ~3 строки: список прокручивается, окно не растёт.
	size := canvas.NewRectangle(color.Transparent)
	size.SetMinSize(fyne.NewSize(0, 90))
	f.box = container.NewVBox(container.NewStack(size, container.NewVScroll(f.list)), f.button)
	f.refresh()
	return f
}

func (f *processListField) object() fyne.CanvasObject { return f.box }

func (f *processListField) values() []string {
	out := make([]string, len(f.selected))
	copy(out, f.selected)
	return out
}

func (f *processListField) setEnabled(on bool) {
	if on {
		f.button.Enable()
	} else {
		f.button.Disable()
	}
}

func (f *processListField) changed() {
	f.refresh()
	if f.onChange != nil {
		f.onChange()
	}
}

func (f *processListField) refresh() {
	f.selected = dedupeProcessStrings(f.selected)
	sortProcessStrings(f.selected)
	f.list.Objects = nil
	for i := range f.selected {
		idx := i
		lbl := widget.NewLabel(f.selected[i])
		lbl.Truncation = fyne.TextTruncateEllipsis
		remove := widget.NewButton(locale.T("−"), func() {
			f.selected = append(f.selected[:idx], f.selected[idx+1:]...)
			f.changed()
		})
		f.list.Add(container.NewBorder(nil, nil, nil, remove, lbl))
	}
	f.list.Refresh()
}

func (f *processListField) add(name string) {
	item := normalizeProcName(name)
	for _, s := range f.selected {
		if strings.EqualFold(s, item) {
			return
		}
	}
	f.selected = append(f.selected, item)
	f.changed()
}

// openPicker — окно со списком запущенных процессов (имена без повторов).
func (f *processListField) openPicker() {
	if f.app == nil {
		return
	}
	w := f.app.NewWindow(locale.T("Select Processes"))
	w.Resize(fyne.NewSize(500, 400))

	getProcesses := func() []string {
		procs, err := process.GetProcesses()
		if err != nil {
			return []string{}
		}
		items := make([]string, 0, len(procs))
		for _, p := range procs {
			items = append(items, p.Name)
		}
		items = dedupeProcessStrings(items)
		sortProcessStrings(items)
		return items
	}

	listData := getProcesses()
	selectedIdx := -1
	procList := widget.NewList(
		func() int { return len(listData) },
		func() fyne.CanvasObject { return container.NewHBox(widget.NewLabel(""), layout.NewSpacer()) },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			lbl := o.(*fyne.Container).Objects[0].(*widget.Label)
			lbl.SetText(listData[i])
		},
	)
	procList.OnSelected = func(id widget.ListItemID) { selectedIdx = id }

	addBtn := widget.NewButton(locale.T("+ Add"), func() {
		if selectedIdx >= 0 && selectedIdx < len(listData) {
			f.add(listData[selectedIdx])
		}
	})
	refreshBtn := widget.NewButton(locale.T("Refresh"), func() {
		listData = getProcesses()
		procList.Refresh()
	})
	closeBtn := widget.NewButton(locale.T("Close"), func() { w.Close() })

	content := container.NewBorder(nil, container.NewHBox(layout.NewSpacer(), refreshBtn, addBtn, closeBtn), nil, nil, container.NewScroll(procList))
	w.SetContent(content)
	w.Show()
}

// normalizeProcName strips the legacy "PID: name" prefix from a process string,
// returning just the trimmed process name. Inputs without that prefix are
// returned trimmed as-is.
func normalizeProcName(s string) string {
	parts := strings.SplitN(strings.TrimSpace(s), ": ", 2)
	if len(parts) == 2 {
		return strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(s)
}

// sortProcessStrings sorts process strings in place, case-insensitively by name.
func sortProcessStrings(items []string) {
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i]) < strings.ToLower(items[j])
	})
}

// dedupeProcessStrings normalizes each item (via normalizeProcName), drops empty
// results, and removes case-insensitive duplicates, preserving first-seen order.
func dedupeProcessStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		n := normalizeProcName(item)
		key := strings.ToLower(n)
		if n == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, n)
	}
	return out
}
