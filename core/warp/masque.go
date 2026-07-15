package warp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MasqueAccount is a registered WARP MASQUE account plus everything needed to
// emit a sing-box masque outbound. Unlike the WireGuard WARP account it uses an
// ECDSA P-256 key pair (DER-encoded) and has no reserved/AWG fields; it carries
// a transport selector (network h3/h2). PrivateKeyDER and Token are secrets.
type MasqueAccount struct {
	PrivateKeyDER string // base64 SEC1 DER (x509.MarshalECPrivateKey) — secret, on-device only
	ServerPubDER  string // base64 PKIX DER of the endpoint public key (from Cloudflare)
	ClientV4      string // interface address v4 (CIDR)
	ClientV6      string // interface address v6 (CIDR)
	Server        string // endpoint host/IP
	Port          int    // default 443
	DeviceID      string
	Token         string // Bearer (secret)
	Network       string // "h3" (default) | "h2"
	SNI           string // optional
	IdleTimeout   string // optional Go duration
	KeepAlive     string // optional Go duration
	CreatedAt     string
}

// DisplayTag mirrors the LxBox MASQUE node tag convention.
func (a *MasqueAccount) DisplayTag() string { return "🔥🎭 WARP (MASQUE)" }

// GenerateECDSAKeypair returns (privateBase64DER, publicBase64DER) for a fresh
// P-256 pair. The private key is SEC1 DER (x509.MarshalECPrivateKey) and the
// public key is PKIX DER (x509.MarshalPKIXPublicKey) — exactly the encodings the
// sing-box masque outbound parses (x509.ParseECPrivateKey / ParsePKIXPublicKey).
// The private key stays on the machine; only the public key is enrolled.
func GenerateECDSAKeypair() (privDER string, pubDER string, err error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("warp masque: keygen: %w", err)
	}
	privBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("warp masque: marshal private: %w", err)
	}
	pubBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("warp masque: marshal public: %w", err)
	}
	return base64.StdEncoding.EncodeToString(privBytes),
		base64.StdEncoding.EncodeToString(pubBytes), nil
}

// ToMasqueURI builds a masque:// share URI for the account (the launcher parser
// turns it back into a masque outbound). Mirrors the LxBox toMasqueUri contract:
// masque://<privDER>@<server>:<port>?publickey=&address=&profile=cloudflare
//
//	&network=&mtu=[&sni=][&idle_timeout=][&keep_alive=]#tag
func (a *MasqueAccount) ToMasqueURI() (string, error) {
	if a.PrivateKeyDER == "" || a.ServerPubDER == "" {
		return "", fmt.Errorf("warp masque: missing key material")
	}
	if a.ClientV4 == "" && a.ClientV6 == "" {
		return "", fmt.Errorf("warp masque: missing interface address")
	}
	network := a.Network
	if network == "" {
		network = "h3"
	}
	port := a.Port
	if port == 0 {
		port = 443
	}
	addrs := make([]string, 0, 2)
	if a.ClientV4 != "" {
		addrs = append(addrs, ensureCIDR(a.ClientV4, false))
	}
	if a.ClientV6 != "" {
		addrs = append(addrs, ensureCIDR(a.ClientV6, true))
	}
	q := url.Values{}
	q.Set("publickey", a.ServerPubDER)
	q.Set("address", strings.Join(addrs, ","))
	q.Set("profile", "cloudflare")
	q.Set("network", network)
	q.Set("mtu", strconv.Itoa(warpMTU))
	if a.SNI != "" {
		q.Set("sni", a.SNI)
	}
	if a.IdleTimeout != "" {
		q.Set("idle_timeout", a.IdleTimeout)
	}
	if a.KeepAlive != "" {
		q.Set("keep_alive", a.KeepAlive)
	}
	u := &url.URL{
		Scheme:   "masque",
		User:     url.User(a.PrivateKeyDER),
		Host:     net.JoinHostPort(a.Server, strconv.Itoa(port)), // re-brackets IPv6
		RawQuery: q.Encode(),
		Fragment: a.DisplayTag(),
	}
	return u.String(), nil
}

