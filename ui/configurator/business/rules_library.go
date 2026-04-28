// Package business: rules library — клон пресетов шаблона в custom_rules и миграция state (027).
package business

import (
	"encoding/json"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardtemplate "singbox-launcher/core/template"
)

// CloneTemplateSelectableToRuleState — глубокая копия пресета шаблона в RuleState для custom_rules.
func CloneTemplateSelectableToRuleState(
	tr *wizardtemplate.TemplateSelectableRule,
	enabled bool,
	selectedOutbound string,
	availableOutbounds []string,
) *wizardmodels.RuleState {
	if tr == nil {
		return nil
	}
	raw, err := json.Marshal(tr)
	if err != nil {
		debuglog.DebugLog("rules_library: marshal TemplateSelectableRule: %v", err)
		return nil
	}
	var copyTR wizardtemplate.TemplateSelectableRule
	if err := json.Unmarshal(raw, &copyTR); err != nil {
		debuglog.DebugLog("rules_library: unmarshal TemplateSelectableRule: %v", err)
		return nil
	}
	rs := &wizardmodels.RuleState{
		Rule:             copyTR,
		Enabled:          enabled,
		SelectedOutbound: selectedOutbound,
	}
	wizardmodels.EnsureDefaultOutbound(rs, availableOutbounds)
	return rs
}

// effectivePresetOutbound — DefaultOutbound пресета или первый тег из options.
func effectivePresetOutbound(tr *wizardtemplate.TemplateSelectableRule, options []string) string {
	if tr != nil && tr.DefaultOutbound != "" {
		return tr.DefaultOutbound
	}
	if len(options) > 0 {
		return options[0]
	}
	return ""
}

func disableRuleIfSRSPending(execDir string, rs *wizardmodels.RuleState, tr *wizardtemplate.TemplateSelectableRule) {
	if rs == nil || tr == nil || len(tr.RuleSets) == 0 {
		return
	}
	if !services.AllSRSDownloaded(execDir, tr.RuleSets) {
		rs.Enabled = false
	}
}

// ClonePresetWithSRSGuard — клон пресета для списка custom_rules (засев, библиотека). nil при ошибке клона.
func ClonePresetWithSRSGuard(model *wizardmodels.WizardModel, tr *wizardtemplate.TemplateSelectableRule, enabled bool, options []string) *wizardmodels.RuleState {
	if model == nil || tr == nil {
		return nil
	}
	out := effectivePresetOutbound(tr, options)
	rs := CloneTemplateSelectableToRuleState(tr, enabled, out, options)
	if rs == nil {
		return nil
	}
	disableRuleIfSRSPending(model.ExecDir, rs, tr)
	return rs
}

// AppendClonedPresetsToCustomRules добавляет в конец CustomRules клоны пресетов шаблона, для которых picked[i]==true.
// len(picked) должен совпадать с len(rules); иначе возвращает 0. Возвращает число успешно добавленных правил.
func AppendClonedPresetsToCustomRules(model *wizardmodels.WizardModel, rules []wizardtemplate.TemplateSelectableRule, picked []bool) int {
	if model == nil || len(picked) != len(rules) {
		return 0
	}
	options := EnsureDefaultAvailableOutbounds(GetAvailableOutbounds(model))
	n := 0
	for i := range rules {
		if !picked[i] {
			continue
		}
		tr := &rules[i]
		if rs := ClonePresetWithSRSGuard(model, tr, tr.IsDefault, options); rs != nil {
			model.CustomRules = append(model.CustomRules, rs)
			n++
		}
	}
	return n
}

// ApplyRulesLibraryMigration сливает старый слой selectable_rule_states в custom_rules (один раз).
// Если rules_library_merged уже true — ничего не делает.
// Если selectable_rule_states пуст — только выставляет merged (custom без дублирования блока шаблона).
func ApplyRulesLibraryMigration(sf *wizardmodels.WizardStateFile, td *wizardtemplate.TemplateData, execDir string) {
	if sf == nil || td == nil || sf.RulesLibraryMerged {
		return
	}
	if len(sf.SelectableRuleStates) == 0 {
		sf.RulesLibraryMerged = true
		sf.SelectableRuleStates = nil
		sf.Version = wizardmodels.WizardStateVersion
		return
	}

	savedByLabel := make(map[string]wizardmodels.PersistedSelectableRuleState, len(sf.SelectableRuleStates))
	for _, pr := range sf.SelectableRuleStates {
		savedByLabel[pr.Label] = pr
	}

	merged := make([]wizardmodels.PersistedCustomRule, 0, len(td.SelectableRules)+len(sf.CustomRules))
	for i := range td.SelectableRules {
		tr := &td.SelectableRules[i]
		enabled := tr.IsDefault
		outbound := tr.DefaultOutbound
		if saved, ok := savedByLabel[tr.Label]; ok {
			enabled = saved.Enabled
			outbound = saved.SelectedOutbound
		}
		rs := CloneTemplateSelectableToRuleState(tr, enabled, outbound, nil)
		if rs == nil {
			continue
		}
		disableRuleIfSRSPending(execDir, rs, tr)
		merged = append(merged, wizardmodels.ToPersistedCustomRule(rs))
	}
	merged = append(merged, sf.CustomRules...)
	sf.CustomRules = merged
	sf.SelectableRuleStates = nil
	sf.RulesLibraryMerged = true
	sf.Version = wizardmodels.WizardStateVersion
}

// EnsureCustomRulesDefaultOutbounds заполняет SelectedOutbound из DefaultOutbound пресета или первого доступного тега.
// Вызывать после LoadState, когда миграция клонировала пресеты без списка outbounds.
func EnsureCustomRulesDefaultOutbounds(model *wizardmodels.WizardModel) {
	if model == nil {
		return
	}
	opts := EnsureDefaultAvailableOutbounds(GetAvailableOutbounds(model))
	for _, rs := range model.CustomRules {
		wizardmodels.EnsureDefaultOutbound(rs, opts)
	}
}
