// Package models содержит модели данных визарда конфигурации.
//
// Файл rule_state.go определяет RuleState - модель состояния правила маршрутизации.
//
// RuleState содержит только бизнес-данные правила (без GUI зависимостей):
//   - Rule - правило из шаблона (TemplateSelectableRule) с исходными данными правила
//   - Enabled - включено ли правило (используется ли в финальной конфигурации)
//   - SelectedOutbound - выбранный outbound для правила (может быть "reject", "drop" или имя outbound)
//
// В отличие от старой архитектуры, здесь НЕТ ссылки на виджет (*widget.Select),
// что позволяет использовать RuleState в бизнес-логике без зависимостей от Fyne GUI.
//
// Связь между RuleState и GUI виджетами осуществляется через презентер,
// который хранит виджеты в GUIState и связывает их с RuleState через RuleWidget структуру.
//
// RuleState - это основная модель данных для правил маршрутизации.
// Определение структуры данных отделено от утилит (rule_state_utils.go) и констант (constants.go).
//
// Используется в:
//   - models/wizard_model.go - WizardModel содержит SelectableRuleStates и CustomRules ([]*RuleState)
//   - business/generator.go - MergeRouteSection использует RuleState для слияния правил
//   - presentation/presenter_methods.go - RefreshOutboundOptions обновляет outbounds для RuleState
//   - dialogs/add_rule_dialog.go - работает с RuleState при добавлении/редактировании правил
package models

import (
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// RuleState описывает состояние правила маршрутизации.
type RuleState struct {
	// Rule - правило из шаблона
	Rule wizardtemplate.TemplateSelectableRule
	// Enabled - включено ли правило
	Enabled bool
	// SelectedOutbound - выбранный outbound для правила
	SelectedOutbound string
}
