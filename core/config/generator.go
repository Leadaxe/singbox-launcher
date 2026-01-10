package config

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
)

// OutboundGenerationResult contains the result of outbound generation with statistics
type OutboundGenerationResult struct {
	OutboundsJSON        []string // Array of generated JSON strings (nodes + selectors)
	NodesCount           int      // Number of generated nodes
	LocalSelectorsCount  int      // Number of local selectors
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

// GenerateNodeJSON generates JSON string for a parsed node with correct field order.
// Handles all proxy types (vless, vmess, trojan, shadowsocks) and includes
// TLS configuration, transport settings, and other protocol-specific fields.
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

	// 4. server_port
	parts = append(parts, fmt.Sprintf(`"server_port":%d`, node.Port))

	// 5. uuid (for vless/vmess) or password (for trojan) or method/password (for ss)
	if node.Scheme == "vless" || node.Scheme == "vmess" {
		parts = append(parts, fmt.Sprintf(`"uuid":%q`, node.UUID))

		// For VMESS add additional fields
		if node.Scheme == "vmess" {
			// security
			if security, ok := node.Outbound["security"].(string); ok && security != "" {
				parts = append(parts, fmt.Sprintf(`"security":%q`, security))
			}

			// alter_id
			if alterID, ok := node.Outbound["alter_id"].(int); ok {
				parts = append(parts, fmt.Sprintf(`"alter_id":%d`, alterID))
			}

			// НЕ добавляем поле network - sing-box не поддерживает его для vmess
			// Используем только transport для ws/http/grpc

			// transport
			if transport, ok := node.Outbound["transport"].(map[string]interface{}); ok && len(transport) > 0 {
				var transportParts []string
				if tType, ok := transport["type"].(string); ok {
					transportParts = append(transportParts, fmt.Sprintf(`"type":%q`, tType))
				}
				if path, ok := transport["path"].(string); ok {
					transportParts = append(transportParts, fmt.Sprintf(`"path":%q`, path))
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

	// 6. flow (if present)
	if node.Flow != "" {
		parts = append(parts, fmt.Sprintf(`"flow":%q`, node.Flow))
	}

	// 7. tls (if present) - with correct field order
	if tlsData, ok := node.Outbound["tls"].(map[string]interface{}); ok {
		var tlsParts []string

		// enabled
		if enabled, ok := tlsData["enabled"].(bool); ok {
			tlsParts = append(tlsParts, fmt.Sprintf(`"enabled":%v`, enabled))
		}

		// server_name
		if serverName, ok := tlsData["server_name"].(string); ok {
			tlsParts = append(tlsParts, fmt.Sprintf(`"server_name":%q`, serverName))
		}

		// alpn (for VMESS and Hysteria2)
		if alpn, ok := tlsData["alpn"].([]string); ok && len(alpn) > 0 {
			alpnJSON, _ := json.Marshal(alpn)
			tlsParts = append(tlsParts, fmt.Sprintf(`"alpn":%s`, string(alpnJSON)))
		}

		// utls
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

		// insecure (for VMESS and Hysteria2)
		if insecure, ok := tlsData["insecure"].(bool); ok && insecure {
			tlsParts = append(tlsParts, fmt.Sprintf(`"insecure":%v`, insecure))
		}

		// reality
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

	// Build final JSON
	jsonStr := "{" + strings.Join(parts, ",") + "}"
	return fmt.Sprintf("\t// %s\n\t%s,", node.Label, jsonStr), nil
}

// GenerateSelectorWithFilteredAddOutbounds generates JSON string for a selector with filtered addOutbounds.
// This function implements the third pass of the three-pass generation algorithm:
//   - Filters nodes based on outboundConfig.Filters
//   - Adds addOutbounds, but only includes dynamic outbounds that are valid (isValid == true)
//   - Constants (not in outboundsInfo, e.g., "direct-out", "auto-proxy-out") are always added
//   - Determines default outbound from preferredDefault if specified
//   - Builds the selector JSON with correct field order
//
// Parameters:
//   - allNodes: All available parsed nodes to filter from
//   - outboundConfig: The selector configuration to generate JSON for
//   - outboundsInfo: Map of tag -> outboundInfo for all dynamically created selectors (used to check validity)
//
// Returns:
//   - JSON string representation of the selector, or empty string if no valid outbounds found
//   - error if JSON generation fails
func GenerateSelectorWithFilteredAddOutbounds(
	allNodes []*ParsedNode,
	outboundConfig OutboundConfig,
	outboundsInfo map[string]*outboundInfo,
) (string, error) {
	// Filter nodes based on filters (version 3)
	filterMap := outboundConfig.Filters
	log.Printf("Parser: GenerateSelectorWithFilteredAddOutbounds for '%s' (type: %s): filters=%v, addOutbounds=%v, allNodes=%d",
		outboundConfig.Tag, outboundConfig.Type, filterMap, outboundConfig.AddOutbounds, len(allNodes))

	filteredNodes := filterNodesForSelector(allNodes, filterMap)
	log.Printf("Parser: filterNodesForSelector returned %d nodes for '%s'", len(filteredNodes), outboundConfig.Tag)

	// Build outbounds list with unique tags
	// Pre-allocate with estimated capacity to reduce allocations
	estimatedSize := len(outboundConfig.AddOutbounds) + len(filteredNodes)
	outboundsList := make([]string, 0, estimatedSize)
	seenTags := make(map[string]bool, estimatedSize)
	duplicateCountInSelector := 0

	// Add addOutbounds first (version 3) - only valid dynamic ones + all constants
	addOutboundsList := outboundConfig.AddOutbounds
	if len(addOutboundsList) > 0 {
		log.Printf("Parser: Processing %d addOutbounds for selector '%s'", len(addOutboundsList), outboundConfig.Tag)
		for _, tag := range addOutboundsList {
			if seenTags[tag] {
				duplicateCountInSelector++
				log.Printf("Parser: Skipping duplicate tag '%s' in addOutbounds for selector '%s'", tag, outboundConfig.Tag)
				continue
			}

			if addInfo, exists := outboundsInfo[tag]; exists {
				// This is a dynamically created outbound - check if it's valid
				if addInfo.isValid {
					outboundsList = append(outboundsList, tag)
					seenTags[tag] = true
					log.Printf("Parser: Adding valid dynamic addOutbound '%s' to selector '%s'", tag, outboundConfig.Tag)
				} else {
					log.Printf("Parser: Skipping invalid (empty) dynamic addOutbound '%s' for selector '%s'", tag, outboundConfig.Tag)
				}
			} else {
				// This is a constant from template (direct-out, auto-proxy-out, etc.)
				// Constants always exist, always add them
				outboundsList = append(outboundsList, tag)
				seenTags[tag] = true
				log.Printf("Parser: Adding constant addOutbound '%s' to selector '%s'", tag, outboundConfig.Tag)
			}
		}
	}

	// Add filtered node tags (without duplicates)
	log.Printf("Parser: Processing %d filtered nodes for selector '%s'", len(filteredNodes), outboundConfig.Tag)
	for _, node := range filteredNodes {
		if !seenTags[node.Tag] {
			outboundsList = append(outboundsList, node.Tag)
			seenTags[node.Tag] = true
		} else {
			duplicateCountInSelector++
			log.Printf("Parser: Skipping duplicate tag '%s' in filtered nodes for selector '%s'", node.Tag, outboundConfig.Tag)
		}
	}

	// Check if we have any outbounds at all (addOutbounds + filteredNodes)
	if len(outboundsList) == 0 {
		log.Printf("Parser: No outbounds (neither addOutbounds nor filteredNodes) for %s '%s'", outboundConfig.Type, outboundConfig.Tag)
		return "", nil
	}

	if duplicateCountInSelector > 0 {
		log.Printf("Parser: Removed %d duplicate tags from selector '%s' outbounds list", duplicateCountInSelector, outboundConfig.Tag)
	}
	log.Printf("Parser: Selector '%s' will have %d unique outbounds", outboundConfig.Tag, len(outboundsList))

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

// GenerateSelector generates JSON string for a selector from filtered nodes.
// Filters nodes based on outboundConfig.Filters, adds addOutbounds,
// determines default outbound from preferredDefault if specified, and builds
// the selector JSON with correct field order.
// NOTE: This function is kept for backward compatibility but doesn't filter addOutbounds.
// Use GenerateSelectorWithFilteredAddOutbounds for new code that needs filtering.
func GenerateSelector(allNodes []*ParsedNode, outboundConfig OutboundConfig) (string, error) {
	// Filter nodes based on filters (version 3)
	filterMap := outboundConfig.Filters
	log.Printf("Parser: GenerateSelector for '%s' (type: %s): filters=%v, addOutbounds=%v, allNodes=%d",
		outboundConfig.Tag, outboundConfig.Type, filterMap, outboundConfig.AddOutbounds, len(allNodes))

	filteredNodes := filterNodesForSelector(allNodes, filterMap)
	log.Printf("Parser: filterNodesForSelector returned %d nodes for '%s'", len(filteredNodes), outboundConfig.Tag)

	// Build outbounds list with unique tags
	outboundsList := make([]string, 0)
	seenTags := make(map[string]bool)
	duplicateCountInSelector := 0

	// Add addOutbounds first (version 3)
	addOutboundsList := outboundConfig.AddOutbounds
	if len(addOutboundsList) > 0 {
		log.Printf("Parser: Adding %d addOutbounds to selector '%s'", len(addOutboundsList), outboundConfig.Tag)
		for _, tag := range addOutboundsList {
			if !seenTags[tag] {
				outboundsList = append(outboundsList, tag)
				seenTags[tag] = true
			} else {
				duplicateCountInSelector++
				log.Printf("Parser: Skipping duplicate tag '%s' in addOutbounds for selector '%s'", tag, outboundConfig.Tag)
			}
		}
	}

	// Add filtered node tags (without duplicates)
	log.Printf("Parser: Processing %d filtered nodes for selector '%s'", len(filteredNodes), outboundConfig.Tag)
	for _, node := range filteredNodes {
		if !seenTags[node.Tag] {
			outboundsList = append(outboundsList, node.Tag)
			seenTags[node.Tag] = true
		} else {
			duplicateCountInSelector++
			log.Printf("Parser: Skipping duplicate tag '%s' in filtered nodes for selector '%s'", node.Tag, outboundConfig.Tag)
		}
	}

	// Check if we have any outbounds at all (addOutbounds + filteredNodes)
	if len(outboundsList) == 0 {
		log.Printf("Parser: No outbounds (neither addOutbounds nor filteredNodes) for %s '%s'", outboundConfig.Type, outboundConfig.Tag)
		return "", nil
	}

	if duplicateCountInSelector > 0 {
		log.Printf("Parser: Removed %d duplicate tags from selector '%s' outbounds list", duplicateCountInSelector, outboundConfig.Tag)
	}
	log.Printf("Parser: Selector '%s' will have %d unique outbounds", outboundConfig.Tag, len(outboundsList))

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

// GenerateOutboundsFromParserConfig processes ParserConfig and generates all outbounds using a three-pass algorithm.
//
// The three-pass algorithm ensures that dynamic addOutbounds are only added if they are valid (non-empty):
//
// Pass 1: Creates outboundsInfo map for all selectors and counts only filtered nodes (without addOutbounds).
//
// Pass 2: Performs topological sorting to process selectors in dependency order, then calculates total
//
//	outboundCount for each selector (filteredNodes + valid addOutbounds). Sets isValid flag for each selector.
//
// Pass 3: Generates JSON only for valid selectors, filtering addOutbounds to include only:
//   - Dynamic selectors that are valid (isValid == true)
//   - Constants (always included, e.g., "direct-out", "auto-proxy-out")
//
// Returns array of JSON strings: first all nodes, then local selectors (per source), then global selectors.
// This function eliminates code duplication between UpdateConfigFromSubscriptions and parseAndPreview.
//
// Parameters:
//   - parserConfig: The parser configuration containing proxy sources and outbound definitions
//   - tagCounts: Map for tracking tag usage counts (passed to loadNodesFunc)
//   - progressCallback: Optional callback for progress updates (progress 0-100, status message)
//   - loadNodesFunc: Function to load and parse nodes from a ProxySource
//
// Returns:
//   - OutboundGenerationResult with generated JSON strings and statistics
//   - error if no nodes are parsed or if generation fails
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
			log.Printf("GenerateOutboundsFromParserConfig: Error processing source %d/%d: %v", i+1, totalSources, err)
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
	nodesCount := 0

	for _, node := range allNodes {
		nodeJSON, err := GenerateNodeJSON(node)
		if err != nil {
			log.Printf("GenerateOutboundsFromParserConfig: Warning: Failed to generate JSON for node %s: %v", node.Tag, err)
			continue
		}
		selectorsJSON = append(selectorsJSON, nodeJSON)
		nodesCount++
	}

	// Step 3: Pass 1 - Create outboundsInfo and count nodes only
	// Build map of all dynamically created outbounds (local + global)
	outboundsInfo := make(map[string]*outboundInfo)

	if progressCallback != nil {
		progressCallback(60, "Analyzing outbounds (pass 1)...")
	}

	// Process local selectors (per source)
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

			// Check for duplicate tags (local selector with same tag as existing one)
			if existingInfo, exists := outboundsInfo[outboundConfig.Tag]; exists {
				selectorType := "global"
				if existingInfo.isLocal {
					selectorType = "local"
				}
				log.Printf("GenerateOutboundsFromParserConfig: Warning: Duplicate tag '%s' detected. "+
					"Local selector from source %d will overwrite %s selector. This may cause unexpected behavior.",
					outboundConfig.Tag, i+1, selectorType)
				_ = existingInfo // Suppress unused variable warning (used in log message construction)
			}

			outboundsInfo[outboundConfig.Tag] = &outboundInfo{
				config:        outboundConfig,
				filteredNodes: filteredNodes,
				outboundCount: len(filteredNodes), // Pass 1: only nodes, no addOutbounds yet
				isValid:       false,              // Will be set in pass 2
				isLocal:       true,
			}
		}
	}

	// Process global selectors
	for _, outboundConfig := range parserConfig.ParserConfig.Outbounds {
		filteredNodes := filterNodesForSelector(allNodes, outboundConfig.Filters)

		// Check for duplicate tags (global selector with same tag as existing local one)
		if existingInfo, exists := outboundsInfo[outboundConfig.Tag]; exists {
			if existingInfo.isLocal {
				log.Printf("GenerateOutboundsFromParserConfig: Warning: Duplicate tag '%s' detected. "+
					"Global selector will overwrite local selector. This may cause unexpected behavior.",
					outboundConfig.Tag)
			} else {
				log.Printf("GenerateOutboundsFromParserConfig: Warning: Duplicate tag '%s' detected. "+
					"Multiple global selectors with same tag. This may cause unexpected behavior.",
					outboundConfig.Tag)
			}
			_ = existingInfo // Suppress unused variable warning
		}

		outboundsInfo[outboundConfig.Tag] = &outboundInfo{
			config:        outboundConfig,
			filteredNodes: filteredNodes,
			outboundCount: len(filteredNodes), // Pass 1: only nodes, no addOutbounds yet
			isValid:       false,              // Will be set in pass 2
			isLocal:       false,
		}
	}

	// Step 4: Pass 2 - Topological sort and count total outboundCount (nodes + valid addOutbounds)
	// This pass uses Kahn's algorithm for topological sorting to ensure dependencies are processed
	// before dependents. This is necessary because the outboundCount of a selector depends on
	// the outboundCount of its addOutbounds (if they are dynamic selectors).
	if progressCallback != nil {
		progressCallback(70, "Calculating outbound dependencies (pass 2)...")
	}

	// Build dependency graph (only dynamic dependencies, not constants)
	// dependents: tag -> list of outbounds that depend on it (reverse dependency graph)
	// inDegree: tag -> number of unprocessed dependencies (incoming edges count)
	dependents := make(map[string][]string, len(outboundsInfo))
	inDegree := make(map[string]int, len(outboundsInfo))

	// Initialize all nodes with zero dependencies
	for tag := range outboundsInfo {
		inDegree[tag] = 0
		dependents[tag] = []string{}
	}

	// Build graph: for each outbound, find its dynamic dependencies
	for tag, info := range outboundsInfo {
		for _, addTag := range info.config.AddOutbounds {
			if _, exists := outboundsInfo[addTag]; exists {
				// Dynamic dependency - add to graph (addTag is a dependency of tag)
				dependents[addTag] = append(dependents[addTag], tag)
				inDegree[tag]++
			}
			// Constants are not in outboundsInfo, so they don't affect the graph
		}
	}

	// Topological sort: process leaves first (outbounds with no dependencies)
	// Pre-allocate queue with estimated size
	queue := make([]string, 0, len(outboundsInfo))
	for tag, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, tag)
		}
	}

	// Process in topological order
	processedCount := 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		info := outboundsInfo[current]

		// Calculate total outboundCount: nodes + valid addOutbounds
		// Start with the number of filtered nodes
		totalCount := len(info.filteredNodes)

		// Add valid addOutbounds to the count
		for _, addTag := range info.config.AddOutbounds {
			if addInfo, exists := outboundsInfo[addTag]; exists {
				// Dynamic outbound - check if it's valid (already calculated due to topological order)
				// Topological sorting guarantees that addInfo.outboundCount is already calculated
				if addInfo.outboundCount > 0 {
					totalCount++ // Add the selector itself as one outbound
				}
				// If outboundCount == 0, skip (empty selector is not added)
			} else {
				// Constant from template (direct-out, auto-proxy-out, etc.)
				// Constants always exist and are always added
				totalCount++
			}
		}

		// Update the outbound info with calculated values
		info.outboundCount = totalCount
		info.isValid = (totalCount > 0)
		processedCount++

		// Update inDegree for dependents
		for _, dependent := range dependents[current] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				// All dependencies processed - can process this one now
				queue = append(queue, dependent)
			}
		}
	}

	// Check: all should be processed (no cycles)
	// If not all outbounds were processed, it indicates a cycle in the dependency graph
	// Cycles are not allowed in the parser configuration
	if processedCount != len(outboundsInfo) {
		unprocessed := make([]string, 0)
		for tag := range outboundsInfo {
			// Check which outbounds still have unprocessed dependencies
			if inDegree[tag] > 0 {
				unprocessed = append(unprocessed, tag)
			}
		}
		log.Printf("GenerateOutboundsFromParserConfig: Warning: Not all outbounds processed (processed: %d, total: %d). "+
			"Possible cycles in dependency graph. Unprocessed outbounds: %v",
			processedCount, len(outboundsInfo), unprocessed)
	}

	// Step 5: Pass 3 - Generate JSON only for valid selectors with filtered addOutbounds
	localSelectorsCount := 0
	globalSelectorsCount := 0

	if progressCallback != nil {
		progressCallback(80, "Generating selectors (pass 3)...")
	}

	// Generate local selectors (per source)
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
			if !exists {
				continue
			}

			// Only generate if valid
			if !info.isValid {
				log.Printf("GenerateOutboundsFromParserConfig: Skipping empty local selector '%s'", outboundConfig.Tag)
				continue
			}

			selectorJSON, err := GenerateSelectorWithFilteredAddOutbounds(sourceNodes, outboundConfig, outboundsInfo)
			if err != nil {
				log.Printf("GenerateOutboundsFromParserConfig: Warning: Failed to generate local selector %s for source %d: %v",
					outboundConfig.Tag, i+1, err)
				continue
			}
			if selectorJSON != "" {
				selectorsJSON = append(selectorsJSON, selectorJSON)
				localSelectorsCount++
			}
		}
	}

	// Generate global selectors
	for _, outboundConfig := range parserConfig.ParserConfig.Outbounds {
		info, exists := outboundsInfo[outboundConfig.Tag]
		if !exists {
			continue
		}

		// Only generate if valid
		if !info.isValid {
			log.Printf("GenerateOutboundsFromParserConfig: Skipping empty global selector '%s'", outboundConfig.Tag)
			continue
		}

		selectorJSON, err := GenerateSelectorWithFilteredAddOutbounds(allNodes, outboundConfig, outboundsInfo)
		if err != nil {
			log.Printf("GenerateOutboundsFromParserConfig: Warning: Failed to generate global selector %s: %v",
				outboundConfig.Tag, err)
			continue
		}
		if selectorJSON != "" {
			selectorsJSON = append(selectorsJSON, selectorJSON)
			globalSelectorsCount++
		}
	}

	return &OutboundGenerationResult{
		OutboundsJSON:        selectorsJSON,
		NodesCount:           nodesCount,
		LocalSelectorsCount:  localSelectorsCount,
		GlobalSelectorsCount: globalSelectorsCount,
	}, nil
}

