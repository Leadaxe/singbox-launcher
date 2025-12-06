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
	// Создаем контекст с таймаутом
	ctx, cancel := context.WithTimeout(context.Background(), NetworkRequestTimeout)
	defer cancel()

	// Используем универсальный HTTP клиент
	client := createHTTPClient(NetworkRequestTimeout)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set user agent to avoid blocking
	req.Header.Set("User-Agent", "singbox-launcher/1.0")

	resp, err := client.Do(req)
	if err != nil {
		// Проверяем тип ошибки
		if IsNetworkError(err) {
			return nil, fmt.Errorf("network error: %s", GetNetworkErrorMessage(err))
		}
		return nil, fmt.Errorf("failed to fetch subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subscription server returned status %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read subscription content: %w", err)
	}

	// Check if content is empty
	if len(content) == 0 {
		return nil, fmt.Errorf("subscription returned empty content")
	}

	// Decode base64 if needed
	decoded, err := DecodeSubscriptionContent(content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode subscription content: %w", err)
	}

	return decoded, nil
}

// ParserConfig represents the configuration structure from @ParcerConfig block
// Supports both version 1 (legacy) and version 2 (with parser settings)
type ParserConfig struct {
	// Version 1 structure (legacy support)
	Version      int `json:"version,omitempty"`
	ParserConfig struct {
		// Version 2: version moved inside ParserConfig
		Version   int                `json:"version,omitempty"`
		Proxies   []ProxySource      `json:"proxies"`
		Outbounds []OutboundConfig   `json:"outbounds"`
		Parser    struct {
			Reload      string `json:"reload,omitempty"`      // Интервал автоматического обновления
			LastUpdated string `json:"last_updated,omitempty"` // Время последнего обновления (RFC3339, UTC)
		} `json:"parser,omitempty"`
	} `json:"ParserConfig"`
}

// ParserConfigVersion is the current version of ParserConfig format
const ParserConfigVersion = 2

// ProxySource represents a proxy subscription source
type ProxySource struct {
	Source string              `json:"source"`
	Skip   []map[string]string `json:"skip,omitempty"`
}

// OutboundConfig represents an outbound selector configuration
type OutboundConfig struct {
	Tag       string                 `json:"tag"`
	Type      string                 `json:"type"`
	Options   map[string]interface{} `json:"options,omitempty"`
	Outbounds struct {
		Proxies          map[string]interface{} `json:"proxies,omitempty"`
		AddOutbounds     []string               `json:"addOutbounds,omitempty"`
		PreferredDefault map[string]interface{} `json:"preferredDefault,omitempty"`
	} `json:"outbounds,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// ExtractParcerConfig extracts the @ParcerConfig block from config.json
// Returns the parsed ParserConfig structure and error if extraction or parsing fails
func ExtractParcerConfig(configPath string) (*ParserConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config.json: %w", err)
	}

	// Find the @ParcerConfig block using regex
	// Pattern matches: /** @ParcerConfig ... */
	pattern := regexp.MustCompile(`/\*\*\s*@ParcerConfig\s*\n([\s\S]*?)\*/`)
	matches := pattern.FindSubmatch(data)

	if len(matches) < 2 {
		return nil, fmt.Errorf("@ParcerConfig block not found in config.json")
	}

	// Extract the JSON content from the comment block
	jsonContent := strings.TrimSpace(string(matches[1]))

	// Parse the JSON
	var parserConfig ParserConfig
	if err := json.Unmarshal([]byte(jsonContent), &parserConfig); err != nil {
		return nil, fmt.Errorf("failed to parse @ParcerConfig JSON: %w", err)
	}

	// Backward compatibility: if version is at top level (version 1), migrate to version 2
	if parserConfig.Version > 0 && parserConfig.ParserConfig.Version == 0 {
		log.Printf("ExtractParcerConfig: Detected version 1 format, migrating to version 2")
		parserConfig.ParserConfig.Version = parserConfig.Version
		parserConfig.Version = 0 // Clear top-level version
	}

	// If no version specified, set to current version
	if parserConfig.ParserConfig.Version == 0 {
		parserConfig.ParserConfig.Version = ParserConfigVersion
		log.Printf("ExtractParcerConfig: No version specified, defaulting to version %d", ParserConfigVersion)
	}

	log.Printf("ExtractParcerConfig: Successfully extracted @ParcerConfig (version %d) with %d proxy sources and %d outbounds",
		parserConfig.ParserConfig.Version,
		len(parserConfig.ParserConfig.Proxies),
		len(parserConfig.ParserConfig.Outbounds))

	return &parserConfig, nil
}

// UpdateLastUpdatedInConfig updates the last_updated field in the @ParcerConfig block
func UpdateLastUpdatedInConfig(configPath string, lastUpdated time.Time) error {
	log.Printf("UpdateLastUpdatedInConfig: Updating last_updated to %s", lastUpdated.Format(time.RFC3339))

	// Read config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Find the @ParcerConfig block using regex
	pattern := regexp.MustCompile(`(/\*\*\s*@ParcerConfig\s*\n)([\s\S]*?)(\*/)`)
	matches := pattern.FindSubmatch(data)

	if len(matches) < 4 {
		return fmt.Errorf("@ParcerConfig block not found in config.json")
	}

	// Extract the JSON content from the comment block
	jsonContent := strings.TrimSpace(string(matches[2]))

	// Parse the JSON
	var parserConfig ParserConfig
	if err := json.Unmarshal([]byte(jsonContent), &parserConfig); err != nil {
		return fmt.Errorf("failed to parse @ParcerConfig JSON: %w", err)
	}

	// Backward compatibility: migrate version 1 to version 2 if needed
	if parserConfig.Version > 0 && parserConfig.ParserConfig.Version == 0 {
		parserConfig.ParserConfig.Version = parserConfig.Version
		parserConfig.Version = 0
	}

	// Ensure version is set
	if parserConfig.ParserConfig.Version == 0 {
		parserConfig.ParserConfig.Version = ParserConfigVersion
	}

	// Update last_updated field (create parser object if it doesn't exist)
	parserConfig.ParserConfig.Parser.LastUpdated = lastUpdated.Format(time.RFC3339)

	// Serialize back to JSON with indentation
	// Wrap ParserConfig in outer object for version 2 format
	outerJSON := map[string]interface{}{
		"ParserConfig": parserConfig.ParserConfig,
	}
	finalJSON, err := json.MarshalIndent(outerJSON, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal outer @ParcerConfig: %w", err)
	}

	newBlock := string(matches[1]) + string(finalJSON) + "\n" + string(matches[3])

	// Replace the block in the file
	newContent := pattern.ReplaceAll(data, []byte(newBlock))

	// Write to file
	if err := os.WriteFile(configPath, newContent, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	log.Printf("UpdateLastUpdatedInConfig: Successfully updated last_updated to %s", parserConfig.ParserConfig.Parser.LastUpdated)
	return nil
}
