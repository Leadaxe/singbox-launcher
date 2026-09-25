package subscription

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/config/registry"
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
// и не собирается обратно, а кладётся в ParsedNode.Outbound как есть (Р2 в
// PLAN); правила значений и формы исполняет реестр стадией ниже, в единственной
// точке рождения тела (materializeBody → nodeflow.Sanitize) — одинаково для
// этого входа, ручного JSON и тела из бэкапа (SPEC 142 волна 2: рукописный
// SanitizeSingboxOutboundMap снят вместе с файлом singbox_sanitize.go).

// singboxIgnoredSections — секции целого конфига, которые импорт не читает.
// Показываются пользователю в превью, чтобы «проглочено молча» не выглядело
// как потеря данных.
var singboxIgnoredSections = []string{"route", "dns", "inbounds", "experimental"}

// singboxServiceTypes — служебные типы, которые не являются узлами.
// Собственный набор, намеренно не переиспользующий Xray-список
// (у Xray это freedom/blackhole/loopback, у sing-box — direct/block/dns).
var singboxServiceTypes = map[string]struct{}{
	"direct": {}, "block": {}, "dns": {},
}

// singboxGroupTypes — типы outbound-групп.
var singboxGroupTypes = map[string]struct{}{
	"selector": {}, "urltest": {},
}

// IsSingboxServiceType сообщает, является ли тип служебным (не узел).
func IsSingboxServiceType(t string) bool {
	_, ok := singboxServiceTypes[strings.ToLower(strings.TrimSpace(t))]
	return ok
}

// IsSingboxGroupType сообщает, является ли тип группой (selector/urltest).
func IsSingboxGroupType(t string) bool {
	_, ok := singboxGroupTypes[strings.ToLower(strings.TrimSpace(t))]
	return ok
}

// mapString возвращает строковое поле map или "".
func mapString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

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
	// SectionFragments — сколько фрагментов конфига (DNS-серверы, DNS-правила,
	// правила маршрута) поехали с узлом как его секции (SPEC 121 §6).
	// 0 — правило извлечения не выполнено (узлов не один) либо связки нет.
	SectionFragments int
	// rejected — записи, которые узлом не стали, с их местом в Nodes
	// (SPEC 116 W11). Не экспортируется: единственный читатель — чистый парсер
	// тела в этом же пакете, а `UnsupportedTypes` выше отвечает на другой
	// вопрос («какие типы мы не умеем») и записей поштучно не хранит.
	rejected jsonRejectSink
}

