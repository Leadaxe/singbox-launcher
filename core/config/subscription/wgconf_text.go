package subscription

import (
	"fmt"
	"strings"
)

// Pasted WireGuard/AmneziaWG .conf import (SPEC 076).
//
// A multi-line [Interface]/[Peer] text pasted into the Sources Add field is not
// a URI and would be destroyed by per-line classification. These helpers carve
// the conf blocks out of the pasted text and convert each to the canonical
// wireguard:// URI (SPEC 075 converter), so downstream storage/parse/share
// paths stay URI-only. AWG fields and the AWG MTU clamp are handled by
// parseWireGuardURI as usual.

// ExtractWGConfBlocks splits pasted text into [Interface]/[Peer] blocks and the
// remaining text. A block starts at a line equal to "[Interface]" (case-
// insensitive, as wg-quick treats section names) and runs until the next such
// line. Text before the first block is returned as rest for the normal
// line-by-line classification, so links and conf text can be pasted together.
// With no [Interface] line, rest == input and blocks is empty.
func ExtractWGConfBlocks(input string) (rest string, blocks []string) {
	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	var restLines, block []string
	inBlock := false
	for _, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line), "[interface]") {
			if inBlock {
				blocks = append(blocks, strings.Join(block, "\n"))
			}
			block = []string{line}
			inBlock = true
			continue
		}
		if inBlock {
			block = append(block, line)
		} else {
			restLines = append(restLines, line)
		}
	}
	if inBlock {
		blocks = append(blocks, strings.Join(block, "\n"))
	}
	return strings.Join(restLines, "\n"), blocks
}

// ConvertWGConfText converts one [Interface]/[Peer] block to the canonical
// wireguard:// URI accepted by ParseNode. The URI fragment (node label) is the
// Endpoint host: a pasted .conf carries no display name, and a fixed fallback
// would give every pasted node the same tag.
func ConvertWGConfText(confText string) (string, error) {
	_, peer := parseWGConfSections(confText)
	label := wgEndpointHost(peer["endpoint"])
	if label == "" {
		return "", fmt.Errorf("missing required fields: [Peer] endpoint")
	}
	return wgConfToURI(confText, label)
}

// wgEndpointHost extracts the host from a host:port endpoint ("" if malformed).
// IPv6 brackets are stripped: "[2001:db8::1]:51820" → "2001:db8::1".
func wgEndpointHost(endpoint string) string {
	i := strings.LastIndex(endpoint, ":")
	if i <= 0 {
		return ""
	}
	host := strings.TrimSpace(endpoint[:i])
	return strings.Trim(host, "[]")
}
