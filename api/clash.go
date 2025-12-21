package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/muhammadmuzzammil1998/jsonc"
)

// LoadClashAPIConfig reads the Clash API URL and token from the sing-box config.json
func LoadClashAPIConfig(configPath string) (baseURL, token string, err error) {
	// Internal function to strip comments.
	stripComments := func(data []byte) []byte {
		commentRegex := regexp.MustCompile(`(?m)\s+//.*$|/\*[\s\S]*?\*/`)
		var clean = commentRegex.ReplaceAll(data, nil)
		emptyLineRegex := regexp.MustCompile(`(?m)^\s*\n`)
		return emptyLineRegex.ReplaceAll(clean, nil)
	}
	removeTrailingCommas := func(data []byte) []byte {
		re := regexp.MustCompile(`,(\s*[\]\}])`)
		return re.ReplaceAll(data, []byte("$1"))
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		log.Printf("LoadClashAPIConfig: Failed to read config.json: %v", err)
		return "", "", fmt.Errorf("failed to read config.json: %w", err)
	}
	// Convert JSONC (with comments/trailing commas) into clean JSON.
	cleanData := jsonc.ToJSON(data)
	cleanData = removeTrailingCommas(stripComments(cleanData))

	var jsonData map[string]interface{}
	if err := json.Unmarshal(cleanData, &jsonData); err != nil {
		log.Printf("LoadClashAPIConfig: Failed to parse JSON: %v", err)
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

	log.Printf("Clash API loaded from config: %s / %s", baseURL, token)
	return baseURL, token, nil
}

const (
	httpDialTimeoutSeconds    = 5
	httpRequestTimeoutSeconds = 20 // Increased to 20 seconds for better reliability
)

// Global HTTP client with timeout for all HTTP requests
var httpClient = &http.Client{
	Timeout: time.Duration(httpRequestTimeoutSeconds) * time.Second,
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: time.Duration(httpDialTimeoutSeconds) * time.Second,
		}).DialContext,
	},
}

// TestAPIConnection attempts to connect to the Clash API.
func TestAPIConnection(baseURL, token string, logFile *os.File) error {
	logMessage := fmt.Sprintf("[%s] GET /version request started for API test.\n", time.Now().Format("2006-01-02 15:04:05"))
	if logFile != nil {
		fmt.Fprint(logFile, logMessage)
	}

	url := fmt.Sprintf("%s/version", baseURL)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(httpRequestTimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] Error creating API test request: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
		}
		return fmt.Errorf("failed to create API test request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] Error executing API test request: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
		}
		// Проверяем тип ошибки для более понятного сообщения
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return fmt.Errorf("network timeout: connection timed out")
		}
		if opErr, ok := err.(*net.OpError); ok && opErr.Op == "dial" {
			return fmt.Errorf("network error: cannot connect to server")
		}
		return fmt.Errorf("failed to execute API test request: %w", err)
	}
	defer resp.Body.Close()

	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] GET /version response status for API test: %d\n", time.Now().Format("2006-01-02 15:04:05"), resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] Unexpected status code for API test: %d, body: %s\n", time.Now().Format("2006-01-02 15:04:05"), resp.StatusCode, string(bodyBytes))
		}
		return fmt.Errorf("unexpected status code for API test: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}
	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] Clash API connection successful.\n", time.Now().Format("2006-01-02 15:04:05"))
	}
	return nil
}

// ProxyInfo holds the proxy name and traffic usage.
type ProxyInfo struct {
	Name    string
	Traffic [2]int64 // [up, down]
	Delay   int64    // Last known delay in ms
}

