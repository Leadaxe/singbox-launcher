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
	// Версию протокола ссылка несёт СХЕМОЙ: у socks-ссылки параметра под неё
	// нет ни в одном диалекте. Пара с парсером обязательна — узел с version
	// 4/4a, отданный этой ссылкой, обязан вернуться собой же (round-trip
	// корпуса), а значит эмиттер и парсер читают одну таблицу.
	u := &url.URL{
		Scheme:   socksSchemeForVersion(mapGetString(out, "version")),
		User:     userinfo,
		Host:     hostPort(server, port),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}