// RejectedRecords — неразобранные записи импорта в порядке появления
// (SPEC 116 W11): каждая обязана материализоваться узлом kind=unsupported на
// своей позиции. Возвращается парой (позиция в Nodes, причина, исходник).
func (r *SingboxImportResult) RejectedRecords() []jsonRejectedRecord {
	if r == nil {
		return nil
	}
	return r.rejected.list()
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

	// SPEC 121 §6: связка конфига принадлежит узлу. Обычный узел получает её
	// только когда он в конфиге один; узел tailnet — и в многоузловом
	// конфиге, по явной ссылке на свой тег (NODE_SECTIONS.md §6). Теги
	// считаются ДО разбора: ссылки внутри `dns`/`route` смотрят на тег
	// записи, а не на тег, который выдаст лаунчер.
	sectionCarriers := map[string]bool{}
	for _, tag := range SectionCarrierTags(cfg) {
		if tag != "" {
			sectionCarriers[tag] = true
		}
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
			// Запись без типа — тоже неразобранная запись, а не пустое место
			// (W11): собрать из неё нечего, но провайдер её прислал.
			result.rejected.add(len(result.Nodes),
				"outbound rejected: missing type", marshalRawJSONElement(entry))
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
			// W11: и не молчаливая пропажа — запись остаётся в составе узлом
			// kind=unsupported на своей позиции, со своим исходником.
			debuglog.WarnLog("Parser: singbox import: config %d entry %d (%s): %v",
				cfgIdx, entryIdx, entryType, err)
			unsupported[entryType] = struct{}{}
			result.rejected.addCoded(len(result.Nodes),
				fmt.Sprintf("%s outbound rejected: %v", entryType, err),
				rejectCodeOf(err),
				marshalRawJSONElement(entry))
			continue
		}

		if shouldSkipNode(node, skip) {
			continue
		}

		// SPEC 121 §6: секции достаются носителю связки и только ему. Узел
		// kind=unsupported сюда не доходит — он не прошёл разбор выше, и
		// связку без узла показывать было бы нечему.
		if rawTag != "" && sectionCarriers[rawTag] {
			if ns := ExtractNodeSections(cfg, rawTag); ns != nil {
				node.Sections = ns
				n := nodeSectionEntryCount(ns)
				result.SectionFragments += n
				debuglog.InfoLog("Parser: singbox import: node %q carries %d config fragment(s) (%s)",
					rawTag, n, strings.Join(sortedNodeSectionKinds(ns), ", "))
			}
		}
		// Голый узел tailnet — тот, у которого в конфиге не нашлось ни одной
		// своей записи, — получает КАНОНИЧЕСКУЮ связку (NODE_SECTIONS.md §6).
		// Без неё узел бесполезен: tailnet поднимется, но ни имена `*.ts.net`,
		// ни адреса `100.64.0.0/10` в него не пойдут, и пользователь узнал бы
		// об этом только по молчащим именам.
		if node.Sections.IsEmpty() && isSingboxTailscaleEntry(entry) {
			if ns := defaultTailscaleNodeSections(); ns != nil {
				node.Sections = ns
				result.SectionFragments += nodeSectionEntryCount(ns)
				debuglog.InfoLog("Parser: singbox import: bare tailscale node %q gets the canonical bundle", rawTag)
			}
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
		groupNode, rejectReason := singboxGroupToNode(groupEntry, nodeByTag, &result.Warnings)
		if rejectReason != "" {
			// Пустая или безымянная группа — не молчаливая пропажа, а
			// неразобранная запись на своей позиции (обкатка W13 заход 3:
			// «пустые сломанные узлы — как сломанный узел hysteria»).
			result.rejected.add(len(result.Nodes), rejectReason,
				marshalRawJSONElement(groupEntry))
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
// Работа минимальна по замыслу (Р2): схема по типу, копия map, адрес и тег для
// списков. Никакой пересборки полей и никаких правил значений — их исполняет
// реестр при материализации тела.
func parseSingboxEntry(entry map[string]interface{}, cfgIdx, entryIdx int) (*configtypes.ParsedNode, error) {
	entryType := strings.ToLower(strings.TrimSpace(mapString(entry, "type")))
	if entryType == "" {
		return nil, fmt.Errorf("missing type")
	}

	// Схему узла даёт реестр: обратная карта `singbox_type`, суженная до
	// схем, которые приходят входом sing-box (`sources`). Тип вне её —
	// неподдержанная запись.
	scheme, ok := registry.MustGet().NodeSchemeForSingboxType(entryType)
	if !ok {
		return nil, fmt.Errorf("unsupported outbound type %q", entryType)
	}

	ob := copyJSONMap(entry)

	// Адрес узла — для списков и skip-фильтров. Обязательность server и
	// server_port (и то, что у wireguard/tailscale адреса в корне нет) судит
	// реестр при материализации тела (`required`, `format: port`): запись без
	// адреса станет узлом kind=unsupported на своей позиции с кодом реестра.
	server := mapString(entry, "server")
	port := xrayJSONInt(entry["server_port"])

	tag := mapString(entry, "tag")
	if tag == "" {
		tag = fmt.Sprintf("%s-%d-%d", scheme, cfgIdx+1, entryIdx+1)
		ob["tag"] = tag
	}

	node := &configtypes.ParsedNode{
		Tag:    tag,
		Scheme: scheme,
		Server: server,
		Port:   port,
		Label:  tag,
		// Вход назван явно: тело приехало в СОБСТВЕННОЙ форме ядра, и правила
		// значений с `except_sources` обязаны это видеть (потолок MTU у
		// AmneziaWG здесь не заменяет значение, а предупреждает о нём).
		Source:      configtypes.NodeSourceSingbox,
		Outbound:    ob,
		SourceIndex: configtypes.UnsetSourceIndex,
	}

	// UUID/Flow заполняются для skip-фильтров и эмиссии: getNodeValue и
	// GenerateNodeJSON читают их из скалярных полей, а не из map.
	node.UUID = singboxCredentialFromMap(ob, scheme)
	node.Flow = mapString(ob, "flow")

	// D-119 — reality, переживший санитайз, с отпечатком вне chrome-семейства:
	// отпечаток уходит как есть, узел предупреждает (SPEC 083 ядра).

	return node, nil
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
	case "hysteria":
		// v1 хранит секрет в auth_str (а не password): см. option/hysteria.go.
		return mapString(ob, "auth_str")
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
