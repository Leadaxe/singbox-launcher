package configtypes

import (
	"strings"
	"testing"
)

// TestBuildSubscriptionUserAgent guards the fix for the "panel serves JSON
// config instead of subscription list" bug. Two invariants:
//   - product brand token is "LxBox-desktop/" (distinguishes from Android LxBox);
//   - a "sing-box" token is present so substring-matching panels
//     (Remnawave/Marzban-style) recognize a sing-box client and return the
//     base64/URI list the launcher can ingest — and a bare "singbox" (no hyphen,
//     the failure trigger) never appears.
func TestBuildSubscriptionUserAgent(t *testing.T) {
	ua := BuildSubscriptionUserAgent()

	if !strings.HasPrefix(ua, "LxBox-desktop/") {
		t.Errorf("UA must start with brand token LxBox-desktop/, got %q", ua)
	}
	// The whole point: no bare "singbox" anywhere, but "sing-box" present.
	if strings.Contains(ua, "singbox") {
		t.Errorf("UA must not contain bare 'singbox' (panels mis-route it), got %q", ua)
	}
	if !strings.Contains(ua, "sing-box/") {
		t.Errorf("UA must carry a 'sing-box/<core>' token for panel recognition, got %q", ua)
	}
}
