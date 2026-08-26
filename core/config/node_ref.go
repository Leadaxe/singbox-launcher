package config

import (
	"fmt"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// Ссылки на узлы (SPEC 101 → SPEC 112 → SPEC 112-A) и строгая двухпроходность
// сборки.
//
// # Ссылка — объект, а не тег
//
// Внешняя ссылка на конкретный узел хранится ПАРОЙ:
//
//	source_id — ULID источника-цели (ProxySource.ID);
//	tag       — identity-тег узла ВНУТРИ этого источника (ParsedNode.IdentityTag).
//
// Финальный конфиговый тег в состоянии не хранится: он производная от
// tag_prefix / tag_postfix / tag_mask источника-цели и от глобальной
// уникализации, то есть меняется от правок, к самому узлу отношения не
// имеющих. Хранимый тег был бы кэшем, который протухает от смены префикса,
// переименования источника или node_tag — ровно тем же классом багов, из-за
// которого до SPEC 112 протухал контент-хеш. Вычисление не протухает.
//
// # Двухпроходность — инвариант сборки
//
// Проход 1 (GenerateOutboundsFromParserConfig, цикл по источникам): каждый
// узел получает ФИНАЛЬНЫЙ configTag — prefix/mask/uniquify. Ни одна ссылка на
// этом проходе не резолвится, потому что тег цели может быть ещё не вычислен:
// источник-цель имеет право стоять в списке НИЖЕ ссылающегося.
//
// Проход 2 (этот файл): строится nodeRefIndex — полная карта
// «источник + identity-тег → узел», — и ТОЛЬКО после неё материализуются
// ссылки.
//
// Правило на будущее (родственно build-graph-sanitizer, где все рёбра
// проверяются одним финальным проходом): любой новый вид ссылки на узел обязан
// резолвиться через nodeRefIndex на проходе 2. Резолв из тела прохода 1 —
// ошибка по построению: карты там ещё нет, а частный поиск «по тегу прямо
// сейчас» даст правильный ответ на всех конфигах, кроме тех, где источники
// стоят в неудобном порядке.

// nodeRefIndex — карта резолва ссылок на узлы. Строится ОДИН раз, на проходе 2
// сборки, когда финальные теги всех узлов уже вычислены.
//
// Заводить её раньше нельзя, и это не стилистика: до конца прохода 1 часть
// узлов ещё не имеет финального тега.
type nodeRefIndex struct {
	// bySourceID — узлы источника по его ULID; внутри — по identity-тегу.
	// Идентичность уникальна в рамках источника (SPEC 112), поэтому ключ
	// составной именно так, а не глобально.
	bySourceID map[string]map[string]*ParsedNode
	// singleNodeBySourceID — источники, давшие РОВНО один узел (server, chain,
	// подписка из одной ноды). Нужен для миграции переходных состояний, где
	// identity-тег ссылки записан не был.
	singleNodeBySourceID map[string]*ParsedNode
	// byFinalTag — глобальный поиск по ФИНАЛЬНОМУ тегу. Путь для ссылок без
	// source_id: состояний dev-сборок между SPEC 112 и 112-A и результата
	// label-fallback миграции с упразднённого хеша.
	byFinalTag map[string]*ParsedNode
	// sourceIndexByID — позиция источника в ParserConfig.Proxies, чтобы
	// отличить «источник удалён» от «узел в источнике не найден»: тексты
	// ошибок у этих случаев разные (SPEC 112-A, «Понятные ошибки»).
	sourceIndexByID map[string]int
}

// buildNodeRefIndex — единственное место, где строится карта резолва.
//
// Узлы-группы (SchemeGroup) кандидатами не являются: дозвон через selector —
// задача DetourTag (SPEC 077), и идентичности у групп нет (SPEC 112).
func buildNodeRefIndex(parserConfig *ParserConfig, nodesBySource map[int][]*ParsedNode, allNodes []*ParsedNode) *nodeRefIndex {
	idx := &nodeRefIndex{
		bySourceID:           make(map[string]map[string]*ParsedNode),
		singleNodeBySourceID: make(map[string]*ParsedNode),
		byFinalTag:           make(map[string]*ParsedNode, len(allNodes)),
		sourceIndexByID:      make(map[string]int),
	}
	if parserConfig == nil {
		return idx
	}

	for i, ps := range parserConfig.ParserConfig.Proxies {
		id := strings.TrimSpace(ps.ID)
		if id == "" {
			continue // источник без ULID адресовать нечем (тесты, ручной JSON)
		}
		// Первый выигрывает: одинаковые ULID у двух источников — битое
		// состояние, но резолв обязан остаться детерминированным.
		if _, dup := idx.sourceIndexByID[id]; dup {
			debuglog.WarnLog("Parser: два источника с одним id — ссылки разрешаются по первому (%s)", sourceDisplayName(ps, i))
			continue
		}
		idx.sourceIndexByID[id] = i

		byIdentity := make(map[string]*ParsedNode)
		var only *ParsedNode
		count := 0
		for _, n := range nodesBySource[i] {
			if n == nil || n.Scheme == SchemeGroup {
				continue
			}
			count++
			only = n
			id := nodeIdentityTag(n)
			if id == "" {
				continue
			}
			if _, dup := byIdentity[id]; !dup {
				byIdentity[id] = n
			}
		}
		idx.bySourceID[id] = byIdentity
		if count == 1 {
			idx.singleNodeBySourceID[id] = only
		}
	}

	for _, n := range allNodes {
		if n == nil || n.Scheme == SchemeGroup || n.Tag == "" {
			continue
		}
		if _, dup := idx.byFinalTag[n.Tag]; !dup {
			idx.byFinalTag[n.Tag] = n
		}
	}
	return idx
}

// nodeIdentityTag — identity-тег узла (SPEC 112) с тем же запасным правилом,
// что и NodeIdentity: узлы, собранные не парсером источника (цепочки, ручные
// конфиги в тестах), идентифицируются собственным тегом.
func nodeIdentityTag(n *ParsedNode) string {
	if n == nil || n.Scheme == SchemeGroup {
		return ""
	}
	if id := strings.TrimSpace(n.IdentityTag); id != "" {
		return id
	}
	return strings.TrimSpace(n.Tag)
}

// nodeRefResolution — исход резолва одной ссылки.
type nodeRefResolution struct {
	node *ParsedNode
	// problem — человекочитаемая причина, если узел не найден. Пусто у успеха.
	problem string
}

// resolve материализует ссылку «source_id + identity-тег» в узел.
//
// Резолв СТРОГИЙ (дополнение к SPEC 112-A, отменяющее послабление «id
// главнее»): при непустом source_id обязаны сойтись ОБЕ части. Расхождение —
// не повод чинить ссылку молча: узел под другим именем это другой узел, и
// подставить его значило бы пустить трафик источника через хоп, которого
// пользователь не выбирал. За честность отвечает UI: операция, меняющая
// идентичность узла, сама сбрасывает ссылки на него и говорит об этом.
//
// Пустой source_id — переходная форма (состояния dev-сборок между SPEC 112 и
// 112-A, а также label-fallback миграции с упразднённого хеша): тег трактуется
// как ФИНАЛЬНЫЙ и ищется глобально, дословно как в SPEC 112.
func (idx *nodeRefIndex) resolve(sourceID, tag string) nodeRefResolution {
	sourceID = strings.TrimSpace(sourceID)
	tag = strings.TrimSpace(tag)

	if sourceID == "" {
		if tag == "" {
			return nodeRefResolution{problem: "ссылка пуста"}
		}
		if n := idx.byFinalTag[tag]; n != nil {
			return nodeRefResolution{node: n}
		}
		return nodeRefResolution{problem: fmt.Sprintf("узла %q нет среди узлов конфига", tag)}
	}

	nodes, known := idx.bySourceID[sourceID]
	if !known {
		return nodeRefResolution{problem: "источник ссылки не найден"}
	}
	if tag == "" {
		// Ссылка без identity-тега на источник из одного узла — им она и есть.
		// Так лечится переходное состояние, где записан лишь source_id.
		if only := idx.singleNodeBySourceID[sourceID]; only != nil {
			return nodeRefResolution{node: only}
		}
		return nodeRefResolution{problem: "в ссылке нет тега узла"}
	}
	if n := nodes[tag]; n != nil {
		return nodeRefResolution{node: n}
	}
	return nodeRefResolution{problem: fmt.Sprintf("узла %q в нём нет", tag)}
}

// sourceDisplayName — как источник зовут в сообщениях (SPEC 112-A, «Понятные
// ошибки»): сначала подпись, за ней тег узла (server/chain), за ним URL
// подписки. ULID в текст не выносится — по нему источник не узнать; он идёт в
// сообщение, только когда другого имени нет вовсе.
func sourceDisplayName(ps ProxySource, index int) string {
	if s := strings.TrimSpace(ps.Label); s != "" {
		return s
	}
	if s := strings.TrimSpace(ps.TagMask); s != "" {
		return s
	}
	if s := strings.TrimSpace(ps.Source); s != "" {
		return s
	}
	if len(ps.Connections) > 0 {
		if s := strings.TrimSpace(ps.Connections[0]); s != "" {
			return s
		}
	}
	if s := strings.TrimSpace(ps.ID); s != "" {
		return "id " + s
	}
	return fmt.Sprintf("#%d", index+1)
}

// detourTargetName — как в сообщении зовут ИСТОЧНИК-ЦЕЛЬ ссылки, когда сам
// источник в конфиге ещё есть; иначе остаётся лишь ULID.
func (idx *nodeRefIndex) detourTargetName(parserConfig *ParserConfig, sourceID string) string {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return ""
	}
	if i, ok := idx.sourceIndexByID[sourceID]; ok && parserConfig != nil &&
		i < len(parserConfig.ParserConfig.Proxies) {
		return sourceDisplayName(parserConfig.ParserConfig.Proxies[i], i)
	}
	return "id " + sourceID
}
