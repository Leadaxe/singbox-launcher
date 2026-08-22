package models

import (
	"testing"

	wizardtemplate "singbox-launcher/core/template"
)

// SPEC 106: системное правило удерживает позицию — проверяем обе стороны
// защиты, а не только отсутствие захвата в UI.

func systemModel() *WizardModel {
	no := false
	return &WizardModel{
		TemplateData: &wizardtemplate.TemplateData{
			Presets: []wizardtemplate.Preset{
				{ID: "traffic-processing", Sortable: &no},
				{ID: "block-ads"},
			},
		},
		PresetRefs: []*PresetRefState{
			{Ref: "traffic-processing"},
			{Ref: "block-ads"},
		},
		RuleOrder: []RuleSlot{
			{Kind: SlotKindPresetRef, Index: 0},
			{Kind: SlotKindPresetRef, Index: 1},
		},
	}
}

func TestSystemRuleCannotBeMoved(t *testing.T) {
	m := systemModel()
	if MoveRuleSlot(m, 0, 1) {
		t.Fatal("системное правило удалось сдвинуть")
	}
	if m.RuleOrder[0].Index != 0 {
		t.Error("порядок изменился")
	}
}

func TestNothingCanBeDroppedAboveSystemRule(t *testing.T) {
	m := systemModel()
	if MoveRuleSlot(m, 1, 0) {
		t.Fatal("чужое правило встало выше системного — sniff перестал быть первым")
	}
}

func TestOrdinaryRulesStillMove(t *testing.T) {
	// Защита не должна ломать обычную перестановку ниже системного правила.
	m := systemModel()
	m.PresetRefs = append(m.PresetRefs, &PresetRefState{Ref: "block-ads"})
	m.RuleOrder = append(m.RuleOrder, RuleSlot{Kind: SlotKindPresetRef, Index: 2})
	if !MoveRuleSlot(m, 2, 1) {
		t.Fatal("обычная перестановка перестала работать")
	}
}
