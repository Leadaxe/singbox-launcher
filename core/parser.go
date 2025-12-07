package core

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MaxNodesPerSubscription limits the maximum number of nodes parsed from a single subscription
// This prevents memory issues with very large subscriptions
const MaxNodesPerSubscription = 500

// ParsedNode represents a parsed proxy node
type ParsedNode struct {
	Tag      string
	Scheme   string
	Server   string
	Port     int
	UUID     string
	Flow     string
	Label    string
	Comment  string
	Query    url.Values
	Outbound map[string]interface{}
}

// updateParserProgress safely calls UpdateParserProgressFunc if it's not nil
func updateParserProgress(ac *AppController, progress float64, status string) {
	if ac.UpdateParserProgressFunc != nil {
		ac.UpdateParserProgressFunc(progress, status)
	}
}

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

// IsDirectLink checks if the input string is a direct proxy link (vless://, vmess://, etc.)
// Exported for use in UI
func IsDirectLink(input string) bool {
	trimmed := strings.TrimSpace(input)
	return strings.HasPrefix(trimmed, "vless://") ||
		strings.HasPrefix(trimmed, "vmess://") ||
		strings.HasPrefix(trimmed, "trojan://") ||
		strings.HasPrefix(trimmed, "ss://")
}

// IsSubscriptionURL checks if the input string is a subscription URL (http:// or https://)
// Exported for use in UI
func IsSubscriptionURL(input string) bool {
	trimmed := strings.TrimSpace(input)
	return strings.HasPrefix(trimmed, "http://") ||
		strings.HasPrefix(trimmed, "https://")
}

// ProcessProxySource processes a single ProxySource:
// - Processes subscription URL from Source field (http/https)
// - Processes direct links from Connections field (vless://, etc.)
// - Also handles legacy format where direct links are in Source (backward compatibility)
// Exported for use in UI
func ProcessProxySource(proxySource ProxySource, tagCounts map[string]int, progressCallback func(float64, string), subscriptionIndex, totalSubscriptions int) ([]*ParsedNode, error) {
	nodes := make([]*ParsedNode, 0)
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

			content, err := FetchSubscription(proxySource.Source)
			if err != nil {
				log.Printf("Parser: Error: Failed to fetch subscription from %s: %v", proxySource.Source, err)
			} else if len(content) > 0 {
				if progressCallback != nil {
					progressCallback(20+float64(subscriptionIndex)*50.0/float64(totalSubscriptions)+10.0/float64(totalSubscriptions),
						fmt.Sprintf("Parsing subscription %d/%d: %s", subscriptionIndex+1, totalSubscriptions, proxySource.Source))
				}

				// Parse subscription content line by line
				subscriptionLines := strings.Split(string(content), "\n")
				for _, subLine := range subscriptionLines {
					subLine = strings.TrimSpace(subLine)
					if subLine == "" {
						continue
					}

					if nodesFromThisSource >= MaxNodesPerSubscription {
						skippedDueToLimit++
						continue
					}

					node, err := ParseNode(subLine, proxySource.Skip)
					if err != nil {
						log.Printf("Parser: Warning: Failed to parse node from subscription %s: %v", proxySource.Source, err)
						continue
					}

					if node != nil {
						node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
						nodes = append(nodes, node)
						nodesFromThisSource++
					}
				}
			}
		} else if IsDirectLink(proxySource.Source) {
			// Legacy формат: прямая ссылка в Source
			if progressCallback != nil {
				progressCallback(20+float64(subscriptionIndex)*50.0/float64(totalSubscriptions),
					fmt.Sprintf("Parsing direct link %d/%d", subscriptionIndex+1, totalSubscriptions))
			}

			if nodesFromThisSource < MaxNodesPerSubscription {
				node, err := ParseNode(proxySource.Source, proxySource.Skip)
				if err != nil {
					log.Printf("Parser: Warning: Failed to parse direct link: %v", err)
				} else if node != nil {
					node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
					nodes = append(nodes, node)
					nodesFromThisSource++
				}
			} else {
				skippedDueToLimit++
			}
		}
	}

	// Обрабатываем прямые ссылки из поля Connections
	for connIndex, connection := range proxySource.Connections {
		connection = strings.TrimSpace(connection)
		if connection == "" {
			continue
		}

		if !IsDirectLink(connection) {
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

		node, err := ParseNode(connection, proxySource.Skip)
		if err != nil {
			log.Printf("Parser: Warning: Failed to parse direct link from connections: %v", err)
			continue
		}

		if node != nil {
			node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
			nodes = append(nodes, node)
			nodesFromThisSource++
		}
	}

	if skippedDueToLimit > 0 {
		log.Printf("Parser: Warning: Source exceeded limit of %d nodes. Skipped %d additional nodes.",
			MaxNodesPerSubscription, skippedDueToLimit)
	}

	return nodes, nil
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
		log.Printf("%s: No duplicate tags found, all tags are unique", logPrefix)
	}
}

