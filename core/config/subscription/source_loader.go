package subscription

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/debuglog"
)

// IsSubscriptionURL checks if the input string is a subscription URL (http:// or https://)
func IsSubscriptionURL(input string) bool {
	trimmed := strings.TrimSpace(input)
	return strings.HasPrefix(trimmed, "http://") ||
		strings.HasPrefix(trimmed, "https://")
}

// MakeTagUnique makes a tag unique by appending a number if it already exists in tagCounts.
// Updates tagCounts map and returns the unique tag.
// logPrefix is used for logging (e.g., "Parser" or "ConfigWizard").
func MakeTagUnique(tag string, tagCounts map[string]int, logPrefix string) string {
	if tagCounts[tag] > 0 {
		// Tag already exists, make it unique
		tagCounts[tag]++
		uniqueTag := fmt.Sprintf("%s-%d", tag, tagCounts[tag])
		debuglog.WarnLog("%s: Duplicate tag '%s' found (occurrence #%d), renamed to '%s'", logPrefix, tag, tagCounts[tag], uniqueTag)
		return uniqueTag
	}

	// First occurrence of this tag
	tagCounts[tag] = 1
	return tag
}

// LogDuplicateTagStatistics logs statistics about duplicate tags found during processing
func LogDuplicateTagStatistics(tagCounts map[string]int, logPrefix string) {
	duplicatesFound := false
	for tag, count := range tagCounts {
		if count > 1 {
			if !duplicatesFound {
				debuglog.DebugLog("%s: === Duplicate Tag Statistics ===", logPrefix)
				duplicatesFound = true
			}
			debuglog.WarnLog("%s: Tag '%s' appeared %d times (original + %d duplicates)", logPrefix, tag, count, count-1)
		}
	}
	if duplicatesFound {
		debuglog.DebugLog("%s: === End of Duplicate Tag Statistics ===", logPrefix)
	}
}

