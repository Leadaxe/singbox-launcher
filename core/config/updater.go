package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
)

// logDuplicateTagStatistics logs statistics about duplicate tags found during processing
func logDuplicateTagStatistics(tagCounts map[string]int, logPrefix string) {
	duplicatesFound := false
	for tag, count := range tagCounts {
		if count > 1 {
			if !duplicatesFound {
				log.Printf("%s: === Duplicate Tag Statistics ===", logPrefix)
				duplicatesFound = true
			}
			log.Printf("%s: Tag '%s' appeared %d times (original + %d duplicates)", logPrefix, tag, count, count-1)
		}
	}
	if duplicatesFound {
		log.Printf("%s: === End of Duplicate Tag Statistics ===", logPrefix)
	}
}

// UpdateConfigFromSubscriptions updates config.json from subscriptions
// This is the main function that coordinates the update process
func UpdateConfigFromSubscriptions(
	configPath string,
	parserConfig *ParserConfig,
	progressCallback func(float64, string),
	loadNodesFunc func(ProxySource, map[string]int, func(float64, string), int, int) ([]*ParsedNode, error),
) error {
	log.Println("Parser: Starting configuration update...")

	// Step 2: Generate all outbounds using unified function
	// Map to track unique tags and their counts
	tagCounts := make(map[string]int)
	log.Printf("Parser: Initializing tag deduplication tracker")

	result, err := GenerateOutboundsFromParserConfig(parserConfig, tagCounts, progressCallback, loadNodesFunc)
	if err != nil {
		if progressCallback != nil {
			progressCallback(-1, fmt.Sprintf("Error: %v", err))
		}
		return fmt.Errorf("failed to generate outbounds: %w", err)
	}

	// Log statistics about duplicates
	logDuplicateTagStatistics(tagCounts, "Parser")

	log.Printf("Parser: Generated %d nodes, %d local selectors, %d global selectors",
		result.NodesCount, result.LocalSelectorsCount, result.GlobalSelectorsCount)

	selectorsJSON := result.OutboundsJSON

	// Final check: ensure we have content to write
	if len(selectorsJSON) == 0 {
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
	if err := WriteToConfig(configPath, content, parserConfig); err != nil {
		if progressCallback != nil {
			progressCallback(-1, fmt.Sprintf("Write error: %v", err))
		}
		return fmt.Errorf("failed to write to config: %w", err)
	}

	log.Printf("Parser: Done! File %s successfully updated.", configPath)
	log.Printf("Parser: Successfully updated last_updated timestamp")

	if progressCallback != nil {
		progressCallback(100, "Configuration updated successfully!")
	}

	return nil
}

// WriteToConfig writes content between @ParserSTART and @ParserEND markers
// Also updates @ParserConfig block with last_updated timestamp in a single file write
func WriteToConfig(configPath string, content string, parserConfig *ParserConfig) error {
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

	// Build new content with updated @ParserSTART/@ParserEND section
	newContent := configStr[:startIdx+len(startMarker)] + "\n" + content + "\n" + configStr[endIdx:]

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

			parserConfigBlock := string(matches[1]) + string(finalJSON) + "\n" + string(matches[3])
			newContent = pattern.ReplaceAllString(newContent, parserConfigBlock)
		}
	}

	// Write to file (single write operation)
	if err := os.WriteFile(configPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
