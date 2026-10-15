// File preset_nodes_view.go — узлы для пресетов с обходом узлов в визарде:
// `for_each_node` и `for_each_node_do` (LxBox §578, SPEC 145 §7, SPEC 166).
//
// Сборка берёт список функцией build.CollectPresetNodes из кэша узлов после
// граф-санитайзера. Визард строит его ТОЙ ЖЕ функцией из кэша последней
// эмиссии (model.GeneratedOutbounds/.GeneratedEndpoints + GeneratedRecordFlags):
// выключенный узел эмиссия не выпускает, поля записи несёт карта, у узла
// подписки их нет. Отбор пресетом (#filter) — build.PresetForEachNodes,
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

// PresetNodesForView — узлы конфига для раскрытия обхода узлов в визарде, в
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
// `for_each_node` тело повторяется по узлам PresetNodesForView, у
// `for_each_node_do` раскрыто один раз с собранными по узлам значениями (одно
// правило с массивом); остальные раскрываются как прежде (узлы не читаются и
// не собираются).
func ExpandPresetForView(model *wizardmodels.WizardModel, tpl *wizardtemplate.Preset, vars map[string]string) (*build.PresetFragments, []build.ExpandWarning, bool) {
	var nodes []wizardtemplate.PresetNode
	if tpl.IsNodeLoop() {
		nodes = PresetNodesForView(model)
	}
	return build.ExpandPresetForNodes(tpl, vars, PresetGlobalVars(model), PresetGlobalDecls(model), model.Target, nodes)
}

// PresetServedTags — теги узлов, которые обслуживает пресет с обходом узлов
// (оба ключа: узлы, прошедшие #filter), в порядке конфига. Пресет без обхода —
// nil: подпись строке не нужна.
func PresetServedTags(model *wizardmodels.WizardModel, tpl *wizardtemplate.Preset, vars map[string]string) []string {
	if model == nil || !tpl.IsNodeLoop() {
		return nil
	}
	nodes := build.PresetForEachNodes(tpl, vars, PresetGlobalVars(model), model.Target, PresetNodesForView(model))
	tags := make([]string, 0, len(nodes))
	for _, n := range nodes {
		tags = append(tags, n.Tag)
	}
	return tags
}

// PresetServedNodesLabel — подпись строки пресета с обходом узлов: теги через
// запятую; подходящих узлов нет — короткая подпись.
func PresetServedNodesLabel(tags []string) string {
	if len(tags) == 0 {
		return locale.T("No matching nodes")
	}
	return strings.Join(tags, ", ")
}