// Helper functions for filtering

func filterNodesForSelector(allNodes []*ParsedNode, filter interface{}) []*ParsedNode {
	if filter == nil {
		return allNodes // No filter, return all nodes
	}

	// Check if filter is an empty map - treat as no filter
	if filterMap, ok := filter.(map[string]interface{}); ok {
		if len(filterMap) == 0 {
			return allNodes // Empty filter object means no filter, return all nodes
		}
	}

	filtered := make([]*ParsedNode, 0)

	// Check if filter is an array
	if filterArray, ok := filter.([]interface{}); ok {
		// OR between filter objects
		for _, node := range allNodes {
			for _, filterObj := range filterArray {
				if filterMap, ok := filterObj.(map[string]interface{}); ok {
					filterStrMap := convertFilterToStringMap(filterMap)
					if matchesFilter(node, filterStrMap) {
						filtered = append(filtered, node)
						break // Node matched at least one filter, add it
					}
				}
			}
		}
	} else if filterMap, ok := filter.(map[string]interface{}); ok {
		// Single filter object (AND between keys)
		filterStrMap := convertFilterToStringMap(filterMap)
		for _, node := range allNodes {
			if matchesFilter(node, filterStrMap) {
				filtered = append(filtered, node)
			}
		}
	}

	return filtered
}

