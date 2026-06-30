package subscription

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// queryGetFold returns the first value for a query key, matching case-insensitively.
// Subscriptions use allowinsecure=0, AllowInsecure=1, etc.
func queryGetFold(q url.Values, name string) string {
	for k, vs := range q {
		if strings.EqualFold(k, name) && len(vs) > 0 {
			return vs[0]
		}
	}
	return ""
}

// normalizePercentDecodeLoop applies URL-unescape until stable (fixes multiply-encoded alpn, etc.).
func normalizePercentDecodeLoop(s string) string {
	for {
		dec, err := url.QueryUnescape(s)
		if err != nil || dec == s {
			break
		}
		s = dec
	}
	return s
}

func tlsInsecureTrue(q url.Values) bool {
	for _, key := range []string{"insecure", "allowInsecure", "allowinsecure"} {
		v := strings.TrimSpace(strings.ToLower(queryGetFold(q, key)))
		if v == "1" || v == "true" || v == "yes" {
			return true
		}
	}
	return false
}

// NormalizeUTLSFingerprint maps subscription variants to sing-box utls names (lowercase).
// sing-box rejects values like "QQ"; the canonical name is "qq".
func NormalizeUTLSFingerprint(fp string) string {
	fp = strings.TrimSpace(strings.ToLower(fp))
	if fp == "" {
		return ""
	}
	return fp
}

// plaintextVLESSPorts are common subscription ports where TLS is typically off (plain HTTP / CF HTTP).
var plaintextVLESSPorts = map[int]struct{}{
	80: {}, 8080: {}, 8880: {}, 2052: {}, 2082: {}, 2086: {}, 2095: {},
}

func shouldVLESSSkipTLSForPort(port int) bool {
	_, ok := plaintextVLESSPorts[port]
	return ok
}

// uriTransportFromQuery builds sing-box V2Ray transport for VLESS/Trojan from URI query.
// See: https://sing-box.sagernet.org/configuration/shared/v2ray-transport/
func uriTransportFromQuery(q url.Values) (map[string]interface{}, bool) {
	typ := strings.ToLower(strings.TrimSpace(queryGetFold(q, "type")))
	headerType := strings.ToLower(strings.TrimSpace(queryGetFold(q, "headerType")))

	// Xray: TCP/raw with HTTP header camouflage → sing-box "http" transport (not plain TCP).
	if (typ == "raw" || typ == "tcp") && headerType == "http" {
		t := map[string]interface{}{"type": "http"}
		if p := queryGetFold(q, "path"); p != "" {
			t["path"] = p
		}
		if host := queryGetFold(q, "host"); host != "" {
			t["host"] = []string{host}
		}
		return t, true
	}

	switch typ {
	case "ws":
		t := map[string]interface{}{"type": "ws"}
		if p := queryGetFold(q, "path"); p != "" {
			t["path"] = p
		}
		// Many subscriptions set only sni= for TLS; reverse proxies expect WS Host to match vhost.
		host := strings.TrimSpace(queryGetFold(q, "host"))
		if host == "" {
			host = strings.TrimSpace(queryGetFold(q, "sni"))
		}
		if host == "" {
			host = strings.TrimSpace(queryGetFold(q, "obfsParam"))
		}
		if host != "" {
			t["headers"] = map[string]string{"Host": host}
		}
		return t, true
	case "grpc":
		t := map[string]interface{}{"type": "grpc"}
		sn := queryGetFold(q, "serviceName")
		if sn == "" {
			sn = queryGetFold(q, "service_name")
		}
		if sn != "" {
			t["service_name"] = sn
		} else if p := queryGetFold(q, "path"); p != "" {
			t["service_name"] = p
		}
		return t, true
	case "http":
		// HTTP transport: "host" is a list in sing-box (not a plain Host header).
		t := map[string]interface{}{"type": "http"}
		if p := queryGetFold(q, "path"); p != "" {
			t["path"] = p
		}
		if host := queryGetFold(q, "host"); host != "" {
			t["host"] = []string{host}
		}
		return t, true
	case "xhttp":
		// Xray "xhttp" (splithttp) → sing-box-lx "xhttp" transport. Distinct
		// wire protocol from httpupgrade; requires a core built with_xhttp
		// (sing-box-lx). See SPEC 071.
		return xhttpTransportFromQuery(q), true
	case "httpupgrade":
		// sing-box "httpupgrade" (HTTP/1.1 Upgrade). Kept separate from xhttp.
		t := map[string]interface{}{"type": "httpupgrade"}
		if p := queryGetFold(q, "path"); p != "" {
			t["path"] = p
		}
		if host := queryGetFold(q, "host"); host != "" {
			t["host"] = host
		}
		return t, true
	case "raw", "tcp", "":
		return nil, false
	default:
		return nil, false
	}
}

