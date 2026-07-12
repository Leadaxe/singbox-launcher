package build

import (
	"testing"

	"singbox-launcher/core/state"
)

func TestBuildChannelOutbounds_SelectorAndAuto(t *testing.T) {
	channels := []state.Channel{
		{
			Tag: "vpn-1", Label: "VPN ①", Enabled: true,
			IncludeDirect: true,
			NodeFilter:    "🇩🇪", NodeFilterInvert: false,
			DefaultFilter: "Best",
			Auto: &state.ChannelAuto{
				URL: "https://cp.cloudflare.com/generate_204", Interval: "5m",
				Mode:     "round_robin",
				Balancer: &state.ChannelBalancer{Pool: 3, PoolTolerance: 30, StickyHash: []string{"process", "domain"}},
			},
		},
	}
	out := BuildChannelOutbounds(channels)
	if len(out) != 2 {
		t.Fatalf("expected selector + auto = 2 outbounds, got %d", len(out))
	}

	sel := out[0]
	if sel.Tag != "vpn-1" || sel.Type != "selector" {
		t.Errorf("selector: tag=%q type=%q", sel.Tag, sel.Type)
	}
	if sel.Filters["tag"] != "/🇩🇪/i" {
		t.Errorf("node filter = %v, want /🇩🇪/i", sel.Filters["tag"])
	}
	if sel.PreferredDefault["tag"] != "/Best/i" {
		t.Errorf("default filter = %v", sel.PreferredDefault["tag"])
	}
	// AddOutbounds should include the auto twin + direct-out.
	hasAuto, hasDirect := false, false
	for _, a := range sel.AddOutbounds {
		if a == "vpn-1-auto" {
			hasAuto = true
		}
		if a == "direct-out" {
			hasDirect = true
		}
	}
	if !hasAuto || !hasDirect {
		t.Errorf("AddOutbounds missing auto/direct: %v", sel.AddOutbounds)
	}

	auto := out[1]
	if auto.Tag != "vpn-1-auto" || auto.Type != "urltest" {
		t.Errorf("auto: tag=%q type=%q", auto.Tag, auto.Type)
	}
	if auto.Options["mode"] != "round_robin" {
		t.Errorf("auto mode = %v", auto.Options["mode"])
	}
	bal, _ := auto.Options["balancer"].(map[string]interface{})
	if bal == nil || bal["pool"] != 3 {
		t.Errorf("auto balancer = %v", auto.Options["balancer"])
	}
}

func TestBuildChannelOutbounds_InvertAndNoAuto(t *testing.T) {
	channels := []state.Channel{
		{Tag: "vpn-2", Enabled: true, NodeFilter: "ru", NodeFilterInvert: true},
	}
	out := BuildChannelOutbounds(channels)
	if len(out) != 1 { // no auto twin
		t.Fatalf("expected 1 outbound (no auto), got %d", len(out))
	}
	if out[0].Filters["tag"] != "!/ru/i" {
		t.Errorf("inverted filter = %v, want !/ru/i", out[0].Filters["tag"])
	}
}

func TestBuildChannelOutbounds_DisabledSkipped_RequiredKept(t *testing.T) {
	channels := []state.Channel{
		{Tag: "vpn-1", Enabled: false}, // required → kept even when disabled
		{Tag: "vpn-3", Enabled: false}, // non-required disabled → skipped
	}
	out := BuildChannelOutbounds(channels)
	if len(out) != 1 || out[0].Tag != "vpn-1" {
		t.Fatalf("expected only required vpn-1, got %+v", out)
	}
}

func TestBuildChannelOutbounds_EmptyChannelsNoop(t *testing.T) {
	if out := BuildChannelOutbounds(nil); out != nil {
		t.Errorf("nil channels should give nil, got %v", out)
	}
}

func TestChannelNodeFilter_BadRegexMatchAll(t *testing.T) {
	// invalid regex → nil (match-all), not a zero-match filter
	if f := channelNodeFilter("[", false); f != nil {
		t.Errorf("bad regex should yield nil (match-all), got %v", f)
	}
	if f := channelNodeFilter("", false); f != nil {
		t.Errorf("empty filter should yield nil, got %v", f)
	}
}
