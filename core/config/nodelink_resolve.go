// File nodelink_resolve.go — ЕДИНЫЙ резолв ссылок NodeLink (SPEC 118 W4,
// Т5; features/directions.md §6).
//
// # Одна ссылка на все виды
//
// В модели v7 все ссылки «через кого» — это `NodeLink{folderId, tag}`:
// detour узла, позиции цепочки, члены Auto-группы. Резолв у них ОДИН и
// выполняется здесь, на проходе 2 сборки, когда финальные теги вычислены у
// всех источников (тот же инвариант двухпроходности, что у node_ref.go).
//
// Словарь целей:
//
//	folderId задан → узел этой папки по СЫРОМУ тегу;
//	folderId пуст  → корневое пространство ФИНАЛЬНЫХ тегов: верхний узел,
//	                 Направление, replace-тег, системный тег шаблона.
//
// # Разная строгость по виду ребра — намеренно
//
//   - detour и позиции цепочек — FAIL-CLOSED: нерезолвящаяся, выключенная
//     или недоступная цель означает, что носитель ссылки деградирует с ⚠, а
//     не молча идёт напрямую. Анонимность не понижается тихо. Кольца по
//     этим рёбрам — тоже fail-closed.
//   - члены Auto — PRUNE: битый или выключенный член выпадает из состава с
//     предупреждением; группа, потерявшая всех, не эмитится (пустой urltest
//     роняет старт ядра).
//
// Разница не стилистическая: член группы — один из многих равноправных, его
// потеря не меняет маршрут остальных; detour — единственный путь.
//
// Исключений по типу узла здесь нет: WireGuard идёт общим путём резолва
// наравне с остальными (проверено запуском ядра — см. resolveCanonicalDetour).
package config

import (
	"sort"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
)

// NodeLinkTargets — словарь целей резолва на одну сборку.
type NodeLinkTargets struct {
	// byFolder — узлы папок: folderId → сырой тег → финальный тег.
	byFolder map[string]map[string]*ParsedNode
	// byRootTag — корневое пространство финальных тегов (верхние узлы).
	byRootTag map[string]*ParsedNode
	// rootNames — прочие законные цели корня без узла за ними: теги
	// Направлений, replace-теги, системные теги шаблона. Ссылка на них
	// легальна (SPEC §4.E.3), но ParsedNode за ними нет.
	rootNames map[string]bool
}

// BuildNodeLinkTargets собирает словарь целей.
//
// nodesBySource — узлы по позиции источника (после прохода 1, теги
// финальные); proxies — сборочная форма источников (даёт folderId и признак
// контейнера); extraRootTags — теги Направлений, replace-тегов и системных
// outbound'ов шаблона.
func BuildNodeLinkTargets(
	proxies []ProxySource,
	nodesBySource map[int][]*ParsedNode,
	extraRootTags []string,
) *NodeLinkTargets {
	t := &NodeLinkTargets{
		byFolder:  make(map[string]map[string]*ParsedNode),
		byRootTag: make(map[string]*ParsedNode),
		rootNames: make(map[string]bool, len(extraRootTags)),
	}
	for i := range proxies {
		cs := proxies[i].Canonical
		nodes := nodesBySource[i]
		if cs != nil && cs.IsContainer && cs.FolderID != "" {
			byRaw := t.byFolder[cs.FolderID]
			if byRaw == nil {
				byRaw = make(map[string]*ParsedNode, len(nodes))
				t.byFolder[cs.FolderID] = byRaw
			}
			for _, n := range nodes {
				if n == nil {
					continue
				}
				raw := canonicalRawTag(n)
				if raw == "" {
					continue
				}
				if _, dup := byRaw[raw]; !dup {
					byRaw[raw] = n
				}
			}
			continue
		}
		// Верхние узлы: их финальный тег и есть адрес в корне.
		for _, n := range nodes {
			if n == nil || n.Tag == "" {
				continue
			}
			if _, dup := t.byRootTag[n.Tag]; !dup {
				t.byRootTag[n.Tag] = n
			}
		}
	}
	for _, tag := range extraRootTags {
		if tag = strings.TrimSpace(tag); tag != "" {
			t.rootNames[tag] = true
		}
	}
	return t
}

