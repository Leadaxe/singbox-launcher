package subscription

import (
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// xrayMapString returns string value for key in m.
func xrayMapString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return strings.TrimSpace(s)
	case fmt.Stringer:
		return strings.TrimSpace(s.String())
	default:
		return strings.TrimSpace(fmt.Sprint(s))
	}
}

// xrayJSONInt coerces JSON-decoded numbers to int.
func xrayJSONInt(v interface{}) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	default:
		return 0
	}
}

// xraySockoptDialerRef returns dialerProxy or dialer from streamSettings.sockopt (Xray).
func xraySockoptDialerRef(streamSettings map[string]interface{}) string {
	if streamSettings == nil {
		return ""
	}
	sockopt, _ := streamSettings["sockopt"].(map[string]interface{})
	if sockopt == nil {
		return ""
	}
	if s := xrayMapString(sockopt, "dialerProxy"); s != "" {
		return s
	}
	return xrayMapString(sockopt, "dialer")
}

// xrayBuildVLESSFromOutbound maps one Xray VLESS outbound (with vnext) into ParsedNode fields and sing-box-shaped Outbound.
func xrayBuildVLESSFromOutbound(ob map[string]interface{}, label string) (*configtypes.ParsedNode, error) {
	settings, _ := ob["settings"].(map[string]interface{})
	if settings == nil {
		return nil, fmt.Errorf("missing settings")
	}
	vnextRaw, ok := settings["vnext"].([]interface{})
	if !ok || len(vnextRaw) == 0 {
		return nil, fmt.Errorf("missing vnext")
	}
	vn0, ok := vnextRaw[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid vnext[0]")
	}
	addr := xrayMapString(vn0, "address")
	if addr == "" {
		return nil, fmt.Errorf("missing vnext address")
	}
	port := xrayJSONInt(vn0["port"])
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid vnext port")
	}
	users, _ := vn0["users"].([]interface{})
	if len(users) == 0 {
		return nil, fmt.Errorf("missing vnext users")
	}
	u0, ok := users[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid vnext user")
	}
	uuid := xrayMapString(u0, "id")
	if uuid == "" {
		return nil, fmt.Errorf("missing user id")
	}
	flow := xrayMapString(u0, "flow")

	streamSettings, _ := ob["streamSettings"].(map[string]interface{})
	network := strings.ToLower(xrayMapString(streamSettings, "network"))
	if network == "" {
		network = "tcp"
	}
	security := strings.ToLower(xrayMapString(streamSettings, "security"))

	outbound := make(map[string]interface{})
	outbound["tag"] = xrayMapString(ob, "tag")
	outbound["type"] = "vless"
	outbound["server"] = addr
	outbound["server_port"] = port
	outbound["uuid"] = uuid

	if flow != "" {
		if flow == "xtls-rprx-vision-udp443" {
			outbound["flow"] = "xtls-rprx-vision"
			outbound["packet_encoding"] = "xudp"
			outbound["server_port"] = 443
		} else {
			outbound["flow"] = flow
		}
	}

	if tls := xrayVLESSTLSFromStreamSettings(streamSettings, security); tls != nil {
		outbound["tls"] = tls
	}

	if tr := xrayTransportFromStreamSettings(streamSettings, network); tr != nil {
		outbound["transport"] = tr
	}

	tag := xrayMapString(ob, "tag")
	if tag == "" {
		tag = "vless"
	}

	// REALITY over TCP often omits flow in Xray; sing-box expects flow for VLESS (same as URI path in buildOutbound).
	if flow == "" && network == "tcp" {
		if tls, ok := outbound["tls"].(map[string]interface{}); ok {
			if reality, ok := tls["reality"].(map[string]interface{}); ok {
				if en, _ := reality["enabled"].(bool); en {
					if pk, _ := reality["public_key"].(string); strings.TrimSpace(pk) != "" {
						flow = "xtls-rprx-vision"
						outbound["flow"] = flow
					}
				}
			}
		}
	}

	node := &configtypes.ParsedNode{
		Tag:      tag,
		Scheme:   "vless",
		Server:   addr,
		Port:     port,
		UUID:     uuid,
		Flow:     flow,
		Label:    label,
		Outbound: outbound,
	}
	return node, nil
}

