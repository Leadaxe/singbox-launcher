package subscription

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
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

// singboxUTLSFingerprints are the names sing-box accepts in tls.utls.fingerprint.
// Mirrors uTLSClientHelloID in sing-box common/tls/utls_client.go — anything else
// aborts config load with "unknown uTLS fingerprint".
var singboxUTLSFingerprints = map[string]struct{}{
	"chrome": {}, "firefox": {}, "edge": {}, "safari": {},
	"360": {}, "qq": {}, "ios": {}, "android": {},
	"random": {}, "randomized": {},
	// Chrome ClientHello variants sing-box maps onto HelloChrome_Auto.
	"chrome_psk": {}, "chrome_psk_shuffle": {}, "chrome_padding_psk_shuffle": {},
	"chrome_pq": {}, "chrome_pq_psk": {},
}

// utlsAliasPrefixes maps uTLS library ClientHelloID names (HelloChrome_120,
// HelloFirefox_Auto, …) onto the browser family sing-box understands. Some lists
// export the Go identifier verbatim instead of the sing-box name.
var utlsAliasPrefixes = []struct {
	prefix string
	name   string
}{
	{"hellochrome", "chrome"},
	{"hellofirefox", "firefox"},
	{"helloedge", "edge"},
	{"hellosafari", "safari"},
	{"helloios", "ios"},
	{"helloandroid", "android"},
	{"hello360", "360"},
	{"helloqq", "qq"},
	{"hellorandomized", "randomized"},
	{"hellorandom", "random"},
}

// NormalizeUTLSFingerprint maps subscription variants to sing-box utls names (lowercase).
// sing-box rejects values like "QQ"; the canonical name is "qq".
//
// Values outside the sing-box allowlist are dropped (""), not passed through: a single
// node carrying e.g. fp=HelloChrome_120 made sing-box abort the whole config with
// "initialize outbound[N]: unknown uTLS fingerprint" so the VPN never started. Callers
// treat "" as "no utls block", degrading that node instead of poisoning config.json.
func NormalizeUTLSFingerprint(fp string) string {
	canon, _ := normalizeUTLSFingerprintEx(fp)
	return canon
}

// normalizeUTLSFingerprintEx additionally reports whether a non-empty value was
// junk. Junk must not silently become something else: the fingerprint is client
// -side camouflage the server never checks, so the node itself is almost
// certainly fine — it gets `chrome` plus a warning rather than a dropped utls
// block or a random per-start identity (SPEC 103, D-029).
func normalizeUTLSFingerprintEx(fp string) (canon string, junk bool) {
	fp = strings.TrimSpace(strings.ToLower(fp))
	if fp == "" {
		return "", false
	}
	if _, ok := singboxUTLSFingerprints[fp]; ok {
		return fp, false
	}
	// uTLS Go identifiers: HelloChrome_120, hellofirefox_auto, HelloChrome-106, …
	bare := strings.NewReplacer("_", "", "-", "", " ", "").Replace(fp)
	for _, alias := range utlsAliasPrefixes {
		if strings.HasPrefix(bare, alias.prefix) {
			return alias.name, false
		}
	}
	return "", true
}

// utlsFingerprintOrFallback resolves the fingerprint for a node: canonical value
// as-is, junk → `chrome` with a warning, absent → empty (callers decide their
// own default). See D-029.
func utlsFingerprintOrFallback(raw string) string {
	canon, junk := normalizeUTLSFingerprintEx(raw)
	if junk {
		debuglog.WarnLog("Parser: unknown uTLS fingerprint %q — using %q instead (fingerprint is client-side camouflage; the node stays)", raw, utlsJunkFallback)
		return utlsJunkFallback
	}
	return canon
}

