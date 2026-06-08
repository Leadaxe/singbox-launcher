package subscription

import (
	"encoding/base64"
	"fmt"
	"net/url"
)

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