// ToMasqueOutbound builds the sing-box masque outbound map from the account,
// following the core SPEC 021 contract (profile cloudflare, network transport,
// ip/ipv6 tunnel addresses, base64-DER keys). Requires core >= lx.2.
func (a *MasqueAccount) ToMasqueOutbound() (map[string]interface{}, error) {
	if a.PrivateKeyDER == "" || a.ServerPubDER == "" {
		return nil, fmt.Errorf("warp masque: missing key material")
	}
	if a.ClientV4 == "" && a.ClientV6 == "" {
		return nil, fmt.Errorf("warp masque: missing interface address")
	}
	network := a.Network
	if network == "" {
		network = "h3"
	}
	port := a.Port
	if port == 0 {
		port = 443
	}
	ob := map[string]interface{}{
		"type":        "masque",
		"tag":         a.DisplayTag(),
		"server":      a.Server,
		"server_port": port,
		"profile":     "cloudflare",
		"network":     network,
		"private_key": a.PrivateKeyDER,
		"public_key":  a.ServerPubDER,
		"mtu":         warpMTU,
	}
	if a.ClientV4 != "" {
		ob["ip"] = ensureCIDR(a.ClientV4, false)
	}
	if a.ClientV6 != "" {
		ob["ipv6"] = ensureCIDR(a.ClientV6, true)
	}
	if a.SNI != "" {
		ob["sni"] = a.SNI
	}
	if a.IdleTimeout != "" {
		ob["idle_timeout"] = a.IdleTimeout
	}
	if a.KeepAlive != "" {
		ob["keep_alive_period"] = a.KeepAlive
	}
	return ob, nil
}

