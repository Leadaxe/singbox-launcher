// Package warp registers Cloudflare WARP accounts and builds sing-box WireGuard
// nodes from them. The X25519 key pair is generated on-device — only the public
// key is sent to Cloudflare (like wgcf / the official client). The private key
// never leaves the machine, so we never rely on third-party "generator" workers
// that would hand back a server-minted private key.
//
// The package is a leaf: it depends only on the standard library and accepts an
// injected HTTP client, so it compiles and tests fast and stays free of the
// launcher's controller graph.
package warp

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// DefaultEndpoint is the WARP endpoint Cloudflare hands back. It is the one the
// RU DPI blocks by name, so the generator can substitute a random ip:port from
// the anycast pool (see endpoints.go) when obfuscation is on.
const DefaultEndpoint = "engage.cloudflareclient.com:2408"

// warpMTU is the tunnel MTU for a WARP WireGuard node. WARP with AmneziaWG
// padding needs headroom under the path MTU; 1280 (the IPv6 minimum) is the
// AmneziaWG-recommended value and matches the parser's awgMaxMTU clamp.
const warpMTU = 1280

// Account is a registered WARP account plus everything needed to emit a sing-box
// WireGuard endpoint. PrivateKey and Token are secrets: never log them, mask
// them in errors, and store them as secret-typed state.
type Account struct {
	PrivateKey string         // base64 X25519 private key (secret, on-device only)
	PeerPublic string         // base64 peer public key from Cloudflare
	ClientV4   string         // interface address v4 (bare IP, no mask)
	ClientV6   string         // interface address v6 (bare IP, no mask)
	ClientID   string         // base64, 3 bytes → WireGuard reserved
	DeviceID   string         // Cloudflare device id (for PATCH license/masque)
	Token      string         // Bearer token (secret)
	AccountID  string         // Cloudflare account id
	Endpoint   string         // host:port (DefaultEndpoint or random from pool)
	License    string         // WARP+ license key (optional)
	WarpPlus   bool           // account_type == "limited"/"unlimited"
	AWG        map[string]any // nil = plain WireGuard; else AmneziaWG obfuscation fields
	CreatedAt  string         // ISO-8601, used for the tos field at registration
}

// Reserved decodes ClientID (base64, 3 bytes) into the WireGuard reserved
// triplet. Returns nil when ClientID is empty or malformed — a WARP node still
// works without reserved on many paths, so a bad value must not fail node build.
func (a *Account) Reserved() []int {
	return parseReserved(a.ClientID)
}

// DisplayTag is the human node label with an emoji that signals the transport
// and WARP+ status, matching the LxBox convention so shared configs read alike.
func (a *Account) DisplayTag() string {
	tag := "🔥☁️ WARP"
	if len(a.AWG) > 0 {
		tag = "🔥⛈️ WARP (AWG)"
	}
	if a.WarpPlus {
		tag += "+"
	}
	return tag
}

// ToWireguardURI builds a wireguard:// share URI for the account. It is fed to
// the existing subscription parser (node_parser_wireguard.go), which is the one
// place that constructs the endpoint map — the generator never duplicates that.
// includeReserved controls whether client_id is emitted as reserved (plain WARP
// often works without it; obfuscated WARP needs it to reach the right anycast).
func (a *Account) ToWireguardURI(includeReserved bool) (string, error) {
	if strings.TrimSpace(a.PrivateKey) == "" {
		return "", fmt.Errorf("warp: empty private key")
	}
	if strings.TrimSpace(a.PeerPublic) == "" {
		return "", fmt.Errorf("warp: empty peer public key")
	}
	host, port, err := splitHostPort(a.endpointOrDefault())
	if err != nil {
		return "", err
	}

	addrs := make([]string, 0, 2)
	if a.ClientV4 != "" {
		addrs = append(addrs, a.ClientV4)
	}
	if a.ClientV6 != "" {
		addrs = append(addrs, a.ClientV6)
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("warp: no interface address")
	}

	q := url.Values{}
	q.Set("publickey", a.PeerPublic)
	q.Set("address", strings.Join(addrs, ","))
	q.Set("allowedips", "0.0.0.0/0,::/0")
	q.Set("mtu", strconv.Itoa(warpMTU))
	if includeReserved {
		if r := a.Reserved(); len(r) == 3 {
			q.Set("reserved", fmt.Sprintf("%d,%d,%d", r[0], r[1], r[2]))
		}
	}
	// AmneziaWG obfuscation params (numeric emitted as decimal, string tags as-is).
	for k, v := range a.AWG {
		q.Set(k, awgValueToString(v))
	}

	// userinfo carries the base64 private key; url.URL percent-encodes '/' and
	// '+' safely. The parser uses PathUnescape to restore the raw key.
	u := &url.URL{
		Scheme:   "wireguard",
		User:     url.User(a.PrivateKey),
		Host:     host + ":" + strconv.Itoa(port),
		RawQuery: q.Encode(),
		Fragment: a.DisplayTag(),
	}
	return u.String(), nil
}

func (a *Account) endpointOrDefault() string {
	if strings.TrimSpace(a.Endpoint) == "" {
		return DefaultEndpoint
	}
	return a.Endpoint
}

// parseReserved decodes a base64 client_id into a 3-int reserved triplet.
func parseReserved(clientID string) []int {
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return nil
	}
	raw, err := stdOrURLBase64(clientID)
	if err != nil || len(raw) < 3 {
		return nil
	}
	return []int{int(raw[0]), int(raw[1]), int(raw[2])}
}

// awgValueToString renders an AmneziaWG field value for the query string.
// Numbers become plain decimals; strings pass through with case preserved.
func awgValueToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// splitHostPort splits "host:port" tolerating IPv6 brackets. WARP endpoints are
// always host:port; a missing port defaults to 2408.
func splitHostPort(hp string) (string, int, error) {
	hp = strings.TrimSpace(hp)
	if hp == "" {
		return "", 0, fmt.Errorf("warp: empty endpoint")
	}
	// Bracketed IPv6: [addr]:port
	if strings.HasPrefix(hp, "[") {
		end := strings.Index(hp, "]")
		if end < 0 {
			return "", 0, fmt.Errorf("warp: malformed IPv6 endpoint %q", hp)
		}
		host := hp[1:end]
		rest := strings.TrimPrefix(hp[end+1:], ":")
		port := 2408
		if rest != "" {
			p, err := strconv.Atoi(rest)
			if err != nil {
				return "", 0, fmt.Errorf("warp: bad port in %q", hp)
			}
			port = p
		}
		return host, port, nil
	}
	i := strings.LastIndex(hp, ":")
	if i < 0 {
		return hp, 2408, nil
	}
	host := hp[:i]
	p, err := strconv.Atoi(hp[i+1:])
	if err != nil {
		return "", 0, fmt.Errorf("warp: bad port in %q", hp)
	}
	return host, p, nil
}