// xhttpStringField maps a transport JSON key (snake_case) to the URL spellings
// it may arrive under. The first non-empty source wins; queryGetFold already
// folds case, so we only list distinct spellings (snake vs camelCase).
type xhttpStringField struct {
	jsonKey string
	urlKeys []string
}

// xhttpStringFields are the v2 string-valued XHTTP transport fields (SPEC 002 v2,
// PARAM_MAP). mode/path/host are handled separately (path needs ?-tail trimming,
// host falls back differently); these are pure passthrough — read as-is, emit
// under jsonKey. Value validation against the allowed sets is left to the core.
var xhttpStringFields = []xhttpStringField{
	{"session_placement", []string{"session_placement", "sessionPlacement"}},
	{"session_key", []string{"session_key", "sessionKey"}},
	{"seq_placement", []string{"seq_placement", "seqPlacement"}},
	{"seq_key", []string{"seq_key", "seqKey"}},
	{"uplink_data_placement", []string{"uplink_data_placement", "uplinkDataPlacement"}},
	{"uplink_data_key", []string{"uplink_data_key", "uplinkDataKey"}},
	{"uplink_chunk_size", []string{"uplink_chunk_size", "uplinkChunkSize"}},
	{"uplink_http_method", []string{"uplink_http_method", "uplinkHTTPMethod"}},
	{"x_padding_key", []string{"x_padding_key", "xPaddingKey"}},
	{"x_padding_header", []string{"x_padding_header", "xPaddingHeader"}},
	{"x_padding_placement", []string{"x_padding_placement", "xPaddingPlacement"}},
	{"x_padding_method", []string{"x_padding_method", "xPaddingMethod"}},
}

// xhttpRangeFields are sc*-fields the core expects as a "min-max" string but
// which real subscriptions often send as a bare number (or a float like 30.0)
// in the extra-JSON. xhttpGet normalizes those to strings before we read them.
var xhttpRangeFields = []xhttpStringField{
	{"sc_max_each_post_bytes", []string{"sc_max_each_post_bytes", "scMaxEachPostBytes"}},
	{"sc_min_posts_interval_ms", []string{"sc_min_posts_interval_ms", "scMinPostsIntervalMs"}},
}

// xhttpTransportFromQuery builds a sing-box-lx "xhttp" (Xray splithttp) transport
// from a VLESS/Trojan/VMess URI query. Distinct from "httpupgrade". Covers the
// full SPEC 002 v2 field set: the base trio (mode/path/host), padding, placement
// and key fields, x-padding obfs, and packet-up tuning. Values come from two
// sources merged into one lookup: flat query params and the `extra` URL-encoded
// JSON (extra wins for its keys). Value normalization is otherwise left to the
// core. See SPEC 071 / sing-box-lx SPEC 002.
func xhttpTransportFromQuery(q url.Values) map[string]interface{} {
	src := xhttpMergeSource(q)
	t := map[string]interface{}{"type": "xhttp"}

	if v := strings.TrimSpace(xhttpGet(src, q, "mode")); v != "" {
		t["mode"] = v
	}
	if p := xhttpCleanPath(xhttpGet(src, q, "path")); p != "" {
		t["path"] = p
	}
	if host := xhttpGet(src, q, "host"); host != "" {
		t["host"] = host
	}
	if pad := xhttpGetAny(src, q, "x_padding_bytes", "xPaddingBytes"); pad != "" {
		t["x_padding_bytes"] = pad
	}
	if xhttpBool(src, q, "no_grpc_header", "noGRPCHeader") {
		t["no_grpc_header"] = true
	}
	if xhttpBool(src, q, "x_padding_obfs_mode", "xPaddingObfsMode") {
		t["x_padding_obfs_mode"] = true
	}
	for _, f := range xhttpStringFields {
		if v := xhttpGetAny(src, q, f.urlKeys...); v != "" {
			t[f.jsonKey] = v
		}
	}
	for _, f := range xhttpRangeFields {
		if v := xhttpRange(xhttpGetAny(src, q, f.urlKeys...)); v != "" {
			t[f.jsonKey] = v
		}
	}
	return t
}

