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

// shareURIWithSecretFromRoot строит ссылку узла tag и СРАЗУ отвечает, несёт
// ли она приватный ключ.
//
// Флаг считается по ТЕЛУ узла (subscription.ShareURICarriesPrivateKey), а не
// по готовой строке: разбирать собственный вывод обратно — лишний шаг, на
// котором предупреждение однажды разойдётся с эмиттером.
func shareURIWithSecretFromRoot(root map[string]interface{}, tag string) (string, bool, error) {
	if tag == "" {
		return "", false, fmt.Errorf("empty outbound tag")
	}
	if root == nil {
		return "", false, fmt.Errorf("nil config root")
	}
	out, outErr := findTaggedInRoot(root, tag, "outbounds", "outbound with tag %q not found")
	if outErr == nil {
		uri, err := subscription.ShareURIFromOutbound(out)
		return uri, err == nil && subscription.ShareURICarriesPrivateKey(out), err
	}
	if shareURITryEndpointAfterOutboundError(outErr) {
		ep, epErr := findTaggedInRoot(root, tag, "endpoints", "endpoint with tag %q not found")
		if epErr == nil {
			uri, err := subscription.ShareURIFromWireGuardEndpoint(ep)
			return uri, err == nil && subscription.ShareURICarriesPrivateKey(ep), err
		}
	}
	return "", false, outErr
}

// ShareProxyURIForOutboundTagFromRoot builds a share URI like ShareProxyURIForOutboundTag using an already-parsed config root.
func ShareProxyURIForOutboundTagFromRoot(root map[string]interface{}, tag string) (string, error) {
	uri, _, err := shareURIWithSecretFromRoot(root, tag)
	return uri, err
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
//
// WireGuard/AmneziaWG узлы живут в endpoints[], а не в outbounds[] (sing-box
// >= 1.11), поэтому при промахе по outbounds[] пробуем endpoints[] — иначе
// «Копировать ссылку сервера» падает на каждом wireguard-узле.
func ShareMainURIForOutboundTag(configPath, tag string) (string, error) {
	uri, _, err := ShareMainURIWithSecretForOutboundTag(configPath, tag)
	return uri, err
}

// ShareMainURIWithSecretForOutboundTag — то же, что ShareMainURIForOutboundTag,
// но вторым значением отдаёт признак «ссылка несёт приватный ключ»: UI обязан
// спросить подтверждение до того, как положит её в буфер.
func ShareMainURIWithSecretForOutboundTag(configPath, tag string) (string, bool, error) {
	if tag == "" {
		return "", false, fmt.Errorf("empty outbound tag")
	}
	root, err := loadConfigRootMap(configPath)
	if err != nil {
		return "", false, err
	}
	return shareURIWithSecretFromRoot(root, tag)
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
	uri, _, err := ShareJumpURIWithSecretForOutboundTag(configPath, tag)
	return uri, err
}

// ShareJumpURIWithSecretForOutboundTag — ссылка узла-перехода плюс признак
// приватного ключа в ней (пара к ShareMainURIWithSecretForOutboundTag).
func ShareJumpURIWithSecretForOutboundTag(configPath, tag string) (string, bool, error) {
	detourTag, err := GetDetourTagForOutboundTag(configPath, tag)
	if err != nil {
		return "", false, err
	}
	if detourTag == "" {
		return "", false, fmt.Errorf("outbound %q has no detour", tag)
	}
	root, err := loadConfigRootMap(configPath)
	if err != nil {
		return "", false, err
	}
	return shareURIWithSecretFromRoot(root, detourTag)
}

// BuildShareURILinesForOutboundTags loads config once and appends one non-empty share URI per tag in order.
// Tags that cannot be encoded (missing outbound, unsupported type, etc.) are skipped without aborting.
func BuildShareURILinesForOutboundTags(configPath string, tags []string) ([]string, error) {
	lines, _, err := BuildShareURILinesWithSecretForOutboundTags(configPath, tags)
	return lines, err
}

// BuildShareURILinesWithSecretForOutboundTags — то же, но вторым значением
// говорит, есть ли ХОТЯ БЫ ОДНА ссылка с приватным ключом во всём блоке.
// Подтверждение при массовом копировании — одно на операцию, а не на узел.
func BuildShareURILinesWithSecretForOutboundTags(configPath string, tags []string) ([]string, bool, error) {
	root, err := loadConfigRootMap(configPath)
	if err != nil {
		return nil, false, err
	}
	var lines []string
	secret := false
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		line, hasKey, err := shareURIWithSecretFromRoot(root, tag)
		if err != nil || strings.TrimSpace(line) == "" {
			continue
		}
		if hasKey {
			secret = true
		}
		lines = append(lines, line)
	}
	return lines, secret, nil
}
