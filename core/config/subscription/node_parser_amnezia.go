package subscription

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// Amnezia vpn:// link import (SPEC 075).
//
// Format (reference: amnezia-vpn/config-decoder, mainwindow.cpp; verified
// reference implementation in scripts/decode_amnezia_vpn.py):
//
//	vpn:// + base64url (alphabet -_, no padding: Qt Base64UrlEncoding|OmitTrailingEquals)
//	payload = qCompress(json, 8): 4-byte big-endian uncompressed size, then a zlib stream
//	json    = full Amnezia profile: containers[] (one per protocol), defaultContainer,
//	          hostName, description, dns1/dns2. The WG/AWG tunnel itself sits inside a
//	          container as last_config — a JSON string whose "config" field holds the
//	          classic [Interface]/[Peer] INI text (incl. AWG Jc/Jmin/.../I1-I5 fields).
//
// Only WireGuard/AmneziaWG containers are importable. The INI is converted to the
// canonical wireguard:// URI and delegated to parseWireGuardURI, so CIDR
// normalization, AWG param promotion and the AWG MTU clamp (SPEC 073) all apply
// unchanged.

const (
	// maxAmneziaLinkLength caps the raw vpn:// link. Amnezia profiles bundle whole
	// certificates for some protocols and routinely exceed MaxURILength; 512 KB is
	// far above any real profile yet keeps a hostile link from ballooning memory.
	maxAmneziaLinkLength = 512 * 1024
	// maxAmneziaProfileJSON caps the decompressed profile (zlib-bomb guard).
	maxAmneziaProfileJSON = 8 * 1024 * 1024
	// maxAmneziaScanDepth bounds the recursive [Interface] search: the deepest
	// known nesting is containers[] → container → proto → last_config (JSON
	// string) → config, i.e. 5 levels; +headroom for schema drift.
	maxAmneziaScanDepth = 8
)

// parseAmneziaVPNLink parses a vpn:// link into a WireGuard/AmneziaWG ParsedNode.
func parseAmneziaVPNLink(uri string, skipFilters []map[string]string) (*configtypes.ParsedNode, error) {
	debuglog.DebugLog("parseAmneziaVPNLink: start (link length %d)", len(uri))
	if len(uri) > maxAmneziaLinkLength {
		return nil, fmt.Errorf("vpn:// link length (%d) exceeds maximum (%d)", len(uri), maxAmneziaLinkLength)
	}
	payload := strings.TrimPrefix(strings.TrimSpace(uri), "vpn://")
	profile, err := decodeAmneziaProfile(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode vpn:// profile: %w", err)
	}

	confText, containerName := amneziaWGConfText(profile)
	if confText == "" {
		return nil, fmt.Errorf("vpn:// profile has no WireGuard/AmneziaWG config (containers: %s)",
			strings.Join(amneziaContainerNames(profile), ", "))
	}
	debuglog.DebugLog("parseAmneziaVPNLink: using container %q (host %q)", containerName, amneziaString(profile, "hostName"))

	label := amneziaString(profile, "description")
	if label == "" {
		label = amneziaString(profile, "hostName")
	}
	if label == "" {
		label = containerName
	}

	wgURI, err := wgConfToURI(confText, label)
	if err != nil {
		return nil, fmt.Errorf("invalid WireGuard config in vpn:// container %q: %w", containerName, err)
	}
	return parseWireGuardURI(wgURI, skipFilters)
}

