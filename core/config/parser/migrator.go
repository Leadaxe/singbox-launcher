package parser

import (
	"encoding/json"
	"fmt"
	"log"

	"singbox-launcher/core/config"
)

// v1ParserConfig represents version 1 configuration structure
// Version 1 had version at top level
type v1ParserConfig struct {
	Version      int `json:"version,omitempty"`
	ParserConfig struct {
		Proxies   []config.ProxySource    `json:"proxies"`
		Outbounds []config.OutboundConfig `json:"outbounds"`
		Parser    struct {
			Reload      string `json:"reload,omitempty"`
			LastUpdated string `json:"last_updated,omitempty"`
		} `json:"parser,omitempty"`
	} `json:"ParserConfig"`
}

// v2OutboundConfig represents version 2 outbound structure with nested "outbounds" object
// Used ONLY in migrateV2ToV3 function
type v2OutboundConfig struct {
	Tag              string                 `json:"tag"`
	Type             string                 `json:"type"`
	Options          map[string]interface{} `json:"options,omitempty"`
	Filters          map[string]interface{} `json:"filters,omitempty"`
	AddOutbounds     []string               `json:"addOutbounds,omitempty"`
	PreferredDefault map[string]interface{} `json:"preferredDefault,omitempty"`
	Comment          string                 `json:"comment,omitempty"`
	Wizard           interface{}            `json:"wizard,omitempty"`
	Outbounds        struct {
		Proxies          map[string]interface{} `json:"proxies,omitempty"`
		AddOutbounds     []string               `json:"addOutbounds,omitempty"`
		PreferredDefault map[string]interface{} `json:"preferredDefault,omitempty"`
	} `json:"outbounds,omitempty"` // Version 2: nested structure
}

// v2ParserConfig represents version 2 configuration structure
// Version 2 has version inside ParserConfig and outbounds with nested structure
type v2ParserConfig struct {
	ParserConfig struct {
		Version   int                  `json:"version,omitempty"`
		Proxies   []config.ProxySource `json:"proxies"`
		Outbounds []v2OutboundConfig   `json:"outbounds"` // Version 2 format with nested outbounds
		Parser    struct {
			Reload      string `json:"reload,omitempty"`
			LastUpdated string `json:"last_updated,omitempty"`
		} `json:"parser,omitempty"`
	} `json:"ParserConfig"`
}

// MigrationFunc is a function that migrates JSON content from version N to version N+1
// Takes JSON string as input, returns migrated JSON string
type MigrationFunc func(jsonContent string) (string, error)

// ConfigMigrator handles automatic migration of ParserConfig between versions
type ConfigMigrator struct {
	migrations map[int]MigrationFunc // For migrations working with JSON strings
}

// NewConfigMigrator creates a new migrator with registered migrations
func NewConfigMigrator() *ConfigMigrator {
	migrator := &ConfigMigrator{
		migrations: make(map[int]MigrationFunc),
	}

	// Register all migrations
	migrator.RegisterMigration(1, migrateV1ToV2)
	migrator.RegisterMigration(2, migrateV2ToV3)
	migrator.RegisterMigration(3, migrateV3ToV4)

	return migrator
}

// RegisterMigration registers a migration function
func (m *ConfigMigrator) RegisterMigration(fromVersion int, fn MigrationFunc) {
	m.migrations[fromVersion] = fn
}

// ExtractVersion extracts version from JSON content string
// Returns version from ParserConfig.version, or from top-level version (legacy v1), or 0 if not found
// Exported for use in parsers package
func ExtractVersion(jsonContent string) int {
	type versionInfo struct {
		Version      int `json:"version,omitempty"`
		ParserConfig struct {
			Version int `json:"version,omitempty"`
		} `json:"ParserConfig"`
	}

	var info versionInfo
	if err := json.Unmarshal([]byte(jsonContent), &info); err != nil {
		return 0
	}

	// Check ParserConfig.version first (version 2+)
	if info.ParserConfig.Version > 0 {
		return info.ParserConfig.Version
	}

	// Check top-level version (legacy version 1)
	if info.Version > 0 {
		return info.Version
	}

	return 0
}

