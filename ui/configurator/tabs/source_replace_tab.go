// File source_replace_tab.go — вкладка «Группа» окна источника: ЗАМЕНА
// (FolderReplace, SPEC 118).
//
// Свёрнутый источник приезжает в список Направлений одной записью вместо
// всех своих узлов. Чем именно его заменить — выбирается здесь: селектором,
// автогруппой или селектором с автогруппой.
//
// SPEC 118 W5: прежняя свёртка (`fold`) с ПОЗИЦИОННЫМ тегом (`3:select`)
// умерла — тег замены теперь явный и живёт в модели (`replace.tag`). Поэтому
// у вкладки появилось поле тега: раньше его вычисляла сборка из номера
// источника в списке, и перестановка источников молча уводила ссылки.
//
// Настройки автогруппы — те же виджеты, что на вкладке «Автовыбор»
// Направления (autogroupform): это одна и та же сущность
// (configtypes.DirectionAuto) на двух уровнях.
package tabs

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/configurator/autogroupform"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// replaceTab — состояние вкладки «Группа».
type replaceTab struct {
	modeSelect *widget.Select
	tagEntry   *widget.Entry
	autoForm   *autogroupform.Form
	autoBlock  *fyne.Container
	tagsLabel  *widget.Label
	content    fyne.CanvasObject

	// modeLabels — подписи режимов в порядке manual / auto / both.
	modeLabels []string

	// applying — правка идёт из кода, а не от пользователя. Без этого
	// флага programmatic SetSelected вызвал бы OnChanged, тот — запись в
	// модель и перерисовку, и форма ушла бы в рекурсию (ловушка SPEC 104).
	applying bool
}

// newReplaceTab собирает вкладку. onChange вызывается после любой правки
// пользователем — вызывающий сохраняет модель.
func newReplaceTab(model *wizardmodels.WizardModel, onChange func()) *replaceTab {
	t := &replaceTab{
		modeLabels: []string{
			locale.T("Selector (manual pick)"),
			locale.T("Auto-select group (urltest)"),
			locale.T("Selector with an auto-select group"),
		},
	}

	t.modeSelect = widget.NewSelect(t.modeLabels, nil)
	t.tagEntry = widget.NewEntry()
	t.tagEntry.SetPlaceHolder(locale.T("Group tag"))
	t.autoForm = autogroupform.New(replaceAutoChoices(model))

	hint := widget.NewLabel(locale.T("The source arrives as one entry instead of every node. Its nodes stay in the config as that group's members."))
	hint.Wrapping = fyne.TextWrapWord

	t.tagsLabel = widget.NewLabel("")
	t.tagsLabel.Wrapping = fyne.TextWrapWord
	t.tagsLabel.Importance = widget.LowImportance

	autoModeLabel := widget.NewLabel(locale.T("Auto-select mode"))
	t.autoBlock = container.NewVBox(
		widget.NewSeparator(),
		t.autoForm.Content(nil, autoModeLabel),
	)

	t.content = container.NewVBox(
		hint,
		autogroupform.TextRow(locale.T("Fold into"), t.modeSelect),
		autogroupform.TextRow(locale.T("Tag"), t.tagEntry),
		t.tagsLabel,
		t.autoBlock,
	)

	// Обработчики — ПОСЛЕ установки значений (см. applying).
	t.modeSelect.OnChanged = func(string) {
		if t.applying {
			return
		}
		t.syncAutoVisible()
		onChange()
	}
	t.tagEntry.OnChanged = func(string) {
		if t.applying {
			return
		}
		onChange()
	}
	t.autoForm.ModeSelect.OnChanged = func(string) {
		if t.applying {
			return
		}
		onChange()
	}
	return t
}

// Load заполняет вкладку из замены источника. defaultTag подставляется, если
// тега ещё нет (новая свёртка): пустой тег в конфиге валит `sing-box check`.
func (t *replaceTab) Load(rep *state.FolderReplace, defaultTag string) {
	t.applying = true
	defer func() { t.applying = false }()

	mode := state.FolderReplaceManual
	var auto *state.AutoStrategy
	tag := defaultTag
	if rep != nil {
		if rep.Mode != "" {
			mode = rep.Mode
		}
		auto = rep.Strategy
		if strings.TrimSpace(rep.Tag) != "" {
			tag = rep.Tag
		}
	}
	switch mode {
	case state.FolderReplaceAuto:
		t.modeSelect.SetSelected(t.modeLabels[1])
	case state.FolderReplaceBoth:
		t.modeSelect.SetSelected(t.modeLabels[2])
	default:
		t.modeSelect.SetSelected(t.modeLabels[0])
	}
	t.tagEntry.SetText(tag)
	t.autoForm.Load(auto)
	t.updateTagsHint()
	t.syncAutoVisible()
}

