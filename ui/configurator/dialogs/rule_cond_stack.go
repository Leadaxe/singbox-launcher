package dialogs

// rule_cond_stack.go — стопка условий правила (SPEC 188 §6): рамки условий и
// групп правил, между ними «AND», под стопкой «+ Condition» с меню условий из
// таблицы реестра. Уже добавленное условие из меню пропадает; «Rule group»
// стоит за чертой и добавляется сколько угодно раз. Тот же код рисует
// правило внутри группы (rule_cond_group.go) — рекурсия без предела.

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// condStackEditor — редактор одной стопки.
type condStackEditor struct {
	ctx    *ruleFormCtx
	frames []*condFrame
	groups []*groupEditor
	// extra и not — то, что форма провозит без полей (wizardmodels.RuleCondStack).
	extra  map[string]interface{}
	not    bool
	items  *fyne.Container
	addBtn *widget.Button
	box    *fyne.Container
}

func newCondStackEditor(ctx *ruleFormCtx, s wizardmodels.RuleCondStack) *condStackEditor {
	e := &condStackEditor{ctx: ctx, extra: s.Extra, not: s.Not, items: container.NewVBox()}
	for _, c := range s.Conds {
		cond, ok := ctx.table.Cond(c.ID)
		if !ok {
			continue
		}
		e.frames = append(e.frames, e.newFrame(cond, c.Values))
	}
	for _, g := range s.Groups {
		e.groups = append(e.groups, e.newGroup(g))
	}
	e.addBtn = widget.NewButton(locale.T("+ Condition"), e.showMenu)
	e.box = container.NewVBox(e.items, container.NewHBox(e.addBtn))
	e.relayout()
	return e
}

func (e *condStackEditor) object() fyne.CanvasObject { return e.box }

func (e *condStackEditor) newFrame(cond registryCond, values map[string]interface{}) *condFrame {
	var f *condFrame
	f = newCondFrame(e.ctx, cond, values, func() {
		c, err := f.read(false)
		e.ctx.confirmRemove(wizardmodels.RuleCondStack{Conds: []wizardmodels.RuleCond{c}}, err,
			locale.T("Remove this condition?"), func() { e.removeFrame(f) })
	})
	return f
}

func (e *condStackEditor) newGroup(g wizardmodels.RuleGroup) *groupEditor {
	var ge *groupEditor
	ge = newGroupEditor(e.ctx, g, func() {
		rg, err := ge.read(false)
		e.ctx.confirmRemove(wizardmodels.RuleCondStack{Groups: []wizardmodels.RuleGroup{rg}}, err,
			locale.T("Remove this rule group and everything in it?"), func() { e.removeGroup(ge) })
	})
	return ge
}

// relayout — рамки условий, затем группы; между соседними — «AND».
func (e *condStackEditor) relayout() {
	e.items.Objects = nil
	add := func(obj fyne.CanvasObject) {
		if len(e.items.Objects) > 0 {
			e.items.Add(ruleJoinLabel(locale.T("AND")))
		}
		e.items.Add(obj)
	}
	for _, f := range e.frames {
		add(f.obj)
	}
	for _, g := range e.groups {
		add(g.obj)
	}
	e.items.Refresh()
	e.box.Refresh()
}

// ruleJoinLabel — подпись связки между рамками («AND», «OR»).
func ruleJoinLabel(text string) fyne.CanvasObject {
	l := widget.NewLabel(text)
	l.Importance = widget.LowImportance
	l.TextStyle = fyne.TextStyle{Bold: true}
	return container.NewCenter(l)
}

func (e *condStackEditor) removeFrame(f *condFrame) {
	for i, x := range e.frames {
		if x == f {
			e.frames = append(e.frames[:i], e.frames[i+1:]...)
			break
		}
	}
	e.relayout()
	e.ctx.changed()
}

func (e *condStackEditor) removeGroup(g *groupEditor) {
	for i, x := range e.groups {
		if x == g {
			e.groups = append(e.groups[:i], e.groups[i+1:]...)
			break
		}
	}
	e.relayout()
	e.ctx.changed()
}

// showMenu — меню «+ Condition»: условия формы, которых ещё нет в стопке, и
// за чертой «Rule group». Меню — штатный PopUpMenu Fyne: снимается сам по
// выбору пункта или клику мимо.
func (e *condStackEditor) showMenu() {
	have := map[string]bool{}
	for _, f := range e.frames {
		have[f.cond.ID] = true
	}
	var items []*fyne.MenuItem
	for _, c := range e.ctx.table.Conds {
		if have[c.ID] {
			continue
		}
		cond := c
		items = append(items, fyne.NewMenuItem(locale.T(cond.Label), func() {
			e.frames = append(e.frames, e.newFrame(cond, nil))
			e.relayout()
			e.ctx.changed()
		}))
	}
	if len(items) > 0 {
		items = append(items, fyne.NewMenuItemSeparator())
	}
	items = append(items, fyne.NewMenuItem(locale.T("Rule group"), func() {
		e.groups = append(e.groups, e.newGroup(wizardmodels.RuleGroup{}))
		e.relayout()
		e.ctx.changed()
	}))
	drv := fyne.CurrentApp().Driver()
	c := drv.CanvasForObject(e.addBtn)
	if c == nil {
		return
	}
	pos := drv.AbsolutePositionForObject(e.addBtn).Add(fyne.NewPos(0, e.addBtn.Size().Height))
	widget.ShowPopUpMenuAtPosition(fyne.NewMenu("", items...), c, pos)
}

// read — стопка из рамок; strict — с проверкой полей (Save).
func (e *condStackEditor) read(strict bool) (wizardmodels.RuleCondStack, error) {
	s := wizardmodels.RuleCondStack{Extra: e.extra, Not: e.not}
	for _, f := range e.frames {
		c, err := f.read(strict)
		if err != nil {
			return s, err
		}
		s.Conds = append(s.Conds, c)
	}
	for _, g := range e.groups {
		rg, err := g.read(strict)
		if err != nil {
			return s, err
		}
		s.Groups = append(s.Groups, rg)
	}
	return s, nil
}
