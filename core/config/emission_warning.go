// File emission_warning.go — деградация эмиссии С АДРЕСАТОМ (SPEC 116 W12,
// фикс 3).
//
// До этой волны эмиссия отдавала предупреждения голыми строками, и отчёт
// «Итога» клал их под субъектом "emission". Пользователь читал «узел "NL-1"
// исключён: цель detour не разрешилась» и шёл искать, у какого из сорока
// источников этот узел живёт, — притом что в момент рождения предупреждения
// источник был известен точно. Подписки такой проблемы не имели: их
// деградации приезжают с `SourceID` и красят строку списка ⚠ (`fetch_degraded`).
//
// Поэтому предупреждение — запись, а не строка: `Text` для человека,
// `SourceID`/`SourceLabel` — чтобы строка Sources встала с ⚠ у виновника, и
// `DirectionTag` — для деградаций, у которых источника нет вовсе
// (столкновение тегов Направлений живёт на вкладке Направлений).
//
// ЛОКАЛИЗАЦИЯ (фикс 2): `Text` собирается через `locale.Tf` с АНГЛИЙСКИМ
// ключом-текстом — тот же механизм, что во всём UI (ключ = английская фраза,
// перевод в `bin/locale/ru.json`). Раньше эти фразы были захардкожены
// по-русски прямо в движке и не переводились никогда.
package config

import (
	"fmt"
	"strings"

	"singbox-launcher/core/config/registry"
	"singbox-launcher/internal/locale"
)

// Фразы эмиссии: ключ локали = АНГЛИЙСКИЙ текст (общий механизм проекта,
// перевод — `bin/locale/ru.json`). До SPEC 116 W12 они были русскими
// литералами прямо в движке и не переводились ни для одного языка.
//
// Константами, а не литералами по месту: ключ обязан совпадать с записью в
// каталоге байт-в-байт, а одна и та же фраза встречается и в резолве, и в
// тесте.
const (
	emitLinkEmptyText         = "the reference is empty"
	emitLinkSourceMissingText = "the referenced source is gone"
	emitLinkNodeMissingText   = "it has no node %q"
	emitLinkGroupFinalTagText = "it has no node %q — this looks like the final tag of group %q; pick the target again"
	emitLinkTargetUnknownText = "target %q is not among nodes, Directions and folder replacements"

	emitDetourUnresolvedText    = "node %q dropped: the detour target did not resolve (%s) — a hop set up for anonymity may not silently become a direct dial"
	emitDetourTargetDroppedText = "node %q dropped: its detour target %q fell out of the config itself"
	emitDetourSelfText          = "node %q dropped: its detour points at itself"
	emitDetourCycleText         = "node %q dropped: detour loop — the chain of hops leads traffic back to itself"

	emitChainHopUnresolvedText = "chain %q: position %q did not resolve (%s)"
	emitNodeNotEmittableText   = "node %q is not emittable — dropped: %v"

	emitTagConflictText = "tag %q is claimed twice: %s and %s"
)

// Коды реестра (contract/registry/warnings.json), которые ставит сборка.
const (
	codeSourceDetourMissing        = "source_detour_missing"
	codeSourceDetourSelf           = "source_detour_self"
	codeSourceDetourCycle          = "source_detour_cycle"
	codeGroupEmpty                 = "group_empty"
	codeGroupMemberDropped         = "group_member_dropped"
	codeChainUnsupportedByCore     = "chain_unsupported_by_core"
	codeChainInvalid               = "chain_invalid"
	codeChainHopMissing            = "chain_hop_missing"
	codeChainNestedPosition        = "chain_nested_position"
	codeChainCycleThroughDirection = "chain_cycle_through_direction"
)

