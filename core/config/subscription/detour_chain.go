package subscription

import (
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// SPEC 094 фаза B — detour-цепочки произвольной глубины.
//
// Порядок вычислений критичен и зафиксирован в SPEC §3.2 B3:
//
//	1. найти рёбра, замыкающие кольца;
//	2. цели detour = цели ВСЕХ рёбер, КРОМЕ снятых на шаге 1;
//	3. кандидаты = entries − служебные − группы − цели detour;
//	4. для каждого кандидата собрать цепочку.
//
// Если шаг 1 выполнить после шага 2, узел внутри кольца попадёт в цели detour
// и исчезнет из подписки совсем — вместо того чтобы остаться рабочим узлом с
// разорванным ребром.

// maxDetourChainDepth — предел длины цепочки (SPEC 094 B2).
const maxDetourChainDepth = 8

// singboxChainInfo — результат анализа detour-графа одного конфига.
type singboxChainInfo struct {
	// detourTargets — теги, которые не становятся самостоятельными узлами.
	detourTargets map[string]struct{}
	// brokenEdges — теги, чьё исходящее detour-ребро замыкает кольцо.
	brokenEdges map[string]struct{}
}

func (c *singboxChainInfo) isDetourTarget(tag string) bool {
	if c == nil || tag == "" {
		return false
	}
	_, ok := c.detourTargets[tag]
	return ok
}

func (c *singboxChainInfo) isBrokenEdge(tag string) bool {
	if c == nil || tag == "" {
		return false
	}
	_, ok := c.brokenEdges[tag]
	return ok
}

// analyzeSingboxDetour строит карту detour-графа конфига.
func analyzeSingboxDetour(entries []map[string]interface{}, byTag map[string]map[string]interface{}) *singboxChainInfo {
	info := &singboxChainInfo{
		detourTargets: make(map[string]struct{}),
		brokenEdges:   make(map[string]struct{}),
	}

	// Шаг 1 — кольца.
	findDetourCycleEdges(entries, byTag, info.brokenEdges)

	// Шаг 2 — цели detour, кроме снятых рёбер.
	for _, entry := range entries {
		ownerTag := mapString(entry, "tag")
		if info.isBrokenEdge(ownerTag) {
			continue
		}
		target := strings.TrimSpace(mapString(entry, "detour"))
		if target == "" {
			continue
		}
		if info.isBrokenEdge(target) {
			// Цель сама владеет снятым ребром, то есть лежит на кольце.
			// Поглотить её как «чужой джамп» нельзя: она осталась без
			// исходящего ребра и обязана выжить самостоятельным узлом,
			// иначе кольцо A→B→A стёрло бы из подписки обе ноды.
			continue
		}
		targetEntry, ok := byTag[target]
		if !ok {
			continue // висячая ссылка: цель не существует, исключать нечего
		}
		// Служебная цель (detour:"direct") узлом и так не является;
		// группа целью быть не может — цепочка на неё не строится (B5).
		targetType := strings.ToLower(strings.TrimSpace(mapString(targetEntry, "type")))
		if IsSingboxServiceType(targetType) || IsSingboxGroupType(targetType) {
			continue
		}
		info.detourTargets[target] = struct{}{}
	}

	return info
}

// findDetourCycleEdges помечает теги, чьё detour-ребро замыкает кольцо.
//
// Обход стартует с каждого узла в порядке файла; settled хранит уже
// обойдённые старты, чтобы не переобходить один граф многократно. На кольце
// A→B→A виновником детерминированно назначается тот, кого обошли первым.
func findDetourCycleEdges(entries []map[string]interface{}, byTag map[string]map[string]interface{}, broken map[string]struct{}) {
	settled := make(map[string]struct{}, len(entries))

	for _, entry := range entries {
		startTag := mapString(entry, "tag")
		if startTag == "" {
			continue
		}
		if _, done := settled[startTag]; done {
			continue
		}

		path := make(map[string]struct{})
		current := startTag

		for {
			if _, seen := path[current]; seen {
				// current уже в текущем пути — ребро, которое сюда привело,
				// замыкает кольцо. Владелец ребра помечается ниже.
				break
			}
			path[current] = struct{}{}
			settled[current] = struct{}{}

			currentEntry, ok := byTag[current]
			if !ok {
				break
			}
			next := strings.TrimSpace(mapString(currentEntry, "detour"))
			if next == "" {
				break
			}
			if _, inPath := path[next]; inPath {
				// Ребро current → next замыкает кольцо: рвём именно его.
				broken[current] = struct{}{}
				break
			}
			current = next
		}
	}
}

// attachChain собирает цепочку detour для узла и записывает её в node.Chain.
func (c *singboxChainInfo) attachChain(
	node *configtypes.ParsedNode,
	entry map[string]interface{},
	byTag map[string]map[string]interface{},
	cfgIdx int,
) {
	if node == nil {
		return
	}

	visited := map[string]struct{}{}
	if tag := mapString(entry, "tag"); tag != "" {
		visited[tag] = struct{}{}
	}

	current := entry
	currentTag := mapString(entry, "tag")

	for depth := 0; depth < maxDetourChainDepth; depth++ {
		if c.isBrokenEdge(currentTag) {
			// Ребро снято как замыкающее кольцо (B3).
			debuglog.DebugLog("Parser: singbox import %q: detour edge broken (cycle)", node.Tag)
			break
		}

		target := strings.TrimSpace(mapString(current, "detour"))
		if target == "" {
			break
		}

		if _, seen := visited[target]; seen {
			debuglog.DebugLog("Parser: singbox import %q: detour cycle at %q — chain truncated", node.Tag, target)
			break
		}

		targetEntry, ok := byTag[target]
		if !ok {
			// B5: висячая ссылка — узел живёт, дозванивается напрямую.
			debuglog.DebugLog("Parser: singbox import %q: detour target %q not found — chain truncated", node.Tag, target)
			break
		}

		targetType := strings.ToLower(strings.TrimSpace(mapString(targetEntry, "type")))
		if IsSingboxServiceType(targetType) {
			// B5: detour:"direct" — цепочка просто заканчивается, молча.
			break
		}
		if IsSingboxGroupType(targetType) {
			// B5: развёрнутая группа в detour дала бы её без членов.
			debuglog.DebugLog("Parser: singbox import %q: detour target %q is a group — chain truncated", node.Tag, target)
			break
		}

		hop, err := parseSingboxEntry(targetEntry, cfgIdx, depth)
		if err != nil {
			debuglog.DebugLog("Parser: singbox import %q: detour hop %q unusable (%v) — chain truncated", node.Tag, target, err)
			break
		}

		node.Chain = append(node.Chain, hop)
		visited[target] = struct{}{}
		current = targetEntry
		currentTag = target
	}

	if len(node.Chain) == maxDetourChainDepth {
		// Достигнут предел: цепочка усечена, но узел рабочий (B2).
		if next := strings.TrimSpace(mapString(current, "detour")); next != "" {
			debuglog.WarnLog("Parser: singbox import %q: detour chain exceeds depth %d — truncated",
				node.Tag, maxDetourChainDepth)
		}
	}

	node.SyncJumpFromChain()
}
