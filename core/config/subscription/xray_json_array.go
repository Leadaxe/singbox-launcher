package subscription

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/textnorm"
)

// IsXrayJSONArrayBody reports whether s is a valid JSON array (used for subscription branch).
func IsXrayJSONArrayBody(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") {
		return false
	}
	if !json.Valid([]byte(s)) {
		return false
	}
	var raw []json.RawMessage
	return json.Unmarshal([]byte(s), &raw) == nil
}

// ParseNodesFromXrayJSONArray parses a JSON array of Xray-style full configs into ParsedNode list.
// Non-Xray elements (e.g. sing-box-only outbounds) are skipped with a debug log.
// skip uses the same rules as URI subscriptions (shouldSkipNode).
func ParseNodesFromXrayJSONArray(jsonBody string, skip []map[string]string) ([]*configtypes.ParsedNode, error) {
	jsonBody = strings.TrimSpace(jsonBody)
	var elems []json.RawMessage
	if err := json.Unmarshal([]byte(jsonBody), &elems); err != nil {
		return nil, fmt.Errorf("subscription JSON array: %w", err)
	}

	// SPEC 094 §342 — ДВА прохода: «кто даёт узлу имя» и «в каком порядке узлы
	// идут» — разные задачи.
	//
	// Провайдеры намеренно переиспользуют один сервер в нескольких элементах:
	// тот же адрес приезжает и как «🇩🇪⚡Германия» (1 узел), и внутри пула
	// «🚀Авто | Лучший сервер» (15 узлов с техническими тегами). Показывать обе
	// записи нельзя — это дубль; но и схлопывать «как получится» тоже: без
	// правила владения пул съедал бы страны, и половина списка исчезала.
	//
	// Проход 1 (черновой, узлы выбрасываются) — элементы от одиночных к
	// многоузловым: право выпустить сервер достаётся элементу с осмысленным
	// remarks, а не пулу. Побочно узнаём владельца каждой идентичности.
	//
	// Проход 2 (боевой) — элементы строго в порядке файла; элемент выпускает
	// только те серверы, что закреплены за ним. Имена осмысленные, а позиции
	// авторские: автор часто ставит рекомендуемый узел первым.
	owner := computeXrayIdentityOwners(elems, skip)

	seen := make(map[string]struct{})
	// memberIdentities помнит, какую идентичность несёт каждый узел-группа в
	// своём составе: члены пула часто уезжают к другим элементам (страна
	// специфичнее пула), и ссылаться на них надо по ИТОГОВЫМ тегам.
	memberIdentities := make(map[*configtypes.ParsedNode][]string)
	// finalTagByIdentity — итоговый тег выжившего сервера.
	finalTagByIdentity := make(map[string]string)

	var out []*configtypes.ParsedNode
	for i, raw := range elems {
		nodes, err := parseXrayJSONArrayElementNodes(raw, i, skip)
		if err != nil {
			debuglog.WarnLog("Parser: Xray JSON array element %d: %v", i, err)
			continue
		}
		rememberGroupMemberIdentities(nodes, memberIdentities)
		out = append(out, filterByIdentityOwner(nodes, i, owner, seen, finalTagByIdentity)...)
	}

	// Элемент, от которого после дедупа остался ровно один сервер, отдаёт ему
	// своё чистое имя: различитель нужен, только когда узлов несколько.
	// У «БС»-элементов провайдера все узлы кроме одного — decoy-дубли чужих
	// стран, и без этого пользователь видел бы технический тег
	// «🇩🇪-Германия-БС-1 proxy-wl-d9yrc-guardora-pro».
	//
	// Делается ДО резолва состава групп: тот работает по идентичностям и
	// подставит уже итоговые теги.
	simplifySoloElementTags(out, finalTagByIdentity)

	// Состав групп резолвится ПОСЛЕ всех элементов: член мог достаться
	// элементу, который ещё не разобран.
	return resolveGroupMembers(out, memberIdentities, finalTagByIdentity), nil
}