// RegisterMasque registers a WARP MASQUE account in two steps (mirrors LxBox):
//  1. POST /reg with a throwaway WireGuard key to create the device.
//  2. PATCH /reg/{id} with the ECDSA public key, key_type=secp256r1,
//     tunnel_type=masque → the server public key + interface addresses.
//
// The ECDSA private key is generated on-device and never sent. network selects
// the transport ("h3" default). A non-empty sni overrides the TLS server name.
func (c *Client) RegisterMasque(ctx context.Context, now time.Time, network, sni string) (*MasqueAccount, error) {
	// Step 1: create a device with a throwaway WG key.
	_, wgPub, err := GenerateKeypair()
	if err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	nowISO := now.UTC().Format("2006-01-02T15:04:05.000Z")
	regBody, _ := json.Marshal(map[string]string{
		"key": wgPub, "install_id": "", "fcm_token": "", "tos": nowISO,
		"model": "PC", "serial_number": "", "locale": "en_US",
	})
	regResp, status, err := c.do(ctx, http.MethodPost, regURL(), "", regBody)
	if err != nil {
		return nil, fmt.Errorf("warp masque: reg request: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("warp masque: registration failed (HTTP %d); API version (%s) may have changed", status, apiVersion)
	}
	var reg struct {
		ID    string `json:"id"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(regResp, &reg); err != nil || reg.ID == "" || reg.Token == "" {
		return nil, fmt.Errorf("warp masque: bad reg response")
	}

	// Step 2: enroll the ECDSA key for MASQUE.
	privDER, pubDER, err := GenerateECDSAKeypair()
	if err != nil {
		return nil, err
	}
	enrollBody, _ := json.Marshal(map[string]string{
		"key": pubDER, "key_type": "secp256r1", "tunnel_type": "masque",
	})
	enrollResp, status, err := c.do(ctx, http.MethodPatch, deviceURL(reg.ID), reg.Token, enrollBody)
	if err != nil {
		return nil, fmt.Errorf("warp masque: enroll request: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("warp masque: enroll failed (HTTP %d)", status)
	}

	acc, err := parseMasqueEnroll(enrollResp, privDER, pubDER, reg.ID, reg.Token, nowISO)
	if err != nil {
		return nil, err
	}
	if network == "h2" {
		acc.Network = "h2"
	} else {
		acc.Network = "h3"
	}
	acc.SNI = strings.TrimSpace(sni)
	acc.IdleTimeout = "5m"
	acc.KeepAlive = "30s"
	return acc, nil
}

// parseMasqueEnroll extracts the MASQUE account from the PATCH-enroll response.
// The server public key may arrive under config.peers[0].public_key (DER) and
// the interface addresses under config.interface.addresses. A fallback keeps the
// client's own pub if the server does not echo one (standard profile).
func parseMasqueEnroll(body []byte, privDER, ownPubDER, deviceID, token, createdAt string) (*MasqueAccount, error) {
	var r struct {
		Config struct {
			Interface struct {
				Addresses struct {
					V4 string `json:"v4"`
					V6 string `json:"v6"`
				} `json:"addresses"`
			} `json:"interface"`
			Peers []struct {
				PublicKey string `json:"public_key"`
				// MASQUE data-plane endpoint: the reachable IP is under
				// endpoint.v4 (with a ":0" placeholder port) plus a ports list —
				// NOT endpoint.host, which is the control-plane hostname.
				Endpoint struct {
					V4    string `json:"v4"`
					V6    string `json:"v6"`
					Host  string `json:"host"`
					Ports []int  `json:"ports"`
				} `json:"endpoint"`
			} `json:"peers"`
		} `json:"config"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("warp masque: bad enroll response: %w", err)
	}
	acc := &MasqueAccount{
		PrivateKeyDER: privDER,
		ClientV4:      r.Config.Interface.Addresses.V4,
		ClientV6:      r.Config.Interface.Addresses.V6,
		DeviceID:      deviceID,
		Token:         token,
		Port:          443,
		CreatedAt:     createdAt,
	}
	if len(r.Config.Peers) > 0 {
		peer := r.Config.Peers[0]
		// Cloudflare returns the server key as PEM — normalize to clean
		// base64(DER) which is what the sing-box masque outbound decodes.
		acc.ServerPubDER = normalizePEMToBase64DER(peer.PublicKey)
		if v4 := hostOnly(peer.Endpoint.V4); v4 != "" {
			acc.Server = v4
		} else if peer.Endpoint.Host != "" {
			acc.Server = hostOnly(peer.Endpoint.Host)
		}
		for _, p := range peer.Endpoint.Ports {
			if p > 0 {
				acc.Port = p
				break
			}
		}
	}
	if acc.ServerPubDER == "" {
		acc.ServerPubDER = ownPubDER // standard profile echo fallback
	}
	if acc.Server == "" {
		acc.Server = "162.159.198.1" // Cloudflare MASQUE anycast default
	}
	return acc, nil
}

// normalizePEMToBase64DER converts a PEM-wrapped key to clean base64(DER). When
// the input has no PEM header it is returned with whitespace stripped (already
// base64). Mirrors LxBox MasqueKeys.normalizeServerPubKey.
func normalizePEMToBase64DER(raw string) string {
	s := strings.TrimSpace(raw)
	if !strings.Contains(s, "-----BEGIN") {
		return strings.Join(strings.Fields(s), "")
	}
	// strip PEM header/footer lines and all whitespace between.
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-----") {
			continue
		}
		b.WriteString(line)
	}
	return b.String()
}

func deviceURL(deviceID string) string {
	return fmt.Sprintf("%s/%s/reg/%s", apiBase, apiVersion, deviceID)
}

// ensureCIDR appends a /32 (v4) or /128 (v6) mask to a bare address.
func ensureCIDR(addr string, v6 bool) string {
	if strings.Contains(addr, "/") {
		return addr
	}
	if v6 {
		return addr + "/128"
	}
	return addr + "/32"
}

// hostOnly strips a :port suffix from "host:port" (IPv6-bracket aware).
func hostOnly(hp string) string {
	if h, _, err := splitHostPort(hp); err == nil {
		return h
	}
	return hp
}
