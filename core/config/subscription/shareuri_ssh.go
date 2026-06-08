package subscription

import (
	"fmt"
	"net/url"
	"strings"
)

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
	shareAppendDetourLiteral(q, out)
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
