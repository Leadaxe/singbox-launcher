package subscription

import (
	"fmt"
	"net/url"
)

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
	shareAppendDetourLiteral(q, out)
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
