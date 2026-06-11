package tabs

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 077: the detour picker writes scratch.DetourTag; applyProxyEditToSource
// must persist it back to the canonical Source so the choice survives closing
// and reopening the edit window (criterion 5). Covers both source types.
func TestApplyProxyEditToSource_DetourPersists(t *testing.T) {
	t.Run("subscription", func(t *testing.T) {
		ps := &configtypes.ProxySource{Source: "https://x/sub", DetourTag: "hop-out"}
		var src wizardmodels.Source
		applyProxyEditToSource(ps, &src)
		if src.Type != wizardmodels.SourceTypeSubscription {
			t.Fatalf("type = %q, want subscription", src.Type)
		}
		if src.DetourTag != "hop-out" {
			t.Errorf("DetourTag = %q, want hop-out", src.DetourTag)
		}
	})

	t.Run("server", func(t *testing.T) {
		ps := &configtypes.ProxySource{Connections: []string{"vless://u@h:443#a"}, TagMask: "srv", DetourTag: "hop-out"}
		var src wizardmodels.Source
		applyProxyEditToSource(ps, &src)
		if src.Type != wizardmodels.SourceTypeServer {
			t.Fatalf("type = %q, want server", src.Type)
		}
		if src.DetourTag != "hop-out" {
			t.Errorf("DetourTag = %q, want hop-out", src.DetourTag)
		}
	})

	// Clearing the detour (picking "(none)") writes empty and must persist.
	t.Run("cleared", func(t *testing.T) {
		ps := &configtypes.ProxySource{Source: "https://x/sub", DetourTag: ""}
		src := wizardmodels.Source{DetourTag: "stale"}
		applyProxyEditToSource(ps, &src)
		if src.DetourTag != "" {
			t.Errorf("DetourTag = %q, want empty after clear", src.DetourTag)
		}
	})
}
