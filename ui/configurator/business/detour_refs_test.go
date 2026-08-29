package business

import (
	"testing"

	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 112-A, «Смена идентичности узла = сброс ссылок с предупреждением».

func modelWithHopAndDependents(hopID, hopTag string) *wizardmodels.WizardModel {
	return &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		{ID: hopID, Node: wizardmodels.Node{Kind: wizardmodels.SourceKindServer, Enabled: true},
			Label: "WARP hop", NodeTag: hopTag, URI: detourTestServerURI},
		{ID: "01PROTON", Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
			Label: "Proton NL", URL: "https://example.com/proton",
			DetourNodeSourceID: hopID, DetourNodeTag: hopTag, DetourNodeLabel: "WARP hop"},
		{ID: "01OTHER", Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
			Label: "Liberty", URL: "https://example.com/liberty"},
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
	got := m.Sources[1]
	if got.DetourNodeSourceID != "" || got.DetourNodeTag != "" ||
		got.DetourNodeLabel != "" || got.DetourNodeHash != "" {
		t.Errorf("ссылка обязана погаснуть целиком, осталось %+v", got)
	}
	if m.Sources[2].DetourNodeTag != "" {
		t.Error("чужой источник тронут не был")
	}
}

// Ссылка на ДРУГОЙ узел того же источника переименования не заметила: у
// подписки много узлов, и смена имени одного не касается остальных.
func TestResetDetourNodeRefs_KeepsSiblingNodeRef(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		{ID: "01LIB", Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
			Label: "AL: Liberty", URL: "https://example.com/liberty"},
		{ID: "01PROTON", Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
			Label: "Proton NL", DetourNodeSourceID: "01LIB", DetourNodeTag: "🇳🇱 Amsterdam-2"},
	}}

	if affected := ResetDetourNodeRefs(m, "01LIB", "🇳🇱 Amsterdam-1"); len(affected) != 0 {
		t.Fatalf("ссылка на соседний узел трогаться не должна, затронуто %v", affected)
	}
	if m.Sources[1].DetourNodeTag != "🇳🇱 Amsterdam-2" {
		t.Error("ссылка на соседний узел стёрта")
	}
}

// Переходная ссылка без source_id гасится, только когда имя однозначно
// принадлежало этому узлу.
func TestResetDetourNodeRefs_TagOnlyRefOnlyWhenUnambiguous(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		{ID: "01WARP", Node: wizardmodels.Node{Kind: wizardmodels.SourceKindServer, Enabled: true}, NodeTag: "hop"},
		{ID: "01PROTON", Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
			Label: "Proton NL", DetourNodeTag: "hop"},
	}}
	if affected := ResetDetourNodeRefs(m, "01WARP", "hop"); len(affected) != 1 {
		t.Fatalf("однозначная tag-only ссылка обязана гаснуть, затронуто %v", affected)
	}

	// Тёзка: двух серверов с одним тегом отличить нечем — ссылку не трогаем,
	// чтобы не погасить чужую.
	m2 := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		{ID: "01WARP", Node: wizardmodels.Node{Kind: wizardmodels.SourceKindServer, Enabled: true}, NodeTag: "hop"},
		{ID: "01TWIN", Node: wizardmodels.Node{Kind: wizardmodels.SourceKindServer, Enabled: true}, NodeTag: "hop"},
		{ID: "01PROTON", Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
			Label: "Proton NL", DetourNodeTag: "hop"},
	}}
	if affected := ResetDetourNodeRefs(m2, "01WARP", "hop"); len(affected) != 0 {
		t.Fatalf("неоднозначная tag-only ссылка трогаться не должна, затронуто %v", affected)
	}
}

