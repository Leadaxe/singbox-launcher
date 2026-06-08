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
