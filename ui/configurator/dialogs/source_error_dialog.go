// Package dialogs — File source_error_dialog.go.
//
// Modal opened from the Sources tab when a subscription source has a
// non-empty `meta.LastErrorMsg` OR a `meta.ProviderAnnounce` (the latter
// is the success-with-notice path — provider sent contentful body PLUS
// an Announce header). SPEC 061 Phase 3.
//
// Two surfacing paths funnel into the same dialog:
//   - Click on the ⚠ (error) / 📢 (notice) icon-button in a source row.
//   - Auto-popup after a manual Update click when the response was an
//     error/notice (so first encounter doesn't need a second click).
//
// Layout (top → bottom):
//
//   ┌────────────────────────────────────────────┐
//   │ ⚠ <ProfileTitle or source label>      [×] │  ← title
//   ├────────────────────────────────────────────┤
//   │ HTTP 200 · empty body                      │  ← http status line
//   │                                            │
//   │ <decoded announce or LastErrorMsg>         │  ← wrap text
//   │                                            │
//   │ [🔗 Open https://t.me/nash_vpn_bot]        │  ← when URL is safe
//   │                                            │
//   │ Last attempt: 2026-05-25 19:04             │  ← footer meta
//   │ Errors in a row: 3                         │
//   └────────────────────────────────────────────┘
//
// URL safety: `internal/urlsafe.IsSafeAnnounceURL` gates the clickable
// button (http/https/tg only). Invalid → URL shown as plain text under
// the message, no `platform.OpenURL` invocation.
package dialogs

import (
	"fmt"
	"net/url"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
	"singbox-launcher/internal/urlsafe"
)

// ShowSourceErrorDialog renders the error/notice modal for a single
// subscription source. `sourceLabel` is the row's display name (e.g.
// "NashVPN" or first 32 chars of URL) used as the dialog title fallback
// when `meta.ProviderAnnounce.ProfileTitle` is empty. `meta` must be
// non-nil; caller checks for emptiness before opening.
//
// Safe to call with meta == nil (silently skips); UI just falls through
// without a popup. Same with parent == nil.
func ShowSourceErrorDialog(parent fyne.Window, sourceLabel string, meta *state.SubscriptionMeta) {
	if parent == nil || meta == nil {
		return
	}

	title := dialogTitle(sourceLabel, meta)
	body := buildSourceErrorBody(parent, meta)

	d := dialog.NewCustom(title, "Close", body, parent)
	d.Resize(fyne.NewSize(520, 380))
	d.Show()
}

// dialogTitle picks the most informative header for the modal.
// ProviderAnnounce.ProfileTitle wins (provider-supplied display name);
// fallback to the source row's own label; final fallback to a generic
// string so the title bar is never blank.
func dialogTitle(sourceLabel string, meta *state.SubscriptionMeta) string {
	prefix := "⚠ "
	if meta.LastStatus != "err" && meta.ProviderAnnounce != nil && !meta.ProviderAnnounce.IsEmpty() {
		// Success-with-notice path → info, not warning.
		prefix = "📢 "
	}
	if meta.ProviderAnnounce != nil && meta.ProviderAnnounce.ProfileTitle != "" {
		return prefix + meta.ProviderAnnounce.ProfileTitle
	}
	if meta.ProfileTitle != "" {
		return prefix + meta.ProfileTitle
	}
	if sourceLabel != "" {
		return prefix + sourceLabel
	}
	return prefix + "Subscription"
}

// buildSourceErrorBody assembles the scrollable content area. Sections
// are conditionally added so a sparse meta (e.g. just LastErrorMsg, no
// announce) renders cleanly without empty placeholders.
func buildSourceErrorBody(parent fyne.Window, meta *state.SubscriptionMeta) fyne.CanvasObject {
	rows := container.NewVBox()

	// 1. HTTP status line — always shown if we have a status code.
	if line := formatHTTPStatusLine(meta); line != "" {
		statusLbl := widget.NewLabel(line)
		statusLbl.TextStyle = fyne.TextStyle{Monospace: true}
		rows.Add(statusLbl)
		rows.Add(widget.NewSeparator())
	}

	// 2. Body: provider announce message OR last error message.
	bodyText := pickBodyText(meta)
	if bodyText != "" {
		msg := widget.NewLabel(bodyText)
		msg.Wrapping = fyne.TextWrapWord
		rows.Add(msg)
	}

	// 3. Actionable URL — clickable button if scheme-safe; plain text otherwise.
	if u := pickURL(meta); u != "" {
		rows.Add(buildURLAffordance(parent, u))
	}

	// 4. Footer meta: last attempt + error streak (if any).
	if footer := formatFooterMeta(meta); footer != "" {
		rows.Add(widget.NewSeparator())
		footerLbl := widget.NewLabel(footer)
		footerLbl.TextStyle = fyne.TextStyle{Italic: true}
		footerLbl.Importance = widget.LowImportance
		rows.Add(footerLbl)
	}

	scroll := container.NewVScroll(rows)
	scroll.SetMinSize(fyne.NewSize(480, 280))
	return scroll
}