// UpdateConfigFromSubscriptions updates config.json by fetching subscriptions and parsing nodes
func UpdateConfigFromSubscriptions(ac *AppController) error {
	log.Println("Parser: Starting configuration update...")

	// Step 1: Extract configuration
	config, err := ExtractParcerConfig(ac.ConfigPath)
	if err != nil {
		updateParserProgress(ac, -1, fmt.Sprintf("Error: %v", err))
		return fmt.Errorf("failed to extract parser config: %w", err)
	}

	// Update progress: Step 1 completed
	updateParserProgress(ac, 5, "Parsed ParcerConfig block")

	// Wait 0.1 sec before showing connection message
	time.Sleep(100 * time.Millisecond)

	// Show connection message
	updateParserProgress(ac, 5, "Connecting...")

	// Small delay before starting to fetch subscriptions
	time.Sleep(100 * time.Millisecond)

	// Step 2: Load and parse subscriptions
	allNodes := make([]*ParsedNode, 0)
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

		nodesFromThisSource, err := ProcessProxySource(proxySource, tagCounts, progressCallback, i, totalSubscriptions)
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
		nodeJSON, err := GenerateNodeJSON(node)
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
		selectorJSON, err := GenerateSelector(allNodes, outboundConfig)
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

	// Update last_updated timestamp in @ParcerConfig block
	if err := UpdateLastUpdatedInConfig(ac.ConfigPath, time.Now().UTC()); err != nil {
		log.Printf("Parser: Warning: Failed to update last_updated timestamp: %v", err)
		// Don't fail the whole operation if timestamp update fails
	} else {
		log.Printf("Parser: Successfully updated last_updated timestamp")
	}

	updateParserProgress(ac, 100, "Configuration updated successfully!")

	return nil
}

