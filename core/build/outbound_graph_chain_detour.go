// File outbound_graph_chain_detour.go — правило 7 граф-санитайзера: цепочка
// как перезапись detour (SPEC 165 ядра, SPEC 189 лаунчера).
//
// Ядро с SPEC 165 при загрузке ставит каждому узлу, до которого цепочка
// доходит на позиции i ≥ 1 (члены групп любой вложенности тоже), detour на
// свой хоп «<цепочка>#<i−1>». Копий узла больше нет — у узла один экземпляр, и
// часть конфигов, которые прежнее ядро принимало, теперь валит загрузку ВСЕГО
// конфига (ALGORITHM ядра, §4.2):
//
//   - К1: узел перехвачен дважды — на позициях ≥ 1 двух цепочек или на двух
//     позициях ≥ 1 одной;
//   - К2: узел перехвачен в одной цепочке и достижим на позиции 0 любой;
//   - К5: у перехваченного узла фиксированный локальный порт или поле
//     привязки сокета — под detour они не действуют, ядро их отвергает;
//   - К6: собственный detour узла позиции 0 ведёт в узел, перехваченный той
//     же цепочкой, — кольцо через хоп.
//
// Политика последнего рубежа та же, что у правил 1–6: деградировать один
// элемент с кодом в отчёт сборки, а не отдать ядру конфиг, который оно
// отвергнет. К1/К2/К6 снимают цепочку (из пары — ту, что позже по порядку
// записей); К5 снимает поля с узла: он всё равно идёт через хоп, и сокет ему
// открывает хоп, а не он сам. К3 (тип chain-hop во входном конфиге) лаунчер
// не порождает; К4 (цикл через группу позиции) ловят правила 4 и 5.
//
// Правило действует, только когда ядро цели переписывает цепочки (версия не
// ниже границы `lx.detour` в registry/core_config.json или неизвестна): старое
// ядро клонирует узлы позиций, и для него эти конфиги законны.
package build

import (
	"sort"
	"strconv"
	"strings"

	"singbox-launcher/core/config/nodeflow"
	"singbox-launcher/core/config/registry"
	"singbox-launcher/internal/debuglog"
)

// chainDetourRewriteActive — исполнит ли конфиг ядро, которое переписывает
// цепочку в detour. Граница — `min_core` блока `lx.detour` реестра: блок и
// перезапись пришли в ядро одной задачей (SPEC 165). Версия неизвестна —
// считается новой, как у гейта `max_core`: лаунчер ставит машинам ядро пина.
func chainDetourRewriteActive(version string) bool {
	version = strings.TrimSpace(version)
	if version == "" {
		return true
	}
	reg, err := registry.Get()
	if err != nil {
		return true
	}
	f, ok := reg.CoreConfigFields()["lx.detour"]
	if !ok || f.MinCore == "" {
		return true
	}
	return nodeflow.CoreAtLeast(version, f.MinCore)
}

// chainHopSocketFields — поля привязки сокета, которые апстрим под detour не
// применяет (сокет узла открывает хоп), ALGORITHM К5 ядра.
var chainHopSocketFields = []string{
	"bind_interface", "inet4_bind_address", "inet6_bind_address",
	"bind_address_no_port", "protect_path", "routing_mark", "reuse_addr",
	"netns", "tcp_fast_open", "tcp_multi_path", "network_strategy",
	"network_type", "fallback_network_type",
}

// chainHopLocalPortFields — фиксированный локальный порт по типу узла
// (ALGORITHM К5 ядра). У MASQUE, hysteria, hysteria2, tuic, TrustTunnel и
// OpenVPN-клиента такого поля в конфиге нет.
var chainHopLocalPortFields = map[string][]string{
	"wireguard":   {"listen_port"},
	"tailscale":   {"listen_port", "relay_server_port"},
	"openconnect": {"dtls_local_port"},
}

// chainIntercept — узел перехвачен цепочкой chain на позиции pos.
type chainIntercept struct {
	chain *graphEntry
	pos   int
}

