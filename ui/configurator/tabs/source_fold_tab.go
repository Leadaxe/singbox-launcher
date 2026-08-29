// File source_fold_tab.go — вкладка «Группа» окна подписки (SPEC 108).
//
// Свёрнутая подписка приезжает в список Направлений одной записью вместо
// всех своих узлов. Чем именно её заменить — выбирается здесь: селектором,
// автогруппой или селектором с автогруппой.
//
// Настройки автогруппы — те же виджеты, что на вкладке «Автовыбор»
// Направления (autogroupform): это одна и та же сущность
// (configtypes.DirectionAuto) на двух уровнях.
package tabs

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/locale"
	"singbox-launcher/ui/configurator/autogroupform"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// foldTab — состояние вкладки «Группа».
type foldTab struct {
	modeSelect *widget.Select
	autoForm   *autogroupform.Form
	autoBlock  *fyne.Container
	tagsLabel  *widget.Label
	content    fyne.CanvasObject

	// modeLabels — подписи режимов в порядке FoldMode*.
	modeLabels []string

	// applying — правка идёт из кода, а не от пользователя. Без этого
	// флага programmatic SetSelected вызвал бы OnChanged, тот — запись в
	// модель и перерисовку, и форма ушла бы в рекурсию (ловушка SPEC 104).
	applying bool
}

// newFoldTab собирает вкладку. onChange вызывается после любой правки
// пользователем — вызывающий сохраняет модель.
func newFoldTab(model *wizardmodels.WizardModel, onChange func()) *foldTab {
	t := &foldTab{
		modeLabels: []string{
			locale.T("Selector (manual pick)"),
			locale.T("Auto-select group (urltest)"),
			locale.T("Selector with an auto-select group"),
		},
	}

	t.modeSelect = widget.NewSelect(t.modeLabels, nil)
	t.autoForm = autogroupform.New(foldAutoChoices(model))

	hint := widget.NewLabel(locale.T("The subscription arrives as one entry instead of every node. Its nodes stay in the config as that group's members."))
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
	t.autoForm.ModeSelect.OnChanged = func(string) {
		if t.applying {
			return
		}
		onChange()
	}
	return t
}

// Load заполняет вкладку из свёртки подписки. tagPrefix/sourceIndex нужны,
// чтобы показать пользователю итоговые теги групп.
func (t *foldTab) Load(fold *configtypes.SourceFold, tagPrefix string, sourceIndex int) {
	t.applying = true
	defer func() { t.applying = false }()

	mode := configtypes.FoldModeSelect
	var auto *configtypes.DirectionAuto
	if fold != nil {
		mode = fold.EffectiveMode()
		auto = fold.Auto
	}
	switch mode {
	case configtypes.FoldModeAuto:
		t.modeSelect.SetSelected(t.modeLabels[1])
	case configtypes.FoldModeSelectAuto:
		t.modeSelect.SetSelected(t.modeLabels[2])
	default:
		t.modeSelect.SetSelected(t.modeLabels[0])
	}
	t.autoForm.Load(auto)
	t.updateTagsHint(mode, tagPrefix, sourceIndex)
	t.syncAutoVisible()
}

// Collect читает вкладку в свёртку.
func (t *foldTab) Collect() *configtypes.SourceFold {
	fold := &configtypes.SourceFold{Mode: t.selectedMode()}
	if fold.HasAuto() {
		fold.Auto = t.autoForm.Collect()
	}
	return fold
}

func (t *foldTab) selectedMode() string {
	switch t.modeSelect.Selected {
	case t.modeLabels[1]:
		return configtypes.FoldModeAuto
	case t.modeLabels[2]:
		return configtypes.FoldModeSelectAuto
	default:
		return configtypes.FoldModeSelect
	}
}

// syncAutoVisible прячет настройки автогруппы у режима «селектор»: там их
// попросту не к чему приложить.
func (t *foldTab) syncAutoVisible() {
	if strings.Contains(t.selectedMode(), configtypes.FoldModeAuto) {
		t.autoBlock.Show()
	} else {
		t.autoBlock.Hide()
	}
}

