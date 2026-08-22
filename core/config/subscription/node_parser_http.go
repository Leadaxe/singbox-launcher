// Package subscription: HTTP(S) CONNECT proxy parser (SPEC 103 §9.B6).
//
// URI spec (LxBox convention, see app/lib/services/parser/uri_parsers/http_parser.dart):
//
//	proxy-http://[user[:pass]@]host[:port][?params]#label   (plain, default port 80)
//	proxy-https://[user[:pass]@]host[:port][?params]#label  (TLS, default port 443)
//	proxy+http://…  / proxy+https://…                       (§268 plus-form aliases)
//
// Custom scheme instead of bare http(s):// — those are intercepted upstream by
// isSubscriptionUrl (subscription list URL) before they would ever reach the
// direct-link parser, and promo links in subscription bodies would otherwise
// masquerade as nodes.
//
// TLS discriminator: the scheme suffix. "…https" (dash- or plus-form) → TLS
// on; "…http" → plain. Userinfo follows the SOCKS convention: user | user:pass
// | :pass (password-only). Query params:
//
//	path=<uri>                     → outbound.path
//	headers=<url-encoded H: V\r\n> → outbound.headers (same serialization as
//	                                  naive extra-headers)
//	sni / peer / host, fp, alpn, insecure(+aliases), security=none
//	                                → TLS block, trojan conventions (only
//	                                  meaningful on the https-form; parsed via
//	                                  the shared trojanTLSFromNode helper)
//
// sing-box outbound type: "http".
package subscription

import (
	"net/url"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/textnorm"
)

// parseHTTPProxyURI parses a proxy-http(s):// / proxy+http(s):// URI into a
// ParsedNode. Mirrors parseMasqueURI's shape: this scheme needs its own
// userinfo/TLS/default-port handling, so it builds node.Outbound directly
// rather than going through the generic net/url branch in ParseNode.
func parseHTTPProxyURI(uri string, skipFilters []map[string]string) (*configtypes.ParsedNode, error) {
	// Percent-encode stray spaces in userinfo, same as the generic path (a
	// promo login pasted with a space would otherwise fail url.Parse outright).
	parsedURL, err := url.Parse(percentEncodeUserinfoSpaces(uri))
	if err != nil {
		return nil, err
	}
	if parsedURL.Hostname() == "" {
		return nil, errHTTPProxy("missing hostname")
	}

	// TLS discriminator: suffix "https" covers both proxy-https and
	// proxy+https; proxy-http / proxy+http stay plain.
	secure := strings.HasSuffix(strings.ToLower(parsedURL.Scheme), "https")

	// userinfo like SOCKS: user | user:pass | :pass (password-only).
	// url.Parse has already percent-decoded User/Password.
	username := ""
	password := ""
	if parsedURL.User != nil {
		username = parsedURL.User.Username()
		if pw, ok := parsedURL.User.Password(); ok {
			password = pw
		}
	}

	server := parsedURL.Hostname()
	defaultPort := 80
	if secure {
		defaultPort = 443
	}
	port := defaultPort
	if p := parsedURL.Port(); p != "" {
		pi, convErr := strconv.Atoi(p)
		if convErr != nil || pi < 1 || pi > 65535 {
			return nil, errHTTPProxy("invalid port")
		}
		port = pi
	}

	q := parsedURL.Query()

	label := parsedURL.Fragment
	if decoded, decErr := url.PathUnescape(label); decErr == nil {
		label = decoded
	}
	label = sanitizeForDisplay(label)
	label = textnorm.NormalizeProxyDisplay(label)
	tag, comment := extractTagAndComment(label)
	if tag == "" {
		tag = generateDefaultTag("http", server, port)
		comment = tag
	}
	tag = normalizeFlagTag(tag)

	outbound := map[string]interface{}{
		"type":        "http",
		"tag":         tag,
		"server":      server,
		"server_port": port,
	}
	if username != "" {
		outbound["username"] = username
	}
	if password != "" {
		outbound["password"] = password
	}
	if path := q.Get("path"); path != "" {
		outbound["path"] = path
	}

	// headers: same serialization as naive extra-headers
	// (URL-encoded "H1: V1\r\nH2: V2"; bad pairs are skipped with a warning).
	if raw := q.Get("headers"); raw != "" {
		if hdrs := parseNaiveExtraHeaders(raw); len(hdrs) > 0 {
			m := make(map[string]interface{}, len(hdrs))
			for k, v := range hdrs {
				m[k] = v
			}
			outbound["headers"] = m
		}
	}

	// TLS: trojan conventions (sni/peer/host, fp with allowlist normalization,
	// alpn, insecure aliases, security=none). Only meaningful on the
	// https-form — proxy-http never carries TLS regardless of query params.
	if secure {
		node := &configtypes.ParsedNode{Server: server, Query: q}
		if tlsData, ok := trojanTLSFromNode(node); ok {
			outbound["tls"] = tlsData
		}
	}

	node := &configtypes.ParsedNode{
		Scheme:   "http",
		Tag:      tag,
		Server:   server,
		Port:     port,
		Label:    label,
		Comment:  comment,
		Query:    q,
		Outbound: outbound,
	}
	if shouldSkipNode(node, skipFilters) {
		return nil, nil
	}
	return node, nil
}

func errHTTPProxy(msg string) error {
	return &httpProxyParseError{msg: msg}
}

type httpProxyParseError struct{ msg string }

func (e *httpProxyParseError) Error() string { return "invalid http proxy URI: " + e.msg }
