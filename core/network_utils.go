package core

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"singbox-launcher/internal/urlredact"
)

const (
	// NetworkDialTimeout - таймаут на подключение к серверу
	NetworkDialTimeout = 5 * time.Second
	// NetworkRequestTimeout - таймаут на выполнение HTTP запроса
	NetworkRequestTimeout = 15 * time.Second
	// NetworkLongTimeout - таймаут для длительных операций (скачивание файлов)
	NetworkLongTimeout = 30 * time.Second
)

// defaultSharedTransport is one process-wide transport for all launcher outbound HTTP(S).
// Sharing it across CreateHTTPClient timeouts enables connection pooling, TLS session reuse,
// and fewer allocations than creating a new Transport per request.
// Safe for concurrent use (https://pkg.go.dev/net/http#Transport).
var defaultSharedTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   NetworkDialTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns:        100,
	MaxIdleConnsPerHost: 32,
	IdleConnTimeout:     90 * time.Second,
	TLSHandshakeTimeout: 10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// CreateHTTPClient returns an HTTP client using defaultSharedTransport with the given overall request timeout.
// Exported for subscription and other packages that inject CreateHTTPClientFunc.
func CreateHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: defaultSharedTransport,
	}
}

// GetURLBytes performs HTTP GET with CreateHTTPClient (proxy from environment, timeouts).
// Returns response body, HTTP status code, and an error if the request could not be sent
// or the response body could not be read.
func GetURLBytes(ctx context.Context, urlStr string, timeout time.Duration) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, 0, err
	}
	client := CreateHTTPClient(timeout)
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

// IsNetworkError проверяет, является ли ошибка сетевой ошибкой
func IsNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Проверка на timeout
	if netErr, ok := err.(net.Error); ok {
		if netErr.Timeout() {
			return true
		}
		if netErr.Temporary() {
			return true
		}
	}

	// Проверка на отсутствие соединения
	if _, ok := err.(*net.OpError); ok {
		return true
	}

	// Проверка на DNS ошибку
	if _, ok := err.(*net.DNSError); ok {
		return true
	}

	// Проверка на контекст (отмена/таймаут)
	if err == context.DeadlineExceeded || err == context.Canceled {
		return true
	}

	return false
}

// GetNetworkErrorMessage возвращает понятное сообщение об ошибке сети
func GetNetworkErrorMessage(err error) string {
	if err == nil {
		return "Unknown network error"
	}

	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return "Network timeout: connection timed out"
	}

	if opErr, ok := err.(*net.OpError); ok {
		if opErr.Op == "dial" {
			return "Network error: cannot connect to server"
		}
		return fmt.Sprintf("Network error: %s", urlredact.RedactURLUserinfo(opErr.Error()))
	}

	if dnsErr, ok := err.(*net.DNSError); ok {
		return fmt.Sprintf("DNS error: cannot resolve hostname (%s)", dnsErr.Name)
	}

	if err == context.DeadlineExceeded {
		return "Request timeout: operation took too long"
	}

	if err == context.Canceled {
		return "Request canceled"
	}

	return fmt.Sprintf("Network error: %s", urlredact.RedactURLUserinfo(err.Error()))
}