// utlsJunkFallback — canonical replacement for an unrecognized fingerprint.
const utlsJunkFallback = "chrome"

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
		// path may carry Xray's `?ed=N` early-data tail; split it into the
		// sing-box max_early_data / early_data_header_name fields (issue #96).
		if p := queryGetFold(q, "path"); p != "" {
			applyWSEarlyData(t, p)
		}
		// Second spelling seen in the wild: flat `ed`/`eh` query params. The
		// path tail wins — it addresses one path, the flat pair the whole link
		// (SPEC 103, §9.E). `eh` without `ed` means nothing: the core enables
		// early data on max_early_data > 0.
		if _, already := t["max_early_data"]; !already {
			if ed, err := strconv.Atoi(strings.TrimSpace(queryGetFold(q, "ed"))); err == nil && ed > 0 {
				t["max_early_data"] = ed
				header := strings.TrimSpace(queryGetFold(q, "eh"))
				if header == "" {
					header = wsEarlyDataHeaderName
				}
				t["early_data_header_name"] = header
			}
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
			// httpupgrade has no early data in sing-box: strip the Xray `?ed=N`
			// tail (and any residual encoding) instead of shipping it inside the
			// path, which the server answers with 404 (SPEC 103, D-028).
			clean, _ := splitWSEarlyData(decodeResidualPercent(p))
			if clean == "" {
				clean = p
			}
			t["path"] = clean
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
	{"sc_stream_up_server_secs", []string{"sc_stream_up_server_secs", "scStreamUpServerSecs"}},
}

// xhttpIntFields are XHTTP fields the core decodes as int64 rather than as a
// string, so they must reach the transport as a number (SPEC 102).
var xhttpIntFields = []xhttpStringField{
	{"sc_max_buffered_posts", []string{"sc_max_buffered_posts", "scMaxBufferedPosts"}},
}

// xhttpBoolFields are the XHTTP flags emitted only when true; the core's default
// is the absent field.
var xhttpBoolFields = []xhttpStringField{
	{"no_grpc_header", []string{"no_grpc_header", "noGRPCHeader"}},
	{"no_sse_header", []string{"no_sse_header", "noSSEHeader"}},
	{"x_padding_obfs_mode", []string{"x_padding_obfs_mode", "xPaddingObfsMode"}},
}

// xhttpXmuxFields maps Xray's xmux members onto the core's snake_case names.
// h_keep_alive_period is an int for the core; the rest are strings (usually
// "min-max" ranges).
var xhttpXmuxFields = []xhttpStringField{
	{"max_concurrency", []string{"max_concurrency", "maxConcurrency"}},
	{"max_connections", []string{"max_connections", "maxConnections"}},
	{"c_max_reuse_times", []string{"c_max_reuse_times", "cMaxReuseTimes"}},
	{"h_max_request_times", []string{"h_max_request_times", "hMaxRequestTimes"}},
	{"h_max_reusable_secs", []string{"h_max_reusable_secs", "hMaxReusableSecs"}},
}

// xhttpXmuxIntFields are the xmux members the core decodes as int.
var xhttpXmuxIntFields = []xhttpStringField{
	{"h_keep_alive_period", []string{"h_keep_alive_period", "hKeepAlivePeriod"}},
}

// xhttpTransportFromQuery builds a sing-box-lx "xhttp" (Xray splithttp) transport
// from a VLESS/Trojan/VMess URI query. Distinct from "httpupgrade". Covers the
// full SPEC 002 v2 field set: the base trio (mode/path/host), padding, placement
// and key fields, x-padding obfs, and packet-up tuning. Values come from two
// sources merged into one lookup: flat query params and the `extra` URL-encoded
// JSON (extra wins for its keys). Value normalization is otherwise left to the
// core. See SPEC 071 / sing-box-lx SPEC 002.
func xhttpTransportFromQuery(q url.Values) map[string]interface{} {
	return xhttpBuildTransport(xhttpMergeSource(q), xhttpFlattenQuery(q))
}

