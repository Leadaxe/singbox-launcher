package outbounds_configurator

import (
	"reflect"
	"testing"

	"fyne.io/fyne/v2"

	"singbox-launcher/core/config"
	"singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// stubEditPresenter is a minimal OutboundEditPresenter implementation for tests.
// Only Model() carries logic; the window/overlay methods are no-ops because
// templateVarChoices never touches them.
type stubEditPresenter struct {
	model *wizardmodels.WizardModel
}

func (s *stubEditPresenter) OpenOutboundEditWindow() fyne.Window { return nil }
func (s *stubEditPresenter) SetOutboundEditWindow(fyne.Window)   {}
func (s *stubEditPresenter) ClearOutboundEditWindow()            {}
func (s *stubEditPresenter) UpdateChildOverlay()                 {}
func (s *stubEditPresenter) Model() *wizardmodels.WizardModel    { return s.model }

func TestLabelForValue(t *testing.T) {
	tests := []struct {
		name         string
		labelToValue map[string]string
		value        string
		want         string
	}{
		{
			name:         "reverse lookup finds matching label",
			labelToValue: map[string]string{"Five Minutes": "5m"},
			value:        "5m",
			want:         "Five Minutes",
		},
		{
			name:         "no match returns empty",
			labelToValue: map[string]string{"Five Minutes": "5m"},
			value:        "30s",
			want:         "",
		},
		{
			name:         "nil map returns empty",
			labelToValue: nil,
			value:        "5m",
			want:         "",
		},
		{
			name:         "empty value matched against placeholder identity",
			labelToValue: map[string]string{"@v": "@v"},
			value:        "@v",
			want:         "@v",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := labelForValue(tt.labelToValue, tt.value); got != tt.want {
				t.Errorf("labelForValue(%v, %q) = %q, want %q", tt.labelToValue, tt.value, got, tt.want)
			}
		})
	}
}

func TestFilterOutUserPatch(t *testing.T) {
	tests := []struct {
		name    string
		updates []config.OutboundUpdate
		want    []config.OutboundUpdate
	}{
		{
			name:    "nil input yields nil",
			updates: nil,
			want:    nil,
		},
		{
			name:    "empty input yields nil",
			updates: []config.OutboundUpdate{},
			want:    nil,
		},
		{
			name: "removes only the USER patch, keeps preset patches in order",
			updates: []config.OutboundUpdate{
				{Ref: "preset-a", Patch: map[string]interface{}{"a": 1}},
				{Ref: config.RefUser, Patch: map[string]interface{}{"u": 1}},
				{Ref: "preset-b", Patch: map[string]interface{}{"b": 1}},
			},
			want: []config.OutboundUpdate{
				{Ref: "preset-a", Patch: map[string]interface{}{"a": 1}},
				{Ref: "preset-b", Patch: map[string]interface{}{"b": 1}},
			},
		},
		{
			name: "only a USER patch yields nil (no preset patches left)",
			updates: []config.OutboundUpdate{
				{Ref: config.RefUser, Patch: map[string]interface{}{"u": 1}},
			},
			want: nil,
		},
		{
			name: "no USER patch leaves slice unchanged",
			updates: []config.OutboundUpdate{
				{Ref: "preset-a", Patch: map[string]interface{}{"a": 1}},
			},
			want: []config.OutboundUpdate{
				{Ref: "preset-a", Patch: map[string]interface{}{"a": 1}},
			},
		},
		{
			name: "multiple USER patches all removed",
			updates: []config.OutboundUpdate{
				{Ref: config.RefUser, Patch: map[string]interface{}{"u1": 1}},
				{Ref: "preset-x", Patch: map[string]interface{}{"x": 1}},
				{Ref: config.RefUser, Patch: map[string]interface{}{"u2": 1}},
			},
			want: []config.OutboundUpdate{
				{Ref: "preset-x", Patch: map[string]interface{}{"x": 1}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterOutUserPatch(tt.updates)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterOutUserPatch = %#v, want %#v", got, tt.want)
			}
			// USER patch must never survive the filter.
			for _, u := range got {
				if u.Ref == config.RefUser {
					t.Errorf("USER patch leaked through filter: %#v", got)
				}
			}
		})
	}
}

