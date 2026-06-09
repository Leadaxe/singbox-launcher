package subscription

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// --- WireGuard (sing-box endpoints[]) ---

// ShareURIFromWireGuardEndpoint builds wireguard:// from one sing-box endpoint object in config.json `endpoints[]`
// (same shape as produced by parseWireGuardURI / GenerateEndpointJSON). Only **single-peer** endpoints are supported:
// subscription-style URIs have one remote server; multiple peers return ErrShareURINotSupported.
func ShareURIFromWireGuardEndpoint(ep map[string]interface{}) (string, error) {
	if ep == nil {
		return "", fmt.Errorf("%w: nil endpoint", ErrShareURINotSupported)
	}
	if strings.ToLower(strings.TrimSpace(mapGetString(ep, "type"))) != "wireguard" {
		return "", fmt.Errorf("%w: endpoint type is not wireguard", ErrShareURINotSupported)
	}
	priv := mapGetString(ep, "private_key")
	if priv == "" {
		return "", fmt.Errorf("%w: wireguard missing private_key", ErrShareURINotSupported)
	}
	peers, err := wireGuardPeerMaps(ep)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrShareURINotSupported, err)
	}
	if len(peers) > 1 {
		return "", fmt.Errorf("%w: wireguard with multiple peers cannot be encoded as one subscription URI", ErrShareURINotSupported)
	}
	peer := peers[0]
	server := mapGetString(peer, "address")
	port := mapGetInt(peer, "port")
	if server == "" {
		return "", fmt.Errorf("%w: wireguard peer missing address", ErrShareURINotSupported)
	}
	if port <= 0 {
		port = 51820
	}
	pub := mapGetString(peer, "public_key")
	if pub == "" {
		return "", fmt.Errorf("%w: wireguard peer missing public_key", ErrShareURINotSupported)
	}
	allowed := stringSliceFromWireGuardField(peer["allowed_ips"])
	if len(allowed) == 0 {
		return "", fmt.Errorf("%w: wireguard peer missing allowed_ips", ErrShareURINotSupported)
	}
	addrList := stringSliceFromWireGuardField(ep["address"])
	if len(addrList) == 0 {
		return "", fmt.Errorf("%w: wireguard missing address", ErrShareURINotSupported)
	}
	q := url.Values{}
	q.Set("publickey", pub)
	q.Set("address", strings.Join(addrList, ","))
	q.Set("allowedips", strings.Join(allowed, ","))
	if mtu := mapGetInt(ep, "mtu"); mtu > 0 && mtu != 1420 {
		q.Set("mtu", strconv.Itoa(mtu))
	}
	if ka := mapGetInt(peer, "persistent_keepalive_interval"); ka > 0 {
		q.Set("keepalive", strconv.Itoa(ka))
	}
	if psk := mapGetString(peer, "pre_shared_key"); psk != "" {
		q.Set("presharedkey", psk)
	}
	if lp := mapGetInt(ep, "listen_port"); lp > 0 {
		q.Set("listenport", strconv.Itoa(lp))
	}
	if name := mapGetString(ep, "name"); name != "" && name != "singbox-wg0" {
		q.Set("name", name)
	}
	if dnsStr := wireGuardDNSToQuery(ep["dns"]); dnsStr != "" {
		q.Set("dns", dnsStr)
	}
	// AmneziaWG (SPEC 073): re-emit obfuscation params so endpoint→URI→endpoint
	// round-trips losslessly. Numeric fields are emitted by PRESENCE (so an
	// explicit jc=0 / junk-off survives); i1–i5 only when non-empty. url.Values
	// escapes the tag chars (<, >, space); the parser decodes them back.
	for _, k := range awgNumericFields {
		if raw, ok := ep[k]; ok {
			if s, ok2 := awgNumericString(raw); ok2 {
				q.Set(k, s)
			}
		}
	}
	for _, k := range awgStringFields {
		if s := mapGetString(ep, k); s != "" {
			q.Set(k, s)
		}
	}
	u := &url.URL{
		Scheme:   "wireguard",
		User:     url.User(url.PathEscape(priv)),
		Host:     net.JoinHostPort(server, strconv.Itoa(port)),
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(ep),
	}
	return u.String(), nil
}

func wireGuardPeerMaps(ep map[string]interface{}) ([]map[string]interface{}, error) {
	v, ok := ep["peers"]
	if !ok {
		return nil, fmt.Errorf("missing peers")
	}
	if typed, ok := v.([]map[string]interface{}); ok {
		if len(typed) == 0 {
			return nil, fmt.Errorf("peers must be a non-empty array")
		}
		return typed, nil
	}
	arr, ok := v.([]interface{})
	if !ok || len(arr) == 0 {
		return nil, fmt.Errorf("peers must be a non-empty array")
	}
	out := make([]map[string]interface{}, 0, len(arr))
	for _, e := range arr {
		m, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		out = append(out, m)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no valid peer objects")
	}
	return out, nil
}

func stringSliceFromWireGuardField(v interface{}) []string {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case string:
		x = strings.TrimSpace(x)
		if x == "" {
			return nil
		}
		return []string{x}
	case []string:
		return x
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, e := range x {
			s := wireGuardJSONElemToString(e)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func wireGuardJSONElemToString(e interface{}) string {
	if e == nil {
		return ""
	}
	switch x := e.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case json.Number:
		s := strings.TrimSpace(x.String())
		return s
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

// awgNumericString formats an AmneziaWG numeric endpoint field for a share-URI
// query. It accepts every shape the value can take depending on provenance:
// freshly parsed (int64), JSON-decoded from state (float64 / json.Number), or
// hand-built (int / uint32). Returns ok=false for nil / non-numeric / empty so
// the caller skips the param. Avoids mapGetInt — that returns `int` (would
// overflow large h-values on 32-bit) and doesn't handle uint32.
func awgNumericString(v interface{}) (string, bool) {
	switch t := v.(type) {
	case int64:
		return strconv.FormatInt(t, 10), true
	case int:
		return strconv.Itoa(t), true
	case uint32:
		return strconv.FormatUint(uint64(t), 10), true
	case uint64:
		return strconv.FormatUint(t, 10), true
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10), true
		}
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case json.Number:
		s := strings.TrimSpace(t.String())
		return s, s != ""
	case string:
		s := strings.TrimSpace(t)
		return s, s != ""
	default:
		return "", false
	}
}

func wireGuardDNSToQuery(v interface{}) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case []string:
		return strings.Join(x, ",")
	case []interface{}:
		return strings.Join(stringSliceFromWireGuardField(x), ",")
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}
