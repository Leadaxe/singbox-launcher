package warp

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	mrand "math/rand"
	"net/http"
	"strings"
	"time"
)

// API endpoint version and client header. Cloudflare periodically bumps these;
// when registration starts failing with HTTP 4xx, update HERE and cross-check
// against a current wgcf / warp-cli release. Kept in one place on purpose.
const (
	apiBase          = "https://api.cloudflareclient.com"
	apiVersion       = "v0a4005"
	clientVersionHdr = "a-6.30-4005"
	userAgent        = "okhttp/3.12.1"
)

// httpDoer is the minimal HTTP surface the client needs. The launcher injects
// its shared client (proxy-from-env, timeouts, macOS TLS fix); tests inject a
// fake. Keeping the dependency to an interface keeps this package a stdlib leaf.
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client registers WARP accounts against the Cloudflare API.
type Client struct {
	http httpDoer
	rng  *mrand.Rand // for random endpoint/SNI; nil = package default source
}

// NewClient builds a Client with the given HTTP doer. A nil doer falls back to
// http.DefaultClient with a 15s timeout so callers that do not need proxy
// awareness still work.
func NewClient(doer httpDoer) *Client {
	if doer == nil {
		doer = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{http: doer}
}

// WithRand injects a deterministic random source (for tests / reproducible
// endpoint selection). Returns the client for chaining.
func (c *Client) WithRand(rng *mrand.Rand) *Client {
	c.rng = rng
	return c
}

// RegisterOptions controls a WARP registration.
type RegisterOptions struct {
	LicenseKey     string     // optional WARP+ license
	Endpoint       string     // "" = default; a non-default value is respected as-is
	Obfuscate      bool       // enable AmneziaWG obfuscation
	Quic           QuicParams // masquerade params (used when Obfuscate)
	RandomEndpoint bool       // when Obfuscate and Endpoint is default, pick from the pool
	Now            time.Time  // registration timestamp (tos); zero = time.Now()
}

// GenerateKeypair returns (privateBase64, publicBase64) for a fresh X25519 pair,
// both raw 32 bytes in standard base64 — the format Cloudflare and WireGuard
// expect. The private key stays on the machine; only the public key is sent.
func GenerateKeypair() (priv string, pub string, err error) {
	curve := ecdh.X25519()
	pk, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("warp: keygen: %w", err)
	}
	priv = base64.StdEncoding.EncodeToString(pk.Bytes())
	pub = base64.StdEncoding.EncodeToString(pk.PublicKey().Bytes())
	return priv, pub, nil
}