// xhttpMergeSource decodes the `extra` query param (URL-encoded JSON) into a
// flat map of stringified values. Numbers become their canonical string ("30.0"
// → "30", "1000000" → "1000000"), bools become "true"/"false". Returns nil when
// there is no usable extra. Flat query params are read separately via xhttpGet,
// so this map only carries the extra-only keys.
func xhttpMergeSource(q url.Values) map[string]string {
	raw := strings.TrimSpace(queryGetFold(q, "extra"))
	if raw == "" {
		return nil
	}
	// queryGetFold returns the already percent-decoded value; the surviving
	// payload is the JSON object itself. Guard against double-encoded inputs.
	if !strings.HasPrefix(strings.TrimSpace(raw), "{") {
		if dec, err := url.QueryUnescape(raw); err == nil {
			raw = dec
		}
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil
	}
	out := make(map[string]string, len(obj))
	for k, v := range obj {
		out[k] = xhttpStringifyJSON(v)
	}
	return out
}

// xhttpStringifyJSON renders a JSON scalar from `extra` as the string sing-box
// wants. Floats drop a redundant ".0" (encoding/json decodes every JSON number
// as float64), so 30.0 → "30" and 1000000 → "1000000" rather than "1e+06".
func xhttpStringifyJSON(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		return strconv.FormatBool(val)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case nil:
		return ""
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// xhttpGet reads a single key, preferring the extra-JSON source over the flat
// query (SPEC 002 §1.5: extra wins for its keys). The flat lookup is
// case-insensitive via queryGetFold.
func xhttpGet(src map[string]string, q url.Values, key string) string {
	if src != nil {
		if v, ok := src[key]; ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return strings.TrimSpace(queryGetFold(q, key))
}

// xhttpGetAny tries each spelling in order and returns the first non-empty value
// from either source. Used for fields with both a snake_case and a camelCase
// spelling.
func xhttpGetAny(src map[string]string, q url.Values, keys ...string) string {
	for _, k := range keys {
		if v := xhttpGet(src, q, k); v != "" {
			return v
		}
	}
	return ""
}

// xhttpBool reads a flag under any of the given spellings, treating 1/true/yes
// as true (case-insensitive).
func xhttpBool(src map[string]string, q url.Values, keys ...string) bool {
	v := strings.ToLower(xhttpGetAny(src, q, keys...))
	return v == "1" || v == "true" || v == "yes"
}

// xhttpCleanPath strips a query-string tail from an XHTTP path. Real nodes ship
// path=/GaMeOpTiMiZeR?ed=2048 — the part after `?` is not the path (SPEC 002
// §4.1). The core normalizes the path itself, but the `?` is trimmed here.
func xhttpCleanPath(p string) string {
	p = strings.TrimSpace(p)
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	return p
}

// xhttpRange normalizes an sc*-range value to the "min-max" string the core
// wants. A bare number N is left as "N" (the core accepts "N" and "N-N" alike);
// xhttpStringifyJSON has already dropped any ".0" float tail. Empty stays empty.
func xhttpRange(v string) string {
	return strings.TrimSpace(v)
}

// maxRealityShortIDHexLen is the maximum hex character count sing-box accepts for outbound
// tls.reality.short_id (8 bytes). Longer values from broken lists are truncated.
const maxRealityShortIDHexLen = 16

// normalizeRealityShortID keeps only hex digits for sing-box REALITY short_id decoding.
// Public lists sometimes paste mojibake (e.g. UTF-8 bytes misread as Latin-1 → U+00C2 in sid),
// spaces, or punctuation; sing-box uses encoding/hex and fails on any non-hex rune.
func normalizeRealityShortID(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToValidUTF8(s, "")
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'a' && r <= 'f':
			b.WriteRune(r)
		case r >= 'A' && r <= 'F':
			b.WriteRune(r - 'A' + 'a')
		}
	}
	out := b.String()
	if len(out) > maxRealityShortIDHexLen {
		out = out[:maxRealityShortIDHexLen]
	}
	return out
}

