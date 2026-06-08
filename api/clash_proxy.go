package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/textnorm"
)

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
		return nil, "", classifyRequestError(err, "failed to execute /proxies request: %w")
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
