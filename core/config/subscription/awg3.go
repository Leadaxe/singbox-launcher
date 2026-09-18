package subscription

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// AmneziaWG 3.x (SPEC 123). Amnezia exports an AWG 3.0/3.1 server as the
// `amnezia-awg2` container with `protocol_version: "3.1"`; its .conf carries,
// on top of the AWG2 set, header protection, content padding, random trailers,
// cookie suppression and ranged timings. All of them sit on the wireguard
// endpoint ROOT next to jc/s1/h1 (sing-box-lx >= 1.14.0-lx.32, docs-lx
// lx-protocols-transports.ru.md §2.10). Cores before lx.32 reject the whole
// config on any of these keys, hence the build-time gate (core_capabilities).
//
// Naming rule, same as the AWG2 set: the URI/.conf key is the .conf key
// lower-cased (Jc → jc, HeaderProtectionKey → headerprotectionkey); the JSON
// key is the core's snake_case option. Both directions (parser and share-URI
// encoder) MUST use these tables — a scheme without an emit branch is silently
// truncated (see emitter/parser pairing rule).

// awg3Field pairs the share-URI/.conf key with the endpoint JSON key.
type awg3Field struct {
	Param string // lower-cased .conf / wireguard:// query key
	JSON  string // endpoint root key in config.json
}

// awg3RangeFields — "N" (JSON number) or "N-M" (JSON string) values.
var awg3RangeFields = []awg3Field{
	{"contentpaddingaddition", "content_padding_addition"},
	{"rekeyaftertime", "rekey_after_time"},
	{"rekeytimeout", "rekey_timeout"},
	{"rejectaftertime", "reject_after_time"},
	{"keepalivetimeout", "keepalive_timeout"},
	{"maxhandshakeattempts", "max_handshake_attempts"},
}

// awg3BoolFields — .conf "on"/"off"; emitted as JSON true only when on
// (off/absent → key absent, never `false`).
var awg3BoolFields = []awg3Field{
	{"randomtrailers", "random_trailers"},
	{"disablecookies", "disable_cookies"},
}

// awg3HeaderKeyField — the only SERVER-side AWG3 value: base64 of 32 bytes
// (`awg genkey`), copied verbatim. With it set, each of s1–s4 must be >= 12
// (the padding carries the header cipher nonce).
var awg3HeaderKeyField = awg3Field{"headerprotectionkey", "header_protection_key"}

// awg3MinPaddingWithHeaderKey — minimum s1–s4 when header_protection_key is set.
const awg3MinPaddingWithHeaderKey = 12

// AWG3RootKeys lists every AWG3 endpoint-root JSON key (header key, range and
// bool fields). Order is stable for deterministic emission/iteration.
func AWG3RootKeys() []string {
	keys := make([]string, 0, 1+len(awg3RangeFields)+len(awg3BoolFields))
	keys = append(keys, awg3HeaderKeyField.JSON)
	for _, f := range awg3RangeFields {
		keys = append(keys, f.JSON)
	}
	for _, f := range awg3BoolFields {
		keys = append(keys, f.JSON)
	}
	return keys
}

// HasAWG3Fields reports whether a wireguard endpoint map (parser output or a
// JSON-decoded node body) carries any AWG3 marker: an AWG3 root key, or a
// RANGED persistent_keepalive_interval ("25-35") on any peer — the range form
// is itself an AWG3 feature the old core rejects. Drives the build-time core
// gate and the AWG level badge.
func HasAWG3Fields(endpoint map[string]interface{}) bool {
	if endpoint == nil {
		return false
	}
	for _, k := range AWG3RootKeys() {
		if v, ok := endpoint[k]; ok && v != nil {
			return true
		}
	}
	peers, err := wireGuardPeerMaps(endpoint)
	if err != nil {
		return false
	}
	for _, p := range peers {
		if s, ok := p["persistent_keepalive_interval"].(string); ok && strings.Contains(s, "-") {
			return true
		}
	}
	return false
}

// awg3ParamKeys lists every AWG3 URI/.conf query key (header key, ranges,
// bools). Used by the query-side detection and by the .conf → URI converter.
func awg3ParamKeys() []string {
	keys := make([]string, 0, 1+len(awg3RangeFields)+len(awg3BoolFields))
	keys = append(keys, awg3HeaderKeyField.Param)
	for _, f := range awg3RangeFields {
		keys = append(keys, f.Param)
	}
	for _, f := range awg3BoolFields {
		keys = append(keys, f.Param)
	}
	return keys
}

