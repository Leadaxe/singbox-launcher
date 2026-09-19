package subscription

import (
	"strings"
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
// (`awg genkey`), copied verbatim. Правила значения — в реестре: 32 байта и
// не все нули (format base64_32 + pattern, on_invalid drop_node с кодом
// awg3_header_key_invalid), а при заданном ключе каждый из s1–s4 обязан быть
// >= 12 — паддинг несёт nonce шифра заголовка (min_when у s1..s4, код
// awg3_padding_too_short).
var awg3HeaderKeyField = awg3Field{"headerprotectionkey", "header_protection_key"}

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