func xrayVLESSTLSFromStreamSettings(streamSettings map[string]interface{}, security string) map[string]interface{} {
	if streamSettings == nil {
		return nil
	}
	if security != "reality" && security != "tls" {
		return nil
	}

	tlsData := map[string]interface{}{
		"enabled": true,
	}

	if security == "reality" {
		rs, _ := streamSettings["realitySettings"].(map[string]interface{})
		if rs == nil {
			return tlsData
		}
		sni := xrayMapString(rs, "serverName")
		if sni == "" {
			sni = xrayMapString(rs, "server_name")
		}
		if sni != "" {
			tlsData["server_name"] = sni
		}
		fp := NormalizeUTLSFingerprint(xrayMapString(rs, "fingerprint"))
		if fp == "" {
			fp = "random"
		}
		tlsData["utls"] = map[string]interface{}{
			"enabled":     true,
			"fingerprint": fp,
		}
		if b, ok := rs["allowInsecure"].(bool); ok && b {
			tlsData["insecure"] = true
		}
		pbk := xrayMapString(rs, "publicKey")
		if pbk == "" {
			pbk = xrayMapString(rs, "public_key")
		}
		sid := xrayMapString(rs, "shortId")
		if sid == "" {
			sid = xrayMapString(rs, "short_id")
		}
		tlsData["reality"] = map[string]interface{}{
			"enabled":    true,
			"public_key": pbk,
			"short_id":   sid,
		}
		return tlsData
	}

	// generic tls
	tlsSettings, _ := streamSettings["tlsSettings"].(map[string]interface{})
	if tlsSettings != nil {
		if sni := xrayMapString(tlsSettings, "serverName"); sni != "" {
			tlsData["server_name"] = sni
		}
		if fp := NormalizeUTLSFingerprint(xrayMapString(tlsSettings, "fingerprint")); fp != "" {
			tlsData["utls"] = map[string]interface{}{
				"enabled":     true,
				"fingerprint": fp,
			}
		}
		if b, ok := tlsSettings["allowInsecure"].(bool); ok && b {
			tlsData["insecure"] = true
		}
	}
	return tlsData
}

func xrayTransportFromStreamSettings(streamSettings map[string]interface{}, network string) map[string]interface{} {
	if streamSettings == nil || network == "" || network == "tcp" {
		return nil
	}
	switch network {
	case "ws":
		ws, _ := streamSettings["wsSettings"].(map[string]interface{})
		if ws == nil {
			return map[string]interface{}{"type": "ws"}
		}
		tr := map[string]interface{}{"type": "ws"}
		if p := xrayMapString(ws, "path"); p != "" {
			tr["path"] = p
		}
		if h := xrayMapString(ws, "host"); h != "" {
			tr["headers"] = map[string]string{"Host": h}
		}
		return tr
	case "grpc":
		gs, _ := streamSettings["grpcSettings"].(map[string]interface{})
		tr := map[string]interface{}{"type": "grpc"}
		if gs != nil {
			if s := xrayMapString(gs, "serviceName"); s != "" {
				tr["service_name"] = s
			}
		}
		return tr
	case "http", "h2":
		hs, _ := streamSettings["httpSettings"].(map[string]interface{})
		tr := map[string]interface{}{"type": "http"}
		if hs != nil {
			if p := xrayMapString(hs, "path"); p != "" {
				tr["path"] = p
			}
			if host := xrayMapString(hs, "host"); host != "" {
				tr["host"] = []string{host}
			}
		}
		return tr
	default:
		return nil
	}
}

// xrayBuildJumpFromSocksOutbound builds ParsedJump from Xray socks outbound (settings.servers[0]).
func xrayBuildJumpFromSocksOutbound(ob map[string]interface{}, jumpTag string) (*configtypes.ParsedJump, error) {
	settings, _ := ob["settings"].(map[string]interface{})
	if settings == nil {
		return nil, fmt.Errorf("missing socks settings")
	}
	servers, _ := settings["servers"].([]interface{})
	if len(servers) == 0 {
		return nil, fmt.Errorf("missing socks servers")
	}
	s0, ok := servers[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid socks server")
	}
	addr := xrayMapString(s0, "address")
	port := xrayJSONInt(s0["port"])
	if addr == "" || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid socks address/port")
	}

	jump := &configtypes.ParsedJump{
		Tag:      jumpTag,
		Scheme:   "socks",
		Server:   addr,
		Port:     port,
		Outbound: map[string]interface{}{"version": "5"},
	}
	users, _ := s0["users"].([]interface{})
	if len(users) > 0 {
		if u0, ok := users[0].(map[string]interface{}); ok {
			user := xrayMapString(u0, "user")
			pass := xrayMapString(u0, "pass")
			if user != "" {
				jump.Outbound["username"] = user
			}
			if pass != "" {
				jump.Outbound["password"] = pass
			}
		}
	}
	return jump, nil
}

// xrayBuildJumpFromOutbound maps the Xray outbound referenced by dialerProxy (any supported protocol) into ParsedJump.
// SOCKS and VLESS are supported; other protocols return an error (element should be skipped with WarnLog).
func xrayBuildJumpFromOutbound(jumpOb map[string]interface{}, jumpTag, label string) (*configtypes.ParsedJump, error) {
	prot := strings.ToLower(xrayMapString(jumpOb, "protocol"))
	switch prot {
	case "socks":
		return xrayBuildJumpFromSocksOutbound(jumpOb, jumpTag)
	case "vless":
		pn, err := xrayBuildVLESSFromOutbound(jumpOb, label)
		if err != nil {
			return nil, err
		}
		if pn.Outbound == nil {
			return nil, fmt.Errorf("vless jump: empty outbound")
		}
		cp := make(map[string]interface{}, len(pn.Outbound)+1)
		for k, v := range pn.Outbound {
			cp[k] = v
		}
		cp["tag"] = jumpTag
		return &configtypes.ParsedJump{
			Tag:      jumpTag,
			Scheme:   "vless",
			Server:   pn.Server,
			Port:     pn.Port,
			UUID:     pn.UUID,
			Flow:     pn.Flow,
			Outbound: cp,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported jump protocol %q (supported: socks, vless)", prot)
	}
}
