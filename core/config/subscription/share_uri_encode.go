// Package subscription: share URI generation from sing-box outbounds and WireGuard endpoints (reverse of ParseNode / parseWireGuardURI).
// Formats follow docs/ParserConfig.md and the same query keys as uriTransportFromQuery / vlessTLSFromNode.
package subscription

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ErrShareURINotSupported is returned for types/ shapes that cannot be encoded as a subscription-style URI
// (selector, urltest, direct, block, dns, wireguard multi-peer, etc.) or when required fields are missing.
var ErrShareURINotSupported = errors.New("outbound cannot be encoded as share URI")

// ShareURIFromOutbound builds a shareable proxy URI (vless://, vmess://, …) from a sing-box outbound map
// as stored in config.json (same shape as buildOutbound / GenerateNodeJSON output).
func ShareURIFromOutbound(out map[string]interface{}) (string, error) {
	if out == nil {
		return "", fmt.Errorf("%w: nil outbound", ErrShareURINotSupported)
	}
	if d := strings.TrimSpace(mapGetString(out, "detour")); d != "" {
		return "", fmt.Errorf("%w: outbound uses detour (chained)", ErrShareURINotSupported)
	}
	typ := strings.ToLower(strings.TrimSpace(mapGetString(out, "type")))
	switch typ {
	case "vless":
		return shareURIFromVLESS(out)
	case "vmess":
		return shareURIFromVMess(out)
	case "trojan":
		return shareURIFromTrojan(out)
	case "shadowsocks":
		return shareURIFromShadowsocks(out)
	case "socks":
		return shareURIFromSocks(out)
	case "hysteria2":
		return shareURIFromHysteria2(out)
	case "ssh":
		return shareURIFromSSH(out)
	case "wireguard":
		return ShareURIFromWireGuardEndpoint(out)
	case "selector", "urltest", "direct", "block", "dns", "http":
		return "", fmt.Errorf("%w: type %q", ErrShareURINotSupported, typ)
	default:
		return "", fmt.Errorf("%w: unknown type %q", ErrShareURINotSupported, typ)
	}
}

func mapGetString(m map[string]interface{}, k string) string {
	v, ok := m[k]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case fmt.Stringer:
		return t.String()
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		return strconv.FormatBool(t)
	default:
		return fmt.Sprint(t)
	}
}

func mapGetInt(m map[string]interface{}, k string) int {
	v, ok := m[k]
	if !ok || v == nil {
		return 0
	}
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	case json.Number:
		i, err := t.Int64()
		if err != nil {
			return 0
		}
		return int(i)
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}

func mapGetBool(m map[string]interface{}, k string) bool {
	v, ok := m[k]
	if !ok || v == nil {
		return false
	}
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(t, "true") || t == "1"
	case float64:
		return t != 0
	case int:
		return t != 0
	default:
		return false
	}
}

func fragmentFromTag(out map[string]interface{}) string {
	return mapGetString(out, "tag")
}

func hostPort(server string, port int) string {
	if server == "" || port <= 0 {
		return ""
	}
	return net.JoinHostPort(server, strconv.Itoa(port))
}

// --- transport → query (VLESS / Trojan) ---

func transportToQuery(q url.Values, tr map[string]interface{}) {
	if len(tr) == 0 {
		return
	}
	typ := strings.ToLower(strings.TrimSpace(mapGetString(tr, "type")))
	switch typ {
	case "ws":
		q.Set("type", "ws")
		if p := mapGetString(tr, "path"); p != "" {
			q.Set("path", p)
		}
		if h, ok := tr["headers"].(map[string]interface{}); ok {
			if host := mapGetString(h, "Host"); host != "" {
				q.Set("host", host)
			}
		}
	case "grpc":
		q.Set("type", "grpc")
		if sn := mapGetString(tr, "service_name"); sn != "" {
			q.Set("serviceName", sn)
		} else if p := mapGetString(tr, "path"); p != "" {
			q.Set("serviceName", p)
		}
	case "http":
		q.Set("type", "http")
		if p := mapGetString(tr, "path"); p != "" {
			q.Set("path", p)
		}
		if hv := tr["host"]; hv != nil {
			switch h := hv.(type) {
			case []interface{}:
				if len(h) > 0 {
					q.Set("host", mapGetString(map[string]interface{}{"x": h[0]}, "x"))
				}
			case []string:
				if len(h) > 0 {
					q.Set("host", h[0])
				}
			case string:
				if h != "" {
					q.Set("host", h)
				}
			}
		}
	case "httpupgrade":
		q.Set("type", "xhttp")
		if p := mapGetString(tr, "path"); p != "" {
			q.Set("path", p)
		}
		if h := mapGetString(tr, "host"); h != "" {
			q.Set("host", h)
		}
	}
}

