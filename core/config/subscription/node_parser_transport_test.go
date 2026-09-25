package subscription

import "testing"

// Чистка short_id и проверка pbk переехали в реестр (SPEC 131 W2d): их
// юниты сняты вместе с функциями. Правила проверяются там, где теперь живут —
// core/config/nodeflow (hex_only, base64_32) и корпус контракта
// (uri/vless/reality_sid_*, reality_pbk_*).

// Regression: a node carrying a raw uTLS identifier (fp=HelloChrome_120) made
// sing-box abort the entire config with "initialize outbound[N]: unknown uTLS
// fingerprint", so no node started. It must map onto the chrome family.
func TestParseNode_VLESS_RawUTLSIdentifierFingerprint(t *testing.T) {
	uri := "vless://a0ee37a5-1844-4087-bc5c-1db6f416d38c@example.com:443?encryption=none&security=tls&sni=example.com&fp=HelloChrome_120#t"
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: err=%v node=%v", err, node)
	}
	tls, ok := node.Outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatal("missing tls")
	}
	ut, ok := tls["utls"].(map[string]interface{})
	if !ok {
		t.Fatal("missing utls")
	}
	if ut["fingerprint"] != "chrome" {
		t.Fatalf("fingerprint = %#v, want chrome", ut["fingerprint"])
	}
}

// Замена мусорного отпечатка на chrome переехала в санитайзер
// (tls.json: on_invalid coerce chrome + utls_fp_unknown). Проверка —
// corpus uri/vless/utls_junk_fp_fallback и TestPipelineSetsDegradationCodes.

// Регрессия «pbk=enabled на security=tls ноде валит весь config.json»
// (broken-list-pbk-junk, v1.1.7) проверяется теперь на выходе конвейера:
// public_key в реестре required с форматом base64_32, поэтому мусорный ключ
// снимает блок reality целиком. См. TestPipelineSetsDegradationCodes
// («мусорный pbk снимает весь блок reality») и corpus
// uri/vless/reality_pbk_junk_on_tls.
