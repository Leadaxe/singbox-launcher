package configtypes

import (
	"strings"
	"testing"
)

// TestBuildSubscriptionUserAgent guards the fix for the "panel serves JSON
// config instead of subscription list" bug. Invariants:
//   - product brand token is "LxBox/" with the "desktop" variant tag;
//   - a bare "singbox" (no hyphen) never appears — it is the trigger that makes
//     substring-matching panels (Remnawave/Marzban-style) serve a full
//     client-config JSON instead of the base64/URI subscription list.
func TestBuildSubscriptionUserAgent(t *testing.T) {
	ua := BuildSubscriptionUserAgent()

	if !strings.HasPrefix(ua, "LxBox/") {
		t.Errorf("UA must start with brand token LxBox/, got %q", ua)
	}
	if !strings.Contains(ua, "desktop") {
		t.Errorf("UA must carry the 'desktop' variant tag, got %q", ua)
	}
	if strings.Contains(ua, "singbox") {
		t.Errorf("UA must not contain bare 'singbox' (panels mis-route it), got %q", ua)
	}
}