// decodeAmneziaProfile turns the base64url payload of a vpn:// link into the
// profile JSON. Tolerant input: whitespace/newlines are dropped (links pasted
// from chats get wrapped), padding is stripped, and the standard base64
// alphabet is accepted as a fallback.
func decodeAmneziaProfile(payload string) (map[string]interface{}, error) {
	payload = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, payload)
	payload = strings.TrimRight(payload, "=")
	if payload == "" {
		return nil, fmt.Errorf("empty payload")
	}

	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		var stdErr error
		raw, stdErr = base64.RawStdEncoding.DecodeString(payload)
		if stdErr != nil {
			return nil, fmt.Errorf("invalid base64: %w", err)
		}
	}
	// qCompress framing: 4-byte big-endian uncompressed size + zlib stream.
	if len(raw) < 5 {
		return nil, fmt.Errorf("payload too short for qCompress framing (%d bytes)", len(raw))
	}
	expected := binary.BigEndian.Uint32(raw[:4])
	if expected == 0 || expected > maxAmneziaProfileJSON {
		return nil, fmt.Errorf("declared uncompressed size %d out of range", expected)
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw[4:]))
	if err != nil {
		return nil, fmt.Errorf("invalid zlib stream: %w", err)
	}
	defer func() { _ = zr.Close() }()
	data, err := io.ReadAll(io.LimitReader(zr, maxAmneziaProfileJSON+1))
	if err != nil {
		return nil, fmt.Errorf("zlib decompression failed: %w", err)
	}
	if len(data) > maxAmneziaProfileJSON {
		return nil, fmt.Errorf("decompressed profile exceeds %d bytes", maxAmneziaProfileJSON)
	}
	if uint32(len(data)) != expected {
		// Header mismatch is suspicious but not fatal: trust the actual stream.
		debuglog.DebugLog("decodeAmneziaProfile: qCompress header says %d bytes, got %d", expected, len(data))
	}

	var profile map[string]interface{}
	if err := json.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("profile is not valid JSON: %w", err)
	}
	return profile, nil
}

// amneziaWGConfText picks the WG/AWG [Interface]/[Peer] text out of the profile.
// The defaultContainer is tried first, then the rest in array order; the first
// container that yields an [Interface] text wins. Returns the text and the
// container name ("" if nothing found).
func amneziaWGConfText(profile map[string]interface{}) (string, string) {
	containers, _ := profile["containers"].([]interface{})
	defaultName, _ := profile["defaultContainer"].(string)

	ordered := make([]map[string]interface{}, 0, len(containers))
	for _, c := range containers {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if name, _ := cm["container"].(string); name != "" && name == defaultName {
			ordered = append([]map[string]interface{}{cm}, ordered...)
		} else {
			ordered = append(ordered, cm)
		}
	}

	matched := 0
	confText, containerName := "", ""
	for _, cm := range ordered {
		if txt := findWGIniText(cm, 0); txt != "" {
			matched++
			if confText == "" {
				confText = txt
				containerName, _ = cm["container"].(string)
			}
		}
	}
	if matched > 1 {
		debuglog.WarnLog("Parser: vpn:// profile has %d WireGuard/AWG containers, importing %q (default container preferred)", matched, containerName)
	}
	return confText, containerName
}

// findWGIniText recursively searches a decoded profile value for a WireGuard
// [Interface]/[Peer] text. JSON-looking strings are unwrapped first: the
// canonical spot is last_config — a JSON *string* whose raw text also contains
// "[Interface]" (with \n escapes), so the nested "config" field must win over
// the wrapper. Known keys are tried before the rest to keep the walk
// deterministic across Go's random map iteration.
func findWGIniText(v interface{}, depth int) string {
	if depth > maxAmneziaScanDepth {
		return ""
	}
	switch t := v.(type) {
	case string:
		if s := strings.TrimSpace(t); strings.HasPrefix(s, "{") {
			var nested interface{}
			if err := json.Unmarshal([]byte(s), &nested); err == nil {
				return findWGIniText(nested, depth+1)
			}
		}
		if strings.Contains(t, "[Interface]") {
			return t
		}
	case map[string]interface{}:
		knownKeys := []string{"config", "last_config", "awg", "wireguard"}
		for _, k := range knownKeys {
			if nv, ok := t[k]; ok {
				if r := findWGIniText(nv, depth+1); r != "" {
					return r
				}
			}
		}
		for k, nv := range t {
			switch k {
			case "config", "last_config", "awg", "wireguard":
				continue
			}
			if r := findWGIniText(nv, depth+1); r != "" {
				return r
			}
		}
	case []interface{}:
		for _, nv := range t {
			if r := findWGIniText(nv, depth+1); r != "" {
				return r
			}
		}
	}
	return ""
}

