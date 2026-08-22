package subscription

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// --- HTTP(S) CONNECT proxy ---

// shareURIFromHTTP is the reverse of parseHTTPProxyURI / GenerateNodeJSON's
// "http" branch. Scheme is chosen by TLS presence: proxy-https:// when the
// outbound carries an enabled tls block, proxy-http:// otherwise (mirrors
// Dart's toUriHttp, node_spec_emit.dart).
func shareURIFromHTTP(out map[string]interface{}) (string, error) {
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if server == "" || port <= 0 {
		return "", fmt.Errorf("%w: http needs server, server_port", ErrShareURINotSupported)
	}

	user := mapGetString(out, "username")
	pass := mapGetString(out, "password")
	var ui *url.Userinfo
	switch {
	case user != "" && pass != "":
		ui = url.UserPassword(user, pass)
	case user != "" && pass == "":
		ui = url.User(user)
	case user == "" && pass != "":
		// Пароль без логина эмитится как ":pass" — ровно так его читает
		// парсер (node_parser_http.go: user | user:pass | :pass). Раньше
		// пароль клали в слот ИМЕНИ, и round-trip превращал password в
		// username: узел уходил к провайдеру с пустым паролем.
		ui = url.UserPassword("", pass)
	}

	q := url.Values{}
	if path := mapGetString(out, "path"); path != "" {
		q.Set("path", path)
	}
	if hdrs, ok := out["headers"].(map[string]interface{}); ok && len(hdrs) > 0 {
		// Sort keys for deterministic output (Go map iteration is random).
		keys := make([]string, 0, len(hdrs))
		for k := range hdrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			v := fmt.Sprint(hdrs[k])
			if strings.ContainsAny(v, "\r\n\x00") {
				continue
			}
			pairs = append(pairs, k+": "+v)
		}
		if len(pairs) > 0 {
			q.Set("headers", strings.Join(pairs, "\r\n"))
		}
	}

	secure := false
	if tls, ok := out["tls"].(map[string]interface{}); ok {
		if en, ok := tls["enabled"].(bool); !ok || en {
			secure = true
			trojanTLSToQuery(q, tls, server)
			// trojanTLSToQuery always sets "sni" (falling back to server); the
			// http share format only carries sni when it differs from the
			// default, so this stays consistent with the parser's own
			// server-as-fallback behavior on round-trip.
		}
	}
	shareAppendDetourLiteral(q, out)

	scheme := "proxy-http"
	if secure {
		scheme = "proxy-https"
	}

	u := &url.URL{
		Scheme:   scheme,
		User:     ui,
		Host:     hostPort(server, port),
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}