// SPEC 113-E, регресс: уникальность тега считается ДО перезаписи модели.
//
// Сброс зовётся УЖЕ после того, как форма записала новое имя. На старом коде
// подсчёт шёл по изменённой модели: переименованный источник тег сменил, тёзка
// оставался один — тег объявлялся уникальным, и tag-only ссылка на ТЁЗКУ
// гасла, хотя его никто не переименовывал.
func TestResetDetourNodeRefs_TagOnlyRefSurvivesRenameOfNamesake(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		// 01WARP уже переименован формой: в модели новый тег, сброс зовут со
		// старым.
		{ID: "01WARP", Node: wizardmodels.Node{Kind: wizardmodels.SourceKindServer, Enabled: true}, NodeTag: "hop-renamed"},
		// Тёзка, которого никто не трогал: ссылка ниже могла вести к нему.
		{ID: "01TWIN", Node: wizardmodels.Node{Kind: wizardmodels.SourceKindServer, Enabled: true}, NodeTag: "hop"},
		{ID: "01PROTON", Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
			Label: "Proton NL", DetourNodeTag: "hop"},
	}}

	affected := ResetDetourNodeRefs(m, "01WARP", "hop")

	if len(affected) != 0 {
		t.Fatalf("неоднозначная tag-only ссылка трогаться не должна, затронуто %v", affected)
	}
	if m.Sources[2].DetourNodeTag != "hop" {
		t.Errorf("ссылка на живого тёзку стёрта: %+v", m.Sources[2])
	}
}

// Обратная сторона того же правила: тёзки нет — ссылка однозначно вела на
// переименованный узел и обязана погаснуть, даже когда тег в модели уже новый.
func TestResetDetourNodeRefs_TagOnlyRefClearedAfterRenameWithoutNamesake(t *testing.T) {
	m := &wizardmodels.WizardModel{Sources: []wizardmodels.Source{
		{ID: "01WARP", Node: wizardmodels.Node{Kind: wizardmodels.SourceKindServer, Enabled: true}, NodeTag: "hop-renamed"},
		{ID: "01PROTON", Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
			Label: "Proton NL", DetourNodeTag: "hop"},
	}}

	if affected := ResetDetourNodeRefs(m, "01WARP", "hop"); len(affected) != 1 {
		t.Fatalf("однозначная tag-only ссылка обязана гаснуть, затронуто %v", affected)
	}
	if m.Sources[1].DetourNodeTag != "" {
		t.Errorf("ссылка не погасла: %+v", m.Sources[1])
	}
}

// Пикер дорезолвивает переходную ссылку (тег без source_id) до полного ref'а:
// цель однозначна, и сохранение формы запишет уже объект.
func TestDetourOptionsWithNodes_UpgradesTagOnlyRef(t *testing.T) {
	m := modelWithServerSource(t, "WARP hop", detourTestServerURI)
	src := &wizardmodels.Source{DetourNodeTag: "WARP hop"} // без source_id

	_, sel, choices := DetourOptionsWithNodes(m, src, none)
	if sel != detourNodeMarker+"WARP hop" {
		t.Fatalf("переходная ссылка обязана показаться выбранной, sel=%q", sel)
	}
	if choices[sel].NodeSourceID != "01SRV0000000000000000000" {
		t.Errorf("пикер обязан дорезолвить source_id, получено %+v", choices[sel])
	}
}

// Полная ссылка на переименованный узел выбранной НЕ считается: резолв на
// сборке строгий, и показать её живой значило бы соврать.
func TestDetourOptionsWithNodes_RenamedTargetShowsDangling(t *testing.T) {
	m := modelWithServerSource(t, "WARP hop", detourTestServerURI)
	m.Sources[0].NodeTag = "hop-renamed"
	src := &wizardmodels.Source{
		DetourNodeSourceID: "01SRV0000000000000000000",
		DetourNodeTag:      "hop-was",
		DetourNodeLabel:    "WARP hop",
	}

	opts, sel, choices := DetourOptionsWithNodes(m, src, none)
	if choices[sel].NodeTag != "hop-was" {
		t.Errorf("выбранной обязана остаться повисшая ссылка, получено %+v", choices[sel])
	}
	// Живой узел при этом всё равно предлагается как отдельная опция.
	if !contains(opts, detourNodeMarker+"WARP hop") {
		t.Errorf("живой узел обязан остаться в списке, получено %v", opts)
	}
}
