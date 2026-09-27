package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"singbox-launcher/internal/debuglog"
)

// FlushDNSCache очищает кэш DNS работающего ядра: POST /cache/dns/flush
// Clash API (SPEC 147). Ядро вызывает DNSRouter.ClearCache: чистит кэш в
// памяти и, если включено store_dns, удаляет бакет dns_cache из cache.db.
// Выделения FakeIP этот вызов не трогает.
func FlushDNSCache(baseURL, token string) error {
	ctx, err := requestContext()
	if err != nil {
		return err
	}
	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(httpRequestTimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/cache/dns/flush", nil)
	if err != nil {
		return fmt.Errorf("failed to create DNS flush request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := getHTTPClient().Do(req)
	if err != nil {
		writeLog(debuglog.LevelInfo, "[%s] Error executing DNS flush request: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
		return classifyRequestError(err, "failed to execute DNS flush request: %w")
	}
	defer debuglog.RunAndLog("FlushDNSCache: close response body", resp.Body.Close)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code for DNS flush: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}
	writeLog(debuglog.LevelVerbose, "[%s] POST /cache/dns/flush: DNS cache cleared\n", time.Now().Format("2006-01-02 15:04:05"))
	return nil
}
