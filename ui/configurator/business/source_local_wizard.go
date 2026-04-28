package business

import (
	"errors"
	"strings"

	"singbox-launcher/core/config"
)

// Markers in proxies[i].outbounds[].comment (SPEC 026).
const (
	WizardMarkerAuto   = "WIZARD:auto"
	WizardMarkerSelect = "WIZARD:selector"
	// wizardMarkerSelectLegacy matches older comments that used "WIZARD:select" instead of WIZARD:selector.
	wizardMarkerSelectLegacy = "WIZARD:select"
)

// ErrWizardOutboundTagConflict is returned when enabling a wizard local group would collide with a non-wizard outbound tag.
var ErrWizardOutboundTagConflict = errors.New("wizard local outbound: tag already used without WIZARD marker")

func effectiveTagPrefix(tagPrefix string, sourceIndex int) string {
	p := strings.TrimSpace(tagPrefix)
	if p != "" {
		return p
	}
	return GenerateTagPrefix(sourceIndex + 1)
}

// LocalAutoOutboundTag returns trim(tag_prefix)+"auto" (SPEC §1).
func LocalAutoOutboundTag(tagPrefix string, sourceIndex int) string {
	return effectiveTagPrefix(tagPrefix, sourceIndex) + "auto"
}

// LocalSelectOutboundTag returns trim(tag_prefix)+"select" (SPEC §1).
func LocalSelectOutboundTag(tagPrefix string, sourceIndex int) string {
	return effectiveTagPrefix(tagPrefix, sourceIndex) + "select"
}

func commentHasWizardAuto(comment string) bool {
	return strings.Contains(comment, WizardMarkerAuto)
}

func commentHasWizardSelect(comment string) bool {
	return strings.Contains(comment, WizardMarkerSelect) || strings.Contains(comment, wizardMarkerSelectLegacy)
}

// ProxyHasLocalAuto reports whether the source has an outbound with WIZARD:auto marker.
func ProxyHasLocalAuto(proxy *config.ProxySource) bool {
	if proxy == nil {
		return false
	}
	for _, ob := range proxy.Outbounds {
		if commentHasWizardAuto(ob.Comment) {
			return true
		}
	}
	return false
}

// ProxyHasLocalSelect reports whether the source has an outbound with WIZARD select marker.
func ProxyHasLocalSelect(proxy *config.ProxySource) bool {
	if proxy == nil {
		return false
	}
	for _, ob := range proxy.Outbounds {
		if commentHasWizardSelect(ob.Comment) {
			return true
		}
	}
	return false
}

func removeOutboundsWithCommentPredicate(outbounds []config.OutboundConfig, pred func(string) bool) []config.OutboundConfig {
	if len(outbounds) == 0 {
		return outbounds
	}
	out := make([]config.OutboundConfig, 0, len(outbounds))
	for _, ob := range outbounds {
		if pred(ob.Comment) {
			continue
		}
		out = append(out, ob)
	}
	return out
}

// RemoveWizardAutoOutbounds removes all local outbounds whose comment contains WIZARD:auto.
func RemoveWizardAutoOutbounds(proxy *config.ProxySource) {
	if proxy == nil {
		return
	}
	proxy.Outbounds = removeOutboundsWithCommentPredicate(proxy.Outbounds, commentHasWizardAuto)
}

// RemoveWizardSelectOutbounds removes all local outbounds whose comment contains WIZARD select markers.
func RemoveWizardSelectOutbounds(proxy *config.ProxySource) {
	if proxy == nil {
		return
	}
	proxy.Outbounds = removeOutboundsWithCommentPredicate(proxy.Outbounds, commentHasWizardSelect)
}

// SyncExposeFlagWhenNoLocalGroups clears expose_group_tags_to_global if neither local group exists.
func SyncExposeFlagWhenNoLocalGroups(proxy *config.ProxySource) {
	if proxy == nil {
		return
	}
	if !ProxyHasLocalAuto(proxy) && !ProxyHasLocalSelect(proxy) {
		proxy.ExposeGroupTagsToGlobal = false
	}
}