// LoadNodesFromSource loads and processes nodes from a config.ProxySource
// Handles subscriptions, legacy direct links, and connections
// Returns list of parsed nodes with processed tags
func LoadNodesFromSource(
	proxySource config.ProxySource,
	tagCounts map[string]int,
	progressCallback func(float64, string),
	subscriptionIndex, totalSubscriptions int,
) ([]*config.ParsedNode, error) {
	startTime := time.Now()
	debuglog.DebugLog("LoadNodesFromSource: START source %d/%d at %s",
		subscriptionIndex+1, totalSubscriptions, startTime.Format("15:04:05.000"))

	nodes := make([]*config.ParsedNode, 0)
	nodesFromThisSource := 0
	skippedDueToLimit := 0

	// Process subscription from Source field
	if proxySource.Source != "" {
		// Check if source is a direct link (legacy format)
		if IsSubscriptionURL(proxySource.Source) {
			// This is a subscription - download and parse
			if progressCallback != nil {
				progressCallback(20+float64(subscriptionIndex)*50.0/float64(totalSubscriptions),
					fmt.Sprintf("Downloading subscription %d/%d: %s", subscriptionIndex+1, totalSubscriptions, proxySource.Source))
			}

			fetchStartTime := time.Now()
			debuglog.DebugLog("LoadNodesFromSource: Fetching subscription %d/%d: %s",
				subscriptionIndex+1, totalSubscriptions, proxySource.Source)
			content, err := FetchSubscription(proxySource.Source)
			fetchDuration := time.Since(fetchStartTime)
			if err != nil {
				debuglog.DebugLog("LoadNodesFromSource: Failed to fetch subscription %d/%d (took %v): %v",
					subscriptionIndex+1, totalSubscriptions, fetchDuration, err)
				debuglog.ErrorLog("Parser: Failed to fetch subscription from %s: %v", proxySource.Source, err)
			} else if len(content) > 0 {
				debuglog.DebugLog("LoadNodesFromSource: Fetched subscription %d/%d: %d bytes in %v",
					subscriptionIndex+1, totalSubscriptions, len(content), fetchDuration)

				if progressCallback != nil {
					progressCallback(20+float64(subscriptionIndex)*50.0/float64(totalSubscriptions)+10.0/float64(totalSubscriptions),
						fmt.Sprintf("Parsing subscription %d/%d: %s", subscriptionIndex+1, totalSubscriptions, proxySource.Source))
				}

				// Parse subscription content line by line
				parseStartTime := time.Now()
				// Normalize line endings (handle \r\n, \r, \n)
				contentStr := string(content)
				contentStr = strings.ReplaceAll(contentStr, "\r\n", "\n")
				contentStr = strings.ReplaceAll(contentStr, "\r", "\n")
				subscriptionLines := strings.Split(contentStr, "\n")
				debuglog.DebugLog("LoadNodesFromSource: Parsing subscription %d/%d: %d lines",
					subscriptionIndex+1, totalSubscriptions, len(subscriptionLines))

				lineCount := 0
				for _, subLine := range subscriptionLines {
					subLine = strings.TrimSpace(subLine)
					if subLine == "" {
						continue
					}
					lineCount++

					if nodesFromThisSource >= config.MaxNodesPerSubscription {
						skippedDueToLimit++
						if skippedDueToLimit == 1 {
						debuglog.DebugLog("LoadNodesFromSource: Reached limit of %d nodes for subscription %d/%d",
							config.MaxNodesPerSubscription, subscriptionIndex+1, totalSubscriptions)
						}
						continue
					}

					nodeStartTime := time.Now()
					node, err := ParseNode(subLine, proxySource.Skip)
					if err != nil {
					debuglog.DebugLog("LoadNodesFromSource: Failed to parse node %d from subscription %d/%d (took %v): %v",
						lineCount, subscriptionIndex+1, totalSubscriptions, time.Since(nodeStartTime), err)
					debuglog.WarnLog("Parser: Failed to parse node from subscription %s: %v", proxySource.Source, err)
						continue
					}

					if node != nil {
						// Apply prefix, postfix, or mask to tag if specified (with variable substitution)
						node.Tag = applyTagPrefixPostfix(node, proxySource.TagPrefix, proxySource.TagPostfix, proxySource.TagMask, nodesFromThisSource+1)
						node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
						nodes = append(nodes, node)
						nodesFromThisSource++
						if nodesFromThisSource%50 == 0 {
						debuglog.DebugLog("LoadNodesFromSource: Parsed %d nodes from subscription %d/%d (elapsed: %v)",
							nodesFromThisSource, subscriptionIndex+1, totalSubscriptions, time.Since(parseStartTime))
						}
					}
				}
			debuglog.DebugLog("LoadNodesFromSource: Parsed subscription %d/%d: %d nodes in %v (processed %d lines)",
				subscriptionIndex+1, totalSubscriptions, nodesFromThisSource, time.Since(parseStartTime), lineCount)
			}
		} else if IsDirectLink(proxySource.Source) {
			// Legacy format: direct link in Source
		debuglog.DebugLog("LoadNodesFromSource: Processing direct link in Source field for %d/%d",
			subscriptionIndex+1, totalSubscriptions)
			if progressCallback != nil {
				progressCallback(20+float64(subscriptionIndex)*50.0/float64(totalSubscriptions),
					fmt.Sprintf("Parsing direct link %d/%d", subscriptionIndex+1, totalSubscriptions))
			}

			if nodesFromThisSource < config.MaxNodesPerSubscription {
				parseStartTime := time.Now()
				node, err := ParseNode(proxySource.Source, proxySource.Skip)
				if err != nil {
				debuglog.DebugLog("LoadNodesFromSource: Failed to parse direct link (took %v): %v",
					time.Since(parseStartTime), err)
				debuglog.WarnLog("Parser: Failed to parse direct link: %v", err)
				} else if node != nil {
					// Apply prefix, postfix, or mask to tag if specified (with variable substitution)
					node.Tag = applyTagPrefixPostfix(node, proxySource.TagPrefix, proxySource.TagPostfix, proxySource.TagMask, nodesFromThisSource+1)
					node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
					nodes = append(nodes, node)
					nodesFromThisSource++
					debuglog.DebugLog("LoadNodesFromSource: Parsed direct link in %v", time.Since(parseStartTime))
				}
			} else {
				skippedDueToLimit++
			}
		}
	}

	// Process direct links from Connections field
	connectionsStartTime := time.Now()
	debuglog.DebugLog("LoadNodesFromSource: Processing %d direct connections for source %d/%d",
		len(proxySource.Connections), subscriptionIndex+1, totalSubscriptions)
	for connIndex, connection := range proxySource.Connections {
		connection = strings.TrimSpace(connection)
		if connection == "" {
			continue
		}

		if !IsDirectLink(connection) {
		debuglog.DebugLog("LoadNodesFromSource: Invalid direct link format in connections %d/%d: %s",
			connIndex+1, len(proxySource.Connections), connection)
		debuglog.WarnLog("Parser: Invalid direct link format in connections: %s", connection)
			continue
		}

		if progressCallback != nil {
			progressCallback(20+float64(subscriptionIndex)*50.0/float64(totalSubscriptions),
				fmt.Sprintf("Parsing direct link %d/%d (connection %d)", subscriptionIndex+1, totalSubscriptions, connIndex+1))
		}

		if nodesFromThisSource >= config.MaxNodesPerSubscription {
			skippedDueToLimit++
			continue
		}

		parseStartTime := time.Now()
		node, err := ParseNode(connection, proxySource.Skip)
		if err != nil {
		debuglog.DebugLog("LoadNodesFromSource: Failed to parse connection %d/%d (took %v): %v",
			connIndex+1, len(proxySource.Connections), time.Since(parseStartTime), err)
		debuglog.WarnLog("Parser: Failed to parse direct link from connections: %v", err)
			continue
		}

		if node != nil {
			// Apply prefix, postfix, or mask to tag if specified (with variable substitution)
			node.Tag = applyTagPrefixPostfix(node, proxySource.TagPrefix, proxySource.TagPostfix, proxySource.TagMask, nodesFromThisSource+1)
			node.Tag = MakeTagUnique(node.Tag, tagCounts, "Parser")
			nodes = append(nodes, node)
			nodesFromThisSource++
		}
	}
	if len(proxySource.Connections) > 0 {
		debuglog.DebugLog("LoadNodesFromSource: Processed %d connections in %v",
			len(proxySource.Connections), time.Since(connectionsStartTime))
	}

	if skippedDueToLimit > 0 {
		debuglog.DebugLog("LoadNodesFromSource: Source %d/%d exceeded limit, skipped %d nodes",
			subscriptionIndex+1, totalSubscriptions, skippedDueToLimit)
		debuglog.WarnLog("Parser: Source exceeded limit of %d nodes. Skipped %d additional nodes.",
			config.MaxNodesPerSubscription, skippedDueToLimit)
	}

	totalDuration := time.Since(startTime)
	debuglog.DebugLog("LoadNodesFromSource: END source %d/%d (total duration: %v, nodes: %d)",
		subscriptionIndex+1, totalSubscriptions, totalDuration, len(nodes))
	return nodes, nil
}