// parseWGConfSections splits a [Interface]/[Peer] INI text into two maps with
// lower-cased keys. Only the first [Peer] section is honored (multi-peer is not
// supported by the WG share/parse path, see shareuri_wireguard.go).
func parseWGConfSections(text string) (iface, peer map[string]string) {
	iface, peer = map[string]string{}, map[string]string{}
	section := ""
	peerSections := 0
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(strings.Trim(line, "[]"))
			if section == "peer" {
				peerSections++
			}
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		switch section {
		case "interface":
			iface[key] = value
		case "peer":
			if peerSections == 1 {
				peer[key] = value
			}
		}
	}
	return iface, peer
}

// wgConfToURI converts parsed [Interface]/[Peer] data into the canonical
// wireguard:// URI accepted by parseWireGuardURI. AWG fields (Jc/Jmin/.../I1-I5)
// map 1:1 to their lower-case query params; MTU is passed through verbatim —
// parseWireGuardURI clamps AWG endpoints to awgMaxMTU (SPEC 073).
func wgConfToURI(confText, label string) (string, error) {
	iface, peer := parseWGConfSections(confText)

	var missing []string
	for _, req := range []struct{ key, section string }{
		{"privatekey", "Interface"}, {"address", "Interface"},
		{"publickey", "Peer"}, {"endpoint", "Peer"},
	} {
		m := iface
		if req.section == "Peer" {
			m = peer
		}
		if m[req.key] == "" {
			missing = append(missing, "["+req.section+"] "+req.key)
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("missing required fields: %s", strings.Join(missing, ", "))
	}
	endpoint := peer["endpoint"]
	if !strings.Contains(endpoint, ":") {
		return "", fmt.Errorf("peer endpoint %q has no port", endpoint)
	}

	q := url.Values{}
	q.Set("publickey", peer["publickey"])
	q.Set("address", stripSpaces(iface["address"]))
	allowed := stripSpaces(peer["allowedips"])
	if allowed == "" {
		allowed = "0.0.0.0/0,::/0"
	}
	q.Set("allowedips", allowed)
	for confKey, param := range map[string]string{
		"persistentkeepalive": "keepalive",
		"presharedkey":        "presharedkey",
		"listenport":          "listenport",
		"mtu":                 "mtu",
	} {
		if v := peer[confKey]; v != "" {
			q.Set(param, v)
		} else if v := iface[confKey]; v != "" {
			q.Set(param, v)
		}
	}
	if v := iface["dns"]; v != "" {
		q.Set("dns", stripSpaces(v))
	}
	for _, k := range awgNumericFields {
		if v := iface[k]; v != "" {
			q.Set(k, v)
		}
	}
	for _, k := range awgStringFields {
		if v := iface[k]; v != "" {
			q.Set(k, v)
		}
	}

	u := url.URL{
		Scheme:   "wireguard",
		User:     url.User(iface["privatekey"]),
		Host:     endpoint,
		RawQuery: q.Encode(),
		Fragment: label,
	}
	return u.String(), nil
}

// amneziaContainerNames lists container names for error messages.
func amneziaContainerNames(profile map[string]interface{}) []string {
	containers, _ := profile["containers"].([]interface{})
	names := make([]string, 0, len(containers))
	for _, c := range containers {
		if cm, ok := c.(map[string]interface{}); ok {
			if name, _ := cm["container"].(string); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// amneziaString reads a trimmed top-level string field from the profile.
func amneziaString(profile map[string]interface{}, key string) string {
	s, _ := profile[key].(string)
	return strings.TrimSpace(s)
}

// stripSpaces removes blanks inside comma-separated lists ("10.8.1.2/32, ::/0").
func stripSpaces(s string) string {
	return strings.ReplaceAll(s, " ", "")
}
