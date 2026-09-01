// File chain_nodes.go — превращение источников-цепочек в узлы (SPEC 110).
//
// Цепочка ссылается на теги, которые становятся окончательными только после
// загрузки ВСЕХ источников: подписка переименовывает узлы префиксом и
// уникализирует дубли, а Направления и вовсе разворачиваются позже. Поэтому
// цепочка не может собраться внутри своего источника, как собирается сервер
// из URI, — её узел строится здесь, когда весь пул уже известен.
//
// Тот же довод, по которому здесь же живёт resolveNodeTagDetours (SPEC 101).
package config

import (
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// ChainDegradation — цепочка, не ставшая узлом, и почему.
//
// Не «тихо пропустить»: пользователь настроил маршрут, увидел его в списке
// источников и вправе узнать, почему трафик пошёл не туда.
type ChainDegradation struct {
	Tag    string
	Name   string
	Reason string
}

// chainSourceTag — тег будущего узла цепочки.
//
// Берётся из канона (сырой тег chain-узла = его финальный тег: у корневого
// узла тег-политики нет), а при пустом — запасное `chain-<N>` по позиции в
// списке: пустой тег в конфиге валит `sing-box check`, и оставить его нельзя
// даже когда пользователь не удосужился назвать цепочку.
func chainSourceTag(src ProxySource, index int) string {
	if t := canonicalChainTag(src); t != "" {
		return t
	}
	return "chain-" + strconv.Itoa(index+1)
}

// canonicalChainTag — тег chain-узла источника (пусто, если такого узла нет).
func canonicalChainTag(src ProxySource) string {
	if src.Canonical == nil {
		return ""
	}
	for i := range src.Canonical.Nodes {
		if src.Canonical.Nodes[i].Kind == canonicalKindChain {
			return strings.TrimSpace(src.Canonical.Nodes[i].Tag)
		}
	}
	return ""
}

// ResolveChainSources строит узлы для источников-цепочек и дописывает их к
// пулу.
//
// Возвращает обновлённый пул и список деградировавших цепочек.
//
// Проба ядра — первым делом (SPEC 110 T1): ядро без `with_lx_chain`
// отвергает ВЕСЬ конфиг на неизвестном типе outbound'а, то есть одна
// настроенная цепочка оставила бы пользователя вообще без VPN.
//
// Порядок разрешения — по списку источников, и цепочка может ссылаться на
// цепочку, объявленную ВЫШЕ: так вложенность остаётся выразимой, но циклы
// невозможны по построению — ровно тем же приёмом, что `include` у
// Направлений.
func ResolveChainSources(
	parserConfig *ParserConfig,
	allNodes []*ParsedNode,
	nodesBySource map[int][]*ParsedNode,
	directionTags map[string]bool,
) ([]*ParsedNode, []ChainDegradation) {
	if parserConfig == nil {
		return allNodes, nil
	}

	// Есть ли вообще цепочки: конфиги без них должны собираться ровно так
	// же, как раньше, не платя ни за один лишний проход.
	hasChain := false
	for _, src := range parserConfig.ParserConfig.Proxies {
		if src.Chain != nil && !src.Disabled {
			hasChain = true
			break
		}
	}
	if !hasChain {
		return allNodes, nil
	}

	supported, unsupportedReason := chainSupported()
	if unsupportedReason == "" {
		unsupportedReason = "the core does not support chains"
	}

	// Теги, на которые цепочка вправе сослаться. Узлы и Направления —
	// изначально, цепочки — по мере разрешения (см. выше про порядок).
	known := make(map[string]bool, len(allNodes)+len(directionTags)+8)
	for _, n := range allNodes {
		if n != nil && n.Tag != "" {
			known[n.Tag] = true
		}
	}
	for tag := range directionTags {
		known[tag] = true
	}
	// Служебные теги шаблона, которые форма предлагает позициями. Шаблонные
	// константы подмешиваются только на финальной сборке и здесь неизвестны —
	// без этой добавки предложенный формой `direct-out` («первый хоп без
	// прокси») деградировал бы цепочку с причиной, противоречащей UI.
	for _, tag := range ChainBuiltinHopTags {
		known[tag] = true
	}
	// SPEC 118 W4: теги ЗАМЕН свёрнутых папок — законные позиции
	// (features/directions.md §5: «резолв NodeLink видит replace-теги наравне
	// с узлами»). Узлом такая цель не является: замена разворачивается
	// локальной группой на проходе 0, и без этой добавки хоп на неё
	// деградировал бы цепочку, хотя цель в конфиге есть.
	for i := range parserConfig.ParserConfig.Proxies {
		ps := parserConfig.ParserConfig.Proxies[i]
		if ps.Disabled || ps.Canonical == nil {
			continue
		}
		for _, tag := range FolderReplaceTags(ps.Canonical.Replace) {
			known[tag] = true
		}
	}

	// Для проверок состава: узлы по тегам (reality) и уже разрешённые
	// цепочки (вложенность). Раньше эти валидаторы существовали, но
	// вызывались только из формы — то есть действовали лишь в момент
	// редактирования: подписка обновлялась, узел становился reality, и
	// сохранённая цепочка со strip[tls.utls] валила старт ядра, а check
	// молчал (chain-check-misses-start-errors). Сборка — второй рубеж.
	nodesByTag := make(map[string]*ParsedNode, len(allNodes))
	for _, n := range allNodes {
		if n != nil && n.Tag != "" {
			nodesByTag[n.Tag] = n
		}
	}
	chainTags := make(map[string]bool, 4)

	var broken []ChainDegradation
	for i, src := range parserConfig.ParserConfig.Proxies {
		if src.Chain == nil || src.Disabled {
			continue
		}
		tag := chainSourceTag(src, i)
		name := tag
		if s := strings.TrimSpace(src.Label); s != "" {
			name = s
		}

		degrade := func(reason string) {
			debuglog.WarnLog("chain: источник %q не стал узлом: %s", tag, reason)
			broken = append(broken, ChainDegradation{Tag: tag, Name: name, Reason: reason})
		}

		if !supported {
			degrade(unsupportedReason)
			continue
		}
		if reason := ChainEmitError(tag, src.Chain); reason != "" {
			degrade(reason)
			continue
		}
		// Коллизия имени: цепочка, названная как существующий узел,
		// Направление или другая цепочка, дала бы два outbound'а с одним
		// тегом — ядро отвергает такой конфиг целиком. Узлы подписок через
		// это не проходят (MakeTagUnique), цепочки шли в обход. После
		// ChainEmitError: собственные диагностики цепочки информативнее.
		if known[tag] {
			degrade("the name “" + tag + "” is already taken by another node, Direction or chain")
			continue
		}
		// Позиция, которой нет среди известных тегов, — ссылка в никуда, на
		// которой ядро не стартует. Цепочка выпадает ЦЕЛИКОМ, а не теряет
		// позицию: маршрут без хопа — это другой маршрут, и подменять его
		// молча нельзя.
		missing := ""
		for _, hop := range src.Chain.Hops {
			if !known[hop] {
				missing = hop
				break
			}
		}
		if missing != "" {
			degrade("position " + missing + " not found among nodes and Directions")
			continue
		}
		if conflicts := ChainRealityConflict(src.Chain, nodesByTag); len(conflicts) > 0 {
			degrade("strip removes tls.utls while positions " + strings.Join(conflicts, ", ") +
				" are reality nodes: the core refuses to start with such a config")
			continue
		}
		if nested := ChainNestedConflict(src.Chain, chainTags); len(nested) > 0 {
			degrade("chains " + strings.Join(nested, ", ") +
				" are not in the first position — the core allows a nested chain only at position 0")
			continue
		}

		node := &ParsedNode{
			Tag:   tag,
			Label: name,
			// SPEC 112: идентичность узла цепочки — её собственный тег. Он же
			// финальный: ни префиксов, ни маски у цепочки нет, тег задаётся
			// пользователем напрямую. Проставляется явно, чтобы ссылка на
			// цепочку (SPEC 112-A) резолвилась той же картой, что и на узел
			// подписки, а не падала на запасное правило.
			IdentityTag: tag,
			Scheme:      configtypes.ChainOutboundType,
			Outbound:    ChainOutboundObject(tag, src.Chain),
			SourceIndex: i,
			EmitRaw:     true,
		}
		allNodes = append(allNodes, node)
		nodesBySource[i] = append(nodesBySource[i], node)
		known[tag] = true
		chainTags[tag] = true
		nodesByTag[tag] = node
		debuglog.DebugLog("chain: источник %q стал узлом из %d позиций", tag, len(src.Chain.Hops))
	}
	return allNodes, broken
}
