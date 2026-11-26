// File chain_validate.go — предупреждения о цепочке, которые нельзя вывести
// из неё одной (SPEC 110, фаза 4).
//
// От ChainEmitError отличаются тем, что смотрят НЕ на цепочку, а на то, из
// чего она составлена: какие узлы стоят на позициях и что у них внутри.
// Эмиттер этого не знает и знать не должен — он получает уже отобранный
// набор тегов.
package config

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// ChainNestedConflict — теги вложенных цепочек, стоящих не на позиции 0.
//
// Ядро допускает вложенную цепочку только первой позицией
// (`protocol/chain/chain.go:279`): звено — это «узел через предыдущую
// позицию», а цепочка не узел, её нельзя пересобрать под чужой диалер.
func ChainNestedConflict(c *configtypes.SourceChain, chainTags map[string]bool) []string {
	if c == nil || len(chainTags) == 0 {
		return nil
	}
	var out []string
	hops := c.HopsOrNil()
	for i := 1; i < len(hops); i++ {
		if chainTags[hops[i]] {
			out = append(out, hops[i])
		}
	}
	return out
}

// ChainLayerTag — служебный тег префикса цепочки: путь от клиента до позиции
// pos включительно.
//
// Парен ChainInternalTag: там эти теги распознаются, чтобы не пустить их в
// конфиг, здесь — собираются, чтобы спросить у ядра задержку. Схема имени
// принадлежит ядру (`protocol/chain`), и держать её в одном файле с
// распознавателем обязательно: разойдись они — проба молча мерила бы не то.
func ChainLayerTag(chainTag string, pos int) string {
	return chainTag + "#" + strconv.Itoa(pos)
}

