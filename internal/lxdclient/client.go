//go:build darwin

package lxdclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Config описывает подключение к демону.
type Config struct {
	// Addr — host:port управляющего канала.
	Addr string
	// ServerFingerprint — пин серверного сертификата (lowercase hex). Пусто =
	// plain h2c без TLS (dev-режим демона на loopback).
	ServerFingerprint string
	// Identity — клиентская пара для mTLS. Обязателна для всех admin/gRPC
	// вызовов при TLS, кроме Enroll (там достаточно кода).
	Identity *Identity
	// Secret — Bearer-секрет обеих плоскостей (пусто = аутентификация
	// секретом выключена на стороне демона).
	Secret string
	// AllowUnpinnedTLS — говорить TLS без пина сервера (InsecureSkipVerify).
	// Используется ТОЛЬКО для bootstrap-вызова MintInvite к локальной службе
	// на loopback: отпечатка ещё нет (он приедет в приглашении), а канал уже
	// TLS. Локальная машина = trust boundary; для удалённых демонов пин
	// обязателен.
	AllowUnpinnedTLS bool
}

// TLSEnabled — включён ли TLS-канал (пин установлен либо явно разрешён
// bootstrap без пина).
func (c Config) TLSEnabled() bool { return c.ServerFingerprint != "" || c.AllowUnpinnedTLS }

// AddrString возвращает адрес управляющего канала (для логов/статуса).
func (c *Client) AddrString() string { return c.cfg.Addr }

// Client — REST-клиент admin-плоскости + фабрика gRPC-соединений.
type Client struct {
	cfg   Config
	httpc *http.Client
}

const restTimeout = 30 * time.Second

// New создаёт клиента. Все REST-вызовы идут с общим таймаутом restTimeout;
// стримы gRPC живут на собственном соединении без таймаута.
func New(cfg Config) *Client {
	transport := &http.Transport{}
	if cfg.TLSEnabled() {
		transport.TLSClientConfig = cfg.tlsConfig()
	}
	return &Client{
		cfg:   cfg,
		httpc: &http.Client{Timeout: restTimeout, Transport: transport},
	}
}

// tlsConfig строит клиентский TLS: свой сертификат + пин сервера по
// отпечатку. Серверный сертификат демона самоподписан и без SAN, поэтому
// штатная верификация невозможна by design — доверие держится на пине
// (InsecureSkipVerify + VerifyPeerCertificate), как в модели 057 форка.
func (c Config) tlsConfig() *tls.Config {
	conf := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true,
	}
	if c.ServerFingerprint != "" {
		conf.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 {
				return fmt.Errorf("lxdclient: server presented no certificate")
			}
			got := FingerprintOf(rawCerts[0])
			if got != strings.ToLower(c.ServerFingerprint) {
				return fmt.Errorf("lxdclient: server fingerprint does not match the pinned one (got %s…)", got[:12])
			}
			return nil
		}
	}
	if c.Identity != nil {
		conf.Certificates = []tls.Certificate{c.Identity.TLSCert}
	}
	return conf
}

func (c *Client) baseURL() string {
	scheme := "http"
	if c.cfg.TLSEnabled() {
		scheme = "https"
	}
	return scheme + "://" + c.cfg.Addr
}

// --- REST admin plane ---------------------------------------------------

// StatusInfo — ответ GET /admin/status.
type StatusInfo struct {
	Status           string `json:"status"` // idle | started | fatal
	ActiveSHA        string `json:"active_sha256"`
	LastGoodSHA      string `json:"last_good_sha256"`
	LastError        string `json:"last_error"`
	InterruptedApply bool   `json:"interrupted_apply"`
}

// ApplyError — типизированная ошибка применения конфига.
type ApplyError struct {
	StatusCode int
	RolledBack bool
	Message    string
}

func (e *ApplyError) Error() string {
	if e.RolledBack {
		return fmt.Sprintf("config rejected, daemon rolled back to last-good: %s", e.Message)
	}
	return e.Message
}

// Rejected — конфиг не прошёл валидацию (422): работающий инстанс не тронут.
func (e *ApplyError) Rejected() bool { return e.StatusCode == http.StatusUnprocessableEntity }

func (c *Client) do(method, path string, body io.Reader, contentType string) (*http.Response, error) {
	req, err := http.NewRequest(method, c.baseURL()+path, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.cfg.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.Secret)
	}
	return c.httpc.Do(req)
}

func decodeError(resp *http.Response) string {
	var payload struct {
		Error string `json:"error"`
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if json.Unmarshal(data, &payload) == nil && payload.Error != "" {
		return payload.Error
	}
	return fmt.Sprintf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
}

// Status возвращает состояние демона.
func (c *Client) Status() (StatusInfo, error) {
	resp, err := c.do(http.MethodGet, "/admin/status", nil, "")
	if err != nil {
		return StatusInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return StatusInfo{}, fmt.Errorf("lxdclient: status: %s", decodeError(resp))
	}
	var info StatusInfo
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&info); err != nil {
		return StatusInfo{}, fmt.Errorf("lxdclient: status decode: %w", err)
	}
	return info, nil
}

