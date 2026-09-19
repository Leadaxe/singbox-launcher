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

// chainNodeTag — тег будущего узла цепочки.
//
// Берётся из записи прохода 2 (сырой тег chain-узла = его финальный тег:
// тег-политика к цепочке не применяется), а при пустом — запасное
// `chain-<N>` по позиции источника в списке: пустой тег в конфиге валит
// `sing-box check`, и оставить его нельзя даже когда пользователь не
// удосужился назвать цепочку. Вторая и следующие безымянные цепочки одного
// источника получают ещё и свой номер: одно запасное имя на две цепочки
// столкнулось бы само с собой.
func chainNodeTag(bc configtypes.BuiltChain, sourceIndex, chainIndex int) string {
	if t := strings.TrimSpace(bc.Tag); t != "" {
		return t
	}
	tag := "chain-" + strconv.Itoa(sourceIndex+1)
	if chainIndex > 0 {
		tag += "-" + strconv.Itoa(chainIndex+1)
	}
	return tag
}

// chainHopUnresolvedMark — префикс позиции цепочки, ссылка которой НЕ
// резолвнулась на проходе 2 (ResolveCanonicalChainHops).
//
// Зачем маркер, а не сырой тег. Проход 2 знает точно, что позиция не
// разрешилась: цель выключена, папки нет, узла в ней нет. Но раньше он
// оставлял в hops сырой тег «в расчёте на то, что ResolveChainSources
// уронит цепочку», а тот проверяет позиции по `known` — пространству
// ФИНАЛЬНЫХ тегов КОРНЯ. Совпадение имён там не редкость: хоп
// {FolderID:"F1", Tag:"US-1"} при выключенной папке F1 и корневом узле с
// финальным тегом `US-1` проходил проверку, и цепочка молча собиралась
// через ЧУЖОЙ сервер — то есть настройка анонимности подменялась другим
// маршрутом без предупреждения.
//
// Символы взяты заведомо непечатные: тег узла приходит из подписки и из
// формы, и оба пути прогоняют его через нормализацию имени — совпасть с
// маркером он не может, поэтому `known[маркированный]` ложен ВСЕГДА, а
// деградация fail-closed наступает независимо от имён в корне.
const chainHopUnresolvedMark = "\x00unresolved\x00"

// markChainHopUnresolved помечает позицию нерезолвимой, сохраняя внутри
// исходный тег: он нужен текстам причин.
func markChainHopUnresolved(tag string) string {
	return chainHopUnresolvedMark + tag
}

// chainHopDisplayTag снимает маркер: человеку в причине показывают тег
// позиции, каким он стоит в цепочке, а не наш служебный префикс.
func chainHopDisplayTag(hop string) string {
	return strings.TrimPrefix(hop, chainHopUnresolvedMark)
}

// chainHopIsUnresolved — позиция помечена нерезолвимой проходом 2.
func chainHopIsUnresolved(hop string) bool {
	return strings.HasPrefix(hop, chainHopUnresolvedMark)
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
		if len(src.Chains) > 0 && !src.Disabled {
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
		if src.Disabled {
			continue
		}
		// Цепочек у источника бывает несколько (папка): каждая — свой узел,
		// свои проверки и своя деградация; выпавшая не роняет соседок.
		for ci, bc := range src.Chains {
			if bc.Chain == nil {
				continue
			}
			if node, reason := buildChainNode(src, i, ci, bc, supported, unsupportedReason, known, nodesByTag, chainTags); node != nil {
				allNodes = append(allNodes, node)
				nodesBySource[i] = append(nodesBySource[i], node)
				known[node.Tag] = true
				chainTags[node.Tag] = true
				nodesByTag[node.Tag] = node
				debuglog.DebugLog("chain: source %q became a node of %d positions", node.Tag, len(bc.Chain.Hops))
			} else {
				debuglog.WarnLog("chain: source %q did not become a node: %s", reason.Tag, reason.Reason)
				broken = append(broken, reason)
			}
		}
	}
	return allNodes, broken
}

