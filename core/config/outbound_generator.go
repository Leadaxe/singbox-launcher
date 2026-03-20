// Package config: outbound_generator.go — генерация outbounds для sing-box из ParserConfig и подписок.
//
// # Логика работы
//
// Вход: ParserConfig (источники подписок proxies, глобальные селекторы outbounds) и функция загрузки нод.
// Выход: массив JSON-строк для вставки в config.json (ноды + локальные селекторы + глобальные селекторы).
//
// Зачем три прохода:
//   - Селекторы могут ссылаться друг на друга через addOutbounds (например "proxy-out" включает "auto-proxy-out").
//   - Пустой селектор (0 нод и все динамические addOutbounds тоже пустые) не должен попадать в конфиг и не должен
//     учитываться как валидный addOutbound у других. Поэтому сначала собираем все селекторы и только ноды (pass 1),
//     затем в порядке зависимостей считаем «полный» размер каждого и флаг isValid (pass 2), затем генерируем JSON
//     только для валидных и с отфильтрованным списком addOutbounds (pass 3).
//
// Этапы:
//
//  1. Загрузка нод: для каждого proxy source вызывается loadNodesFunc → allNodes, nodesBySource.
//  2. Генерация JSON нод: каждый ParsedNode → одна JSON-строка (GenerateNodeJSON).
//  3. Pass 1 — buildOutboundsInfo: по конфигу строим map[tag]*outboundInfo для всех селекторов (локальных и глобальных),
//     для каждого — отфильтрованные ноды и начальный outboundCount = len(filteredNodes). isValid пока false.
//  4. Pass 2 — computeOutboundValidity: топологическая сортировка по графу зависимостей addOutbounds;
//     в этом порядке для каждого селектора считаем outboundCount = nodes + число валидных addOutbounds (динамические
//     с outboundCount > 0 + константы типа direct-out). isValid = (outboundCount > 0).
//  5. Pass 3 — generateSelectorJSONs: для каждого селектора с isValid == true вызываем GenerateSelectorWithFilteredAddOutbounds
//     (в список addOutbounds попадают только валидные динамические и константы). Итог: срез JSON локальных и глобальных селекторов.
//
// Итоговый порядок в OutboundsJSON: [ ноды..., локальные селекторы..., глобальные селекторы... ].
//
// Фильтрация нод для селекторов задаётся в ParserConfig (filters: literal, /regex/i, !literal, !/regex/i по полям tag, host, scheme и т.д.).
// Реализация фильтров — в конце файла (filterNodesForSelector, matchesFilter, matchesPattern и др.).
package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// OutboundGenerationResult is the return value of GenerateOutboundsFromParserConfig: slice of JSON strings
// (nodes, then local selectors, then global selectors) and counts for each category.
// WireGuard nodes go to EndpointsJSON (sing-box endpoints), not OutboundsJSON.
type OutboundGenerationResult struct {
	OutboundsJSON        []string // Generated JSON lines for outbounds array (nodes, then local, then global selectors)
	EndpointsJSON       []string // Generated JSON lines for endpoints array (WireGuard nodes only)
	NodesCount           int      // Number of node outbounds (non-WireGuard)
	EndpointsCount       int      // Number of WireGuard endpoint nodes
	LocalSelectorsCount  int      // Number of local (per-source) selectors
	GlobalSelectorsCount int      // Number of global selectors
}

// outboundInfo stores information about a dynamically created outbound selector.
// This structure is used during the three-pass generation process to:
// - Pass 1: Store filtered nodes and initial node count
// - Pass 2: Calculate total outboundCount (nodes + valid addOutbounds) and validity
// - Pass 3: Generate JSON only for valid selectors with filtered addOutbounds
type outboundInfo struct {
	config        OutboundConfig // Original outbound configuration
	filteredNodes []*ParsedNode  // Nodes that match this selector's filters
	outboundCount int            // Total count: filteredNodes + valid addOutbounds (calculated in pass 2)
	isValid       bool           // true if outboundCount > 0 (set in pass 2)
	isLocal       bool           // true if it's a local selector (from proxySource.Outbounds), false if global
}

