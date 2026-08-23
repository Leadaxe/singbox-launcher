// File chain_cycle.go — цикл «группа содержит цепочку, которая через эту
// группу идёт» (SPEC 110, T9).
//
// В первой редакции ТЗ цепочка была Направлением, и циклы были невозможны
// по построению: `include` разрешён только на записи, стоящие ВЫШЕ по
// списку. Когда цепочка стала источником, то есть узлом, это свойство
// исчезло — а сценарий, который его ломает, самый частый из всех:
//
//	цепочка «через Германию наружу» = [proxy-out, exit-node]
//	proxy-out — Направление с фильтром «все узлы» → ловит и саму цепочку
//	⇒ proxy-out содержит цепочку, которая идёт через proxy-out
//
// Ядро на таком не падает — оно разворачивает позицию в тот outbound,
// который группа выбрала в данный момент, — но пользователь получает
// маршрут, которого не задумывал: выбрав цепочку внутри proxy-out, он
// замыкает трафик на неё же. Плюс подменяет смысл самой цепочки: «через
// Германию» превращается в «через то, что сейчас выбрано, а выбрано может
// быть это же».
//
// Поэтому цепочка не попадает в состав Направления, через которое проходит.
// Транзитивно: цепочка → цепочка → Направление считается тем же случаем.
package config

import (
	"singbox-launcher/internal/debuglog"
)

// chainHopsByTag — карта «тег цепочки → её позиции», по узлам пула.
//
// Строится по узлам, а не по источникам: к этому моменту цепочки уже
// разрешены в узлы, и только у узла тег окончателен.
func chainHopsByTag(allNodes []*ParsedNode) map[string][]string {
	var out map[string][]string
	for _, n := range allNodes {
		if n == nil || n.Scheme != "chain" || n.Outbound == nil {
			continue
		}
		raw, ok := n.Outbound["outbounds"].([]interface{})
		if !ok {
			continue
		}
		hops := make([]string, 0, len(raw))
		for _, h := range raw {
			if s, ok := h.(string); ok {
				hops = append(hops, s)
			}
		}
		if out == nil {
			out = make(map[string][]string, 4)
		}
		out[n.Tag] = hops
	}
	return out
}

// chainPassesThrough — проходит ли цепочка chainTag через тег target.
//
// Транзитивно через вложенные цепочки. seen защищает от зацикливания на
// испорченных данных: ResolveChainSources циклов не создаёт, но эта функция
// не должна зависеть от чужих инвариантов, чтобы не подвесить сборку.
func chainPassesThrough(chainTag, target string, hopsByTag map[string][]string, seen map[string]bool) bool {
	if seen[chainTag] {
		return false
	}
	seen[chainTag] = true
	for _, hop := range hopsByTag[chainTag] {
		if hop == target {
			return true
		}
		if _, nested := hopsByTag[hop]; nested {
			if chainPassesThrough(hop, target, hopsByTag, seen) {
				return true
			}
		}
	}
	return false
}

// dropChainsThroughDirection убирает из отобранного состава цепочки,
// проходящие через это же Направление (T9).
//
// Вызывается после фильтра: фильтр Направления не знает, что такое цепочка,
// и знать не должен — «все узлы» обязано означать все узлы.
//
// Вторым значением возвращает теги выброшенных цепочек: это предупреждение
// пользователю (`chain_cycle_through_direction`), а не внутренняя деталь —
// он собрал маршрут и вправе знать, почему тот не появился в группе.
func dropChainsThroughDirection(nodes []*ParsedNode, directionTag string, hopsByTag map[string][]string) ([]*ParsedNode, []string) {
	if directionTag == "" || len(hopsByTag) == 0 {
		return nodes, nil
	}
	// Сначала считаем, есть ли что выбрасывать: без цепочек в составе (а
	// это подавляющее большинство Направлений) вход возвращается как есть,
	// и конфиги без цепочек собираются байт-в-байт как раньше.
	cyclic := make(map[string]bool, 2)
	for _, n := range nodes {
		if n == nil || n.Scheme != "chain" || cyclic[n.Tag] {
			continue
		}
		if chainPassesThrough(n.Tag, directionTag, hopsByTag, make(map[string]bool, 4)) {
			cyclic[n.Tag] = true
		}
	}
	if len(cyclic) == 0 {
		return nodes, nil
	}
	out := make([]*ParsedNode, 0, len(nodes))
	var dropped []string
	for _, n := range nodes {
		if n != nil && cyclic[n.Tag] {
			debuglog.WarnLog("chain: цепочка %q не входит в %q — она через него проходит",
				n.Tag, directionTag)
			dropped = append(dropped, n.Tag)
			continue
		}
		out = append(out, n)
	}
	return out, dropped
}