// applyTagPrefixPostfix applies prefix and postfix to a node tag if specified in ProxySource.
// If tagMask is set, it replaces the entire tag and ignores prefix/postfix.
// Supports variable substitution in prefix, postfix, and mask.
// Returns the modified tag.
func applyTagPrefixPostfix(node *config.ParsedNode, tagPrefix, tagPostfix, tagMask string, nodeNum int) string {
	// If tag_mask is set, use it to replace the entire tag (ignores prefix/postfix)
	if tagMask != "" {
		return replaceTagVariables(tagMask, node, nodeNum)
	}

	tag := node.Tag

	// Replace variables in prefix
	if tagPrefix != "" {
		prefix := replaceTagVariables(tagPrefix, node, nodeNum)
		tag = prefix + tag
	}

	// Replace variables in postfix
	if tagPostfix != "" {
		postfix := replaceTagVariables(tagPostfix, node, nodeNum)
		tag = tag + postfix
	}

	return tag
}

// replaceTagVariables replaces variables in tag prefix/postfix with actual values from node.
// Supported variables:
//   - {$tag} - original node tag
//   - {$scheme} or {$protocol} - protocol (vless, vmess, trojan, ss, hysteria2)
//   - {$server} - server address
//   - {$port} - server port (number)
//   - {$label} - label from URL (fragment after #)
//   - {$comment} - comment
//   - {$num} - node sequential number starting from 1
func replaceTagVariables(template string, node *config.ParsedNode, nodeNum int) string {
	result := template

	// Replace {$tag}
	result = strings.ReplaceAll(result, "{$tag}", node.Tag)

	// Replace {$scheme} or {$protocol}
	result = strings.ReplaceAll(result, "{$scheme}", node.Scheme)
	result = strings.ReplaceAll(result, "{$protocol}", node.Scheme)

	// Replace {$server}
	result = strings.ReplaceAll(result, "{$server}", node.Server)

	// Replace {$port}
	result = strings.ReplaceAll(result, "{$port}", strconv.Itoa(node.Port))

	// Replace {$label}
	result = strings.ReplaceAll(result, "{$label}", node.Label)

	// Replace {$comment}
	result = strings.ReplaceAll(result, "{$comment}", node.Comment)

	// Replace {$num}
	result = strings.ReplaceAll(result, "{$num}", strconv.Itoa(nodeNum))

	return result
}
