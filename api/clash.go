package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/textnorm"
	"singbox-launcher/internal/urlredact"

	"github.com/muhammadmuzzammil1998/jsonc"
)

// ErrPlatformInterrupt is returned when a request is aborted due to system sleep (platform cancelled the context).
var ErrPlatformInterrupt = errors.New("platform: interrupt")

// LoadClashAPIConfig reads the Clash API URL and token from the sing-box config.json
func LoadClashAPIConfig(configPath string) (baseURL, token string, err error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		// Cold-start logging: на свежей инсталляции config.json ещё не существует
		// (пользователь не нажал Save → не пересобирали через RunParser). Это не
		// ошибка приложения, а ожидаемое первое-запуск состояние. Логируем как
		// DEBUG, чтобы не пугать пользователя красным ERROR в логах. Все callers
		// обрабатывают возвращаемую error как «нет clash API» и продолжают работу.
		if os.IsNotExist(err) {
			debuglog.DebugLog("LoadClashAPIConfig: config.json not present yet (cold start): %v", err)
		} else {
			debuglog.ErrorLog("LoadClashAPIConfig: Failed to read config.json: %v", err)
		}
		return "", "", fmt.Errorf("failed to read config.json: %w", err)
	}
	cleanData := jsonc.ToJSON(data)

	var jsonData map[string]interface{}
	if err := json.Unmarshal(cleanData, &jsonData); err != nil {
		debuglog.ErrorLog("LoadClashAPIConfig: Failed to parse JSON: %v", err)
		return "", "", fmt.Errorf("failed to parse JSON: %w", err)
	}

	exp, ok := jsonData["experimental"].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("no 'experimental' section found in config.json")
	}
	api, ok := exp["clash_api"].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("no 'clash_api' section found under 'experimental' in config.json")
	}

	host, _ := api["external_controller"].(string)
	secret, _ := api["secret"].(string)

	if host == "" || secret == "" {
		return "", "", fmt.Errorf("'external_controller' or 'secret' is empty in Clash API config")
	}

	baseURL = "http://" + host
	token = secret

	debuglog.DebugLog("Clash API loaded from config: %s / token=%s", baseURL, urlredact.RedactToken(token))
	return baseURL, token, nil
}

const (
	httpDialTimeoutSeconds    = 5
	httpRequestTimeoutSeconds = 20 // Increased to 20 seconds for better reliability
)

// httpIdleConnTimeout limits connection reuse; avoids stale connections after sleep/hibernation.
const httpIdleConnTimeoutSec = 30

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

// apiLogFile is the target for API request logging (api.log). Set via SetAPILogFile.
var apiLogFile *os.File

var (
	apiLogSinkMu sync.RWMutex
	apiLogSink   func(debuglog.Level, string)
)

// SetAPILogFile sets the log file for API requests. Call after opening log files, pass nil before closing.
func SetAPILogFile(f *os.File) {
	apiLogFile = f
}

// SetAPILogSink sets an optional callback for the diagnostics log viewer (API tab).
// The callback receives (level, line) for each writeLog() and must not block.
// Call ClearAPILogSink when the log viewer window is closed.
func SetAPILogSink(fn func(debuglog.Level, string)) {
	apiLogSinkMu.Lock()
	defer apiLogSinkMu.Unlock()
	apiLogSink = fn
}

// ClearAPILogSink removes the API log sink (e.g. when the log viewer is closed).
func ClearAPILogSink() {
	SetAPILogSink(nil)
}

// writeLog writes to api.log when level <= GlobalLevel (same rule as debuglog.Log).
func writeLog(level debuglog.Level, format string, args ...interface{}) {
	if level > debuglog.GlobalLevel {
		return
	}
	line := fmt.Sprintf(format, args...)
	if apiLogFile != nil {
		_, _ = fmt.Fprintf(apiLogFile, format, args...)
	}
	apiLogSinkMu.RLock()
	fn := apiLogSink
	apiLogSinkMu.RUnlock()
	if fn != nil {
		fn(level, line)
	}
}

// requestContext returns the platform power context for an outgoing request, or ErrPlatformInterrupt if the system is sleeping.
func requestContext() (context.Context, error) {
	if platform.IsSleeping() {
		return nil, ErrPlatformInterrupt
	}
	return platform.PowerContext(), nil
}

