// Package config: outbound_jsonbuilder.go — low-level JSON string helpers used when emitting
// outbound/selector JSON via fmt.Sprintf + strings.Join (string-concat builder).
//
// These leaf helpers escape strings safely for JSON, sanitize // comment lines, and append the
// optional sing-box "transport" object. They are shared by the generators in outbound_generator.go.
package config

import (
	"encoding/json"
	"fmt"
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

// appendOutboundTransportParts appends a sing-box "transport" object from node.Outbound (VLESS, VMess, Trojan).
func appendOutboundTransportParts(parts []string, outbound map[string]interface{}) []string {
	if outbound == nil {
		return parts
	}
	transport, ok := outbound["transport"].(map[string]interface{})
	if !ok || len(transport) == 0 {
		return parts
	}
	var transportParts []string
	if tType, ok := transport["type"].(string); ok && tType != "" {
		transportParts = append(transportParts, fmt.Sprintf(`"type":%s`, marshalJSONString(tType)))
	}
	if path, ok := transport["path"].(string); ok && path != "" {
		transportParts = append(transportParts, fmt.Sprintf(`"path":%s`, marshalJSONString(path)))
	}
	switch hostVal := transport["host"].(type) {
	case string:
		if hostVal != "" {
			transportParts = append(transportParts, fmt.Sprintf(`"host":%s`, marshalJSONString(hostVal)))
		}
	case []string:
		if len(hostVal) > 0 {
			hostJSON, err := json.Marshal(hostVal)
			if err == nil {
				transportParts = append(transportParts, fmt.Sprintf(`"host":%s`, string(hostJSON)))
			}
		}
	}
	if serviceName, ok := transport["service_name"].(string); ok && serviceName != "" {
		transportParts = append(transportParts, fmt.Sprintf(`"service_name":%s`, marshalJSONString(serviceName)))
	}
	// XHTTP-specific fields (SPEC 071); only present when type=="xhttp".
	if mode, ok := transport["mode"].(string); ok && mode != "" {
		transportParts = append(transportParts, fmt.Sprintf(`"mode":%s`, marshalJSONString(mode)))
	}
	if pad, ok := transport["x_padding_bytes"].(string); ok && pad != "" {
		transportParts = append(transportParts, fmt.Sprintf(`"x_padding_bytes":%s`, marshalJSONString(pad)))
	}
	if v, ok := transport["no_grpc_header"].(bool); ok && v {
		transportParts = append(transportParts, `"no_grpc_header":true`)
	}
	// headers may arrive as map[string]string (ws Host) or map[string]interface{}
	// (URI/Raw-JSON path) — handle both so they aren't silently dropped.
	switch headers := transport["headers"].(type) {
	case map[string]string:
		if len(headers) > 0 {
			var headerParts []string
			for k, v := range headers {
				headerParts = append(headerParts, fmt.Sprintf(`%s:%s`, marshalJSONString(k), marshalJSONString(v)))
			}
			transportParts = append(transportParts, fmt.Sprintf(`"headers":{%s}`, strings.Join(headerParts, ",")))
		}
	case map[string]interface{}:
		var headerParts []string
		for k, v := range headers {
			vb, err := json.Marshal(v)
			if err != nil {
				continue
			}
			headerParts = append(headerParts, fmt.Sprintf(`%s:%s`, marshalJSONString(k), string(vb)))
		}
		if len(headerParts) > 0 {
			transportParts = append(transportParts, fmt.Sprintf(`"headers":{%s}`, strings.Join(headerParts, ",")))
		}
	}
	if len(transportParts) > 0 {
		transportJSON := "{" + strings.Join(transportParts, ",") + "}"
		parts = append(parts, fmt.Sprintf(`"transport":%s`, transportJSON))
	}
	return parts
}