// ParseNode parses a single node URI and applies skip filters (exported for use in UI)
func ParseNode(uri string, skipFilters []map[string]string) (*ParsedNode, error) {
	// Determine scheme
	scheme := ""
	uriToParse := uri

	// Handle VMess base64 format
	if strings.HasPrefix(uri, "vmess://") {
		scheme = "vmess"
		// VMess might be in base64 format, decode if needed
		base64Part := strings.TrimPrefix(uri, "vmess://")
		decoded, err := DecodeSubscriptionContent([]byte(base64Part))
		if err == nil && len(decoded) > 0 {
			// Try to parse as JSON VMess config
			var vmessConfig map[string]interface{}
			if err := json.Unmarshal(decoded, &vmessConfig); err == nil {
				// Convert VMess JSON to URI format (simplified)
				// For now, we'll handle it as a special case
				return parseVMessJSON(vmessConfig, skipFilters)
			}
		}
	} else if strings.HasPrefix(uri, "vless://") {
		scheme = "vless"
	} else if strings.HasPrefix(uri, "trojan://") {
		scheme = "trojan"
	} else if strings.HasPrefix(uri, "ss://") {
		scheme = "ss"
	} else {
		return nil, fmt.Errorf("unsupported scheme")
	}

	// Parse URI
	parsedURL, err := url.Parse(uriToParse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URI: %w", err)
	}

	// Extract components
	node := &ParsedNode{
		Scheme: scheme,
		Server: parsedURL.Hostname(),
		Query:  parsedURL.Query(),
	}

	// Extract port
	port := parsedURL.Port()
	if port == "" {
		// Default ports
		switch scheme {
		case "vless", "vmess":
			node.Port = 443
		case "trojan":
			node.Port = 443
		case "ss":
			node.Port = 443
		}
	} else {
		if p, err := strconv.Atoi(port); err == nil {
			node.Port = p
		} else {
			node.Port = 443 // Fallback
		}
	}

	// Extract UUID/user
	if parsedURL.User != nil {
		node.UUID = parsedURL.User.Username()
	}

	// Extract fragment (label)
	node.Label = parsedURL.Fragment
	// URL decode the fragment if needed
	if node.Label != "" {
		if decoded, err := url.QueryUnescape(node.Label); err == nil {
			node.Label = decoded
		}
	}

	// For some formats, label might be in path or userinfo
	if node.Label == "" {
		// Try to extract from path (some formats use path for label)
		if parsedURL.Path != "" && parsedURL.Path != "/" {
			node.Label = strings.TrimPrefix(parsedURL.Path, "/")
		} else if parsedURL.User != nil {
			// Some formats encode label in username
			node.Label = parsedURL.User.Username()
		}
	}

	// Extract tag and comment from label
	node.Tag, node.Comment = extractTagAndComment(node.Label)

	// Normalize flag
	node.Tag = normalizeFlagTag(node.Tag)

	// Extract flow
	node.Flow = parsedURL.Query().Get("flow")

	// Apply skip filters
	if shouldSkipNode(node, skipFilters) {
		return nil, nil // Node should be skipped
	}

	// Build outbound JSON based on scheme
	node.Outbound = buildOutbound(node)

	return node, nil
}

// extractTagAndComment extracts tag and comment from label
func extractTagAndComment(label string) (tag, comment string) {
	parts := strings.Split(label, "|")
	if len(parts) > 1 {
		tag = parts[0]
		comment = label // Full label as comment
	} else {
		tag = label
		comment = label
	}
	return strings.TrimSpace(tag), strings.TrimSpace(comment)
}

// normalizeFlagTag normalizes flag tags (e.g., 🇪🇳 -> 🇬🇧)
func normalizeFlagTag(tag string) string {
	return strings.ReplaceAll(tag, "🇪🇳", "🇬🇧")
}

// shouldSkipNode checks if node should be skipped based on filters
func shouldSkipNode(node *ParsedNode, skipFilters []map[string]string) bool {
	// OR between filters: if at least one matches, skip
	for _, filter := range skipFilters {
		// AND inside filter: all keys must match
		allKeysMatch := true
		for key, pattern := range filter {
			value := getNodeValue(node, key)
			if !matchesPattern(value, pattern) {
				allKeysMatch = false
				break
			}
		}
		if allKeysMatch {
			return true // Skip node
		}
	}
	return false // Don't skip
}

// getNodeValue extracts value from node by key
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

// matchesPattern checks if value matches pattern
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

