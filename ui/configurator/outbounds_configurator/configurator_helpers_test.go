package outbounds_configurator

import (
	"reflect"
	"testing"

	"singbox-launcher/core/config"
	"singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// makeParserConfig builds a *config.ParserConfig with the given per-source
// proxies and global outbounds. Helper keeps the table-driven tests readable.
func makeParserConfig(proxies []config.ProxySource, global []config.OutboundConfig) *config.ParserConfig {
	pc := &config.ParserConfig{}
	pc.ParserConfig.Proxies = proxies
	pc.ParserConfig.Outbounds = global
	return pc
}

func TestCollectRows(t *testing.T) {
	tests := []struct {
		name         string
		pc           *config.ParserConfig
		presetLabels map[string]string
		requiredTags map[string]bool
		// Expected per-row assertions, indexed by the produced row order.
		wantLen    int
		assertRows func(t *testing.T, rows []outboundRow)
	}{
		{
			name: "per-source rows precede global rows; disabled source skipped",
			pc: makeParserConfig(
				[]config.ProxySource{
					{
						Source:    "SubA",
						Outbounds: []config.OutboundConfig{{Tag: "A:auto"}, {Tag: "A:select"}},
					},
					{
						Source:    "SubDisabled",
						Disabled:  true,
						Outbounds: []config.OutboundConfig{{Tag: "D:auto"}},
					},
				},
				[]config.OutboundConfig{{Tag: "global-direct"}},
			),
			wantLen: 3,
			assertRows: func(t *testing.T, rows []outboundRow) {
				// First two rows are per-source (SubA), in slice order.
				if rows[0].IsGlobal || rows[0].SourceIndex != 0 || rows[0].IndexInSlice != 0 || rows[0].Outbound.Tag != "A:auto" {
					t.Errorf("row0 = %+v, want per-source SubA[0] A:auto", rows[0])
				}
				if rows[0].SourceLabel != "SubA" {
					t.Errorf("row0 SourceLabel = %q, want %q", rows[0].SourceLabel, "SubA")
				}
				if rows[1].IsGlobal || rows[1].SourceIndex != 0 || rows[1].IndexInSlice != 1 || rows[1].Outbound.Tag != "A:select" {
					t.Errorf("row1 = %+v, want per-source SubA[1] A:select", rows[1])
				}
				// Disabled source (SubDisabled, SourceIndex 1) must not appear.
				for _, r := range rows {
					if !r.IsGlobal && r.SourceIndex == 1 {
						t.Errorf("disabled source row leaked: %+v", r)
					}
				}
				// Last row is the global direct outbound.
				g := rows[2]
				if !g.IsGlobal || g.IndexInSlice != 0 || g.Outbound.Tag != "global-direct" {
					t.Errorf("row2 = %+v, want global direct", g)
				}
				if g.IsPreset || g.IsTemplate || g.IsRequired || g.HasUserPatch {
					t.Errorf("direct global row should have no flags set: %+v", g)
				}
				if g.SourceLabel != "" {
					t.Errorf("direct global SourceLabel = %q, want empty", g.SourceLabel)
				}
			},
		},
		{
			name: "global template ref, required vs non-required",
			pc: makeParserConfig(nil, []config.OutboundConfig{
				{Tag: "tmpl-req", Ref: config.RefTemplate},
				{Tag: "tmpl-opt", Ref: config.RefTemplate},
			}),
			requiredTags: map[string]bool{"tmpl-req": true},
			wantLen:      2,
			assertRows: func(t *testing.T, rows []outboundRow) {
				req := rows[0]
				if !req.IsTemplate || !req.IsRequired {
					t.Errorf("tmpl-req row = %+v, want IsTemplate && IsRequired", req)
				}
				if req.IsPreset {
					t.Errorf("tmpl-req should not be IsPreset: %+v", req)
				}
				if req.SourceLabel != "🔒" {
					t.Errorf("required template SourceLabel = %q, want lock emoji", req.SourceLabel)
				}
				opt := rows[1]
				if !opt.IsTemplate || opt.IsRequired {
					t.Errorf("tmpl-opt row = %+v, want IsTemplate && !IsRequired", opt)
				}
				if opt.SourceLabel != "" {
					t.Errorf("non-required template SourceLabel = %q, want empty", opt.SourceLabel)
				}
			},
		},
		{
			name: "nil requiredTags means no template row is required",
			pc: makeParserConfig(nil, []config.OutboundConfig{
				{Tag: "tmpl-x", Ref: config.RefTemplate},
			}),
			requiredTags: nil,
			wantLen:      1,
			assertRows: func(t *testing.T, rows []outboundRow) {
				if !rows[0].IsTemplate {
					t.Errorf("row should be IsTemplate: %+v", rows[0])
				}
				if rows[0].IsRequired {
					t.Errorf("row should not be required when requiredTags is nil: %+v", rows[0])
				}
				if rows[0].SourceLabel != "" {
					t.Errorf("SourceLabel = %q, want empty", rows[0].SourceLabel)
				}
			},
		},
		{
			name: "global preset ref with known label",
			pc: makeParserConfig(nil, []config.OutboundConfig{
				{Tag: "p1-out", Ref: "preset-one"},
			}),
			presetLabels: map[string]string{"preset-one": "Preset One"},
			wantLen:      1,
			assertRows: func(t *testing.T, rows []outboundRow) {
				r := rows[0]
				if !r.IsPreset || r.IsTemplate {
					t.Errorf("row = %+v, want IsPreset && !IsTemplate", r)
				}
				if r.PresetID != "preset-one" {
					t.Errorf("PresetID = %q, want %q", r.PresetID, "preset-one")
				}
				if r.PresetLabel != "Preset One" {
					t.Errorf("PresetLabel = %q, want %q", r.PresetLabel, "Preset One")
				}
				if r.SourceLabel != "🔒 Preset One" {
					t.Errorf("SourceLabel = %q, want %q", r.SourceLabel, "🔒 Preset One")
				}
			},
		},
		{
			name: "global preset ref dangling (no label) falls back to ref id",
			pc: makeParserConfig(nil, []config.OutboundConfig{
				{Tag: "p2-out", Ref: "ghost-preset"},
			}),
			presetLabels: map[string]string{"other": "Other"},
			wantLen:      1,
			assertRows: func(t *testing.T, rows []outboundRow) {
				r := rows[0]
				if !r.IsPreset {
					t.Errorf("row = %+v, want IsPreset", r)
				}
				if r.PresetLabel != "ghost-preset" {
					t.Errorf("PresetLabel = %q, want fallback %q", r.PresetLabel, "ghost-preset")
				}
				if r.SourceLabel != "🔒 ghost-preset" {
					t.Errorf("SourceLabel = %q, want %q", r.SourceLabel, "🔒 ghost-preset")
				}
			},
		},
		{
			name: "global preset ref with nil presetLabels uses ref id",
			pc: makeParserConfig(nil, []config.OutboundConfig{
				{Tag: "p3-out", Ref: "preset-nil"},
			}),
			presetLabels: nil,
			wantLen:      1,
			assertRows: func(t *testing.T, rows []outboundRow) {
				if rows[0].PresetLabel != "preset-nil" {
					t.Errorf("PresetLabel = %q, want %q", rows[0].PresetLabel, "preset-nil")
				}
			},
		},
		{
			name: "HasUserPatch badge appended for referenced template entry",
			pc: makeParserConfig(nil, []config.OutboundConfig{
				{
					Tag: "tmpl-req", Ref: config.RefTemplate,
					Updates: []config.OutboundUpdate{
						{Ref: "some-preset", Patch: map[string]interface{}{"x": 1}},
						{Ref: config.RefUser, Patch: map[string]interface{}{"y": 2}},
					},
				},
			}),
			requiredTags: map[string]bool{"tmpl-req": true},
			wantLen:      1,
			assertRows: func(t *testing.T, rows []outboundRow) {
				r := rows[0]
				if !r.HasUserPatch {
					t.Errorf("row should have HasUserPatch=true: %+v", r)
				}
				// Required template => base label "🔒", then " ✏" appended.
				if r.SourceLabel != "🔒 ✏" {
					t.Errorf("SourceLabel = %q, want %q", r.SourceLabel, "🔒 ✏")
				}
			},
		},
		{
			name: "HasUserPatch badge for preset entry",
			pc: makeParserConfig(nil, []config.OutboundConfig{
				{
					Tag: "p-out", Ref: "preset-one",
					Updates: []config.OutboundUpdate{
						{Ref: config.RefUser, Patch: map[string]interface{}{"y": 2}},
					},
				},
			}),
			presetLabels: map[string]string{"preset-one": "Preset One"},
			wantLen:      1,
			assertRows: func(t *testing.T, rows []outboundRow) {
				r := rows[0]
				if !r.HasUserPatch {
					t.Errorf("row should have HasUserPatch=true: %+v", r)
				}
				if r.SourceLabel != "🔒 Preset One ✏" {
					t.Errorf("SourceLabel = %q, want %q", r.SourceLabel, "🔒 Preset One ✏")
				}
			},
		},
		{
			name: "HasUserPatch on direct global does not append badge",
			pc: makeParserConfig(nil, []config.OutboundConfig{
				{
					Tag: "direct", Ref: "",
					Updates: []config.OutboundUpdate{
						{Ref: config.RefUser, Patch: map[string]interface{}{"y": 2}},
					},
				},
			}),
			wantLen: 1,
			assertRows: func(t *testing.T, rows []outboundRow) {
				r := rows[0]
				// HasUserPatch is still computed true, but the badge is only
				// appended when IsTemplate || IsPreset.
				if !r.HasUserPatch {
					t.Errorf("HasUserPatch should be true: %+v", r)
				}
				if r.IsTemplate || r.IsPreset {
					t.Errorf("direct row should not be template/preset: %+v", r)
				}
				if r.SourceLabel != "" {
					t.Errorf("direct row SourceLabel = %q, want empty (no badge)", r.SourceLabel)
				}
			},
		},
		{
			name: "empty source label is synthesized (not crashed)",
			pc: makeParserConfig(
				[]config.ProxySource{
					{Source: "", Outbounds: []config.OutboundConfig{{Tag: "x"}}},
				},
				nil,
			),
			wantLen: 1,
			assertRows: func(t *testing.T, rows []outboundRow) {
				if rows[0].SourceLabel == "" {
					t.Errorf("empty source should get a synthesized label, got empty")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := collectRows(tt.pc, tt.presetLabels, tt.requiredTags)
			if len(rows) != tt.wantLen {
				t.Fatalf("collectRows len = %d, want %d (rows=%+v)", len(rows), tt.wantLen, rows)
			}
			if tt.assertRows != nil {
				tt.assertRows(t, rows)
			}
		})
	}
}

func TestCollectAllTags(t *testing.T) {
	tests := []struct {
		name string
		pc   *config.ParserConfig
		want []string
	}{
		{
			name: "local first then global, disabled source skipped",
			pc: makeParserConfig(
				[]config.ProxySource{
					{Source: "A", Outbounds: []config.OutboundConfig{{Tag: "a1"}, {Tag: "a2"}}},
					{Source: "B", Disabled: true, Outbounds: []config.OutboundConfig{{Tag: "b1"}}},
					{Source: "C", Outbounds: []config.OutboundConfig{{Tag: "c1"}}},
				},
				[]config.OutboundConfig{{Tag: "g1"}, {Tag: "g2"}},
			),
			want: []string{"a1", "a2", "c1", "g1", "g2"},
		},
		{
			name: "only global",
			pc:   makeParserConfig(nil, []config.OutboundConfig{{Tag: "g1"}}),
			want: []string{"g1"},
		},
		{
			name: "empty config yields nil",
			pc:   makeParserConfig(nil, nil),
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collectAllTags(tt.pc)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("collectAllTags = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTagsAbove(t *testing.T) {
	rows := []outboundRow{
		{Outbound: &config.OutboundConfig{Tag: "t0"}},
		{Outbound: &config.OutboundConfig{Tag: "t1"}},
		{Outbound: &config.OutboundConfig{Tag: "t2"}},
	}
	tests := []struct {
		name     string
		rowIndex int
		want     []string
	}{
		{name: "index 0 -> nil", rowIndex: 0, want: nil},
		{name: "negative -> nil", rowIndex: -5, want: nil},
		{name: "index 1 -> first tag", rowIndex: 1, want: []string{"t0"}},
		{name: "index 2 -> first two tags", rowIndex: 2, want: []string{"t0", "t1"}},
		{name: "index == len -> all tags", rowIndex: 3, want: []string{"t0", "t1", "t2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tagsAbove(rows, tt.rowIndex)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("tagsAbove(%d) = %v, want %v", tt.rowIndex, got, tt.want)
			}
		})
	}
}

func TestSameScope(t *testing.T) {
	tests := []struct {
		name string
		a    outboundRow
		b    outboundRow
		want bool
	}{
		{
			name: "both global",
			a:    outboundRow{IsGlobal: true},
			b:    outboundRow{IsGlobal: true},
			want: true,
		},
		{
			name: "global vs source",
			a:    outboundRow{IsGlobal: true},
			b:    outboundRow{IsGlobal: false, SourceIndex: 0},
			want: false,
		},
		{
			name: "source vs global",
			a:    outboundRow{IsGlobal: false, SourceIndex: 2},
			b:    outboundRow{IsGlobal: true},
			want: false,
		},
		{
			name: "same source index",
			a:    outboundRow{IsGlobal: false, SourceIndex: 1},
			b:    outboundRow{IsGlobal: false, SourceIndex: 1},
			want: true,
		},
		{
			name: "different source index",
			a:    outboundRow{IsGlobal: false, SourceIndex: 1},
			b:    outboundRow{IsGlobal: false, SourceIndex: 2},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameScope(tt.a, tt.b); got != tt.want {
				t.Errorf("sameScope = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTemplateRequiredTags(t *testing.T) {
	// Valid template JSON: wrapped {"ParserConfig": {...}} (capital P).
	validJSON := `{
		"ParserConfig": {
			"outbounds": [
				{"tag": "req-a", "required": true},
				{"tag": "opt-b", "required": false},
				{"tag": "req-c", "required": true},
				{"tag": "", "required": true},
				{"required": true}
			]
		}
	}`

	tests := []struct {
		name  string
		model *wizardmodels.WizardModel
		want  map[string]bool
	}{
		{
			name:  "nil model",
			model: nil,
			want:  nil,
		},
		{
			name:  "nil TemplateData",
			model: &wizardmodels.WizardModel{TemplateData: nil},
			want:  nil,
		},
		{
			name:  "empty ParserConfig string",
			model: &wizardmodels.WizardModel{TemplateData: &template.TemplateData{ParserConfig: ""}},
			want:  nil,
		},
		{
			name:  "malformed JSON yields nil",
			model: &wizardmodels.WizardModel{TemplateData: &template.TemplateData{ParserConfig: `{not valid json`}},
			want:  nil,
		},
		{
			name:  "valid: only required tags with non-empty tag",
			model: &wizardmodels.WizardModel{TemplateData: &template.TemplateData{ParserConfig: validJSON}},
			want:  map[string]bool{"req-a": true, "req-c": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := templateRequiredTags(tt.model)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("templateRequiredTags = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTemplateGlobalOutbounds(t *testing.T) {
	validJSON := `{
		"ParserConfig": {
			"outbounds": [
				{"tag": "g1", "type": "selector"},
				{"tag": "g2", "type": "urltest"}
			]
		}
	}`

	tests := []struct {
		name     string
		model    *wizardmodels.WizardModel
		wantTags []string
		wantNil  bool
	}{
		{
			name:    "nil model",
			model:   nil,
			wantNil: true,
		},
		{
			name:    "nil TemplateData",
			model:   &wizardmodels.WizardModel{TemplateData: nil},
			wantNil: true,
		},
		{
			name:    "empty ParserConfig string",
			model:   &wizardmodels.WizardModel{TemplateData: &template.TemplateData{ParserConfig: ""}},
			wantNil: true,
		},
		{
			name:    "malformed JSON yields nil",
			model:   &wizardmodels.WizardModel{TemplateData: &template.TemplateData{ParserConfig: `{bad`}},
			wantNil: true,
		},
		{
			name:     "valid template outbounds in declaration order",
			model:    &wizardmodels.WizardModel{TemplateData: &template.TemplateData{ParserConfig: validJSON}},
			wantTags: []string{"g1", "g2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := templateGlobalOutbounds(tt.model)
			if tt.wantNil {
				if got != nil {
					t.Errorf("templateGlobalOutbounds = %v, want nil", got)
				}
				return
			}
			var gotTags []string
			for _, ob := range got {
				gotTags = append(gotTags, ob.Tag)
			}
			if !reflect.DeepEqual(gotTags, tt.wantTags) {
				t.Errorf("templateGlobalOutbounds tags = %v, want %v", gotTags, tt.wantTags)
			}
		})
	}
}

func TestPresetLabelsByID(t *testing.T) {
	tests := []struct {
		name  string
		model *wizardmodels.WizardModel
		want  map[string]string
	}{
		{
			name:  "nil model",
			model: nil,
			want:  nil,
		},
		{
			name:  "nil TemplateData",
			model: &wizardmodels.WizardModel{TemplateData: nil},
			want:  nil,
		},
		{
			name: "label used when set; id used as fallback when label empty",
			model: &wizardmodels.WizardModel{
				TemplateData: &template.TemplateData{
					Presets: []template.Preset{
						{ID: "p1", Label: "Preset One"},
						{ID: "p2", Label: ""},
					},
				},
			},
			want: map[string]string{"p1": "Preset One", "p2": "p2"},
		},
		{
			name: "empty presets yields empty (non-nil) map",
			model: &wizardmodels.WizardModel{
				TemplateData: &template.TemplateData{Presets: nil},
			},
			want: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := presetLabelsByID(tt.model)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("presetLabelsByID = %v, want %v", got, tt.want)
			}
		})
	}
}