// GetProxiesInGroup retrieves proxies from a group, their traffic stats, and last delay from the Clash API.
func GetProxiesInGroup(baseURL, token, groupName string, logFile *os.File) ([]ProxyInfo, string, error) {
	// --- Helper function for logging ---
	logMsg := func(format string, a ...interface{}) {
		if logFile != nil {
			timestamp := time.Now().Format("2006-01-02 15:04:05")
			fmt.Fprintf(logFile, "[%s] "+format+"\n", append([]interface{}{timestamp}, a...)...)
		}
	}

	logMsg("GetProxiesInGroup: Starting request for group '%s'", groupName)

	url := fmt.Sprintf("%s/proxies", baseURL)
	logMsg("GetProxiesInGroup: Request URL: %s", url)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(httpRequestTimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		logMsg("GetProxiesInGroup: ERROR: Failed to create request: %v", err)
		return nil, "", fmt.Errorf("failed to create /proxies request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		logMsg("GetProxiesInGroup: ERROR: Failed to execute request: %v", err)
		// Проверяем тип ошибки для более понятного сообщения
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return nil, "", fmt.Errorf("network timeout: connection timed out")
		}
		if opErr, ok := err.(*net.OpError); ok && opErr.Op == "dial" {
			return nil, "", fmt.Errorf("network error: cannot connect to server")
		}
		return nil, "", fmt.Errorf("failed to execute /proxies request: %w", err)
	}
	defer resp.Body.Close()

	logMsg("GetProxiesInGroup: Response status: %s", resp.Status)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logMsg("GetProxiesInGroup: ERROR: Failed to read response body: %v", err)
		return nil, "", fmt.Errorf("failed to read /proxies response: %w", err)
	}

	logMsg("GetProxiesInGroup: Raw response body:\n%s", string(body))

	// Проверяем статус-код перед парсингом JSON
	if resp.StatusCode != http.StatusOK {
		var errorResp map[string]interface{}
		if err := json.Unmarshal(body, &errorResp); err == nil {
			if message, ok := errorResp["message"].(string); ok {
				logMsg("GetProxiesInGroup: ERROR: API returned error: %s (status: %d)", message, resp.StatusCode)
				return nil, "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, message)
			}
		}
		logMsg("GetProxiesInGroup: ERROR: Unexpected status code: %d, body: %s", resp.StatusCode, string(body))
		return nil, "", fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// Теперь безопасно парсим успешный ответ
	var raw map[string]map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		logMsg("GetProxiesInGroup: ERROR: Failed to unmarshal JSON: %v", err)
		return nil, "", fmt.Errorf("failed to unmarshal /proxies response: %w", err)
	}

	proxiesMap, ok := raw["proxies"]
	if !ok {
		logMsg("GetProxiesInGroup: ERROR: 'proxies' key not found in the response.")
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
		logMsg("GetProxiesInGroup: ERROR: Proxy group '%s' not found. Available groups: %v", groupName, availableGroups)
		return nil, "", fmt.Errorf("proxy group '%s' not found", groupName)
	}

	rawList, ok := group["all"].([]interface{})
	if !ok {
		logMsg("GetProxiesInGroup: ERROR: Invalid or missing 'all' field for group '%s'", groupName)
		return nil, "", fmt.Errorf("invalid or missing 'all' field for group %s", groupName)
	}

	nowProxy, _ := group["now"].(string)
	logMsg("GetProxiesInGroup: Current active proxy in group '%s' is '%s'", groupName, nowProxy)

	var proxies []ProxyInfo
	for _, v := range rawList {
		name, ok := v.(string)
		if !ok {
			continue
		}
		pi := ProxyInfo{Name: name}
		if node, ok := proxiesMap[name].(map[string]interface{}); ok {
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

	sort.Slice(proxies, func(i, j int) bool {
		return proxies[i].Name < proxies[j].Name
	})

	logMsg("GetProxiesInGroup: Successfully parsed %d proxies from group '%s'.", len(proxies), groupName)
	return proxies, nowProxy, nil
}

// SwitchProxy switches the active proxy within the specified group.
func SwitchProxy(baseURL, token, group, proxy string, logFile *os.File) error {
	payloadStr := fmt.Sprintf("{\"name\":\"%s\"}", proxy)
	logMessage := fmt.Sprintf("[%s] PUT /proxies/%s request started with payload: %s\n", time.Now().Format("2006-01-02 15:04:05"), group, payloadStr)
	if logFile != nil {
		fmt.Fprint(logFile, logMessage)
	}

	url := fmt.Sprintf("%s/proxies/%s", baseURL, group)
	payload := strings.NewReader(payloadStr)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(httpRequestTimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "PUT", url, payload)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] Error creating switch request for %s/%s: %v\n", time.Now().Format("2006-01-02 15:04:05"), group, proxy, err)
		}
		return fmt.Errorf("failed to create switch request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] Error executing switch request for %s/%s: %v\n", time.Now().Format("2006-01-02 15:04:05"), group, proxy, err)
		}
		// Проверяем тип ошибки для более понятного сообщения
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return fmt.Errorf("network timeout: connection timed out")
		}
		if opErr, ok := err.(*net.OpError); ok && opErr.Op == "dial" {
			return fmt.Errorf("network error: cannot connect to server")
		}
		return fmt.Errorf("failed to execute switch request: %w", err)
	}
	defer resp.Body.Close()

	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] PUT /proxies/%s response status: %d\n", time.Now().Format("2006-01-02 15:04:05"), group, resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] Unexpected status code for switch %s/%s: %d, body: %s\n", time.Now().Format("2006-01-02 15:04:05"), group, proxy, resp.StatusCode, string(bodyBytes))
		}
		return fmt.Errorf("unexpected status code for switch: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}
	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] Successfully switched group '%s' to '%s'.\n", time.Now().Format("2006-01-02 15:04:05"), group, proxy)
	}
	return nil
}

