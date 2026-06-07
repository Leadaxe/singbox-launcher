// Package platform — shared SPEC 065 cleanup API (all platforms).
package platform

// GhostTunCleanupAggressive removes every singbox-tun WinTun adapter when sing-box
// is confirmed dead. Use after Stop/taskkill: adapters often keep DN_STARTED and
// never get CM_PROB_PHANTOM, so phantom-only mode would skip them.
//
// GhostTunCleanupPhantomOnly is the original SPEC 065 filter (CM_PROB_PHANTOM,
// not DN_STARTED). Kept for reference; callers should prefer Aggressive.
type GhostTunCleanupMode bool

const (
	GhostTunCleanupPhantomOnly GhostTunCleanupMode = false
	GhostTunCleanupAggressive  GhostTunCleanupMode = true
)