func convertFilterToStringMap(filter map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range filter {
		if str, ok := v.(string); ok {
			result[k] = str
		}
	}
	return result
}

func matchesFilter(node *ParsedNode, filter map[string]string) bool {
	for key, pattern := range filter {
		value := getNodeValue(node, key)
		if !matchesPattern(value, pattern) {
			return false // At least one key doesn't match
		}
	}
	return true // All keys match
}

func getNodeValue(node *ParsedNode, key string) string {
	switch key {
	case "tag":
		return node.Tag
	case "host":
		return node.Server
	case "label":
		return node.Label
	case "scheme":
		return node.Scheme
	case "fragment":
		return node.Label // fragment == label
	case "comment":
		return node.Comment
	default:
		return ""
	}
}

func matchesPattern(value, pattern string) bool {
	// Negation literal: !literal
	if strings.HasPrefix(pattern, "!") && !strings.HasPrefix(pattern, "!/") {
		literal := strings.TrimPrefix(pattern, "!")
		return value != literal
	}

	// Negation regex: !/regex/i
	if strings.HasPrefix(pattern, "!/") && strings.HasSuffix(pattern, "/i") {
		regexStr := strings.TrimPrefix(pattern, "!/")
		regexStr = strings.TrimSuffix(regexStr, "/i")
		re, err := regexp.Compile("(?i)" + regexStr)
		if err != nil {
			log.Printf("Parser: Invalid regex pattern %s: %v", pattern, err)
			return false
		}
		return !re.MatchString(value)
	}

	// Regex: /regex/i
	if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/i") {
		regexStr := strings.TrimPrefix(pattern, "/")
		regexStr = strings.TrimSuffix(regexStr, "/i")
		re, err := regexp.Compile("(?i)" + regexStr)
		if err != nil {
			log.Printf("Parser: Invalid regex pattern %s: %v", pattern, err)
			return false
		}
		return re.MatchString(value)
	}

	// Literal match
	return value == pattern
}