// hasAWG3Params reports whether a wireguard:// query carries an AWG3 marker: an
// AWG3 param, or a RANGED keepalive ("25-35").
//
// Комментарий здесь долго утверждал обратное коду: будто AWG3-узел сохраняет
// прописанный сервером MTU (1376 у экспортов Amnezia) и потолок AWG2 к нему не
// применяется. На деле hasAWGParams возвращал hasAWG3Params, то есть AWG3
// клампился наравне со всеми, — и решение владельца 05.09.2026 именно такое
// (на 1376 данные не шли, на 1280 туннель заработал). Расхождение снято
// вместе с переносом правила в реестр: потолок 1280 действует на ЛЮБОМ
// AmneziaWG-узле, включая AWG3 (wireguard.body.fields.mtu.max_when,
// контракт 1.1.5, находка №5 LEGACY_AUDIT).
func hasAWG3Params(q url.Values) bool {
	for _, k := range awg3ParamKeys() {
		if strings.TrimSpace(q.Get(k)) != "" {
			return true
		}
	}
	return strings.Contains(strings.TrimSpace(q.Get("keepalive")), "-")
}

// parseAWG3Range validates an AWG3 timing value: a bare uint32 ("25") or a
// range "N-M" with N <= M. Returns int64 for the single form and a normalized
// "N-M" string for the range — the two shapes the core accepts on these keys.
//
// Unlike the h1–h4 magic headers, reversed bounds are NOT swapped: a timing
// range is a client-side setting the core happily defaults, so an inverted
// range is a human typo that must be surfaced, not silently repaired
// (SPEC 123 §2 "Политика ошибок").
func parseAWG3Range(raw string) (interface{}, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false
	}
	if n, err := strconv.ParseUint(raw, 10, 32); err == nil {
		return int64(n), true
	}
	loStr, hiStr, found := strings.Cut(raw, "-")
	if !found {
		return nil, false
	}
	lo, errLo := strconv.ParseUint(strings.TrimSpace(loStr), 10, 32)
	hi, errHi := strconv.ParseUint(strings.TrimSpace(hiStr), 10, 32)
	if errLo != nil || errHi != nil || hi < lo {
		return nil, false
	}
	return fmt.Sprintf("%d-%d", lo, hi), true
}

// parseAWG3Bool maps a .conf/URI boolean to the core's shape: on/true/1 → true,
// off/false/0/empty → "not set" (the key is omitted, never written as `false`),
// anything else → invalid so the caller drops the field with a warning.
func parseAWG3Bool(raw string) (value, ok bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "on", "true", "1":
		return true, true
	case "", "off", "false", "0":
		return false, true
	default:
		return false, false
	}
}

