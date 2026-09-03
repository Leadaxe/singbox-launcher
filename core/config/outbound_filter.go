// Package config: outbound_filter.go — filtering logic for selector outbounds.
//
// Functions here determine which nodes match a selector's filters (tag, host, scheme, label, etc.).
// Supports literal match, negation !literal, regex /pattern/i, negation regex !/pattern/i.
// Used by outbound_generator.go (GenerateSelectorWithFilteredAddOutbounds, buildOutboundsInfo)
// and by PreviewSelectorNodes for UI preview.
package config

import (
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// filterNodesForSelector returns nodes that match the filter. filter may be nil (all nodes),
// a single map (AND of key/pattern), or a slice of maps (OR of maps). Empty map = no filter.
//
// FilterDirectionCandidatePool — ПУЛ КАНДИДАТОВ Направлений
// (SPEC 118 W4, Т5; features/directions.md §2).
//
// Правило пула, и ничего кроме него:
//
//   - верхние узлы — все до единого;
//   - узлы папок БЕЗ replace — под финальными тегами;
//   - у папок С replace вместо узлов пул видит только теги замены (их
//     подмешивает генератор селекторов, см. collectExposeTagCandidates);
//   - служебные узлы (релеи BYPASS, SPEC 120) — только если их источник
//     это разрешил галкой `RelaysInDirections`.
//
// Про релеи. Релей — дозвонщик внутри чужого маршрута, а не «страна»,
// которую выбирают, и по умолчанию Направлению он не кандидат. Раньше это
// правило жило ТОЛЬКО в пикере формы (business.NodesForDirectionPicker), а
// сборка считала состав заново по фильтрам — и Направление с фильтром по
// `scheme` или `host` молча забирало релеи, хотя в списке их не показывали.
// Два разных ответа на «кто входит в Направление» — это и есть дефект;
// правило обязано быть одно, и оно здесь.
//
// Механизма «спрятать узел из пула поштучно» не существует: кому служебный
// транспорт мешает — заводит фильтр или убирает транспорт в папку. Узел вне
// пула по-прежнему эмитится в outbounds и легален как хоп цепочки, цель
// detour и член Auto — пул это видимость для Направлений, не для конфига.
func FilterDirectionCandidatePool(allNodes []*ParsedNode, proxies []ProxySource) []*ParsedNode {
	if len(allNodes) == 0 {
		return allNodes
	}
	out := make([]*ParsedNode, 0, len(allNodes))
	for _, n := range allNodes {
		idx := n.SourceIndex
		if idx < 0 || idx >= len(proxies) {
			out = append(out, n)
			continue
		}
		cs := proxies[idx].Canonical
		// Свёрнутая папка представлена в пуле только тегами замены.
		if cs != nil && cs.IsContainer && cs.Replace != nil {
			continue
		}
		// Служебный узел — только с разрешения своего источника.
		if n.Service && (cs == nil || !cs.RelaysInDirections) {
			continue
		}
		out = append(out, n)
	}
	return out
}

func filterNodesForSelector(allNodes []*ParsedNode, filter interface{}) []*ParsedNode {
	if filter == nil {
		return allNodes // No filter, return all nodes
	}

	// Check if filter is an empty map - treat as no filter
	if filterMap, ok := filter.(map[string]interface{}); ok {
		if len(filterMap) == 0 {
			return allNodes // Empty filter object means no filter, return all nodes
		}
	}

	filtered := make([]*ParsedNode, 0)

	// Check if filter is an array
	if filterArray, ok := filter.([]interface{}); ok {
		// OR between filter objects
		for _, node := range allNodes {
			for _, filterObj := range filterArray {
				if filterMap, ok := filterObj.(map[string]interface{}); ok {
					filterStrMap := convertFilterToStringMap(filterMap)
					if matchesFilter(node, filterStrMap) {
						filtered = append(filtered, node)
						break // Node matched at least one filter, add it
					}
				}
			}
		}
	} else if filterMap, ok := filter.(map[string]interface{}); ok {
		// Single filter object (AND between keys)
		filterStrMap := convertFilterToStringMap(filterMap)
		for _, node := range allNodes {
			if matchesFilter(node, filterStrMap) {
				filtered = append(filtered, node)
			}
		}
	}

	return filtered
}

// convertFilterToStringMap flattens filter map to string values for matching (non-string values are skipped).
//
// SPEC 104: ключ с НЕВАЛИДНОЙ регуляркой отбрасывается, как будто его нет —
// опечатка в фильтре Направления не должна оставлять пользователя без
// узлов. MatchesPattern на битом выражении возвращает false, и без этой
// проверки одна лишняя скобка делала бы направление пустым (а с запасным
// составом из §3.3 — ещё и блокирующим весь трафик правила).
//
// Чинить это здесь, а не в MatchesPattern, обязательно: тот же матчер
// обслуживает skip-фильтры подписок, где «битый паттерн = совпало всё»
// означало бы выбросить все узлы разом.
func convertFilterToStringMap(filter map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range filter {
		str, ok := v.(string)
		if !ok {
			continue
		}
		if !configtypes.PatternCompiles(str) {
			debuglog.WarnLog("filters: %q=%q — malformed expression, key skipped", k, str)
			continue
		}
		result[k] = str
	}
	return result
}

// matchesFilter returns true if the node has matching values for every key in filter (AND); each value is checked with matchesPattern.
func matchesFilter(node *ParsedNode, filter map[string]string) bool {
	for key, pattern := range filter {
		value := getNodeValue(node, key)
		if !matchesPattern(value, pattern) {
			return false // At least one key doesn't match
		}
	}
	return true // All keys match
}

// getNodeValue returns the node field used in filters: tag, host, label, scheme, fragment (alias for label), comment.
//
// `label` (и его алиас `fragment`) — ИСХОДНОЕ имя узла из подписки, до
// tag-политики источника: prefix/postfix/mask и уникализация дублей меняют
// Tag, но не Label. Отбирать по нему имеет смысл ровно там, где маска
// перештамповала теги во что-то своё, а различать узлы надо по имени
// провайдера.
//
// Ключи оставлены, хотя ни один шаблон и ни один пресет ими не пользуется:
// они документированы (docs/ParserConfig.ru.md) и доступны в ручном JSON
// Направления, а неизвестный ключ фильтра не игнорируется — он не совпадает
// НИ С ЧЕМ (getNodeValue вернёт ""), то есть удаление молча опустошило бы
// такое Направление, а пустое уезжает в конфиг с запасным составом
// [block, direct] и блокирует трафик своих правил.
func getNodeValue(node *ParsedNode, key string) string {
	switch key {
	case "tag":
		return node.Tag
	case "host":
		return node.Server
	case "label":
		return node.Label
	case "scheme":
		return node.Scheme
	case "fragment":
		return node.Label // fragment == label
	case "comment":
		return node.Comment
	case "flow":
		return node.Flow
	default:
		return ""
	}
}

// matchesPattern matches value against pattern: literal, !literal, /regex/i, !/regex/i. Case-insensitive for regex.
// Delegates to the shared configtypes.MatchesPattern so selector filters and subscription skip-filters
// stay byte-equivalent (see core/config/configtypes/matcher.go).
func matchesPattern(value, pattern string) bool {
	return configtypes.MatchesPattern(value, pattern)
}

// PreviewSelectorNodes returns nodes that match outboundConfig.Filters and the default tag
// based on outboundConfig.PreferredDefault. It is used by UI layers to build a selector
// preview that is consistent with the real selector generation logic.
//
// allNodes must be the same set of nodes that will be used for selector generation
// (i.e. result of the same LoadNodesFromSource pipeline that GenerateOutboundsFromParserConfig uses).
// PreviewGlobalSelectorNodes applies exclude_from_global, then the same filter logic as PreviewSelectorNodes.
func PreviewGlobalSelectorNodes(allNodes []*ParsedNode, proxies []ProxySource, outboundConfig Direction) ([]*ParsedNode, string) {
	pool := FilterDirectionCandidatePool(allNodes, proxies)
	return PreviewSelectorNodes(pool, outboundConfig)
}

func PreviewSelectorNodes(allNodes []*ParsedNode, outboundConfig Direction) ([]*ParsedNode, string) {
	filtered := filterNodesForSelector(allNodes, outboundConfig.Filters)

	defaultTag := ""
	if len(outboundConfig.PreferredDefault) > 0 {
		preferredFilter := convertFilterToStringMap(outboundConfig.PreferredDefault)
		for _, node := range filtered {
			if matchesFilter(node, preferredFilter) {
				defaultTag = node.Tag
				break
			}
		}
	}

	return filtered, defaultTag
}

// ExposeTagSyntheticNode builds a minimal ParsedNode for ParserConfig.outbounds[].filters (SPEC §5):
// tag and comment from the wizard local outbound; host/scheme/label left empty.
func ExposeTagSyntheticNode(tag, comment string) *ParsedNode {
	return &ParsedNode{Tag: tag, Comment: comment, SourceIndex: UnsetSourceIndex}
}

// SelectorFiltersAcceptNode reports whether a single node matches the same filter rules as filterNodesForSelector
// (including OR-array and AND-object semantics).
func SelectorFiltersAcceptNode(filter interface{}, node *ParsedNode) bool {
	if node == nil {
		return false
	}
	matched := filterNodesForSelector([]*ParsedNode{node}, filter)
	return len(matched) > 0
}
