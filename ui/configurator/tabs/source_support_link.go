package tabs

import (
	"net/url"
	"strings"

	"fyne.io/fyne/v2/widget"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/fynewidget"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/urlsafe"
	"singbox-launcher/ui/icons"
)

// supportLinkButton builds a small clickable icon for a source's provider
// support / web-page link, to sit inline in the row's action cluster (info
// panel) — no extra row height, no visible URL text.
//
//   - URL = meta.SupportURL (preferred) or meta.ProfileWebPageURL.
//   - Icon: blue Telegram plane for Telegram links (t.me / tg://), else a
//     generic link icon.
//   - Tooltip: the full URL.
//   - Click: opens the URL for safe schemes (http/https/tg, per urlsafe); an
//     unsafe-but-present URL still shows the icon + tooltip but does not open.
//   - Nothing present → nil (caller omits the button).
func supportLinkButton(meta *corestate.SubscriptionMeta, rowGetter func() *fynewidget.HoverRow) *fynewidget.HoverForwardButton {
	if meta == nil {
		return nil
	}
	raw := strings.TrimSpace(meta.SupportURL)
	if raw == "" {
		raw = strings.TrimSpace(meta.ProfileWebPageURL)
	}
	if raw == "" {
		return nil
	}

	iconRes := icons.Link
	if isTelegramURL(raw) {
		iconRes = icons.Telegram
	}
	safe := urlsafe.IsSafeAnnounceURL(raw)

	btn := fynewidget.NewHoverForwardButtonWithIcon("", iconRes, func() {
		if !safe {
			return
		}
		if err := platform.OpenURL(raw); err != nil {
			debuglog.WarnLog("source support link: open %q failed: %v", raw, err)
		}
	}, rowGetter)
	btn.Importance = widget.LowImportance
	fynewidget.SetToolTipSafe(btn, raw)
	return btn
}

// isTelegramURL reports whether raw is a Telegram link: scheme tg:// or a
// t.me / telegram.* host.
func isTelegramURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Scheme, "tg") {
		return true
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "t.me", "telegram.me", "telegram.org", "telegram.dog":
		return true
	}
	return strings.HasSuffix(host, ".t.me")
}
