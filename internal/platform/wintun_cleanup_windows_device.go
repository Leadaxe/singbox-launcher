//go:build windows
// +build windows

package platform

import (
	"fmt"
	"syscall"
	"unsafe"

	"singbox-launcher/internal/debuglog"
)

// ───────────────────────────────────────────────────────────────────────────
// Public API
// ───────────────────────────────────────────────────────────────────────────

// CleanupGhostSingboxTunAdapters scans Net-class devices and removes stale
// singbox-tun WinTun adapters. Returns the number of adapters removed.
//
// mode GhostTunCleanupAggressive (default for callers): sing-box is confirmed
// dead — remove every singbox-tun + Wintun match regardless of phantom /
// DN_STARTED. Needed because Windows Stop uses taskkill: WinTun often never
// sets CM_PROB_PHANTOM on Win7.
//
// mode GhostTunCleanupPhantomOnly: original SPEC 065 filters (phantom + not
// DN_STARTED).
//
// On non-Win7 (Win8/10/11) this is a strict no-op: the bug doesn't exist
// there, and we explicitly avoid touching SetupAPI.
//
// All errors are non-fatal and logged at Warn level. The cleanup must
// never affect VPN stop behavior — return values exist purely for telemetry.
func CleanupGhostSingboxTunAdapters(mode GhostTunCleanupMode) (removed int, err error) {
	aggressive := bool(mode)

	// Phase A: NLA profile / signature cleanup. Runs on ALL Windows
	// versions, not just Win7. NLA-cache accumulation is a global Windows
	// behavior: the Network Location Awareness service caches every
	// network it sees and never garbage-collects them. On Win7 this
	// shows up in the "Choose Network Location" dialog (visible suffix
	// growth); on Win8+ it just bloats the registry silently. Cleaning
	// is safe everywhere — see cleanupNLAProfiles for the match-by-
	// Description-prefix-"singbox-tun" safety story.
	//
	// Order: before device cleanup. The callsite guarantees sing-box
	// is not running, so any "singbox-tun*" profile in the registry
	// is by definition stale.
	pRemoved, sRemoved := cleanupNLAProfiles()
	if pRemoved > 0 || sRemoved > 0 {
		debuglog.WarnLog("ghost-tun cleanup: NLA done profiles=%d signatures=%d", pRemoved, sRemoved)
	}

	// Phase B: device-node cleanup — Win7 only. On Win8+ the WinTun
	// driver sets CM_PROB_PHANTOM on destroyed adapters correctly and
	// Windows itself garbage-collects them; we explicitly avoid touching
	// SetupAPI there.
	if !isWindows7() {
		debuglog.DebugLog("ghost-tun cleanup: device-cleanup skipped (os=%s, not Win7)", cachedOSDesc)
		return 0, nil
	}
	if aggressive {
		debuglog.WarnLog("ghost-tun cleanup: scanning aggressive (os=%s, Win7)", cachedOSDesc)
	} else {
		debuglog.WarnLog("ghost-tun cleanup: scanning phantom-only (os=%s, Win7)", cachedOSDesc)
	}

	// Enumerate Net-class devices including phantoms (flag = 0).
	h, _, callErr := procSetupDiGetClassDevsW.Call(
		uintptr(unsafe.Pointer(&guidDevClassNet)),
		0, // Enumerator
		0, // hwndParent
		0, // Flags — DO NOT pass DIGCF_PRESENT; we WANT phantoms.
	)
	if h == invalidHandle {
		debuglog.WarnLog("ghost-tun cleanup: SetupDiGetClassDevs failed: %v", callErr)
		return 0, fmt.Errorf("SetupDiGetClassDevs: %w", callErr)
	}
	defer procSetupDiDestroyDeviceInfoList.Call(h)

	var (
		index   uint32
		scanned int
		skipped int
	)

	for {
		if removed >= maxRemovalsPerCall {
			debuglog.WarnLog("ghost-tun cleanup: hit cap=%d; stopping enumeration to be safe", maxRemovalsPerCall)
			break
		}

		var devInfo spDevInfoData
		devInfo.cbSize = uint32(unsafe.Sizeof(devInfo))

		ret, _, _ := procSetupDiEnumDeviceInfo.Call(
			h,
			uintptr(index),
			uintptr(unsafe.Pointer(&devInfo)),
		)
		if ret == 0 {
			// ERROR_NO_MORE_ITEMS — clean end of enumeration.
			break
		}
		scanned++

		// Filter 1: NetConnectionID prefix. On Win7 the FriendlyName for a
		// network adapter is the device-class name ("Wintun Userspace
		// Tunnel"), not the connection name. The "singbox-tunN" name
		// WinTun sets via WintunCreateAdapter is the NetConnectionID,
		// stored in registry under
		// HKLM\SYSTEM\CurrentControlSet\Control\Network\<class>\<instance>\Connection\Name.
		// Reading SPDRP_FRIENDLYNAME here was the v0.9.9.1 bug (filter
		// rejected all candidates because "Wintun Userspace Tunnel"
		// doesn't start with "singbox-tun"). Confirmed by Win7 user log:
		// scanned=15 removed=0 skipped=15.
		//
		// v0.9.9.2 follow-up: on a real Win7 user the registry export
		// showed the WinTun adapters' NetConnectionID was the default
		// Russian "Подключение по локальной сети N" — NOT "singbox-tunN"
		// either. Windows apparently does not preserve the name passed
		// to WintunCreateAdapter into NetConnectionID on Win7 (the
		// "singbox-tunN" name visible in the Network Location dialog
		// is a Network Location Awareness profile name derived from
		// elsewhere). Phantom-only mode keeps the prefix check (best-
		// effort idempotent for sources where NetConnectionID matches).
		// Aggressive mode drops the name check entirely and relies on
		// the service-name guard + DN_STARTED gate below.
		// Filter 2 (service == "Wintun") is a hard requirement in both
		// modes — without it aggressive cleanup would touch WireGuard or any
		// other non-WinTun network adapter. Filters 3/4 (status readable, not
		// DN_STARTED, CM_PROB_PHANTOM in phantom-only mode) are folded into the
		// pure ghostTunDecision predicate, unit-tested in wintun_cleanup_test.go.
		name := getNetConnectionID(h, &devInfo)
		service := getRegistryPropertyW(h, &devInfo, spdrpService)
		status, problem, ok := getDevNodeStatus(devInfo.devInst)

		if remove, reason := ghostTunDecision(bool(aggressive), name, service, ok, status, problem); !remove {
			debuglog.DebugLog("ghost-tun cleanup: skip name=%q reason=%s service=%q status=0x%x problem=%d aggressive=%v",
				name, reason, service, status, problem, bool(aggressive))
			skipped++
			index++
			continue
		}

		debuglog.WarnLog("ghost-tun cleanup: removing name=%q service=%q (aggressive=%v)", name, service, bool(aggressive))
		callRet, _, callErr := procSetupDiCallClassInstaller.Call(
			uintptr(difRemove),
			h,
			uintptr(unsafe.Pointer(&devInfo)),
		)
		if callRet == 0 {
			// Removal failed — log and continue with the next adapter.
			// Common cause on Win7 is ERROR_ACCESS_DENIED (not running
			// elevated). We tolerate this silently — there's no point
			// in spamming the log if the launcher isn't elevated and
			// can't fix anything anyway.
			if errno, ok := callErr.(syscall.Errno); ok && errno == syscall.ERROR_ACCESS_DENIED {
				debuglog.WarnLog("ghost-tun cleanup: DIF_REMOVE access-denied name=%q (run launcher as Administrator on Win7?)", name)
			} else {
				debuglog.WarnLog("ghost-tun cleanup: DIF_REMOVE failed name=%q err=%v", name, callErr)
			}
			skipped++
			index++
			continue
		}
		removed++
		// SetupAPI: after DIF_REMOVE the next device may shift into this
		// index — do not increment so we don't skip it.
	}

	debuglog.WarnLog("ghost-tun cleanup: done scanned=%d removed=%d skipped=%d", scanned, removed, skipped)

	return removed, nil
}
