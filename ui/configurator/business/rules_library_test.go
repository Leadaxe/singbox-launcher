package business

import (
	"testing"

	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardtemplate "singbox-launcher/core/template"
)

func TestCloneTemplateSelectableToRuleState(t *testing.T) {
	tr := &wizardtemplate.TemplateSelectableRule{
		Label:           "L",
		Description:     "D",
		Rule:            map[string]interface{}{"domain": []string{"x.test"}},
		HasOutbound:     true,
		DefaultOutbound: "direct-out",
	}
	rs := CloneTemplateSelectableToRuleState(tr, true, "proxy-out", []string{"direct-out", "proxy-out"})
	if rs == nil {
		t.Fatal("nil")
	}
	if rs.Rule.Label != "L" {
		t.Fatalf("label %q", rs.Rule.Label)
	}
	if rs.SelectedOutbound != "proxy-out" {
		t.Fatalf("outbound %q", rs.SelectedOutbound)
	}
	// mutating clone must not affect template
	rs.Rule.Label = "M"
	if tr.Label != "L" {
		t.Fatal("template mutated")
	}
}

func TestApplyRulesLibraryMigration_IdempotentEmptySelectable(t *testing.T) {
	td := &wizardtemplate.TemplateData{
		SelectableRules: []wizardtemplate.TemplateSelectableRule{
			{Label: "A", Rule: map[string]interface{}{"domain": []string{"a.test"}}},
		},
	}
	sf := &wizardmodels.WizardStateFile{
		Version:              2,
		RulesLibraryMerged:   false,
		SelectableRuleStates: nil,
		CustomRules: []wizardmodels.PersistedCustomRule{
			{Label: "C", Rule: map[string]interface{}{"ip_cidr": []string{"10.0.0.0/8"}}},
		},
	}
	ApplyRulesLibraryMigration(sf, td, "")
	if !sf.RulesLibraryMerged {
		t.Fatal("expected merged flag")
	}
	if len(sf.CustomRules) != 1 {
		t.Fatalf("custom len %d", len(sf.CustomRules))
	}
}

func TestApplyRulesLibraryMigration_MergesSelectable(t *testing.T) {
	td := &wizardtemplate.TemplateData{
		SelectableRules: []wizardtemplate.TemplateSelectableRule{
			{Label: "A", IsDefault: true, Rule: map[string]interface{}{"domain": []string{"a.test"}}, HasOutbound: true, DefaultOutbound: "direct-out"},
		},
	}
	sf := &wizardmodels.WizardStateFile{
		Version:            2,
		RulesLibraryMerged: false,
		SelectableRuleStates: []wizardmodels.PersistedSelectableRuleState{
			{Label: "A", Enabled: false, SelectedOutbound: "proxy-out"},
		},
		CustomRules: []wizardmodels.PersistedCustomRule{
			{Label: "Tail", Rule: map[string]interface{}{"ip_cidr": []string{"192.168.0.0/16"}}},
		},
	}
	ApplyRulesLibraryMigration(sf, td, "")
	if len(sf.CustomRules) != 2 {
		t.Fatalf("want 2 custom, got %d", len(sf.CustomRules))
	}
	if sf.CustomRules[0].Label != "A" || sf.CustomRules[0].Enabled {
		t.Fatalf("first rule: %+v", sf.CustomRules[0])
	}
	if sf.CustomRules[1].Label != "Tail" {
		t.Fatalf("second rule: %+v", sf.CustomRules[1])
	}
	if len(sf.SelectableRuleStates) != 0 {
		t.Fatal("selectable should be cleared")
	}
}

func TestAppendClonedPresetsToCustomRules(t *testing.T) {
	m := wizardmodels.NewWizardModel()
	m.CustomRules = nil
	rules := []wizardtemplate.TemplateSelectableRule{
		{Label: "P1", Rule: map[string]interface{}{"domain": []string{"a.test"}}, HasOutbound: true, DefaultOutbound: "direct-out", IsDefault: true},
		{Label: "P2", Rule: map[string]interface{}{"domain": []string{"b.test"}}, HasOutbound: true, DefaultOutbound: "direct-out"},
	}
	picked := []bool{true, false}
	n := AppendClonedPresetsToCustomRules(m, rules, picked)
	if n != 1 || len(m.CustomRules) != 1 {
		t.Fatalf("want 1 appended, got n=%d len=%d", n, len(m.CustomRules))
	}
	if m.CustomRules[0].Rule.Label != "P1" {
		t.Fatalf("label %q", m.CustomRules[0].Rule.Label)
	}
}

func TestAppendClonedPresetsToCustomRules_MismatchLen(t *testing.T) {
	m := wizardmodels.NewWizardModel()
	rules := []wizardtemplate.TemplateSelectableRule{{Label: "A"}}
	if AppendClonedPresetsToCustomRules(m, rules, []bool{true, false}) != 0 {
		t.Fatal("expected 0 when len mismatch")
	}
}

func TestEnsureCustomRulesDefaultOutbounds(t *testing.T) {
	m := wizardmodels.NewWizardModel()
	m.CustomRules = []*wizardmodels.RuleState{
		{
			Rule: wizardtemplate.TemplateSelectableRule{
				Label: "X",
				Rule:  map[string]interface{}{"domain": []string{"z.test"}},
			},
			Enabled:          true,
			SelectedOutbound: "",
		},
	}
	EnsureCustomRulesDefaultOutbounds(m)
	if m.CustomRules[0].SelectedOutbound == "" {
		t.Fatal("expected SelectedOutbound filled from defaults")
	}
}