// Collect читает вкладку в замену. defaultTag — запасной тег на случай, когда
// пользователь стёр поле: замена без тега не эмитится вовсе.
func (t *replaceTab) Collect(defaultTag string) *state.FolderReplace {
	rep := &state.FolderReplace{Mode: t.selectedMode(), Tag: strings.TrimSpace(t.tagEntry.Text)}
	if rep.Tag == "" {
		rep.Tag = defaultTag
	}
	if rep.Mode != state.FolderReplaceManual {
		rep.Strategy = t.autoForm.Collect()
	}
	return rep
}

func (t *replaceTab) selectedMode() string {
	switch t.modeSelect.Selected {
	case t.modeLabels[1]:
		return state.FolderReplaceAuto
	case t.modeLabels[2]:
		return state.FolderReplaceBoth
	default:
		return state.FolderReplaceManual
	}
}

// syncAutoVisible прячет настройки автогруппы у режима «селектор»: там их
// попросту не к чему приложить.
func (t *replaceTab) syncAutoVisible() {
	if t.selectedMode() == state.FolderReplaceManual {
		t.autoBlock.Hide()
	} else {
		t.autoBlock.Show()
	}
}

// updateTagsHint показывает теги, под которыми группы уедут в конфиг: на них
// ссылаются addOutbounds Направлений, и угадывать их пользователь не обязан.
//
// Двойник режима both носит производный тег `<tag>-auto` — той же формулой,
// что твины Направлений (на совпадении формулы держится узнавание пар).
func (t *replaceTab) updateTagsHint() {
	tag := strings.TrimSpace(t.tagEntry.Text)
	if tag == "" {
		t.tagsLabel.SetText("")
		return
	}
	var tags []string
	switch t.selectedMode() {
	case state.FolderReplaceAuto:
		tags = []string{tag}
	case state.FolderReplaceBoth:
		tags = []string{tag + "-auto", tag}
	default:
		tags = []string{tag}
	}
	t.tagsLabel.SetText(locale.Tf("Tags: %s", strings.Join(tags, ", ")))
}

// defaultReplaceTag — тег замены по умолчанию для источника: его префикс
// тегов плюс `select`, а при пустом префиксе — позиционный `<номер>:`.
//
// Формула та же, что материализовала миграция v6→v7: у пользователя,
// пришедшего со свёрткой, тег остаётся прежним, и его правила не теряют цель.
func defaultReplaceTag(p *wizardmodels.Source, sourceIndex int) string {
	prefix := ""
	if p != nil && p.TagPolicy != nil {
		prefix = strings.TrimSpace(p.TagPolicy.Prefix)
	}
	if prefix == "" {
		prefix = strconv.Itoa(sourceIndex+1) + ":"
	}
	return prefix + "select"
}

// replaceAutoChoices собирает варианты значений полей автогруппы из
// переменных шаблона (`@urltest_*`): пользователь должен иметь возможность
// выбрать «наследовать из Settings», а не только конкретное число.
func replaceAutoChoices(model *wizardmodels.WizardModel) autogroupform.Choices {
	var vars []template.TemplateVar
	if model != nil && model.TemplateData != nil {
		vars = model.TemplateData.Vars
	}
	return autogroupform.Choices{
		Interval:  templateVarChoicesForReplace(vars, "urltest_interval"),
		Tolerance: templateVarChoicesForReplace(vars, "urltest_tolerance"),
		URL:       templateVarChoicesForReplace(vars, "urltest_url"),
	}
}

// templateVarChoicesForReplace — зеркало outbounds_configurator.templateVarChoices,
// но без презентера: первым вариантом идёт сам placeholder («@urltest_url»),
// то есть «наследовать значение из Settings».
func templateVarChoicesForReplace(vars []template.TemplateVar, varName string) autogroupform.VarChoices {
	placeholder := "@" + varName
	out := autogroupform.VarChoices{
		Labels:       []string{placeholder},
		LabelToValue: map[string]string{placeholder: placeholder},
	}
	for _, v := range vars {
		if v.Name != varName {
			continue
		}
		for i, opt := range v.Options {
			label := opt
			if i < len(v.OptionTitles) && v.OptionTitles[i] != "" {
				label = v.OptionTitles[i]
			}
			out.Labels = append(out.Labels, label)
			out.LabelToValue[label] = opt
		}
		break
	}
	return out
}
