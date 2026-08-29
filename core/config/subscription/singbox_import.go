package subscription

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// SPEC 094 фаза A — импорт sing-box JSON.
//
// Все четыре входные формы (одиночный outbound, массив outbound'ов, целый
// конфиг, массив конфигов) нормализуются вызывающим к «массиву конфигов» и
// разбираются ОДНИМ ядром: отдельные пути для каждой формы разошлись бы по
// поведению при первой же правке.
//
// Принципиальное отличие от URI-пути: входной outbound УЖЕ является sing-box
// JSON — тем самым, который лаунчер эмитит. Поэтому он не разбирается на поля
// и не собирается обратно, а прогоняется через санитайзы (singbox_sanitize.go)
// и кладётся в ParsedNode.Outbound почти как есть (Р2 в PLAN).

// singboxIgnoredSections — секции целого конфига, которые импорт не читает.
// Показываются пользователю в превью, чтобы «проглочено молча» не выглядело
// как потеря данных.
var singboxIgnoredSections = []string{"route", "dns", "inbounds", "experimental"}

// SingboxImportResult — результат разбора sing-box JSON.
type SingboxImportResult struct {
	// Nodes — узлы в порядке появления в конфиге. Импортированные группы
	// (SchemeGroup) лежат здесь же, после обычных узлов: для лаунчера это
	// рядовые ноды, а не сущности вкладки Outbounds (SPEC 094 A5).
	Nodes []*configtypes.ParsedNode
	// IgnoredSections — фактически присутствовавшие секции из singboxIgnoredSections.
	IgnoredSections []string
	// UnsupportedTypes — типы outbound'ов, которые импорт не смог разобрать.
	UnsupportedTypes []string
	// Warnings — per-record деградации импорта (потерянные члены групп и
	// т.п.): SPEC 118 Т3 требует «не молча» — fetch персистит их в
	// updateStatus, лог сам по себе пользователя не достигает.
	Warnings []string
}

// ParseSingboxBody разбирает тело подписки, классифицированное как sing-box JSON.
//
// kind должен быть одним из BodyKind*Singbox*; иначе возвращается ошибка —
// это программная ошибка вызывающего, а не проблема данных.
func ParseSingboxBody(body string, kind BodyKind, skip []map[string]string) (*SingboxImportResult, error) {
	configs, err := normalizeSingboxBodyToConfigs(body, kind)
	if err != nil {
		return nil, err
	}
	return ParseNodesFromSingboxConfigs(configs, skip)
}

// normalizeSingboxBodyToConfigs приводит любую из четырёх форм к массиву конфигов.
func normalizeSingboxBodyToConfigs(body string, kind BodyKind) ([]map[string]interface{}, error) {
	trimmed := strings.TrimSpace(body)

	switch kind {
	case BodyKindSingboxOutbound:
		var ob map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &ob); err != nil {
			return nil, fmt.Errorf("singbox outbound: %w", err)
		}
		return []map[string]interface{}{
			{"outbounds": []interface{}{ob}},
		}, nil

	case BodyKindSingboxOutboundArray:
		var obs []interface{}
		if err := json.Unmarshal([]byte(trimmed), &obs); err != nil {
			return nil, fmt.Errorf("singbox outbound array: %w", err)
		}
		return []map[string]interface{}{
			{"outbounds": obs},
		}, nil

	case BodyKindSingboxConfig:
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(trimmed), &cfg); err != nil {
			return nil, fmt.Errorf("singbox config: %w", err)
		}
		return []map[string]interface{}{cfg}, nil

	case BodyKindSingboxConfigArray:
		var raws []json.RawMessage
		if err := json.Unmarshal([]byte(trimmed), &raws); err != nil {
			return nil, fmt.Errorf("singbox config array: %w", err)
		}
		out := make([]map[string]interface{}, 0, len(raws))
		for i, raw := range raws {
			var cfg map[string]interface{}
			if err := json.Unmarshal(raw, &cfg); err != nil {
				// Один битый элемент не должен ронять остальные.
				debuglog.WarnLog("Parser: singbox config array element %d: %v", i, err)
				continue
			}
			out = append(out, cfg)
		}
		return out, nil

	default:
		return nil, fmt.Errorf("body kind %v is not a sing-box JSON form", kind)
	}
}

