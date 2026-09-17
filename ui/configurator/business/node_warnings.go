// File node_warnings.go — предупреждения узла, адресуемые ФИНАЛЬНЫМ тегом
// (SPEC 131 §6, волна W3).
//
// # Зачем слой
//
// Вкладка Servers, окно Core runtime и окно Info живут на `api.ProxyInfo` из
// Clash API, а тот знает про узел ровно одно имя — тег, под которым узел
// уехал в конфиг. Предупреждения же живут в `state.json`, у записи узла, и
// адресуются СЫРЫМ тегом в рамках источника (идентичность, SPEC 112).
// Между ними стоит тег-машина: `norm(prefix + сырой + postfix)`.
//
// Отсюда индекс: один проход по состоянию строит карту «финальный тег →
// предупреждения», и три поверхности спрашивают её тем именем, которое у них
// на руках. Второй реализации этого перевода не заводится — политика тегов
// уже умеет считать финальный тег (`state.NodeLinkFinalTag`), и повторять её
// правила здесь значило бы завести четвёртую копию тег-машины.
//
// # Чего индекс НЕ делает
//
// Глобальной уникализации (суффикс при столкновении тегов) он не повторяет:
// её раскрывает только сборка. Узел, чей тег столкнулся с чужим, в индексе
// не найдётся, и строка останется без ⚠ — молчание вместо ЧУЖОГО
// предупреждения. Показать деградацию не на том узле было бы хуже, чем не
// показать её вовсе.
//
// Кэш — по (путь, mtime, размер) state.json, тот же приём, что у
// LoadConfigNodes: список серверов зовёт индекс на каждую строку, и разбор
// состояния на каждую перерисовку съел бы список из пятисот узлов.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package business

import (
	"os"
	"strings"
	"sync"

	"singbox-launcher/core/state"
)

// NodeWarningIndex — снимок предупреждений состояния по финальному тегу.
type NodeWarningIndex struct {
	byTag map[string][]state.NodeWarning
}

// Lookup — предупреждения узла с таким финальным тегом; nil, если их нет.
//
// Отсутствие записи — не ошибка: Clash API отдаёт и служебные outbound'ы, и
// теги, которых в состоянии не было никогда (Направления, группы).
func (idx *NodeWarningIndex) Lookup(tag string) []state.NodeWarning {
	if idx == nil || idx.byTag == nil {
		return nil
	}
	return idx.byTag[strings.TrimSpace(tag)]
}

// Len — сколько узлов состояния несут предупреждения.
func (idx *NodeWarningIndex) Len() int {
	if idx == nil {
		return 0
	}
	return len(idx.byTag)
}

type nodeWarnCacheEntry struct {
	modTime int64
	size    int64
	parsed  *NodeWarningIndex
}

var nodeWarnCache struct {
	mu     sync.RWMutex
	byPath map[string]nodeWarnCacheEntry
}

// LoadNodeWarnings читает state.json по пути path и строит индекс.
//
// При любой ошибке — пустой индекс, а не nil: вызывающий делает Lookup без
// проверок, и строка просто остаётся без пометки.
func LoadNodeWarnings(path string) *NodeWarningIndex {
	path = strings.TrimSpace(path)
	if path == "" {
		return &NodeWarningIndex{}
	}
	info, err := os.Stat(path)
	if err != nil {
		return &NodeWarningIndex{}
	}
	modTime, size := info.ModTime().UnixNano(), info.Size()

	nodeWarnCache.mu.RLock()
	if e, ok := nodeWarnCache.byPath[path]; ok &&
		e.parsed != nil && e.modTime == modTime && e.size == size {
		nodeWarnCache.mu.RUnlock()
		return e.parsed
	}
	nodeWarnCache.mu.RUnlock()

	parsed := buildNodeWarningIndex(path)

	nodeWarnCache.mu.Lock()
	if nodeWarnCache.byPath == nil {
		nodeWarnCache.byPath = make(map[string]nodeWarnCacheEntry, 2)
	}
	nodeWarnCache.byPath[path] = nodeWarnCacheEntry{
		modTime: modTime, size: size, parsed: parsed,
	}
	nodeWarnCache.mu.Unlock()
	return parsed
}

// InvalidateNodeWarningsCache сбрасывает кэш индекса.
//
// Кэш инвалидируется ПАРОЙ с кэшем разбора конфига (память
// `cache-invalidation-pairs`): обе проекции снимает одно событие —
// пересборка, — и забытый сброс одной из них даёт вечное «предупреждений
// нет» рядом со свежим составом.
func InvalidateNodeWarningsCache() {
	nodeWarnCache.mu.Lock()
	nodeWarnCache.byPath = nil
	nodeWarnCache.mu.Unlock()
}

// buildNodeWarningIndex — один проход по источникам состояния.
func buildNodeWarningIndex(path string) *NodeWarningIndex {
	s, err := state.Load(path)
	if err != nil || s == nil {
		return &NodeWarningIndex{}
	}
	idx := &NodeWarningIndex{byTag: make(map[string][]state.NodeWarning, 8)}
	for i := range s.Sources {
		src := &s.Sources[i]
		// Узловой источник (server/chain/auto): состава нет, узел и есть
		// источник, а тег-политика к самому себе не применяется.
		if len(src.Nodes) == 0 {
			idx.put(strings.TrimSpace(src.Tag), src.Warnings)
			continue
		}
		for j := range src.Nodes {
			n := &src.Nodes[j]
			if len(n.Warnings) == 0 {
				continue
			}
			final, ok := state.NodeLinkFinalTag(src.TagPolicy, n.Tag)
			if !ok {
				// Политика с переменными ({$num}): финальный тег раскрывает
				// только сборка. Молчим — см. шапку файла.
				continue
			}
			idx.put(final, n.Warnings)
		}
	}
	return idx
}

// put — записать, не затирая первого владельца тега.
//
// Столкновение финальных тегов разрешает сборка суффиксом, и который из двух
// узлов стал в конфиге этим тегом, здесь не известно. Первый по порядку
// состояния — то же правило, по которому эмиссия раздаёт слоты.
func (idx *NodeWarningIndex) put(tag string, warns []state.NodeWarning) {
	if tag == "" || len(warns) == 0 {
		return
	}
	if _, dup := idx.byTag[tag]; dup {
		return
	}
	idx.byTag[tag] = warns
}
