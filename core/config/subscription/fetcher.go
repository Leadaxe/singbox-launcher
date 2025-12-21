package subscription

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"singbox-launcher/core/config"
)

// NetworkRequestTimeout defines the timeout for network requests
const NetworkRequestTimeout = 30 * time.Second

// CreateHTTPClientFunc is a function variable that should be set to core.CreateHTTPClient
var CreateHTTPClientFunc func(timeout time.Duration) *http.Client

// IsNetworkErrorFunc is a function variable that should be set to core.IsNetworkError
var IsNetworkErrorFunc func(err error) bool

// GetNetworkErrorMessageFunc is a function variable that should be set to core.GetNetworkErrorMessage
var GetNetworkErrorMessageFunc func(err error) string

// FetchSubscription fetches subscription content from URL and decodes it
// Returns decoded content and error if fetch or decode fails
func FetchSubscription(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), NetworkRequestTimeout)
	defer cancel()

	// Use simple HTTP client if CreateHTTPClientFunc not set
	var client *http.Client
	if CreateHTTPClientFunc != nil {
		client = CreateHTTPClientFunc(NetworkRequestTimeout)
	} else {
		client = &http.Client{Timeout: NetworkRequestTimeout}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set user agent to avoid server detecting sing-box and returning JSON config
	req.Header.Set("User-Agent", config.SubscriptionUserAgent)

	resp, err := client.Do(req)
	if err != nil {
		if IsNetworkErrorFunc != nil && IsNetworkErrorFunc(err) {
			return nil, fmt.Errorf("network error: %s", GetNetworkErrorMessageFunc(err))
		}
		return nil, fmt.Errorf("failed to fetch subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subscription server returned status %d", resp.StatusCode)
	}

	// Limit response size to prevent memory exhaustion
	const maxResponseSize = 10 * 1024 * 1024 // 10 MB
	limitedReader := io.LimitReader(resp.Body, maxResponseSize+1)

	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("FetchSubscription: failed to read subscription content: %w", err)
	}

	if len(content) == 0 {
		return nil, fmt.Errorf("FetchSubscription: subscription returned empty content")
	}

	// Check if content was truncated (exceeds max size)
	if len(content) > maxResponseSize {
		return nil, fmt.Errorf("FetchSubscription: subscription content too large (exceeds %d bytes)", maxResponseSize)
	}

	// Log preview of raw content for debugging
	const previewLen = 200
	preview := string(content)
	if len(preview) > previewLen {
		preview = preview[:previewLen] + "..."
	}
	log.Printf("[DEBUG] FetchSubscription: Raw content preview (first %d bytes): %q", len(content), preview)

	// Use DecodeSubscriptionContent for decoding
	decoded, err := DecodeSubscriptionContent(content)
	if err != nil {
		return nil, fmt.Errorf("FetchSubscription: failed to decode subscription content: %w", err)
	}

	return decoded, nil
}
