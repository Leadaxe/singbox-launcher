// Package config: outbound_jsonbuilder.go — low-level JSON string helpers used when emitting
// outbound/selector JSON via fmt.Sprintf + strings.Join (string-concat builder).
//
// These leaf helpers escape strings safely for JSON, sanitize // comment lines, and append the
// optional sing-box "transport" object. They are shared by the generators in outbound_generator.go.
package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// xhttpV2StringKeys are the SPEC 002 v2 string-valued XHTTP transport keys, in
// the deterministic order they are emitted into the sing-box "transport" object.
// All are already snake_case (the parser maps camelCase URL params onto these);
// the value is written verbatim, validation is the core's job. Range fields
// (sc_max_each_post_bytes, sc_min_posts_interval_ms) are strings here too — the
// parser normalized bare numbers to "min-max" form before they reached us.
var xhttpV2StringKeys = []string{
	"session_placement",
	"session_key",
	"seq_placement",
	"seq_key",
	"uplink_data_placement",
	"uplink_data_key",
	"uplink_chunk_size",
	"uplink_http_method",
	"x_padding_key",
	"x_padding_header",
	"x_padding_placement",
	"x_padding_method",
	"sc_max_each_post_bytes",
	"sc_min_posts_interval_ms",
	"sc_stream_up_server_secs",
}

// xhttpIntKeys are the XHTTP transport keys the core decodes as int64, not as a
// string like every other tuning field. Emitting them quoted makes sing-box
// reject the whole config ("cannot unmarshal string into … of type int64").
var xhttpIntKeys = []string{
	"sc_max_buffered_posts",
}

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
	// WebSocket early data (issue #96). Emitted right after path so the JSON is
	// deterministic for golden tests. max_early_data is read type-tolerantly: the
	// parser produces int, but a round-trip through state.json turns it into
	// float64. early_data_header_name pairs with it (must be Sec-WebSocket-Protocol
	// for Xray compatibility) and rides through as a plain string.
	if maxED := transportInt(transport["max_early_data"]); maxED > 0 {
		transportParts = append(transportParts, fmt.Sprintf(`"max_early_data":%d`, maxED))
	}
	if hdr, ok := transport["early_data_header_name"].(string); ok && hdr != "" {
		transportParts = append(transportParts, fmt.Sprintf(`"early_data_header_name":%s`, marshalJSONString(hdr)))
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
	// XHTTP-specific fields (SPEC 071 base + SPEC 002 v2); only present when
	// type=="xhttp". String fields are emitted in a fixed order so the JSON is
	// deterministic; bool fields are written only when true (their absence is
	// the core default).
	if mode, ok := transport["mode"].(string); ok && mode != "" {
		transportParts = append(transportParts, fmt.Sprintf(`"mode":%s`, marshalJSONString(mode)))
	}
	if pad, ok := transport["x_padding_bytes"].(string); ok && pad != "" {
		transportParts = append(transportParts, fmt.Sprintf(`"x_padding_bytes":%s`, marshalJSONString(pad)))
	}
	if v, ok := transport["no_grpc_header"].(bool); ok && v {
		transportParts = append(transportParts, `"no_grpc_header":true`)
	}
	for _, key := range xhttpV2StringKeys {
		if s, ok := transport[key].(string); ok && s != "" {
			transportParts = append(transportParts, fmt.Sprintf(`%s:%s`, marshalJSONString(key), marshalJSONString(s)))
		}
	}
	for _, key := range xhttpIntKeys {
		if n := transportInt(transport[key]); n > 0 {
			transportParts = append(transportParts, fmt.Sprintf(`%s:%d`, marshalJSONString(key), n))
		}
	}
	if v, ok := transport["x_padding_obfs_mode"].(bool); ok && v {
		transportParts = append(transportParts, `"x_padding_obfs_mode":true`)
	}
	if v, ok := transport["no_sse_header"].(bool); ok && v {
		transportParts = append(transportParts, `"no_sse_header":true`)
	}
	// xmux is an object, not a scalar: the core decodes it as a nested struct
	// whose members are strings except h_keep_alive_period (int). Values are
	// written verbatim — the parser already mapped them onto the core's
	// snake_case names, and validation is the core's job.
	if xmux, ok := transport["xmux"].(map[string]interface{}); ok && len(xmux) > 0 {
		keys := make([]string, 0, len(xmux))
		for k := range xmux {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var xmuxParts []string
		for _, k := range keys {
			vb, err := json.Marshal(xmux[k])
			if err != nil {
				continue
			}
			xmuxParts = append(xmuxParts, fmt.Sprintf(`%s:%s`, marshalJSONString(k), string(vb)))
		}
		if len(xmuxParts) > 0 {
			transportParts = append(transportParts, fmt.Sprintf(`"xmux":{%s}`, strings.Join(xmuxParts, ",")))
		}
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

// transportInt coerces a transport numeric field to int. Values built by the
// parser are int, but after a round-trip through state.json (JSON unmarshal into
// interface{}) they come back as float64 — and older/hand-edited state may carry
// json.Number. Anything else yields 0.
func transportInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0
		}
		return int(i)
	default:
		return 0
	}
}