// MigrateRaw migrates JSON content from its current version to the target version
// Accepts jsonContent string and currentVersion (0 if not determined)
// Returns migrated clean ParserConfig
func (m *ConfigMigrator) MigrateRaw(jsonContent string, currentVersion int, targetVersion int) (*config.ParserConfig, error) {
	if jsonContent == "" {
		return nil, fmt.Errorf("json content is empty")
	}

	// If version not provided, extract it from JSON
	if currentVersion == 0 {
		currentVersion = ExtractVersion(jsonContent)
	}

	// Handle legacy version 1 format (version at top level)
	if currentVersion == 0 {
		// No version specified - treat as version 1 and migrate from there
		currentVersion = 1
		log.Printf("ConfigMigrator: No version specified, treating as version 1 and migrating to version %d", targetVersion)
	}

	// Check if version is too new
	if currentVersion > targetVersion {
		return nil, fmt.Errorf("config version %d is newer than supported version %d. Please update the application",
			currentVersion, targetVersion)
	}

	// Apply migrations sequentially (string → string)
	currentJSON := jsonContent
	for version := currentVersion; version < targetVersion; version++ {
		migration, exists := m.migrations[version]
		if !exists {
			return nil, fmt.Errorf("migration from version %d to %d not found", version, version+1)
		}

		log.Printf("ConfigMigrator: Migrating from version %d to version %d", version, version+1)

		var err error
		currentJSON, err = migration(currentJSON)
		if err != nil {
			return nil, fmt.Errorf("failed to migrate from version %d to %d: %w", version, version+1, err)
		}

		log.Printf("ConfigMigrator: Successfully migrated to version %d", version+1)
	}

	// Parse final JSON into clean ParserConfig (version 3)
	var parserConfig *config.ParserConfig
	if err := json.Unmarshal([]byte(currentJSON), &parserConfig); err != nil {
		return nil, fmt.Errorf("failed to parse migrated @ParserConfig JSON: %w", err)
	}

	return parserConfig, nil
}

// migrateV1ToV2 migrates JSON content from version 1 to version 2
// Version 1 had version at top level, version 2 moved it inside ParserConfig
// Takes JSON string, returns migrated JSON string
func migrateV1ToV2(jsonContent string) (string, error) {
	// Parse JSON into version 1 structure
	var v1 v1ParserConfig
	if err := json.Unmarshal([]byte(jsonContent), &v1); err != nil {
		return "", fmt.Errorf("failed to parse version 1 config: %w", err)
	}

	// Convert to version 2 structure
	v2 := v2ParserConfig{
		ParserConfig: struct {
			Version   int                  `json:"version,omitempty"`
			Proxies   []config.ProxySource `json:"proxies"`
			Outbounds []v2OutboundConfig   `json:"outbounds"`
			Parser    struct {
				Reload      string `json:"reload,omitempty"`
				LastUpdated string `json:"last_updated,omitempty"`
			} `json:"parser,omitempty"`
		}{
			Version:   2,
			Proxies:   v1.ParserConfig.Proxies,
			Outbounds: convertV1OutboundsToV2(v1.ParserConfig.Outbounds),
			Parser:    v1.ParserConfig.Parser,
		},
	}

	// Ensure parser object exists and set default reload
	if v2.ParserConfig.Parser.Reload == "" {
		v2.ParserConfig.Parser.Reload = "4h"
		log.Printf("migrateV1ToV2: Set default reload to '4h'")
	}

	// Serialize to JSON
	resultJSON, err := json.MarshalIndent(v2, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal version 2 config: %w", err)
	}

	log.Printf("migrateV1ToV2: Successfully migrated from version 1 to version 2")
	return string(resultJSON), nil
}

// convertV1OutboundsToV2 converts version 1 outbounds to version 2 format
// Version 1 outbounds are already flat, version 2 keeps them flat (no nested structure yet)
func convertV1OutboundsToV2(v1Outbounds []config.OutboundConfig) []v2OutboundConfig {
	v2Outbounds := make([]v2OutboundConfig, 0, len(v1Outbounds))
	for _, v1 := range v1Outbounds {
		v2 := v2OutboundConfig{
			Tag:              v1.Tag,
			Type:             v1.Type,
			Options:          v1.Options,
			Filters:          v1.Filters,
			AddOutbounds:     v1.AddOutbounds,
			PreferredDefault: v1.PreferredDefault,
			Comment:          v1.Comment,
			Wizard:           v1.Wizard,
		}
		v2Outbounds = append(v2Outbounds, v2)
	}
	return v2Outbounds
}