// xhttpBuildTransport is the single place where an XHTTP transport object is
// assembled, shared by the share-URI parser and the Xray-JSON converter so both
// branches support exactly the same field set (SPEC 102 R2). Values arrive
// pre-stringified in two layers: `primary` wins over `fallback` for keys present
// in both (SPEC 002 §1.5 — Xray's `extra` overrides the flat settings).
//
// Callers are responsible for flattening their own source into these maps:
// the URI branch decodes the `extra` JSON and folds the query string, the Xray
// branch flattens `xhttpSettings` and its nested `extra` object.
func xhttpBuildTransport(primary, fallback map[string]string) map[string]interface{} {
	t := map[string]interface{}{"type": "xhttp"}

	if v := xhttpLookup(primary, fallback, "mode"); v != "" {
		t["mode"] = v
	}
	if p := xhttpCleanPath(xhttpLookup(primary, fallback, "path")); p != "" {
		t["path"] = p
	}
	if host := xhttpLookup(primary, fallback, "host"); host != "" {
		t["host"] = host
	}
	if pad := xhttpLookup(primary, fallback, "x_padding_bytes", "xPaddingBytes"); pad != "" {
		// "0-0" is a meaningful value (padding disabled), not an empty field, so
		// the guard tests for a non-empty string rather than a non-zero range.
		t["x_padding_bytes"] = pad
	}
	for _, f := range xhttpBoolFields {
		if xhttpLookupBool(primary, fallback, f.urlKeys...) {
			t[f.jsonKey] = true
		}
	}
	for _, f := range xhttpStringFields {
		if v := xhttpLookup(primary, fallback, f.urlKeys...); v != "" {
			t[f.jsonKey] = v
		}
	}
	for _, f := range xhttpRangeFields {
		if v := xhttpRange(xhttpLookup(primary, fallback, f.urlKeys...)); v != "" {
			t[f.jsonKey] = v
		}
	}
	for _, f := range xhttpIntFields {
		if n, ok := xhttpLookupInt(primary, fallback, f.urlKeys...); ok {
			t[f.jsonKey] = n
		}
	}
	if xmux := xhttpXmuxFromSource(primary, fallback); len(xmux) > 0 {
		t["xmux"] = xmux
	}
	return t
}

// xhttpXmuxFromSource assembles the nested xmux object. Xray ships it as a
// sub-object of `extra`; callers flatten it into the same layers under its own
// member names, so the lookup is identical to the top-level fields.
func xhttpXmuxFromSource(primary, fallback map[string]string) map[string]interface{} {
	xmux := make(map[string]interface{}, len(xhttpXmuxFields))
	for _, f := range xhttpXmuxFields {
		if v := xhttpLookup(primary, fallback, f.urlKeys...); v != "" {
			xmux[f.jsonKey] = v
		}
	}
	for _, f := range xhttpXmuxIntFields {
		if n, ok := xhttpLookupInt(primary, fallback, f.urlKeys...); ok {
			xmux[f.jsonKey] = n
		}
	}
	if len(xmux) == 0 {
		return nil
	}
	return xmux
}

// xhttpLookupInt reads a numeric field. Values arrive as strings from both
// sources (the JSON layers are stringified on the way in), so "30" and "30.0"
// both yield 30. Reports false when the key is absent or not a number, leaving
// the field unset rather than writing a zero the core would act on.
func xhttpLookupInt(primary, fallback map[string]string, keys ...string) (int, bool) {
	raw := xhttpLookup(primary, fallback, keys...)
	if raw == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return n, true
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return int(f), true
	}
	return 0, false
}

