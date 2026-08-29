// File tag_guard.go — ЕДИНЫЙ гард занятости тегов (SPEC 118 W4, Т5;
// features/directions.md §8).
//
// Занятость тега проверяется одним множеством на всю сборку и на все
// операции именования. В него входят одновременно:
//
//   - теги Направлений и их твины `<tag>-auto`;
//   - replace-теги папок и их `-auto`-двойники (режим both);
//   - теги верхних узлов;
//   - системные теги шаблона (direct/block-outbound'ы и пр.).
//
// Зачем одно множество, а не проверка на месте: Направление `x` и папка с
// replace-тегом `x` дали бы ДВА `x-auto`, и ядро отвергло бы весь конфиг за
// дубль тега. Частная проверка «занят ли тег среди Направлений» этого не
// видит по построению — она не знает про замены. Гард живёт здесь, и любой
// новый вид тега обязан появиться сначала в нём (то же правило, что у
// реестра переписи ссылок).
package config

import (
	"sort"
	"strings"

	"singbox-launcher/internal/locale"
)

// TagOwnerKind — чей это тег (для внятного сообщения об отказе).
type TagOwnerKind string

// Значения — АНГЛИЙСКИЕ ключи локали (SPEC 116 W12, фикс 2): владелец
// подставляется в текст предупреждения и обязан переводиться вместе с ним.
const (
	TagOwnerDirection TagOwnerKind = "a Direction"
	TagOwnerTwin      TagOwnerKind = "a Direction auto-group"
	TagOwnerReplace   TagOwnerKind = "a folder replacement"
	TagOwnerNode      TagOwnerKind = "a node"
	TagOwnerSystem    TagOwnerKind = "a template system tag"
)

// Localized — вид владельца на языке пользователя.
func (k TagOwnerKind) Localized() string { return locale.T(string(k)) }

// TagGuard — множество занятых тегов с указанием владельца.
type TagGuard struct {
	owners map[string]TagOwnerKind
	// conflicts — теги, на которые претендовали двое; заполняется при
	// построении и отдаётся вызывающему как отказ сборки.
	//
	// Записью, а не строкой (SPEC 116 W12, фикс 3): отчёт обязан знать САМ
	// ТЕГ, чтобы поставить ⚠ у виновной строки, — из готовой фразы его
	// пришлось бы выковыривать регуляркой.
	conflicts []TagConflict
}

// TagConflict — двое на один тег.
type TagConflict struct {
	Tag  string
	Prev TagOwnerKind
	Kind TagOwnerKind
}

// Text — фраза для человека (переведённая).
func (c TagConflict) Text() string {
	return locale.Tf(emitTagConflictText, c.Tag, c.Prev.Localized(), c.Kind.Localized())
}

// NewTagGuard — пустой гард.
func NewTagGuard() *TagGuard {
	return &TagGuard{owners: make(map[string]TagOwnerKind, 32)}
}

// Claim занимает тег. Второй претендент на тот же тег фиксируется конфликтом
// (первый остаётся владельцем — резолв обязан быть детерминированным).
func (g *TagGuard) Claim(tag string, kind TagOwnerKind) {
	tag = strings.TrimSpace(tag)
	if tag == "" || g == nil {
		return
	}
	if prev, taken := g.owners[tag]; taken {
		if prev == kind && kind == TagOwnerSystem {
			return // системные теги приходят из нескольких секций шаблона
		}
		g.conflicts = append(g.conflicts, TagConflict{Tag: tag, Prev: prev, Kind: kind})
		return
	}
	g.owners[tag] = kind
}

// Taken — занят ли тег (кем угодно).
func (g *TagGuard) Taken(tag string) bool {
	if g == nil {
		return false
	}
	_, ok := g.owners[strings.TrimSpace(tag)]
	return ok
}

// Owner — вид владельца тега ("" — свободен).
func (g *TagGuard) Owner(tag string) TagOwnerKind {
	if g == nil {
		return ""
	}
	return g.owners[strings.TrimSpace(tag)]
}

// Conflicts — все столкновения, найденные при построении.
func (g *TagGuard) Conflicts() []TagConflict {
	if g == nil {
		return nil
	}
	return g.conflicts
}

// ConflictTexts — те же столкновения фразами (лог, тесты, копия отчёта).
func (g *TagGuard) ConflictTexts() []string {
	if g == nil || len(g.conflicts) == 0 {
		return nil
	}
	out := make([]string, 0, len(g.conflicts))
	for _, c := range g.conflicts {
		out = append(out, c.Text())
	}
	return out
}

