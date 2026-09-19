// Package config: outbound_jsonbuilder.go — low-level JSON string helpers used when emitting
// outbound/selector JSON via fmt.Sprintf + strings.Join (string-concat builder).
//
// These leaf helpers escape strings safely for JSON, sanitize // comment lines, and append the
// optional sing-box "transport" object. They are shared by the generators in outbound_generator.go.
package config

import (
	"encoding/json"
	"strings"
)

// marshalJSONString returns s as a JSON string literal (including quotes).
// encoding/json replaces invalid UTF-8 with U+FFFD, unlike fmt %q / strconv.Quote which can emit
// escapes that are invalid in JSON (e.g. \xNN) and break sing-box decode.
func marshalJSONString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

// sanitizeOutboundLineComment removes newlines so a // comment does not swallow the next JSON line
// (subscription fragments may contain raw line breaks). Invalid UTF-8 is replaced so the whole
// config file stays valid UTF-8 for strict decoders (comments are not JSON string literals).
func sanitizeOutboundLineComment(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.TrimSpace(s)
	return strings.ToValidUTF8(s, "\uFFFD")
}

// outboundHasTransport reports whether the outbound carries a non-empty v2ray
// transport (ws/grpc/http/httpupgrade/xhttp/…). Used to suppress VLESS `flow`,
// which sing-box accepts only over bare TLS/Reality (no transport).
func outboundHasTransport(outbound map[string]interface{}) bool {
	if outbound == nil {
		return false
	}
	transport, ok := outbound["transport"].(map[string]interface{})
	if !ok || len(transport) == 0 {
		return false
	}
	tType, _ := transport["type"].(string)
	return tType != ""
}
