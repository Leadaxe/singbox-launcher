// SPEC 118 §4.B.10 + §4.E.7 — единый гард занятости тегов на стороне модели.
//
// Ключевой сценарий: на МИГРИРОВАННОМ состоянии сброс осиротевших целей
// правил не стреляет по replace-тегам и верхним узлам. Проверяется на
// множестве известных целей (KnownRuleTargetTags) — это и есть тот вход,
// по которому resetForeignRuleTargets принимает решение.
package business

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// migratedLikeModel — модель в форме, которую даёт миграция v6→v7: подписка
// со свёрткой both (материализованный тег `[P]select` + двойник
// `[P]select-auto`), верхний server-узел и Направление с автовыбором.
func migratedLikeModel() *wizardmodels.WizardModel {
	m := wizardmodels.NewWizardModel()
	sub := corestate.Source{
		Node: corestate.Node{Kind: corestate.SourceKindSubscription, Enabled: true},
		ID:   "01J00000000000000000000SUB",
		Name: "Proton NL",
		URL:  "https://example.invalid/sub",
		Replace: &corestate.FolderReplace{
			Mode: corestate.FolderReplaceBoth,
			Tag:  "[P]select",
		},
	}
	srv := corestate.Source{
		Node: corestate.Node{Kind: corestate.SourceKindServer, Tag: "Tokyo", Enabled: true},
		ID:   "01J00000000000000000000SRV",
	}
	chain := corestate.Source{
		Node: corestate.Node{Kind: corestate.SourceKindChain, Tag: "chain-1", Enabled: true},
		ID:   "01J0000000000000000000CHN0",
	}
	m.Sources = []corestate.Source{sub, srv, chain}
	m.GlobalOutbounds = []configtypes.Direction{
		{Tag: "video-out", Type: "selector", Auto: &configtypes.DirectionAuto{Interval: "3m"}},
		{Tag: "paused-out", Type: "selector", Disabled: true},
	}
	return m
}

func TestKnownRuleTargets_KeepsReplaceTagsAndRootNodes(t *testing.T) {
	known := KnownRuleTargetTags(migratedLikeModel())

	// §4.B.10: правило, целившееся в fold-тег, после миграции целится в
	// replace-тег. Не знай его reset — и живое правило ушло бы в direct
	// необратимо.
	for _, tag := range []string{"[P]select", "[P]select-auto"} {
		if !known[tag] {
			t.Errorf("replace-тег %q не в известных целях — правило было бы сброшено на direct", tag)
		}
	}
	// Верхние узлы: цепочка и сервер — законные цели ссылок.
	for _, tag := range []string{"Tokyo", "chain-1"} {
		if !known[tag] {
			t.Errorf("верхний узел %q не в известных целях", tag)
		}
	}
	// Твин Направления и выключенное Направление тоже известны (§4.E.9).
	for _, tag := range []string{"video-out", "video-out-auto", "paused-out"} {
		if !known[tag] {
			t.Errorf("тег %q не в известных целях", tag)
		}
	}
	// Чужой тег известным не становится — иначе сброс перестал бы работать.
	if known["ghost-out"] {
		t.Error("несуществующая цель попала в известные — сброс осиротевших правил не сработает")
	}
}

func TestModelTagOwners_NamesTheOwnerKind(t *testing.T) {
	owners := ModelTagOwners(migratedLikeModel())
	cases := map[string]string{
		"video-out":      "Direction",
		"video-out-auto": "Direction auto group",
		"[P]select":      "folder replacement",
		"[P]select-auto": "folder replacement",
		"Tokyo":          "node",
	}
	for tag, want := range cases {
		if got := owners[tag]; got != want {
			t.Errorf("владелец %q = %q, want %q", tag, got, want)
		}
	}
}

// §4.E.7 — Направление `x` и папка с заменой `x` (или `x-auto`) не могут
// сосуществовать: два `x-auto` дали бы ядру дубль тега.
func TestDirectionTagTaken_CollidesWithReplaceTag(t *testing.T) {
	m := migratedLikeModel()

	if !DirectionTagTaken(m, "[P]select", "") {
		t.Error("тег замены не считается занятым — Направление смогло бы его отобрать")
	}
	// Направление `[P]select-auto` столкнулось бы с двойником замены.
	if !DirectionTagTaken(m, "[P]select-auto", "") {
		t.Error("двойник замены не считается занятым")
	}
	// Направление `x`, чей твин `x-auto` совпал бы с чужим тегом замены.
	m.Sources[0].Replace.Tag = "x"
	if !DirectionTagTaken(m, "x", "") {
		t.Error("столкновение Направление x + замена x не поймано")
	}
	// Верхний узел тоже занят.
	if !DirectionTagTaken(m, "Tokyo", "") {
		t.Error("тег верхнего узла не считается занятым")
	}
	// Своё же имя не занято: открыть форму и сохранить — обычный сценарий.
	if DirectionTagTaken(m, "video-out", "video-out") {
		t.Error("собственный тег Направления объявлен занятым")
	}
	// Свободное имя остаётся свободным.
	if DirectionTagTaken(m, "brand-new-out", "") {
		t.Error("свободный тег объявлен занятым")
	}
}
