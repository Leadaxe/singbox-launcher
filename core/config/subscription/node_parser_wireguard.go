package subscription

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/textnorm"
)

// parseWireGuardURI parses wireguard:// URI into ParsedNode with sing-box endpoint in Outbound.
// Format: wireguard://<PRIVATE_KEY>@<SERVER_IP>:<PORT>?publickey=...&address=...&allowedips=...
// Required query: publickey, address, allowedips. Optional: mtu, keepalive, presharedkey, listenport, name, dns.
func parseWireGuardURI(uri string, skipFilters []map[string]string) (*configtypes.ParsedNode, error) {
	debuglog.DebugLog("parseWireGuardURI: start")
	if len(uri) > MaxURILength {
		debuglog.DebugLog("parseWireGuardURI: error URI length exceeded")
		return nil, fmt.Errorf("URI length (%d) exceeds maximum (%d)", len(uri), MaxURILength)
	}
	// Extract fragment from raw URI; url.Parse may not set Fragment for non-standard schemes.
	fragmentFromRaw := ""
	if i := strings.LastIndex(uri, "#"); i >= 0 {
		fragmentFromRaw = strings.TrimSpace(uri[i+1:])
	}
	// A standard base64 private key may contain a raw '/' in the userinfo
	// (`wireguard://AbC/DeF...@host`). url.Parse treats that '/' as the start of
	// the path and drops the userinfo, so the key would be lost. Percent-encode
	// raw '/' in the userinfo before parsing; the PathUnescape below restores it.
	uri = percentEncodeWGUserinfoSlashes(uri)
	parsedURL, err := url.Parse(uri)
	if err != nil {
		debuglog.DebugLog("parseWireGuardURI: error parse URL: %v", err)
		return nil, fmt.Errorf("failed to parse wireguard URI: %w", err)
	}
	if parsedURL.Hostname() == "" {
		debuglog.DebugLog("parseWireGuardURI: error missing hostname")
		return nil, fmt.Errorf("invalid wireguard URI: missing hostname")
	}
	if parsedURL.User == nil || parsedURL.User.Username() == "" {
		debuglog.DebugLog("parseWireGuardURI: error missing private key (userinfo)")
		return nil, fmt.Errorf("invalid wireguard URI: missing private key (userinfo)")
	}
	// Use PathUnescape so + in base64 is preserved (QueryUnescape would turn + into space and break the key)
	privateKey, err := url.PathUnescape(parsedURL.User.Username())
	if err != nil {
		privateKey = parsedURL.User.Username()
	}
	privateKey = strings.TrimSpace(privateKey)
	if privateKey == "" {
		return nil, fmt.Errorf("invalid wireguard URI: empty private key")
	}
	// Validate base64 private key (optional but recommended)
	if _, err := base64.StdEncoding.DecodeString(privateKey); err != nil {
		if _, err2 := base64.URLEncoding.DecodeString(privateKey); err2 != nil {
			debuglog.DebugLog("parseWireGuardURI: warning private key may not be valid base64")
		}
	}

	port := 51820
	if p := parsedURL.Port(); p != "" {
		if pi, err := strconv.Atoi(p); err == nil {
			port = pi
		}
	}

	q := parsedURL.Query()
	// Preserve + in base64 (query parser would decode + as space)
	publicKey := queryParamPreservePlus(parsedURL, "publickey")
	if publicKey == "" {
		publicKey = q.Get("publickey")
	}
	addressParam := q.Get("address")
	allowedipsParam := q.Get("allowedips")
	if publicKey == "" {
		debuglog.DebugLog("parseWireGuardURI: error missing publickey")
		return nil, fmt.Errorf("invalid wireguard URI: missing required query parameter publickey")
	}
	if addressParam == "" {
		debuglog.DebugLog("parseWireGuardURI: error missing address")
		return nil, fmt.Errorf("invalid wireguard URI: missing required query parameter address")
	}
	if allowedipsParam == "" {
		debuglog.DebugLog("parseWireGuardURI: error missing allowedips")
		return nil, fmt.Errorf("invalid wireguard URI: missing required query parameter allowedips")
	}

	addressDecoded, _ := url.QueryUnescape(addressParam)
	allowedipsDecoded, _ := url.QueryUnescape(allowedipsParam)
	// sing-box wants CIDRs (netip.Prefix): a bare IP like "172.16.0.2" (common in
	// AmneziaWG/.conf exports) fails to load with `ParsePrefix: no '/'`. Default a
	// bare address to /32 (IPv4) or /128 (IPv6).
	addressList := normalizeWGPrefixes(splitAndTrim(addressDecoded, ","))
	allowedipsList := normalizeWGPrefixes(splitAndTrim(allowedipsDecoded, ","))
	if len(addressList) == 0 || len(allowedipsList) == 0 {
		return nil, fmt.Errorf("invalid wireguard URI: address or allowedips empty after parse")
	}

	// AmneziaWG transport padding (S3/S4) inflates every data packet, so an AWG
	// endpoint needs a lower MTU than plain WireGuard — otherwise a full-size
	// packet exceeds the path MTU and the OS rejects it with EMSGSIZE
	// ("message too long"): the handshake succeeds but data silently stops.
	// Default AWG to awgMaxMTU and clamp any higher URI value down to it; honor an
	// explicitly lower value; plain WireGuard keeps the upstream 1420 default.
	isAWG := hasAWGParams(q)
	mtu := defaultWireGuardMTU
	if isAWG {
		mtu = awgMaxMTU
	}
	if m := q.Get("mtu"); m != "" {
		if mi, err := strconv.Atoi(m); err == nil {
			mtu = mi
		}
	}
	if isAWG && mtu > awgMaxMTU {
		debuglog.DebugLog("parseWireGuardURI: clamping AWG mtu %d -> %d (AmneziaWG padding overhead)", mtu, awgMaxMTU)
		mtu = awgMaxMTU
	}
	listenport := 0
	if lp := q.Get("listenport"); lp != "" {
		if lpi, err := strconv.Atoi(lp); err == nil {
			listenport = lpi
		}
	}
	name := q.Get("name")
	if name == "" {
		name = "singbox-wg0"
	}
	if decoded, err := url.QueryUnescape(name); err == nil {
		name = decoded
	}

	peer := map[string]interface{}{
		"address":     parsedURL.Hostname(),
		"port":        port,
		"public_key":  publicKey,
		"allowed_ips": allowedipsList,
	}
	if keepalive := q.Get("keepalive"); keepalive != "" {
		if ki, err := strconv.Atoi(keepalive); err == nil {
			peer["persistent_keepalive_interval"] = ki
		}
	}
	if psk := queryParamPreservePlus(parsedURL, "presharedkey"); psk != "" {
		peer["pre_shared_key"] = psk
	} else if psk := q.Get("presharedkey"); psk != "" {
		peer["pre_shared_key"] = psk
	}

	endpoint := map[string]interface{}{
		"type":        "wireguard",
		"tag":         "", // set below after tag is computed
		"name":        name,
		"system":      false,
		"mtu":         mtu,
		"address":     addressList,
		"private_key": privateKey,
		"peers":       []map[string]interface{}{peer},
	}
	if listenport != 0 {
		endpoint["listen_port"] = listenport
	}

	// AmneziaWG (SPEC 073): promote obfuscation params from the query into the
	// endpoint root (sing-box-lx with_awg shape). No-op for a plain WG URI.
	applyAWGFields(endpoint, q)

	label := parsedURL.Fragment
	if label == "" && fragmentFromRaw != "" {
		label = fragmentFromRaw
	}
	if label == "" {
		label = name
	}
	if decoded, err := url.QueryUnescape(label); err == nil {
		label = decoded
	}
	label = sanitizeForDisplay(label)
	label = textnorm.NormalizeProxyDisplay(label)
	tag, comment := extractTagAndComment(label)
	if tag == "" {
		tag = generateDefaultTag("wireguard", parsedURL.Hostname(), port)
		comment = tag
	}
	tag = normalizeFlagTag(tag)
	endpoint["tag"] = tag

	node := &configtypes.ParsedNode{
		Scheme:   "wireguard",
		Tag:      tag,
		Server:   parsedURL.Hostname(),
		Port:     port,
		Label:    label,
		Comment:  comment,
		Query:    q,
		Outbound: endpoint,
	}

	if shouldSkipNode(node, skipFilters) {
		return nil, nil
	}
	debuglog.DebugLog("parseWireGuardURI: success tag=%s", node.Tag)
	return node, nil
}

