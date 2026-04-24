// Package subscription: NaïveProxy helpers.
//
// URI spec (de-facto, DuckSoft 2020, not officially endorsed by klzgrad but
// adopted by NaiveGUI / v2rayN / NekoRay / sing-box GUI clients):
//
//	naive+https://<user>:<pass>@<host>:<port>/?<params>#<label>
//	naive+quic://<user>:<pass>@<host>:<port>/?<params>#<label>
//
// Query params:
//   - padding=true|false — no sing-box equivalent, ignored with warning.
//   - extra-headers=<urlencoded "H1: V1\r\nH2: V2"> — HTTP headers injected
//     on the upstream CONNECT request.
//
// sing-box outbound JSON shape (sing-box ≥ 1.13.0):
//
//	{
//	  "type": "naive",
//	  "server": "...", "server_port": 443,
//	  "username": "...", "password": "...",
//	  "tls": {"enabled": true, "server_name": "..."},
//	  "quic": true | false,
//	  "quic_congestion_control": "bbr",
//	  "extra_headers": {"H1": "V1", ...}
//	}
//
// sing-box naive outbound TLS block supports ONLY server_name / certificate /
// certificate_path / ech. No alpn / utls / reality / min_version — we deliberately
// do not emit them even if present on the ParsedNode.
package subscription

import (
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// naiveHeaderNameCharset enumerates characters allowed in HTTP header names per
// the NaïveProxy URI spec (see DuckSoft gist §Extra Headers Encoding). Using a
// lookup map so membership check is O(1).
//
// The charset is a subset of RFC 7230 tchar — narrower than strictly needed,
// but we follow the spec verbatim to maximize client-side interop.
var naiveHeaderNameCharset = func() map[byte]struct{} {
	const chars = "!#$%&'*+-.0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ\\^_`abcdefghijklmnopqrstuvwxyz|~"
	m := make(map[byte]struct{}, len(chars))
	for i := 0; i < len(chars); i++ {
		m[chars[i]] = struct{}{}
	}
	return m
}()

// isValidNaiveHeaderName reports whether s consists only of characters allowed
// in a NaïveProxy extra-headers name.
func isValidNaiveHeaderName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if _, ok := naiveHeaderNameCharset[s[i]]; !ok {
			return false
		}
	}
	return true
}

// parseNaiveExtraHeaders parses the URL-decoded value of the `extra-headers`
// query param into a Header→Value map.
//
// Format: "Header1: Value1\r\nHeader2: Value2". Individual invalid pairs
// (malformed, illegal chars in name, CR/LF/NUL in value) are skipped with a
// warning — a single bad pair must not fail the whole parse, because other
// valid headers on the same node are still useful.
//
// Returns nil for empty input or when no valid pairs were found.
func parseNaiveExtraHeaders(s string) map[string]string {
	if s == "" {
		return nil
	}
	out := make(map[string]string)
	for _, line := range strings.Split(s, "\r\n") {
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx <= 0 {
			debuglog.WarnLog("Parser: naive: extra-headers entry missing ':' separator, skipping: %q", line)
			continue
		}
		name := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if !isValidNaiveHeaderName(name) {
			debuglog.WarnLog("Parser: naive: extra-headers name contains forbidden characters, skipping: %q", name)
			continue
		}
		if strings.ContainsAny(val, "\r\n\x00") {
			debuglog.WarnLog("Parser: naive: extra-headers value contains CR/LF/NUL, skipping pair %q", name)
			continue
		}
		out[name] = val
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// buildNaiveOutbound populates a sing-box outbound map for a `naive` ParsedNode.
//
// Caller (buildOutbound in node_parser.go) has already set "type", "tag",
// "server", "server_port". This function adds auth, TLS block, transport
// (HTTP/2 vs QUIC) and extra headers.
func buildNaiveOutbound(node *configtypes.ParsedNode, outbound map[string]interface{}) {
	if node.UUID != "" {
		outbound["username"] = node.UUID
	}
	if pw := node.Query.Get("password"); pw != "" {
		outbound["password"] = pw
	}

	// QUIC vs HTTP/2 transport.
	if node.Query.Get("quic") == "true" {
		outbound["quic"] = true
		outbound["quic_congestion_control"] = "bbr"
	}

	// Extra headers: parse the raw, already-URL-decoded value into a map.
	if raw := node.Query.Get("extra-headers"); raw != "" {
		if hdrs := parseNaiveExtraHeaders(raw); len(hdrs) > 0 {
			// sing-box expects map[string]interface{}; convert from map[string]string.
			m := make(map[string]interface{}, len(hdrs))
			for k, v := range hdrs {
				m[k] = v
			}
			outbound["extra_headers"] = m
		}
	}

	// TLS is always enabled for naive — even under QUIC, the transport
	// negotiates TLS 1.3 through QUIC's handshake. server_name defaults
	// to the host; no support for alpn / reality / utls / min_version.
	outbound["tls"] = map[string]interface{}{
		"enabled":     true,
		"server_name": node.Server,
	}
}
