// Package urlsafe — scheme allowlist helpers for URLs we render as
// clickable affordances (Announce-Url buttons, support links).
//
// Background: a hostile subscription provider can send
// `javascript:fetch(...)`, `file:///etc/passwd`, `data:text/html,<script>`
// or other crafted URLs as the `Announce-Url` response header. We let
// fyne.OpenURL invoke the OS browser/Telegram opener, which in turn
// forwards the URL to whatever handler is registered — including `file:`
// (Finder) and `data:` (browser). The launcher must vet schemes itself.
package urlsafe

import (
	"net/url"
	"strings"
)

// IsSafeAnnounceURL — true if u parses and uses one of the schemes we render
// as a clickable button:
//
//   - http / https — standard web links (billing page, status page)
//   - tg — Telegram deep links (`tg://resolve?domain=foo_bot`); used by
//     NashVPN-class providers to drive users to support bots
//
// Everything else (javascript, file, data, mailto, ftp, …) is hidden so the
// UI shows the URL as plain text — no `fyne.OpenURL` invocation.
//
// Empty / unparseable input → false (caller fall-through to plain text).
func IsSafeAnnounceURL(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "tg":
		// Reject scheme-only without host (e.g. `tg://`) — nothing actionable.
		if u.Scheme == "tg" {
			return u.Host != "" || u.Opaque != ""
		}
		return u.Host != ""
	}
	return false
}