// ChainInternalTag — тег вида `<chain>#<i>`, который цепочка резервирует под
// свои звенья.
//
// Такие теги существуют только в рантайме ядра: в конфиге их нет, целью
// правила они быть не могут, и предлагать их пользователю — значит дать
// собрать конфиг со ссылкой в никуда (SPEC 110 T6).
//
// Пробел перед `#` исключает совпадение: публичные подписки сплошь и рядом
// называют узлы «Germany #1», и без этой оговорки живой узел пропадал бы из
// кандидатов позиций и целей правил. Имя цепочки обрезается по краям при
// сохранении, поэтому настоящий служебный тег пробела перед `#` не несёт.
func ChainInternalTag(tag string) bool {
	i := strings.LastIndex(tag, "#")
	if i <= 0 || i == len(tag)-1 {
		return false
	}
	if tag[i-1] == ' ' || tag[i-1] == '\t' {
		return false
	}
	for _, r := range tag[i+1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ChainBuiltinHopTags — служебные теги шаблона, которые форма предлагает
// позициями цепочки и которые ResolveChainSources обязан считать известными:
// шаблонные константы подмешиваются в конфиг только на финальной сборке, и
// без этого списка форма и сборка расходились бы («direct-out» предложен —
// «direct-out» не найден).
var ChainBuiltinHopTags = []string{"direct-out"}

// ChainDetourProblem — то, на чём ядро с перезаписью цепочки в detour (SPEC 165
// форка) отвергнет весь конфиг. Chain — цепочка, которую надо снять (или
// исправить в редакторе); Code и Params — запись реестра warnings.json.
type ChainDetourProblem struct {
	Chain  string
	Code   string
	Params map[string]string
	Reason string
}

// chainHopLocalPortFields — фиксированные локальные порты по типу узла
// (ALGORITHM К5 ядра): под detour сокет узлу открывает хоп.
var chainHopLocalPortFields = map[string][]string{
	"wireguard":   {"listen_port"},
	"tailscale":   {"listen_port", "relay_server_port"},
	"openconnect": {"dtls_local_port"},
}

// ChainDetourProblems проверяет граф исходящих (outbounds и endpoints в форме
// конфига, в порядке записей) по правилам перезаписи цепочки ядра (SPEC 165,
// сверено с ядром d9fc75468) и возвращает проблемы в порядке цепочек. Одна и
// та же функция зовётся сборкой (снимает цепочку) и редактором цепочки
// (показывает причину до сохранения), чтобы они не разошлись.
//
//   - одно место на тег: узел или группа, достижимые с позиций цепочек (все
//     члены групп любой вложенности, кроме direct и block), стоят в одном
//     месте — не в двух цепочках (позиция 0 тоже считается) и не на двух
//     позициях ≥ 1 одной цепочки; снимается поздняя из двух цепочек;
//   - кольцо: узел позиции ≥ 1 ходит через позицию ниже, и если оттуда (по
//     членам групп, detour и позициям цепочек) путь возвращается к нему —
//     кольцо (`[A, Группа{A, X}]`, свой detour узла позиции 0 в узел выше);
//   - фиксированный локальный порт у узла позиции ≥ 1;
//   - тег хопа `<цепочка>#<i>` занят;
//   - rewrite цепочки меняет detour.
func ChainDetourProblems(entries []map[string]interface{}) []ChainDetourProblem {
	byTag := make(map[string]map[string]interface{}, len(entries))
	var chains []map[string]interface{}
	for _, e := range entries {
		tag, _ := e["tag"].(string)
		if tag == "" {
			continue
		}
		byTag[tag] = e
		if t, _ := e["type"].(string); t == configtypes.ChainOutboundType {
			chains = append(chains, e)
		}
	}
	typeOf := func(tag string) string {
		t, _ := byTag[tag]["type"].(string)
		return t
	}
	strs := func(v interface{}) []string {
		arr, _ := v.([]interface{})
		out := make([]string, 0, len(arr))
		for _, x := range arr {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	isGroup := func(tag string) bool {
		t := typeOf(tag)
		return t == "selector" || t == "urltest"
	}
	// reach — тег и всё, что достижимо из него через члены групп.
	reach := func(tag string) []string {
		var out []string
		seen := map[string]bool{}
		var walk func(string)
		walk = func(t string) {
			if seen[t] {
				return
			}
			seen[t] = true
			out = append(out, t)
			if isGroup(t) {
				for _, m := range strs(byTag[t]["outbounds"]) {
					walk(m)
				}
			}
		}
		walk(tag)
		return out
	}
	counted := func(tag string) bool {
		if _, ok := byTag[tag]; !ok {
			return false // статика шаблона (direct-out): ядро её не перехватывает
		}
		t := typeOf(tag)
		return t != "direct" && t != "block"
	}

	var out []ChainDetourProblem
	dropped := map[string]bool{}
	add := func(chain, code, reason string, params map[string]string) {
		if dropped[chain] {
			return
		}
		dropped[chain] = true
		p := map[string]string{"chain": chain}
		for k, v := range params {
			p[k] = v
		}
		out = append(out, ChainDetourProblem{Chain: chain, Code: code, Params: p, Reason: reason})
	}

	// Собственные проверки цепочки: тег хопа, rewrite, локальный порт.
	for _, c := range chains {
		ctag, _ := c["tag"].(string)
		hops := strs(c["outbounds"])
		for i := range hops {
			if ht := ChainLayerTag(ctag, i); byTag[ht] != nil {
				add(ctag, "chain_hop_tag_taken", "tag “"+ht+"” is reserved for the chain hop but already used", map[string]string{"tag": ht})
			}
		}
		if rw, ok := c["rewrite"].(map[string]interface{}); ok {
			for typ, patch := range rw {
				if pm, ok := patch.(map[string]interface{}); ok {
					if _, has := pm["detour"]; has {
						add(ctag, "chain_rewrite_detour", "rewrite["+typ+"] cannot change detour", map[string]string{"type": typ})
					}
				}
			}
		}
		for pos := 1; pos < len(hops); pos++ {
			for _, t := range reach(hops[pos]) {
				if !counted(t) || isGroup(t) {
					continue
				}
				var fields []string
				for _, f := range chainHopLocalPortFields[typeOf(t)] {
					if v, has := byTag[t][f]; has && !chainZeroJSON(v) {
						fields = append(fields, f)
					}
				}
				if len(fields) > 0 {
					add(ctag, "chain_hop_local_port", "node “"+t+"” at position "+strconv.Itoa(pos+1)+" has "+strings.Join(fields, ", "),
						map[string]string{"node": t, "position": strconv.Itoa(pos + 1), "fields": strings.Join(fields, ", ")})
				}
			}
		}
	}

	// Одно место на тег.
	type place struct {
		chain string
		pos   int
		order int
	}
	places := map[string][]place{}
	for ci, c := range chains {
		ctag, _ := c["tag"].(string)
		if dropped[ctag] {
			continue
		}
		for pos, h := range strs(c["outbounds"]) {
			for _, t := range reach(h) {
				if !counted(t) {
					continue
				}
				dup := false
				for _, p := range places[t] {
					if p.chain == ctag && p.pos == pos {
						dup = true
					}
				}
				if !dup {
					places[t] = append(places[t], place{ctag, pos, ci})
				}
			}
		}
	}
	tags := make([]string, 0, len(places))
	for t := range places {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	for _, t := range tags {
		ps := places[t]
	pairs:
		for i := range ps {
			for j := i + 1; j < len(ps); j++ {
				a, b := ps[i], ps[j]
				if a.chain == b.chain && (a.pos == 0 || b.pos == 0) {
					continue // позиция 0 и позиция выше одной цепочки — кольцо, ниже
				}
				if a.chain == b.chain && a.pos == b.pos {
					continue
				}
				victim, other := b, a
				if a.order > b.order {
					victim, other = a, b
				}
				add(victim.chain, "chain_detour_conflict",
					"“"+t+"” is needed on chain “"+other.chain+"” position "+strconv.Itoa(other.pos+1)+" and on chain “"+victim.chain+"” position "+strconv.Itoa(victim.pos+1)+" — one node or group has one path",
					map[string]string{"node": t, "other": other.chain})
				break pairs
			}
		}
	}

	// Кольца: рёбра — члены групп, detour (у перехваченного узла — хоп, то
	// есть позиция ниже его цепочки) и позиции цепочек.
	intercept := map[string]string{} // узел → тег позиции ниже
	interceptChain := map[string]string{}
	for _, c := range chains {
		ctag, _ := c["tag"].(string)
		if dropped[ctag] {
			continue
		}
		hops := strs(c["outbounds"])
		for pos := 1; pos < len(hops); pos++ {
			for _, t := range reach(hops[pos]) {
				if counted(t) && !isGroup(t) && typeOf(t) != configtypes.ChainOutboundType {
					if _, ok := intercept[t]; !ok {
						intercept[t] = hops[pos-1]
						interceptChain[t] = ctag
					}
				}
			}
		}
	}
	next := func(tag string) []string {
		e := byTag[tag]
		if e == nil {
			return nil
		}
		var refs []string
		if isGroup(tag) || typeOf(tag) == configtypes.ChainOutboundType {
			refs = append(refs, strs(e["outbounds"])...)
		}
		if below, ok := intercept[tag]; ok {
			refs = append(refs, below)
		} else if d, _ := e["detour"].(string); d != "" {
			refs = append(refs, d)
		}
		return refs
	}
	reaches := func(from, target string) bool {
		seen := map[string]bool{}
		stack := []string{from}
		for len(stack) > 0 {
			t := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if t == target {
				return true
			}
			if seen[t] {
				continue
			}
			seen[t] = true
			stack = append(stack, next(t)...)
		}
		return false
	}
	ringNodes := make([]string, 0, len(intercept))
	for t := range intercept {
		ringNodes = append(ringNodes, t)
	}
	sort.Strings(ringNodes)
	for _, t := range ringNodes {
		if reaches(intercept[t], t) {
			add(interceptChain[t], "chain_detour_ring",
				"node “"+t+"” goes through the position below it, and that path leads back to “"+t+"” — a ring",
				map[string]string{"node": t})
		}
	}
	return out
}

// chainZeroJSON — значение, которое ядро считает незаданным (0, "", false,
// пустой список или объект).
func chainZeroJSON(v interface{}) bool {
	switch x := v.(type) {
	case nil:
		return true
	case bool:
		return !x
	case string:
		return x == ""
	case float64:
		return x == 0
	case json.Number:
		return x.String() == "0"
	case []interface{}:
		return len(x) == 0
	case map[string]interface{}:
		return len(x) == 0
	}
	return false
}

// ChainEditProblems — проблемы перезаписи цепочки для редактора: граф последней
// сборки (строки OutboundsJSON/EndpointsJSON модели), в котором запись
// редактируемой цепочки (по прежнему тегу originalTag) заменена объектом chain
// из формы. Возвращает тексты реестра на языке UI для проблем, где цепочка —
// снимаемая или вторая сторона конфликта.
func ChainEditProblems(outboundsJSON, endpointsJSON []string, originalTag string, chain map[string]interface{}) []string {
	tag, _ := chain["tag"].(string)
	var graph []map[string]interface{}
	replaced := false
	parse := func(lines []string) {
		for _, line := range lines {
			body := line
			if i := strings.Index(body, "{"); i >= 0 {
				body = body[i:]
			}
			body = strings.TrimRight(strings.TrimSpace(body), ",")
			var m map[string]interface{}
			if json.Unmarshal([]byte(body), &m) != nil {
				continue
			}
			if t, _ := m["tag"].(string); t != "" && (t == originalTag || t == tag) {
				if !replaced {
					graph = append(graph, chain)
					replaced = true
				}
				continue
			}
			graph = append(graph, m)
		}
	}
	parse(outboundsJSON)
	parse(endpointsJSON)
	if !replaced {
		graph = append(graph, chain)
	}
	var out []string
	for _, p := range ChainDetourProblems(graph) {
		if p.Chain != tag && p.Params["other"] != tag {
			continue
		}
		out = append(out, registryWarningText(p.Code, p.Params, p.Reason))
	}
	return out
}