// --- VLESS ---

func vlessTLSToQuery(q url.Values, tls map[string]interface{}, server string, port int) {
	if tls == nil {
		if shouldVLESSSkipTLSForPort(port) {
			return
		}
		q.Set("security", "tls")
		if server != "" {
			q.Set("sni", server)
		}
		return
	}
	en, hasEn := tls["enabled"].(bool)
	if hasEn && !en {
		q.Set("security", "none")
		return
	}
	if reality, ok := tls["reality"].(map[string]interface{}); ok {
		pbk := mapGetString(reality, "public_key")
		if pbk != "" {
			q.Set("pbk", pbk)
			if sid := mapGetString(reality, "short_id"); sid != "" {
				q.Set("sid", sid)
			}
			sni := mapGetString(tls, "server_name")
			if sni == "" {
				sni = server
			}
			if sni != "" {
				q.Set("sni", sni)
			}
			if utls, ok := tls["utls"].(map[string]interface{}); ok {
				if fp := mapGetString(utls, "fingerprint"); fp != "" && fp != "random" {
					q.Set("fp", fp)
				}
			}
			shareAppendALPNInsecure(q, tls)
			return
		}
	}
	// Plain TLS
	q.Set("security", "tls")
	if sni := mapGetString(tls, "server_name"); sni != "" {
		q.Set("sni", sni)
	} else if server != "" {
		q.Set("sni", server)
	}
	if utls, ok := tls["utls"].(map[string]interface{}); ok {
		if fp := mapGetString(utls, "fingerprint"); fp != "" && fp != "random" {
			q.Set("fp", fp)
		}
	}
	shareAppendALPNInsecure(q, tls)
}

func shareAppendALPNInsecure(q url.Values, tls map[string]interface{}) {
	if alpn, ok := tls["alpn"].([]interface{}); ok && len(alpn) > 0 {
		parts := make([]string, 0, len(alpn))
		for _, a := range alpn {
			s := mapGetString(map[string]interface{}{"v": a}, "v")
			if s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			q.Set("alpn", strings.Join(parts, ","))
		}
	} else if alpn, ok := tls["alpn"].([]string); ok && len(alpn) > 0 {
		q.Set("alpn", strings.Join(alpn, ","))
	}
	if mapGetBool(tls, "insecure") {
		q.Set("insecure", "1")
	}
}

