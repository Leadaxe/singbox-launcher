package subscription

import (
	"fmt"
	"net/url"
	"strconv"
)

// --- AnyTLS ---

// shareURIFromAnyTLS encodes a sing-box anytls outbound back into an anytls://
// URI (reverse of buildAnyTLSOutbound). The single credential (password) is the
// userinfo username, matching Trojan. Round-trip is by meaning, not byte-exact:
// insecure is emitted as the canonical `insecure=1`.
func shareURIFromAnyTLS(out map[string]interface{}) (string, error) {
	pass := mapGetString(out, "password")
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if pass == "" || server == "" || port <= 0 {
		return "", fmt.Errorf("%w: anytls needs password, server, server_port", ErrShareURINotSupported)
	}

	q := url.Values{}
	if v := mapGetString(out, "idle_session_check_interval"); v != "" {
		q.Set("idle_session_check_interval", v)
	}
	if v := mapGetString(out, "idle_session_timeout"); v != "" {
		q.Set("idle_session_timeout", v)
	}
	if n := mapGetInt(out, "min_idle_session"); n > 0 {
		q.Set("min_idle_session", strconv.Itoa(n))
	}
	if tls, ok := out["tls"].(map[string]interface{}); ok {
		if sni := mapGetString(tls, "server_name"); sni != "" {
			q.Set("sni", sni)
		}
		shareAppendALPNInsecure(q, tls) // alpn + insecure=1
		if utls, ok := tls["utls"].(map[string]interface{}); ok {
			if fp := mapGetString(utls, "fingerprint"); fp != "" {
				q.Set("fp", fp)
			}
		}
	}
	shareAppendDetourLiteral(q, out)

	u := &url.URL{
		Scheme:   "anytls",
		User:     url.User(pass),
		Host:     hostPort(server, port),
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}