// isValidRealityPublicKey reports whether pbk is a usable REALITY public_key.
// A REALITY key is an X25519 public key: 32 bytes, shared as base64url without
// padding (43 chars). Public lists sometimes paste junk into pbk (e.g. literal
// "enabled", "true", an empty token) while declaring security=tls — sing-box then
// rejects the whole config with "invalid public_key" and the VPN won't start at
// all. We treat any non-decodable / wrong-length value as "no reality" so the node
// degrades to plain TLS instead of poisoning the generated config.json.
func isValidRealityPublicKey(pbk string) bool {
	pbk = strings.TrimSpace(pbk)
	// REALITY uses base64url; tolerate a stray '=' pad and base64std variants.
	pbk = strings.TrimRight(pbk, "=")
	if len(pbk) != 43 {
		return false
	}
	if _, err := base64.RawURLEncoding.DecodeString(pbk); err == nil {
		return true
	}
	_, err := base64.RawStdEncoding.DecodeString(pbk)
	return err == nil
}

func applyTLSQueryExtras(q url.Values, tlsData map[string]interface{}) {
	if alpn := queryGetFold(q, "alpn"); alpn != "" {
		alpn = normalizePercentDecodeLoop(alpn)
		alpnList := strings.Split(alpn, ",")
		for i := range alpnList {
			alpnList[i] = strings.TrimSpace(alpnList[i])
		}
		tlsData["alpn"] = alpnList
	}
	if tlsInsecureTrue(q) {
		tlsData["insecure"] = true
	}
}

// vlessTLSFromNode returns sing-box tls map for VLESS and whether TLS should be included.
func vlessTLSFromNode(node *configtypes.ParsedNode) (map[string]interface{}, bool) {
	q := node.Query
	sec := strings.ToLower(strings.TrimSpace(queryGetFold(q, "security")))
	pbk := strings.TrimSpace(queryGetFold(q, "pbk"))

	if sec == "none" {
		return nil, false
	}

	sni := queryGetFold(q, "sni")
	if sni == "" {
		sni = queryGetFold(q, "peer")
	}
	if sni == "" {
		sni = node.Server
	}
	fp := NormalizeUTLSFingerprint(queryGetFold(q, "fp"))
	if fp == "" {
		fp = NormalizeUTLSFingerprint(queryGetFold(q, "fingerprint"))
	}
	if fp == "" {
		fp = "random"
	}

	// Only build a REALITY block when pbk is a usable X25519 public key. We gate on
	// the key itself, not on security=reality, because many real lists carry pbk
	// without an explicit security=reality (e.g. xhttp+reality nodes). Broken public
	// lists sometimes attach a junk pbk (e.g. "enabled") to a plain security=tls
	// node; emitting that as public_key makes sing-box reject the entire config
	// ("invalid public_key") and nothing starts. In that case fall through to plain
	// TLS below.
	if isValidRealityPublicKey(pbk) {
		tlsData := map[string]interface{}{
			"enabled":     true,
			"server_name": sni,
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": fp,
			},
			"reality": map[string]interface{}{
				"enabled":    true,
				"public_key": strings.TrimSpace(pbk),
				"short_id":   normalizeRealityShortID(queryGetFold(q, "sid")),
			},
		}
		applyTLSQueryExtras(q, tlsData)
		return tlsData, true
	}

	if sec == "reality" {
		tlsData := map[string]interface{}{
			"enabled":     true,
			"server_name": sni,
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": fp,
			},
		}
		applyTLSQueryExtras(q, tlsData)
		return tlsData, true
	}

	if sec == "" && shouldVLESSSkipTLSForPort(node.Port) {
		return nil, false
	}

	tlsData := map[string]interface{}{
		"enabled":     true,
		"server_name": sni,
		"utls": map[string]interface{}{
			"enabled":     true,
			"fingerprint": fp,
		},
	}
	applyTLSQueryExtras(q, tlsData)
	return tlsData, true
}

// trojanTLSFromNode returns TLS config for Trojan (WebSocket/raw over TLS).
func trojanTLSFromNode(node *configtypes.ParsedNode) map[string]interface{} {
	q := node.Query
	sec := strings.ToLower(strings.TrimSpace(queryGetFold(q, "security")))
	if sec == "none" {
		return map[string]interface{}{
			"enabled": false,
		}
	}

	sni := queryGetFold(q, "sni")
	if sni == "" {
		sni = queryGetFold(q, "peer")
	}
	if sni == "" {
		sni = queryGetFold(q, "host")
	}
	if sni == "" {
		sni = node.Server
	}

	tlsData := map[string]interface{}{
		"enabled":     true,
		"server_name": sni,
	}
	if fp := NormalizeUTLSFingerprint(queryGetFold(q, "fp")); fp != "" {
		tlsData["utls"] = map[string]interface{}{
			"enabled":     true,
			"fingerprint": fp,
		}
	}
	applyTLSQueryExtras(q, tlsData)
	return tlsData
}
