package parser

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"singbox-launcher/core/config"
)

// MaxConfigFileSize defines the maximum allowed size for config.json file
const MaxConfigFileSize = 50 * 1024 * 1024 // 50 MB

// ExtractParserConfig extracts the @ParserConfig block from config.json
// Returns the parsed ParserConfig structure and error if extraction or parsing fails
// Uses ConfigMigrator for handling legacy versions and migrations
// Uses ExtractParserConfigBlock for regex parsing
func ExtractParserConfig(configPath string) (*config.ParserConfig, error) {
	// Check file size before reading
	fileInfo, err := os.Stat(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat config.json: %w", err)
	}
	if fileInfo.Size() > MaxConfigFileSize {
		return nil, fmt.Errorf("config.json file size (%d bytes) exceeds maximum (%d bytes)", fileInfo.Size(), MaxConfigFileSize)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config.json: %w", err)
	}

	// Validate JSON content size
	if len(data) > MaxConfigFileSize {
		return nil, fmt.Errorf("config.json content size (%d bytes) exceeds maximum (%d bytes)", len(data), MaxConfigFileSize)
	}

	// Extract JSON content from @ParserConfig block
	jsonContent, err := ExtractParserConfigBlock(data)
	if err != nil {
		return nil, err
	}

	// Validate extracted JSON content size
	if len(jsonContent) > MaxConfigFileSize {
		return nil, fmt.Errorf("extracted @ParserConfig JSON size (%d bytes) exceeds maximum (%d bytes)", len(jsonContent), MaxConfigFileSize)
	}

	// Extract version from JSON to check if migration is needed
	currentVersion := ExtractVersion(jsonContent)

	var parserConfig *config.ParserConfig

	// If version is already current, parse directly without migration
	if currentVersion == config.ParserConfigVersion {
		if err := json.Unmarshal([]byte(jsonContent), &parserConfig); err != nil {
			return nil, fmt.Errorf("failed to parse @ParserConfig JSON: %w", err)
		}
	} else {
		// Version needs migration or is 0 - use migrator (it will handle version 0 and check for too new versions)
		migrator := NewConfigMigrator()
		var err error
		parserConfig, err = migrator.MigrateRaw(jsonContent, currentVersion, config.ParserConfigVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to migrate config: %w", err)
		}
	}

	// Normalize defaults (but don't update last_updated - this is loading existing config)
	config.NormalizeParserConfig(parserConfig, false)

	log.Printf("ExtractParserConfig: Successfully extracted @ParserConfig (version %d) with %d proxy sources and %d outbounds",
		parserConfig.ParserConfig.Version,
		len(parserConfig.ParserConfig.Proxies),
		len(parserConfig.ParserConfig.Outbounds))

	return parserConfig, nil
}