// simplifySoloElementTags убирает различитель у элементов, от которых выжил
// ровно один сервер.
//
// Работает по базовому тегу (имя элемента): если его несёт единственный узел
// и он же не конфликтует с чужим тегом, различитель отбрасывается.
//
// finalTagByIdentity обновляется вместе с тегом, чтобы состав групп
// (резолвится позже, по идентичностям) сослался на новое имя.
func simplifySoloElementTags(
	nodes []*configtypes.ParsedNode,
	finalTagByIdentity map[string]string,
) {
	byBase := make(map[string][]*configtypes.ParsedNode)
	taken := make(map[string]struct{}, len(nodes))

	for _, n := range nodes {
		if n == nil {
			continue
		}
		taken[n.Tag] = struct{}{}
		if n.Scheme == configtypes.SchemeGroup {
			continue
		}
		if base, _, ok := strings.Cut(n.Tag, " "); ok {
			byBase[base] = append(byBase[base], n)
		}
	}

	for base, group := range byBase {
		if len(group) != 1 {
			continue // узлов несколько — различитель нужен
		}
		if _, clash := taken[base]; clash {
			continue // базовое имя уже занято другим узлом или группой
		}
		node := group[0]
		delete(taken, node.Tag)
		node.Tag = base
		taken[base] = struct{}{}
		if node.Outbound != nil {
			node.Outbound["tag"] = base
		}
		// Группы ссылаются на членов по идентичности — обновляем и её.
		if NodeIdentityHashFunc != nil {
			if hash := NodeIdentityHashFunc(node); hash != "" {
				finalTagByIdentity[hash] = base
			}
		}
	}
}

// rememberGroupMemberIdentities запоминает идентичности членов узла-группы до
// того, как фильтр владения раскидает их по элементам.
func rememberGroupMemberIdentities(
	nodes []*configtypes.ParsedNode,
	memberIdentities map[*configtypes.ParsedNode][]string,
) {
	if NodeIdentityHashFunc == nil {
		return
	}

	byTag := make(map[string]string, len(nodes))
	for _, n := range nodes {
		if n == nil || n.Scheme == configtypes.SchemeGroup {
			continue
		}
		if hash := NodeIdentityHashFunc(n); hash != "" {
			byTag[n.Tag] = hash
		}
	}

	for _, n := range nodes {
		if n == nil || n.Scheme != configtypes.SchemeGroup || n.Outbound == nil {
			continue
		}
		raw, _ := n.Outbound[configtypes.GroupMembersKey].([]interface{})
		ids := make([]string, 0, len(raw))
		for _, item := range raw {
			if tag, ok := item.(string); ok {
				if hash, known := byTag[tag]; known {
					ids = append(ids, hash)
				}
			}
		}
		memberIdentities[n] = ids
	}
}

// resolveGroupMembers переписывает состав узлов-групп на итоговые теги
// выживших серверов.
//
// Член пула часто уезжает к другому элементу: «Авто | Лучший сервер»
// перечисляет те же серверы, что и «🇩🇪 Германия», а право на имя у страны.
// Группа должна ссылаться на итоговый тег («🇩🇪-Германия»), а не исчезать.
func resolveGroupMembers(
	nodes []*configtypes.ParsedNode,
	memberIdentities map[*configtypes.ParsedNode][]string,
	finalTagByIdentity map[string]string,
) []*configtypes.ParsedNode {
	if len(memberIdentities) == 0 {
		return nodes
	}

	out := make([]*configtypes.ParsedNode, 0, len(nodes))
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Scheme != configtypes.SchemeGroup || node.Outbound == nil {
			out = append(out, node)
			continue
		}

		ids, tracked := memberIdentities[node]
		if !tracked {
			out = append(out, node)
			continue
		}

		members := make([]interface{}, 0, len(ids))
		dedup := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			tag, alive := finalTagByIdentity[id]
			if !alive {
				continue
			}
			if _, dup := dedup[tag]; dup {
				continue
			}
			dedup[tag] = struct{}{}
			members = append(members, tag)
		}

		if len(members) == 0 {
			debuglog.WarnLog("Parser: Xray group %q has no surviving members — dropped", node.Tag)
			continue
		}
		node.Outbound[configtypes.GroupMembersKey] = members
		out = append(out, node)
	}
	return out
}

