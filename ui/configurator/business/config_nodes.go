package business

import (
	"encoding/json"
	"os"
	"strings"
	"sync"

	"singbox-launcher/internal/debuglog"
)

// SPEC 095 — разбор сгенерированного config.json для списка серверов.
//
// Список живёт на api.ProxyInfo из Clash API, а тот несёт только имя, тип,
// пинг и трафик. Транспорта, TLS, сервера и состава групп там нет — всё это
// лежит в config.json, который лаунчер сам и сгенерировал. Этот слой читает
// файл и отдаёт по тегу разобранные поля.
//
// Аналог ParsedConfig/ConfigNode в LxBox (app/lib/models/config_node.dart).

// ConfigNode — узел из config.json, каким его видит UI.
type ConfigNode struct {
	// Tag — тег outbound'а; совпадает с именем в Clash API.
	Tag string
	// Type — "vless", "urltest", "wireguard" и т.д.
	Type string
	// Kind — "outbound" или "endpoint": WireGuard эмитится в endpoints.
	Kind string
	// Server / ServerPort — адрес; у групп и endpoint'ов пусто.
	Server     string
	ServerPort int
	// Transport — метка транспорта для подзаголовка (см. deriveTransport).
	Transport string
	// Security — метка TLS/Reality/AWG (см. deriveSecurity).
	Security string
	// Detour — тег следующего хопа, либо пусто.
	Detour string
	// GroupMembers — состав selector/urltest; nil у обычных узлов.
	GroupMembers []string
	// Raw — исходный объект целиком, для экрана Info.
	Raw map[string]interface{}
}

// IsGroup сообщает, является ли узел группой выбора.
func (n *ConfigNode) IsGroup() bool {
	if n == nil {
		return false
	}
	return n.Type == "selector" || n.Type == "urltest"
}

// IsService сообщает, служебный ли это outbound (не показывается как сервер).
func (n *ConfigNode) IsService() bool {
	if n == nil {
		return false
	}
	switch n.Type {
	case "direct", "block", "dns":
		return true
	}
	return false
}

// SubtitleParts возвращает части подзаголовка: протокол, транспорт, security.
//
// Пустые части опускает вызывающий — здесь они остаются как есть, чтобы
// решение о формате принимал UI.
func (n *ConfigNode) SubtitleParts() []string {
	if n == nil {
		return nil
	}
	parts := make([]string, 0, 3)
	if n.Type != "" {
		parts = append(parts, n.Type)
	}
	if n.Transport != "" {
		parts = append(parts, n.Transport)
	}
	if n.Security != "" {
		parts = append(parts, n.Security)
	}
	return parts
}

// ConfigNodes — снимок разобранного config.json.
type ConfigNodes struct {
	byTag map[string]*ConfigNode
}

// Lookup возвращает узел по тегу; nil, если такого нет.
//
// Отсутствие узла — не ошибка: Clash API может отдать тег, которого нет в
// текущем конфиге (гонка перегенерации), и список обязан это пережить.
func (c *ConfigNodes) Lookup(tag string) *ConfigNode {
	if c == nil || c.byTag == nil {
		return nil
	}
	return c.byTag[tag]
}

// Len возвращает число разобранных узлов.
func (c *ConfigNodes) Len() int {
	if c == nil {
		return 0
	}
	return len(c.byTag)
}

// configNodesCache — кэш разбора: список перерисовывается часто, а на 500+
// узлах чтение файла на каждую строку заметно тормозит.
var configNodesCache struct {
	mu      sync.RWMutex
	path    string
	modTime int64
	size    int64
	parsed  *ConfigNodes
}

// LoadConfigNodes читает и разбирает config.json по пути path.
//
// Результат кэшируется по (путь, mtime, размер): перегенерация конфига меняет
// mtime, и кэш обновляется сам — отдельный сигнал инвалидации не нужен.
//
// При любой ошибке возвращает пустой снимок, а не nil: вызывающий делает
// Lookup без проверок, и подзаголовок просто не рисуется.
func LoadConfigNodes(path string) *ConfigNodes {
	path = strings.TrimSpace(path)
	if path == "" {
		return &ConfigNodes{}
	}

	info, err := os.Stat(path)
	if err != nil {
		return &ConfigNodes{}
	}
	modTime, size := info.ModTime().UnixNano(), info.Size()

	configNodesCache.mu.RLock()
	if configNodesCache.parsed != nil &&
		configNodesCache.path == path &&
		configNodesCache.modTime == modTime &&
		configNodesCache.size == size {
		cached := configNodesCache.parsed
		configNodesCache.mu.RUnlock()
		return cached
	}
	configNodesCache.mu.RUnlock()

	parsed := parseConfigNodesFile(path)

	configNodesCache.mu.Lock()
	configNodesCache.path = path
	configNodesCache.modTime = modTime
	configNodesCache.size = size
	configNodesCache.parsed = parsed
	configNodesCache.mu.Unlock()

	return parsed
}