// allRootLinkTargets — законные цели КОРНЕВОГО пространства без узла за
// ними: теги Направлений (и их твинов), replace-теги папок и системные теги
// шаблона (`ChainBuiltinHopTags` — direct/block).
//
// Одно место сбора: разойдись оно с гардом занятости — и ссылка на живой
// replace-тег читалась бы висячей (fail-closed на ровном месте).
func allRootLinkTargets(parserConfig *ParserConfig, directionTags map[string]bool) []string {
	var out []string
	for tag := range directionTags {
		out = append(out, tag)
	}
	if parserConfig != nil {
		for _, d := range parserConfig.ParserConfig.Outbounds {
			if d.Tag == "" || d.Disabled {
				continue
			}
			out = append(out, d.Tag)
			if d.Auto != nil {
				out = append(out, d.Tag+twinSuffix)
			}
			out = append(out, d.AddOutbounds...)
		}
		for i := range parserConfig.ParserConfig.Proxies {
			ps := parserConfig.ParserConfig.Proxies[i]
			if ps.Canonical != nil && ps.Canonical.Replace != nil {
				out = append(out, FolderReplaceTags(ps.Canonical.Replace)...)
			}
			// Мостовые локальные группы (Fold) — те же цели до W5.
			for _, ob := range ps.LocalGroups {
				if ob.Tag != "" {
					out = append(out, ob.Tag)
				}
			}
		}
	}
	out = append(out, ChainBuiltinHopTags...)
	sort.Strings(out)
	return out
}

// rootNodeTagsForGuard — финальные теги ВЕРХНИХ узлов (вне папок): вход
// гарда занятости.
func rootNodeTagsForGuard(parserConfig *ParserConfig, nodesBySource map[int][]*ParsedNode) []string {
	if parserConfig == nil {
		return nil
	}
	var out []string
	for i := range parserConfig.ParserConfig.Proxies {
		cs := parserConfig.ParserConfig.Proxies[i].Canonical
		if cs != nil && cs.IsContainer {
			continue // узлы папки живут в её пространстве, не в корневом
		}
		for _, n := range nodesBySource[i] {
			if n != nil && n.Tag != "" {
				out = append(out, n.Tag)
			}
		}
	}
	sort.Strings(out)
	return out
}

// canonicalRawTag — сырой тег узла в его контейнере. У канонических узлов это
// IdentityTag (снят до тег-политики); у мостовых — он же (SPEC 112).
func canonicalRawTag(n *ParsedNode) string {
	if s := strings.TrimSpace(n.IdentityTag); s != "" {
		return s
	}
	return strings.TrimSpace(n.Tag)
}

// NodeLinkResolution — исход резолва одной ссылки.
type NodeLinkResolution struct {
	// Node — найденный узел (nil, если целью был тег без узла за ним).
	Node *ParsedNode
	// Tag — финальный тег цели; пусто = не разрешилось.
	Tag string
	// Problem — человекочитаемая причина, если цель не найдена.
	Problem string
}

// Resolve материализует одну ссылку в финальный тег.
//
// Выключенный узел до этой точки не доезжает вовсе: эмиссия его не создаёт
// (canonical_emit.go), поэтому «выключенная цель» и «отсутствующая цель» —
// один исход, и он fail-closed для detour/хопов.
func (t *NodeLinkTargets) Resolve(link configtypes.NodeLink) NodeLinkResolution {
	tag := strings.TrimSpace(link.Tag)
	if tag == "" {
		return NodeLinkResolution{Problem: locale.T(emitLinkEmptyText)}
	}
	if folder := strings.TrimSpace(link.FolderID); folder != "" {
		byRaw, known := t.byFolder[folder]
		if !known {
			return NodeLinkResolution{Problem: locale.T(emitLinkSourceMissingText)}
		}
		if n := byRaw[tag]; n != nil {
			return NodeLinkResolution{Node: n, Tag: n.Tag}
		}
		return NodeLinkResolution{Problem: locale.Tf(emitLinkNodeMissingText, tag)}
	}
	if n := t.byRootTag[tag]; n != nil {
		return NodeLinkResolution{Node: n, Tag: n.Tag}
	}
	if t.rootNames[tag] {
		// Направление, replace-тег, системный тег — узла за ними нет, но
		// ссылка законна (§4.E.3).
		return NodeLinkResolution{Tag: tag}
	}
	return NodeLinkResolution{Problem: locale.Tf(emitLinkTargetUnknownText, tag)}
}

