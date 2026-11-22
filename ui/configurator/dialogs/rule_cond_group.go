package dialogs

// rule_cond_group.go — рамка «Rule group» (SPEC 188 §6): «all of / any of»,
// «Not», список правил и «+ Rule». Каждое правило группы — та же стопка
// условий (condStackEditor) со своим «+ Condition»: рекурсия без предела.
// Хранение — составное правило `logical` (SPEC §3).

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config/registry"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// registryCond — условие таблицы реестра (короткое имя для сигнатур окна).
type registryCond = registry.RuleCondition

// groupEditor — редактор группы правил.
type groupEditor struct {
	ctx      *ruleFormCtx
	mode     *widget.RadioGroup
	not      *widget.Check
	rules    []*condStackEditor
	rulesBox *fyne.Container
	obj      fyne.CanvasObject
}

func newGroupEditor(ctx *ruleFormCtx, g wizardmodels.RuleGroup, onRemove func()) *groupEditor {
	ge := &groupEditor{ctx: ctx, rulesBox: container.NewVBox()}
	allOf, anyOf := locale.T("all of"), locale.T("any of")
	ge.mode = widget.NewRadioGroup([]string{allOf, anyOf}, nil)
	ge.mode.Horizontal = true
	ge.mode.Required = true
	if g.Any {
		ge.mode.SetSelected(anyOf)
	} else {
		ge.mode.SetSelected(allOf)
	}
	ge.mode.OnChanged = func(string) {
		ge.relayout()
		ctx.changed()
	}
	ge.not = widget.NewCheck(locale.T("Not"), nil)
	ge.not.SetChecked(g.Not)
	ge.not.OnChanged = func(bool) { ctx.changed() }

	for _, r := range g.Rules {
		ge.rules = append(ge.rules, newCondStackEditor(ctx, r))
	}
	if len(ge.rules) == 0 {
		ge.rules = append(ge.rules, newCondStackEditor(ctx, wizardmodels.RuleCondStack{}))
	}

	title := widget.NewLabel(locale.T("Rule group"))
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Truncation = fyne.TextTruncateEllipsis
	remove := widget.NewButtonWithIcon("", theme.CancelIcon(), onRemove)
	remove.Importance = widget.LowImportance
	addRule := widget.NewButton(locale.T("+ Rule"), func() {
		ge.rules = append(ge.rules, newCondStackEditor(ctx, wizardmodels.RuleCondStack{}))
		ge.relayout()
		ctx.changed()
	})
	body := container.NewVBox(
		container.NewBorder(nil, nil, nil, remove, title),
		container.NewHBox(ge.mode, ge.not),
		ge.rulesBox,
		container.NewHBox(addRule),
	)
	ge.obj = fynewidget.NewFramedPanel(body)
	ge.relayout()
	return ge
}

func (ge *groupEditor) anyOf() bool { return ge.mode.Selected == locale.T("any of") }

// relayout — правила группы рамками; между ними связка режима группы.
func (ge *groupEditor) relayout() {
	join := locale.T("AND")
	if ge.anyOf() {
		join = locale.T("OR")
	}
	ge.rulesBox.Objects = nil
	for i, r := range ge.rules {
		if i > 0 {
			ge.rulesBox.Add(ruleJoinLabel(join))
		}
		rule := r
		remove := widget.NewButtonWithIcon("", theme.CancelIcon(), func() { ge.removeRule(rule) })
		remove.Importance = widget.LowImportance
		title := widget.NewLabel(locale.T("Rule"))
		title.Importance = widget.LowImportance
		title.Truncation = fyne.TextTruncateEllipsis
		ge.rulesBox.Add(fynewidget.NewFramedPanel(container.NewVBox(
			container.NewBorder(nil, nil, nil, remove, title),
			rule.object(),
		)))
	}
	ge.rulesBox.Refresh()
}

func (ge *groupEditor) removeRule(r *condStackEditor) {
	for i, x := range ge.rules {
		if x == r {
			ge.rules = append(ge.rules[:i], ge.rules[i+1:]...)
			break
		}
	}
	ge.relayout()
	ge.ctx.changed()
}

func (ge *groupEditor) read(strict bool) (wizardmodels.RuleGroup, error) {
	g := wizardmodels.RuleGroup{Any: ge.anyOf(), Not: ge.not.Checked}
	for _, r := range ge.rules {
		s, err := r.read(strict)
		if err != nil {
			return g, err
		}
		g.Rules = append(g.Rules, s)
	}
	return g, nil
}
