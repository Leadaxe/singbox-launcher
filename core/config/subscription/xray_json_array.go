package subscription

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/textnorm"
)

// IsXrayJSONArrayBody reports whether s is a valid JSON array (used for subscription branch).
func IsXrayJSONArrayBody(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") {
		return false
	}
	if !json.Valid([]byte(s)) {
		return false
	}
	var raw []json.RawMessage
	return json.Unmarshal([]byte(s), &raw) == nil
}

// ParseNodesFromXrayJSONArray parses a JSON array of Xray-style full configs into ParsedNode list.
// Non-Xray elements (e.g. sing-box-only outbounds) are skipped with a debug log.
// skip uses the same rules as URI subscriptions (shouldSkipNode).
func ParseNodesFromXrayJSONArray(jsonBody string, skip []map[string]string) ([]*configtypes.ParsedNode, error) {
	jsonBody = strings.TrimSpace(jsonBody)
	var elems []json.RawMessage
	if err := json.Unmarshal([]byte(jsonBody), &elems); err != nil {
		return nil, fmt.Errorf("subscription JSON array: %w", err)
	}

	var out []*configtypes.ParsedNode
	for i, raw := range elems {
		node, err := parseXrayJSONArrayElement(raw, i, skip)
		if err != nil {
			debuglog.WarnLog("Parser: Xray JSON array element %d: %v", i, err)
			continue
		}
		if node != nil {
			out = append(out, node)
		}
	}
	return out, nil
}

func parseXrayJSONArrayElement(raw json.RawMessage, elemIndex int, skip []map[string]string) (*configtypes.ParsedNode, error) {
	var root map[string]interface{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("invalid element JSON: %w", err)
	}

	if !xrayElementHasProtocolOutbounds(root) {
		debuglog.DebugLog("Parser: Xray JSON array element %d: skip (no Xray protocol outbounds; sing-box array is follow-up)", elemIndex)
		return nil, nil
	}

	outboundsRaw, ok := root["outbounds"].([]interface{})
	if !ok || len(outboundsRaw) == 0 {
		return nil, fmt.Errorf("missing outbounds")
	}

	byTag := make(map[string]map[string]interface{})
	var vlessCands []struct {
		ob     map[string]interface{}
		dialer string
		tag    string
	}

	for _, obRaw := range outboundsRaw {
		ob, ok := obRaw.(map[string]interface{})
		if !ok {
			continue
		}
		tag := xrayMapString(ob, "tag")
		if tag != "" {
			byTag[tag] = ob
		}
		prot := strings.ToLower(xrayMapString(ob, "protocol"))
		if prot != "vless" {
			continue
		}
		settings, _ := ob["settings"].(map[string]interface{})
		if settings == nil {
			continue
		}
		vnext, ok := settings["vnext"].([]interface{})
		if !ok || len(vnext) == 0 {
			continue
		}
		streamSettings, _ := ob["streamSettings"].(map[string]interface{})
		dialer := xraySockoptDialerRef(streamSettings)
		vlessCands = append(vlessCands, struct {
			ob     map[string]interface{}
			dialer string
			tag    string
		}{ob: ob, dialer: dialer, tag: tag})
	}

	if len(vlessCands) == 0 {
		return nil, nil
	}

	mainOb := pickMainXrayVLESS(vlessCands, elemIndex)

	remarksRaw := strings.TrimSpace(xrayMapString(root, "remarks"))
	label := remarksRaw
	if label == "" {
		label = xrayMapString(mainOb, "tag")
	}
	if label == "" {
		label = fmt.Sprintf("xray-%d", elemIndex)
	}

	node, err := xrayBuildVLESSFromOutbound(mainOb, label)
	if err != nil {
		return nil, fmt.Errorf("vless mapping: %w", err)
	}

	base := xrayTagBaseFromRoot(remarksRaw, elemIndex)
	mainTag := base
	jumpTag := base + xrayJumpOutboundTagSuffix
	node.Tag = mainTag
	if node.Outbound != nil {
		node.Outbound["tag"] = mainTag
	}

	streamSettings, _ := mainOb["streamSettings"].(map[string]interface{})
	dialerRef := xraySockoptDialerRef(streamSettings)
	if dialerRef != "" {
		jumpOb, ok := byTag[dialerRef]
		if !ok {
			debuglog.WarnLog("Parser: Xray element %d: dialerProxy %q not found, skipping node", elemIndex, dialerRef)
			return nil, nil
		}
		jump, err := xrayBuildJumpFromOutbound(jumpOb, jumpTag, label)
		if err != nil {
			debuglog.WarnLog("Parser: Xray element %d: dialerProxy %q jump outbound: %v, skipping node", elemIndex, dialerRef, err)
			return nil, nil
		}
		node.Jump = jump
	}

	if shouldSkipNode(node, skip) {
		return nil, nil
	}

	return node, nil
}