// InvalidateConfigNodesCache сбрасывает кэш разбора.
//
// Обычно не требуется — кэш сам замечает смену mtime, — но при записи конфига
// в ту же наносекунду (быстрый rebuild подряд) страховка не лишняя.
func InvalidateConfigNodesCache() {
	configNodesCache.mu.Lock()
	configNodesCache.parsed = nil
	configNodesCache.mu.Unlock()
}

// parseConfigNodesFile читает файл и разбирает его.
func parseConfigNodesFile(path string) *ConfigNodes {
	data, err := os.ReadFile(path)
	if err != nil {
		debuglog.DebugLog("LoadConfigNodes: read %s: %v", path, err)
		return &ConfigNodes{}
	}
	return ParseConfigNodes(data)
}

// ParseConfigNodes разбирает содержимое config.json.
//
// Конфиг лаунчера содержит //-комментарии и маркеры /** @ParserSTART */ —
// обычный json.Unmarshal на нём падает, поэтому текст сначала чистится.
func ParseConfigNodes(data []byte) *ConfigNodes {
	cleaned := stripJSONComments(data)

	var root struct {
		Outbounds []map[string]interface{} `json:"outbounds"`
		Endpoints []map[string]interface{} `json:"endpoints"`
	}
	if err := json.Unmarshal(cleaned, &root); err != nil {
		debuglog.DebugLog("ParseConfigNodes: %v", err)
		return &ConfigNodes{}
	}

	out := &ConfigNodes{byTag: make(map[string]*ConfigNode, len(root.Outbounds)+len(root.Endpoints))}
	for _, ob := range root.Outbounds {
		if node := configNodeFromRaw(ob, "outbound"); node != nil {
			out.byTag[node.Tag] = node
		}
	}
	for _, ep := range root.Endpoints {
		if node := configNodeFromRaw(ep, "endpoint"); node != nil {
			out.byTag[node.Tag] = node
		}
	}
	return out
}

// configNodeFromRaw строит ConfigNode из сырого объекта.
func configNodeFromRaw(raw map[string]interface{}, kind string) *ConfigNode {
	tag := strings.TrimSpace(cfgNodeString(raw, "tag"))
	if tag == "" {
		// Endpoint'ы WireGuard в старых шаблонах именуются полем name.
		tag = strings.TrimSpace(cfgNodeString(raw, "name"))
	}
	if tag == "" {
		return nil
	}

	typ := strings.ToLower(strings.TrimSpace(cfgNodeString(raw, "type")))

	node := &ConfigNode{
		Tag:        tag,
		Type:       typ,
		Kind:       kind,
		Server:     cfgNodeString(raw, "server"),
		ServerPort: cfgNodeInt(raw["server_port"]),
		Detour:     cfgNodeString(raw, "detour"),
		Transport:  deriveTransport(typ, raw),
		Security:   deriveSecurity(typ, raw),
		Raw:        raw,
	}

	if members, ok := raw["outbounds"].([]interface{}); ok {
		node.GroupMembers = make([]string, 0, len(members))
		for _, m := range members {
			if s, ok := m.(string); ok {
				node.GroupMembers = append(node.GroupMembers, s)
			}
		}
	}

	return node
}

// stripJSONComments убирает //-комментарии, /**…*/-маркеры и висячие запятые.
//
// Работает посимвольно с учётом строковых литералов: наивный regexp съел бы
// "//" внутри URL (`https://example.com`) и сломал бы разбор.
func stripJSONComments(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString := false
	escaped := false

	for i := 0; i < len(data); i++ {
		c := data[i]

		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}

		if c == '"' {
			inString = true
			out = append(out, c)
			continue
		}

		// //-комментарий до конца строки.
		if c == '/' && i+1 < len(data) && data[i+1] == '/' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			if i < len(data) {
				out = append(out, '\n')
			}
			continue
		}

		// /* … */ — в том числе маркеры парсера.
		if c == '/' && i+1 < len(data) && data[i+1] == '*' {
			i += 2
			for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
				i++
			}
			i++ // встанем на '/', цикл прибавит ещё
			continue
		}

		out = append(out, c)
	}

	return stripTrailingCommas(out)
}

// stripTrailingCommas убирает запятые перед закрывающей скобкой.
func stripTrailingCommas(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inString := false
	escaped := false

	for i := 0; i < len(data); i++ {
		c := data[i]

		if inString {
			out = append(out, c)
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			out = append(out, c)
			continue
		}

		if c == ',' {
			// Заглянем вперёд: если дальше только пробелы и ']' или '}' —
			// запятая висячая.
			j := i + 1
			for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
				j++
			}
			if j < len(data) && (data[j] == ']' || data[j] == '}') {
				continue
			}
		}
		out = append(out, c)
	}
	return out
}

// jsonString возвращает строковое поле или "".
func cfgNodeString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

// jsonInt приводит числовое поле JSON к int.
func cfgNodeInt(v interface{}) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	default:
		return 0
	}
}