// EmissionWarning — одна деградация эмиссии.
//
// Пустые SourceID и DirectionTag законны: не у всякой деградации есть
// адресат в состоянии (узел из ручного outbound шаблона). Такая запись
// остаётся годной для отчёта, но красить ею нечего.
type EmissionWarning struct {
	// Text — уже переведённая фраза для человека.
	Text string
	// SourceID / SourceLabel — источник-виновник (ULID и человеческое имя).
	SourceID    string
	SourceLabel string
	// DirectionTag — Направление-виновник, если источника нет.
	DirectionTag string
	// Code / Params — код реестра (warnings.json) и его подстановки, если
	// деградации код назначен. UI отчёта переводит запись по коду; Text
	// остаётся запасным текстом (лог, строка Sources).
	Code   string
	Params map[string]string
}

// registryWarningText — текст кода реестра на текущем языке UI с
// подстановками; fallback — если кода в реестре нет или он не прочитался.
func registryWarningText(code string, params map[string]string, fallback string) string {
	reg, err := registry.Get()
	if err != nil {
		return fallback
	}
	_, text, ok := reg.WarningText(code, locale.GetLang(), params)
	if !ok || strings.TrimSpace(text) == "" {
		return fallback
	}
	return text
}

// String — фраза без адресата: для лога и для мест, которым нужен просто текст.
func (w EmissionWarning) String() string { return w.Text }

// EmissionWarningTexts — только фразы, в порядке записей.
//
// Нужен там, где адресат уже учтён отдельно (`sourceParseFailure` собирает
// причины ОДНОГО источника) или не нужен вовсе (лог).
func EmissionWarningTexts(ws []EmissionWarning) []string {
	if len(ws) == 0 {
		return nil
	}
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.Text)
	}
	return out
}

// sourceOfNodeTag — источник, чей узел носит финальный тег.
//
// Нужен столкновению тегов: гард знает только сам тег, а строку в списке
// красят по ULID источника.
func sourceOfNodeTag(proxies []ProxySource, nodesBySource map[int][]*ParsedNode, tag string) (ProxySource, int, bool) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ProxySource{}, -1, false
	}
	for i := range proxies {
		for _, n := range nodesBySource[i] {
			if n != nil && n.Tag == tag {
				return proxies[i], i, true
			}
		}
	}
	return ProxySource{}, -1, false
}

// directionOwningTag — Направление, которому принадлежит тег (само или его
// твин); "" — ничьё.
func directionOwningTag(directions []Direction, tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	for i := range directions {
		d := directions[i]
		if d.Tag == tag {
			// У развёрнутого твина чинить надо родителя: сам твин —
			// производная его настройки, отдельной записи у пользователя нет.
			if p := strings.TrimSpace(d.TwinOf); p != "" {
				return p
			}
			return d.Tag
		}
		if strings.TrimSpace(d.TwinTag) == tag {
			return d.Tag
		}
	}
	return ""
}

// sourceDisplayName — как источник зовут в сообщениях (SPEC 112-A, «Понятные
// ошибки»): сначала подпись, за ней тег узла (server/chain), за ним URL
// подписки. ULID в текст не выносится — по нему источник не узнать; он идёт в
// сообщение, только когда другого имени нет вовсе.
func sourceDisplayName(ps ProxySource, index int) string {
	if s := strings.TrimSpace(ps.Label); s != "" {
		return s
	}
	if s := strings.TrimSpace(ps.Source); s != "" {
		return s
	}
	if len(ps.Connections) > 0 {
		if s := strings.TrimSpace(ps.Connections[0]); s != "" {
			return s
		}
	}
	if s := strings.TrimSpace(ps.ID); s != "" {
		return "id " + s
	}
	return fmt.Sprintf("#%d", index+1)
}

// emissionWarningsFor проставляет адресата-источник пачке фраз.
func emissionWarningsFor(ps ProxySource, index int, texts []string) []EmissionWarning {
	if len(texts) == 0 {
		return nil
	}
	label := sourceDisplayName(ps, index)
	out := make([]EmissionWarning, 0, len(texts))
	for _, t := range texts {
		if strings.TrimSpace(t) == "" {
			continue
		}
		out = append(out, EmissionWarning{Text: t, SourceID: strings.TrimSpace(ps.ID), SourceLabel: label})
	}
	return out
}
