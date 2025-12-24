package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	"github.com/muhammadmuzzammil1998/jsonc"
)

// GetSelectorGroupsFromConfig extracts selector group names from config.json
func GetSelectorGroupsFromConfig(configPath string) ([]string, string, error) {
	// Internal function to strip comments
	stripComments := func(data []byte) []byte {
		commentRegex := regexp.MustCompile(`(?m)\s+//.*$|/\*[\s\S]*?\*/`)
		var clean = commentRegex.ReplaceAll(data, nil)
		emptyLineRegex := regexp.MustCompile(`(?m)^\s*\n`)
		return emptyLineRegex.ReplaceAll(clean, nil)
	}
	removeTrailingCommas := func(data []byte) []byte {
		re := regexp.MustCompile(`,(\s*[\]\}])`)
		return re.ReplaceAll(data, []byte("$1"))
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read config.json: %w", err)
	}

	// Convert JSONC (with comments/trailing commas) into clean JSON
	cleanData := jsonc.ToJSON(data)
	cleanData = removeTrailingCommas(stripComments(cleanData))

	var jsonData map[string]interface{}
	if err := json.Unmarshal(cleanData, &jsonData); err != nil {
		return nil, "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Extract selector groups from outbounds
	outbounds, ok := jsonData["outbounds"].([]interface{})
	if !ok {
		return []string{"proxy-out"}, "", nil
	}

	var selectorGroups []string
	var defaultSelector string

	// Get default from route.final
	if route, ok := jsonData["route"].(map[string]interface{}); ok {
		if final, ok := route["final"].(string); ok {
			defaultSelector = final
		}
	}

	// Find all selector type outbounds
	for _, outbound := range outbounds {
		outboundMap, ok := outbound.(map[string]interface{})
		if !ok {
			continue
		}

		outboundType, _ := outboundMap["type"].(string)
		if outboundType == "selector" {
			if tag, ok := outboundMap["tag"].(string); ok {
				// Skip if already in list
				found := false
				for _, existing := range selectorGroups {
					if existing == tag {
						found = true
						break
					}
				}
				if !found {
					selectorGroups = append(selectorGroups, tag)
				}
			}
		}
	}

	// If no selectors found, return default
	if len(selectorGroups) == 0 {
		selectorGroups = []string{"proxy-out"}
	}

	// If defaultSelector is not in the list, use first one
	if defaultSelector != "" {
		found := false
		for _, group := range selectorGroups {
			if group == defaultSelector {
				found = true
				break
			}
		}
		if !found {
			defaultSelector = selectorGroups[0]
		}
	} else {
		defaultSelector = selectorGroups[0]
	}

	return selectorGroups, defaultSelector, nil
}

// GetTunInterfaceName extracts TUN interface name from config.json
// Returns empty string if no TUN interface is configured
func GetTunInterfaceName(configPath string) (string, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config: %w", err)
	}

	// Parse JSONC (with comments) to clean JSON
	cleanData := jsonc.ToJSON(data)

	var config map[string]interface{}
	if err := json.Unmarshal(cleanData, &config); err != nil {
		return "", fmt.Errorf("failed to parse config: %w", err)
	}

	inbounds, ok := config["inbounds"].([]interface{})
	if !ok {
		return "", nil // No inbounds section, no TUN interface
	}

	for _, inbound := range inbounds {
		inboundMap, ok := inbound.(map[string]interface{})
		if !ok {
			continue
		}

		if inboundMap["type"] == "tun" {
			if interfaceName, ok := inboundMap["interface_name"].(string); ok && interfaceName != "" {
				return interfaceName, nil
			}
		}
	}

	return "", nil // No TUN interface found in config
}



