package tabs

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/locale"
	wizardbusiness "singbox-launcher/ui/configurator/business"
)

// detourTreePicker — выбор цели detour двухуровневым деревом: кнопка с
// подписью текущего выбора открывает под собой попап, где «без detour»
// стоит первым, а цели разложены по веткам «Направления» и «Узлы».
//
// Плоский Select смешивал группы и узлы в одну простыню, и отличал их только
// маркер «» » перед узлом. Принадлежность теперь показывает ветка.
//
// Ключи опций те же, что отдаёт DetourOptionsWithNodes (у узлов — с
// маркером), поэтому обработчик выбора у окна не меняется.
type detourTreePicker struct {
	win       fyne.Window
	none      string
	options   []string
	choices   map[string]wizardbusiness.DetourChoice
	selected  string
	onChanged func(string)

	btn    *widget.Button
	label  *widget.Label
	widget fyne.CanvasObject
	popup  *widget.PopUp
}

const (
	detourTreeBranchDirections = "b:dir"
	detourTreeBranchNodes      = "b:node"
	detourTreeLeafPrefix       = "o:"
	// detourTreeMaxRows — сколько строк попап показывает без прокрутки.
	detourTreeMaxRows = 12
)

func newDetourTreePicker(win fyne.Window, none string) *detourTreePicker {
	p := &detourTreePicker{win: win, none: none, selected: none}
	// Подпись — отдельной меткой с обрезкой поверх кнопки без текста: текст
	// самой кнопки не обрезается, и длинное имя узла раздувало бы окно.
	p.label = widget.NewLabel(none)
	p.label.Truncation = fyne.TextTruncateEllipsis
	p.btn = widget.NewButton("", p.show)
	p.btn.Alignment = widget.ButtonAlignLeading
	icon := widget.NewIcon(theme.MenuDropDownIcon())
	p.widget = container.NewStack(p.btn, container.NewBorder(nil, nil, nil, icon, p.label))
	return p
}

// SetOptions — новый набор опций и выбор; onChanged не зовётся.
func (p *detourTreePicker) SetOptions(options []string, selected string, choices map[string]wizardbusiness.DetourChoice) {
	p.options = options
	p.choices = choices
	p.selected = selected
	p.label.SetText(p.display(selected))
}

// display — подпись опции: в дереве и на кнопке узел идёт без маркера.
func (p *detourTreePicker) display(opt string) string {
	c, ok := p.choices[opt]
	if !ok || c.Display == "" {
		return opt
	}
	if c.Missing {
		return c.Display + " ⚠ " + locale.T("(missing)")
	}
	return c.Display
}

// branchOf — ветка опции; пусто для «без detour».
func (p *detourTreePicker) branchOf(opt string) string {
	switch p.choices[opt].Kind {
	case wizardbusiness.DetourKindDirection:
		return detourTreeBranchDirections
	case wizardbusiness.DetourKindNode:
		return detourTreeBranchNodes
	}
	return ""
}

// show — попап-дерево под кнопкой; раскрыта ветка текущего выбора.
func (p *detourTreePicker) show() {
	if p.win == nil {
		return
	}
	children := map[string][]string{}
	var roots []string
	if p.none != "" {
		roots = append(roots, detourTreeLeafPrefix+p.none)
	}
	for _, opt := range p.options {
		if opt == p.none {
			continue
		}
		if b := p.branchOf(opt); b != "" {
			children[b] = append(children[b], detourTreeLeafPrefix+opt)
		}
	}
	// Пустая ветка не показывается вовсе.
	for _, b := range []string{detourTreeBranchDirections, detourTreeBranchNodes} {
		if len(children[b]) > 0 {
			roots = append(roots, b)
		}
	}
	children[""] = roots

	branchTitle := func(id string) string {
		title := locale.T("Directions")
		if id == detourTreeBranchNodes {
			title = locale.T("Nodes")
		}
		return title + " (" + strconv.Itoa(len(children[id])) + ")"
	}

	var tree *widget.Tree
	tree = widget.NewTree(
		func(id widget.TreeNodeID) []widget.TreeNodeID { return children[id] },
		func(id widget.TreeNodeID) bool { return id == "" || !strings.HasPrefix(id, detourTreeLeafPrefix) },
		func(bool) fyne.CanvasObject {
			check := widget.NewIcon(theme.ConfirmIcon())
			label := widget.NewLabel("")
			label.Truncation = fyne.TextTruncateEllipsis
			return container.NewBorder(nil, nil, check, nil, label)
		},
		func(id widget.TreeNodeID, branch bool, obj fyne.CanvasObject) {
			row := obj.(*fyne.Container)
			label := row.Objects[0].(*widget.Label)
			check := row.Objects[1].(*widget.Icon)
			if branch {
				check.Hide()
				label.TextStyle = fyne.TextStyle{Bold: true}
				label.SetText(branchTitle(id))
				return
			}
			opt := strings.TrimPrefix(id, detourTreeLeafPrefix)
			// Галка на выбранной строке, у остальных — пустое место той же
			// ширины, чтобы подписи стояли ровно.
			if opt == p.selected {
				check.SetResource(theme.ConfirmIcon())
			} else {
				check.SetResource(nil)
			}
			check.Show()
			label.TextStyle = fyne.TextStyle{}
			label.SetText(p.display(opt))
		},
	)

	resize := func() {}
	tree.OnSelected = func(id widget.TreeNodeID) {
		// Выделение дерева не держим: выбор показывает галка, а повторный
		// клик по той же строке должен срабатывать снова.
		tree.Unselect(id)
		if !strings.HasPrefix(id, detourTreeLeafPrefix) {
			tree.ToggleBranch(id)
			return
		}
		opt := strings.TrimPrefix(id, detourTreeLeafPrefix)
		p.hide()
		if opt == p.selected {
			return
		}
		p.selected = opt
		p.label.SetText(p.display(opt))
		if p.onChanged != nil {
			p.onChanged(opt)
		}
	}
	tree.OnBranchOpened = func(widget.TreeNodeID) { resize() }
	tree.OnBranchClosed = func(widget.TreeNodeID) { resize() }
	if b := p.branchOf(p.selected); b != "" {
		tree.OpenBranch(b)
	}

	p.hide()
	p.popup = widget.NewPopUp(tree, p.win.Canvas())
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(p.btn)
	pos.Y += p.btn.Size().Height

	// Высота — по числу видимых строк, не больше detourTreeMaxRows и не
	// ниже края окна; растёт и сжимается при раскрытии веток.
	th := theme.Current()
	rowH := fyne.Max(widget.NewLabel("Xg").MinSize().Height, th.Size(theme.SizeNameInlineIcon)) +
		th.Size(theme.SizeNamePadding)
	resize = func() {
		if p.popup == nil {
			return
		}
		rows := len(roots)
		for _, b := range roots {
			if tree.IsBranchOpen(b) {
				rows += len(children[b])
			}
		}
		if rows > detourTreeMaxRows {
			rows = detourTreeMaxRows
		}
		h := float32(rows)*rowH + 2*th.Size(theme.SizeNamePadding)
		if room := p.win.Canvas().Size().Height - pos.Y - th.Size(theme.SizeNamePadding); room > rowH && h > room {
			h = room
		}
		p.popup.Resize(fyne.NewSize(p.btn.Size().Width, h))
	}
	resize()
	p.popup.ShowAtPosition(pos)
	if p.selected != "" {
		tree.ScrollTo(detourTreeLeafPrefix + p.selected)
	}
}

func (p *detourTreePicker) hide() {
	if p.popup != nil {
		p.popup.Hide()
		p.popup = nil
	}
}
