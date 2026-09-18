package subscription

import "singbox-launcher/core/config/registry"

// isValidShadowsocksMethod — знает ли ядро такой шифр shadowsocks.
//
// Словарь берётся ИЗ РЕЕСТРА (allowlists.json, ss_methods), а не из списка в
// коде: прежний список держал 9 значений против 18 у ядра и ДРОПАЛ УЗЕЛ на
// девяти рабочих legacy-шифрах (rc4-md5, aes-*-cfb/ctr, chacha20-ietf,
// xchacha20) — оба клиента были строже ядра, и пользователь не получал ни
// узла, ни объяснения (DRIFT §7.10, решение владельца 18.09.2026).
//
// На входе узла эта проверка больше не стоит вовсе: там решает санитайзер по
// правилу реестра (drop_node с кодом ss_method_invalid, а legacy — живой узел
// с info-кодом ss_method_legacy). Остался один вызывающий — эмиттер share-URI,
// которому нужно знать, можно ли вообще выразить узел ссылкой.
func isValidShadowsocksMethod(method string) bool {
	for _, v := range registry.MustGet().Allowlist("ss_methods") {
		if v == method {
			return true
		}
	}
	return false
}