func TestTemplateVarChoices(t *testing.T) {
	// Template var "interval" with two options, the second carrying a title.
	makePresenter := func(td *template.TemplateData) *stubEditPresenter {
		if td == nil {
			return &stubEditPresenter{model: &wizardmodels.WizardModel{TemplateData: nil}}
		}
		return &stubEditPresenter{model: &wizardmodels.WizardModel{TemplateData: td}}
	}

	tdWithVar := &template.TemplateData{
		Vars: []template.TemplateVar{
			{
				Name:         "interval",
				Options:      []string{"5m", "30s"},
				OptionTitles: []string{"", "Thirty Seconds"},
			},
			{
				Name:    "other",
				Options: []string{"x"},
			},
		},
	}

	t.Run("placeholder first; options append using titles when present", func(t *testing.T) {
		labels, labelToValue := templateVarChoices(makePresenter(tdWithVar), "interval", "")
		wantLabels := []string{"@interval", "5m", "Thirty Seconds"}
		if !reflect.DeepEqual(labels, wantLabels) {
			t.Errorf("labels = %v, want %v", labels, wantLabels)
		}
		wantMap := map[string]string{
			"@interval":      "@interval",
			"5m":             "5m",
			"Thirty Seconds": "30s",
		}
		if !reflect.DeepEqual(labelToValue, wantMap) {
			t.Errorf("labelToValue = %v, want %v", labelToValue, wantMap)
		}
	})

	t.Run("nil presenter yields placeholder only", func(t *testing.T) {
		labels, labelToValue := templateVarChoices(nil, "interval", "")
		wantLabels := []string{"@interval"}
		if !reflect.DeepEqual(labels, wantLabels) {
			t.Errorf("labels = %v, want %v", labels, wantLabels)
		}
		if !reflect.DeepEqual(labelToValue, map[string]string{"@interval": "@interval"}) {
			t.Errorf("labelToValue = %v, want placeholder-only", labelToValue)
		}
	})

	t.Run("nil TemplateData yields placeholder only", func(t *testing.T) {
		labels, labelToValue := templateVarChoices(makePresenter(nil), "interval", "")
		if !reflect.DeepEqual(labels, []string{"@interval"}) {
			t.Errorf("labels = %v, want placeholder-only", labels)
		}
		if !reflect.DeepEqual(labelToValue, map[string]string{"@interval": "@interval"}) {
			t.Errorf("labelToValue = %v, want placeholder-only", labelToValue)
		}
	})

	t.Run("unknown var name yields placeholder only", func(t *testing.T) {
		labels, _ := templateVarChoices(makePresenter(tdWithVar), "nonexistent", "")
		if !reflect.DeepEqual(labels, []string{"@nonexistent"}) {
			t.Errorf("labels = %v, want placeholder-only", labels)
		}
	})

	t.Run("custom currentValue preserved when not among options", func(t *testing.T) {
		labels, labelToValue := templateVarChoices(makePresenter(tdWithVar), "interval", "7m")
		want := []string{"@interval", "5m", "Thirty Seconds", "7m"}
		if !reflect.DeepEqual(labels, want) {
			t.Errorf("labels = %v, want %v", labels, want)
		}
		if labelToValue["7m"] != "7m" {
			t.Errorf("labelToValue[7m] = %q, want %q", labelToValue["7m"], "7m")
		}
	})

	t.Run("currentValue matching an option is not duplicated", func(t *testing.T) {
		labels, _ := templateVarChoices(makePresenter(tdWithVar), "interval", "30s")
		want := []string{"@interval", "5m", "Thirty Seconds"}
		if !reflect.DeepEqual(labels, want) {
			t.Errorf("labels = %v, want %v (no duplicate for 30s)", labels, want)
		}
	})

	t.Run("currentValue equal to placeholder is not appended", func(t *testing.T) {
		labels, _ := templateVarChoices(makePresenter(tdWithVar), "interval", "@interval")
		want := []string{"@interval", "5m", "Thirty Seconds"}
		if !reflect.DeepEqual(labels, want) {
			t.Errorf("labels = %v, want %v (placeholder not re-appended)", labels, want)
		}
	})
}
