//go:build windows
// +build windows

package platform

// NLA signature cleanup.
//
// SPEC 070 Stage C is a pure file split with zero behavior change: it only
// moves existing declarations between files in the same package. The NLA
// signature cleanup (NetworkList\Signatures\Unmanaged entries whose
// ProfileGuid matches a just-deleted singbox-tun profile) is NOT a separate
// declaration — it is "Phase B" inline inside cleanupNLAProfiles, which
// returns (profilesRemoved, signaturesRemoved) as a single unit and shares
// the matchedGUIDs set built during profile cleanup. Extracting Phase B into
// its own symbol would change logic / signatures, which the split forbids, so
// it stays with the profile logic in wintun_cleanup_windows_nla_profiles.go.
//
// This file is intentionally a placeholder so the signature-cleanup concern
// has a discoverable home; the implementation lives in cleanupNLAProfiles
// (wintun_cleanup_windows_nla_profiles.go), Phase B.