// percentEncodeWGUserinfoSlashes percent-encodes a raw '/' in the userinfo of a
// wireguard:// / awg:// URI (between "://" and the authority's '@') so url.Parse
// does not mistake the base64 private key's '/' for a path separator and drop the
// key. Already-encoded URIs (no raw '/' in userinfo) are returned unchanged.
func percentEncodeWGUserinfoSlashes(uri string) string {
	const sep = "://"
	si := strings.Index(uri, sep)
	if si < 0 {
		return uri
	}
	start := si + len(sep)
	rest := uri[start:]
	at := strings.IndexByte(rest, '@')
	if at < 0 {
		return uri
	}
	// The '@' must belong to the authority, not the query/fragment.
	if strings.IndexAny(rest[:at], "?#") >= 0 {
		return uri
	}
	userinfo := rest[:at]
	if !strings.Contains(userinfo, "/") {
		return uri
	}
	return uri[:start] + strings.ReplaceAll(userinfo, "/", "%2F") + uri[start+at:]
}

// normalizeWGPrefixes ensures every entry is a CIDR (netip.Prefix): a bare IP
// gets /32 (IPv4) or /128 (IPv6). sing-box rejects an address/allowed_ip without
// a prefix length (`netip.ParsePrefix("172.16.0.2"): no '/'`).
func normalizeWGPrefixes(addrs []string) []string {
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if !strings.Contains(a, "/") {
			if strings.Contains(a, ":") {
				a += "/128" // IPv6
			} else {
				a += "/32" // IPv4
			}
		}
		out = append(out, a)
	}
	return out
}

