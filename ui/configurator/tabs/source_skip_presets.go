package tabs

import (
	"encoding/json"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/build"
	wizardtemplate "singbox-launcher/core/template"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"

	"singbox-launcher/internal/locale"
)

// skipPresetsText / skipPresetsHint — переключатель поля записи skip_presets
// (LxBox §578, контракт 1.1.86).
const (
	skipPresetsText = "Skip presets"
	skipPresetsHint = "Presets will not add routing or DNS rules for this node."
)

// skipPresetsBlock — переключатель «Skip presets» формы узла-сервера.
//
// Виден у своего сервера и члена папки (у узла подписки записи нет), когда в
// шаблоне есть пресет с обходом узлов (for_each_node / for_each_node_do),
// который узел обслуживает и которого переключатель его лишает: #filter
// пропускает узел без skip_presets и отсекает с ним. Пресет, чей #filter
// skip_presets не читает (сбор имён серверов всех узлов), переключатель не
// показывает — выключение ничего бы не изменило. nil — показывать нечего.
// Переключатель правит рабочую копию узла; в модель она уходит по Save окна
// целиком.
func skipPresetsBlock(m *wizardmodels.WizardModel, link wizardmodels.NodeLink, node *wizardmodels.Node) fyne.CanvasObject {
	if m == nil || m.TemplateData == nil || node == nil || node.Kind != wizardmodels.SourceKindServer {
		return nil
	}
	if fid := strings.TrimSpace(link.FolderID); fid != "" {
		for i := range m.Sources {
			if m.Sources[i].ID == fid && m.Sources[i].Kind == wizardmodels.SourceKindSubscription {
				return nil
			}
		}
	}
	// Пресет обслуживает узел ⇔ узел проходит #filter его обхода (SPEC 166):
	// та же функция отбора, что у сборки, с переменными пресета из правила.
	// Проба дважды: без skip_presets (иначе включённый переключатель прятал
	// бы сам себя) и с ним — переключатель нужен, только если он узел снимает.
	var body map[string]interface{}
	if err := json.Unmarshal(node.Body, &body); err != nil || len(body) == 0 {
		return nil
	}
	tag, _ := body["tag"].(string)
	probe := []wizardtemplate.PresetNode{{Tag: tag, Body: body}}
	skipped := []wizardtemplate.PresetNode{{Tag: tag, Body: body, SkipPresets: true}}
	globals := wizardbusiness.PresetGlobalVars(m)
	served := false
	for i := range m.TemplateData.Presets {
		p := &m.TemplateData.Presets[i]
		if !p.IsNodeLoop() {
			continue
		}
		vars := presetRefVars(m, p.ID)
		if len(build.PresetForEachNodes(p, vars, globals, m.Target, probe)) > 0 &&
			len(build.PresetForEachNodes(p, vars, globals, m.Target, skipped)) == 0 {
			served = true
			break
		}
	}
	if !served {
		return nil
	}
	check := widget.NewCheck(locale.T(skipPresetsText), func(on bool) {
		node.SkipPresets = on
	})
	check.SetChecked(node.SkipPresets)
	hint := widget.NewLabel(locale.T(skipPresetsHint))
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance
	return container.NewVBox(check, hint)
}

// presetRefVars — значения переменных пресета из его правила в модели; правила
// нет — nil (раскрытие возьмёт умолчания шаблона).
func presetRefVars(m *wizardmodels.WizardModel, ref string) map[string]string {
	for _, pr := range m.PresetRefs {
		if pr != nil && pr.Ref == ref {
			return pr.Vars
		}
	}
	return nil
}
