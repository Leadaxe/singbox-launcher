package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"singbox-launcher/core/config/subscription"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// indentEndpointsBlock prefixes each line with indent (for pretty endpoints block in config).
func indentEndpointsBlock(s, indent string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = indent + line
	}
	return strings.Join(lines, "\n")
}

// UpdateConfigFromSubscriptions updates config.json from subscriptions
// This is the main function that coordinates the update process
func UpdateConfigFromSubscriptions(
	configPath string,
	parserConfig *ParserConfig,
	progressCallback func(float64, string),
	loadNodesFunc func(ProxySource, map[string]int, func(float64, string), int, int) ([]*ParsedNode, error),
) error {
	debuglog.InfoLog("Parser: Starting configuration update...")

	tagCounts := make(map[string]int)
	debuglog.DebugLog("Parser: Initializing tag deduplication tracker")

	result, err := GenerateOutboundsFromParserConfig(parserConfig, tagCounts, progressCallback, loadNodesFunc)
	if err != nil {
		if progressCallback != nil {
			progressCallback(-1, fmt.Sprintf("Error: %v", err))
		}
		return fmt.Errorf("failed to generate outbounds: %w", err)
	}

	subscription.LogDuplicateTagStatistics(tagCounts, "Parser")

	debuglog.InfoLog("Parser: Generated %d nodes, %d local selectors, %d global selectors",
		result.NodesCount, result.LocalSelectorsCount, result.GlobalSelectorsCount)

	selectorsJSON := result.OutboundsJSON

	// Final check: ensure we have something to write (outbounds and/or endpoints)
	if len(selectorsJSON) == 0 && len(result.EndpointsJSON) == 0 {
		if progressCallback != nil {
			progressCallback(-1, "Error: nothing to write to configuration")
		}
		return fmt.Errorf("no content generated - cannot write empty result to config")
	}

	// Step 3: Write to file
	if progressCallback != nil {
		progressCallback(90, "Writing to config file...")
	}

	content := strings.Join(selectorsJSON, "\n")
	// Join with ",\n" so array elements are separated by comma (each EndpointsJSON item can be multiline)
	endpointsContent := strings.Join(result.EndpointsJSON, ",\n")
	if err := WriteToConfig(configPath, content, endpointsContent, parserConfig); err != nil {
		if progressCallback != nil {
			progressCallback(-1, fmt.Sprintf("Write error: %v", err))
		}
		return fmt.Errorf("failed to write to config: %w", err)
	}

	debuglog.InfoLog("Parser: Done! File %s successfully updated.", configPath)
	debuglog.DebugLog("Parser: Successfully updated last_updated timestamp")

	if progressCallback != nil {
		progressCallback(100, "Configuration updated successfully!")
	}

	return nil
}

// PopulateParserMarkers replaces content between @ParserSTART/@ParserEND (and optionally
// @ParserSTART_E/@ParserEND_E) markers in configText. Works purely in memory.
// Used by WriteToConfig (file-based) and by wizard save (to populate config-check.json in memory).
func PopulateParserMarkers(configText string, outboundsContent string, endpointsContent string) (string, error) {
	startMarker := "/** @ParserSTART */"
	endMarker := "/** @ParserEND */"

	startIdx := strings.Index(configText, startMarker)
	endIdx := strings.Index(configText, endMarker)

	if startIdx == -1 || endIdx == -1 {
		return "", fmt.Errorf("markers @ParserSTART or @ParserEND not found")
	}
	if endIdx <= startIdx {
		return "", fmt.Errorf("invalid marker positions")
	}

	result := configText[:startIdx+len(startMarker)] + "\n" + outboundsContent + "\n" + configText[endIdx:]

	if endpointsContent != "" {
		startE := "/** @ParserSTART_E */"
		endE := "/** @ParserEND_E */"
		idxEStart := strings.Index(result, startE)
		idxEEnd := strings.Index(result, endE)
		if idxEStart != -1 && idxEEnd != -1 && idxEEnd > idxEStart {
			indented := indentEndpointsBlock(endpointsContent, "    ")
			result = result[:idxEStart+len(startE)] + "\n" + indented + "\n" + result[idxEEnd:]
		}
	}

	return result, nil
}

// WriteToConfig writes outbounds content between @ParserSTART and @ParserEND, and optionally
// endpoints content between @ParserSTART_E and @ParserEND_E. Also updates @ParserConfig block.
// If endpointsContent is non-empty and markers @ParserSTART_E/@ParserEND_E exist, that block is updated.
// If endpoints markers are missing, only outbounds block is updated (no error).
func WriteToConfig(configPath string, content string, endpointsContent string, parserConfig *ParserConfig) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	newContent, err := PopulateParserMarkers(string(data), content, endpointsContent)
	if err != nil {
		return err
	}

	if parserConfig != nil {
		NormalizeParserConfig(parserConfig, true)

		pattern := regexp.MustCompile(`(/\*\*\s*@ParserConfig\s*\n)([\s\S]*?)(\*/)`)
		matches := pattern.FindSubmatch([]byte(newContent))

		if len(matches) >= 4 {
			outerJSON := map[string]interface{}{
				"ParserConfig": parserConfig.ParserConfig,
			}
			finalJSON, err := json.MarshalIndent(outerJSON, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal outer @ParserConfig: %w", err)
			}

			// Build replacement block as []byte to avoid $ interpretation by regexp
			var parserConfigBlock []byte
			parserConfigBlock = append(parserConfigBlock, matches[1]...)
			parserConfigBlock = append(parserConfigBlock, finalJSON...)
			parserConfigBlock = append(parserConfigBlock, '\n')
			parserConfigBlock = append(parserConfigBlock, matches[3]...)

			newContentBytes := []byte(newContent)
			matchLoc := pattern.FindIndex(newContentBytes)
			if matchLoc != nil {
				var result []byte
				result = append(result, newContentBytes[:matchLoc[0]]...)
				result = append(result, parserConfigBlock...)
				result = append(result, newContentBytes[matchLoc[1]:]...)
				newContent = string(result)
			}
		}
	}

	// Atomic write: stage to .tmp then rename over configPath. A crash or
	// power loss mid-WriteFile would otherwise truncate the user's config.json
	// to zero bytes, which makes sing-box refuse to start and forces the user
	// back through the Wizard. Rename is atomic on POSIX and on Windows NTFS
	// since MoveFileEx with MOVEFILE_REPLACE_EXISTING is the default for
	// os.Rename on Go 1.22+.
	tmp := configPath + ".tmp"
	if err := os.WriteFile(tmp, []byte(newContent), platform.DefaultFileMode); err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}
	if err := os.Rename(tmp, configPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("failed to replace config file: %w", err)
	}

	return nil
}