// xhttpFlattenQuery folds a URL query into the flat string map the shared
// builder consumes. Repeated keys keep the first value, matching queryGetFold.
func xhttpFlattenQuery(q url.Values) map[string]string {
	if len(q) == 0 {
		return nil
	}
	out := make(map[string]string, len(q))
	for k, vs := range q {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

// xhttpLookup returns the first non-empty value for any of the given key
// spellings, preferring the primary layer over the fallback for each spelling in
// turn. Keys are matched case-insensitively, so a subscription may ship either
// camelCase or snake_case.
func xhttpLookup(primary, fallback map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := xhttpMapGetFold(primary, k); v != "" {
			return v
		}
		if v := xhttpMapGetFold(fallback, k); v != "" {
			return v
		}
	}
	return ""
}

// xhttpMapGetFold reads a key from a flat map case-insensitively. The exact hit
// is tried first so the common path avoids scanning the map.
func xhttpMapGetFold(m map[string]string, key string) string {
	if len(m) == 0 {
		return ""
	}
	if v, ok := m[key]; ok {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
	}
	return ""
}

// xhttpLookupBool reads a flag under any of the given spellings, treating
// 1/true/yes as true (case-insensitive).
func xhttpLookupBool(primary, fallback map[string]string, keys ...string) bool {
	v := strings.ToLower(xhttpLookup(primary, fallback, keys...))
	return v == "1" || v == "true" || v == "yes"
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

// wsEarlyDataHeaderName is the header sing-box must use to stay compatible with
// Xray-core WebSocket 0-RTT. Xray sends early data in this header; with an empty
// early_data_header_name sing-box would instead append it to the path (the V2Ray
// convention), which an Xray server does not understand. See the sing-box
// v2ray-transport docs and issue #96.
const wsEarlyDataHeaderName = "Sec-WebSocket-Protocol"

// splitWSEarlyData separates Xray's `?ed=N` tail from a WebSocket path. Xray
// encodes WebSocket Early Data into the path (`/api/v2/channel?ed=2560`) instead
// of a dedicated field, but sing-box treats the whole string as a literal path
// and the server answers 404 (issue #96). We return the clean path plus the
// max_early_data value (0 = not set / not a number).
//
// The `?`-tail is always stripped from the path — any query tail breaks the WS
// route match — but a malformed `ed` (missing, non-numeric) only zeroes the
// early-data value rather than dropping the node, matching how we degrade other
// broken share-URI fields instead of poisoning the config.
func splitWSEarlyData(path string) (string, int) {
	path = strings.TrimSpace(path)
	i := strings.IndexByte(path, '?')
	if i < 0 {
		return path, 0
	}
	tail := path[i+1:]
	clean := path[:i]
	q, err := url.ParseQuery(tail)
	if err != nil {
		return clean, 0
	}
	// Xray writes lowercase `ed`, but read it case-insensitively to match how the
	// rest of the parser folds query keys (queryGetFold).
	ed := strings.TrimSpace(queryGetFold(q, "ed"))
	if ed == "" {
		return clean, 0
	}
	n, err := strconv.Atoi(ed)
	if err != nil || n <= 0 {
		return clean, 0
	}
	return clean, n
}

// applyWSEarlyData sets the WebSocket transport path and, when a positive
// max_early_data was parsed from the Xray `?ed=N` tail, the two sing-box early
// data fields. Shared by every WS transport builder (URI, Xray JSON, VMess) so
// the `?ed=` conversion stays consistent. An empty path leaves the key unset.
func applyWSEarlyData(tr map[string]interface{}, rawPath string) {
	clean, maxED := splitWSEarlyData(decodeResidualPercent(rawPath))
	if clean != "" {
		tr["path"] = clean
	}
	if maxED > 0 {
		tr["max_early_data"] = maxED
		tr["early_data_header_name"] = wsEarlyDataHeaderName
	}
}

// decodeResidualPercent strips leftover percent-encoding from a path that was
// encoded twice by the provider's panel (`path=%2F%252Fassignment`). The core
// hands the path to the server verbatim (transport/v2raywebsocket/client.go →
// net/url.setPath), so a leftover `%2F` travels as `%252F` and the server 404s
// on a path it never published. Bounded to two passes, matching LxBox
// (transport.dart decodeResidualPercent) — SPEC 103, D-028.
func decodeResidualPercent(raw string) string {
	v := raw
	for i := 0; i < 2; i++ {
		if !strings.Contains(v, "%") {
			break
		}
		dec, err := url.QueryUnescape(v)
		if err != nil || dec == v {
			break
		}
		v = dec
	}
	return v
}

// appendEarlyDataToPath re-encodes a positive max_early_data back into an Xray
// `?ed=N` path tail. Used by the share-URI exporters so a node → share-link →
// node round-trip preserves WebSocket early data (the inverse of splitWSEarlyData).
func appendEarlyDataToPath(path string, maxED int) string {
	if maxED <= 0 {
		return path
	}
	sep := "?"
	if strings.ContainsRune(path, '?') {
		sep = "&"
	}
	return path + sep + "ed=" + strconv.Itoa(maxED)
}

// xhttpRange normalizes an sc*-range value to the "min-max" string the core
// wants. A bare number N is left as "N" (the core accepts "N" and "N-N" alike);
// xhttpStringifyJSON has already dropped any ".0" float tail. Empty stays empty.
func xhttpRange(v string) string {
	return strings.TrimSpace(v)
}

// maxRealityShortIDHexLen is the maximum hex character count sing-box accepts for outbound
// tls.reality.short_id (8 bytes, common/tls/reality_client.go: `decodedLen > 8` → E.New
// "invalid short_id", a fatal error for the whole config, not a per-node skip).
const maxRealityShortIDHexLen = 16

// normalizeRealityShortID keeps only hex digits for sing-box REALITY short_id decoding.
// Public lists sometimes paste mojibake (e.g. UTF-8 bytes misread as Latin-1 → U+00C2 in sid),
// spaces, or punctuation; sing-box uses encoding/hex and fails on any non-hex rune.
//
// SPEC 103 D-032: a value that is already invalid before truncation — odd hex length
// (encoding/hex: "odd length hex string", fatal) or more than 16 hex chars (decodes to
// >8 bytes, fatal) — is not "the same short_id, just longer": truncating it produces a
// DIFFERENT short_id the subscription never specified, silently corrupting the node
// instead of degrading it. So validity is checked on the filtered value BEFORE any
// truncation; an invalid value is dropped to "" (node stays, without REALITY sid) rather
// than truncated. Canon = the model LxBox already used.
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
	if len(out)%2 != 0 || len(out) > maxRealityShortIDHexLen {
		debuglog.WarnLog("Parser: reality short_id %q is not a valid ≤16-char even-length hex string — dropping it, node stays", s)
		return ""
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
	fp := utlsFingerprintOrFallback(queryGetFold(q, "fp"))
	if fp == "" {
		fp = utlsFingerprintOrFallback(queryGetFold(q, "fingerprint"))
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

// trojanTLSFromNode returns the sing-box tls map for Trojan (WebSocket/raw over
// TLS) and whether a tls block should be emitted at all.
//
// security=none omits the key entirely rather than emitting
// `"tls":{"enabled":false}` — same contract as vlessTLSFromNode. The explicit
// disabled block is what sing-box cores 1.14.0-lx.5..lx.18 crash on: the
// upstream ECH-retry commit builds a TLS dialer whenever a tls block is
// present, while the config constructor returns (nil, nil) for enabled:false,
// so the dialer wraps a nil config and SIGSEGVs on the first dial — URL test
// included, killing the whole core process (sing-box-lx SPEC 045). Omitting
// the key yields the same plain-TCP dial on every core version.
func trojanTLSFromNode(node *configtypes.ParsedNode) (map[string]interface{}, bool) {
	q := node.Query
	sec := strings.ToLower(strings.TrimSpace(queryGetFold(q, "security")))
	if sec == "none" {
		return nil, false
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
	if fp := utlsFingerprintOrFallback(queryGetFold(q, "fp")); fp != "" {
		tlsData["utls"] = map[string]interface{}{
			"enabled":     true,
			"fingerprint": fp,
		}
	}
	applyTLSQueryExtras(q, tlsData)
	return tlsData, true
}
