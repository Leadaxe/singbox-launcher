package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"singbox-launcher/internal/debuglog"
)

const (
	httpDialTimeoutSeconds    = 5
	httpRequestTimeoutSeconds = 20 // Increased to 20 seconds for better reliability
)

// httpIdleConnTimeout limits connection reuse; avoids stale connections after sleep/hibernation.
const httpIdleConnTimeoutSec = 30

// clashHTTPClient creates a new HTTP client for Clash API with timeouts and idle connection limit.
// Used at init and when resetting transport after system resume (Windows sleep/hibernation).
func clashHTTPClient() *http.Client {
	return &http.Client{
		Timeout: time.Duration(httpRequestTimeoutSeconds) * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: time.Duration(httpDialTimeoutSeconds) * time.Second,
			}).DialContext,
			IdleConnTimeout: httpIdleConnTimeoutSec * time.Second,
		},
	}
}

var (
	httpClientMu sync.Mutex
	httpClient   = clashHTTPClient()
)

// getHTTPClient returns the current Clash API HTTP client (safe for concurrent use).
func getHTTPClient() *http.Client {
	httpClientMu.Lock()
	defer httpClientMu.Unlock()
	return httpClient
}

// ResetClashHTTPTransport replaces the global Clash API HTTP client with a new one and closes
// idle connections of the old transport. Call after system resume from sleep/hibernation
// so that stale TCP connections are not reused.
func ResetClashHTTPTransport() {
	httpClientMu.Lock()
	old := httpClient
	httpClient = clashHTTPClient()
	httpClientMu.Unlock()
	if old != nil && old.Transport != nil {
		if t, ok := old.Transport.(*http.Transport); ok {
			t.CloseIdleConnections()
		}
	}
}

// TestAPIConnection attempts to connect to the Clash API. Aborts with ErrPlatformInterrupt when the system is sleeping or context is cancelled.
func TestAPIConnection(baseURL, token string) error {
	ctx, err := requestContext()
	if err != nil {
		return err
	}
	logMessage := fmt.Sprintf("[%s] GET /version request started for API test.\n", time.Now().Format("2006-01-02 15:04:05"))
	writeLog(debuglog.LevelVerbose, "%s", logMessage)

	url := fmt.Sprintf("%s/version", baseURL)
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(httpRequestTimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		writeLog(debuglog.LevelInfo, "[%s] Error creating API test request: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
		return fmt.Errorf("failed to create API test request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := getHTTPClient().Do(req)
	defer func() {
		if resp != nil {
			debuglog.RunAndLog("TestAPIConnection: close response body", resp.Body.Close)
		}
	}()
	if err != nil {
		writeLog(debuglog.LevelInfo, "[%s] Error executing API test request: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
		return classifyRequestError(err, "failed to execute API test request: %w")
	}

	writeLog(debuglog.LevelVerbose, "[%s] GET /version response status for API test: %d\n", time.Now().Format("2006-01-02 15:04:05"), resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		writeLog(debuglog.LevelInfo, "[%s] Unexpected status code for API test: %d, body: %s\n", time.Now().Format("2006-01-02 15:04:05"), resp.StatusCode, string(bodyBytes))
		return fmt.Errorf("unexpected status code for API test: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}
	writeLog(debuglog.LevelVerbose, "[%s] Clash API connection successful.\n", time.Now().Format("2006-01-02 15:04:05"))
	return nil
}
