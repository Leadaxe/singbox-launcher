package core

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"singbox-launcher/core/parsers"
)

// MaxNodesPerSubscription limits the maximum number of nodes parsed from a single subscription
// This prevents memory issues with very large subscriptions
const MaxNodesPerSubscription = 500

// MakeTagUnique makes a tag unique by appending a number if it already exists in tagCounts.
// Updates tagCounts map and returns the unique tag.
// logPrefix is used for logging (e.g., "Parser" or "ConfigWizard").
func MakeTagUnique(tag string, tagCounts map[string]int, logPrefix string) string {
	if tagCounts[tag] > 0 {
		// Tag already exists, make it unique
		tagCounts[tag]++
		uniqueTag := fmt.Sprintf("%s-%d", tag, tagCounts[tag])
		log.Printf("%s: Duplicate tag '%s' found (occurrence #%d), renamed to '%s'", logPrefix, tag, tagCounts[tag], uniqueTag)
		return uniqueTag
	} else {
		// First occurrence, just mark it
		tagCounts[tag] = 1
		log.Printf("%s: First occurrence of tag '%s'", logPrefix, tag)
		return tag
	}
}

// IsSubscriptionURL checks if the input string is a subscription URL (http:// or https://)
// Exported for use in UI
func IsSubscriptionURL(input string) bool {
	trimmed := strings.TrimSpace(input)
	return strings.HasPrefix(trimmed, "http://") ||
		strings.HasPrefix(trimmed, "https://")
}

// updateParserProgress safely calls UpdateParserProgressFunc if it's not nil
func updateParserProgress(ac *AppController, progress float64, status string) {
	if ac.UpdateParserProgressFunc != nil {
		ac.UpdateParserProgressFunc(progress, status)
	}
}

// LogDuplicateTagStatistics logs statistics about duplicate tags found in tagCounts.
// logPrefix is used for logging (e.g., "Parser" or "ConfigWizard").
func LogDuplicateTagStatistics(tagCounts map[string]int, logPrefix string) {
	duplicateCount := 0
	for tag, count := range tagCounts {
		if count > 1 {
			duplicateCount++
			log.Printf("%s: Tag '%s' had %d occurrences (renamed duplicates)", logPrefix, tag, count)
		}
	}
	if duplicateCount > 0 {
		log.Printf("%s: Found %d tags with duplicates, all have been renamed", logPrefix, duplicateCount)
	} else {
		log.Printf("%s: No duplicate tags found, all tags are unique", logPrefix, duplicateCount)
	}
}

