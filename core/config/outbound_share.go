package config

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/config/subscription"
)

func loadConfigRootMap(configPath string) (map[string]interface{}, error) {
	cleanData, err := getConfigJSON(configPath)
	if err != nil {
		return nil, err
	}
	var root map[string]interface{}
	if err := json.Unmarshal(cleanData, &root); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return root, nil
}

func findTaggedInRoot(root map[string]interface{}, tag, arrayKey, notFoundFmt string) (map[string]interface{}, error) {
	rawList, ok := root[arrayKey].([]interface{})
	if !ok {
		return nil, fmt.Errorf("%s not found or invalid", arrayKey)
	}
	for _, raw := range rawList {
		om, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if t, _ := om["tag"].(string); t == tag {
			return om, nil
		}
	}
	return nil, fmt.Errorf(notFoundFmt, tag)
}

// GetOutboundMapByTag returns the raw outbound object from config.json outbounds[] with the given tag.
func GetOutboundMapByTag(configPath, tag string) (map[string]interface{}, error) {
	if tag == "" {
		return nil, fmt.Errorf("empty outbound tag")
	}
	root, err := loadConfigRootMap(configPath)
	if err != nil {
		return nil, err
	}
	return findTaggedInRoot(root, tag, "outbounds", "outbound with tag %q not found")
}

// shareURITryEndpointAfterOutboundError is true when the tag is missing from outbounds (or outbounds absent), so we may resolve WireGuard in endpoints[].
func shareURITryEndpointAfterOutboundError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "not found") || strings.Contains(s, "outbounds not found")
}

// ShareProxyURIForOutboundTagFromRoot builds a share URI like ShareProxyURIForOutboundTag using an already-parsed config root.
func ShareProxyURIForOutboundTagFromRoot(root map[string]interface{}, tag string) (string, error) {
	if tag == "" {
		return "", fmt.Errorf("empty outbound tag")
	}
	if root == nil {
		return "", fmt.Errorf("nil config root")
	}
	out, outErr := findTaggedInRoot(root, tag, "outbounds", "outbound with tag %q not found")
	if outErr == nil {
		return subscription.ShareURIFromOutbound(out)
	}
	if shareURITryEndpointAfterOutboundError(outErr) {
		ep, epErr := findTaggedInRoot(root, tag, "endpoints", "endpoint with tag %q not found")
		if epErr == nil {
			return subscription.ShareURIFromWireGuardEndpoint(ep)
		}
	}
	return "", outErr
}

// ShareProxyURIForOutboundTag builds a subscription-style share URI from the sing-box outbound with the given tag,
// or from a WireGuard entry in endpoints[] with that tag if no matching outbound exists.
// Parses config.json once per call.
func ShareProxyURIForOutboundTag(configPath, tag string) (string, error) {
	if tag == "" {
		return "", fmt.Errorf("empty outbound tag")
	}
	root, err := loadConfigRootMap(configPath)
	if err != nil {
		return "", err
	}
	return ShareProxyURIForOutboundTagFromRoot(root, tag)
}

// ShareMainURIForOutboundTag builds a share URI for the outbound itself.
// If detour is present, it is ignored (removed) so the main hop can still be exported.
func ShareMainURIForOutboundTag(configPath, tag string) (string, error) {
	out, err := GetOutboundMapByTag(configPath, tag)
	if err != nil {
		return "", err
	}
	return subscription.ShareURIFromOutbound(out)
}

// GetDetourTagForOutboundTag returns outbound.detour for the given outbound tag.
// Empty result means no detour configured.
func GetDetourTagForOutboundTag(configPath, tag string) (string, error) {
	out, err := GetOutboundMapByTag(configPath, tag)
	if err != nil {
		return "", err
	}
	detour, _ := out["detour"].(string)
	return strings.TrimSpace(detour), nil
}

// ShareJumpURIForOutboundTag builds a share URI for the jump outbound referenced by detour.
func ShareJumpURIForOutboundTag(configPath, tag string) (string, error) {
	detourTag, err := GetDetourTagForOutboundTag(configPath, tag)
	if err != nil {
		return "", err
	}
	if detourTag == "" {
		return "", fmt.Errorf("outbound %q has no detour", tag)
	}
	return ShareProxyURIForOutboundTag(configPath, detourTag)
}

// BuildShareURILinesForOutboundTags loads config once and appends one non-empty share URI per tag in order.
// Tags that cannot be encoded (missing outbound, unsupported type, etc.) are skipped without aborting.
func BuildShareURILinesForOutboundTags(configPath string, tags []string) ([]string, error) {
	root, err := loadConfigRootMap(configPath)
	if err != nil {
		return nil, err
	}
	var lines []string
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		line, err := ShareProxyURIForOutboundTagFromRoot(root, tag)
		if err != nil || strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil
}