// queryParamPreservePlus returns the first value for key in u.RawQuery, decoded with PathUnescape.
// This preserves '+' in base64 (QueryUnescape decodes '+' as space and would break keys).
func queryParamPreservePlus(u *url.URL, key string) string {
	for _, pair := range strings.Split(u.RawQuery, "&") {
		if i := strings.Index(pair, "="); i >= 0 {
			k := strings.TrimSpace(pair[:i])
			if k != key {
				continue
			}
			val := pair[i+1:]
			if d, err := url.PathUnescape(val); err == nil {
				return d
			}
			return val
		}
	}
	return ""
}

// AmneziaWG (AWG 2.0) field names, promoted to the WireGuard endpoint root in
// the sing-box-lx `with_awg` config shape (SPEC 073). Shared by the URI parser
// (applyAWGFields) and the share-URI encoder (shareuri_wireguard.go).
//   - numeric: jc/jmin/jmax, s1–s4, h1–h4 — uint32 (emitted as JSON number)
//   - string:  i1–i5 — case-sensitive tag strings (<b 0xHEX>, <r N>, <c>, …)
var (
	awgNumericFields = []string{"jc", "jmin", "jmax", "s1", "s2", "s3", "s4", "h1", "h2", "h3", "h4"}
	awgStringFields  = []string{"i1", "i2", "i3", "i4", "i5"}
)

const (
	// defaultWireGuardMTU is the upstream WireGuard tunnel MTU.
	defaultWireGuardMTU = 1420
	// awgMaxMTU caps AmneziaWG endpoints. It is the AmneziaWG-recommended client
	// MTU and the IPv6 minimum, leaving headroom for S3/S4 transport padding so
	// the obfuscated packet stays under a 1500-byte path (1500 - 28 UDP/IP - 32
	// WireGuard - 60 max S3/S4 = 1380 ceiling; 1280 adds margin for PPPoE/mobile/
	// nested paths). A too-high MTU fails silently (handshake OK, no data), so we
	// clamp rather than trust the URI value. See SPEC 073 and the lx-config docs.
	awgMaxMTU = 1280
)

// hasAWGParams reports whether the query carries any AmneziaWG obfuscation field
// (numeric jc/jmin/jmax/s/h or string i1-i5). Drives the MTU policy: AWG
// endpoints are clamped to awgMaxMTU; a plain WireGuard URI is left untouched.
func hasAWGParams(q url.Values) bool {
	for _, k := range awgNumericFields {
		if strings.TrimSpace(q.Get(k)) != "" {
			return true
		}
	}
	for _, k := range awgStringFields {
		if strings.TrimSpace(q.Get(k)) != "" {
			return true
		}
	}
	return false
}