// migrateV2ToV3 migrates JSON content from version 2 to version 3
// Version 3 removes nested "outbounds" object and renames "proxies" to "filters"
// Takes JSON string, returns migrated JSON string
func migrateV2ToV3(jsonContent string) (string, error) {
	// Parse JSON into version 2 structure
	var v2 v2ParserConfig
	if err := json.Unmarshal([]byte(jsonContent), &v2); err != nil {
		return "", fmt.Errorf("failed to parse version 2 config: %w", err)
	}

	// Convert to version 3 structure (clean ParserConfig)
	v3 := config.ParserConfig{
		ParserConfig: struct {
			Version   int                     `json:"version,omitempty"`
			Proxies   []config.ProxySource    `json:"proxies"`
			Outbounds []config.OutboundConfig `json:"outbounds"`
			Parser    struct {
				Reload      string `json:"reload,omitempty"`
				LastUpdated string `json:"last_updated,omitempty"`
			} `json:"parser,omitempty"`
		}{
			Version:   3,
			Proxies:   v2.ParserConfig.Proxies,
			Outbounds: convertV2OutboundsToV3(v2.ParserConfig.Outbounds),
			Parser:    v2.ParserConfig.Parser,
		},
	}

	// Serialize to JSON
	resultJSON, err := json.MarshalIndent(v3, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal version 3 config: %w", err)
	}

	log.Printf("migrateV2ToV3: Successfully migrated from version 2 to version 3")
	return string(resultJSON), nil
}

// convertV2OutboundsToV3 converts version 2 outbounds to version 3 format
// Migrates nested "outbounds" structure to flat structure
// This is the ONLY place where v2OutboundConfig is used
func convertV2OutboundsToV3(v2Outbounds []v2OutboundConfig) []config.OutboundConfig {
	v3Outbounds := make([]config.OutboundConfig, 0, len(v2Outbounds))
	for _, v2 := range v2Outbounds {
		v3 := config.OutboundConfig{
			Tag:              v2.Tag,
			Type:             v2.Type,
			Options:          v2.Options,
			Filters:          v2.Filters,
			AddOutbounds:     v2.AddOutbounds,
			PreferredDefault: v2.PreferredDefault,
			Comment:          v2.Comment,
			Wizard:           v2.Wizard,
		}

		// Migrate nested outbounds structure to flat structure
		if len(v2.Outbounds.Proxies) > 0 {
			// Create Filters field if it doesn't exist
			if v3.Filters == nil {
				v3.Filters = make(map[string]interface{})
			}
			// Copy proxies to filters
			for k, v := range v2.Outbounds.Proxies {
				v3.Filters[k] = v
			}
			log.Printf("migrateV2ToV3: Migrated 'outbounds.proxies' to 'filters' for outbound '%s'", v2.Tag)
		}

		if len(v2.Outbounds.AddOutbounds) > 0 {
			// Copy addOutbounds to top level
			v3.AddOutbounds = v2.Outbounds.AddOutbounds
			log.Printf("migrateV2ToV3: Migrated 'outbounds.addOutbounds' to top level for outbound '%s'", v2.Tag)
		}

		if len(v2.Outbounds.PreferredDefault) > 0 {
			// Copy preferredDefault to top level
			v3.PreferredDefault = v2.Outbounds.PreferredDefault
			log.Printf("migrateV2ToV3: Migrated 'outbounds.preferredDefault' to top level for outbound '%s'", v2.Tag)
		}

		v3Outbounds = append(v3Outbounds, v3)
	}
	return v3Outbounds
}

// migrateV3ToV4 migrates JSON content from version 3 to version 4
// Version 4 adds local outbounds to ProxySource, but the OutboundConfig structure remains the same.
// Takes JSON string, returns migrated JSON string
func migrateV3ToV4(jsonContent string) (string, error) {
	var v3 config.ParserConfig
	if err := json.Unmarshal([]byte(jsonContent), &v3); err != nil {
		return "", fmt.Errorf("failed to parse version 3 config: %w", err)
	}
	v3.ParserConfig.Version = 4 // Update version number
	resultJSON, err := json.MarshalIndent(v3, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal version 4 config: %w", err)
	}
	log.Printf("migrateV3ToV4: Successfully migrated from version 3 to version 4")
	return string(resultJSON), nil
}