func xrayElementHasProtocolOutbounds(root map[string]interface{}) bool {
	outboundsRaw, ok := root["outbounds"].([]interface{})
	if !ok {
		return false
	}
	for _, obRaw := range outboundsRaw {
		ob, ok := obRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if _, ok := ob["protocol"].(string); ok {
			return true
		}
	}
	return false
}

func pickMainXrayVLESS(cands []struct {
	ob     map[string]interface{}
	dialer string
	tag    string
}, elemIndex int) map[string]interface{} {
	var withDial []int
	for i, c := range cands {
		if c.dialer != "" {
			withDial = append(withDial, i)
		}
	}

	pickIdx := 0
	switch {
	case len(withDial) == 1:
		pickIdx = withDial[0]
	case len(withDial) > 1:
		pickIdx = withDial[0]
		for _, i := range withDial {
			if cands[i].tag == "proxy" {
				pickIdx = i
				break
			}
		}
		debuglog.WarnLog("Parser: Xray element %d: multiple VLESS with dialerProxy; using tag %q",
			elemIndex, cands[pickIdx].tag)
	default:
		if len(cands) == 1 {
			pickIdx = 0
		} else {
			for i, c := range cands {
				if c.tag == "proxy" {
					pickIdx = i
					break
				}
			}
			debuglog.WarnLog("Parser: Xray element %d: multiple VLESS without dialerProxy; using tag %q",
				elemIndex, cands[pickIdx].tag)
		}
	}
	return cands[pickIdx].ob
}

const xrayTagBaseMaxRunes = 48

// Suffix for the SOCKS jump outbound tag (main outbound uses the base slug only; detour points at base+suffix).
const xrayJumpOutboundTagSuffix = "_jump_server"

// xrayTagBaseFromRoot returns a stable tag prefix for main/jump outbounds.
// When remarks is non-empty, it is sanitized into a slug; otherwise "xray-{index}".
func xrayTagBaseFromRoot(remarksRaw string, elemIndex int) string {
	if strings.TrimSpace(remarksRaw) == "" {
		return fmt.Sprintf("xray-%d", elemIndex)
	}
	return xrayRemarksToTagBase(remarksRaw, elemIndex)
}

// xrayRemarksToTagBase builds a tag slug from Xray remarks (for sing-box tag fields):
// letters and digits (any script), Unicode regional indicators (flag emoji, e.g. 🇱🇻), and hyphens between runs.
func xrayRemarksToTagBase(remarks string, elemIndex int) string {
	s := strings.TrimSpace(textnorm.NormalizeProxyDisplay(remarks))
	if s == "" {
		return fmt.Sprintf("xray-%d", elemIndex)
	}
	var b strings.Builder
	lastSep := false
	for _, r := range s {
		if xrayTagSlugKeepRune(r) {
			b.WriteRune(r)
			lastSep = false
			continue
		}
		if r == '_' || r == '-' {
			if b.Len() > 0 && !lastSep {
				b.WriteRune('-')
				lastSep = true
			}
			continue
		}
		// spaces, punctuation, emoji → single hyphen between word runs
		if b.Len() > 0 && !lastSep {
			b.WriteRune('-')
			lastSep = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fmt.Sprintf("xray-%d", elemIndex)
	}
	runes := []rune(out)
	if len(runes) > xrayTagBaseMaxRunes {
		out = string(runes[:xrayTagBaseMaxRunes])
		out = strings.TrimRight(out, "-")
	}
	if out == "" {
		return fmt.Sprintf("xray-%d", elemIndex)
	}
	return out
}

// xrayTagSlugKeepRune returns true for letters, digits, and Regional Indicator symbols (U+1F1E6..U+1F1FF),
// which form UTF-8 flag emoji in pairs. Other symbols (punctuation, generic emoji) are not kept in the slug.
func xrayTagSlugKeepRune(r rune) bool {
	if unicode.IsLetter(r) || unicode.IsNumber(r) {
		return true
	}
	return r >= 0x1F1E6 && r <= 0x1F1FF
}