// ProcessProxySource delegates to the internal parser logic
// This method is moved from parser.go to ConfigService to encapsulate logic
func (svc *ConfigService) ProcessProxySource(proxySource ProxySource, tagCounts map[string]int, progressCallback func(float64, string), subscriptionIndex, totalSubscriptions int) ([]*parsers.ParsedNode, error) {
	startTime := time.Now()
	log.Printf("[DEBUG] ProcessProxySource: START source %d/%d at %s",
		subscriptionIndex+1, totalSubscriptions, startTime.Format("15:04:05.000"))

	nodes := make([]*parsers.ParsedNode, 0)
	nodesFromThisSource := 0
	skippedDueToLimit := 0

	// Обрабатываем подписку из поля Source
	if proxySource.Source != "" {
		// Проверяем, не является ли source прямой ссылкой (legacy формат)
		if IsSubscriptionURL(proxySource.Source) {
			// Это подписка - скачиваем и парсим
			if progressCallback != nil {
				progressCallback(20+float64(subscriptionIndex)*50.0/float64(totalSubscriptions),
					fmt.Sprintf("Downloading subscription %d/%d: %s", subscriptionIndex+1, totalSubscriptions, proxySource.Source))
			}

			fetchStartTime := time.Now()
			log.Printf("[DEBUG] ProcessProxySource: Fetching subscription %d/%d: %s",
				subscriptionIndex+1, totalSubscriptions, proxySource.Source)
			content, err := FetchSubscription(proxySource.Source)
			fetchDuration := time.Since(fetchStartTime)
			if err != nil {
				log.Printf("[DEBUG] ProcessProxySource: Failed to fetch subscription %d/%d (took %v): %v",
					subscriptionIndex+1, totalSubscriptions, fetchDuration, err)
				log.Printf("Parser: Error: Failed to fetch subscription from %s: %v", proxySource.Source, err)
			} else if len(content) > 0 {
				log.Printf("[DEBUG] ProcessProxySource: Fetched subscription %d/%d: %d bytes in %v",
					subscriptionIndex+1, totalSubscriptions, len(content), fetchDuration)

				if progressCallback != nil {
					progressCallback(20+float64(subscriptionIndex)*50.0/float64(totalSubscriptions)+10.0/float64(totalSubscriptions),
						fmt.Sprintf("Parsing subscription %d/%d: %s", subscriptionIndex+1, totalSubscriptions, proxySource.Source))
				}

				// Parse subscription content line by line
				parseStartTime := time.Now()
				subscriptionLines := strings.Split(string(content), "\n")
				log.Printf("[DEBUG] ProcessProxySource: Parsing subscription %d/%d: %d lines",
					subscriptionIndex+1, totalSubscriptions, len(subscriptionLines))

				lineCount := 0
				for _, subLine := range subscriptionLines {
					subLine = strings.TrimSpace(subLine)
					if subLine == "" {
						continue
					}
					lineCount++

					if nodesFromThisSource >= MaxNodesPerSubscription {
						skippedDueToLimit++
						if skippedDueToLimit == 1 {
							log.Printf("[DEBUG] ProcessProxySource: Reached limit of %d nodes for subscription %d/%d",
								MaxNodesPerSubscription, subscriptionIndex+1, totalSubscriptions)
						}
						continue
					}

					nodeStartTime := time.Now()
					node, err := parsers.ParseNode(subLine, proxySource.Skip)
					if err != nil {
						log.Printf("[DEBUG] ProcessProxySource: Failed to parse node %d from subscription %d/%d (took %v): %v",
							lineCount, subscriptionIndex+1, totalSubscriptions, time.Since(nodeStartTime), err)
						log.Printf("Parser: Warning: Failed to parse node from subscription %s: %v", proxySource.Source, err)
						continue
					}

					if node != nil {
						node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
						nodes = append(nodes, node)
						nodesFromThisSource++
						if nodesFromThisSource%50 == 0 {
							log.Printf("[DEBUG] ProcessProxySource: Parsed %d nodes from subscription %d/%d (elapsed: %v)",
								nodesFromThisSource, subscriptionIndex+1, totalSubscriptions, time.Since(parseStartTime))
						}
					}
				}
				log.Printf("[DEBUG] ProcessProxySource: Parsed subscription %d/%d: %d nodes in %v (processed %d lines)",
					subscriptionIndex+1, totalSubscriptions, nodesFromThisSource, time.Since(parseStartTime), lineCount)
			}
		} else if parsers.IsDirectLink(proxySource.Source) {
			// Legacy формат: прямая ссылка в Source
			log.Printf("[DEBUG] ProcessProxySource: Processing direct link in Source field for %d/%d",
				subscriptionIndex+1, totalSubscriptions)
			if progressCallback != nil {
				progressCallback(20+float64(subscriptionIndex)*50.0/float64(totalSubscriptions),
					fmt.Sprintf("Parsing direct link %d/%d", subscriptionIndex+1, totalSubscriptions))
			}

			if nodesFromThisSource < MaxNodesPerSubscription {
				parseStartTime := time.Now()
				node, err := parsers.ParseNode(proxySource.Source, proxySource.Skip)
				if err != nil {
					log.Printf("[DEBUG] ProcessProxySource: Failed to parse direct link (took %v): %v",
						time.Since(parseStartTime), err)
					log.Printf("Parser: Warning: Failed to parse direct link: %v", err)
				} else if node != nil {
					node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
					nodes = append(nodes, node)
					nodesFromThisSource++
					log.Printf("[DEBUG] ProcessProxySource: Parsed direct link in %v", time.Since(parseStartTime))
				}
			} else {
				skippedDueToLimit++
			}
		}
	}

	// Обрабатываем прямые ссылки из поля Connections
	connectionsStartTime := time.Now()
	log.Printf("[DEBUG] ProcessProxySource: Processing %d direct connections for source %d/%d",
		len(proxySource.Connections), subscriptionIndex+1, totalSubscriptions)
	for connIndex, connection := range proxySource.Connections {
		connection = strings.TrimSpace(connection)
		if connection == "" {
			continue
		}

		if !parsers.IsDirectLink(connection) {
			log.Printf("[DEBUG] ProcessProxySource: Invalid direct link format in connections %d/%d: %s",
				connIndex+1, len(proxySource.Connections), connection)
			log.Printf("Parser: Warning: Invalid direct link format in connections: %s", connection)
			continue
		}

		if progressCallback != nil {
			progressCallback(20+float64(subscriptionIndex)*50.0/float64(totalSubscriptions),
				fmt.Sprintf("Parsing direct link %d/%d (connection %d)", subscriptionIndex+1, totalSubscriptions, connIndex+1))
		}

		if nodesFromThisSource >= MaxNodesPerSubscription {
			skippedDueToLimit++
			continue
		}

		parseStartTime := time.Now()
		node, err := parsers.ParseNode(connection, proxySource.Skip)
		if err != nil {
			log.Printf("[DEBUG] ProcessProxySource: Failed to parse connection %d/%d (took %v): %v",
				connIndex+1, len(proxySource.Connections), time.Since(parseStartTime), err)
			log.Printf("Parser: Warning: Failed to parse direct link from connections: %v", err)
			continue
		}

		if node != nil {
			node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
			nodes = append(nodes, node)
			nodesFromThisSource++
		}
	}
	if len(proxySource.Connections) > 0 {
		log.Printf("[DEBUG] ProcessProxySource: Processed %d connections in %v",
			len(proxySource.Connections), time.Since(connectionsStartTime))
	}

	if skippedDueToLimit > 0 {
		log.Printf("[DEBUG] ProcessProxySource: Source %d/%d exceeded limit, skipped %d nodes",
			subscriptionIndex+1, totalSubscriptions, skippedDueToLimit)
		log.Printf("Parser: Warning: Source exceeded limit of %d nodes. Skipped %d additional nodes.",
			MaxNodesPerSubscription, skippedDueToLimit)
	}

	totalDuration := time.Since(startTime)
	log.Printf("[DEBUG] ProcessProxySource: END source %d/%d (total duration: %v, nodes: %d)",
		subscriptionIndex+1, totalSubscriptions, totalDuration, len(nodes))
	return nodes, nil
}