func tagUsedByNonWizardOutbound(outbounds []config.OutboundConfig, tag string, wizardOK func(string) bool) bool {
	for _, ob := range outbounds {
		if ob.Tag != tag {
			continue
		}
		if wizardOK(ob.Comment) {
			continue
		}
		return true
	}
	return false
}

var defaultLocalURLTestOptions = map[string]interface{}{
	"url":                         "https://cp.cloudflare.com/generate_204",
	"interval":                    "5m",
	"tolerance":                   100,
	"interrupt_exist_connections": true,
}

// EnsureLocalAuto creates or keeps a urltest outbound with WIZARD:auto marker.
func EnsureLocalAuto(proxy *config.ProxySource, sourceIndex int) error {
	if proxy == nil {
		return nil
	}
	if ProxyHasLocalAuto(proxy) {
		return nil
	}
	autoTag := LocalAutoOutboundTag(proxy.TagPrefix, sourceIndex)
	if tagUsedByNonWizardOutbound(proxy.Outbounds, autoTag, commentHasWizardAuto) {
		return ErrWizardOutboundTagConflict
	}
	proxy.Outbounds = removeOutboundsWithCommentPredicate(proxy.Outbounds, commentHasWizardAuto)
	proxy.Outbounds = append(proxy.Outbounds, config.OutboundConfig{
		Tag:      autoTag,
		Type:     "urltest",
		Options:  defaultLocalURLTestOptions,
		Filters:  map[string]interface{}{},
		Comment:  "local auto " + WizardMarkerAuto,
	})
	return nil
}

// EnsureLocalSelect creates or keeps a selector with WIZARD:selector marker and default on local auto.
func EnsureLocalSelect(proxy *config.ProxySource, sourceIndex int) error {
	if proxy == nil {
		return nil
	}
	if err := EnsureLocalAuto(proxy, sourceIndex); err != nil {
		return err
	}
	if ProxyHasLocalSelect(proxy) {
		return nil
	}
	autoTag := LocalAutoOutboundTag(proxy.TagPrefix, sourceIndex)
	selTag := LocalSelectOutboundTag(proxy.TagPrefix, sourceIndex)
	if tagUsedByNonWizardOutbound(proxy.Outbounds, selTag, commentHasWizardSelect) {
		return ErrWizardOutboundTagConflict
	}
	proxy.Outbounds = removeOutboundsWithCommentPredicate(proxy.Outbounds, commentHasWizardSelect)
	opts := map[string]interface{}{
		"interrupt_exist_connections": true,
		"default":                     autoTag,
	}
	proxy.Outbounds = append(proxy.Outbounds, config.OutboundConfig{
		Tag:          selTag,
		Type:         "selector",
		Options:      opts,
		Filters:      map[string]interface{}{},
		AddOutbounds: []string{autoTag},
		Comment:      "local select " + WizardMarkerSelect,
	})
	return nil
}

// RenameWizardLocalOutboundTags updates tag fields and select→auto references when tag_prefix changes.
func RenameWizardLocalOutboundTags(proxy *config.ProxySource, sourceIndex int) {
	if proxy == nil {
		return
	}
	autoTag := LocalAutoOutboundTag(proxy.TagPrefix, sourceIndex)
	selTag := LocalSelectOutboundTag(proxy.TagPrefix, sourceIndex)
	for i := range proxy.Outbounds {
		ob := &proxy.Outbounds[i]
		if commentHasWizardAuto(ob.Comment) {
			ob.Tag = autoTag
		}
		if commentHasWizardSelect(ob.Comment) {
			ob.Tag = selTag
			ob.AddOutbounds = []string{autoTag}
			if ob.Options == nil {
				ob.Options = map[string]interface{}{}
			}
			ob.Options["default"] = autoTag
		}
	}
}
