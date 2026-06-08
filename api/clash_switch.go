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
		return classifyRequestError(err, "failed to execute switch request: %w")
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
