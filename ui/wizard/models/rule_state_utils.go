// Package models содержит модели данных визарда конфигурации.
//
// Файл rule_state_utils.go содержит утилиты для работы с RuleState:
//   - GetEffectiveOutbound - возвращает эффективный outbound для правила (SelectedOutbound или DefaultOutbound из Rule)
//   - EnsureDefaultOutbound - обеспечивает, что правило имеет выбранный outbound (использует DefaultOutbound или первый доступный)
//
// Эти функции работают только с данными RuleState, без зависимостей от GUI,
// что делает их переиспользуемыми в бизнес-логике и тестах.
//
// Утилиты для работы с RuleState - это вспомогательные функции, отдельные от структуры данных.
//
// Используется в:
//   - business/generator.go - GetEffectiveOutbound вызывается при слиянии правил маршрутизации
//   - presentation/presenter_methods.go - EnsureDefaultOutbound вызывается при инициализации правил
package models

// GetEffectiveOutbound возвращает эффективный outbound для правила (SelectedOutbound или DefaultOutbound).
func GetEffectiveOutbound(ruleState *RuleState) string {
	if ruleState.SelectedOutbound != "" {
		return ruleState.SelectedOutbound
	}
	return ruleState.Rule.DefaultOutbound
}

// EnsureDefaultOutbound обеспечивает, что правило имеет выбранный outbound.
func EnsureDefaultOutbound(ruleState *RuleState, availableOutbounds []string) {
	if ruleState.SelectedOutbound == "" {
		if ruleState.Rule.DefaultOutbound != "" {
			ruleState.SelectedOutbound = ruleState.Rule.DefaultOutbound
		} else if len(availableOutbounds) > 0 {
			ruleState.SelectedOutbound = availableOutbounds[0]
		}
	}
}