// ParseNodesFromSingboxConfigs — ядро импорта (SPEC 094 A3).
func ParseNodesFromSingboxConfigs(configs []map[string]interface{}, skip []map[string]string) (*SingboxImportResult, error) {
	result := &SingboxImportResult{
		Nodes: make([]*configtypes.ParsedNode, 0),
	}

	ignored := make(map[string]struct{})
	unsupported := make(map[string]struct{})

	for cfgIdx, cfg := range configs {
		for _, section := range singboxIgnoredSections {
			if _, present := cfg[section]; present {
				ignored[section] = struct{}{}
			}
		}
		parseSingboxConfig(cfg, cfgIdx, skip, result, unsupported)
	}

	// Детерминированный порядок: множества выше не сохраняют порядок вставки.
	result.IgnoredSections = orderedSubset(singboxIgnoredSections, ignored)
	result.UnsupportedTypes = sortedKeys(unsupported)

	if len(result.IgnoredSections) > 0 {
		debuglog.DebugLog("Parser: singbox import: ignored sections: %s",
			strings.Join(result.IgnoredSections, ", "))
	}
	if len(result.UnsupportedTypes) > 0 {
		debuglog.WarnLog("Parser: singbox import: unsupported outbound types skipped: %s",
			strings.Join(result.UnsupportedTypes, ", "))
	}

	return result, nil
}

// parseSingboxConfig разбирает один конфиг: узлы, затем группы (A7).
func parseSingboxConfig(
	cfg map[string]interface{},
	cfgIdx int,
	skip []map[string]string,
	result *SingboxImportResult,
	unsupported map[string]struct{},
) {
	entries := singboxAllEntries(cfg)
	if len(entries) == 0 {
		return
	}

	// Индекс по тегу нужен и группам (резолв состава), и цепочкам (фаза B).
	byTag := make(map[string]map[string]interface{}, len(entries))
	for _, entry := range entries {
		tag := mapString(entry, "tag")
		if tag == "" {
			continue
		}
		if _, exists := byTag[tag]; !exists {
			byTag[tag] = entry
		}
	}

	chainInfo := analyzeSingboxDetour(entries, byTag)

	var groupEntries []map[string]interface{}
	// nodeByTag — тег исходного конфига → импортированный узел; нужен группам.
	nodeByTag := make(map[string]*configtypes.ParsedNode, len(entries))

	for entryIdx, entry := range entries {
		entryType := strings.ToLower(strings.TrimSpace(mapString(entry, "type")))
		if entryType == "" {
			continue
		}
		if IsSingboxServiceType(entryType) {
			continue // A3
		}
		if IsSingboxGroupType(entryType) {
			groupEntries = append(groupEntries, entry)
			continue // A5 — разбираются после узлов
		}

		rawTag := mapString(entry, "tag")
		// Цель чужого detour самостоятельным узлом не становится — кроме
		// случая, когда её ребро было снято как замыкающее кольцо (B3).
		if chainInfo.isDetourTarget(rawTag) {
			continue
		}

		node, err := parseSingboxEntry(entry, cfgIdx, entryIdx)
		if err != nil {
			// A6: гранулярность узла — соседи и остальная подписка живут.
			debuglog.WarnLog("Parser: singbox import: config %d entry %d (%s): %v",
				cfgIdx, entryIdx, entryType, err)
			unsupported[entryType] = struct{}{}
			continue
		}

		if shouldSkipNode(node, skip) {
			continue
		}

		chainInfo.attachChain(node, entry, byTag, cfgIdx)

		result.Nodes = append(result.Nodes, node)
		if rawTag != "" {
			nodeByTag[rawTag] = node
		}
	}

	// A5/A7: группы идут ПОСЛЕ узлов — и в тот же список. Это рядовые узлы
	// без привилегий, а не записи вкладки Outbounds (см. singbox_groups.go).
	for _, groupEntry := range groupEntries {
		groupNode, ok := singboxGroupToNode(groupEntry, nodeByTag, &result.Warnings)
		if !ok {
			continue
		}
		result.Nodes = append(result.Nodes, groupNode)
	}
}

// singboxAllEntries возвращает outbounds ++ endpoints одним списком (A2).
//
// WireGuard и MASQUE в sing-box >= 1.11 живут в endpoints, но правила разбора
// для них те же — разделять источники значило бы дублировать всю логику.
func singboxAllEntries(cfg map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, 8)
	for _, section := range []string{"outbounds", "endpoints"} {
		raw, ok := cfg[section].([]interface{})
		if !ok {
			continue
		}
		for _, item := range raw {
			entry, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			out = append(out, entry)
		}
	}
	return out
}

