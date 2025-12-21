package parser

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"singbox-launcher/core/config"
)

// ExtractParserConfig extracts the @ParserConfig block from config.json
// Returns the parsed ParserConfig structure and error if extraction or parsing fails
// Uses ConfigMigrator for handling legacy versions and migrations
// Uses ExtractParserConfigBlock for regex parsing
func ExtractParserConfig(configPath string) (*config.ParserConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config.json: %w", err)
	}

	// Extract JSON content from @ParserConfig block
	jsonContent, err := ExtractParserConfigBlock(data)
	if err != nil {
		return nil, err
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
