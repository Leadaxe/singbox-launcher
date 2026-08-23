// File outbound_graph_sanitize.go — финальный рубеж перед эмиссией
// outbound-секций: один проход по ГРАФУ зависимостей вместо трёх частных
// проверок, живших в непересекающихся подграфах.
//
// У ядра рёбра зависимостей одни — member группы, detour узла, позиция
// цепочки, — и любая висячая ссылка или кольцо в этом графе фатальны для
// конфига ЦЕЛИКОМ: `sing-box check` отвергает файл с сообщением, которое
// указывает не на виновника, а на первого, кто на него сослался. Частные
// проверки выше по конвейеру (sanitizeNodeDetours, dropChainsThroughDirection,
// dropNodesDetouringThroughGroup) ловят каждая свой класс, но не транзит
// через рёбра ЧУЖОГО вида: узел, задетуренный на группу через промежуточный
// узел; цепочка, чей хоп исчез уже ПОСЛЕ её разрешения (hash-detour дропнул
// источник); группа, у которой после каскада удалений не осталось участников.
//
// Здесь — последняя точка, где виден весь граф целиком, поэтому политика
// одна и окончательная: деградировать один элемент с warning, а не отдать
// ядру конфиг, который оно отвергнет.
//
// Правила:
//  1. detour на несуществующий тег — ключ удаляется (узел ходит напрямую);
//  2. группа: участники-призраки исключаются; пустая группа удаляется;
//     default вне состава заменяется первым участником;
//  3. цепочка: позиция на несуществующий тег ИЛИ другая цепочка на позиции
//     ≥ 1 — цепочка удаляется целиком (маршрут без хопа — другой маршрут,
//     см. WIZARD_STATE.md);
//  4. группа, стоящая позицией ≥ 1 какой-либо цепочки (транзитивно, через
//     вложенные группы), не содержит цепочек: ядро обходит ЛИСТЬЯ группы на
//     старте и отвергает вложенную цепочку («nested chain is only allowed at
//     position 0») — check это не ловит, падает только run;
//  5. кольцо по любым рёбрам (detour → member → позиция цепочки → …)
//     разрывается по ребру, замкнувшему цикл.
//
// Удаление узла может делать висячими новые ссылки — проход повторяется до
// фикспойнта. finalTags мутируется на месте: секция route собирается позже
// и обязана видеть уже итоговое множество тегов.
package build