// GetDelay gets the delay for the specified proxy node.
func GetDelay(baseURL, token, proxyName string, logFile *os.File) (int64, error) {
	logMessage := fmt.Sprintf("[%s] GET /proxies/%s/delay request started.\n", time.Now().Format("2006-01-02 15:04:05"), proxyName)
	if logFile != nil {
		fmt.Fprint(logFile, logMessage)
	}

	url := fmt.Sprintf("%s/proxies/%s/delay?timeout=5000&url=http://www.gstatic.com/generate_204", baseURL, proxyName)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(httpRequestTimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] Error creating delay request for %s: %v\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, err)
		}
		return 0, fmt.Errorf("failed to create delay request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := httpClient.Do(req)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] Error executing delay request for %s: %v\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, err)
		}
		// Проверяем тип ошибки для более понятного сообщения
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return 0, fmt.Errorf("network timeout: connection timed out")
		}
		if opErr, ok := err.(*net.OpError); ok && opErr.Op == "dial" {
			return 0, fmt.Errorf("network error: cannot connect to server")
		}
		return 0, fmt.Errorf("failed to execute delay request: %w", err)
	}
	defer resp.Body.Close()

	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] GET /proxies/%s/delay response status: %d\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] Unexpected status code for delay %s: %d, body: %s\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, resp.StatusCode, string(bodyBytes))
		}
		return 0, fmt.Errorf("unexpected status code for delay: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] Error reading response body for delay %s: %v\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, err)
		}
		return 0, fmt.Errorf("failed to read response body for delay: %w", err)
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] GET /proxies/%s/delay response body: %s\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, string(body))
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] Error unmarshalling JSON for delay %s: %v\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, err)
		}
		return 0, fmt.Errorf("failed to unmarshal JSON for delay: %w", err)
	}

	delay, ok := data["delay"].(float64)
	if !ok {
		if logFile != nil {
			fmt.Fprintf(logFile, "[%s] Unexpected response structure for delay %s, 'delay' field missing or wrong type\n", time.Now().Format("2006-01-02 15:04:05"), proxyName)
		}
		return 0, fmt.Errorf("unexpected response structure, 'delay' field missing or wrong type")
	}

	if logFile != nil {
		fmt.Fprintf(logFile, "[%s] Successfully got delay for %s: %d ms.\n", time.Now().Format("2006-01-02 15:04:05"), proxyName, int64(delay))
	}

	return int64(delay), nil
}