// computeXrayIdentityOwners — проход 1 §342: определяет, какой элемент вправе
// выпустить каждый сервер.
//
// Элементы обходятся от одиночных к многоузловым (стабильно: при равном числе
// узлов — в порядке файла), и первый встретивший идентичность её и получает.
// Так «🇩🇪⚡Германия» забирает сервер себе, а пул «Авто | Лучший сервер»,
// перечисляющий тот же адрес, его отдаёт.
func computeXrayIdentityOwners(elems []json.RawMessage, skip []map[string]string) map[string]int {
	if NodeIdentityHashFunc == nil {
		// Без хука идентичности владения нет — дедуп не работает вовсе.
		return nil
	}

	order := make([]int, len(elems))
	for i := range elems {
		order[i] = i
	}
	counts := make([]int, len(elems))
	for i, raw := range elems {
		counts[i] = xrayElementPayloadCount(raw)
	}
	// Сортировка вставками: стабильна по построению, а элементов подписки
	// обычно десятки — незачем тащить sort.SliceStable ради этого.
	for i := 1; i < len(order); i++ {
		for j := i; j > 0; j-- {
			if counts[order[j]] >= counts[order[j-1]] {
				break
			}
			order[j], order[j-1] = order[j-1], order[j]
		}
	}

	owner := make(map[string]int)
	for _, idx := range order {
		nodes, err := parseXrayJSONArrayElementNodes(elems[idx], idx, skip)
		if err != nil {
			continue
		}
		for _, node := range nodes {
			if node == nil || node.Scheme == configtypes.SchemeGroup {
				continue // группа — не сервер, идентичности не занимает
			}
			hash := NodeIdentityHashFunc(node)
			if hash == "" {
				continue
			}
			if _, taken := owner[hash]; !taken {
				owner[hash] = idx
			}
		}
	}
	return owner
}

// filterByIdentityOwner — проход 2 §342: элемент выпускает только закреплённые
// за ним серверы.
//
// seen страхует от повтора внутри одного владельца. Узлы-группы проходят
// всегда: они не серверы, а объединяют то, что выжило.
func filterByIdentityOwner(
	nodes []*configtypes.ParsedNode,
	elemIndex int,
	owner map[string]int,
	seen map[string]struct{},
	finalTagByIdentity map[string]string,
) []*configtypes.ParsedNode {
	if owner == nil {
		return nodes
	}

	kept := make([]*configtypes.ParsedNode, 0, len(nodes))

	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Scheme == configtypes.SchemeGroup {
			// Состав резолвится в самом конце, по всей подписке: член мог
			// уехать к элементу, который ещё не разобран.
			kept = append(kept, node)
			continue
		}

		hash := NodeIdentityHashFunc(node)
		if hash == "" {
			kept = append(kept, node)
			continue
		}
		if ownerIdx, ok := owner[hash]; ok && ownerIdx != elemIndex {
			debuglog.DebugLog("Parser: Xray element %d: %q belongs to element %d — dropped here",
				elemIndex, node.Tag, ownerIdx)
			continue
		}
		if _, dup := seen[hash]; dup {
			continue
		}
		seen[hash] = struct{}{}
		finalTagByIdentity[hash] = node.Tag
		kept = append(kept, node)
	}

	return kept
}

// xrayElementPayloadCount — сколько непослужебных outbound'ов в элементе.
// Это и есть мера «насколько специфично» его имя: 1 узел — конкретная страна,
// 15 — пул балансировщика.
func xrayElementPayloadCount(raw json.RawMessage) int {
	var root map[string]interface{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return 0
	}
	obs, ok := root["outbounds"].([]interface{})
	if !ok {
		return 0
	}
	n := 0
	for _, item := range obs {
		ob, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if IsXrayServiceProtocol(xrayMapString(ob, "protocol")) {
			continue
		}
		n++
	}
	return n
}

