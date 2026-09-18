// File node_sources.go — ОТКУДА узел, адресуемое ФИНАЛЬНЫМ тегом.
//
// # Зачем слой
//
// Близнец node_warnings.go и построен тем же приёмом по той же причине:
// вкладка Servers живёт на `api.ProxyInfo` из Clash API, а тот знает про узел
// ровно одно имя — тег, под которым узел уехал в конфиг. Принадлежность же
// узла источнику живёт в `state.json`, где узел адресуется СЫРЫМ тегом в
// рамках источника (идентичность, SPEC 112). Между ними стоит тег-машина:
// `norm(prefix + сырой + postfix)`.
//
// Индекс нужен фильтру списка (категория «Источник»): чипы перечисляют
// подписки и папки, реально представленные в текущем списке, и клик по чипу
// обязан оставить узлы именно этой подписки.
//
// Тег-политику индекс не повторяет — зовёт `state.NodeLinkFinalTag`, ту же,
// что зовут предупреждения. Четвёртой копии правил тегов не заводится.
//
// # Чего индекс НЕ делает
//
// Узлы, чей финальный тег политика раскрыть не может (переменные `{$num}`),
// в индекс не попадают: раскрывает их только сборка. Фильтр по источнику
// такие узлы НЕ отсекает — они проходят любой выбор чипов (Lookup отдаёт
// ok=false: «источник неизвестен», а не «чужой источник»).
// Соврать про принадлежность хуже, чем промолчать: узел с неизвестным
// происхождением, спрятанный выбором «Proton», выглядел бы пропавшим.
//
// Глобальной уникализации (суффикс при столкновении тегов) индекс тоже не
// повторяет — как и предупреждения, отдаёт тег первому владельцу.
//
// Кэш — по (путь, mtime, размер) state.json; инвалидация ПАРОЙ с
// InvalidateConfigNodesCache / InvalidateNodeWarningsCache (память
// `cache-invalidation-pairs`): все три проекции снимает одно событие —
// пересборка.
//
// go1.20-совместимо (Win7-джоба): без slices/maps/min/max/clear.
package business

import (
	"net/url"
	"os"
	"strings"
	"sync"

	"singbox-launcher/core/state"
)

// NodeSource — источник узла, каким его называет UI.
type NodeSource struct {
	// ID — ULID источника; ключ выбора в фильтре. Устойчив к переименованию.
	ID string
	// Name — отображаемое имя: имя источника, иначе заголовок профиля от
	// провайдера, иначе хост ссылки, иначе тег узлового источника.
	Name string
}

// NodeSourceIndex — снимок принадлежности узлов источникам по финальному тегу.
type NodeSourceIndex struct {
	byTag map[string]NodeSource
}

// Lookup — источник узла с таким финальным тегом; ok=false, если неизвестен.
func (idx *NodeSourceIndex) Lookup(tag string) (NodeSource, bool) {
	if idx == nil || idx.byTag == nil {
		return NodeSource{}, false
	}
	src, ok := idx.byTag[strings.TrimSpace(tag)]
	return src, ok
}

// Len — сколько узлов состояния удалось приписать источнику.
func (idx *NodeSourceIndex) Len() int {
	if idx == nil {
		return 0
	}
	return len(idx.byTag)
}

type nodeSourceCacheEntry struct {
	modTime int64
	size    int64
	parsed  *NodeSourceIndex
}

var nodeSourceCache struct {
	mu     sync.RWMutex
	byPath map[string]nodeSourceCacheEntry
}

// LoadNodeSources читает state.json по пути path и строит индекс.
//
// При любой ошибке — пустой индекс, а не nil: вызывающий делает Lookup без
// проверок, и категория «Источник» просто остаётся без чипов.
func LoadNodeSources(path string) *NodeSourceIndex {
	path = strings.TrimSpace(path)
	if path == "" {
		return &NodeSourceIndex{}
	}
	info, err := os.Stat(path)
	if err != nil {
		return &NodeSourceIndex{}
	}
	modTime, size := info.ModTime().UnixNano(), info.Size()

	nodeSourceCache.mu.RLock()
	if e, ok := nodeSourceCache.byPath[path]; ok &&
		e.parsed != nil && e.modTime == modTime && e.size == size {
		nodeSourceCache.mu.RUnlock()
		return e.parsed
	}
	nodeSourceCache.mu.RUnlock()

	parsed := buildNodeSourceIndex(path)

	nodeSourceCache.mu.Lock()
	if nodeSourceCache.byPath == nil {
		nodeSourceCache.byPath = make(map[string]nodeSourceCacheEntry, 2)
	}
	nodeSourceCache.byPath[path] = nodeSourceCacheEntry{
		modTime: modTime, size: size, parsed: parsed,
	}
	nodeSourceCache.mu.Unlock()
	return parsed
}

