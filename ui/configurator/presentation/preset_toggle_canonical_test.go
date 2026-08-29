package presentation

// SPEC 117 §5.C — сценарий C4: preset toggle производит ОДНУ запись — в
// canonical model.GlobalOutbounds; повторный toggle идемпотентен, выключение
// пресета убирает его записи. Четвёртой копии (legacy-вида) не существует —
// нет и объекта для неё.

import (
	"os"
	"path/filepath"
	"testing"

	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// presetAddedTag — Направление, которое добавляет пресет russian
// (template.presets[russian].outbounds mode=add).
const presetAddedTag = "ru VPN 🇷🇺"

// findRepoRootForTemplate поднимается до каталога с go.mod и
// bin/wizard_template.json (тот же приём, что в business-тестах).
func findRepoRootForTemplate(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := wd
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "bin", "wizard_template.json")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("project root not found from %s", wd)
	return ""
}

func countGlobalTag(m *wizardmodels.WizardModel, tag string) int {
	n := 0
	for i := range m.GlobalOutbounds {
		if m.GlobalOutbounds[i].Tag == tag {
			n++
		}
	}
	return n
}

func TestPresetToggle_SingleCanonicalWriteAndIdempotent(t *testing.T) {
	root := findRepoRootForTemplate(t)
	td, err := wizardtemplate.LoadTemplateData(root)
	if err != nil {
		t.Fatalf("load template: %v", err)
	}

	m := wizardmodels.NewWizardModel()
	m.TemplateData = td
	m.ExecDir = root
	m.Sources = []wizardmodels.Source{{
		ID:   "01C4SUB00000000000000000",
		Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
		URL:  "https://example.com/sub",
	}}
	m.PresetRefs = []*wizardmodels.PresetRefState{{Ref: "russian", Enabled: true}}
	wizardmodels.RebuildRuleOrder(m)

	p := NewWizardPresenter(m, &GUIState{}, nil)

	rev := m.Revision
	p.RefreshAfterPresetToggle()
	if m.Revision <= rev {
		t.Fatal("toggle: ревизия модели не выросла")
	}
	if got := countGlobalTag(m, presetAddedTag); got != 1 {
		t.Fatalf("после toggle записей %q в GlobalOutbounds: %d, ожидалась ровно 1", presetAddedTag, got)
	}
	// Единственность записи: у источника локальных копий не появилось.
	if len(m.Sources[0].Outbounds) != 0 {
		t.Fatalf("toggle затронул локальные Outbounds источника: %+v", m.Sources[0].Outbounds)
	}

	// Идемпотентность: повторный вызов не плодит дублей.
	globalsBefore := len(m.GlobalOutbounds)
	p.RefreshAfterPresetToggle()
	if got := countGlobalTag(m, presetAddedTag); got != 1 {
		t.Fatalf("повторный toggle: записей %q = %d, ожидалась 1", presetAddedTag, got)
	}
	if len(m.GlobalOutbounds) != globalsBefore {
		t.Fatalf("повторный toggle изменил состав: %d → %d", globalsBefore, len(m.GlobalOutbounds))
	}

	// Выключение пресета убирает его запись (drop stale, тоже одна запись).
	m.PresetRefs[0].Enabled = false
	p.RefreshAfterPresetToggle()
	if got := countGlobalTag(m, presetAddedTag); got != 0 {
		t.Fatalf("после выключения пресета записей %q = %d, ожидалось 0", presetAddedTag, got)
	}
}
