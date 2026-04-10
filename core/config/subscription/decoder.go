package subscription

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"singbox-launcher/internal/debuglog"
)

// tryDecodeBase64 attempts to decode base64 string using multiple encoding variants
// Returns decoded bytes and source description, or error if all attempts fail
func tryDecodeBase64(s string) ([]byte, string, error) {
	// Try URL-safe base64 without padding (most common in subscriptions)
	if decoded, err := base64.URLEncoding.WithPadding(base64.NoPadding).DecodeString(s); err == nil {
		return decoded, "URL-safe base64", nil
	}

	// Try standard base64 without padding
	if decoded, err := base64.StdEncoding.WithPadding(base64.NoPadding).DecodeString(s); err == nil {
		return decoded, "Standard base64", nil
	}

	// Try URL-safe base64 with padding
	if decoded, err := base64.URLEncoding.DecodeString(s); err == nil {
		return decoded, "URL-safe base64 (with padding)", nil
	}

	// Try standard base64 with padding
	if decoded, err := base64.StdEncoding.DecodeString(s); err == nil {
		return decoded, "Standard base64 (with padding)", nil
	}

	return nil, "", fmt.Errorf("failed to decode base64")
}

// DecodeSubscriptionContent декодирует содержимое подписки (base64 или plain text).
// Возвращает декодированные байты или оригинальный контент, если это уже готовые ссылки.
func DecodeSubscriptionContent(content []byte) ([]byte, error) {
	if len(content) == 0 {
		return nil, fmt.Errorf("subscription content is empty")
	}

	contentStr := strings.TrimSpace(string(content))
	// If content is only whitespace, return original content (not an error)
	if contentStr == "" {
		return content, nil
	}

	// Try to decode as base64
	decoded, source, err := tryDecodeBase64(contentStr)
	if err == nil {
		// Validate decoded content
		if len(decoded) == 0 {
			return nil, fmt.Errorf("decoded content is empty")
		}

		if !utf8.Valid(decoded) {
			return nil, fmt.Errorf("decoded content contains invalid UTF-8 sequences")
		}

		// Count nodes (lines)
		decodedStr := string(decoded)
		lineCount := strings.Count(decodedStr, "\n")
		if lineCount == 0 || !strings.HasSuffix(decodedStr, "\n") {
			lineCount++ // Count last line if no newline or doesn't end with newline
		}

		debuglog.DebugLog("DecodeSubscriptionContent: %s: successfully decoded: %d node(s)", source, lineCount)
		return decoded, nil
	}

	// JSON array of full configs (Xray-style subscription): pass through as subscription body.
	if strings.HasPrefix(strings.TrimSpace(contentStr), "[") {
		trimmed := strings.TrimSpace(contentStr)
		if json.Valid([]byte(trimmed)) {
			var elems []json.RawMessage
			if err := json.Unmarshal([]byte(trimmed), &elems); err == nil {
				debuglog.DebugLog("DecodeSubscriptionContent: JSON array subscription (%d element(s))", len(elems))
				return []byte(trimmed), nil
			}
		}
	}

	// Single JSON object or invalid JSON array: not a supported subscription list
	if strings.HasPrefix(strings.TrimSpace(contentStr), "{") || strings.HasPrefix(strings.TrimSpace(contentStr), "[") {
		debuglog.DebugLog("DecodeSubscriptionContent: Content is JSON configuration, not a subscription list")
		return nil, fmt.Errorf("subscription URL returned JSON configuration instead of subscription list (base64 or plain text links)")
	}

	// Check if it's plain text links
	if strings.Contains(contentStr, "://") {
		debuglog.DebugLog("DecodeSubscriptionContent: Detected plain text subscription (contains '://')")
		return content, nil
	}

	return nil, fmt.Errorf("failed to decode base64 content: %w", err)
}
