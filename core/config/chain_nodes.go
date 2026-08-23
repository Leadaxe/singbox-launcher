// File chain_nodes.go — превращение источников-цепочек в узлы (SPEC 110).
//
// Цепочка ссылается на теги, которые становятся окончательными только после
// загрузки ВСЕХ источников: подписка переименовывает узлы префиксом и
// уникализирует дубли, а Направления и вовсе разворачиваются позже. Поэтому
// цепочка не может собраться внутри своего источника, как собирается сервер
// из URI, — её узел строится здесь, когда весь пул уже известен.
//
// Тот же довод, по которому здесь же живёт resolveNodeHashDetours (SPEC 101).
package config

import (
	"strconv"

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
// Имя источника, а при пустом — запасное `chain-<N>` по позиции в списке:
// пустой тег в конфиге валит `sing-box check`, и оставить его нельзя даже
// когда пользователь не удосужился назвать цепочку.
func chainSourceTag(src ProxySource, index int) string {
	if t := src.TagMask; t != "" {
		return t
	}
	return "chain-" + strconv.Itoa(index+1)
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
		unsupportedReason = "ядро не поддерживает цепочки"
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

	var broken []ChainDegradation
	for i, src := range parserConfig.ParserConfig.Proxies {
		if src.Chain == nil || src.Disabled {
			continue
		}
		tag := chainSourceTag(src, i)
		name := tag
		if src.TagMask == "" && src.Source != "" {
			name = src.Source
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
			degrade("позиция " + missing + " не найдена среди узлов и Направлений")
			continue
		}

		node := &ParsedNode{
			Tag:         tag,
			Label:       name,
			Scheme:      configtypes.ChainOutboundType,
			Outbound:    ChainOutboundObject(tag, src.Chain),
			SourceIndex: i,
			EmitRaw:     true,
		}
		allNodes = append(allNodes, node)
		nodesBySource[i] = append(nodesBySource[i], node)
		known[tag] = true
		debuglog.DebugLog("chain: источник %q стал узлом из %d позиций", tag, len(src.Chain.Hops))
	}
	return allNodes, broken
}