// chainSocketStrip — снятые поля К5 одной цепочки, для отчёта.
type chainSocketStrip struct {
	chain  *graphEntry
	nodes  []string
	fields map[string]bool
}

// chainPositionLeaves — узлы, до которых доходит позиция ref: сама позиция,
// если это узел, или листья группы любой вложенности. direct и block ядро не
// перехватывает; цепочка на позиции 0 — не узел (её позиции считаются у неё).
// Теги вне кэша (статика шаблона) узлами кэша не являются и пропускаются.
func chainPositionLeaves(ref string, byTag map[string]*graphEntry) []*graphEntry {
	var out []*graphEntry
	seen := map[string]bool{}
	var walk func(tag string)
	walk = func(tag string) {
		if seen[tag] {
			return
		}
		seen[tag] = true
		e := byTag[tag]
		if e == nil || e.dropped || e.m == nil {
			return
		}
		switch {
		case e.isGroup():
			for _, m := range e.members() {
				walk(m)
			}
		case e.isChain():
		default:
			if t := e.typ(); t == "direct" || t == "block" {
				return
			}
			out = append(out, e)
		}
	}
	walk(ref)
	return out
}

// applyChainDetourRules — одна итерация правила 7. Возвращает true, если
// что-то изменилось (снята цепочка или поля узла): фикспойнт вызовет снова.
func applyChainDetourRules(
	entries []*graphEntry,
	byTag map[string]*graphEntry,
	dropChain func(chain *graphEntry, code string, params map[string]string, why string),
	strips map[*graphEntry]*chainSocketStrip,
	stripOrder *[]*graphEntry,
) bool {
	var chains []*graphEntry
	for _, e := range entries {
		if !e.dropped && e.m != nil && e.isChain() {
			chains = append(chains, e)
		}
	}
	if len(chains) == 0 {
		return false
	}
	order := make(map[*graphEntry]int, len(chains))
	for i, c := range chains {
		order[c] = i
	}
	later := func(a, b *graphEntry) *graphEntry {
		if order[a] >= order[b] {
			return a
		}
		return b
	}

	// Позиция 0 каждой цепочки: узлы, которые перехватывать нельзя (К2).
	entry := map[*graphEntry]*graphEntry{} // узел → цепочка, где он на позиции 0
	for _, c := range chains {
		hops := c.members()
		if len(hops) == 0 {
			continue
		}
		for _, leaf := range chainPositionLeaves(hops[0], byTag) {
			if _, ok := entry[leaf]; !ok {
				entry[leaf] = c
			}
		}
	}

	intercepted := map[*graphEntry]chainIntercept{}
	for _, c := range chains {
		for pos, ref := range c.members() {
			if pos == 0 {
				continue
			}
			for _, leaf := range chainPositionLeaves(ref, byTag) {
				if prev, ok := intercepted[leaf]; ok {
					victim := later(c, prev.chain)
					other := prev.chain
					if victim == prev.chain {
						other = c
					}
					dropChain(victim, "chain_detour_conflict", map[string]string{
						"node":  leaf.tag,
						"other": other.tag,
					}, "node “"+leaf.tag+"” is at position ≥ 1 twice (chains “"+prev.chain.tag+"”, “"+c.tag+"”) — the core gives a node one detour")
					return true
				}
				if d, ok := entry[leaf]; ok {
					victim := later(c, d)
					other := d
					if victim == d {
						other = c
					}
					dropChain(victim, "chain_detour_conflict", map[string]string{
						"node":  leaf.tag,
						"other": other.tag,
					}, "node “"+leaf.tag+"” is at position ≥ 1 of “"+c.tag+"” and at position 0 of “"+d.tag+"” — the core gives a node one detour")
					return true
				}
				intercepted[leaf] = chainIntercept{chain: c, pos: pos}
			}
		}
	}

	// К6: собственный detour узла позиции 0 (его перехват не трогает) не
	// должен приводить в узел, перехваченный той же цепочкой.
	for _, c := range chains {
		hops := c.members()
		if len(hops) == 0 {
			continue
		}
		for _, leaf := range chainPositionLeaves(hops[0], byTag) {
			if hit := chainDetourWalkHits(leaf, c, byTag, intercepted); hit != nil {
				dropChain(c, "chain_detour_conflict", map[string]string{
					"node":  hit.tag,
					"other": c.tag,
				}, "the detour of “"+leaf.tag+"” (position 1) leads into “"+hit.tag+"”, which the chain routes through itself — a ring")
				return true
			}
		}
	}

	// К5: поля сокета и локального порта у перехваченных узлов.
	changed := false
	for _, e := range entries {
		ic, ok := intercepted[e]
		if !ok {
			continue
		}
		fields := append([]string(nil), chainHopSocketFields...)
		fields = append(fields, chainHopLocalPortFields[e.typ()]...)
		var removed []string
		for _, f := range fields {
			if _, has := e.m[f]; has {
				delete(e.m, f)
				removed = append(removed, f)
			}
		}
		if len(removed) == 0 {
			continue
		}
		e.dirty = true
		changed = true
		debuglog.WarnLog("build: %q is at position %d of chain %q — %s removed: the core dials it through the hop, where they do not apply",
			e.tag, ic.pos+1, ic.chain.tag, strings.Join(removed, ", "))
		s := strips[ic.chain]
		if s == nil {
			s = &chainSocketStrip{chain: ic.chain, fields: map[string]bool{}}
			strips[ic.chain] = s
			*stripOrder = append(*stripOrder, ic.chain)
		}
		s.nodes = append(s.nodes, e.tag)
		for _, f := range removed {
			s.fields[f] = true
		}
	}
	return changed
}

