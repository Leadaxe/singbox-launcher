package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"singbox-launcher/internal/debuglog"
)

// PingTestEndpoint describes a single HTTP endpoint that Clash uses
// for delay measurement via /proxies/{name}/delay (url query param).
type PingTestEndpoint struct {
	Title string
	URL   string
}

// Default endpoints for ping delay measurement (Clash /proxies/{name}/delay url param).
// Titles are used in the UI; URLs are passed to Clash as-is.
var (
	PingTestEndpointGStatic = PingTestEndpoint{
		Title: "GStatic",
		URL:   "http://www.gstatic.com/generate_204",
	}
	PingTestEndpointGoogle = PingTestEndpoint{
		Title: "Google",
		URL:   "https://www.google.com/generate_204",
	}
	PingTestEndpointGosuslugi = PingTestEndpoint{
		Title: "Gosuslugi",
		URL:   "https://gosuslugi.ru/favicon.ico",
	}
	PingTestEndpointYaStaticICO = PingTestEndpoint{
		Title: "YaStatic",
		URL:   "https://yastatic.net/s3/home-misc/favicon.ico",
	}
)

// pingTestURL is the current endpoint used for delay checks.
// It is process-wide and can be overridden at runtime from the UI.
var pingTestURL = PingTestEndpointGoogle.URL

// pingTestAllConcurrency is the worker count for bulk "ping all" on the Servers tab (see UI options).
var pingTestAllConcurrency = 20

func normalizePingTestAllConcurrency(n int) int {
	switch n {
	case 1, 5, 10, 20, 50, 100:
		return n
	default:
		return 20
	}
}

// GetPingTestAllConcurrency returns the number of parallel delay requests for ping-all.
func GetPingTestAllConcurrency() int {
	return pingTestAllConcurrency
}

// SetPingTestAllConcurrency sets parallel workers for ping-all; invalid values become 20.
func SetPingTestAllConcurrency(n int) {
	pingTestAllConcurrency = normalizePingTestAllConcurrency(n)
}

// GetPingTestURL returns the current endpoint used for delay checks.
func GetPingTestURL() string {
	return pingTestURL
}

// SetPingTestURL sets the endpoint used for delay checks.
// If url is empty or only whitespace, it falls back to PingTestEndpointGoogle.URL.
func SetPingTestURL(url string) {
	if strings.TrimSpace(url) == "" {
		pingTestURL = PingTestEndpointGoogle.URL
		return
	}
	pingTestURL = url
}

// GetDelay asks Clash to measure latency for the specified proxy node (GetPingTestURL). Returns ErrPlatformInterrupt when the system is sleeping or context is cancelled.
func GetDelay(baseURL, token, proxyName string) (int64, error) {
	ctx, err := requestContext()
	if err != nil {
		return 0, err
	}
	logMessage := fmt.Sprintf("[%s] GET /proxies/%s/delay request started.\n", time.Now().Format("2006-01-02 15:04:05"), proxyName)
	writeLog(debuglog.LevelVerbose, "%s", logMessage)

	encName := url.PathEscape(proxyName)
	delayURL := fmt.Sprintf("%s/proxies/%s/delay?timeout=5000&url=%s", baseURL, encName, url.QueryEscape(GetPingTestURL()))
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(httpRequestTimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "GET", delayURL, nil)
	if err != nil {
		writeLog(debuglog.LevelInfo, "[%s] Error creating delay request for %s: %v\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, err)
		return 0, fmt.Errorf("failed to create delay request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := getHTTPClient().Do(req)
	defer func() {
		if resp != nil {
			debuglog.RunAndLog("GetDelay: close response body", resp.Body.Close)
		}
	}()
	if err != nil {
		writeLog(debuglog.LevelInfo, "[%s] Error executing delay request for %s: %v\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, err)
		return 0, classifyRequestError(err, "failed to execute delay request: %w")
	}

	writeLog(debuglog.LevelVerbose, "[%s] GET /proxies/%s/delay response status: %d\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		writeLog(debuglog.LevelInfo, "[%s] Unexpected status code for delay %s: %d, body: %s\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, resp.StatusCode, string(bodyBytes))
		return 0, fmt.Errorf("unexpected status code for delay: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeLog(debuglog.LevelInfo, "[%s] Error reading response body for delay %s: %v\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, err)
		return 0, fmt.Errorf("failed to read response body for delay: %w", err)
	}

	writeLog(debuglog.LevelTrace, "[%s] GET /proxies/%s/delay response body: %s\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, string(body))

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		writeLog(debuglog.LevelInfo, "[%s] Error unmarshalling JSON for delay %s: %v\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, err)
		return 0, fmt.Errorf("failed to unmarshal JSON for delay: %w", err)
	}

	delay, ok := data["delay"].(float64)
	if !ok {
		writeLog(debuglog.LevelInfo, "[%s] Unexpected response structure for delay %s, 'delay' field missing or wrong type\n", time.Now().Format("2006-01-02 15:04:05"), proxyName)
		return 0, fmt.Errorf("unexpected response structure, 'delay' field missing or wrong type")
	}

	writeLog(debuglog.LevelVerbose, "[%s] Successfully got delay for %s: %d ms.\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, int64(delay))

	return int64(delay), nil
}