// appendOutboundTransportParts appends a sing-box "transport" object from node.Outbound (VLESS, VMess, Trojan).
func appendOutboundTransportParts(parts []string, outbound map[string]interface{}) []string {
	if outbound == nil {
		return parts
	}
	transport, ok := outbound["transport"].(map[string]interface{})
	if !ok || len(transport) == 0 {
		return parts
	}
	var transportParts []string
	if tType, ok := transport["type"].(string); ok && tType != "" {
		transportParts = append(transportParts, fmt.Sprintf(`"type":%q`, tType))
	}
	if path, ok := transport["path"].(string); ok && path != "" {
		transportParts = append(transportParts, fmt.Sprintf(`"path":%q`, path))
	}
	switch hostVal := transport["host"].(type) {
	case string:
		if hostVal != "" {
			transportParts = append(transportParts, fmt.Sprintf(`"host":%q`, hostVal))
		}
	case []string:
		if len(hostVal) > 0 {
			hostJSON, err := json.Marshal(hostVal)
			if err == nil {
				transportParts = append(transportParts, fmt.Sprintf(`"host":%s`, string(hostJSON)))
			}
		}
	}
	if serviceName, ok := transport["service_name"].(string); ok && serviceName != "" {
		transportParts = append(transportParts, fmt.Sprintf(`"service_name":%q`, serviceName))
	}
	if headers, ok := transport["headers"].(map[string]string); ok && len(headers) > 0 {
		var headerParts []string
		for k, v := range headers {
			headerParts = append(headerParts, fmt.Sprintf(`%q:%q`, k, v))
		}
		transportParts = append(transportParts, fmt.Sprintf(`"headers":{%s}`, strings.Join(headerParts, ",")))
	}
	if len(transportParts) > 0 {
		transportJSON := "{" + strings.Join(transportParts, ",") + "}"
		parts = append(parts, fmt.Sprintf(`"transport":%s`, transportJSON))
	}
	return parts
}