// parseSingboxEntry конвертирует один outbound/endpoint в ParsedNode.
//
// Работа минимальна по замыслу (Р2): валидация обязательных полей, копия map,
// прогон санитайзов. Никакой пересборки полей.
func parseSingboxEntry(entry map[string]interface{}, cfgIdx, entryIdx int) (*configtypes.ParsedNode, error) {
	entryType := strings.ToLower(strings.TrimSpace(mapString(entry, "type")))
	if entryType == "" {
		return nil, fmt.Errorf("missing type")
	}

	scheme, ok := singboxTypeToScheme(entryType)
	if !ok {
		return nil, fmt.Errorf("unsupported outbound type %q", entryType)
	}

	ob := copyJSONMap(entry)

	// WireGuard/MASQUE описывают адрес иначе (address[]/peers), общая проверка
	// server/server_port к ним не применима.
	server := mapString(entry, "server")
	port := xrayJSONInt(entry["server_port"])

	if !singboxTypeIsAddressless(entryType) {
		if server == "" {
			return nil, fmt.Errorf("missing server")
		}
		if port <= 0 || port > 65535 {
			return nil, fmt.Errorf("invalid server_port %d", port)
		}
	}

	tag := mapString(entry, "tag")
	if tag == "" {
		tag = fmt.Sprintf("%s-%d-%d", scheme, cfgIdx+1, entryIdx+1)
		ob["tag"] = tag
	}

	sanitizeCodes := SanitizeSingboxOutboundMap(ob, tag)

	node := &configtypes.ParsedNode{
		Tag:         tag,
		Scheme:      scheme,
		Server:      server,
		Port:        port,
		Label:       tag,
		Outbound:    ob,
		SourceIndex: configtypes.UnsetSourceIndex,
	}

	// UUID/Flow заполняются для skip-фильтров и эмиссии: getNodeValue и
	// GenerateNodeJSON читают их из скалярных полей, а не из map.
	node.UUID = singboxCredentialFromMap(ob, scheme)
	node.Flow = mapString(ob, "flow")

	// Деградации санитайзера — на узел: конверт узла едет в UI и в LxBox.
	for _, code := range sanitizeCodes {
		node.AddWarning(code)
	}

	return node, nil
}

// singboxSchemeByType — соответствие sing-box type → внутренняя схема лаунчера.
//
// Схемы совпадают с теми, что выдаёт URI-парсер: дальше по конвейеру
// (GenerateNodeJSON, skip-фильтры, share-URI) работает один и тот же код.
var singboxSchemeByType = map[string]string{
	"vless":       "vless",
	"vmess":       "vmess",
	"trojan":      "trojan",
	"shadowsocks": "ss",
	"hysteria2":   "hysteria2",
	"tuic":        "tuic",
	"anytls":      "anytls",
	"ssh":         "ssh",
	"socks":       "socks",
	"http":        "http",
	"naive":       "naive",
	"wireguard":   "wireguard",
	"masque":      "masque",
}

func singboxTypeToScheme(t string) (string, bool) {
	s, ok := singboxSchemeByType[t]
	return s, ok
}

// SchemeFromSingboxType — тот же словарь наружу (SPEC 118 W4): эмиссия из
// материализованных nodes[] определяет схему узла по типу его тела, и второй
// таблицы соответствий заводить нельзя — она разъехалась бы с этой.
func SchemeFromSingboxType(t string) (string, bool) {
	return singboxTypeToScheme(strings.ToLower(strings.TrimSpace(t)))
}

// singboxTypeIsAddressless — типы без server/server_port на верхнем уровне.
func singboxTypeIsAddressless(t string) bool {
	return t == "wireguard"
}

// singboxCredentialFromMap достаёт учётные данные в поле UUID ParsedNode.
//
// ParsedNode.UUID исторически хранит «главный секрет» узла независимо от
// протокола (для trojan/ss/hysteria2 это пароль) — см. GenerateNodeJSON.
func singboxCredentialFromMap(ob map[string]interface{}, scheme string) string {
	switch scheme {
	case "vless", "vmess", "tuic":
		return mapString(ob, "uuid")
	case "trojan", "hysteria2", "anytls", "ss":
		return mapString(ob, "password")
	default:
		return ""
	}
}

// copyJSONMap делает глубокую копию декодированного JSON.
//
// Копия обязательна: санитайзы правят map на месте, а исходный entry ещё нужен
// для резолва цепочек и групп по сырым тегам.
func copyJSONMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = copyJSONValue(v)
	}
	return dst
}

func copyJSONValue(v interface{}) interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		return copyJSONMap(x)
	case []interface{}:
		out := make([]interface{}, len(x))
		for i, item := range x {
			out[i] = copyJSONValue(item)
		}
		return out
	default:
		return v
	}
}

// orderedSubset возвращает элементы order, присутствующие в set.
func orderedSubset(order []string, set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for _, item := range order {
		if _, ok := set[item]; ok {
			out = append(out, item)
		}
	}
	return out
}

// sortedKeys возвращает отсортированные ключи множества.
func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	// Небольшие множества: вставками, без импорта sort ради трёх элементов.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