// parseXrayJSONArrayElementNodes разбирает один элемент массива в СПИСОК узлов
// (SPEC 094 фаза C).
//
// До SPEC 094 отсюда возвращался ровно один узел и только vless: элемент с
// vmess/trojan/ss/hysteria2 давал ноль нод, а элемент с несколькими vless терял
// все, кроме одного.
//
// Именование (C3) сохраняет обратную совместимость тегов: единственный узел
// элемента получает чистое имя из remarks — ровно как раньше, — и лишь при
// нескольких узлах добавляется различитель.
func parseXrayJSONArrayElementNodes(raw json.RawMessage, elemIndex int, skip []map[string]string) ([]*configtypes.ParsedNode, error) {
	var root map[string]interface{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("invalid element JSON: %w", err)
	}

	if !xrayElementHasProtocolOutbounds(root) {
		debuglog.DebugLog("Parser: Xray JSON array element %d: skip (no Xray protocol outbounds)", elemIndex)
		return nil, nil
	}

	outboundsRaw, ok := root["outbounds"].([]interface{})
	if !ok || len(outboundsRaw) == 0 {
		return nil, fmt.Errorf("missing outbounds")
	}

	// Индекс по тегу нужен для резолва dialerProxy-целей.
	byTag := make(map[string]map[string]interface{})
	candidates := make([]map[string]interface{}, 0, len(outboundsRaw))
	dialerTargets := make(map[string]struct{})

	for _, obRaw := range outboundsRaw {
		ob, ok := obRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if tag := xrayMapString(ob, "tag"); tag != "" {
			byTag[tag] = ob
		}
		if IsXrayServiceProtocol(xrayMapString(ob, "protocol")) {
			continue
		}
		candidates = append(candidates, ob)

		streamSettings, _ := ob["streamSettings"].(map[string]interface{})
		if ref := xraySockoptDialerRef(streamSettings); ref != "" {
			dialerTargets[ref] = struct{}{}
		}
	}

	// Цель dialerProxy — звено цепочки, а не самостоятельный узел.
	payload := make([]map[string]interface{}, 0, len(candidates))
	for _, ob := range candidates {
		if tag := xrayMapString(ob, "tag"); tag != "" {
			if _, isTarget := dialerTargets[tag]; isTarget {
				continue
			}
		}
		payload = append(payload, ob)
	}
	if len(payload) == 0 {
		return nil, nil
	}

	remarksRaw := strings.TrimSpace(xrayMapString(root, "remarks"))
	base := xrayTagBaseFromRoot(remarksRaw, elemIndex)

	// §322: элемент с балансировщиком — это ПУЛ, а не набор самостоятельных
	// серверов. Его узлы получают различители и объединяются узлом-группой с
	// чистым именем из remarks; без балансировщика единственный узел
	// сохраняет чистое имя, как было до SPEC 094.
	hasBalancer := xrayElementHasBalancer(root)
	single := len(payload) == 1 && !hasBalancer

	// Дедуп ВНУТРИ элемента: один сервер, продублированный в самом пуле,
	// — это дубль. Между элементами дедупа нет (см. source_loader.go).
	seenInElement := make(map[string]struct{}, len(payload))

	out := make([]*configtypes.ParsedNode, 0, len(payload))
	memberTags := make([]string, 0, len(payload))
	unsupported := make(map[string]struct{})

	for idx, ob := range payload {
		label := remarksRaw
		if label == "" {
			label = xrayMapString(ob, "tag")
		}
		if label == "" {
			label = fmt.Sprintf("xray-%d", elemIndex)
		}

		node, err := xrayNodeFromOutbound(ob, label)
		if err != nil {
			// C1: протокол не исчезает молча.
			protocol := strings.ToLower(strings.TrimSpace(xrayMapString(ob, "protocol")))
			if protocol != "" {
				unsupported[protocol] = struct{}{}
			}
			debuglog.DebugLog("Parser: Xray element %d outbound %d: %v", elemIndex, idx, err)
			continue
		}
		if node == nil {
			continue
		}

		node.Tag = xrayElementNodeTag(base, ob, idx, single)
		if node.Outbound != nil {
			node.Outbound["tag"] = node.Tag
		}

		// dialerProxy → цепочка (C4). Глубина берётся из фазы B.
		if err := attachXrayDialerChain(node, ob, byTag, node.Tag, label); err != nil {
			debuglog.WarnLog("Parser: Xray element %d: %v — skipping node %q", elemIndex, err, node.Tag)
			continue
		}

		if shouldSkipNode(node, skip) {
			continue
		}

		// Дедуп внутри элемента: тот же сервер, перечисленный в пуле дважды.
		if NodeIdentityHashFunc != nil {
			if hash := NodeIdentityHashFunc(node); hash != "" {
				if _, dup := seenInElement[hash]; dup {
					debuglog.DebugLog("Parser: Xray element %d: %q duplicates an earlier node — dropped",
						elemIndex, node.Tag)
					continue
				}
				seenInElement[hash] = struct{}{}
			}
		}

		out = append(out, node)
		memberTags = append(memberTags, node.Tag)
	}

	for protocol := range unsupported {
		debuglog.WarnLog("Parser: Xray element %d: unsupported protocol %q skipped", elemIndex, protocol)
	}

	// §322: узел-группа идёт ПОСЛЕ своих членов — порядок списка совпадает с
	// порядком появления, и группа логично следует за пулом.
	if hasBalancer {
		if group := xrayBalancerFromElement(root, remarksRaw, elemIndex, memberTags); group != nil {
			out = append(out, group)
		}
	}

	return out, nil
}

