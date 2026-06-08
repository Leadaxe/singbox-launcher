package subscription

import (
	"fmt"
	"net/url"
)

// --- SOCKS ---

func shareURIFromSocks(out map[string]interface{}) (string, error) {
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if server == "" || port <= 0 {
		return "", fmt.Errorf("%w: socks needs server, server_port", ErrShareURINotSupported)
	}
	user := mapGetString(out, "username")
	pass := mapGetString(out, "password")
	var userinfo *url.Userinfo
	if user != "" || pass != "" {
		userinfo = url.UserPassword(user, pass)
	}
	u := &url.URL{
		Scheme:   "socks5",
		User:     userinfo,
		Host:     hostPort(server, port),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}
