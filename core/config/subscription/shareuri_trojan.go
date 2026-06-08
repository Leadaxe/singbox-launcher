package subscription

import (
	"fmt"
	"net/url"
)

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
	shareAppendDetourLiteral(q, out)
	u := &url.URL{
		Scheme:   "trojan",
		User:     url.User(url.PathEscape(pass)),
		Host:     hostPort(server, port),
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}
