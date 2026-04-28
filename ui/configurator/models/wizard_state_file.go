// Package models — модель данных визарда конфигурации.
//
// SPEC 052 phase 7 cleanup: вся on-disk-схема state.json живёт в
// core/state (v5). Wizard'овские типы — алиасы на core/state, чтобы
// один формат на проекте, без custom MarshalJSON/UnmarshalJSON.
//
// Старые имена (WizardStateFile, PersistedCustomRule, …) сохранены
// как алиасы — это снижает шум в callsite'ах wizard'а; функционально
// всё через core/state.
package models

import (
	"fmt"
	"regexp"
	"time"

	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/constants"
)

// WizardStateFile — алиас на корневую модель core/state.
//
// Wizard работает с этим типом в RAM (поля parser_config / connections /
// custom_rules / vars / dns_options); сохранение на диск всегда v5
// через corestate.Save.
type WizardStateFile = corestate.State

// Алиасы на типы корневой модели — чтобы wizardmodels.X-обращения
// сохранили знакомую сигнатуру.
type (
	ConfigParam                  = corestate.ConfigParam
	PersistedSettingVar          = corestate.SettingVar
	PersistedSelectableRuleState = corestate.SelectableRuleState
	PersistedCustomRule          = corestate.CustomRule
	PersistedDNSState            = corestate.DNSOptions

	// SPEC 052 phase 7: v5-источники в wizard model.
	Source             = corestate.Source
	SourceType         = corestate.SourceType
	SubscriptionMeta   = corestate.SubscriptionMeta
	UserInfo           = corestate.UserInfo
	ConnectionsSection = corestate.ConnectionsSection
	TagSpec            = corestate.TagSpec
	UpdateSpec         = corestate.UpdateSpec
	Defaults           = corestate.Defaults
)

// Re-export of v5 SourceType constants for UI.
const (
	SourceTypeSubscription = corestate.SourceTypeSubscription
	SourceTypeServer       = corestate.SourceTypeServer
)

// WizardStateVersion — для callsite'ов которые используют это для
// инициализации; теперь равно core/state.SchemaVersion (v5).
const WizardStateVersion = corestate.SchemaVersion

// MaxStateIDLength — максимальная длина ID состояния.
const MaxStateIDLength = 50

// StateFileName — имя файла текущего состояния.
const StateFileName = constants.WizardStateFileName

// stateIDRegex — допустимые символы для ID состояния.
var stateIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// WizardStateMetadata — метаданные состояния для списка.
type WizardStateMetadata struct {
	ID        string    `json:"id"`
	Comment   string    `json:"comment,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	IsCurrent bool      `json:"is_current"`
}

// ValidateStateID проверяет валидность ID состояния.
func ValidateStateID(id string) error {
	if id == "" {
		return fmt.Errorf("state ID cannot be empty")
	}
	if len(id) > MaxStateIDLength {
		return fmt.Errorf("state ID exceeds maximum length of %d characters", MaxStateIDLength)
	}
	if !stateIDRegex.MatchString(id) {
		return fmt.Errorf("state ID can only contain letters (a-z, A-Z), numbers (0-9), hyphen (-), and underscore (_)")
	}
	return nil
}

// PersistedCustomRuleToRuleState — конвертер core/state CustomRule → wizard RuleState.
//
// (раньше был метод (*PersistedCustomRule).ToRuleState; после алиаса на
// corestate.CustomRule методы больше не привязать — переносим в free-function.)
func PersistedCustomRuleToRuleState(pcr *PersistedCustomRule) *RuleState {
	if pcr == nil {
		return nil
	}
	rule := pcr.Rule
	if rule == nil {
		rule = make(map[string]interface{})
	}
	ruleType := pcr.Type
	if !isKnownRuleType(ruleType) {
		ruleType = DetermineRuleType(rule)
	}
	tsr := wizardtemplate.TemplateSelectableRule{
		Label:           pcr.Label,
		Description:     pcr.Description,
		Rule:            rule,
		DefaultOutbound: pcr.DefaultOutbound,
		HasOutbound:     pcr.HasOutbound,
		Params:          pcr.Params,
	}
	if ruleType == RuleTypeSRS && len(pcr.RuleSet) > 0 {
		tsr.RuleSets = pcr.RuleSet
	}
	return &RuleState{
		Rule:             tsr,
		Enabled:          pcr.Enabled,
		SelectedOutbound: pcr.SelectedOutbound,
	}
}

// ToPersistedSelectableRuleState — конвертер RuleState (UI) → core/state форма.
func ToPersistedSelectableRuleState(ruleState *RuleState) PersistedSelectableRuleState {
	return PersistedSelectableRuleState{
		Label:            ruleState.Rule.Label,
		Enabled:          ruleState.Enabled,
		SelectedOutbound: ruleState.SelectedOutbound,
	}
}

// ToPersistedCustomRule — конвертер RuleState (UI) → core/state форма.
func ToPersistedCustomRule(ruleState *RuleState) PersistedCustomRule {
	ruleType := DetermineRuleType(ruleState.Rule.Rule)
	p := PersistedCustomRule{
		Label:            ruleState.Rule.Label,
		Type:             ruleType,
		Enabled:          ruleState.Enabled,
		SelectedOutbound: ruleState.SelectedOutbound,
		Description:      ruleState.Rule.Description,
		Rule:             ruleState.Rule.Rule,
		DefaultOutbound:  ruleState.Rule.DefaultOutbound,
		HasOutbound:      ruleState.Rule.HasOutbound,
	}
	if len(ruleState.Rule.Params) > 0 {
		p.Params = make(map[string]interface{}, len(ruleState.Rule.Params))
		for k, v := range ruleState.Rule.Params {
			p.Params[k] = v
		}
	}
	if ruleType == RuleTypeSRS && len(ruleState.Rule.RuleSets) > 0 {
		p.RuleSet = ruleState.Rule.RuleSets
	}
	return p
}

// Известные константы типов правил (мост на corestate.RuleType*).
const (
	RuleTypeIPS       = corestate.RuleTypeIPS
	RuleTypeURLs      = corestate.RuleTypeURLs
	RuleTypeProcesses = corestate.RuleTypeProcesses
	RuleTypeSRS       = corestate.RuleTypeSRS
	RuleTypeRaw       = corestate.RuleTypeRaw
)

func isKnownRuleType(s string) bool { return corestate.IsKnownRuleType(s) }

// DetermineRuleType определяет тип правила по содержимому rule.
func DetermineRuleType(rule map[string]interface{}) string {
	if rule == nil {
		return RuleTypeRaw
	}
	hasIP := hasKey(rule, "ip_cidr")
	hasDomain := hasKey(rule, "domain") || hasKey(rule, "domain_suffix") || hasKey(rule, "domain_keyword") || hasKey(rule, "domain_regex")
	hasProcess := hasKey(rule, "process_name") || hasKey(rule, "process_path_regex")
	hasRuleSet := hasKey(rule, "rule_set")
	if hasIP && !hasDomain && !hasProcess && !hasRuleSet {
		return RuleTypeIPS
	}
	if hasDomain && !hasIP && !hasProcess && !hasRuleSet {
		return RuleTypeURLs
	}
	if hasProcess && !hasIP && !hasDomain && !hasRuleSet {
		return RuleTypeProcesses
	}
	if hasRuleSet && !hasIP && !hasDomain && !hasProcess {
		return RuleTypeSRS
	}
	return RuleTypeRaw
}

func hasKey(m map[string]interface{}, key string) bool {
	_, ok := m[key]
	return ok
}
