package subscription

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"singbox-launcher/core/config/registry"
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

// isValidShadowsocksMethod — знает ли ядро такой шифр shadowsocks.
//
// Словарь берётся ИЗ РЕЕСТРА (allowlists.json, ss_methods), а не из списка в
// коде: прежний список держал 9 значений против 18 у ядра и ДРОПАЛ УЗЕЛ на
// девяти рабочих legacy-шифрах (rc4-md5, aes-*-cfb/ctr, chacha20-ietf,
// xchacha20) — оба клиента были строже ядра, и пользователь не получал ни
// узла, ни объяснения (DRIFT §7.10, решение владельца 18.09.2026).
//
// На входе узла этой проверки нет вовсе: там решает санитайзер по правилу
// реестра (drop_node с кодом ss_method_invalid, а legacy — живой узел с
// info-кодом ss_method_legacy). Единственный вызывающий — эмиттер share-URI,
// которому нужно знать, можно ли вообще выразить узел ссылкой; поэтому
// функция и живёт рядом с ним, а не в парсере, которого больше нет.
func isValidShadowsocksMethod(method string) bool {
	for _, v := range registry.MustGet().Allowlist("ss_methods") {
		if v == method {
			return true
		}
	}
	return false
}