// Apply отправляет конфиг демону: валидация сабпроцессом → подмена инстанса
// → last-good, с автооткатом на провале старта. 422 → ApplyError.Rejected().
func (c *Client) Apply(config []byte) error {
	resp, err := c.do(http.MethodPost, "/admin/apply", bytes.NewReader(config), "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var payload struct {
		Error      string `json:"error"`
		RolledBack bool   `json:"rolled_back"`
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	message := strings.TrimSpace(string(data))
	if json.Unmarshal(data, &payload) == nil && payload.Error != "" {
		message = payload.Error
	}
	return &ApplyError{StatusCode: resp.StatusCode, RolledBack: payload.RolledBack, Message: message}
}

// Start поднимает ядро из last-good (без нового конфига).
func (c *Client) Start() error { return c.simplePost("/admin/start") }

// Stop гасит ядро; демон и канал остаются жить.
func (c *Client) Stop() error { return c.simplePost("/admin/stop") }

// Rollback откатывает на last-good.
func (c *Client) Rollback() error { return c.simplePost("/admin/rollback") }

func (c *Client) simplePost(path string) error {
	resp, err := c.do(http.MethodPost, path, nil, "")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lxdclient: %s: %s", path, decodeError(resp))
	}
	return nil
}

// ActiveConfig возвращает активный конфиг демона (GET /admin/config).
func (c *Client) ActiveConfig() ([]byte, error) {
	resp, err := c.do(http.MethodGet, "/admin/config", nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("lxdclient: config: %s", decodeError(resp))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 32<<20))
}

// --- Сопряжение ---------------------------------------------------------

// Enroll регистрирует клиентский сертификат по одноразовому коду. Вызывается
// на клиенте, собранном из приглашения (пин уже установлен), до появления
// доверия — маршрут /admin/enroll защищён только кодом.
func (c *Client) Enroll(code, name string) error {
	if c.cfg.Identity == nil {
		return fmt.Errorf("lxdclient: enroll requires a client keypair")
	}
	body, err := json.Marshal(map[string]string{
		"code":     code,
		"name":     name,
		"cert_pem": string(c.cfg.Identity.CertPEM),
	})
	if err != nil {
		return err
	}
	resp, err := c.do(http.MethodPost, "/admin/enroll", bytes.NewReader(body), "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lxdclient: enroll: %s", decodeError(resp))
	}
	return nil
}

// MintInvite просит демона выдать свежее приглашение (operator-маршрут:
// loopback + Bearer-секрет, клиентский пин не требуется). Используется для
// авто-сопряжения с локально установленной службой — лаунчер сам знает секрет.
// Имя в теле — метка будущего клиента (новые демоны учитывают, старые
// игнорируют тело).
func (c *Client) MintInvite() (string, error) {
	body, _ := json.Marshal(map[string]string{"name": "singbox-launcher"})
	resp, err := c.do(http.MethodPost, "/admin/client-code", bytes.NewReader(body), "application/json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("lxdclient: client-code: %s", decodeError(resp))
	}
	var payload struct {
		Invite string `json:"invite"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&payload); err != nil {
		return "", err
	}
	return payload.Invite, nil
}

// --- gRPC plane ---------------------------------------------------------

// DialGRPC создаёт gRPC-соединение к плоскости daemon.StartedService: mTLS с
// пином (или plain h2c без TLS) + Bearer-метаданные на каждый вызов.
func (c *Client) DialGRPC() (*grpc.ClientConn, error) {
	transportCredentials := insecure.NewCredentials()
	if c.cfg.TLSEnabled() {
		transportCredentials = credentials.NewTLS(c.cfg.tlsConfig())
	}
	return grpc.NewClient(c.cfg.Addr,
		grpc.WithTransportCredentials(transportCredentials),
		grpc.WithUnaryInterceptor(c.authUnaryInterceptor()),
		grpc.WithStreamInterceptor(c.authStreamInterceptor()),
	)
}

func (c *Client) authUnaryInterceptor() grpc.UnaryClientInterceptor {
	secret := c.cfg.Secret
	return func(ctx context.Context, method string, req, reply any, conn *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if secret != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+secret)
		}
		return invoker(ctx, method, req, reply, conn, opts...)
	}
}

func (c *Client) authStreamInterceptor() grpc.StreamClientInterceptor {
	secret := c.cfg.Secret
	return func(ctx context.Context, desc *grpc.StreamDesc, conn *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		if secret != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+secret)
		}
		return streamer(ctx, desc, conn, method, opts...)
	}
}
