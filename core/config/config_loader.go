package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/muhammadmuzzammil1998/jsonc"
)

// GetSelectorGroupsFromConfig extracts selector group names from config.json
func GetSelectorGroupsFromConfig(configPath string) ([]string, string, error) {
	cleanData, err := getConfigJSON(configPath)
	if err != nil {
		return nil, "", err
	}
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

// getConfigJSON reads config and returns JSON safe to parse (JSONC).
func getConfigJSON(configPath string) ([]byte, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}
	return jsonc.ToJSON(data), nil
}

// ConfigHasTun returns true if config has any TUN inbound (used to decide if privilege escalation is needed on macOS).
func ConfigHasTun(configPath string) (bool, error) {
	cleanData, err := getConfigJSON(configPath)
	if err != nil {
		return false, err
	}
	var config map[string]interface{}
	if err := json.Unmarshal(cleanData, &config); err != nil {
		return false, fmt.Errorf("failed to parse config: %w", err)
	}
	inbounds, ok := config["inbounds"].([]interface{})
	if !ok {
		return false, nil
	}
	for _, inbound := range inbounds {
		inboundMap, ok := inbound.(map[string]interface{})
		if !ok {
			continue
		}
		if inboundMap["type"] == "tun" {
			return true, nil
		}
	}
	return false, nil
}

// ExperimentalCacheFileFromSection parses experimental.cache_file from the JSON value of the top-level
// "experimental" key (not the full config). Returns whether removal should be attempted and the path string
// from JSON (may be relative to the sing-box working directory, typically bin/).
func ExperimentalCacheFileFromSection(raw json.RawMessage) (shouldRemove bool, path string) {
	var exp struct {
		CacheFile *struct {
			Enabled *bool  `json:"enabled"`
			Path    string `json:"path"`
		} `json:"cache_file"`
	}
	if err := json.Unmarshal(raw, &exp); err != nil || exp.CacheFile == nil {
		return false, ""
	}
	path = strings.TrimSpace(exp.CacheFile.Path)
	if path == "" {
		return false, ""
	}
	if exp.CacheFile.Enabled != nil && !*exp.CacheFile.Enabled {
		return false, ""
	}
	return true, path
}