// InvalidateNodeSourcesCache сбрасывает кэш индекса.
//
// Инвалидируется ПАРОЙ с InvalidateConfigNodesCache и
// InvalidateNodeWarningsCache — см. шапку файла.
func InvalidateNodeSourcesCache() {
	nodeSourceCache.mu.Lock()
	nodeSourceCache.byPath = nil
	nodeSourceCache.mu.Unlock()
}

// buildNodeSourceIndex — один проход по источникам состояния.
func buildNodeSourceIndex(path string) *NodeSourceIndex {
	s, err := state.Load(path)
	if err != nil || s == nil {
		return &NodeSourceIndex{}
	}
	idx := &NodeSourceIndex{byTag: make(map[string]NodeSource, 16)}
	for i := range s.Sources {
		src := &s.Sources[i]
		entry := NodeSource{ID: sourceIndexID(src), Name: sourceDisplayName(src)}
		if entry.ID == "" || entry.Name == "" {
			continue
		}
		// Узловой источник (server/chain/auto): состава нет, узел и есть
		// источник, а тег-политика к самому себе не применяется.
		if len(src.Nodes) == 0 {
			idx.put(strings.TrimSpace(src.NodeTagOrLabel()), entry)
			continue
		}
		for j := range src.Nodes {
			n := &src.Nodes[j]
			final, ok := state.NodeLinkFinalTag(src.TagPolicy, n.Tag)
			if n.Kind == state.SourceKindChain {
				// Тег цепочки тег-политику не проходит: в конфиге она зовётся
				// своим тегом (core/config/canonical_emit.go, проход 2).
				final = strings.TrimSpace(n.Tag)
				ok = final != ""
			}
			if !ok {
				// Политика с переменными ({$num}): финальный тег раскрывает
				// только сборка. Молчим — см. шапку файла.
				continue
			}
			idx.put(final, entry)
		}
	}
	return idx
}

// sourceIndexID — ключ источника для фильтра.
//
// ULID, если он есть; иначе тег узлового источника — старые состояния его без
// ULID переживали, и остаться без чипа «сервер, добавленный руками» было бы
// хуже, чем ключ другой природы: ключ здесь живёт только в памяти окна.
func sourceIndexID(src *state.Source) string {
	if id := strings.TrimSpace(src.ID); id != "" {
		return id
	}
	return strings.TrimSpace(src.NodeTagOrLabel())
}

// sourceDisplayName — как источник зовут в чипе.
//
// Порядок повторяет остальной UI: каноническое имя → заголовок профиля от
// провайдера (у подписки, добавленной одной ссылкой, имя часто пусто) → хост
// ссылки → тег узлового источника.
func sourceDisplayName(src *state.Source) string {
	if name := strings.TrimSpace(src.Name); name != "" {
		return name
	}
	if src.Meta != nil {
		if title := strings.TrimSpace(src.Meta.ProfileTitle); title != "" {
			return title
		}
	}
	if raw := strings.TrimSpace(src.URL); raw != "" {
		if u, err := url.Parse(raw); err == nil && u.Host != "" {
			return u.Host
		}
	}
	return strings.TrimSpace(src.NodeTagOrLabel())
}

// put — записать, не затирая первого владельца тега.
//
// Столкновение финальных тегов разрешает сборка суффиксом, и который из двух
// узлов стал в конфиге этим тегом, здесь не известно. Первый по порядку
// состояния — то же правило, по которому эмиссия раздаёт слоты, и то же, по
// которому раздаёт их индекс предупреждений.
func (idx *NodeSourceIndex) put(tag string, src NodeSource) {
	if tag == "" {
		return
	}
	if _, dup := idx.byTag[tag]; dup {
		return
	}
	idx.byTag[tag] = src
}