// ApplyCanonicalNodeLinks — проход 2 для канонических узлов: detour, позиции
// цепочек, состав Auto-групп.
//
// Возвращает уцелевшие узлы и предупреждения. Порядок обработки —
// топологический по рёбрам detour (цель раньше ссылающегося), поэтому
// выпадение КАСКАДИРУЕТ: выпал узел-хоп — выпадает и тот, кто через него
// ходил. Кольца fail-closed целиком.
func ApplyCanonicalNodeLinks(
	proxies []ProxySource,
	nodesBySource map[int][]*ParsedNode,
	allNodes []*ParsedNode,
	targets *NodeLinkTargets,
) ([]*ParsedNode, []EmissionWarning) {
	if targets == nil {
		return allNodes, nil
	}
	var warnings []EmissionWarning
	// SPEC 116 W12 фикс 3: адресат берётся у САМОГО узла (SourceIndex), а не
	// угадывается по тексту причины. Иначе строка Sources не знает, у кого
	// ставить ⚠, — а именно за этим предупреждение и пишется.
	addr := func(n *ParsedNode, text string) EmissionWarning {
		w := EmissionWarning{Text: text}
		if n != nil && n.SourceIndex != UnsetSourceIndex && n.SourceIndex >= 0 && n.SourceIndex < len(proxies) {
			ps := proxies[n.SourceIndex]
			w.SourceID = strings.TrimSpace(ps.ID)
			w.SourceLabel = sourceDisplayName(ps, n.SourceIndex)
		}
		return w
	}

	// dropped — узлы, выпавшие fail-closed; повторный проход видит их
	// отсутствие и роняет тех, кто на них ссылался (каскад).
	dropped := make(map[*ParsedNode]bool)

	// Отдельной проверки WireGuard здесь БОЛЬШЕ НЕТ: раньше detour у WG
	// снимался молча, и о висячей цели приходилось предупреждать своим
	// проходом. Теперь WG идёт общим путём резолва — непойманная цель даёт
	// ту же ошибку, что у остальных узлов, и второе сообщение о том же было
	// бы дублем.

	// Итерация до фикспойнта: каждое выпадение может открыть следующее.
	// Верхняя граница защитная — каждый содержательный проход что-то
	// удаляет, их конечное число.
	for iter := 0; iter <= len(allNodes)+1; iter++ {
		changed := false
		for _, n := range allNodes {
			if n == nil || dropped[n] {
				continue
			}
			if reason := resolveCanonicalDetour(n, targets, dropped); reason != "" {
				dropped[n] = true
				warnings = append(warnings, addr(n, reason))
				debuglog.WarnLog("nodelink: %s", reason)
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Кольца detour: то, что осталось после фикспойнта и ходит по кругу.
	if cycled := detectCanonicalDetourCycles(allNodes, dropped); len(cycled) > 0 {
		for _, n := range cycled {
			dropped[n] = true
			reason := locale.Tf(emitDetourCycleText, n.Tag)
			warnings = append(warnings, addr(n, reason))
			debuglog.WarnLog("nodelink: %s", reason)
		}
	}

	// Auto-группы — prune: состав фильтруется, пустая группа не эмитится.
	for _, n := range allNodes {
		if n == nil || dropped[n] || n.Scheme != configtypes.SchemeGroup {
			continue
		}
		if len(n.CanonicalGroupMembers) == 0 && n.CanonicalGroupDefault == "" {
			continue // импортированная группа мостового пути — её состав уже сведён
		}
		for _, w := range resolveCanonicalGroup(n, targets, dropped) {
			warnings = append(warnings, addr(n, w))
		}
		if len(groupMemberTags(n)) == 0 {
			dropped[n] = true
			reason := locale.Tf(emitGroupEmptyText, n.Tag)
			warnings = append(warnings, addr(n, reason))
			debuglog.WarnLog("nodelink: %s", reason)
		}
	}

	if len(dropped) == 0 {
		return allNodes, warnings
	}
	kept := make([]*ParsedNode, 0, len(allNodes))
	for _, n := range allNodes {
		if n != nil && !dropped[n] {
			kept = append(kept, n)
		}
	}
	pruneNodesBySource(nodesBySource, kept)
	return kept, warnings
}

// nodeWarning — фраза и узел, к которому она относится: адресата ставит
// вызывающий, у которого на руках список источников.
type nodeWarning struct {
	node *ParsedNode
	text string
}

// resolveCanonicalDetour штампует detour узлу. Возвращает непустую причину,
// если носитель обязан выпасть (fail-closed).
func resolveCanonicalDetour(n *ParsedNode, targets *NodeLinkTargets, dropped map[*ParsedNode]bool) string {
	if n.CanonicalDetour == nil {
		return ""
	}
	// Исключения для WireGuard ЗДЕСЬ НЕТ (проверено запуском ядра
	// 1.14.0-lx.28: endpoint/wireguard с `detour` стартует и честно
	// дозванивается через указанный outbound).
	//
	// Раньше detour у WG снимался молча «правилом модели», и настройка,
	// выставленная в форме — хоть личная у узла, хоть общая у папки, —
	// пропадала между состоянием и конфигом без единого слова. Для узлов
	// Proton поверх WARP это означало, что заданный маршрут просто не
	// работал.
	res := targets.Resolve(*n.CanonicalDetour)
	if res.Problem != "" {
		return locale.Tf(emitDetourUnresolvedText, n.Tag, res.Problem)
	}
	if res.Node != nil && dropped[res.Node] {
		return locale.Tf(emitDetourTargetDroppedText, n.Tag, res.Tag)
	}
	if res.Tag == n.Tag {
		return locale.Tf(emitDetourSelfText, n.Tag)
	}
	if n.Outbound == nil {
		n.Outbound = map[string]interface{}{}
	}
	n.Outbound["detour"] = res.Tag
	return ""
}

// detectCanonicalDetourCycles — участники колец по рёбрам detour среди
// живых узлов. Граф функциональный (у узла не больше одного detour), поэтому
// обход тривиален: идём по единственному ребру, пока не упрёмся в уже
// посещённое.
func detectCanonicalDetourCycles(allNodes []*ParsedNode, dropped map[*ParsedNode]bool) []*ParsedNode {
	byTag := make(map[string]*ParsedNode, len(allNodes))
	for _, n := range allNodes {
		if n != nil && !dropped[n] && n.Tag != "" {
			if _, dup := byTag[n.Tag]; !dup {
				byTag[n.Tag] = n
			}
		}
	}
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := make(map[*ParsedNode]int, len(byTag))
	var cycled []*ParsedNode
	seen := make(map[*ParsedNode]bool)

	for _, start := range allNodes {
		if start == nil || dropped[start] || color[start] != white {
			continue
		}
		path := make([]*ParsedNode, 0, 8)
		cur := start
		for {
			if color[cur] == grey {
				at := -1
				for k, v := range path {
					if v == cur {
						at = k
						break
					}
				}
				if at >= 0 {
					for _, v := range path[at:] {
						if !seen[v] {
							seen[v] = true
							cycled = append(cycled, v)
						}
					}
				}
				break
			}
			if color[cur] == black {
				break
			}
			color[cur] = grey
			path = append(path, cur)

			if cur.Outbound == nil {
				break
			}
			next, _ := cur.Outbound["detour"].(string)
			next = strings.TrimSpace(next)
			if next == "" {
				break
			}
			target := byTag[next]
			if target == nil {
				break // цель — группа/Направление, у неё detour'а нет
			}
			cur = target
		}
		for k := len(path) - 1; k >= 0; k-- {
			color[path[k]] = black
		}
	}
	return cycled
}

// resolveCanonicalGroup сводит состав Auto-группы: члены → финальные теги,
// битые и выключенные выпадают с предупреждением (prune).
func resolveCanonicalGroup(n *ParsedNode, targets *NodeLinkTargets, dropped map[*ParsedNode]bool) []string {
	var warnings []string
	members := make([]interface{}, 0, len(n.CanonicalGroupMembers))
	seen := make(map[string]struct{}, len(n.CanonicalGroupMembers))
	for _, link := range n.CanonicalGroupMembers {
		res := targets.Resolve(link)
		if res.Problem != "" || (res.Node != nil && dropped[res.Node]) {
			why := res.Problem
			if why == "" {
				why = locale.T(emitMemberDroppedReasonText)
			}
			w := locale.Tf(emitGroupMemberLostText, n.Tag, link.Tag, why)
			warnings = append(warnings, w)
			debuglog.WarnLog("nodelink: %s", w)
			continue
		}
		if _, dup := seen[res.Tag]; dup {
			continue
		}
		seen[res.Tag] = struct{}{}
		members = append(members, res.Tag)
	}
	if n.Outbound == nil {
		n.Outbound = map[string]interface{}{}
	}
	n.Outbound[configtypes.GroupMembersKey] = members

	// default — только у selector и только из состава: ядро отвергает ВЕСЬ
	// конфиг, если умолчание не входит в группу.
	delete(n.Outbound, "default")
	if def := strings.TrimSpace(n.CanonicalGroupDefault); def != "" {
		groupType, _ := n.Outbound["type"].(string)
		if groupType != "selector" {
			// urltest со stray default не плодим (форма канона).
			return warnings
		}
		res := targets.Resolve(configtypes.NodeLink{FolderID: canonicalGroupFolder(n), Tag: def})
		if res.Problem == "" {
			if _, inList := seen[res.Tag]; inList {
				n.Outbound["default"] = res.Tag
				return warnings
			}
		}
		w := locale.Tf(emitGroupDefaultDroppedText, n.Tag, def)
		warnings = append(warnings, w)
		debuglog.WarnLog("nodelink: %s", w)
	}
	return warnings
}

// canonicalGroupFolder — папка, в чьём пространстве адресован default группы:
// та же, что у её членов (default — сырой тег члена, SPEC Т2).
func canonicalGroupFolder(n *ParsedNode) string {
	if len(n.CanonicalGroupMembers) > 0 {
		return n.CanonicalGroupMembers[0].FolderID
	}
	return ""
}