// GenerateNodeJSON returns a single JSON object string for one proxy node (sing-box outbound).
// Field order and presence follow sing-box expectations. Supports: vless, vmess, trojan, shadowsocks, hysteria2, ssh.
// Includes optional TLS (including reality), transport (ws/http/grpc), and protocol-specific options.
// Returned string ends with a trailing comma and may include a leading comment line (node label) for readability.
func GenerateNodeJSON(node *ParsedNode) (string, error) {
	// Build JSON with correct field order
	var parts []string

	// 1. tag
	parts = append(parts, fmt.Sprintf(`"tag":%q`, node.Tag))

	// 2. type
	if node.Scheme == "ss" {
		parts = append(parts, fmt.Sprintf(`"type":%q`, "shadowsocks"))
	} else {
		parts = append(parts, fmt.Sprintf(`"type":%q`, node.Scheme))
	}

	// 3. server
	parts = append(parts, fmt.Sprintf(`"server":%q`, node.Server))

	// 4. server_port (prefer outbound map: buildOutbound may adjust port, e.g. vision-udp443 → 443)
	serverPort := node.Port
	if node.Outbound != nil {
		if sp, ok := node.Outbound["server_port"].(int); ok && sp > 0 {
			serverPort = sp
		}
	}
	parts = append(parts, fmt.Sprintf(`"server_port":%d`, serverPort))

	// 5. uuid (for vless/vmess) or password (for trojan) or method/password (for ss)
	if node.Scheme == "vless" || node.Scheme == "vmess" {
		parts = append(parts, fmt.Sprintf(`"uuid":%q`, node.UUID))

		if node.Scheme == "vmess" {
			if security, ok := node.Outbound["security"].(string); ok && security != "" {
				parts = append(parts, fmt.Sprintf(`"security":%q`, security))
			}
			if alterID, ok := node.Outbound["alter_id"].(int); ok {
				parts = append(parts, fmt.Sprintf(`"alter_id":%d`, alterID))
			}
		}
	} else if node.Scheme == "trojan" {
		parts = append(parts, fmt.Sprintf(`"password":%q`, node.UUID))
	} else if node.Scheme == "hysteria2" {
		// Password is required for Hysteria2
		if password, ok := node.Outbound["password"].(string); ok && password != "" {
			passwordJSON, err := json.Marshal(password)
			if err != nil {
				return "", fmt.Errorf("failed to marshal hysteria2 password: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"password":%s`, string(passwordJSON)))
		}
		// server_ports (optional) - array of port ranges for sing-box 1.9+
		if serverPorts, ok := node.Outbound["server_ports"].([]string); ok && len(serverPorts) > 0 {
			serverPortsJSON, err := json.Marshal(serverPorts)
			if err != nil {
				return "", fmt.Errorf("failed to marshal hysteria2 server_ports: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"server_ports":%s`, string(serverPortsJSON)))
		}
		// up_mbps (optional)
		if upMbps, ok := node.Outbound["up_mbps"].(int); ok && upMbps > 0 {
			parts = append(parts, fmt.Sprintf(`"up_mbps":%d`, upMbps))
		}
		// down_mbps (optional)
		if downMbps, ok := node.Outbound["down_mbps"].(int); ok && downMbps > 0 {
			parts = append(parts, fmt.Sprintf(`"down_mbps":%d`, downMbps))
		}
		// obfs (optional)
		if obfs, ok := node.Outbound["obfs"].(map[string]interface{}); ok && len(obfs) > 0 {
			var obfsParts []string
			if obfsType, ok := obfs["type"].(string); ok {
				obfsParts = append(obfsParts, fmt.Sprintf(`"type":%q`, obfsType))
			}
			if obfsPassword, ok := obfs["password"].(string); ok && obfsPassword != "" {
				obfsPasswordJSON, err := json.Marshal(obfsPassword)
				if err != nil {
					return "", fmt.Errorf("failed to marshal hysteria2 obfs password: %w", err)
				}
				obfsParts = append(obfsParts, fmt.Sprintf(`"password":%s`, string(obfsPasswordJSON)))
			}
			if len(obfsParts) > 0 {
				obfsJSON := "{" + strings.Join(obfsParts, ",") + "}"
				parts = append(parts, fmt.Sprintf(`"obfs":%s`, obfsJSON))
			}
		}
	} else if node.Scheme == "ss" {
		// Extract method and password from outbound
		// Use json.Marshal to properly escape strings for JSON (handles binary data correctly)
		// This prevents invalid \xXX escape sequences that JSON doesn't support
		if method, ok := node.Outbound["method"].(string); ok && method != "" {
			methodJSON, err := json.Marshal(method)
			if err != nil {
				return "", fmt.Errorf("failed to marshal shadowsocks method: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"method":%s`, string(methodJSON)))
		}
		if password, ok := node.Outbound["password"].(string); ok && password != "" {
			passwordJSON, err := json.Marshal(password)
			if err != nil {
				return "", fmt.Errorf("failed to marshal shadowsocks password: %w", err)
			}
			parts = append(parts, fmt.Sprintf(`"password":%s`, string(passwordJSON)))
		}
	}

	// 6. flow (if present) — use node.Outbound["flow"] when set so Xray-only values like
	// xtls-rprx-vision-udp443 stay in node.Flow for filters but sing-box gets xtls-rprx-vision
	flowOut := node.Flow
	if node.Outbound != nil {
		if f, ok := node.Outbound["flow"].(string); ok && f != "" {
			flowOut = f
		}
	}
	if flowOut != "" {
		parts = append(parts, fmt.Sprintf(`"flow":%q`, flowOut))
	}
	if node.Scheme == "vless" && node.Outbound != nil {
		if pe, ok := node.Outbound["packet_encoding"].(string); ok && pe != "" {
			parts = append(parts, fmt.Sprintf(`"packet_encoding":%q`, pe))
		}
	}

	parts = appendOutboundTransportParts(parts, node.Outbound)

	// 7. tls (if present) - with correct field order
	if node.Outbound != nil {
		if tlsData, ok := node.Outbound["tls"].(map[string]interface{}); ok {
			if disabled, ok := tlsData["enabled"].(bool); ok && !disabled {
				parts = append(parts, `"tls":{"enabled":false}`)
			} else {
				var tlsParts []string

				if enabled, ok := tlsData["enabled"].(bool); ok {
					tlsParts = append(tlsParts, fmt.Sprintf(`"enabled":%v`, enabled))
				}

				if serverName, ok := tlsData["server_name"].(string); ok && serverName != "" {
					tlsParts = append(tlsParts, fmt.Sprintf(`"server_name":%q`, serverName))
				}

				if alpn, ok := tlsData["alpn"].([]string); ok && len(alpn) > 0 {
					alpnJSON, _ := json.Marshal(alpn)
					tlsParts = append(tlsParts, fmt.Sprintf(`"alpn":%s`, string(alpnJSON)))
				}

				if utls, ok := tlsData["utls"].(map[string]interface{}); ok {
					var utlsParts []string
					if utlsEnabled, ok := utls["enabled"].(bool); ok {
						utlsParts = append(utlsParts, fmt.Sprintf(`"enabled":%v`, utlsEnabled))
					}
					if fingerprint, ok := utls["fingerprint"].(string); ok {
						utlsParts = append(utlsParts, fmt.Sprintf(`"fingerprint":%q`, fingerprint))
					}
					utlsJSON := "{" + strings.Join(utlsParts, ",") + "}"
					tlsParts = append(tlsParts, fmt.Sprintf(`"utls":%s`, utlsJSON))
				}

				if insecure, ok := tlsData["insecure"].(bool); ok && insecure {
					tlsParts = append(tlsParts, fmt.Sprintf(`"insecure":%v`, insecure))
				}

				if reality, ok := tlsData["reality"].(map[string]interface{}); ok {
					var realityParts []string
					if realityEnabled, ok := reality["enabled"].(bool); ok {
						realityParts = append(realityParts, fmt.Sprintf(`"enabled":%v`, realityEnabled))
					}
					if publicKey, ok := reality["public_key"].(string); ok {
						realityParts = append(realityParts, fmt.Sprintf(`"public_key":%q`, publicKey))
					}
					if shortID, ok := reality["short_id"].(string); ok {
						realityParts = append(realityParts, fmt.Sprintf(`"short_id":%q`, shortID))
					}
					realityJSON := "{" + strings.Join(realityParts, ",") + "}"
					tlsParts = append(tlsParts, fmt.Sprintf(`"reality":%s`, realityJSON))
				}

				tlsJSON := "{" + strings.Join(tlsParts, ",") + "}"
				parts = append(parts, fmt.Sprintf(`"tls":%s`, tlsJSON))
			}
		}
	}

	// Build final JSON
	jsonStr := "{" + strings.Join(parts, ",") + "}"
	return fmt.Sprintf("\t// %s\n\t%s,", node.Label, jsonStr), nil
}

// GenerateSelectorWithFilteredAddOutbounds builds one selector/urltest outbound as a JSON string.
// Used in pass 3: only valid selectors are generated, and addOutbounds are filtered so that
// dynamic refs point only to selectors with isValid == true; constants (e.g. direct-out, auto-proxy-out) are always included.
// Nodes are filtered by outboundConfig.Filters (tag, host, scheme, etc.; literal and /regex/i). default is set from preferredDefault when specified.
// Returned string is one line (or comment + line), with trailing comma, ready to concatenate into the outbounds array.
func GenerateSelectorWithFilteredAddOutbounds(
	allNodes []*ParsedNode,
	outboundConfig OutboundConfig,
	outboundsInfo map[string]*outboundInfo,
) (string, error) {
	// Filter nodes based on filters (version 3)
	filterMap := outboundConfig.Filters
	debuglog.DebugLog("Parser: GenerateSelectorWithFilteredAddOutbounds for '%s' (type: %s): filters=%v, addOutbounds=%v, allNodes=%d",
		outboundConfig.Tag, outboundConfig.Type, filterMap, outboundConfig.AddOutbounds, len(allNodes))

	filteredNodes := filterNodesForSelector(allNodes, filterMap)
	debuglog.DebugLog("Parser: filterNodesForSelector returned %d nodes for '%s'", len(filteredNodes), outboundConfig.Tag)

	// Build outbounds list with unique tags
	// Pre-allocate with estimated capacity to reduce allocations
	estimatedSize := len(outboundConfig.AddOutbounds) + len(filteredNodes)
	outboundsList := make([]string, 0, estimatedSize)
	seenTags := make(map[string]bool, estimatedSize)
	duplicateCountInSelector := 0

	// Add addOutbounds first (version 3) - only valid dynamic ones + all constants
	addOutboundsList := outboundConfig.AddOutbounds
	if len(addOutboundsList) > 0 {
		debuglog.DebugLog("Parser: Processing %d addOutbounds for selector '%s'", len(addOutboundsList), outboundConfig.Tag)
		for _, tag := range addOutboundsList {
			if seenTags[tag] {
				duplicateCountInSelector++
				debuglog.DebugLog("Parser: Skipping duplicate tag '%s' in addOutbounds for selector '%s'", tag, outboundConfig.Tag)
				continue
			}

			if addInfo, exists := outboundsInfo[tag]; exists {
				// This is a dynamically created outbound - check if it's valid
				if addInfo.isValid {
					outboundsList = append(outboundsList, tag)
					seenTags[tag] = true
					debuglog.DebugLog("Parser: Adding valid dynamic addOutbound '%s' to selector '%s'", tag, outboundConfig.Tag)
				} else {
					debuglog.DebugLog("Parser: Skipping invalid (empty) dynamic addOutbound '%s' for selector '%s'", tag, outboundConfig.Tag)
				}
			} else {
				// This is a constant from template (direct-out, auto-proxy-out, etc.)
				// Constants always exist, always add them
				outboundsList = append(outboundsList, tag)
				seenTags[tag] = true
				debuglog.DebugLog("Parser: Adding constant addOutbound '%s' to selector '%s'", tag, outboundConfig.Tag)
			}
		}
	}

	// Add filtered node tags (without duplicates)
	debuglog.DebugLog("Parser: Processing %d filtered nodes for selector '%s'", len(filteredNodes), outboundConfig.Tag)
	for _, node := range filteredNodes {
		if !seenTags[node.Tag] {
			outboundsList = append(outboundsList, node.Tag)
			seenTags[node.Tag] = true
		} else {
			duplicateCountInSelector++
			debuglog.DebugLog("Parser: Skipping duplicate tag '%s' in filtered nodes for selector '%s'", node.Tag, outboundConfig.Tag)
		}
	}

	// Check if we have any outbounds at all (addOutbounds + filteredNodes)
	if len(outboundsList) == 0 {
		debuglog.DebugLog("Parser: No outbounds (neither addOutbounds nor filteredNodes) for %s '%s'", outboundConfig.Type, outboundConfig.Tag)
		return "", nil
	}

	if duplicateCountInSelector > 0 {
		debuglog.DebugLog("Parser: Removed %d duplicate tags from selector '%s' outbounds list", duplicateCountInSelector, outboundConfig.Tag)
	}
	debuglog.DebugLog("Parser: Selector '%s' will have %d unique outbounds", outboundConfig.Tag, len(outboundsList))

	// Determine default - only if preferredDefault is specified in config (version 3)
	preferredDefaultMap := outboundConfig.PreferredDefault
	defaultTag := ""
	if len(preferredDefaultMap) > 0 {
		// Find first node matching preferredDefault filter
		preferredFilter := convertFilterToStringMap(preferredDefaultMap)
		for _, node := range filteredNodes {
			if matchesFilter(node, preferredFilter) {
				defaultTag = node.Tag
				break
			}
		}
	}
	// Note: We do NOT automatically set default to first node if preferredDefault is not specified
	// This allows urltest/selector to work without a default value when preferredDefault is not configured

	// Build selector JSON with correct field order
	var parts []string

	// 1. tag
	parts = append(parts, fmt.Sprintf(`"tag":%q`, outboundConfig.Tag))

	// 2. type
	parts = append(parts, fmt.Sprintf(`"type":%q`, outboundConfig.Type))

	// 3. default (if present) - BEFORE outbounds
	if defaultTag != "" {
		parts = append(parts, fmt.Sprintf(`"default":%q`, defaultTag))
	}

	// 4. outbounds
	outboundsJSON, _ := json.Marshal(outboundsList)
	parts = append(parts, fmt.Sprintf(`"outbounds":%s`, string(outboundsJSON)))

	// 5. interrupt_exist_connections (if present)
	if val, ok := outboundConfig.Options["interrupt_exist_connections"]; ok {
		if boolVal, ok := val.(bool); ok {
			parts = append(parts, fmt.Sprintf(`"interrupt_exist_connections":%v`, boolVal))
		} else {
			valJSON, _ := json.Marshal(val)
			parts = append(parts, fmt.Sprintf(`"interrupt_exist_connections":%s`, string(valJSON)))
		}
	}

	// 6. Other options (in order they appear)
	for key, value := range outboundConfig.Options {
		if key != "interrupt_exist_connections" {
			valJSON, _ := json.Marshal(value)
			parts = append(parts, fmt.Sprintf(`%q:%s`, key, string(valJSON)))
		}
	}

	// Build final JSON
	jsonStr := "{" + strings.Join(parts, ",") + "}"

	// Add comment if present
	result := ""
	if outboundConfig.Comment != "" {
		result = fmt.Sprintf("\t// %s\n", outboundConfig.Comment)
	}
	result += fmt.Sprintf("\t%s,", jsonStr)

	return result, nil
}

// GenerateEndpointJSON returns a single JSON object string for one WireGuard endpoint (sing-box endpoints array).
// node.Outbound must contain the full endpoint map built by the wireguard URI parser.
// Uses node.Tag (with tag_prefix applied by source) for the endpoint "tag" so selectors can reference it.
// Returned string is pretty-printed (multi-line); trailing comma is added by the caller when inserting into the array.
func GenerateEndpointJSON(node *ParsedNode) (string, error) {
	if node.Scheme != "wireguard" || node.Outbound == nil {
		return "", fmt.Errorf("GenerateEndpointJSON requires wireguard node with Outbound set")
	}
	// Use node.Tag (includes tag_prefix, e.g. "4:wg-parnas") so endpoint tag matches outbound references
	endpoint := make(map[string]interface{})
	for k, v := range node.Outbound {
		endpoint[k] = v
	}
	if node.Tag != "" {
		endpoint["tag"] = node.Tag
	}
	jsonBytes, err := json.MarshalIndent(endpoint, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal wireguard endpoint: %w", err)
	}
	result := ""
	if node.Comment != "" {
		result = "// " + node.Comment + "\n"
	}
	result += string(jsonBytes)
	return result, nil
}

// buildOutboundsInfo implements pass 1: builds the outboundsInfo map for every selector (local and global).
// For each selector we store config, filtered nodes (from Filters), and outboundCount = len(filteredNodes).
// isValid is left false; it is set in pass 2. Duplicate tags are logged via logDuplicateTagIfExists.
func buildOutboundsInfo(
	parserConfig *ParserConfig,
	nodesBySource map[int][]*ParsedNode,
	allNodes []*ParsedNode,
	progressCallback func(float64, string),
) map[string]*outboundInfo {
	if progressCallback != nil {
		progressCallback(60, "Analyzing outbounds (pass 1)...")
	}
	outboundsInfo := make(map[string]*outboundInfo)

	for i, proxySource := range parserConfig.ParserConfig.Proxies {
		if len(proxySource.Outbounds) == 0 {
			continue
		}
		sourceNodes, ok := nodesBySource[i]
		if !ok {
			sourceNodes = []*ParsedNode{}
		}
		for _, outboundConfig := range proxySource.Outbounds {
			filteredNodes := filterNodesForSelector(sourceNodes, outboundConfig.Filters)
			logDuplicateTagIfExists(outboundsInfo, outboundConfig.Tag, "local", i+1)
			outboundsInfo[outboundConfig.Tag] = &outboundInfo{
				config:        outboundConfig,
				filteredNodes: filteredNodes,
				outboundCount: len(filteredNodes),
				isValid:       false,
				isLocal:       true,
			}
		}
	}

	for _, outboundConfig := range parserConfig.ParserConfig.Outbounds {
		filteredNodes := filterNodesForSelector(allNodes, outboundConfig.Filters)
		logDuplicateTagIfExists(outboundsInfo, outboundConfig.Tag, "global", 0)
		outboundsInfo[outboundConfig.Tag] = &outboundInfo{
			config:        outboundConfig,
			filteredNodes: filteredNodes,
			outboundCount: len(filteredNodes),
			isValid:       false,
			isLocal:       false,
		}
	}
	return outboundsInfo
}

// logDuplicateTagIfExists logs a warning when a new selector tag already exists in outboundsInfo.
// kind is "local" or "global"; sourceIndex is the 1-based proxy source index (used only when kind == "local").
func logDuplicateTagIfExists(outboundsInfo map[string]*outboundInfo, tag, kind string, sourceIndex int) {
	existingInfo, exists := outboundsInfo[tag]
	if !exists {
		return
	}
	if kind == "local" {
		selectorType := "global"
		if existingInfo.isLocal {
			selectorType = "local"
		}
		debuglog.WarnLog("GenerateOutboundsFromParserConfig: Duplicate tag '%s' detected. "+
			"Local selector from source %d will overwrite %s selector. This may cause unexpected behavior.",
			tag, sourceIndex, selectorType)
	} else {
		if existingInfo.isLocal {
			debuglog.WarnLog("GenerateOutboundsFromParserConfig: Duplicate tag '%s' detected. "+
				"Global selector will overwrite local selector. This may cause unexpected behavior.", tag)
		} else {
			debuglog.WarnLog("GenerateOutboundsFromParserConfig: Duplicate tag '%s' detected. "+
				"Multiple global selectors with same tag. This may cause unexpected behavior.", tag)
		}
	}
}

// computeOutboundValidity implements pass 2: topological sort over selectors by addOutbounds dependencies,
// then for each selector (in that order) sets outboundCount = len(filteredNodes) + count of valid addOutbounds
// (dynamic with outboundCount > 0 plus constants). isValid = (outboundCount > 0). Uses Kahn's algorithm;
// if not all selectors are processed, a cycle is reported in the log.
func computeOutboundValidity(outboundsInfo map[string]*outboundInfo, progressCallback func(float64, string)) {
	if progressCallback != nil {
		progressCallback(70, "Calculating outbound dependencies (pass 2)...")
	}
	dependents := make(map[string][]string, len(outboundsInfo))
	inDegree := make(map[string]int, len(outboundsInfo))
	for tag := range outboundsInfo {
		inDegree[tag] = 0
		dependents[tag] = []string{}
	}
	for tag, info := range outboundsInfo {
		for _, addTag := range info.config.AddOutbounds {
			if _, exists := outboundsInfo[addTag]; exists {
				dependents[addTag] = append(dependents[addTag], tag)
				inDegree[tag]++
			}
		}
	}
	queue := make([]string, 0, len(outboundsInfo))
	for tag, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, tag)
		}
	}
	processedCount := 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		info := outboundsInfo[current]
		totalCount := len(info.filteredNodes)
		for _, addTag := range info.config.AddOutbounds {
			if addInfo, exists := outboundsInfo[addTag]; exists {
				if addInfo.outboundCount > 0 {
					totalCount++
				}
			} else {
				totalCount++
			}
		}
		info.outboundCount = totalCount
		info.isValid = (totalCount > 0)
		processedCount++
		for _, dependent := range dependents[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}
	if processedCount != len(outboundsInfo) {
		unprocessed := make([]string, 0)
		for tag := range outboundsInfo {
			if inDegree[tag] > 0 {
				unprocessed = append(unprocessed, tag)
			}
		}
		debuglog.WarnLog("GenerateOutboundsFromParserConfig: Not all outbounds processed (processed: %d, total: %d). "+
			"Possible cycles in dependency graph. Unprocessed outbounds: %v",
			processedCount, len(outboundsInfo), unprocessed)
	}
}

// generateSelectorJSONs implements pass 3: iterates local then global selectors, and for each with isValid == true
// calls GenerateSelectorWithFilteredAddOutbounds and appends the result. Returns the slice of selector JSON strings
// and the local and global counts.
func generateSelectorJSONs(
	parserConfig *ParserConfig,
	nodesBySource map[int][]*ParsedNode,
	allNodes []*ParsedNode,
	outboundsInfo map[string]*outboundInfo,
	progressCallback func(float64, string),
) ([]string, int, int) {
	if progressCallback != nil {
		progressCallback(80, "Generating selectors (pass 3)...")
	}
	var out []string
	localCount := 0
	globalCount := 0

	for i, proxySource := range parserConfig.ParserConfig.Proxies {
		if len(proxySource.Outbounds) == 0 {
			continue
		}
		sourceNodes, ok := nodesBySource[i]
		if !ok {
			sourceNodes = []*ParsedNode{}
		}
		for _, outboundConfig := range proxySource.Outbounds {
			info, exists := outboundsInfo[outboundConfig.Tag]
			if !exists || !info.isValid {
				if exists && !info.isValid {
					debuglog.DebugLog("GenerateOutboundsFromParserConfig: Skipping empty local selector '%s'", outboundConfig.Tag)
				}
				continue
			}
			selectorJSON, err := GenerateSelectorWithFilteredAddOutbounds(sourceNodes, outboundConfig, outboundsInfo)
			if err != nil {
				debuglog.WarnLog("GenerateOutboundsFromParserConfig: Failed to generate local selector %s for source %d: %v",
					outboundConfig.Tag, i+1, err)
				continue
			}
			if selectorJSON != "" {
				out = append(out, selectorJSON)
				localCount++
			}
		}
	}

	for _, outboundConfig := range parserConfig.ParserConfig.Outbounds {
		info, exists := outboundsInfo[outboundConfig.Tag]
		if !exists || !info.isValid {
			if exists && !info.isValid {
				debuglog.DebugLog("GenerateOutboundsFromParserConfig: Skipping empty global selector '%s'", outboundConfig.Tag)
			}
			continue
		}
		selectorJSON, err := GenerateSelectorWithFilteredAddOutbounds(allNodes, outboundConfig, outboundsInfo)
		if err != nil {
			debuglog.WarnLog("GenerateOutboundsFromParserConfig: Failed to generate global selector %s: %v",
				outboundConfig.Tag, err)
			continue
		}
		if selectorJSON != "" {
			out = append(out, selectorJSON)
			globalCount++
		}
	}
	return out, localCount, globalCount
}

// GenerateOutboundsFromParserConfig is the main entry point: loads nodes from all proxy sources via loadNodesFunc,
// generates node JSONs, then runs the three passes (buildOutboundsInfo, computeOutboundValidity, generateSelectorJSONs)
// and returns the concatenated JSON strings (nodes, then local selectors, then global selectors) plus counts.
// progressCallback(0–100, message) is optional for UI progress. tagCounts is passed to loadNodesFunc for deduplication.
func GenerateOutboundsFromParserConfig(
	parserConfig *ParserConfig,
	tagCounts map[string]int,
	progressCallback func(float64, string),
	loadNodesFunc func(ProxySource, map[string]int, func(float64, string), int, int) ([]*ParsedNode, error),
) (*OutboundGenerationResult, error) {
	// Step 1: Process all proxy sources and collect nodes
	allNodes := make([]*ParsedNode, 0)
	nodesBySource := make(map[int][]*ParsedNode) // Map source index to its nodes

	totalSources := len(parserConfig.ParserConfig.Proxies)
	if progressCallback != nil {
		progressCallback(10, fmt.Sprintf("Processing %d sources...", totalSources))
	}

	for i, proxySource := range parserConfig.ParserConfig.Proxies {
		if progressCallback != nil {
			progressCallback(10+float64(i)*30.0/float64(totalSources),
				fmt.Sprintf("Processing source %d/%d...", i+1, totalSources))
		}

		nodesFromSource, err := loadNodesFunc(proxySource, tagCounts, progressCallback, i, totalSources)
		if err != nil {
			debuglog.ErrorLog("GenerateOutboundsFromParserConfig: Error processing source %d/%d: %v", i+1, totalSources, err)
			continue
		}

		if len(nodesFromSource) > 0 {
			allNodes = append(allNodes, nodesFromSource...)
			nodesBySource[i] = nodesFromSource
		}
	}

	if len(allNodes) == 0 {
		return nil, fmt.Errorf("no nodes parsed from any source")
	}

	// Step 2: Generate JSON for all nodes
	if progressCallback != nil {
		progressCallback(40, fmt.Sprintf("Generating JSON for %d nodes...", len(allNodes)))
	}

	selectorsJSON := make([]string, 0)
	endpointsJSON := make([]string, 0)
	nodesCount := 0
	endpointsCount := 0

	for _, node := range allNodes {
		if node.Scheme == "wireguard" {
			epJSON, err := GenerateEndpointJSON(node)
			if err != nil {
				debuglog.WarnLog("GenerateOutboundsFromParserConfig: Failed to generate JSON for endpoint %s: %v", node.Tag, err)
				continue
			}
			endpointsJSON = append(endpointsJSON, epJSON)
			endpointsCount++
		} else {
			nodeJSON, err := GenerateNodeJSON(node)
			if err != nil {
				debuglog.WarnLog("GenerateOutboundsFromParserConfig: Failed to generate JSON for node %s: %v", node.Tag, err)
				continue
			}
			selectorsJSON = append(selectorsJSON, nodeJSON)
			nodesCount++
		}
	}

	outboundsInfo := buildOutboundsInfo(parserConfig, nodesBySource, allNodes, progressCallback)
	computeOutboundValidity(outboundsInfo, progressCallback)
	selectorJSONs, localSelectorsCount, globalSelectorsCount := generateSelectorJSONs(parserConfig, nodesBySource, allNodes, outboundsInfo, progressCallback)
	selectorsJSON = append(selectorsJSON, selectorJSONs...)

	return &OutboundGenerationResult{
		OutboundsJSON:        selectorsJSON,
		EndpointsJSON:        endpointsJSON,
		NodesCount:           nodesCount,
		EndpointsCount:       endpointsCount,
		LocalSelectorsCount:  localSelectorsCount,
		GlobalSelectorsCount: globalSelectorsCount,
	}, nil
}


