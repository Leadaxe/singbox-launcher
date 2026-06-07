// Package platform — shared SPEC 065 cleanup API (all platforms).
package platform

import "strings"

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

// WinTun ghost-adapter decision constants. Defined here (not in the
// windows-tagged file) so ghostTunDecision + its test compile on all platforms
// (audit TG4).
const (
	dnStarted     = 0x00000008 // DN_STARTED — driver loaded & started
	cmProbPhantom = 24         // CM_PROB_PHANTOM — phantom (was removed)

	adapterNamePrefix = "singbox-tun" // NetConnectionID prefix we own
	wintunServiceName = "Wintun"      // driver service name set by WinTun
)

// ghostTunDecision is the pure skip/remove decision for one enumerated Net
// adapter, extracted from CleanupGhostSingboxTunAdapters (windows) so it is
// unit-testable off-Windows (audit TG4). remove=true → DIF_REMOVE the adapter;
// remove=false + reason → caller logs the skip.
//
//	aggressive: phantom-only (false) vs Stop/taskkill aggressive (true)
//	name:       NetConnectionID
//	service:    driver service name (SPDRP_SERVICE)
//	statusOK:   getDevNodeStatus succeeded
//	status:     CM DevNode status bits (DN_STARTED, …)
//	problem:    CM problem code (CM_PROB_PHANTOM, …)
func ghostTunDecision(aggressive bool, name, service string, statusOK bool, status, problem uint32) (remove bool, reason string) {
	// Filter 1: name prefix — phantom-only mode only. Aggressive drops it
	// because Win7 doesn't preserve the WintunCreateAdapter name into
	// NetConnectionID; the service guard below keeps aggressive safe.
	if !aggressive && !strings.HasPrefix(name, adapterNamePrefix) {
		return false, "name-prefix-mismatch"
	}
	// Filter 2: must be a WinTun adapter — hard guard in BOTH modes, else
	// aggressive cleanup could touch WireGuard / other network adapters.
	if !strings.EqualFold(service, wintunServiceName) {
		return false, "service-mismatch"
	}
	// Filter 3: status must be readable and the adapter not active — never
	// remove a running tunnel (might belong to another WinTun client).
	if !statusOK {
		return false, "status-readback-failed"
	}
	if status&dnStarted != 0 {
		return false, "active-DN_STARTED"
	}
	// Filter 4: phantom-only mode additionally requires CM_PROB_PHANTOM.
	if !aggressive && problem != cmProbPhantom {
		return false, "not-phantom"
	}
	return true, ""
}
