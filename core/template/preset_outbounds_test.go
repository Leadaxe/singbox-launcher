package template

import (
	"strings"
	"testing"
)

// Tests cover validatePresetOutbounds entry-level normalization & strip
// (SPEC 056 Phase 2). All cases use the same minimal Preset shell with
// just .Vars (for if/if_or references) and .Outbounds (the field under test).

// makePresetForTest — fresh Preset with id="test" and given vars.
func makePresetForTest(t *testing.T, outs []PresetOutbound, vars ...PresetVar) *Preset {
	t.Helper()
	return &Preset{
		ID:        "test",
		Vars:      append([]PresetVar(nil), vars...),
		Outbounds: append([]PresetOutbound(nil), outs...),
	}
}

func TestValidatePresetOutbounds_EmptyModeBecomesAdd(t *testing.T) {
	p := makePresetForTest(t, []PresetOutbound{
		{Tag: "x", Type: "selector"}, // Mode == ""
	})
	warns := validatePresetOutbounds(p)
	if len(warns) != 0 {
		t.Fatalf("expected no warnings, got %d: %v", len(warns), warns)
	}
	if len(p.Outbounds) != 1 || p.Outbounds[0].Mode != "add" {
		t.Fatalf("expected Mode normalized to add, got %q", p.Outbounds[0].Mode)
	}
}

func TestValidatePresetOutbounds_UnknownModeStrips(t *testing.T) {
	p := makePresetForTest(t, []PresetOutbound{
		{Mode: "delete", Tag: "x", Type: "selector"},
		{Mode: "add", Tag: "y", Type: "direct"},
	})
	warns := validatePresetOutbounds(p)
	if len(warns) != 1 || warns[0].Action != "strip" {
		t.Fatalf("expected 1 strip warning, got %v", warns)
	}
	if !strings.Contains(warns[0].Message, "unknown mode") {
		t.Fatalf("warning text mismatch: %q", warns[0].Message)
	}
	if len(p.Outbounds) != 1 || p.Outbounds[0].Tag != "y" {
		t.Fatalf("expected only y entry kept, got %+v", p.Outbounds)
	}
}

func TestValidatePresetOutbounds_EmptyTagStrips(t *testing.T) {
	p := makePresetForTest(t, []PresetOutbound{
		{Mode: "add", Type: "selector"}, // no Tag
	})
	warns := validatePresetOutbounds(p)
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "empty tag") {
		t.Fatalf("expected empty-tag warning, got %v", warns)
	}
	if len(p.Outbounds) != 0 {
		t.Fatalf("expected entry stripped, got %+v", p.Outbounds)
	}
}

func TestValidatePresetOutbounds_AddRequiresType(t *testing.T) {
	p := makePresetForTest(t, []PresetOutbound{
		{Mode: "add", Tag: "x"}, // no Type
	})
	warns := validatePresetOutbounds(p)
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "requires non-empty type") {
		t.Fatalf("expected add-needs-type warning, got %v", warns)
	}
	if len(p.Outbounds) != 0 {
		t.Fatalf("expected entry stripped, got %+v", p.Outbounds)
	}
}

func TestValidatePresetOutbounds_UpdateDropsType(t *testing.T) {
	p := makePresetForTest(t, []PresetOutbound{
		{Mode: "update", Tag: "x", Type: "selector"}, // type forbidden
	})
	warns := validatePresetOutbounds(p)
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "cannot change type") {
		t.Fatalf("expected type-drop warning, got %v", warns)
	}
	if len(p.Outbounds) != 1 {
		t.Fatalf("expected entry kept (with type dropped), got %+v", p.Outbounds)
	}
	if p.Outbounds[0].Type != "" {
		t.Fatalf("expected type cleared on update entry, got %q", p.Outbounds[0].Type)
	}
}

func TestValidatePresetOutbounds_DuplicateTagKeepsFirst(t *testing.T) {
	p := makePresetForTest(t, []PresetOutbound{
		{Mode: "update", Tag: "proxy-out", Filters: map[string]interface{}{"a": 1}},
		{Mode: "update", Tag: "proxy-out", Filters: map[string]interface{}{"b": 2}},
	})
	warns := validatePresetOutbounds(p)
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "duplicate tag") {
		t.Fatalf("expected duplicate-tag warning, got %v", warns)
	}
	if len(p.Outbounds) != 1 {
		t.Fatalf("expected only first entry kept, got %+v", p.Outbounds)
	}
	if got := p.Outbounds[0].Filters["a"]; got != 1 {
		t.Fatalf("expected first (a:1) to win, got Filters=%+v", p.Outbounds[0].Filters)
	}
}

func TestValidatePresetOutbounds_IfReferencesUnknownVarWarns(t *testing.T) {
	p := makePresetForTest(t,
		[]PresetOutbound{
			{Mode: "add", Tag: "x", Type: "selector", If: []string{"missing_var"}},
		},
		PresetVar{Name: "known_bool", Type: "bool", Default: "false"},
	)
	warns := validatePresetOutbounds(p)
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "unknown var") {
		t.Fatalf("expected unknown-var warning, got %v", warns)
	}
	// Entry kept despite bad ref — match-time fallback will skip.
	if len(p.Outbounds) != 1 {
		t.Fatalf("expected entry kept on bad if-ref, got %+v", p.Outbounds)
	}
}

func TestValidatePresetOutbounds_IfReferencesNonBoolVarWarns(t *testing.T) {
	p := makePresetForTest(t,
		[]PresetOutbound{
			{Mode: "add", Tag: "x", Type: "selector", IfOr: []string{"name_text"}},
		},
		PresetVar{Name: "name_text", Type: "text", Default: "hello"},
	)
	warns := validatePresetOutbounds(p)
	if len(warns) != 1 || !strings.Contains(warns[0].Message, "not a bool var") {
		t.Fatalf("expected not-bool-var warning, got %v", warns)
	}
}

func TestValidatePresetOutbounds_NoOutboundsIsNoop(t *testing.T) {
	p := makePresetForTest(t, nil)
	warns := validatePresetOutbounds(p)
	if len(warns) != 0 {
		t.Fatalf("expected no warnings on empty outbounds, got %v", warns)
	}
	if p.Outbounds != nil {
		t.Fatalf("expected Outbounds untouched (nil), got %+v", p.Outbounds)
	}
}
