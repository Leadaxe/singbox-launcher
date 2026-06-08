package build

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// TestApplyOutboundUpdatePatch_Empty — пустой patch = noop.
func TestApplyOutboundUpdatePatch_Empty(t *testing.T) {
	target := configtypes.OutboundConfig{Tag: "x", Type: "selector"}
	out := applyOutboundUpdatePatch(target, nil)
	if out.Tag != "x" || out.Type != "selector" {
		t.Errorf("noop patch changed target: %+v", out)
	}
}
