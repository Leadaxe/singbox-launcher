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
	"fmt"
	"sort"
	"strings"
)

// TagOwnerKind — чей это тег (для внятного сообщения об отказе).
type TagOwnerKind string

const (
	TagOwnerDirection TagOwnerKind = "Направление"
	TagOwnerTwin      TagOwnerKind = "авто-группа Направления"
	TagOwnerReplace   TagOwnerKind = "замена папки"
	TagOwnerNode      TagOwnerKind = "узел"
	TagOwnerSystem    TagOwnerKind = "системный тег шаблона"
)

// TagGuard — множество занятых тегов с указанием владельца.
type TagGuard struct {
	owners map[string]TagOwnerKind
	// conflicts — теги, на которые претендовали двое; заполняется при
	// построении и отдаётся вызывающему как отказ сборки.
	conflicts []string
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
		g.conflicts = append(g.conflicts, fmt.Sprintf("тег %q занят дважды: %s и %s", tag, prev, kind))
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
func (g *TagGuard) Conflicts() []string {
	if g == nil {
		return nil
	}
	return g.conflicts
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
	for i := range directions {
		d := directions[i]
		if d.Tag == "" || d.Disabled {
			// Выключенное Направление в конфиг не идёт, но имя за собой
			// держит: включить обратно оно обязано без коллизии.
			if d.Tag == "" {
				continue
			}
		}
		g.Claim(d.Tag, TagOwnerDirection)
		if d.Auto != nil {
			g.Claim(d.Tag+twinSuffix, TagOwnerTwin)
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