// buildOutbound builds outbound JSON structure for node
func buildOutbound(node *ParsedNode) map[string]interface{} {
	outbound := make(map[string]interface{})
	outbound["tag"] = node.Tag
	outbound["type"] = node.Scheme
	outbound["server"] = node.Server
	outbound["server_port"] = node.Port

	if node.Scheme == "vless" {
		outbound["uuid"] = node.UUID
		if node.Flow != "" {
			outbound["flow"] = node.Flow
		}

		// Build TLS structure with correct field order
		// Store TLS data for ordered serialization
		sni := node.Query.Get("sni")
		if sni == "" {
			sni = node.Server // Fallback to server hostname
		}
		fp := node.Query.Get("fp")
		if fp == "" {
			fp = "random"
		}
		pbk := node.Query.Get("pbk")
		sid := node.Query.Get("sid")

		// Store TLS components for ordered serialization
		tlsData := map[string]interface{}{
			"enabled":     true,
			"server_name": sni,
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": fp,
			},
		}

		// Only add Reality section if public_key is not empty
		// Sing-box requires valid public_key when Reality is enabled
		if pbk != "" {
			tlsData["reality"] = map[string]interface{}{
				"enabled":    true,
				"public_key": pbk,
				"short_id":   sid,
			}
		}

		outbound["tls"] = tlsData
	} else if node.Scheme == "vmess" {
		outbound["uuid"] = node.UUID
		// Add VMess-specific fields if needed
	} else if node.Scheme == "trojan" {
		outbound["password"] = node.UUID
		// Add Trojan-specific fields if needed
	} else if node.Scheme == "ss" {
		// Add Shadowsocks-specific fields if needed
	}

	return outbound
}

// GenerateNodeJSON generates JSON string for a node with correct field order (exported for use in UI)
func GenerateNodeJSON(node *ParsedNode) (string, error) {
	// Build JSON with correct field order
	var parts []string

	// 1. tag
	parts = append(parts, fmt.Sprintf(`"tag":%q`, node.Tag))

	// 2. type
	parts = append(parts, fmt.Sprintf(`"type":%q`, node.Scheme))

	// 3. server
	parts = append(parts, fmt.Sprintf(`"server":%q`, node.Server))

	// 4. server_port
	parts = append(parts, fmt.Sprintf(`"server_port":%d`, node.Port))

	// 5. uuid (for vless/vmess) or password (for trojan)
	if node.Scheme == "vless" || node.Scheme == "vmess" {
		parts = append(parts, fmt.Sprintf(`"uuid":%q`, node.UUID))
	} else if node.Scheme == "trojan" {
		parts = append(parts, fmt.Sprintf(`"password":%q`, node.UUID))
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

// GenerateSelector generates JSON string for a selector (exported for use in UI)
func GenerateSelector(allNodes []*ParsedNode, outboundConfig OutboundConfig) (string, error) {
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

// filterNodesForSelector filters nodes based on outbounds.proxies filter
// Filter can be a single object (AND between keys) or array of objects (OR between objects, AND inside)
func filterNodesForSelector(allNodes []*ParsedNode, filter interface{}) []*ParsedNode {
	if filter == nil {
		return allNodes // No filter, return all nodes
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

// parseVMessJSON parses VMess JSON configuration
func parseVMessJSON(vmessConfig map[string]interface{}, skipFilters []map[string]string) (*ParsedNode, error) {
	node := &ParsedNode{
		Scheme: "vmess",
		Query:  make(url.Values),
	}

	// Extract common fields
	if add, ok := vmessConfig["add"].(string); ok {
		node.Server = add
	}
	if port, ok := vmessConfig["port"].(float64); ok {
		node.Port = int(port)
	} else {
		node.Port = 443
	}
	if id, ok := vmessConfig["id"].(string); ok {
		node.UUID = id
	}
	if ps, ok := vmessConfig["ps"].(string); ok {
		node.Label = ps
		node.Tag, node.Comment = extractTagAndComment(ps)
		node.Tag = normalizeFlagTag(node.Tag)
	}

	// Extract TLS settings
	if tls, ok := vmessConfig["tls"].(string); ok && tls == "tls" {
		if sni, ok := vmessConfig["sni"].(string); ok {
			node.Query.Set("sni", sni)
		}
	}

	// Apply skip filters
	if shouldSkipNode(node, skipFilters) {
		return nil, nil
	}

	// Build outbound
	node.Outbound = buildOutbound(node)
	return node, nil
}

// convertFilterToStringMap converts filter map to string map
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
func matchesFilter(node *ParsedNode, filter map[string]string) bool {
	for key, pattern := range filter {
		value := getNodeValue(node, key)
		if !matchesPattern(value, pattern) {
			return false // At least one key doesn't match
		}
	}
	return true // All keys match
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
