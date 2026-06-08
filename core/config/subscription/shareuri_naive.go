package subscription

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// --- Naive ---

// shareURIFromNaive is the reverse of buildNaiveOutbound / the naive ParseNode
// branch. Produces "naive+https://…" or "naive+quic://…" depending on whether
// the outbound has `"quic": true`. See SPECS/044-F-C-NAIVE_PROXY_PARSER/SPEC.md
// for the round-trip contract (padding is dropped, extra-headers keys sorted
// lexicographically for deterministic output).
func shareURIFromNaive(out map[string]interface{}) (string, error) {
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if server == "" || port <= 0 {
		return "", fmt.Errorf("%w: naive needs server, server_port", ErrShareURINotSupported)
	}

	scheme := "naive+https"
	if mapGetBool(out, "quic") {
		scheme = "naive+quic"
	}

	q := url.Values{}
	if hdrs, ok := out["extra_headers"].(map[string]interface{}); ok && len(hdrs) > 0 {
		// Sort keys for deterministic round-trip (Go map iteration is random).
		keys := make([]string, 0, len(hdrs))
		for k := range hdrs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			v := fmt.Sprint(hdrs[k])
			if strings.ContainsAny(v, "\r\n\x00") {
				// Defensive: the generator/parser should have rejected these
				// on the way in, but if someone hand-edited config.json and
				// put a newline in a header value, skip it rather than emit
				// a broken URI.
				continue
			}
			pairs = append(pairs, k+": "+v)
		}
		if len(pairs) > 0 {
			q.Set("extra-headers", strings.Join(pairs, "\r\n"))
		}
	}
	shareAppendDetourLiteral(q, out)

	user := mapGetString(out, "username")
	pass := mapGetString(out, "password")
	var ui *url.Userinfo
	switch {
	case user != "" && pass != "":
		ui = url.UserPassword(user, pass)
	case user != "" && pass == "":
		ui = url.User(user)
	case user == "" && pass != "":
		// Spec convention: password-only put in user slot (mirrors hysteria2).
		ui = url.User(pass)
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
