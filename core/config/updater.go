package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// logDuplicateTagStatistics логирует статистику дупликатов тегов.
// Дубликат subscription.LogDuplicateTagStatistics — нельзя импортировать из-за циклической зависимости.
func logDuplicateTagStatistics(tagCounts map[string]int, logPrefix string) {
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

	logDuplicateTagStatistics(tagCounts, "Parser")

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

// WriteToConfig writes outbounds content between @ParserSTART and @ParserEND, and optionally
// endpoints content between @ParserSTART_E and @ParserEND_E. Also updates @ParserConfig block.
// If endpointsContent is non-empty and markers @ParserSTART_E/@ParserEND_E exist, that block is updated.
// If endpoints markers are missing, only outbounds block is updated (no error).
func WriteToConfig(configPath string, content string, endpointsContent string, parserConfig *ParserConfig) error {
	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	configStr := string(data)

	// Find outbounds markers
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

	// Build new content with updated @ParserSTART/@ParserEND section
	newContent := configStr[:startIdx+len(startMarker)] + "\n" + content + "\n" + configStr[endIdx:]

	// If endpoints content provided, replace block between @ParserSTART_E and @ParserEND_E
	if endpointsContent != "" {
		startE := "/** @ParserSTART_E */"
		endE := "/** @ParserEND_E */"
		idxEStart := strings.Index(newContent, startE)
		idxEEnd := strings.Index(newContent, endE)
		if idxEStart != -1 && idxEEnd != -1 && idxEEnd > idxEStart {
			// Indent each line so the block matches wizard output (4 spaces)
			indented := indentEndpointsBlock(endpointsContent, "    ")
			newContent = newContent[:idxEStart+len(startE)] + "\n" + indented + "\n" + newContent[idxEEnd:]
		}
	}

	// Also update @ParserConfig block if parserConfig is provided
	if parserConfig != nil {
		// Update last_updated timestamp (this is when we create/update config)
		NormalizeParserConfig(parserConfig, true)

		// Find the @ParserConfig block using regex
		pattern := regexp.MustCompile(`(/\*\*\s*@ParserConfig\s*\n)([\s\S]*?)(\*/)`)
		matches := pattern.FindSubmatch([]byte(newContent))

		if len(matches) >= 4 {
			// Serialize parserConfig to JSON with indentation
			outerJSON := map[string]interface{}{
				"ParserConfig": parserConfig.ParserConfig,
			}
			finalJSON, err := json.MarshalIndent(outerJSON, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal outer @ParserConfig: %w", err)
			}

			// Build replacement block as []byte to avoid $ interpretation
			// When using ReplaceAllString, Go interprets $ as special character for regex groups
			// This causes placeholders like {$num} and {$scheme} to be corrupted to {}
			var parserConfigBlock []byte
			parserConfigBlock = append(parserConfigBlock, matches[1]...)
			parserConfigBlock = append(parserConfigBlock, finalJSON...)
			parserConfigBlock = append(parserConfigBlock, '\n')
			parserConfigBlock = append(parserConfigBlock, matches[3]...)

			// Manual replacement to avoid $ interpretation by regexp.ReplaceAll/ReplaceAllString
			// Find the match location in the content
			newContentBytes := []byte(newContent)
			matchLoc := pattern.FindIndex(newContentBytes)
			if matchLoc != nil {
				// Replace manually: before match + replacement block + after match
				var result []byte
				result = append(result, newContentBytes[:matchLoc[0]]...)
				result = append(result, parserConfigBlock...)
				result = append(result, newContentBytes[matchLoc[1]:]...)
				newContent = string(result)
			}
		}
	}

	// Write to file (single write operation)
	if err := os.WriteFile(configPath, []byte(newContent), platform.DefaultFileMode); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
