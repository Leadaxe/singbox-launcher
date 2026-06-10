// Package subscription: share URI generation from sing-box outbounds and WireGuard endpoints (reverse of ParseNode / parseWireGuardURI).
// Formats follow docs/ParserConfig.md and the same query keys as uriTransportFromQuery / vlessTLSFromNode.
package subscription

import (
	"errors"
	"fmt"
	"strings"
)

// ErrShareURINotSupported is returned for types/ shapes that cannot be encoded as a subscription-style URI
// (selector, urltest, direct, block, dns, wireguard multi-peer, etc.) or when required fields are missing.
var ErrShareURINotSupported = errors.New("outbound cannot be encoded as share URI")

// ShareURIFromOutbound builds a shareable proxy URI (vless://, vmess://, …) from a sing-box outbound map
// as stored in config.json (same shape as buildOutbound / GenerateNodeJSON output).
func ShareURIFromOutbound(out map[string]interface{}) (string, error) {
	if out == nil {
		return "", fmt.Errorf("%w: nil outbound", ErrShareURINotSupported)
	}
	typ := strings.ToLower(strings.TrimSpace(mapGetString(out, "type")))
	switch typ {
	case "vless":
		return shareURIFromVLESS(out)
	case "vmess":
		return shareURIFromVMess(out)
	case "trojan":
		return shareURIFromTrojan(out)
	case "shadowsocks":
		return shareURIFromShadowsocks(out)
	case "socks":
		return shareURIFromSocks(out)
	case "hysteria2":
		return shareURIFromHysteria2(out)
	case "tuic":
		return shareURIFromTuic(out)
	case "ssh":
		return shareURIFromSSH(out)
	case "naive":
		return shareURIFromNaive(out)
	case "wireguard":
		return ShareURIFromWireGuardEndpoint(out)
	case "selector", "urltest", "direct", "block", "dns", "http":
		return "", fmt.Errorf("%w: type %q", ErrShareURINotSupported, typ)
	default:
		return "", fmt.Errorf("%w: unknown type %q", ErrShareURINotSupported, typ)
	}
}
