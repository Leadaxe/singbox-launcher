// File source_chain_candidates_test.go — пул кандидатов позиций цепочки
// (SPEC 118 Т8, поведенческие проверки W6).
//
// Позиция цепочки — ССЫЛКА (NodeLink), а не строка. Узел ПАПКИ адресуется
// парой «id папки + сырой тег», узел верхнего уровня — своим финальным тегом,
// свёрнутая папка — только тегами замены. Разница не формальная: финальный
// тег узла папки считается её тег-политикой на каждой сборке, и корневая
// ссылка на него ведёт в никуда — цепочка деградирует fail-closed.
package tabs

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// modelWithChainTargets — модель из трёх видов целей: подписка (папочное
// пространство), свёрнутая подписка (только замены) и верхний узел.
func modelWithChainTargets() *wizardmodels.WizardModel {
	m := &wizardmodels.WizardModel{
		Sources: []wizardmodels.Source{
			// 0 — обычная подписка: её узлы адресуются папочной ссылкой.
			{Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
				ID: "SUB1", Name: "nl",
				TagPolicy: &corestate.TagPolicy{Prefix: "[NL] "},
				Nodes: []wizardmodels.Node{
					{Kind: wizardmodels.SourceKindServer, Tag: "n1", Enabled: true},
				}},
			// 1 — СВЁРНУТАЯ подписка: конфиг видит только теги замены.
			{Node: wizardmodels.Node{Kind: wizardmodels.SourceKindSubscription, Enabled: true},
				ID: "SUB2", Name: "de",
				Replace: &corestate.FolderReplace{Mode: corestate.FolderReplaceBoth, Tag: "de"},
				Nodes: []wizardmodels.Node{
					{Kind: wizardmodels.SourceKindServer, Tag: "d1", Enabled: true},
				}},
			// 2 — верхний узел: его финальный тег и есть адрес в корне.
			{Node: wizardmodels.Node{Kind: wizardmodels.SourceKindServer, Tag: "warp", Enabled: true},
				ID: "SRV"},
		},
		GlobalOutbounds: []configtypes.Direction{{Tag: "vpn"}},
	}
	// Пул — как его собрал бы RebuildNodePool: финальные теги от эмиссии,
	// IdentityTag — сырой тег в контейнере, SourceIndex — чей узел.
	m.NodePool = []*configtypes.ParsedNode{
		{Tag: "[NL] n1", IdentityTag: "n1", SourceIndex: 0},
		{Tag: "d1", IdentityTag: "d1", SourceIndex: 1},
		{Tag: "warp", IdentityTag: "warp", SourceIndex: 2},
	}
	return m
}

func candidateByTag(cands []chainHopCandidate, tag string) (chainHopCandidate, bool) {
	for _, c := range cands {
		if c.Tag == tag {
			return c, true
		}
	}
	return chainHopCandidate{}, false
}

func TestChainHopCandidatesAddressFolderNodesByLink(t *testing.T) {
	cands := collectChainHopCandidates(modelWithChainTargets(), "self")

	// Узел подписки: показан ФИНАЛЬНЫМ тегом, а адресуется папочной ссылкой.
	nl, ok := candidateByTag(cands, "[NL] n1")
	if !ok {
		t.Fatalf("узел подписки не предложен: %+v", cands)
	}
	if nl.Link.FolderID != "SUB1" || nl.Link.Tag != "n1" {
		t.Errorf("ссылка на узел подписки = %+v, ожидали {SUB1, n1} — "+
			"корневая ссылка на финальный тег протухнет от правки префикса", nl.Link)
	}

	// Верхний узел: корневая ссылка, тег и есть адрес.
	warp, ok := candidateByTag(cands, "warp")
	if !ok {
		t.Fatal("верхний узел не предложен")
	}
	if warp.Link.FolderID != "" || warp.Link.Tag != "warp" {
		t.Errorf("ссылка на верхний узел = %+v, ожидали корневую {\"\", warp}", warp.Link)
	}

	// Направление — корневая ссылка.
	if d, ok := candidateByTag(cands, "vpn"); !ok || d.Kind != hopKindDirection || d.Link.FolderID != "" {
		t.Errorf("Направление предложено неверно: %+v (ok=%v)", d, ok)
	}
}

// Свёрнутая папка представлена ТОЛЬКО тегами замены: её узлы под своими
// именами конфиг не видит, и предложить их значило бы вести к fail-closed.
func TestChainHopCandidatesFoldedFolderOffersOnlyReplaceTags(t *testing.T) {
	cands := collectChainHopCandidates(modelWithChainTargets(), "self")

	if _, ok := candidateByTag(cands, "d1"); ok {
		t.Error("узел СВЁРНУТОЙ папки предложен позицией — в конфиге его тега нет")
	}
	// both → селектор + `-auto`-двойник (формула твинов).
	for _, want := range []string{"de", "de-auto"} {
		c, ok := candidateByTag(cands, want)
		if !ok {
			t.Errorf("тег замены %q не предложен", want)
			continue
		}
		if c.Link.FolderID != "" || c.Link.Tag != want {
			t.Errorf("тег замены %q адресован не корневой ссылкой: %+v", want, c.Link)
		}
	}
}