// buildChainNode — одна цепочка источника: узел либо причина, по которой он
// не собрался.
//
// Порядок проверок нормативен: собственные диагностики цепочки информативнее
// коллизии имени, а позиция, которой нет, — раньше проверок состава.
func buildChainNode(
	src ProxySource,
	sourceIndex, chainIndex int,
	bc configtypes.BuiltChain,
	supported bool,
	unsupportedReason string,
	known map[string]bool,
	nodesByTag map[string]*ParsedNode,
	chainTags map[string]bool,
) (*ParsedNode, ChainDegradation) {
	tag := chainNodeTag(bc, sourceIndex, chainIndex)
	name := tag
	// Подпись источника называет цепочку только у корневой записи: у папки
	// подпись — имя контейнера, а цепочек в нём может быть несколько.
	if src.Canonical == nil || !src.Canonical.IsContainer {
		if s := strings.TrimSpace(src.Label); s != "" {
			name = s
		}
	}
	degrade := func(reason string) (*ParsedNode, ChainDegradation) {
		return nil, ChainDegradation{Tag: tag, Name: name, Reason: reason}
	}

	if !supported {
		return degrade(unsupportedReason)
	}
	if reason := ChainEmitError(tag, bc.Chain); reason != "" {
		return degrade(reason)
	}
	// Коллизия имени: цепочка, названная как существующий узел, Направление
	// или другая цепочка, дала бы два outbound'а с одним тегом — ядро
	// отвергает такой конфиг целиком. Узлы подписок через это не проходят
	// (MakeTagUnique), цепочки шли в обход. После ChainEmitError: собственные
	// диагностики цепочки информативнее.
	if known[tag] {
		return degrade("the name “" + tag + "” is already taken by another node, Direction or chain")
	}
	// Позиция, которой нет среди известных тегов, — ссылка в никуда, на
	// которой ядро не стартует. Цепочка выпадает ЦЕЛИКОМ, а не теряет
	// позицию: маршрут без хопа — это другой маршрут, и подменять его молча
	// нельзя.
	for _, hop := range bc.Chain.Hops {
		// Маркер проверяется ПЕРВЫМ: позиция, про которую проход 2 уже знает,
		// что её цель не нашлась, роняет цепочку независимо от того, носит ли
		// кто-то в корне такое же имя.
		if chainHopIsUnresolved(hop) || !known[hop] {
			return degrade("position " + chainHopDisplayTag(hop) + " not found among nodes and Directions")
		}
	}
	if conflicts := ChainRealityConflict(bc.Chain, nodesByTag); len(conflicts) > 0 {
		return degrade("strip removes tls.utls while positions " + strings.Join(conflicts, ", ") +
			" are reality nodes: the core refuses to start with such a config")
	}
	if nested := ChainNestedConflict(bc.Chain, chainTags); len(nested) > 0 {
		return degrade("chains " + strings.Join(nested, ", ") +
			" are not in the first position — the core allows a nested chain only at position 0")
	}

	return &ParsedNode{
		Tag:   tag,
		Label: name,
		// SPEC 112: идентичность узла цепочки — её собственный тег. Он же
		// финальный: ни префиксов, ни маски у цепочки нет, тег задаётся
		// пользователем напрямую. Проставляется явно, чтобы ссылка на цепочку
		// (SPEC 112-A) резолвилась той же картой, что и на узел подписки, а не
		// падала на запасное правило.
		IdentityTag: tag,
		Scheme:      configtypes.ChainOutboundType,
		Outbound:    ChainOutboundObject(tag, bc.Chain),
		SourceIndex: sourceIndex,
		EmitRaw:     true,
		// SPEC 132: обратный путь «финальный тег → узел состояния». У
		// сборочной формы, положенной вызывающим напрямую, ссылки нет —
		// такому узлу тег не сопоставится, и страховка на него не действует.
		CanonicalLink: bc.Link,
	}, ChainDegradation{}
}

// sourceHasPendingChains — есть ли у источника цепочки, которые соберёт
// проход 2.
//
// Узел-цепочка на проходе 1 не эмитится (errCanonicalChainDeferred), и
// источник, у которого кроме цепочек ничего нет, выглядел там пустым: пометка
// «не дал ни одного узла» ставилась и собравшейся цепочке. Условие то же, что
// у ResolveCanonicalChainHops: включённый узел-цепочка с позициями. Сборочная
// форма, положенная вызывающим напрямую (ps.Chains), тоже считается.
func sourceHasPendingChains(ps ProxySource) bool {
	if len(ps.Chains) > 0 {
		return true
	}
	if ps.Canonical == nil {
		return false
	}
	for i := range ps.Canonical.Nodes {
		cn := &ps.Canonical.Nodes[i]
		if cn.Kind == canonicalKindChain && cn.Enabled && len(cn.Hops) > 0 {
			return true
		}
	}
	return false
}

// chainSourceFailure — запись «источник не дал ни одного узла» для
// источника, ни одна цепочка которого не собралась.
//
// Причина — то, что сказал о его цепочках проход 2, а не общее «ничего не
// осталось после разбора и фильтров»: разбора у цепочки нет. Подпись — тег
// цепочки, когда у источника своей подписи нет (корневая цепочка), иначе
// строка отчёта называла бы пустое имя.
func chainSourceFailure(ps ProxySource, index int, broken []ChainDegradation) SourceExclusion {
	var reasons []string
	firstTag := ""
	for ci, bc := range ps.Chains {
		tag := chainNodeTag(bc, index, ci)
		if firstTag == "" {
			firstTag = tag
		}
		for _, b := range broken {
			if b.Tag == tag {
				reasons = appendReason(reasons, b.Reason)
			}
		}
	}
	if firstTag == "" && ps.Canonical != nil {
		// Проход 2 не начинался (узлов нет вовсе): тег берётся из канона.
		for i := range ps.Canonical.Nodes {
			if cn := &ps.Canonical.Nodes[i]; cn.Kind == canonicalKindChain {
				firstTag = strings.TrimSpace(cn.Tag)
				break
			}
		}
	}
	failure := sourceParseFailure(ps, reasons)
	if failure.SourceLabel == "" {
		failure.SourceLabel = firstTag
	}
	return failure
}