// normalizeAWG3HeaderKey validates the header protection key: base64 (any of
// the four encodings, url-safe converted to std like the WireGuard keys) of
// exactly 32 bytes, not all-zero. Without a correct key the handshake is
// impossible AND the core rejects the whole config, so the caller drops the
// node rather than the field (SPEC 123 §2).
func normalizeAWG3HeaderKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("header_protection_key is empty")
	}
	var raw []byte
	var err error
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.URLEncoding,
		base64.RawStdEncoding, base64.RawURLEncoding,
	} {
		if raw, err = enc.DecodeString(value); err == nil {
			break
		}
	}
	if err != nil {
		return "", fmt.Errorf("header_protection_key is not base64")
	}
	if len(raw) != 32 {
		return "", fmt.Errorf("header_protection_key decodes to %d bytes, want 32", len(raw))
	}
	allZero := true
	for _, b := range raw {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		return "", fmt.Errorf("header_protection_key is all zeros")
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// applyAWG3Fields promotes AWG3 params from a wireguard:// query onto the
// endpoint root and returns degradation codes for the node envelope.
//
// The header key is read with queryParamPreservePlus: q.Get would have turned
// its base64 '+' into a space (form-urlencoded semantics) and the key would be
// rejected as junk — the same trap publickey/presharedkey already dodge.
//
// A malformed key is NOT dropped here: it is validated in validateAWG3, which
// kills the node — the field-level policy (drop the field, keep the node)
// applies to timings and bools only.
func applyAWG3Fields(endpoint map[string]interface{}, u *url.URL, q url.Values) []string {
	var codes []string
	if raw := strings.TrimSpace(queryParamPreservePlusOrGet(u, q, awg3HeaderKeyField.Param)); raw != "" {
		endpoint[awg3HeaderKeyField.JSON] = raw
	}
	for _, f := range awg3RangeFields {
		raw := strings.TrimSpace(q.Get(f.Param))
		if raw == "" {
			continue
		}
		v, ok := parseAWG3Range(raw)
		if !ok {
			debuglog.WarnLog("Parser: AWG3 %s=%q is not a uint32 or an ordered N-M range — field dropped, the core will use its WireGuard default", f.Param, raw)
			codes = append(codes, WarnAWG3FieldInvalid)
			continue
		}
		endpoint[f.JSON] = v
	}
	for _, f := range awg3BoolFields {
		raw := strings.TrimSpace(q.Get(f.Param))
		if raw == "" {
			continue
		}
		v, ok := parseAWG3Bool(raw)
		if !ok {
			debuglog.WarnLog("Parser: AWG3 %s=%q is not a boolean — field dropped", f.Param, raw)
			codes = append(codes, WarnAWG3FieldInvalid)
			continue
		}
		if v {
			endpoint[f.JSON] = true
		}
	}
	if awg3RandomTrailersWithWideHeaders(endpoint) {
		debuglog.DebugLog("applyAWG3Fields: random_trailers with a wide h1-h4 range — uplink data packets may be misclassified by the server (docs-lx §2.10)")
		codes = append(codes, WarnAWG3RandomTrailersWideHeaders)
	}
	return codes
}

// queryParamPreservePlusOrGet reads a query param preserving '+' (base64), with
// the decoded value as fallback for callers that pass an already-built query.
func queryParamPreservePlusOrGet(u *url.URL, q url.Values, key string) string {
	if u != nil {
		if v := queryParamPreservePlus(u, key); v != "" {
			return v
		}
	}
	return q.Get(key)
}

// awg3WideHeaderRange — ширина диапазона h1–h4, с которой ложное срабатывание
// классификатора на приёмнике сервера перестаёт быть пренебрежимым
// (ширина/2³² на каждый data-пакет, docs-lx §2.10).
const awg3WideHeaderRange = 65536

// awg3RandomTrailersWithWideHeaders reports the documented bad combination:
// random_trailers on, plus at least one RANGED h1–h4 wide enough that the
// server's reference receiver starts mistaking data packets for handshakes.
// Nothing is removed — it is a property of the protocol pair, so it only earns
// an info code (SPEC 123 §2).
func awg3RandomTrailersWithWideHeaders(endpoint map[string]interface{}) bool {
	if v, _ := endpoint["random_trailers"].(bool); !v {
		return false
	}
	for _, k := range []string{"h1", "h2", "h3", "h4"} {
		s, ok := endpoint[k].(string)
		if !ok {
			continue
		}
		loStr, hiStr, found := strings.Cut(s, "-")
		if !found {
			continue
		}
		lo, errLo := strconv.ParseUint(strings.TrimSpace(loStr), 10, 32)
		hi, errHi := strconv.ParseUint(strings.TrimSpace(hiStr), 10, 32)
		if errLo != nil || errHi != nil {
			continue
		}
		if hi >= lo && hi-lo >= awg3WideHeaderRange {
			return true
		}
	}
	return false
}

// validateAWG3 returns the errors that kill the NODE (as opposed to a field):
// a header protection key that cannot work, and padding too short to carry the
// header cipher nonce. Both make the core reject the whole config, so one such
// node would take the user's entire VPN down — same policy as awgHeaderOverlap.
//
// Normalizes the key in place when it is valid (url-safe base64 → std, which is
// the only encoding the core decodes).
func validateAWG3(endpoint map[string]interface{}) error {
	raw, ok := endpoint[awg3HeaderKeyField.JSON].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}
	normalized, err := normalizeAWG3HeaderKey(raw)
	if err != nil {
		return fmt.Errorf("invalid wireguard URI: %v — the core rejects such an endpoint; node skipped", err)
	}
	endpoint[awg3HeaderKeyField.JSON] = normalized
	// Nonce защиты заголовка берётся из первых 12 байт паддинга сообщения,
	// поэтому каждый из s1–s4 обязан быть >= 12; ядро отвергает конфиг целиком.
	for _, k := range []string{"s1", "s2", "s3", "s4"} {
		n, _ := endpoint[k].(int64)
		if n < awg3MinPaddingWithHeaderKey {
			return fmt.Errorf("invalid wireguard URI: %s=%d is below the minimum %d required by header_protection_key (the padding carries the header cipher nonce); node skipped",
				k, n, awg3MinPaddingWithHeaderKey)
		}
	}
	return nil
}