// xrayElementNodeTag формирует тег узла (SPEC 094 C3).
//
// Единственный узел элемента сохраняет ЧИСТОЕ имя из remarks — так теги
// существующих подписок не меняются при переходе на многоузловой разбор.
// При нескольких узлах добавляется различитель: тег outbound'а, иначе индекс.
func xrayElementNodeTag(base string, ob map[string]interface{}, idx int, single bool) string {
	if single {
		return base
	}
	if tag := strings.TrimSpace(xrayMapString(ob, "tag")); tag != "" {
		return fmt.Sprintf("%s %s", base, tag)
	}
	return fmt.Sprintf("%s %d", base, idx+1)
}

// attachXrayDialerChain разворачивает цепочку dialerProxy в node.Chain.
//
// Глубина ограничена maxDetourChainDepth (фаза B); кольца рвутся по посещённым
// тегам. Ошибка возвращается только когда цепочка объявлена, но непригодна —
// такой узел лучше пропустить, чем отдать ядру заведомо нерабочим.
func attachXrayDialerChain(
	node *configtypes.ParsedNode,
	ob map[string]interface{},
	byTag map[string]map[string]interface{},
	ownerTag, label string,
) error {
	streamSettings, _ := ob["streamSettings"].(map[string]interface{})
	ref := xraySockoptDialerRef(streamSettings)
	if ref == "" {
		return nil
	}

	visited := map[string]struct{}{}
	if tag := xrayMapString(ob, "tag"); tag != "" {
		visited[tag] = struct{}{}
	}

	for depth := 0; depth < maxDetourChainDepth && ref != ""; depth++ {
		if _, seen := visited[ref]; seen {
			debuglog.DebugLog("Parser: Xray dialerProxy cycle at %q — chain truncated", ref)
			break
		}
		hopOb, ok := byTag[ref]
		if !ok {
			return fmt.Errorf("dialerProxy %q not found", ref)
		}
		visited[ref] = struct{}{}

		hopTag := fmt.Sprintf("%s%s", ownerTag, xrayJumpOutboundTagSuffix)
		if depth > 0 {
			hopTag = fmt.Sprintf("%s%s%d", ownerTag, xrayJumpOutboundTagSuffix, depth+1)
		}

		hop, err := xrayChainHopFromOutbound(hopOb, hopTag, label)
		if err != nil {
			return fmt.Errorf("dialerProxy %q: %w", ref, err)
		}

		if len(node.Chain) > 0 {
			prev := node.Chain[len(node.Chain)-1]
			if prev.Outbound != nil {
				prev.Outbound["detour"] = hop.Tag
			}
		}
		node.Chain = append(node.Chain, hop)

		hopStream, _ := hopOb["streamSettings"].(map[string]interface{})
		ref = xraySockoptDialerRef(hopStream)
	}

	node.SyncJumpFromChain()
	return nil
}