// applyAWGFields extracts AmneziaWG obfuscation params from a wireguard:// (or
// awg://) query and promotes them to the endpoint root. Numeric fields are
// stored as int64 (full uint32 range, safe on 32-bit, marshals as a JSON
// number); h1–h4 may instead carry an AWG 2.0 randomization range "lo-hi",
// stored as a normalized string (SPEC 073.2, core >= lx.6 picks an in-range
// value per handshake); i1–i5 are stored as non-empty strings with their tag
// case preserved. A bad value is skipped with a debug log (forward-compat: one
// broken param must not drop the whole node, matching the mtu/keepalive
// policy). A plain WireGuard URI (no AWG params) leaves endpoint untouched.
func applyAWGFields(endpoint map[string]interface{}, q url.Values) {
	for _, k := range awgNumericFields {
		raw := strings.TrimSpace(q.Get(k))
		if raw == "" {
			continue
		}
		if n, err := strconv.ParseUint(raw, 10, 32); err == nil {
			endpoint[k] = int64(n)
			continue
		}
		// SPEC 073.2: h1–h4 may carry an AWG 2.0 randomization range "lo-hi".
		if awgHeaderFields[k] {
			if rng, ok := parseAWGHeaderRange(raw); ok {
				endpoint[k] = rng
				continue
			}
			// A silently dropped header means the core falls back to the WG
			// default message type and the handshake won't match the server —
			// the exact failure mode of the original 073.2 bug. Warn loudly.
			debuglog.WarnLog("Parser: AWG %s=%q is not a uint32 or lo-hi range — field dropped, the core will use the WireGuard default header", k, raw)
			continue
		}
		debuglog.DebugLog("applyAWGFields: skip %s=%q (invalid value)", k, raw)
	}
	for _, k := range awgStringFields {
		// q.Get already URL-decodes (incl. '+' → space and %3C → '<'); the tag
		// case must be preserved exactly, so do NOT lower-case.
		v := strings.TrimSpace(q.Get(k))
		if v == "" {
			continue
		}
		endpoint[k] = v
	}
	if a, b := awgHeaderOverlap(endpoint); a != "" {
		debuglog.WarnLog("Parser: AWG magic headers %s and %s overlap — the core will reject this endpoint ('headers must not overlap')", a, b)
	}
}

// awgHeaderOverlap reports a pair of magic-header fields whose effective
// ranges overlap ("", "" when all four are disjoint). Mirrors the core
// contract (SPEC 073.2): an unset/zero header counts as its WireGuard default
// message type (h1=1 … h4=4), a single value as [v,v], a range as [lo,hi].
// The core rejects an overlapping set at load with "headers must not
// overlap", so the parser warns already at import time; the node itself is
// still produced — the core's error stays the source of truth.
func awgHeaderOverlap(endpoint map[string]interface{}) (string, string) {
	type span struct {
		name   string
		lo, hi uint64
	}
	spans := make([]span, 0, 4)
	for i, k := range []string{"h1", "h2", "h3", "h4"} {
		s := span{name: k, lo: uint64(i + 1), hi: uint64(i + 1)} // WG default
		switch v := endpoint[k].(type) {
		case int64:
			if v > 0 {
				s.lo, s.hi = uint64(v), uint64(v)
			}
		case string:
			loStr, hiStr, _ := strings.Cut(v, "-")
			lo, errLo := strconv.ParseUint(loStr, 10, 32)
			hi, errHi := strconv.ParseUint(hiStr, 10, 32)
			if errLo == nil && errHi == nil {
				s.lo, s.hi = lo, hi
			}
		}
		spans = append(spans, s)
	}
	for i := 0; i < len(spans); i++ {
		for j := i + 1; j < len(spans); j++ {
			if spans[i].lo <= spans[j].hi && spans[j].lo <= spans[i].hi {
				return spans[i].name, spans[j].name
			}
		}
	}
	return "", ""
}

// awgHeaderFields — magic-header fields (h1–h4) that, unlike the other AWG
// numerics, may carry an AWG 2.0 randomization range besides a plain uint32.
var awgHeaderFields = map[string]bool{"h1": true, "h2": true, "h3": true, "h4": true}

// parseAWGHeaderRange validates an AWG 2.0 header randomization range "lo-hi"
// (both bounds uint32) and returns it normalized (bounds ordered). The range
// stays a string: the sing-box-lx core (>= 1.13.13-lx.6) accepts "h1": "N-M"
// in the endpoint JSON and picks a fresh in-range value per handshake — better
// obfuscation than any fixed value the launcher could choose. Cores before
// lx.6 reject the string form — hence the RequiredCoreVersion bump (SPEC 073.2).
func parseAWGHeaderRange(raw string) (string, bool) {
	loStr, hiStr, found := strings.Cut(raw, "-")
	if !found {
		return "", false
	}
	lo, errLo := strconv.ParseUint(strings.TrimSpace(loStr), 10, 32)
	hi, errHi := strconv.ParseUint(strings.TrimSpace(hiStr), 10, 32)
	if errLo != nil || errHi != nil {
		return "", false
	}
	if hi < lo {
		lo, hi = hi, lo
	}
	return fmt.Sprintf("%d-%d", lo, hi), true
}

// splitAndTrim splits a string by separator, trims whitespace from each part,
// and returns only non-empty parts.
func splitAndTrim(s string, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
