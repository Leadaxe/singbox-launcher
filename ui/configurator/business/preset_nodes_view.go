// File preset_nodes_view.go — узлы для пресетов с `for_each` в визарде
// (LxBox §578, SPEC 145 §7).
//
// Сборка берёт список функцией build.CollectPresetNodes из кэша узлов после
// граф-санитайзера. Визард строит его ТОЙ ЖЕ функцией из кэша последней
// эмиссии (model.GeneratedOutbounds/.GeneratedEndpoints + GeneratedSkipPresets):
// выключенный узел эмиссия не выпускает, skip_presets несёт карта, у узла
// подписки поля нет. Отбор пресетом (node_type, filter) — build.PresetForEachNodes,
// тот же, что у раскрытия в сборке.
//
// Допущение: до сборки готового конфига нет. Узел, который сборка потом
// снимет гейтом реестра или граф-санитайзером, визард ещё назовёт.
package business

import (
	"strings"

	"singbox-launcher/core/build"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// PresetNodesForView — узлы конфига для раскрытия `for_each` в визарде, в
// порядке секций шаблона. До первой эмиссии список пуст.
func PresetNodesForView(model *wizardmodels.WizardModel) []wizardtemplate.PresetNode {
	if model == nil {
		return nil
	}
	var order []string
	if model.TemplateData != nil {
		order = model.TemplateData.ConfigOrder
	}
	return build.CollectPresetNodes(inMemoryCacheFromModel(model), order)
}

// ExpandPresetForView — раскрытие пресета для экранов визарда: у пресета с
// `for_each` тело повторяется по узлам PresetNodesForView, остальные
// раскрываются как прежде (узлы не читаются и не собираются).
func ExpandPresetForView(model *wizardmodels.WizardModel, tpl *wizardtemplate.Preset, vars map[string]string) (*build.PresetFragments, []build.ExpandWarning, bool) {
	var nodes []wizardtemplate.PresetNode
	if tpl != nil && tpl.ForEach != nil {
		nodes = PresetNodesForView(model)
	}
	return build.ExpandPresetForNodes(tpl, vars, PresetGlobalVars(model), PresetGlobalDecls(model), model.Target, nodes)
}

// PresetServedTags — теги узлов, которые обслуживает пресет с `for_each`, в
// порядке конфига. Пресет без for_each — nil: подпись строке не нужна.
func PresetServedTags(model *wizardmodels.WizardModel, tpl *wizardtemplate.Preset, vars map[string]string) []string {
	if model == nil || tpl == nil || tpl.ForEach == nil {
		return nil
	}
	nodes := build.PresetForEachNodes(tpl, vars, PresetGlobalVars(model), model.Target, PresetNodesForView(model))
	tags := make([]string, 0, len(nodes))
	for _, n := range nodes {
		tags = append(tags, n.Tag)
	}
	return tags
}

// PresetServedNodesLabel — подпись строки пресета с `for_each`: теги через
// запятую; подходящих узлов нет — короткая подпись.
func PresetServedNodesLabel(tags []string) string {
	if len(tags) == 0 {
		return locale.T("No matching nodes")
	}
	return strings.Join(tags, ", ")
}