// Register creates a new anonymous WARP account. The private key is generated
// locally; on success the returned Account carries everything needed to emit a
// WireGuard node. A non-empty LicenseKey upgrades to WARP+ (failure is
// non-fatal: a free account is returned). Obfuscate attaches AmneziaWG fields
// (a purely client-side config; the registration request itself is unchanged).
func (c *Client) Register(ctx context.Context, opts RegisterOptions) (*Account, error) {
	priv, pub, err := GenerateKeypair()
	if err != nil {
		return nil, err
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowISO := now.UTC().Format("2006-01-02T15:04:05.000Z")

	reqBody, err := json.Marshal(map[string]string{
		"key":           pub,
		"install_id":    "",
		"fcm_token":     "",
		"tos":           nowISO,
		"model":         "PC",
		"serial_number": "",
		"locale":        "en_US",
	})
	if err != nil {
		return nil, fmt.Errorf("warp: marshal reg body: %w", err)
	}

	respBody, status, err := c.do(ctx, http.MethodPost, regURL(), "", reqBody)
	if err != nil {
		return nil, fmt.Errorf("warp: registration request: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("warp: registration failed (HTTP %d); the API version (%s) may have changed", status, apiVersion)
	}

	// Resolve the effective endpoint: a user-picked (non-default) endpoint wins;
	// otherwise, when obfuscating, optionally pick a random pool endpoint to
	// dodge the blocked default; else keep the default.
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	if opts.Obfuscate && opts.RandomEndpoint && endpoint == DefaultEndpoint {
		endpoint = RandomEndpoint(c.rng)
	}

	acc, err := parseRegistration(respBody, priv, endpoint, nowISO)
	if err != nil {
		return nil, err
	}

	if key := strings.TrimSpace(opts.LicenseKey); key != "" {
		c.applyLicenseSafe(ctx, acc, key)
	}
	if opts.Obfuscate {
		quic := opts.Quic
		if opts.RandomEndpoint && strings.TrimSpace(quic.SNI) == "" {
			quic.SNI = RandomSNI(c.rng)
		}
		acc.AWG = buildAWGFields(quic)
	}
	return acc, nil
}

// parseRegistration extracts the Account fields from a /reg response. A
// user-picked endpoint is preserved; the Cloudflare-returned peer host is used
// only when the caller left the default.
func parseRegistration(body []byte, priv, endpoint, createdAt string) (*Account, error) {
	var r struct {
		ID      string `json:"id"`
		Token   string `json:"token"`
		Account struct {
			ID          string `json:"id"`
			AccountType string `json:"account_type"`
		} `json:"account"`
		Config struct {
			ClientID  string `json:"client_id"`
			Interface struct {
				Addresses struct {
					V4 string `json:"v4"`
					V6 string `json:"v6"`
				} `json:"addresses"`
			} `json:"interface"`
			Peers []struct {
				PublicKey string `json:"public_key"`
				Endpoint  struct {
					Host string `json:"host"`
				} `json:"endpoint"`
			} `json:"peers"`
		} `json:"config"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("warp: bad response: not JSON: %w", err)
	}
	if len(r.Config.Peers) == 0 || r.Config.Peers[0].PublicKey == "" {
		return nil, fmt.Errorf("warp: bad response: missing peer public key")
	}

	acc := &Account{
		PrivateKey: priv,
		PeerPublic: r.Config.Peers[0].PublicKey,
		ClientV4:   r.Config.Interface.Addresses.V4,
		ClientV6:   r.Config.Interface.Addresses.V6,
		ClientID:   r.Config.ClientID,
		DeviceID:   r.ID,
		Token:      r.Token,
		AccountID:  r.Account.ID,
		Endpoint:   endpoint,
		WarpPlus:   r.Account.AccountType != "" && r.Account.AccountType != "free",
		CreatedAt:  createdAt,
	}
	return acc, nil
}

// applyLicenseSafe attempts a WARP+ upgrade. Any failure is swallowed (the
// account stays free) — a broken license must not sink an otherwise valid
// registration. On success WarpPlus is set and License is recorded.
func (c *Client) applyLicenseSafe(ctx context.Context, acc *Account, key string) {
	if acc.DeviceID == "" || acc.Token == "" {
		return
	}
	reqBody, err := json.Marshal(map[string]string{"license": key})
	if err != nil {
		return
	}
	respBody, status, err := c.do(ctx, http.MethodPatch, accountURL(acc.DeviceID), acc.Token, reqBody)
	if err != nil || status != http.StatusOK {
		return
	}
	var r struct {
		AccountType string `json:"account_type"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return
	}
	acc.License = key
	if r.AccountType != "" && r.AccountType != "free" {
		acc.WarpPlus = true
	}
}

// do performs one API request with the standard headers. bearer, when non-empty,
// adds Authorization. It returns the body, status code, and any transport error.
func (c *Client) do(ctx context.Context, method, url, bearer string, body []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("CF-Client-Version", clientVersionHdr)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	// Cap the read: registration responses are a few KB.
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

func regURL() string { return fmt.Sprintf("%s/%s/reg", apiBase, apiVersion) }
func accountURL(deviceID string) string {
	return fmt.Sprintf("%s/%s/reg/%s/account", apiBase, apiVersion, deviceID)
}

// stdOrURLBase64 decodes base64 that may be standard or URL-encoded, with or
// without padding. Cloudflare client_id is short standard base64.
func stdOrURLBase64(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		if raw, err := enc.DecodeString(s); err == nil {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("warp: not base64: %q", s)
}