func shareURIFromVLESS(out map[string]interface{}) (string, error) {
	uuid := mapGetString(out, "uuid")
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if uuid == "" || server == "" || port <= 0 {
		return "", fmt.Errorf("%w: vless needs uuid, server, server_port", ErrShareURINotSupported)
	}
	q := url.Values{}
	q.Set("encryption", "none")
	if tr, ok := out["transport"].(map[string]interface{}); ok {
		transportToQuery(q, tr)
	}
	if tls, ok := out["tls"].(map[string]interface{}); ok {
		vlessTLSToQuery(q, tls, server, port)
	} else if !shouldVLESSSkipTLSForPort(port) {
		vlessTLSToQuery(q, nil, server, port)
	}
	if f := mapGetString(out, "flow"); f != "" {
		q.Set("flow", f)
	}
	if pe := mapGetString(out, "packet_encoding"); pe != "" {
		q.Set("packetEncoding", pe)
	}
	hp := hostPort(server, port)
	u := &url.URL{
		Scheme:   "vless",
		User:     url.User(url.PathEscape(uuid)),
		Host:     hp,
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}

// --- VMess ---

func shareURIFromVMess(out map[string]interface{}) (string, error) {
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	id := mapGetString(out, "uuid")
	tag := fragmentFromTag(out)
	if server == "" || port <= 0 || id == "" {
		return "", fmt.Errorf("%w: vmess needs server, server_port, uuid", ErrShareURINotSupported)
	}
	vm := map[string]interface{}{
		"v":    "2",
		"ps":   tag,
		"add":  server,
		"port": port,
		"id":   id,
		"aid":  0,
		"scy":  mapGetString(out, "security"),
		"net":  "tcp",
		"type": "none",
		"host": "",
		"path": "",
		"tls":  "",
	}
	if vm["scy"] == "" || vm["scy"] == nil {
		vm["scy"] = "auto"
	}
	if aid := mapGetInt(out, "alter_id"); aid > 0 {
		vm["aid"] = aid
	}
	if tr, ok := out["transport"].(map[string]interface{}); ok {
		net := strings.ToLower(mapGetString(tr, "type"))
		vm["net"] = net
		switch net {
		case "ws":
			vm["path"] = mapGetString(tr, "path")
			if h, ok := tr["headers"].(map[string]interface{}); ok {
				vm["host"] = mapGetString(h, "Host")
			}
		case "grpc":
			vm["path"] = mapGetString(tr, "service_name")
			if vm["path"] == "" {
				vm["path"] = mapGetString(tr, "path")
			}
		case "http":
			vm["path"] = mapGetString(tr, "path")
			if hv := tr["host"]; hv != nil {
				switch h := hv.(type) {
				case []interface{}:
					if len(h) > 0 {
						vm["host"] = mapGetString(map[string]interface{}{"x": h[0]}, "x")
					}
				case []string:
					if len(h) > 0 {
						vm["host"] = h[0]
					}
				}
			}
		case "httpupgrade":
			vm["net"] = "ws"
			vm["path"] = mapGetString(tr, "path")
			vm["host"] = mapGetString(tr, "host")
		}
	}
	if tls, ok := out["tls"].(map[string]interface{}); ok {
		if mapGetBool(tls, "enabled") {
			vm["tls"] = "tls"
			sni := mapGetString(tls, "server_name")
			if sni == "" {
				sni = server
			}
			vm["sni"] = sni
			if alpn, ok := tls["alpn"].([]interface{}); ok && len(alpn) > 0 {
				parts := make([]string, 0, len(alpn))
				for _, a := range alpn {
					parts = append(parts, mapGetString(map[string]interface{}{"v": a}, "v"))
				}
				vm["alpn"] = strings.Join(parts, ",")
			} else if alpn, ok := tls["alpn"].([]string); ok && len(alpn) > 0 {
				vm["alpn"] = strings.Join(alpn, ",")
			}
			if utls, ok := tls["utls"].(map[string]interface{}); ok {
				if fp := mapGetString(utls, "fingerprint"); fp != "" {
					vm["fp"] = fp
				}
			}
			if mapGetBool(tls, "insecure") {
				vm["insecure"] = "1"
			}
		}
	}
	raw, err := json.Marshal(vm)
	if err != nil {
		return "", err
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(raw), nil
}

// --- Trojan ---

func trojanTLSToQuery(q url.Values, tls map[string]interface{}, server string) {
	if tls == nil {
		q.Set("sni", server)
		return
	}
	if en, ok := tls["enabled"].(bool); ok && !en {
		q.Set("security", "none")
		return
	}
	sni := mapGetString(tls, "server_name")
	if sni == "" {
		sni = server
	}
	if sni != "" {
		q.Set("sni", sni)
	}
	if utls, ok := tls["utls"].(map[string]interface{}); ok {
		if fp := mapGetString(utls, "fingerprint"); fp != "" {
			q.Set("fp", fp)
		}
	}
	shareAppendALPNInsecure(q, tls)
}

func shareURIFromTrojan(out map[string]interface{}) (string, error) {
	pass := mapGetString(out, "password")
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if pass == "" || server == "" || port <= 0 {
		return "", fmt.Errorf("%w: trojan needs password, server, server_port", ErrShareURINotSupported)
	}
	q := url.Values{}
	if tr, ok := out["transport"].(map[string]interface{}); ok {
		transportToQuery(q, tr)
	}
	if tls, ok := out["tls"].(map[string]interface{}); ok {
		trojanTLSToQuery(q, tls, server)
	} else {
		trojanTLSToQuery(q, nil, server)
	}
	u := &url.URL{
		Scheme:   "trojan",
		User:     url.User(url.PathEscape(pass)),
		Host:     hostPort(server, port),
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}

// --- Shadowsocks ---

func shareURIFromShadowsocks(out map[string]interface{}) (string, error) {
	method := mapGetString(out, "method")
	password := mapGetString(out, "password")
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if method == "" || password == "" || server == "" || port <= 0 {
		return "", fmt.Errorf("%w: shadowsocks needs method, password, server, server_port", ErrShareURINotSupported)
	}
	if !isValidShadowsocksMethod(method) {
		return "", fmt.Errorf("%w: unsupported SS method %q", ErrShareURINotSupported, method)
	}
	userinfo := method + ":" + password
	b64 := base64.StdEncoding.EncodeToString([]byte(userinfo))
	hp := hostPort(server, port)
	frag := fragmentFromTag(out)
	u := &url.URL{
		Scheme:   "ss",
		User:     url.User(b64),
		Host:     hp,
		Fragment: frag,
	}
	return u.String(), nil
}

// --- SOCKS ---

func shareURIFromSocks(out map[string]interface{}) (string, error) {
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if server == "" || port <= 0 {
		return "", fmt.Errorf("%w: socks needs server, server_port", ErrShareURINotSupported)
	}
	user := mapGetString(out, "username")
	pass := mapGetString(out, "password")
	var userinfo *url.Userinfo
	if user != "" || pass != "" {
		userinfo = url.UserPassword(user, pass)
	}
	u := &url.URL{
		Scheme:   "socks5",
		User:     userinfo,
		Host:     hostPort(server, port),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}

// --- Hysteria2 ---

func shareURIFromHysteria2(out map[string]interface{}) (string, error) {
	pass := mapGetString(out, "password")
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if pass == "" || server == "" || port <= 0 {
		return "", fmt.Errorf("%w: hysteria2 needs password, server, server_port", ErrShareURINotSupported)
	}
	q := url.Values{}
	if tls, ok := out["tls"].(map[string]interface{}); ok {
		if sni := mapGetString(tls, "server_name"); sni != "" {
			q.Set("sni", sni)
		}
		if mapGetBool(tls, "insecure") {
			q.Set("insecure", "1")
		}
		if alpn, ok := tls["alpn"].([]interface{}); ok && len(alpn) > 0 {
			parts := make([]string, 0, len(alpn))
			for _, a := range alpn {
				parts = append(parts, mapGetString(map[string]interface{}{"v": a}, "v"))
			}
			if len(parts) > 0 {
				q.Set("alpn", strings.Join(parts, ","))
			}
		} else if alpn, ok := tls["alpn"].([]string); ok && len(alpn) > 0 {
			q.Set("alpn", strings.Join(alpn, ","))
		}
	}
	if sp, ok := out["server_ports"].([]interface{}); ok && len(sp) > 0 {
		parts := make([]string, 0, len(sp))
		for _, v := range sp {
			s := mapGetString(map[string]interface{}{"v": v}, "v")
			if s != "" {
				parts = append(parts, s)
			}
		}
		if mq := hysteria2ServerPortsToMportQuery(parts); mq != "" {
			q.Set("mport", mq)
		}
	} else if sp, ok := out["server_ports"].([]string); ok && len(sp) > 0 {
		if mq := hysteria2ServerPortsToMportQuery(sp); mq != "" {
			q.Set("mport", mq)
		}
	}
	if obfs, ok := out["obfs"].(map[string]interface{}); ok {
		if ot := mapGetString(obfs, "type"); ot != "" {
			q.Set("obfs", ot)
		}
		if op := mapGetString(obfs, "password"); op != "" {
			q.Set("obfs-password", op)
		}
	}
	if up := mapGetInt(out, "up_mbps"); up > 0 {
		q.Set("upmbps", strconv.Itoa(up))
	}
	if down := mapGetInt(out, "down_mbps"); down > 0 {
		q.Set("downmbps", strconv.Itoa(down))
	}
	u := &url.URL{
		Scheme:   "hysteria2",
		User:     url.User(url.PathEscape(pass)),
		Host:     hostPort(server, port),
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}

// --- SSH ---

func shareURIFromSSH(out map[string]interface{}) (string, error) {
	user := mapGetString(out, "user")
	if user == "" {
		user = "root"
	}
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if server == "" {
		return "", fmt.Errorf("%w: ssh needs server", ErrShareURINotSupported)
	}
	if port <= 0 {
		port = 22
	}
	if mapGetString(out, "private_key") != "" {
		return "", fmt.Errorf("%w: ssh with inline private_key cannot be encoded as URI", ErrShareURINotSupported)
	}
	pass := mapGetString(out, "password")
	q := url.Values{}
	if pkp := mapGetString(out, "private_key_path"); pkp != "" {
		q.Set("private_key_path", pkp)
	}
	if hk, ok := out["host_key"].([]interface{}); ok && len(hk) > 0 {
		parts := make([]string, 0, len(hk))
		for _, x := range hk {
			parts = append(parts, mapGetString(map[string]interface{}{"v": x}, "v"))
		}
		q.Set("host_key", strings.Join(parts, ","))
	} else if hk, ok := out["host_key"].([]string); ok && len(hk) > 0 {
		q.Set("host_key", strings.Join(hk, ","))
	}
	if algs, ok := out["host_key_algorithms"].([]interface{}); ok && len(algs) > 0 {
		parts := make([]string, 0, len(algs))
		for _, x := range algs {
			parts = append(parts, mapGetString(map[string]interface{}{"v": x}, "v"))
		}
		q.Set("host_key_algorithms", strings.Join(parts, ","))
	} else if algs, ok := out["host_key_algorithms"].([]string); ok && len(algs) > 0 {
		q.Set("host_key_algorithms", strings.Join(algs, ","))
	}
	if cv := mapGetString(out, "client_version"); cv != "" {
		q.Set("client_version", cv)
	}
	if pp := mapGetString(out, "private_key_passphrase"); pp != "" {
		q.Set("private_key_passphrase", pp)
	}
	var ui *url.Userinfo
	if pass != "" {
		ui = url.UserPassword(user, pass)
	} else {
		ui = url.User(url.PathEscape(user))
	}
	u := &url.URL{
		Scheme:   "ssh",
		User:     ui,
		Host:     hostPort(server, port),
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}

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
