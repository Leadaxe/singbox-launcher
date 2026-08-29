package business

import (
	"testing"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 112-A, «Смена идентичности узла = сброс ссылок с предупреждением».
//
// SPEC 118 W5: ссылка на цель дозвона — ОДИН объект NodeLink; прежняя тройня
// detour_tag / detour_node_source_id / detour_node_hash умерла. Поведение при
// переименовании узла — прежнее: ссылка на старое имя гаснет и об этом
// говорят вслух.

// serverSource — верхний узел-сервер с заданным тегом.
func serverSource(id, tag, label string) wizardmodels.Source {
	return wizardmodels.Source{
		ID:    id,
		Node:  wizardmodels.Node{Kind: wizardmodels.SourceKindServer, Enabled: true, Tag: tag},
		Label: label,
	}
}

// subSource — подписка со ссылкой detour (link может быть nil).
func subSource(id, label string, link *wizardmodels.NodeLink) wizardmodels.Source {
	return wizardmodels.Source{
		ID:    id,
		Node:  wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true, Detour: link},
		Label: label,
		URL:   "https://example.com/" + id,
	}
}

func modelWithHopAndDependents(hopID, hopTag string) *wizardmodels.WizardModel {
	return &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		serverSource(hopID, hopTag, "WARP hop"),
		subSource("01PROTON", "Proton NL", &wizardmodels.NodeLink{FolderID: hopID, Tag: hopTag}),
		subSource("01OTHER", "Liberty", nil),
	}}
}

// Переименование узла = смена идентичности: ссылка на прежнее имя гаснет, и
// зависимый источник назван в отчёте для окна.
func TestResetDetourNodeRefs_ClearsFullRef(t *testing.T) {
	m := modelWithHopAndDependents("01WARP", "🔥🎭 WARP (MASQUE)")

	affected := ResetDetourNodeRefs(m, "01WARP", "🔥🎭 WARP (MASQUE)")
	if len(affected) != 1 || affected[0] != "Proton NL" {
		t.Fatalf("затронутые источники = %v, ожидался [Proton NL]", affected)
	}
	if got := m.Sources[1].Detour; got != nil {
		t.Errorf("ссылка обязана погаснуть целиком, осталось %+v", got)
	}
	if m.Sources[2].Detour != nil {
		t.Error("чужой источник тронут не был")
	}
}

// Ссылка на ДРУГОЙ узел того же источника переименования не заметила: у
// подписки много узлов, и смена имени одного не касается остальных.
func TestResetDetourNodeRefs_KeepsSiblingNodeRef(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		subSource("01LIB", "AL: Liberty", nil),
		subSource("01PROTON", "Proton NL", &wizardmodels.NodeLink{FolderID: "01LIB", Tag: "🇳🇱 Amsterdam-2"}),
	}}

	if affected := ResetDetourNodeRefs(m, "01LIB", "🇳🇱 Amsterdam-1"); len(affected) != 0 {
		t.Fatalf("ссылка на соседний узел трогаться не должна, затронуто %v", affected)
	}
	if d := m.Sources[1].Detour; d == nil || d.Tag != "🇳🇱 Amsterdam-2" {
		t.Error("ссылка на соседний узел стёрта")
	}
}

// Ссылка корневого пространства (без folderId) гасится, только когда имя
// однозначно: тёзка среди верхних узлов делает адресацию неоднозначной, и
// сброс задел бы чужую ссылку.
func TestResetDetourNodeRefs_TagOnlyRefOnlyWhenUnambiguous(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		serverSource("01WARP", "hop", ""),
		subSource("01PROTON", "Proton NL", &wizardmodels.NodeLink{Tag: "hop"}),
	}}
	if affected := ResetDetourNodeRefs(m, "01WARP", "hop"); len(affected) != 1 {
		t.Fatalf("однозначная ссылка обязана погаснуть, затронуто %v", affected)
	}
	if m.Sources[1].Detour != nil {
		t.Error("однозначная ссылка не погасла")
	}

	m2 := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		serverSource("01WARP", "hop", ""),
		serverSource("01TWIN", "hop", ""),
		subSource("01PROTON", "Proton NL", &wizardmodels.NodeLink{Tag: "hop"}),
	}}
	if affected := ResetDetourNodeRefs(m2, "01WARP", "hop"); len(affected) != 0 {
		t.Fatalf("неоднозначная ссылка трогаться не должна, затронуто %v", affected)
	}
	if d := m2.Sources[2].Detour; d == nil || d.Tag != "hop" {
		t.Error("сброс задел неоднозначную ссылку")
	}
}

// SPEC 113-E: подсчёт однозначности идёт по состоянию ДО правки —
// переименованный источник уже носит новое имя, и считать его тёзкой нельзя,
// а любой ДРУГОЙ носитель прежнего имени делает тег неоднозначным.
func TestResetDetourNodeRefs_TagOnlyRefSurvivesRenameOfNamesake(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		// Переименованный источник (тег уже новый).
		serverSource("01WARP", "hop-renamed", ""),
		// Живой тёзка прежнего имени.
		serverSource("01TWIN", "hop", ""),
		subSource("01PROTON", "Proton NL", &wizardmodels.NodeLink{Tag: "hop"}),
	}}

	affected := ResetDetourNodeRefs(m, "01WARP", "hop")
	if len(affected) != 0 {
		t.Fatalf("ссылка на живого тёзку сброшена: %v", affected)
	}
	if d := m.Sources[2].Detour; d == nil || d.Tag != "hop" {
		t.Error("ссылка на живого тёзку стёрта")
	}
}

// Тёзки нет — ссылка на прежнее имя мертва и гаснет.
func TestResetDetourNodeRefs_TagOnlyRefClearedAfterRenameWithoutNamesake(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		serverSource("01WARP", "hop-renamed", ""),
		subSource("01PROTON", "Proton NL", &wizardmodels.NodeLink{Tag: "hop"}),
	}}

	if affected := ResetDetourNodeRefs(m, "01WARP", "hop"); len(affected) != 1 {
		t.Fatalf("мёртвая ссылка обязана погаснуть, затронуто %v", affected)
	}
	if m.Sources[1].Detour != nil {
		t.Error("мёртвая ссылка не погасла")
	}
}