import (
	"encoding/json"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// graphEntry — разобранная запись кэша (outbound или endpoint).
type graphEntry struct {
	prefix     string // `// comment`-префикс легаси-записей
	m          map[string]interface{}
	raw        json.RawMessage // исходная строка для неразбираемых записей
	tag        string
	isEndpoint bool
	dirty      bool
	dropped    bool
}

func (e *graphEntry) typ() string {
	t, _ := e.m["type"].(string)
	return t
}

func (e *graphEntry) isGroup() bool {
	t := e.typ()
	return t == "selector" || t == "urltest"
}

func (e *graphEntry) isChain() bool { return e.typ() == "chain" }

// members возвращает срез строковых ссылок из m["outbounds"].
func (e *graphEntry) members() []string {
	arr, _ := e.m["outbounds"].([]interface{})
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func (e *graphEntry) setMembers(members []string) {
	arr := make([]interface{}, len(members))
	for i, s := range members {
		arr[i] = s
	}
	e.m["outbounds"] = arr
	e.dirty = true
}

// sanitizeOutboundGraph — точка входа: принимает кэш и множество финальных
// тегов (template static + generated), возвращает кэш с вычищенным графом.
// finalTags мутируется: теги удалённых узлов исключаются.
func sanitizeOutboundGraph(cache *ParsedCache, finalTags map[string]bool) *ParsedCache {
	if cache == nil || (len(cache.Outbounds) == 0 && len(cache.Endpoints) == 0) {
		return cache
	}

	entries := make([]*graphEntry, 0, len(cache.Outbounds)+len(cache.Endpoints))
	parse := func(raw json.RawMessage, isEndpoint bool) {
		prefix, jsonPart := splitEntryComment(string(raw))
		body := strings.TrimRight(strings.TrimSpace(jsonPart), ",")
		e := &graphEntry{prefix: prefix, raw: raw, isEndpoint: isEndpoint}
		if body != "" {
			dec := json.NewDecoder(strings.NewReader(body))
			dec.UseNumber()
			var m map[string]interface{}
			if err := dec.Decode(&m); err == nil {
				e.m = m
				e.tag, _ = m["tag"].(string)
			}
		}
		entries = append(entries, e)
	}
	for _, raw := range cache.Outbounds {
		parse(raw, false)
	}
	for _, raw := range cache.Endpoints {
		parse(raw, true)
	}

	byTag := make(map[string]*graphEntry, len(entries))
	for _, e := range entries {
		if e.m != nil && e.tag != "" {
			byTag[e.tag] = e
		}
	}

	drop := func(e *graphEntry, why string) {
		if e.dropped {
			return
		}
		e.dropped = true
		delete(finalTags, e.tag)
		debuglog.WarnLog("build: outbound %q удалён из конфига: %s", e.tag, why)
	}

	// Fixpoint: каждое исправление может открыть следующее. Верхняя граница —
	// защитная: каждый содержательный проход удаляет узел или ребро, их
	// конечное число.
	for iter := 0; iter < len(entries)*4+8; iter++ {
		changed := false
		for _, e := range entries {
			if e.dropped || e.m == nil {
				continue
			}
			if sanitizeEntryRefs(e, byTag, finalTags, drop) {
				changed = true
			}
		}
		if pruneChainLeavesUnderGroups(entries, byTag) {
			changed = true
		}
		if breakDependencyCycle(entries, byTag, drop) {
			changed = true
		}
		if !changed {
			break
		}
	}

	out := &ParsedCache{}
	*out = *cache
	out.Outbounds = rebuildEntries(entries, false, true)
	out.Endpoints = rebuildEntries(entries, true, false)
	return out
}

// sanitizeEntryRefs — правила 1–3 для одной записи. Возвращает true, если
// что-то изменилось.
func sanitizeEntryRefs(e *graphEntry, byTag map[string]*graphEntry, finalTags map[string]bool, drop func(*graphEntry, string)) bool {
	changed := false

	if d, _ := e.m["detour"].(string); d != "" && !finalTags[d] {
		debuglog.WarnLog("build: node %q detour %q not found in final config — dropping detour (direct dial)", e.tag, d)
		delete(e.m, "detour")
		e.dirty = true
		changed = true
	}

	switch {
	case e.isChain():
		for i, ref := range e.members() {
			if !finalTags[ref] {
				drop(e, "позиция «"+ref+"» не существует в финальном конфиге — маршрут без хопа был бы другим маршрутом")
				return true
			}
			if i >= 1 {
				// Прямая ссылка на другую цепочку законна только позицией 0
				// (инвариант ядра, protocol/chain). Группы с цепочками в
				// листьях чистит pruneChainLeavesUnderGroups.
				if t := byTag[ref]; t != nil && !t.dropped && t.isChain() {
					drop(e, "цепочка «"+ref+"» на позиции ≥ 1 — ядро допускает вложенную цепочку только первой позицией")
					return true
				}
			}
		}
	case e.isGroup():
		members := e.members()
		kept := members[:0]
		var lost []string
		for _, ref := range members {
			if finalTags[ref] {
				kept = append(kept, ref)
			} else {
				lost = append(lost, ref)
			}
		}
		if len(lost) > 0 {
			debuglog.WarnLog("build: группа %q: участники %v не существуют в финальном конфиге — исключены из состава", e.tag, lost)
			e.setMembers(kept)
			changed = true
		}
		if len(kept) == 0 {
			drop(e, "не осталось ни одного участника")
			return true
		}
		if def, _ := e.m["default"].(string); def != "" && !containsString(kept, def) {
			debuglog.WarnLog("build: группа %q: default %q вне состава — заменён на %q", e.tag, def, kept[0])
			e.m["default"] = kept[0]
			e.dirty = true
			changed = true
		}
	}
	return changed
}

// pruneChainLeavesUnderGroups — правило 4: множество групп, достижимых как
// позиция ≥ 1 какой-либо цепочки (транзитивно через вложенные группы), не
// должно содержать цепочек в участниках.
func pruneChainLeavesUnderGroups(entries []*graphEntry, byTag map[string]*graphEntry) bool {
	// Стартовое множество: группы, на которые ссылаются позиции ≥ 1.
	queue := make([]string, 0, 8)
	seen := make(map[string]bool, 8)
	for _, e := range entries {
		if e.dropped || e.m == nil || !e.isChain() {
			continue
		}
		for i, ref := range e.members() {
			if i == 0 {
				continue
			}
			if t := byTag[ref]; t != nil && !t.dropped && t.isGroup() && !seen[ref] {
				seen[ref] = true
				queue = append(queue, ref)
			}
		}
	}
	changed := false
	for len(queue) > 0 {
		tag := queue[0]
		queue = queue[1:]
		g := byTag[tag]
		if g == nil || g.dropped {
			continue
		}
		members := g.members()
		kept := members[:0]
		var lost []string
		for _, ref := range members {
			t := byTag[ref]
			if t != nil && !t.dropped && t.isChain() {
				lost = append(lost, ref)
				continue
			}
			if t != nil && !t.dropped && t.isGroup() && !seen[ref] {
				seen[ref] = true
				queue = append(queue, ref)
			}
			kept = append(kept, ref)
		}
		if len(lost) > 0 {
			debuglog.WarnLog("build: группа %q стоит позицией ≥ 1 цепочки — цепочки %v исключены из её состава (ядро допускает вложенную цепочку только первой позицией)", tag, lost)
			g.setMembers(kept)
			changed = true
			// Пустая группа и default вне состава доработаются правилом 2
			// на следующей итерации фикспойнта.
		}
	}
	return changed
}

// breakDependencyCycle — правило 5: DFS по рёбрам detour/member/позиция.
// Разрывает ПЕРВОЕ найденное кольцо (по ребру, замкнувшему цикл) и
// возвращает true; фикспойнт-цикл вызовет её снова, пока колец не останется.
func breakDependencyCycle(entries []*graphEntry, byTag map[string]*graphEntry, drop func(*graphEntry, string)) bool {
	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := make(map[string]int, len(entries))

	type edge struct {
		from *graphEntry
		ref  string
		kind string // "detour" | "member" | "chain"
	}
	var found *edge

	type tref struct{ ref, kind string }

	var visit func(e *graphEntry) bool
	visit = func(e *graphEntry) bool {
		color[e.tag] = grey
		memberKind := "member"
		if e.isChain() {
			memberKind = "chain"
		}
		refs := make([]tref, 0, 8)
		for _, r := range e.members() {
			refs = append(refs, tref{r, memberKind})
		}
		if d, _ := e.m["detour"].(string); d != "" {
			refs = append(refs, tref{d, "detour"})
		}
		for _, r := range refs {
			t := byTag[r.ref]
			if t == nil || t.dropped || t.m == nil {
				continue
			}
			switch color[r.ref] {
			case grey:
				found = &edge{from: e, ref: r.ref, kind: r.kind}
				return true
			case white:
				if visit(t) {
					return true
				}
			}
		}
		color[e.tag] = black
		return false
	}

	for _, e := range entries {
		if e.dropped || e.m == nil || e.tag == "" {
			continue
		}
		if color[e.tag] == white {
			if visit(e) {
				break
			}
		}
	}
	if found == nil {
		return false
	}

	switch found.kind {
	case "detour":
		debuglog.WarnLog("build: кольцо зависимостей через detour %q → %q — detour снят (узел ходит напрямую)", found.from.tag, found.ref)
		delete(found.from.m, "detour")
		found.from.dirty = true
	case "chain":
		drop(found.from, "кольцо зависимостей через позицию «"+found.ref+"»")
	default: // member
		members := found.from.members()
		kept := members[:0]
		for _, ref := range members {
			if ref != found.ref {
				kept = append(kept, ref)
			}
		}
		debuglog.WarnLog("build: кольцо зависимостей: %q исключён из состава группы %q", found.ref, found.from.tag)
		found.from.setMembers(kept)
	}
	return true
}

// rebuildEntries собирает записи обратно в срез кэша.
func rebuildEntries(entries []*graphEntry, endpoints bool, compact bool) []json.RawMessage {
	var out []json.RawMessage
	for _, e := range entries {
		if e.isEndpoint != endpoints {
			continue
		}
		if e.dropped {
			continue
		}
		if e.m == nil || !e.dirty {
			out = append(out, e.raw)
			continue
		}
		var rebuilt []byte
		var err error
		if compact {
			rebuilt, err = json.Marshal(e.m)
		} else {
			rebuilt, err = json.MarshalIndent(e.m, "", IndentBase)
		}
		if err != nil {
			out = append(out, e.raw)
			continue
		}
		out = append(out, json.RawMessage(e.prefix+string(rebuilt)))
	}
	return out
}
