package subscription

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// --- Hysteria v1 ---

// shareURIFromHysteria собирает hysteria:// из sing-box outbound типа "hysteria".
//
// Форма де-факто клиентов Hysteria 1.x: секрет едет в query (auth=), а не в
// userinfo — этим ссылка отличается от hysteria2://password@host. obfs у v1 —
// одна строка-секрет, поэтому пишется в obfsParam=, а obfs= несёт тип xplus:
// так ссылку понимают и сторонние клиенты, и наш собственный парсер.
func shareURIFromHysteria(out map[string]interface{}) (string, error) {
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if server == "" || port <= 0 {
		return "", fmt.Errorf("%w: hysteria needs server, server_port", ErrShareURINotSupported)
	}
	q := url.Values{}
	if auth := mapGetString(out, "auth_str"); auth != "" {
		q.Set("auth", auth)
	}
	if tls, ok := out["tls"].(map[string]interface{}); ok {
		if sni := mapGetString(tls, "server_name"); sni != "" {
			q.Set("sni", sni)
		}
		if mapGetBool(tls, "insecure") {
			q.Set("insecure", "1")
		}
		switch pins := tls["certificate_public_key_sha256"].(type) {
		case []string:
			if len(pins) > 0 {
				q.Set("pinSHA256", pins[0])
			}
		case []interface{}:
			if len(pins) > 0 {
				if s, ok := pins[0].(string); ok && s != "" {
					q.Set("pinSHA256", s)
				}
			}
		}
		if alpn, ok := tls["alpn"].([]interface{}); ok && len(alpn) > 0 {
			parts := make([]string, 0, len(alpn))
			for _, a := range alpn {
				if s := mapGetString(map[string]interface{}{"v": a}, "v"); s != "" {
					parts = append(parts, s)
				}
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
			if s := mapGetString(map[string]interface{}{"v": v}, "v"); s != "" {
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
	if obfs := mapGetString(out, "obfs"); obfs != "" {
		q.Set("obfs", "xplus")
		q.Set("obfsParam", obfs)
	}
	if up := mapGetInt(out, "up_mbps"); up > 0 {
		q.Set("upmbps", strconv.Itoa(up))
	}
	if down := mapGetInt(out, "down_mbps"); down > 0 {
		q.Set("downmbps", strconv.Itoa(down))
	}
	shareAppendDetourLiteral(q, out)
	u := &url.URL{
		Scheme:   "hysteria",
		Host:     hostPort(server, port),
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}
