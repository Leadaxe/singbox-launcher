// Package config: outbound_filter.go — filtering logic for selector outbounds.
//
// Functions here determine which nodes match a selector's filters (tag, host, scheme, label, etc.).
// Supports literal match, negation !literal, regex /pattern/i, negation regex !/pattern/i.
// Used by outbound_generator.go (GenerateSelectorWithFilteredAddOutbounds, buildOutboundsInfo)
// and by PreviewSelectorNodes for UI preview.
package config

import (
	"singbox-launcher/core/config/configtypes"
)

// filterNodesForSelector returns nodes that match the filter. filter may be nil (all nodes),
// a single map (AND of key/pattern), or a slice of maps (OR of maps). Empty map = no filter.
// FilterNodesExcludeFromGlobal drops nodes whose source has exclude_from_global (SPEC 026).
func FilterNodesExcludeFromGlobal(allNodes []*ParsedNode, proxies []ProxySource) []*ParsedNode {
	if len(allNodes) == 0 {
		return allNodes
	}
	out := make([]*ParsedNode, 0, len(allNodes))
	for _, n := range allNodes {
		idx := n.SourceIndex
		if idx < 0 || idx >= len(proxies) {
			out = append(out, n)
			continue
		}
		if proxies[idx].ExcludeFromGlobal {
			continue
		}
		out = append(out, n)
	}
	return out
}

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

// convertFilterToStringMap flattens filter map to string values for matching (non-string values are skipped).
func convertFilterToStringMap(filter map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range filter {
		if str, ok := v.(string); ok {
			result[k] = str
		}
	}
	return result
}

// matchesFilter returns true if the node has matching values for every key in filter (AND); each value is checked with matchesPattern.
func matchesFilter(node *ParsedNode, filter map[string]string) bool {
	for key, pattern := range filter {
		value := getNodeValue(node, key)
		if !matchesPattern(value, pattern) {
			return false // At least one key doesn't match
		}
	}
	return true // All keys match
}

// getNodeValue returns the node field used in filters: tag, host, label, scheme, fragment (alias for label), comment.
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
	case "flow":
		return node.Flow
	default:
		return ""
	}
}

// matchesPattern matches value against pattern: literal, !literal, /regex/i, !/regex/i. Case-insensitive for regex.
// Delegates to the shared configtypes.MatchesPattern so selector filters and subscription skip-filters
// stay byte-equivalent (see core/config/configtypes/matcher.go).
func matchesPattern(value, pattern string) bool {
	return configtypes.MatchesPattern(value, pattern)
}

// PreviewSelectorNodes returns nodes that match outboundConfig.Filters and the default tag
// based on outboundConfig.PreferredDefault. It is used by UI layers to build a selector
// preview that is consistent with the real selector generation logic.
//
// allNodes must be the same set of nodes that will be used for selector generation
// (i.e. result of the same LoadNodesFromSource pipeline that GenerateOutboundsFromParserConfig uses).
// PreviewGlobalSelectorNodes applies exclude_from_global, then the same filter logic as PreviewSelectorNodes.
func PreviewGlobalSelectorNodes(allNodes []*ParsedNode, proxies []ProxySource, outboundConfig OutboundConfig) ([]*ParsedNode, string) {
	pool := FilterNodesExcludeFromGlobal(allNodes, proxies)
	return PreviewSelectorNodes(pool, outboundConfig)
}

func PreviewSelectorNodes(allNodes []*ParsedNode, outboundConfig OutboundConfig) ([]*ParsedNode, string) {
	filtered := filterNodesForSelector(allNodes, outboundConfig.Filters)

	defaultTag := ""
	if len(outboundConfig.PreferredDefault) > 0 {
		preferredFilter := convertFilterToStringMap(outboundConfig.PreferredDefault)
		for _, node := range filtered {
			if matchesFilter(node, preferredFilter) {
				defaultTag = node.Tag
				break
			}
		}
	}

	return filtered, defaultTag
}

// ExposeTagSyntheticNode builds a minimal ParsedNode for ParserConfig.outbounds[].filters (SPEC §5):
// tag and comment from the wizard local outbound; host/scheme/label left empty.
func ExposeTagSyntheticNode(tag, comment string) *ParsedNode {
	return &ParsedNode{Tag: tag, Comment: comment, SourceIndex: UnsetSourceIndex}
}

// SelectorFiltersAcceptNode reports whether a single node matches the same filter rules as filterNodesForSelector
// (including OR-array and AND-object semantics).
func SelectorFiltersAcceptNode(filter interface{}, node *ParsedNode) bool {
	if node == nil {
		return false
	}
	matched := filterNodesForSelector([]*ParsedNode{node}, filter)
	return len(matched) > 0
}
