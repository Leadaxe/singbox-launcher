package tabs

import (
	"image/color"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
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
// Попап — плоский список строк с отступом, а не widget.Tree: стрелки дерева
// Fyne рисуются треугольниками в полный размер иконки и не настраиваются.
// Здесь у веток тонкий уголок, у выбора — галка, обе по detourIconSize.
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

// detourTreeRow — строка попапа: ветка или опция (лист).
type detourTreeRow struct {
	branch wizardbusiness.DetourKind // ≠ DetourKindNone — строка-ветка
	opt    string                    // ключ опции у листа
}

const (
	// detourTreeMaxRows — сколько строк попап показывает без прокрутки.
	detourTreeMaxRows = 12
	// detourIconSize — уголки и галка: мельче штатной иконки строки.
	detourIconSize = 14
)

// Уголки — Material chevron_right / expand_more: заливка, а не обводка,
// чтобы тема перекрашивала их так же, как штатные иконки.
var (
	detourChevronRight = theme.NewColoredResource(fyne.NewStaticResource("chevron-right.svg", []byte(
		`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"><path d="M10 6L8.59 7.41 13.17 12l-4.58 4.59L10 18l6-6z"/></svg>`)),
		theme.ColorNamePlaceHolder)
	detourChevronDown = theme.NewColoredResource(fyne.NewStaticResource("chevron-down.svg", []byte(
		`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24"><path d="M16.59 8.59L12 13.17 7.41 8.59 6 10l6 6 6-6z"/></svg>`)),
		theme.ColorNamePlaceHolder)
	detourCheck = theme.NewPrimaryThemedResource(theme.ConfirmIcon())
)

// detourSmallIcon — иконка фиксированного размера, по центру по вертикали.
func detourSmallIcon(res fyne.Resource) (*widget.Icon, fyne.CanvasObject) {
	icon := widget.NewIcon(res)
	return icon, container.NewCenter(container.NewGridWrap(fyne.NewSquareSize(detourIconSize), icon))
}

func newDetourTreePicker(win fyne.Window, none string) *detourTreePicker {
	p := &detourTreePicker{win: win, none: none, selected: none}
	// Подпись — отдельной меткой с обрезкой поверх кнопки без текста: текст
	// самой кнопки не обрезается, и длинное имя узла раздувало бы окно.
	p.label = widget.NewLabel(none)
	p.label.Truncation = fyne.TextTruncateEllipsis
	p.btn = widget.NewButton("", p.show)
	_, arrow := detourSmallIcon(detourChevronDown)
	arrowBox := container.NewHBox(arrow, detourSpacer(theme.Padding()))
	p.widget = container.NewStack(p.btn, container.NewBorder(nil, nil, nil, arrowBox, p.label))
	return p
}

// detourSpacer — пустое место заданной ширины.
func detourSpacer(w float32) *canvas.Rectangle {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(w, 0))
	return r
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

// show — попап под кнопкой; раскрыта ветка текущего выбора.
func (p *detourTreePicker) show() {
	if p.win == nil {
		return
	}
	branches := []wizardbusiness.DetourKind{wizardbusiness.DetourKindDirection, wizardbusiness.DetourKindNode}
	children := map[wizardbusiness.DetourKind][]string{}
	for _, opt := range p.options {
		if opt == p.none {
			continue
		}
		if k := p.choices[opt].Kind; k != wizardbusiness.DetourKindNone {
			children[k] = append(children[k], opt)
		}
	}
	open := map[wizardbusiness.DetourKind]bool{p.choices[p.selected].Kind: true}

	var rows []detourTreeRow
	rebuild := func() {
		rows = rows[:0]
		if p.none != "" {
			rows = append(rows, detourTreeRow{opt: p.none})
		}
		// Пустая ветка не показывается вовсе.
		for _, b := range branches {
			if len(children[b]) == 0 {
				continue
			}
			rows = append(rows, detourTreeRow{branch: b})
			if open[b] {
				for _, opt := range children[b] {
					rows = append(rows, detourTreeRow{opt: opt})
				}
			}
		}
	}
	rebuild()

	branchTitle := func(b wizardbusiness.DetourKind) string {
		title := locale.T("Directions")
		if b == wizardbusiness.DetourKindNode {
			title = locale.T("Nodes")
		}
		return title + " (" + strconv.Itoa(len(children[b])) + ")"
	}

	// Строка: [отступ][иконка] подпись. У ветки отступа нет, иконка —
	// уголок; у листа отступ на ширину уголка, иконка — галка или пусто.
	// «Без detour» стоит листом верхнего уровня, вровень с остальными.
	indentW := float32(detourIconSize) + theme.Padding()
	list := widget.NewList(
		func() int { return len(rows) },
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Truncation = fyne.TextTruncateEllipsis
			_, iconBox := detourSmallIcon(nil)
			left := container.NewHBox(detourSpacer(indentW), iconBox)
			return container.NewBorder(nil, nil, left, nil, label)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			if id < 0 || id >= len(rows) {
				return
			}
			row := rows[id]
			c := obj.(*fyne.Container)
			label := c.Objects[0].(*widget.Label)
			left := c.Objects[1].(*fyne.Container)
			indent := left.Objects[0]
			icon := left.Objects[1].(*fyne.Container).Objects[0].(*fyne.Container).Objects[0].(*widget.Icon)
			if row.branch != wizardbusiness.DetourKindNone {
				indent.Hide()
				if open[row.branch] {
					icon.SetResource(detourChevronDown)
				} else {
					icon.SetResource(detourChevronRight)
				}
				label.TextStyle = fyne.TextStyle{Bold: true}
				label.SetText(branchTitle(row.branch))
				return
			}
			indent.Show()
			if row.opt == p.selected {
				icon.SetResource(detourCheck)
			} else {
				icon.SetResource(nil)
			}
			label.TextStyle = fyne.TextStyle{}
			label.SetText(p.display(row.opt))
		},
	)

	p.hide()
	p.popup = widget.NewPopUp(list, p.win.Canvas())
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(p.btn)
	pos.Y += p.btn.Size().Height

	// Высота — по числу видимых строк, не больше detourTreeMaxRows и не
	// ниже края окна; растёт и сжимается при раскрытии веток.
	pad := theme.Padding()
	rowH := list.CreateItem().MinSize().Height + pad
	resize := func() {
		if p.popup == nil {
			return
		}
		n := len(rows)
		if n > detourTreeMaxRows {
			n = detourTreeMaxRows
		}
		h := float32(n)*rowH + pad
		if room := p.win.Canvas().Size().Height - pos.Y - pad; room > rowH && h > room {
			h = room
		}
		p.popup.Resize(fyne.NewSize(p.btn.Size().Width, h))
	}

	list.OnSelected = func(id widget.ListItemID) {
		// Выделение списка не держим: выбор показывает галка, а повторный
		// клик по той же строке должен срабатывать снова.
		list.Unselect(id)
		if id < 0 || id >= len(rows) {
			return
		}
		row := rows[id]
		if row.branch != wizardbusiness.DetourKindNone {
			open[row.branch] = !open[row.branch]
			rebuild()
			list.Refresh()
			resize()
			return
		}
		p.hide()
		if row.opt == p.selected {
			return
		}
		p.selected = row.opt
		p.label.SetText(p.display(row.opt))
		if p.onChanged != nil {
			p.onChanged(row.opt)
		}
	}

	resize()
	p.popup.ShowAtPosition(pos)
	for i, row := range rows {
		if row.branch == wizardbusiness.DetourKindNone && row.opt == p.selected {
			list.ScrollTo(i)
			break
		}
	}
}

func (p *detourTreePicker) hide() {
	if p.popup != nil {
		p.popup.Hide()
		p.popup = nil
	}
}
