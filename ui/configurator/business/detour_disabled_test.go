package business

import (
	"strings"
	"testing"

	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// Пикер detour предлагает ТОЛЬКО включённые узлы: выключенный в конфиг не
// идёт, и выбор указал бы в пустоту — сборка уронила бы носителя fail-closed.
//
// Действующий выбор при этом не теряется, даже если цель погасла: иначе
// человек не узнал бы, куда ведёт его настройка.
func TestDetourPickerHidesDisabledNodes(t *testing.T) {
	node := func(tag string, enabled bool) corestate.Source {
		return corestate.Source{
			Node: corestate.Node{Kind: corestate.SourceKindServer, Tag: tag, Enabled: enabled},
			ID:   "S-" + tag,
		}
	}
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		node("live", true),
		node("dark", false),
		node("holder", true),
	}}
	holder := &m.Sources[2]

	opts, _, _ := DetourOptionsWithNodes(m, holder, "(none)")
	joined := strings.Join(opts, " | ")
	if !strings.Contains(joined, "live") {
		t.Errorf("включённый узел пропал из списка: %s", joined)
	}
	if strings.Contains(joined, "dark") {
		t.Errorf("выключенный узел предлагается целью: %s", joined)
	}

	// Выбор, сделанный до выключения цели, обязан остаться видимым.
	holder.Detour = &corestate.NodeLink{Tag: "dark"}
	opts2, selected, _ := DetourOptionsWithNodes(m, holder, "(none)")
	if selected != "dark" {
		t.Errorf("действующий выбор потерян: %q", selected)
	}
	if !strings.Contains(strings.Join(opts2, " | "), "dark") {
		t.Errorf("выбранная (погасшая) цель исчезла из списка: %v", opts2)
	}
}