// Tags — отсортированный список занятых тегов (детерминизм для реестров и
// тестов).
func (g *TagGuard) Tags() []string {
	if g == nil {
		return nil
	}
	out := make([]string, 0, len(g.owners))
	for tag := range g.owners {
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

// BuildTagGuard собирает гард по сборочной форме.
//
// directions — Направления (их твины добавляются по формуле `<tag>-auto`);
// proxies — источники (даёт replace-теги и верхние узлы); systemTags —
// объявления шаблона (direct-out, block-out, теги пресетов).
//
// rootNodeTags — финальные теги ВЕРХНИХ узлов; они известны только после
// прохода 1, поэтому передаются отдельно, а не выводятся из proxies.
func BuildTagGuard(directions []Direction, proxies []ProxySource, rootNodeTags []string, systemTags []string) *TagGuard {
	g := NewTagGuard()

	for _, tag := range systemTags {
		g.Claim(tag, TagOwnerSystem)
	}
	for _, tag := range rootNodeTags {
		g.Claim(tag, TagOwnerNode)
	}
	for i := range proxies {
		cs := proxies[i].Canonical
		if cs == nil || cs.Replace == nil {
			continue
		}
		for _, tag := range FolderReplaceTags(cs.Replace) {
			g.Claim(tag, TagOwnerReplace)
		}
	}
	// Твины и родители дедуплицируются по ВЛАДЕЛЬЦУ, а не по тегу.
	//
	// Гард строится по СБОРОЧНОЙ форме — то есть уже ПОСЛЕ
	// `ExpandDirectionTwins`, и в списке лежат обе половины пары: сама
	// auto-группа отдельной записью (`TwinOf` = тег родителя) и родитель с
	// `TwinTag`. Наивная формула `d.Tag+twinSuffix` объявляла бы вторым
	// претендентом на `x-auto` ту же самую сущность, и КАЖДОЕ Направление с
	// автовыбором давало ложное «тег занят дважды: Направление и
	// авто-группа Направления».
	//
	// Тот же случай — шаблонная отдельно стоящая `x-auto`: `ExpandDirectionTwins`
	// видит тег занятым, твина НЕ создаёт и оставляет `TwinTag` пустым
	// (`direction_twins.go:98`). Претендент здесь тоже один.
	// Тег, за которым в списке УЖЕ стоит своя запись, производной формулой
	// второй раз не занимается: владелец один и тот же.
	twinTags := make(map[string]bool, len(directions)*2)
	for i := range directions {
		if t := strings.TrimSpace(directions[i].TwinTag); t != "" {
			twinTags[t] = true
		}
		if t := strings.TrimSpace(directions[i].Tag); t != "" {
			twinTags[t] = true
		}
	}
	for i := range directions {
		d := directions[i]
		if d.Tag == "" {
			// Выключенное Направление в конфиг не идёт, но имя за собой
			// держит: включить обратно оно обязано без коллизии.
			continue
		}
		if strings.TrimSpace(d.TwinOf) != "" {
			// Развёрнутая auto-группа: она и есть твин родителя.
			g.Claim(d.Tag, TagOwnerTwin)
			continue
		}
		g.Claim(d.Tag, TagOwnerDirection)
		if d.Auto == nil {
			continue
		}
		// Пара уже развёрнута: тег твина занят его собственной записью
		// (либо шаблонной одноимённой группой) — второй раз не претендуем.
		if twin := d.Tag + twinSuffix; !twinTags[twin] {
			g.Claim(twin, TagOwnerTwin)
		}
	}
	return g
}

// KnownTargetTags — множество ИЗВЕСТНЫХ целей ссылок: всё из гарда плюс
// addOutbounds Направлений.
//
// Адресат — реестр переписи и сброс осиротевших целей правил
// (`rule_target_reset`): он обязан знать ВСЕ виды тегов из гарда, иначе
// первая же загрузка приняла бы replace-теги за чужие и сбросила живые
// правила на direct (deps-К2, SPEC §4.B.10).
func KnownTargetTags(guard *TagGuard, directions []Direction) []string {
	seen := make(map[string]struct{}, 64)
	var out []string
	add := func(tag string) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return
		}
		if _, dup := seen[tag]; dup {
			return
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	for _, tag := range guard.Tags() {
		add(tag)
	}
	for i := range directions {
		for _, extra := range directions[i].AddOutbounds {
			add(extra)
		}
	}
	sort.Strings(out)
	return out
}
