package subscription

import (
	"net/url"
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/textnorm"
)

// parseMasqueURI parses a MASQUE (CONNECT-IP / WARP) share URI into a sing-box
// masque outbound. Requires core >= lx.2 (masque outbound; the launcher pins
// lx.3). Mirrors the LxBox masque_parser and node_spec_emit contract.
//
// URI shape:
//
//	masque://<privKeyDer>@<server>:<port>?publickey=<serverPubDer>&address=<v4,v6>
//	   &profile=cloudflare&network=h3[&sni=][&mtu=1280][&idle_timeout=][&keep_alive=]#label
//
// The keys are base64(DER): private_key → x509.ParseECPrivateKey (SEC1),
// public_key → x509.ParsePKIXPublicKey (PKIX), same as the core config.
func parseMasqueURI(uri string, skipFilters []map[string]string) (*configtypes.ParsedNode, error) {
	// A raw '/' inside the base64 private key (userinfo) breaks url.Parse — reuse
	// the WireGuard userinfo-slash encoder.
	parsedURL, err := url.Parse(percentEncodeWGUserinfoSlashes(uri))
	if err != nil {
		return nil, err
	}
	if parsedURL.Hostname() == "" {
		return nil, errMasque("missing hostname")
	}

	q := parsedURL.Query()

	// private key: userinfo username, else ?private_key=/?privatekey=
	privKey := ""
	if parsedURL.User != nil {
		if u, e := url.PathUnescape(parsedURL.User.Username()); e == nil {
			privKey = u
		} else {
			privKey = parsedURL.User.Username()
		}
	}
	if privKey == "" {
		privKey = firstNonEmptyQuery(q, "private_key", "privatekey")
	}
	privKey = strings.TrimSpace(privKey)
	if privKey == "" {
		return nil, errMasque("missing private key")
	}

	pubKey := strings.TrimSpace(queryParamPreservePlus(parsedURL, "publickey"))
	if pubKey == "" {
		pubKey = strings.TrimSpace(firstNonEmptyQuery(q, "publickey", "public_key"))
	}
	if pubKey == "" {
		return nil, errMasque("missing publickey (server key)")
	}

	// local tunnel addresses → ip / ipv6 (sing-box masque uses ip/ipv6, not address)
	addrParam, _ := url.QueryUnescape(q.Get("address"))
	addrs := normalizeWGPrefixes(splitAndTrim(addrParam, ","))
	var ip4, ip6 string
	for _, a := range addrs {
		if strings.Contains(a, ":") {
			if ip6 == "" {
				ip6 = a
			}
		} else if ip4 == "" {
			ip4 = a
		}
	}
	if ip4 == "" && ip6 == "" {
		return nil, errMasque("missing address (ip/ipv6)")
	}

	port := 443
	if p := parsedURL.Port(); p != "" {
		if pi, e := strconv.Atoi(p); e == nil {
			port = pi
		}
	}

	network := strings.TrimSpace(q.Get("network"))
	if network == "" {
		network = "h3"
	}
	if network != "h3" && network != "h2" {
		debuglog.WarnLog("Parser: MASQUE network %q invalid (want h3/h2), forcing h3.", network)
		network = "h3"
	}
	profile := strings.TrimSpace(q.Get("profile"))
	if profile == "" {
		profile = "cloudflare"
	}
	mtu := 1280
	if m := strings.TrimSpace(q.Get("mtu")); m != "" {
		if mi, e := strconv.Atoi(m); e == nil && mi > 0 {
			mtu = mi
		}
	}

	outbound := map[string]interface{}{
		"type":        "masque",
		"tag":         "", // set after tag computed
		"server":      parsedURL.Hostname(),
		"server_port": port,
		"profile":     profile,
		"network":     network,
		"private_key": privKey,
		"public_key":  pubKey,
		"mtu":         mtu,
	}
	if ip4 != "" {
		outbound["ip"] = ip4
	}
	if ip6 != "" {
		outbound["ipv6"] = ip6
	}
	if sni := strings.TrimSpace(q.Get("sni")); sni != "" {
		outbound["sni"] = sni
	}
	if it := strings.TrimSpace(q.Get("idle_timeout")); it != "" {
		outbound["idle_timeout"] = it
	}
	if ka := firstNonEmptyQuery(q, "keep_alive", "keep_alive_period"); ka != "" {
		outbound["keep_alive_period"] = strings.TrimSpace(ka)
	}

	label := parsedURL.Fragment
	if decoded, e := url.QueryUnescape(label); e == nil {
		label = decoded
	}
	label = sanitizeForDisplay(label)
	label = textnorm.NormalizeProxyDisplay(label)
	tag, comment := extractTagAndComment(label)
	if tag == "" {
		tag = generateDefaultTag("masque", parsedURL.Hostname(), port)
		comment = tag
	}
	tag = normalizeFlagTag(tag)
	outbound["tag"] = tag

	node := &configtypes.ParsedNode{
		Scheme:   "masque",
		Tag:      tag,
		Server:   parsedURL.Hostname(),
		Port:     port,
		Label:    label,
		Comment:  comment,
		Query:    q,
		Outbound: outbound,
	}
	if shouldSkipNode(node, skipFilters) {
		return nil, nil
	}
	debuglog.DebugLog("parseMasqueURI: success tag=%s network=%s", tag, network)
	return node, nil
}

func errMasque(msg string) error {
	return &masqueParseError{msg: msg}
}

type masqueParseError struct{ msg string }

func (e *masqueParseError) Error() string { return "invalid masque URI: " + e.msg }

// firstNonEmptyQuery returns the first non-empty value among the given keys.
func firstNonEmptyQuery(q url.Values, keys ...string) string {
	for _, k := range keys {
		if v := q.Get(k); v != "" {
			return v
		}
	}
	return ""
}
