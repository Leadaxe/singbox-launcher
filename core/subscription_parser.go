package core

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// DecodeSubscriptionContent decodes subscription content from base64 or returns plain text
// Returns decoded content and error if decoding fails
func DecodeSubscriptionContent(content []byte) ([]byte, error) {
	if len(content) == 0 {
		return nil, fmt.Errorf("content is empty")
	}

	// Try to decode as base64
	decoded, err := base64.URLEncoding.DecodeString(strings.TrimSpace(string(content)))
	if err != nil {
		// If URL encoding fails, try standard encoding
		decoded, err = base64.StdEncoding.DecodeString(strings.TrimSpace(string(content)))
		if err != nil {
			// If both fail, assume it's plain text
			log.Printf("DecodeSubscriptionContent: Content is not base64, treating as plain text")
			return content, nil
		}
	}

	// Check if decoded content is empty
	if len(decoded) == 0 {
		return nil, fmt.Errorf("decoded content is empty")
	}

	return decoded, nil
}

// FetchSubscription fetches subscription content from URL and decodes it
// Returns decoded content and error if fetch or decode fails
func FetchSubscription(url string) ([]byte, error) {
	startTime := time.Now()
	log.Printf("[DEBUG] FetchSubscription: START at %s, URL: %s", startTime.Format("15:04:05.000"), url)

	// Создаем контекст с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), NetworkRequestTimeout)
	defer cancel()

	// Используем универсальный HTTP клиент
	client := createHTTPClient(NetworkRequestTimeout)

	requestStartTime := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Printf("[DEBUG] FetchSubscription: Failed to create request (took %v): %v", time.Since(requestStartTime), err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	log.Printf("[DEBUG] FetchSubscription: Created request in %v", time.Since(requestStartTime))

	// Set user agent to avoid blocking
	req.Header.Set("User-Agent", "singbox-launcher/1.0")

	doStartTime := time.Now()
	log.Printf("[DEBUG] FetchSubscription: Sending HTTP request")
	resp, err := client.Do(req)
	doDuration := time.Since(doStartTime)
	if err != nil {
		log.Printf("[DEBUG] FetchSubscription: HTTP request failed (took %v): %v", doDuration, err)
		// Проверяем тип ошибки
		if IsNetworkError(err) {
			return nil, fmt.Errorf("network error: %s", GetNetworkErrorMessage(err))
		}
		return nil, fmt.Errorf("failed to fetch subscription: %w", err)
	}
	defer resp.Body.Close()
	log.Printf("[DEBUG] FetchSubscription: Received HTTP response in %v (status: %d, content-length: %d)",
		doDuration, resp.StatusCode, resp.ContentLength)

	if resp.StatusCode != http.StatusOK {
		log.Printf("[DEBUG] FetchSubscription: Non-OK status code: %d", resp.StatusCode)
		return nil, fmt.Errorf("subscription server returned status %d", resp.StatusCode)
	}

	readStartTime := time.Now()
	log.Printf("[DEBUG] FetchSubscription: Reading response body")
	content, err := io.ReadAll(resp.Body)
	readDuration := time.Since(readStartTime)
	if err != nil {
		log.Printf("[DEBUG] FetchSubscription: Failed to read response body (took %v): %v", readDuration, err)
		return nil, fmt.Errorf("failed to read subscription content: %w", err)
	}
	log.Printf("[DEBUG] FetchSubscription: Read %d bytes in %v", len(content), readDuration)

	// Check if content is empty
	if len(content) == 0 {
		log.Printf("[DEBUG] FetchSubscription: Empty content received")
		return nil, fmt.Errorf("subscription returned empty content")
	}

	// Decode base64 if needed
	decodeStartTime := time.Now()
	log.Printf("[DEBUG] FetchSubscription: Decoding subscription content")
	decoded, err := DecodeSubscriptionContent(content)
	decodeDuration := time.Since(decodeStartTime)
	if err != nil {
		log.Printf("[DEBUG] FetchSubscription: Failed to decode content (took %v): %v", decodeDuration, err)
		return nil, fmt.Errorf("failed to decode subscription content: %w", err)
	}
	log.Printf("[DEBUG] FetchSubscription: Decoded content in %v (original: %d bytes, decoded: %d bytes)",
		decodeDuration, len(content), len(decoded))

	totalDuration := time.Since(startTime)
	log.Printf("[DEBUG] FetchSubscription: END (total duration: %v)", totalDuration)
	return decoded, nil
}

// ParserConfig represents the configuration structure from @ParserConfig block
// Clean structure for version 3 (legacy versions are migrated automatically)
type ParserConfig struct {
	ParserConfig struct {
		Version   int              `json:"version,omitempty"`
		Proxies   []ProxySource    `json:"proxies"`
		Outbounds []OutboundConfig `json:"outbounds"`
		Parser    struct {
			Reload      string `json:"reload,omitempty"`       // Интервал автоматического обновления
			LastUpdated string `json:"last_updated,omitempty"` // Время последнего обновления (RFC3339, UTC)
		} `json:"parser,omitempty"`
	} `json:"ParserConfig"`
}

// ParserConfigVersion is the current version of ParserConfig format
const ParserConfigVersion = 4

// NormalizeParserConfig normalizes ParserConfig structure:
// - Ensures version is set to ParserConfigVersion
// - Sets default reload to "4h" if not specified
// - Optionally updates last_updated timestamp (if updateLastUpdated is true)
// Note: Migration is handled in ExtractParserConfig
// This function works with already-migrated clean ParserConfig
func NormalizeParserConfig(parserConfig *ParserConfig, updateLastUpdated bool) {
	if parserConfig == nil {
		return
	}

	// Ensure version is set to current version (always update to latest)
	parserConfig.ParserConfig.Version = ParserConfigVersion

	// Ensure parser object exists (create if missing)
	// Set default reload to "4h" if not specified
	if parserConfig.ParserConfig.Parser.Reload == "" {
		parserConfig.ParserConfig.Parser.Reload = "4h"
	}

	// Optionally update last_updated timestamp
	if updateLastUpdated {
		parserConfig.ParserConfig.Parser.LastUpdated = time.Now().UTC().Format(time.RFC3339)
	}
}

// ProxySource represents a proxy subscription source
type ProxySource struct {
	Source      string              `json:"source,omitempty"`
	Connections []string            `json:"connections,omitempty"`
	Skip        []map[string]string `json:"skip,omitempty"`
	Outbounds   []OutboundConfig    `json:"outbounds,omitempty"`   // Local outbounds for this source (version 4)
	TagPrefix   string              `json:"tag_prefix,omitempty"`  // Prefix to add to all node tags from this source
	TagPostfix  string              `json:"tag_postfix,omitempty"` // Postfix to add to all node tags from this source
}

// OutboundConfig represents an outbound selector configuration (version 3)
// Clean structure without legacy fields - used in main codebase
type OutboundConfig struct {
	Tag              string                 `json:"tag"`
	Type             string                 `json:"type"`
	Options          map[string]interface{} `json:"options,omitempty"`
	Filters          map[string]interface{} `json:"filters,omitempty"`
	AddOutbounds     []string               `json:"addOutbounds,omitempty"`
	PreferredDefault map[string]interface{} `json:"preferredDefault,omitempty"`
	Comment          string                 `json:"comment,omitempty"`
	Wizard           string                 `json:"wizard,omitempty"` // "hide" to hide from wizard second tab
}

// ExtractParserConfig extracts the @ParserConfig block from config.json
// Returns the parsed ParserConfig structure and error if extraction or parsing fails
// Uses ConfigMigrator for handling legacy versions and migrations
func ExtractParserConfig(configPath string) (*ParserConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config.json: %w", err)
	}

	// Find the @ParserConfig block using regex
	// Pattern matches: /** @ParserConfig ... */
	pattern := regexp.MustCompile(`/\*\*\s*@ParserConfig\s*\n([\s\S]*?)\*/`)
	matches := pattern.FindSubmatch(data)

	if len(matches) < 2 {
		return nil, fmt.Errorf("@ParserConfig block not found in config.json")
	}

	// Extract the JSON content from the comment block
	jsonContent := strings.TrimSpace(string(matches[1]))

	// Extract version from JSON to check if migration is needed
	currentVersion := extractVersion(jsonContent)

	var parserConfig *ParserConfig

	// If version is already current, parse directly without migration
	if currentVersion == ParserConfigVersion {
		if err := json.Unmarshal([]byte(jsonContent), &parserConfig); err != nil {
			return nil, fmt.Errorf("failed to parse @ParserConfig JSON: %w", err)
		}
	} else {
		// Version needs migration or is 0 - use migrator (it will handle version 0 and check for too new versions)
		migrator := NewConfigMigrator()
		var err error
		parserConfig, err = migrator.MigrateRaw(jsonContent, currentVersion, ParserConfigVersion)
		if err != nil {
			return nil, fmt.Errorf("failed to migrate config: %w", err)
		}
	}

	log.Printf("ExtractParserConfig: Successfully extracted @ParserConfig (version %d) with %d proxy sources and %d outbounds",
		parserConfig.ParserConfig.Version,
		len(parserConfig.ParserConfig.Proxies),
		len(parserConfig.ParserConfig.Outbounds))

	return parserConfig, nil
}