// GenerateSelector generates JSON string for a selector from filtered nodes.
// Filters nodes based on outboundConfig.Outbounds.Proxies, adds addOutbounds,
// determines default outbound from preferredDefault if specified, and builds
// the selector JSON with correct field order.
func (svc *ConfigService) GenerateSelector(allNodes []*parsers.ParsedNode, outboundConfig OutboundConfig) (string, error) {
	// Filter nodes based on outbounds.proxies
	filteredNodes := filterNodesForSelector(allNodes, outboundConfig.Outbounds.Proxies)

	if len(filteredNodes) == 0 {
		log.Printf("Parser: No nodes matched filter for selector %s", outboundConfig.Tag)
		return "", nil
	}

	// Build outbounds list with unique tags
	outboundsList := make([]string, 0)
	seenTags := make(map[string]bool)
	duplicateCountInSelector := 0

	// Add addOutbounds first
	if len(outboundConfig.Outbounds.AddOutbounds) > 0 {
		log.Printf("Parser: Adding %d addOutbounds to selector '%s'", len(outboundConfig.Outbounds.AddOutbounds), outboundConfig.Tag)
		for _, tag := range outboundConfig.Outbounds.AddOutbounds {
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

	if duplicateCountInSelector > 0 {
		log.Printf("Parser: Removed %d duplicate tags from selector '%s' outbounds list", duplicateCountInSelector, outboundConfig.Tag)
	}
	log.Printf("Parser: Selector '%s' will have %d unique outbounds", outboundConfig.Tag, len(outboundsList))

	// Determine default - only if preferredDefault is specified in config
	defaultTag := ""
	if len(outboundConfig.Outbounds.PreferredDefault) > 0 {
		// Find first node matching preferredDefault filter
		preferredFilter := convertFilterToStringMap(outboundConfig.Outbounds.PreferredDefault)
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

// GenerateNodeJSON generates JSON string for a parsed node with correct field order.
// Handles all proxy types (vless, vmess, trojan, shadowsocks) and includes
// TLS configuration, transport settings, and other protocol-specific fields.
func (svc *ConfigService) GenerateNodeJSON(node *parsers.ParsedNode) (string, error) {
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
	} else if node.Scheme == "ss" {
		// Extract method and password from outbound
		if method, ok := node.Outbound["method"].(string); ok && method != "" {
			parts = append(parts, fmt.Sprintf(`"method":%q`, method))
		}
		if password, ok := node.Outbound["password"].(string); ok && password != "" {
			parts = append(parts, fmt.Sprintf(`"password":%q`, password))
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

		// alpn (for VMESS)
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

		// insecure (for VMESS)
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

// Private helper functions for GenerateSelector

func filterNodesForSelector(allNodes []*parsers.ParsedNode, filter interface{}) []*parsers.ParsedNode {
	if filter == nil {
		return allNodes // No filter, return all nodes
	}

	filtered := make([]*parsers.ParsedNode, 0)

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

// matchesFilter checks if node matches filter (AND between keys)
func matchesFilter(node *parsers.ParsedNode, filter map[string]string) bool {
	for key, pattern := range filter {
		value := getNodeValue(node, key)
		if !matchesPattern(value, pattern) {
			return false // At least one key doesn't match
		}
	}
	return true // All keys match
}

// Helpers needed from parser.go private scope, duplicated here because they are small utilities
// Ideally these would be in a shared internal package

func getNodeValue(node *parsers.ParsedNode, key string) string {
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

// UpdateConfigFromSubscriptions updates config.json by fetching subscriptions and parsing nodes.
// This is the main entry point for configuration updates.
// It extracts parser configuration, processes all proxy sources, generates outbound JSON,
// and writes the result to config.json between @ParserSTART and @ParserEND markers.
func (svc *ConfigService) UpdateConfigFromSubscriptions() error {
	ac := svc.ac
	log.Println("Parser: Starting configuration update...")

	// Step 1: Extract configuration
	config, err := ExtractParserConfig(ac.ConfigPath)
	if err != nil {
		updateParserProgress(ac, -1, fmt.Sprintf("Error: %v", err))
		return fmt.Errorf("failed to extract parser config: %w", err)
	}

	// Update progress: Step 1 completed
	updateParserProgress(ac, 5, "Parsed ParserConfig block")

	// Wait 0.1 sec before showing connection message
	time.Sleep(100 * time.Millisecond)

	// Show connection message
	updateParserProgress(ac, 5, "Connecting...")

	// Small delay before starting to fetch subscriptions
	time.Sleep(100 * time.Millisecond)

	// Step 2: Load and parse subscriptions
	allNodes := make([]*parsers.ParsedNode, 0)
	successfulSubscriptions := 0
	totalSubscriptions := len(config.ParserConfig.Proxies)

	// Map to track unique tags and their counts
	tagCounts := make(map[string]int)
	log.Printf("Parser: Initializing tag deduplication tracker")

	updateParserProgress(ac, 20, fmt.Sprintf("Loading subscriptions (0/%d)...", totalSubscriptions))

	for i, proxySource := range config.ParserConfig.Proxies {
		progressCallback := func(p float64, s string) {
			updateParserProgress(ac, p, s)
		}

		nodesFromThisSource, err := svc.ProcessProxySource(proxySource, tagCounts, progressCallback, i, totalSubscriptions)
		if err != nil {
			log.Printf("Parser: Error processing source %d/%d: %v", i+1, totalSubscriptions, err)
			continue
		}

		if len(nodesFromThisSource) > 0 {
			allNodes = append(allNodes, nodesFromThisSource...)
			successfulSubscriptions++
			log.Printf("Parser: Successfully parsed %d nodes from source %d/%d: %s", len(nodesFromThisSource), i+1, totalSubscriptions, proxySource.Source)
		} else {
			log.Printf("Parser: Warning: No valid nodes parsed from source %d/%d: %s", i+1, totalSubscriptions, proxySource.Source)
		}

		// Update progress after parsing source
		progress := 20 + float64(i+1)*50.0/float64(totalSubscriptions)
		updateParserProgress(ac, progress, fmt.Sprintf("Processed sources: %d/%d, nodes: %d", i+1, totalSubscriptions, len(allNodes)))
	}

	// Check if we successfully loaded at least one subscription
	if successfulSubscriptions == 0 {
		updateParserProgress(ac, -1, "Error: failed to load any subscriptions")
		return fmt.Errorf("failed to load any subscriptions - check internet connection and subscription URLs")
	}

	log.Printf("Parser: Parsed %d nodes from subscriptions", len(allNodes))

	// Log statistics about duplicates
	LogDuplicateTagStatistics(tagCounts, "Parser")

	updateParserProgress(ac, 70, fmt.Sprintf("Processed nodes: %d. Generating JSON...", len(allNodes)))

	// Check if we have any nodes before proceeding
	if len(allNodes) == 0 {
		updateParserProgress(ac, -1, "Error: no nodes found in subscriptions")
		return fmt.Errorf("no nodes parsed from subscriptions - check internet connection and subscription URLs")
	}

	// Step 3: Generate selectors
	updateParserProgress(ac, 75, "Generating JSON for nodes...")

	selectorsJSON := make([]string, 0)

	// First, generate JSON for all nodes
	for _, node := range allNodes {
		nodeJSON, err := svc.GenerateNodeJSON(node)
		if err != nil {
			log.Printf("Parser: Warning: Failed to generate JSON for node %s: %v", node.Tag, err)
			continue
		}
		selectorsJSON = append(selectorsJSON, nodeJSON)
	}

	// Check if we have any node JSON before generating selectors
	if len(selectorsJSON) == 0 {
		updateParserProgress(ac, -1, "Error: failed to generate JSON for nodes")
		return fmt.Errorf("failed to generate JSON for any nodes")
	}

	// Then, generate selectors
	updateParserProgress(ac, 85, "Generating selectors...")

	for _, outboundConfig := range config.ParserConfig.Outbounds {
		selectorJSON, err := svc.GenerateSelector(allNodes, outboundConfig)
		if err != nil {
			log.Printf("Parser: Warning: Failed to generate selector %s: %v", outboundConfig.Tag, err)
			continue
		}
		if selectorJSON != "" {
			selectorsJSON = append(selectorsJSON, selectorJSON)
		}
	}

	// Final check: ensure we have content to write
	if len(selectorsJSON) == 0 {
		updateParserProgress(ac, -1, "Error: nothing to write to configuration")
		return fmt.Errorf("no content generated - cannot write empty result to config")
	}

	// Step 4: Write to file
	updateParserProgress(ac, 90, "Writing to config file...")

	content := strings.Join(selectorsJSON, "\n")
	if err := writeToConfig(ac.ConfigPath, content); err != nil {
		updateParserProgress(ac, -1, fmt.Sprintf("Write error: %v", err))
		return fmt.Errorf("failed to write to config: %w", err)
	}

	log.Printf("Parser: Done! File %s successfully updated.", ac.ConfigPath)

	// Update last_updated timestamp in @ParserConfig block
	if err := UpdateLastUpdatedInConfig(ac.ConfigPath, time.Now().UTC()); err != nil {
		log.Printf("Parser: Warning: Failed to update last_updated timestamp: %v", err)
		// Don't fail the whole operation if timestamp update fails
	} else {
		log.Printf("Parser: Successfully updated last_updated timestamp")
	}

	updateParserProgress(ac, 100, "Configuration updated successfully!")

	return nil
}

// writeToConfig writes content between @ParserSTART and @ParserEND markers
func writeToConfig(configPath string, content string) error {
	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	configStr := string(data)

	// Find markers
	startMarker := "/** @ParserSTART */"
	endMarker := "/** @ParserEND */"

	startIdx := strings.Index(configStr, startMarker)
	endIdx := strings.Index(configStr, endMarker)

	if startIdx == -1 || endIdx == -1 {
		return fmt.Errorf("markers @ParserSTART or @ParserEND not found in config.json")
	}

	if endIdx <= startIdx {
		return fmt.Errorf("invalid marker positions")
	}

	// Build new content
	newContent := configStr[:startIdx+len(startMarker)] + "\n" + content + "\n" + configStr[endIdx:]

	// Write to file
	if err := os.WriteFile(configPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
