package subscription

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

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
		if utls, ok := tls["utls"].(map[string]interface{}); ok {
			if fp := mapGetString(utls, "fingerprint"); fp != "" {
				q.Set("fp", fp)
			}
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
	shareAppendDetourLiteral(q, out)
	u := &url.URL{
		Scheme:   "hysteria2",
		User:     url.User(url.PathEscape(pass)),
		Host:     hostPort(server, port),
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}