// updateTagsHint показывает теги, под которыми группы уедут в конфиг: на них
// ссылаются addOutbounds Направлений, и угадывать их пользователь не обязан.
func (t *foldTab) updateTagsHint(mode, tagPrefix string, sourceIndex int) {
	var tags []string
	if mode == configtypes.FoldModeAuto || mode == configtypes.FoldModeSelectAuto {
		tags = append(tags, configtypes.FoldAutoTag(tagPrefix, sourceIndex))
	}
	if mode == configtypes.FoldModeSelect || mode == configtypes.FoldModeSelectAuto {
		tags = append(tags, configtypes.FoldSelectTag(tagPrefix, sourceIndex))
	}
	t.tagsLabel.SetText(fmt.Sprintf(locale.T("Tags: %s"), strings.Join(tags, ", ")))
}

// foldAutoChoices собирает варианты значений полей автогруппы из
// переменных шаблона (`@urltest_*`): пользователь должен иметь возможность
// выбрать «наследовать из Settings», а не только конкретное число.
func foldAutoChoices(model *wizardmodels.WizardModel) autogroupform.Choices {
	var vars []template.TemplateVar
	if model != nil && model.TemplateData != nil {
		vars = model.TemplateData.Vars
	}
	return autogroupform.Choices{
		Interval:  templateVarChoicesForFold(vars, "urltest_interval"),
		Tolerance: templateVarChoicesForFold(vars, "urltest_tolerance"),
		URL:       templateVarChoicesForFold(vars, "urltest_url"),
	}
}

// templateVarChoicesForFold — зеркало outbounds_configurator.templateVarChoices,
// но без презентера: первым вариантом идёт сам placeholder («@urltest_url»),
// то есть «наследовать значение из Settings».
func templateVarChoicesForFold(vars []template.TemplateVar, varName string) autogroupform.VarChoices {
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

// foldTagPrefix — префикс тегов подписки, как его увидит сборка.
//
// SPEC 117: читает canonical state.Source (Tag — *TagSpec; nil = префикса нет).
func foldTagPrefix(p *wizardmodels.Source) string {
	if p == nil || p.TagPolicy == nil {
		return ""
	}
	return p.TagPolicy.Prefix
}

// syncReplaceFromFold — TEMPORARY BRIDGE (SPEC 118 W2-W4), умирает вместе с
// Fold-вкладкой (W5: вкладка Replace правит канон напрямую).
//
// Канонический FolderReplace обязан следовать за правкой свёртки в форме:
// после миграции W2 он заполнен, а мост (legacyFold) предпочитает канон —
// не синхронизируй мы его здесь, правка формы молча игнорировалась бы
// сборкой. Тег материализуется тем же деривативом, что в миграции; смена
// режима меняет теги ровно так же, как меняла в старой позиционной схеме.
func syncReplaceFromFold(p *wizardmodels.Source, sourceIndex int) {
	if p == nil {
		return
	}
	if p.Fold == nil {
		p.Replace = nil
		return
	}
	prefix := foldTagPrefix(p)
	rep := &state.FolderReplace{}
	switch p.Fold.EffectiveMode() {
	case configtypes.FoldModeAuto:
		rep.Mode = state.FolderReplaceAuto
		rep.Tag = configtypes.FoldAutoTag(prefix, sourceIndex)
	case configtypes.FoldModeSelectAuto:
		rep.Mode = state.FolderReplaceBoth
		rep.Tag = configtypes.FoldSelectTag(prefix, sourceIndex)
	default:
		rep.Mode = state.FolderReplaceManual
		rep.Tag = configtypes.FoldSelectTag(prefix, sourceIndex)
	}
	if p.Fold.HasAuto() {
		rep.Strategy = p.Fold.Auto.Clone()
	}
	p.Replace = rep
}