// Сам себя цепочка позицией не берёт: ядро отвергает такую цепочку целиком.
func TestChainHopCandidatesExcludeSelf(t *testing.T) {
	m := modelWithChainTargets()
	if _, ok := candidateByTag(collectChainHopCandidates(m, "warp"), "warp"); ok {
		t.Error("цепочка предложена позицией самой себе")
	}
}

// Ссылки, приехавшие из JSON-вкладки финальными тегами, разворачиваются по
// кандидатам: тег узла папки обязан стать ПАПОЧНОЙ ссылкой, а не корневой.
func TestChainLinksFromTagsResolvesFolderNodes(t *testing.T) {
	f := &chainForm{cands: collectChainHopCandidates(modelWithChainTargets(), "self")}

	got := chainLinksFromTags(f, []string{"[NL] n1", "warp", "unknown-tag"})
	if len(got) != 3 {
		t.Fatalf("позиций = %d, ожидали 3", len(got))
	}
	if got[0].FolderID != "SUB1" || got[0].Tag != "n1" {
		t.Errorf("узел папки из JSON = %+v, ожидали {SUB1, n1}", got[0])
	}
	if got[1].FolderID != "" || got[1].Tag != "warp" {
		t.Errorf("верхний узел из JSON = %+v, ожидали корневую", got[1])
	}
	// Тег без кандидата остаётся корневой ссылкой: пользователь вправе
	// вписать цель, которой ещё нет, и переписывать его ввод форма не должна.
	if got[2].FolderID != "" || got[2].Tag != "unknown-tag" {
		t.Errorf("неизвестный тег = %+v, ожидали корневую ссылку как есть", got[2])
	}
}

// Форма отдаёт позиции модели ссылками, а эмиттеру — финальными тегами.
// Разъедься эти две стороны — и сохранение писало бы в состояние теги, а
// превью JSON показывало бы ссылки.
func TestChainFormCollectsLinksAndTags(t *testing.T) {
	m := modelWithChainTargets()
	cands := collectChainHopCandidates(m, "self")
	f := &chainForm{cands: cands, lookup: chainHopLookup(cands)}
	f.hops = []corestate.NodeLink{{FolderID: "SUB1", Tag: "n1"}, {Tag: "warp"}}

	links := f.CollectLinks()
	if len(links) != 2 || links[0].FolderID != "SUB1" || links[0].Tag != "n1" {
		t.Fatalf("позиции для модели = %+v", links)
	}

	c := f.Collect()
	if c == nil || len(c.Hops) != 2 {
		t.Fatalf("позиции для эмиттера = %+v", c)
	}
	if c.Hops[0] != "[NL] n1" {
		t.Errorf("позиция для эмиттера = %q, ожидали финальный тег «[NL] n1»", c.Hops[0])
	}
	if c.Hops[1] != "warp" {
		t.Errorf("позиция для эмиттера = %q, ожидали «warp»", c.Hops[1])
	}
}

// Сохранение кладёт в узел ИМЕННО ссылки. Прежняя версия складывала позиции
// корневыми ссылками по финальному тегу и рвала хопы на узлы папок: ссылка
// {SUB1, n1} превращалась в {"", "[NL] n1"} — тег, за которым в корне никого
// нет, — и цепочка молча деградировала на первой же пересборке.
func TestApplyChainFormKeepsFolderLinks(t *testing.T) {
	src := &wizardmodels.Source{Node: wizardmodels.Node{
		Kind: wizardmodels.SourceKindChain, Tag: "ch", Enabled: true}}
	hops := []corestate.NodeLink{{FolderID: "SUB1", Tag: "n1"}, {Tag: "warp"}}

	applyChainFormToSource(src, &configtypes.SourceChain{
		Hops: []string{"[NL] n1", "warp"}, IdleTimeout: "2m"}, hops)

	if len(src.Hops) != 2 {
		t.Fatalf("позиций в узле = %d", len(src.Hops))
	}
	if src.Hops[0].FolderID != "SUB1" || src.Hops[0].Tag != "n1" {
		t.Errorf("папочная ссылка потеряна на сохранении: %+v", src.Hops[0])
	}
	// Настройки маршрута живут в теле — тот же дом, что у сервера.
	back := configtypes.ChainFromBody(src.Body, nil)
	if back == nil || back.IdleTimeout != "2m" {
		t.Errorf("настройки маршрута потеряны: %+v", back)
	}
}
