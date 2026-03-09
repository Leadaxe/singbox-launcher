// Package dialogs
//
// rule_type_selection.go — микро-модель выбора типа правила в диалоге Add/Edit Rule.
// Один источник истины: при изменении типа вызывается OnChange, диалог синхронизирует чекбоксы
// в одном месте (с guard от реентрантности).
package dialogs

// RuleTypeSelection хранит выбранный тип правила и уведомляет об изменении.
// Гарантирует один выбранный тип; при снятии галочки вызывающий код задаёт fallback через SetType.
type RuleTypeSelection struct {
	typ      string
	onChange func(string)
}

// NewRuleTypeSelection создаёт модель с начальным типом.
func NewRuleTypeSelection(initialType string) *RuleTypeSelection {
	return &RuleTypeSelection{typ: initialType}
}

// Type возвращает текущий выбранный тип.
func (r *RuleTypeSelection) Type() string {
	return r.typ
}

// SetType задаёт тип и вызывает OnChange только если тип реально изменился.
// Диалог в OnChange синхронизирует чекбоксы (с guard), обновляет видимость и кнопку.
func (r *RuleTypeSelection) SetType(s string) {
	if r.typ == s {
		return
	}
	r.typ = s
	if r.onChange != nil {
		r.onChange(s)
	}
}

// SetOnChange задаёт callback для синхронизации UI при смене типа.
func (r *RuleTypeSelection) SetOnChange(f func(string)) {
	r.onChange = f
}
