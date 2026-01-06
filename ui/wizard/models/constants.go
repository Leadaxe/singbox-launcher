// Package models содержит модели данных визарда конфигурации.
//
// Файл constants.go содержит константы, связанные с бизнес-логикой визарда:
//   - DefaultOutboundTag - тег outbound по умолчанию для правил маршрутизации ("direct-out")
//   - RejectActionName - название действия reject для правил маршрутизации ("reject")
//   - RejectActionMethod - метод действия reject ("drop")
//
// Эти константы используются в бизнес-логике работы с правилами маршрутизации
// и outbounds, поэтому они находятся в пакете models рядом с RuleState и WizardModel.
//
// Константы - это отдельная ответственность от структур данных.
//
// Используется в:
//   - business/generator.go - RejectActionName и RejectActionMethod используются при применении outbound к правилам
//   - models/rule_state.go - DefaultOutboundTag может использоваться при работе с правилами
package models

const (
	// DefaultOutboundTag - тег outbound по умолчанию для правил маршрутизации
	DefaultOutboundTag = "direct-out"
	// RejectActionName - название действия reject в правилах маршрутизации
	RejectActionName = "reject"
	// RejectActionMethod - метод действия reject (drop)
	RejectActionMethod = "drop"
)