// formatHTTPStatusLine returns "HTTP 200 · empty body" / "HTTP 403 ·
// forbidden" / etc. Empty when neither HTTPStatusCode nor a meaningful
// hint can be assembled.
func formatHTTPStatusLine(meta *state.SubscriptionMeta) string {
	if meta.HTTPStatusCode == 0 {
		return ""
	}
	hint := httpStatusHint(meta.HTTPStatusCode)
	switch {
	case meta.HTTPStatusCode == 200 && meta.RawBodyBytes == 0:
		return "HTTP 200 · empty body"
	case hint != "":
		return fmt.Sprintf("HTTP %d · %s", meta.HTTPStatusCode, hint)
	default:
		return fmt.Sprintf("HTTP %d", meta.HTTPStatusCode)
	}
}

// httpStatusHint mirrors subscription.explainHTTPStatus tersely. Kept
// in-file to avoid a UI → subscription dep just for the strings.
func httpStatusHint(code int) string {
	switch code {
	case 200:
		return "ok"
	case 401:
		return "unauthorized — token may have expired"
	case 403:
		return "forbidden — provider blocked this request"
	case 404:
		return "not found"
	case 410:
		return "gone"
	case 429:
		return "rate limited"
	}
	switch {
	case code >= 500:
		return "server error"
	case code >= 400:
		return "client error"
	}
	return ""
}

// pickBodyText returns the most user-actionable message body, in order:
// provider's decoded announce → LastErrorMsg → generic "no details".
func pickBodyText(meta *state.SubscriptionMeta) string {
	if meta.ProviderAnnounce != nil && meta.ProviderAnnounce.Message != "" {
		return meta.ProviderAnnounce.Message
	}
	if meta.LastErrorMsg != "" {
		return meta.LastErrorMsg
	}
	// Even without text, the HWID flag is itself informative.
	if meta.ProviderAnnounce != nil && (meta.ProviderAnnounce.HWIDLimit || meta.ProviderAnnounce.HWIDMaxDevicesReached) {
		return "Provider reports the device limit has been reached. Open the support link below to free a slot."
	}
	return ""
}

// pickURL returns the announce URL (preferred) or the meta convenience
// snapshot, whichever is set.
func pickURL(meta *state.SubscriptionMeta) string {
	if meta.ProviderAnnounce != nil && meta.ProviderAnnounce.URL != "" {
		return meta.ProviderAnnounce.URL
	}
	return meta.LastErrorURL
}

// buildURLAffordance returns either an OpenURL button (safe scheme) or
// plain-text label showing the raw URL.
func buildURLAffordance(parent fyne.Window, raw string) fyne.CanvasObject {
	if !urlsafe.IsSafeAnnounceURL(raw) {
		// Not actionable — show the URL so the user knows what was sent
		// but don't add a click handler that could trigger file://,
		// javascript:, etc.
		lbl := widget.NewLabel("Support URL: " + raw + "  (unsafe scheme — open manually)")
		lbl.Wrapping = fyne.TextWrapWord
		lbl.Importance = widget.LowImportance
		return lbl
	}
	host := hostnameForButton(raw)
	btn := widget.NewButton("🔗 Open "+host, func() {
		if err := platform.OpenURL(raw); err != nil {
			debuglog.WarnLog("source_error_dialog: OpenURL(%s): %v", raw, err)
		}
	})
	btn.Importance = widget.HighImportance
	// HBox with spacer-left so button is left-aligned, not stretched.
	return container.NewHBox(btn, layout.NewSpacer())
}

// hostnameForButton — short label inside the button. For `tg://resolve?domain=foo_bot`
// returns `@foo_bot` (telegram convention); for http(s) URLs returns the host.
func hostnameForButton(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if strings.EqualFold(u.Scheme, "tg") {
		if u.Host == "resolve" {
			if dom := u.Query().Get("domain"); dom != "" {
				return "@" + dom
			}
		}
		return raw
	}
	host := u.Host
	if host == "" {
		return raw
	}
	return host
}

// formatFooterMeta returns "Last attempt: <ts>" + error-streak line.
// Either may be missing; returns "" only when both are.
func formatFooterMeta(meta *state.SubscriptionMeta) string {
	var lines []string
	if meta.LastFetchedAt != "" {
		lines = append(lines, "Last attempt: "+meta.LastFetchedAt)
	}
	if meta.ErrorCount > 1 {
		lines = append(lines, fmt.Sprintf("Errors in a row: %d", meta.ErrorCount))
	}
	return strings.Join(lines, "\n")
}
