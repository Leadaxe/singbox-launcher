package business

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 182: группа свёртки источника — цель detour. Пикер показывает группы
// всех режимов отдельной веткой, у both — только селектор; свою группу,
// группу выключенного источника и имя, занятое Направлением, не показывает;
// группу, из которой переходы ведут обратно к носителю, показывает с Loop.
func TestDetourPickerOffersReplaceGroups(t *testing.T) {
	folded := func(id, tag, mode string, enabled bool) corestate.Source {
		return corestate.Source{
			Node:    corestate.Node{Kind: corestate.SourceKindSubscription, Enabled: enabled},
			ID:      id,
			Name:    id,
			Replace: &corestate.FolderReplace{Mode: mode, Tag: tag},
		}
	}
	m := &wizardmodels.WizardModel{
		GlobalOutbounds: []configtypes.Direction{{Tag: "vpn"}},
		Sources: []corestate.Source{
			folded("MAN", "man", corestate.FolderReplaceManual, true),
			folded("AUTO", "auto", corestate.FolderReplaceAuto, true),
			folded("BOTH", "both", corestate.FolderReplaceBoth, true),
			folded("OFF", "off", corestate.FolderReplaceManual, false),
			folded("CLASH", "vpn", corestate.FolderReplaceManual, true), // имя занято Направлением
			folded("SELF", "self", corestate.FolderReplaceManual, true),
			// LOOP: узел этой подписки ходит через группу SELF.
			func() corestate.Source {
				s := folded("LOOP", "loop", corestate.FolderReplaceManual, true)
				s.Nodes = []corestate.Node{{Kind: corestate.SourceKindServer, Tag: "n", Enabled: true,
					Detour: &corestate.NodeLink{Tag: "self"}}}
				return s
			}(),
		},
	}
	holder := &m.Sources[5]

	_, _, choices := DetourOptionsWithNodes(m, holder, "(none)")
	for _, tag := range []string{"man", "auto", "both", "loop"} {
		c, ok := choices[tag]
		if !ok {
			t.Errorf("группа %q не предложена", tag)
			continue
		}
		if c.Kind != DetourKindReplaceGroup {
			t.Errorf("группа %q стоит не в своей ветке: kind=%d", tag, c.Kind)
		}
		if c.Link == nil || c.Link.FolderID != "" || c.Link.Tag != tag {
			t.Errorf("группа %q адресована не корневой ссылкой: %+v", tag, c.Link)
		}
		if c.Loop != (tag == "loop") {
			t.Errorf("группа %q: Loop=%v", tag, c.Loop)
		}
	}
	for _, tag := range []string{"both-auto", "off", "self"} {
		if _, ok := choices[tag]; ok {
			t.Errorf("%q предложен целью, а не должен", tag)
		}
	}
	if c := choices["vpn"]; c.Kind != DetourKindDirection {
		t.Errorf("имя Направления ушло в группы свёртки: kind=%d", c.Kind)
	}

	// Ссылка на двойник из старого состояния — живая, «пропавшей» не помечается.
	holder.Detour = &corestate.NodeLink{Tag: "both-auto"}
	_, selected, choices := DetourOptionsWithNodes(m, holder, "(none)")
	if c := choices[selected]; selected != "both-auto" || c.Missing || c.Kind != DetourKindReplaceGroup {
		t.Errorf("ссылка на двойник: selected=%q choice=%+v", selected, c)
	}

	// Тот же список — у цепочки: носитель-цепочка, через которую ходит
	// свёрнутый источник, видит его группу с Loop.
	m.Sources[0].Detour = &corestate.NodeLink{Tag: "chain"}
	m.Sources = append(m.Sources, corestate.Source{
		Node: corestate.Node{Kind: corestate.SourceKindChain, Tag: "chain", Enabled: true}, ID: "CH"})
	for _, g := range ReplaceGroupTargets(m, LinkHolder{SourceID: "CH"}) {
		if g.Loop != (g.Tag == "man") {
			t.Errorf("цепочка: группа %q Loop=%v", g.Tag, g.Loop)
		}
	}
}
