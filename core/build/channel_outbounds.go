// Package build — channel_outbounds.go: materializes routing channels (SPEC 087,
// port of LxBox _buildChannelGroups) into selector/urltest OutboundConfig entries
// at build time. These are ephemeral — injected into a copy of parserCfg.Outbounds
// before the native generator runs, never persisted to state.
package build

import (
	"regexp"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
)

const (
	directOutboundTag = "direct-out"
	blockOutboundTag  = "block-out"
)

// BuildChannelOutbounds materializes the enabled channels into selector (+ optional
// urltest twin) OutboundConfig entries. The native generator
// (GenerateSelectorWithFilteredAddOutbounds) then filters nodes by the tag regex,
// resolves the default, and drops empty selectors. A channel with an empty node
// set still gets direct-out/block-out in AddOutbounds so its selector is never
// empty (an empty selector is fatal in sing-box).
//
// Returns nil for no active channels — a zero-diff no-op for users without channels.
func BuildChannelOutbounds(channels []state.Channel) []configtypes.OutboundConfig {
	if len(channels) == 0 {
		return nil
	}
	out := make([]configtypes.OutboundConfig, 0, len(channels)*2)
	for _, ch := range channels {
		if !ch.Enabled && !ch.IsRequired() {
			continue
		}
		out = append(out, buildChannelSelector(ch)...)
	}
	return out
}

// buildChannelSelector builds the selector (and optional urltest twin) for one
// channel. Split out for testability.
func buildChannelSelector(ch state.Channel) []configtypes.OutboundConfig {
	filters := channelNodeFilter(ch.NodeFilter, ch.NodeFilterInvert)

	// AddOutbounds: direct-out / block-out per channel flags, plus the auto twin.
	// direct-out is always added so the selector is never empty even when no node
	// matches (SPEC 087 §6 empty-channel policy). block-out is disallowed on a
	// detour channel (it must forward, not drop).
	add := make([]string, 0, 3)
	emitAuto := ch.Auto != nil
	if emitAuto {
		add = append(add, ch.AutoTag())
	}
	if ch.IncludeBlock && !ch.IsDetour {
		add = append(add, blockOutboundTag)
	}
	// direct-out: honor IncludeDirect; also force it as the non-empty guard unless
	// this is a detour channel wanting block. Keeping it last preserves node
	// priority in the selector list.
	if ch.IncludeDirect || (!ch.IncludeBlock && !ch.IsDetour) {
		add = append(add, directOutboundTag)
	}

	selector := configtypes.OutboundConfig{
		Tag:          ch.Tag,
		Type:         "selector",
		Filters:      filters,
		AddOutbounds: add,
		Options: map[string]interface{}{
			"interrupt_exist_connections": ch.InterruptExistConnections,
		},
		Comment: channelComment(ch),
	}
	if pd := channelPreferredDefault(ch.DefaultFilter); pd != nil {
		selector.PreferredDefault = pd
	}

	res := []configtypes.OutboundConfig{selector}
	if emitAuto {
		res = append(res, buildChannelAuto(ch))
	}
	return res
}

// buildChannelAuto builds the urltest twin (<tag>-auto) for a channel. It carries
// only the same node filter (no direct/block/auto members) plus the urltest and
// balancer options.
func buildChannelAuto(ch state.Channel) configtypes.OutboundConfig {
	a := ch.Auto
	opts := map[string]interface{}{}
	if a.URL != "" {
		opts["url"] = a.URL
	}
	if a.Interval != "" {
		opts["interval"] = a.Interval
	}
	if a.Tolerance > 0 {
		opts["tolerance"] = a.Tolerance
	}
	if a.IdleTimeout != "" {
		opts["idle_timeout"] = a.IdleTimeout
	}
	opts["interrupt_exist_connections"] = a.InterruptExistConnections

	// round_robin → mode + balancer (least_test stays bit-exact with upstream: no
	// mode/balancer). sanitizeBalancerOptions (generator) drops a bare empty
	// sticky_hash — off is only the ["none"] sentinel.
	if a.Mode == "round_robin" {
		opts["mode"] = "round_robin"
		if a.Balancer != nil {
			b := map[string]interface{}{
				"pool":           a.Balancer.Pool,
				"pool_tolerance": a.Balancer.PoolTolerance,
			}
			if len(a.Balancer.StickyHash) > 0 {
				b["sticky_hash"] = toIfaceSlice(a.Balancer.StickyHash)
			}
			opts["balancer"] = b
		}
	}

	return configtypes.OutboundConfig{
		Tag:     ch.AutoTag(),
		Type:    "urltest",
		Filters: channelNodeFilter(ch.NodeFilter, ch.NodeFilterInvert),
		Options: opts,
		Comment: channelComment(ch) + " (auto)",
	}
}

// channelNodeFilter maps a channel's node_filter regex (+ invert) to the
// generator's Filters{"tag": ...} form. An empty filter yields nil (all nodes).
// A syntactically invalid regex yields nil too (LxBox treats a bad filter as
// "all nodes"; the Go matcher would otherwise match zero — so we drop it and
// warn, keeping the channel usable).
func channelNodeFilter(filter string, invert bool) map[string]interface{} {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil
	}
	if _, err := regexp.Compile(filter); err != nil {
		debuglog.WarnLog("Build: channel node_filter %q is not a valid regex — treating as match-all: %v", filter, err)
		return nil
	}
	pattern := "/" + filter + "/i"
	if invert {
		pattern = "!" + pattern
	}
	return map[string]interface{}{"tag": pattern}
}

// channelPreferredDefault maps default_filter to PreferredDefault{"tag": /re/i}.
// Invalid/empty → nil.
func channelPreferredDefault(filter string) map[string]interface{} {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil
	}
	if _, err := regexp.Compile(filter); err != nil {
		debuglog.WarnLog("Build: channel default_filter %q is not a valid regex — ignored: %v", filter, err)
		return nil
	}
	return map[string]interface{}{"tag": "/" + filter + "/i"}
}

func channelComment(ch state.Channel) string {
	if ch.Label != "" {
		return ch.Label
	}
	return ch.Tag
}

func toIfaceSlice(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