// normalizeRequestError maps context.Canceled (e.g. sleep) to ErrPlatformInterrupt; other errors unchanged.
func normalizeRequestError(err error) error {
	if err != nil && errors.Is(err, context.Canceled) {
		return ErrPlatformInterrupt
	}
	return err
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
		if e := normalizeRequestError(err); e != err {
			return e
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return fmt.Errorf("network timeout: connection timed out")
		}
		if opErr, ok := err.(*net.OpError); ok && opErr.Op == "dial" {
			return fmt.Errorf("network error: cannot connect to server")
		}
		return fmt.Errorf("failed to execute API test request: %w", err)
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

// ProxyInfo holds the proxy name and traffic usage.
type ProxyInfo struct {
	// Name is the exact tag from the Clash API (used for /proxies/... requests).
	Name string
	// DisplayName is normalized for UI (UTF-8 repair, angle quotes → " > "). Empty means callers may fall back to Name.
	DisplayName string
	// ClashType is the proxy "type" from GET /proxies (e.g. Selector, URLTest, VLESS). Empty if the API omits it.
	ClashType string
	Traffic   [2]int64 // [up, down]
	Delay     int64    // Last known delay in ms
}

// DisplayOrName returns DisplayName when set, otherwise a normalized form of Name for UI.
func (p ProxyInfo) DisplayOrName() string {
	if p.DisplayName != "" {
		return p.DisplayName
	}
	d := textnorm.NormalizeProxyDisplay(p.Name)
	if d == "" {
		return p.Name
	}
	return d
}

// ContextMenuTypeLine returns ClashType in lowercase for the Servers tab context menu, or unknownLabel if empty.
func (p ProxyInfo) ContextMenuTypeLine(unknownLabel string) string {
	t := strings.TrimSpace(p.ClashType)
	if t == "" {
		return unknownLabel
	}
	return strings.ToLower(t)
}

// GetProxiesInGroup retrieves proxies from a group, their traffic stats, and last delay from the Clash API. Returns ErrPlatformInterrupt when the system is sleeping or context is cancelled.
func GetProxiesInGroup(baseURL, token, groupName string) ([]ProxyInfo, string, error) {
	ctx, err := requestContext()
	if err != nil {
		return nil, "", err
	}
	logMsg := func(level debuglog.Level, format string, a ...interface{}) {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		writeLog(level, "[%s] "+format+"\n", append([]interface{}{timestamp}, a...)...)
	}

	logMsg(debuglog.LevelVerbose, "GetProxiesInGroup: Starting request for group '%s'", groupName)

	url := fmt.Sprintf("%s/proxies", baseURL)
	logMsg(debuglog.LevelTrace, "GetProxiesInGroup: Request URL: %s", url)

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(httpRequestTimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "GET", url, nil)
	if err != nil {
		logMsg(debuglog.LevelInfo, "GetProxiesInGroup: ERROR: Failed to create request: %v", err)
		return nil, "", fmt.Errorf("failed to create /proxies request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := getHTTPClient().Do(req)
	defer func() {
		if resp != nil {
			debuglog.RunAndLog("GetProxiesInGroup: close response body", resp.Body.Close)
		}
	}()
	if err != nil {
		logMsg(debuglog.LevelInfo, "GetProxiesInGroup: ERROR: Failed to execute request: %v", err)
		if e := normalizeRequestError(err); e != err {
			return nil, "", e
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, "", fmt.Errorf("network timeout: connection timed out")
		}
		if opErr, ok := err.(*net.OpError); ok && opErr.Op == "dial" {
			return nil, "", fmt.Errorf("network error: cannot connect to server")
		}
		return nil, "", fmt.Errorf("failed to execute /proxies request: %w", err)
	}

	logMsg(debuglog.LevelVerbose, "GetProxiesInGroup: Response status: %s", resp.Status)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logMsg(debuglog.LevelInfo, "GetProxiesInGroup: ERROR: Failed to read response body: %v", err)
		return nil, "", fmt.Errorf("failed to read /proxies response: %w", err)
	}

	logMsg(debuglog.LevelTrace, "GetProxiesInGroup: Raw response body:\n%s", string(body))

	// Проверяем статус-код перед парсингом JSON
	if resp.StatusCode != http.StatusOK {
		var errorResp map[string]interface{}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			if message, ok := errorResp["message"].(string); ok {
				logMsg(debuglog.LevelInfo, "GetProxiesInGroup: ERROR: API returned error: %s (status: %d)", message, resp.StatusCode)
				return nil, "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, message)
			}
		}
		logMsg(debuglog.LevelInfo, "GetProxiesInGroup: ERROR: Unexpected status code: %d, body: %s", resp.StatusCode, string(body))
		return nil, "", fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// Теперь безопасно парсим успешный ответ
	var raw map[string]map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		logMsg(debuglog.LevelInfo, "GetProxiesInGroup: ERROR: Failed to unmarshal JSON: %v", err)
		return nil, "", fmt.Errorf("failed to unmarshal /proxies response: %w", err)
	}

	proxiesMap, ok := raw["proxies"]
	if !ok {
		logMsg(debuglog.LevelInfo, "GetProxiesInGroup: ERROR: 'proxies' key not found in the response.")
		return nil, "", fmt.Errorf("'proxies' key not found in the response")
	}

	group, ok := proxiesMap[groupName].(map[string]interface{})
	if !ok {
		var availableGroups []string
		for name := range proxiesMap {
			if _, isGroup := proxiesMap[name].(map[string]interface{}); isGroup {
				availableGroups = append(availableGroups, name)
			}
		}
		logMsg(debuglog.LevelInfo, "GetProxiesInGroup: ERROR: Proxy group '%s' not found. Available groups: %v", groupName, availableGroups)
		return nil, "", fmt.Errorf("proxy group '%s' not found", groupName)
	}

	rawList, ok := group["all"].([]interface{})
	if !ok {
		logMsg(debuglog.LevelInfo, "GetProxiesInGroup: ERROR: Invalid or missing 'all' field for group '%s'", groupName)
		return nil, "", fmt.Errorf("invalid or missing 'all' field for group %s", groupName)
	}

	nowProxy, _ := group["now"].(string)
	logMsg(debuglog.LevelVerbose, "GetProxiesInGroup: Current active proxy in group '%s' is '%s'", groupName, nowProxy)

	var proxies []ProxyInfo
	for _, v := range rawList {
		name, ok := v.(string)
		if !ok {
			continue
		}
		disp := textnorm.NormalizeProxyDisplay(name)
		if disp == "" {
			disp = name
		}
		pi := ProxyInfo{Name: name, DisplayName: disp}
		if node, ok := proxiesMap[name].(map[string]interface{}); ok {
			if t, ok := node["type"].(string); ok {
				pi.ClashType = t
			}
			// Парсим трафик (остается на случай, если он появится)
			if f, ok := node["up"].(float64); ok {
				pi.Traffic[0] = int64(f)
			}
			if f, ok := node["down"].(float64); ok {
				pi.Traffic[1] = int64(f)
			}

			// ИЗМЕНЕНО: Парсим последний известный пинг из истории
			if history, ok := node["history"].([]interface{}); ok && len(history) > 0 {
				if lastCheck, ok := history[0].(map[string]interface{}); ok {
					if delay, ok := lastCheck["delay"].(float64); ok {
						pi.Delay = int64(delay)
					}
				}
			}
		}
		proxies = append(proxies, pi)
	}

	// Сортировка убрана - UI управляет сортировкой самостоятельно

	logMsg(debuglog.LevelVerbose, "GetProxiesInGroup: Successfully parsed %d proxies from group '%s'.", len(proxies), groupName)
	return proxies, nowProxy, nil
}

// SwitchProxy switches the active proxy within the specified group. Returns ErrPlatformInterrupt when the system is sleeping or context is cancelled.
func SwitchProxy(baseURL, token, group, proxy string) error {
	ctx, err := requestContext()
	if err != nil {
		return err
	}
	payloadBytes, err := json.Marshal(map[string]string{"name": proxy})
	if err != nil {
		return fmt.Errorf("failed to marshal switch payload: %w", err)
	}
	logMessage := fmt.Sprintf("[%s] PUT /proxies/%s request started with payload: %s\n", time.Now().Format("2006-01-02 15:04:05"), group, string(payloadBytes))
	writeLog(debuglog.LevelVerbose, "%s", logMessage)

	reqURL := fmt.Sprintf("%s/proxies/%s", baseURL, url.PathEscape(group))
	payload := strings.NewReader(string(payloadBytes))

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(httpRequestTimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "PUT", reqURL, payload)
	if err != nil {
		writeLog(debuglog.LevelInfo, "[%s] Error creating switch request for %s/%s: %v\n", time.Now().Format("2006-01-02 15:04:05"), group, proxy, err)
		return fmt.Errorf("failed to create switch request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := getHTTPClient().Do(req)
	defer func() {
		if resp != nil {
			debuglog.RunAndLog("SwitchProxy: close response body", resp.Body.Close)
		}
	}()
	if err != nil {
		writeLog(debuglog.LevelInfo, "[%s] Error executing switch request for %s/%s: %v\n", time.Now().Format("2006-01-02 15:04:05"), group, proxy, err)
		if e := normalizeRequestError(err); e != err {
			return e
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return fmt.Errorf("network timeout: connection timed out")
		}
		if opErr, ok := err.(*net.OpError); ok && opErr.Op == "dial" {
			return fmt.Errorf("network error: cannot connect to server")
		}
		return fmt.Errorf("failed to execute switch request: %w", err)
	}

	writeLog(debuglog.LevelVerbose, "[%s] PUT /proxies/%s response status: %d\n", time.Now().Format("2006-01-02 15:04:05"), group, resp.StatusCode)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		writeLog(debuglog.LevelInfo, "[%s] Unexpected status code for switch %s/%s: %d, body: %s\n", time.Now().Format("2006-01-02 15:04:05"), group, proxy, resp.StatusCode, string(bodyBytes))
		return fmt.Errorf("unexpected status code for switch: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}
	writeLog(debuglog.LevelVerbose, "[%s] Successfully switched group '%s' to '%s'.\n", time.Now().Format("2006-01-02 15:04:05"), group, proxy)
	return nil
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
		if e := normalizeRequestError(err); e != err {
			return 0, e
		}
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return 0, fmt.Errorf("network timeout: connection timed out")
		}
		if opErr, ok := err.(*net.OpError); ok && opErr.Op == "dial" {
			return 0, fmt.Errorf("network error: cannot connect to server")
		}
		return 0, fmt.Errorf("failed to execute delay request: %w", err)
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