// xrayChainHopFromOutbound строит звено цепочки.
//
// В отличие от узла, звеном может быть и socks — он не становится
// самостоятельной нодой, но как первый хоп вполне пригоден.
func xrayChainHopFromOutbound(ob map[string]interface{}, hopTag, label string) (*configtypes.ParsedNode, error) {
	protocol := strings.ToLower(strings.TrimSpace(xrayMapString(ob, "protocol")))
	if protocol == "socks" {
		jump, err := xrayBuildJumpFromSocksOutbound(ob, hopTag)
		if err != nil {
			return nil, err
		}
		return &configtypes.ParsedNode{
			Tag:      jump.Tag,
			Scheme:   jump.Scheme,
			Server:   jump.Server,
			Port:     jump.Port,
			UUID:     jump.UUID,
			Flow:     jump.Flow,
			Label:    label,
			Outbound: jump.Outbound,
		}, nil
	}

	hop, err := xrayNodeFromOutbound(ob, label)
	if err != nil {
		return nil, err
	}
	if hop == nil {
		return nil, fmt.Errorf("service protocol %q cannot be a chain hop", protocol)
	}
	hop.Tag = hopTag
	if hop.Outbound != nil {
		hop.Outbound["tag"] = hopTag
	}
	return hop, nil
}

func xrayElementHasProtocolOutbounds(root map[string]interface{}) bool {
	outboundsRaw, ok := root["outbounds"].([]interface{})
	if !ok {
		return false
	}
	for _, obRaw := range outboundsRaw {
		ob, ok := obRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if _, ok := ob["protocol"].(string); ok {
			return true
		}
	}
	return false
}

const xrayTagBaseMaxRunes = 48

// Suffix for the SOCKS jump outbound tag (main outbound uses the base slug only; detour points at base+suffix).
const xrayJumpOutboundTagSuffix = "_jump_server"

// xrayTagBaseFromRoot returns a stable tag prefix for main/jump outbounds.
// When remarks is non-empty, it is sanitized into a slug; otherwise "xray-{index}".
func xrayTagBaseFromRoot(remarksRaw string, elemIndex int) string {
	if strings.TrimSpace(remarksRaw) == "" {
		return fmt.Sprintf("xray-%d", elemIndex)
	}
	return xrayRemarksToTagBase(remarksRaw, elemIndex)
}

// xrayRemarksToTagBase builds a tag slug from Xray remarks (for sing-box tag fields):
// letters and digits (any script), Unicode regional indicators (flag emoji, e.g. 🇱🇻), and hyphens between runs.
func xrayRemarksToTagBase(remarks string, elemIndex int) string {
	s := strings.TrimSpace(textnorm.NormalizeProxyDisplay(remarks))
	if s == "" {
		return fmt.Sprintf("xray-%d", elemIndex)
	}
	var b strings.Builder
	lastSep := false
	for _, r := range s {
		if xrayTagSlugKeepRune(r) {
			b.WriteRune(r)
			lastSep = false
			continue
		}
		if r == '_' || r == '-' {
			if b.Len() > 0 && !lastSep {
				b.WriteRune('-')
				lastSep = true
			}
			continue
		}
		// spaces, punctuation, emoji → single hyphen between word runs
		if b.Len() > 0 && !lastSep {
			b.WriteRune('-')
			lastSep = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fmt.Sprintf("xray-%d", elemIndex)
	}
	runes := []rune(out)
	if len(runes) > xrayTagBaseMaxRunes {
		out = string(runes[:xrayTagBaseMaxRunes])
		out = strings.TrimRight(out, "-")
	}
	if out == "" {
		return fmt.Sprintf("xray-%d", elemIndex)
	}
	return out
}

// xrayTagSlugKeepRune returns true for letters, digits, and Regional Indicator symbols (U+1F1E6..U+1F1FF),
// which form UTF-8 flag emoji in pairs. Other symbols (punctuation, generic emoji) are not kept in the slug.
func xrayTagSlugKeepRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsNumber(r) {
		return true
	}
	return r >= 0x1F1E6 && r <= 0x1F1FF
}