// chainDetourWalkHits идёт по собственному detour узла leaf (через группы — по
// листьям) и возвращает первый встреченный узел, перехваченный цепочкой c.
// Узел, перехваченный другой цепочкой, дальше не ведёт: его detour ядро
// заменит хопом.
func chainDetourWalkHits(leaf *graphEntry, c *graphEntry, byTag map[string]*graphEntry, intercepted map[*graphEntry]chainIntercept) *graphEntry {
	seen := map[*graphEntry]bool{leaf: true}
	queue := []*graphEntry{leaf}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		d, _ := cur.m["detour"].(string)
		if d == "" {
			continue
		}
		for _, next := range chainPositionLeaves(d, byTag) {
			if seen[next] {
				continue
			}
			seen[next] = true
			if ic, ok := intercepted[next]; ok {
				if ic.chain == c {
					return next
				}
				continue
			}
			queue = append(queue, next)
		}
	}
	return nil
}

// chainSocketStripWarnings — записи отчёта о снятых полях К5, по одной на
// цепочку.
func chainSocketStripWarnings(cache *ParsedCache, strips map[*graphEntry]*chainSocketStrip, order []*graphEntry) []GraphWarning {
	out := make([]GraphWarning, 0, len(order))
	for _, c := range order {
		s := strips[c]
		if s == nil || c.dropped {
			continue
		}
		fields := make([]string, 0, len(s.fields))
		for f := range s.fields {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		tags := strings.Join(s.nodes, ", ")
		if len(s.nodes) > 5 {
			tags = strings.Join(s.nodes[:5], ", ") + ", …"
		}
		origin := cache.NodeOrigins[c.tag]
		out = append(out, GraphWarning{
			SourceID:    origin.SourceID,
			SourceLabel: origin.SourceLabel,
			Subject:     c.tag,
			Code:        "chain_hop_socket_fields_dropped",
			Params: map[string]string{
				"chain":  c.tag,
				"count":  strconv.Itoa(len(s.nodes)),
				"tags":   tags,
				"fields": strings.Join(fields, ", "),
			},
			Reason: "fields " + strings.Join(fields, ", ") + " removed from " + strconv.Itoa(len(s.nodes)) + " node(s) of chain " + c.tag + " (" + tags + ")",
		})
	}
	return out
}
