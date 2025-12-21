package business

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"singbox-launcher/core/config"
	wizardutils "singbox-launcher/ui/wizard/utils"
)

// ValidateParserConfig validates ParserConfig structure and content.
func ValidateParserConfig(parserConfig *config.ParserConfig) error {
	if parserConfig == nil {
		return fmt.Errorf("ParserConfig is nil")
	}

	if parserConfig.ParserConfig.Proxies == nil {
		return fmt.Errorf("ParserConfig.Proxies is nil")
	}

	// Validate each proxy source
	for i, proxy := range parserConfig.ParserConfig.Proxies {
		if proxy.Source != "" {
			if err := ValidateURL(proxy.Source); err != nil {
				return fmt.Errorf("proxy source %d: invalid URL: %w", i, err)
			}
		}

		// Validate connections
		for j, conn := range proxy.Connections {
			if err := ValidateURI(conn); err != nil {
				return fmt.Errorf("proxy %d connection %d: invalid URI: %w", i, j, err)
			}
		}

		// Validate outbounds
		for j, outbound := range proxy.Outbounds {
			if err := ValidateOutbound(&outbound); err != nil {
				return fmt.Errorf("proxy %d outbound %d: %w", i, j, err)
			}
		}
	}

	// Validate global outbounds
	for i, outbound := range parserConfig.ParserConfig.Outbounds {
		if err := ValidateOutbound(&outbound); err != nil {
			return fmt.Errorf("global outbound %d: %w", i, err)
		}
	}

	return nil
}

// ValidateURL validates a URL string.
func ValidateURL(urlStr string) error {
	if urlStr == "" {
		return fmt.Errorf("URL is empty")
	}

	if len(urlStr) > wizardutils.MaxURILength {
		return fmt.Errorf("URL length (%d) exceeds maximum (%d)", len(urlStr), wizardutils.MaxURILength)
	}

	if len(urlStr) < wizardutils.MinURILength {
		return fmt.Errorf("URL length (%d) is less than minimum (%d)", len(urlStr), wizardutils.MinURILength)
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if parsedURL.Scheme == "" {
		return fmt.Errorf("URL must have a scheme (http, https, etc.)")
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}

// ValidateURI validates a URI string (for proxy connections).
func ValidateURI(uri string) error {
	if uri == "" {
		return fmt.Errorf("URI is empty")
	}

	if len(uri) > wizardutils.MaxURILength {
		return fmt.Errorf("URI length (%d) exceeds maximum (%d)", len(uri), wizardutils.MaxURILength)
	}

	if len(uri) < wizardutils.MinURILength {
		return fmt.Errorf("URI length (%d) is less than minimum (%d)", len(uri), wizardutils.MinURILength)
	}

	// Basic URI format check (should start with protocol)
	if !strings.Contains(uri, "://") {
		return fmt.Errorf("URI must contain protocol (e.g., vless://, vmess://)")
	}

	return nil
}

// ValidateOutbound validates an OutboundConfig.
func ValidateOutbound(outbound *config.OutboundConfig) error {
	if outbound == nil {
		return fmt.Errorf("outbound is nil")
	}

	if outbound.Tag == "" {
		return fmt.Errorf("outbound tag is empty")
	}

	if outbound.Type == "" {
		return fmt.Errorf("outbound type is empty")
	}

	// Validate tag length
	if len(outbound.Tag) > 256 {
		return fmt.Errorf("outbound tag length (%d) exceeds maximum (256)", len(outbound.Tag))
	}

	return nil
}

// ValidateRule validates a rule structure.
func ValidateRule(rule map[string]interface{}) error {
	if rule == nil {
		return fmt.Errorf("rule is nil")
	}

	// Check for required fields based on rule type
	// This is a basic validation - more specific validation can be added
	if len(rule) == 0 {
		return fmt.Errorf("rule is empty")
	}

	return nil
}

// ValidateJSONSize validates that JSON data size is within limits.
func ValidateJSONSize(jsonData []byte) error {
	if len(jsonData) > wizardutils.MaxJSONConfigSize {
		return fmt.Errorf("JSON size (%d bytes) exceeds maximum (%d bytes)", len(jsonData), wizardutils.MaxJSONConfigSize)
	}
	return nil
}

// ValidateJSON validates JSON structure and size.
func ValidateJSON(jsonData []byte) error {
	if err := ValidateJSONSize(jsonData); err != nil {
		return err
	}

	if !json.Valid(jsonData) {
		return fmt.Errorf("invalid JSON structure")
	}

	return nil
}

// ValidateHTTPResponseSize validates HTTP response size.
func ValidateHTTPResponseSize(size int64) error {
	if size > wizardutils.MaxSubscriptionSize {
		return fmt.Errorf("HTTP response size (%d bytes) exceeds maximum (%d bytes)", size, wizardutils.MaxSubscriptionSize)
	}
	return nil
}

// ValidateParserConfigJSON validates ParserConfig JSON text.
func ValidateParserConfigJSON(jsonText string) error {
	if jsonText == "" {
		return fmt.Errorf("ParserConfig JSON is empty")
	}

	jsonBytes := []byte(jsonText)
	if err := ValidateJSON(jsonBytes); err != nil {
		return fmt.Errorf("invalid ParserConfig JSON: %w", err)
	}

	var parserConfig config.ParserConfig
	if err := json.Unmarshal(jsonBytes, &parserConfig); err != nil {
		return fmt.Errorf("failed to parse ParserConfig JSON: %w", err)
	}

	return ValidateParserConfig(&parserConfig)
}

