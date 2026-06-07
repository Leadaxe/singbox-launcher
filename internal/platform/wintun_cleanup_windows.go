//go:build windows
// +build windows

// Package platform — wintun_cleanup_windows.go implements SPEC 065 cleanup
// of phantom (ghost) singbox-tun adapters that accumulate on Windows 7 after
// each VPN start/stop cycle.
//
// Mechanism (see SPEC 065 §Проблема for the full story):
//
//   1. sing-box creates a WinTun TUN adapter on VPN start. Adapter gets a
//      PnP device-instance in HKLM\SYSTEM\CurrentControlSet\Enum\ROOT\NET\*
//      and a NetConnectionID "singbox-tun0" in HKLM\...\Network\.
//   2. On VPN stop sing-box calls WintunCloseAdapter which internally
//      issues SetupDiCallClassInstaller(DIF_REMOVE). On Win8/10/11 the PnP
//      manager removes the registry node. On Win7 it only sets the
//      CM_PROB_PHANTOM flag — node and name stay occupied.
//   3. Next VPN start: name "singbox-tun0" is still taken by the phantom,
//      so WinTun bumps the suffix to "singbox-tun1", "tun2", and so on.
//
// This file exposes CleanupGhostSingboxTunAdapters which iterates Net-class
// devices, filters by NetConnectionID prefix + service + phantom flag, and
// removes only entries that pass all guards. Used as a postscript to VPN
// stop (see process_service.go::Stop).
//
// Safety guarantees (see SPEC 065 §Safety inventory for full table):
//   - Returns immediately on non-Win7 (no-op).
//   - Touches only adapters whose NetConnectionID has prefix "singbox-tun".
//     NetConnectionID is the user-visible connection name set by WinTun
//     via WintunCreateAdapter; it lives in registry under
//     HKLM\SYSTEM\CurrentControlSet\Control\Network\<class>\<instance>\Connection\Name.
//     We deliberately do NOT read SPDRP_FRIENDLYNAME — on Win7 the
//     FriendlyName for a WinTun-class adapter is "Wintun Userspace Tunnel"
//     (the device-class display name), not the connection name. That was
//     the v0.9.9.1 hotfix bug — every candidate was filtered out because
//     "Wintun Userspace Tunnel" doesn't start with "singbox-tun"
//     (scanned=15 removed=0 skipped=15 in the field log).
//   - Touches only adapters whose Service is "Wintun".
//   - Touches only adapters with CM_PROB_PHANTOM problem code AND without
//     the DN_STARTED status flag (phantom-only mode).
//   - Caps iteration at 32 removals to defend against runaway enumeration.
//   - Any SetupAPI error is logged at Warn level and never propagated — the
//     cleanup must never affect VPN stop UX.
package platform

import (
	"fmt"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"singbox-launcher/internal/debuglog"
)

// ───────────────────────────────────────────────────────────────────────────
// SetupAPI / cfgmgr32 / ntdll lazy bindings
// ───────────────────────────────────────────────────────────────────────────

var (
	setupapiDLL = windows.NewLazySystemDLL("setupapi.dll")
	cfgmgr32DLL = windows.NewLazySystemDLL("cfgmgr32.dll")
	ntdllDLL    = windows.NewLazySystemDLL("ntdll.dll")

	procSetupDiGetClassDevsW              = setupapiDLL.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInfo             = setupapiDLL.NewProc("SetupDiEnumDeviceInfo")
	procSetupDiGetDeviceRegistryPropertyW = setupapiDLL.NewProc("SetupDiGetDeviceRegistryPropertyW")
	procSetupDiGetDeviceInstanceIdW       = setupapiDLL.NewProc("SetupDiGetDeviceInstanceIdW")
	procSetupDiCallClassInstaller         = setupapiDLL.NewProc("SetupDiCallClassInstaller")
	procSetupDiDestroyDeviceInfoList      = setupapiDLL.NewProc("SetupDiDestroyDeviceInfoList")

	procCMGetDevNodeStatus = cfgmgr32DLL.NewProc("CM_Get_DevNode_Status")

	procRtlGetVersion = ntdllDLL.NewProc("RtlGetVersion")
)

// ───────────────────────────────────────────────────────────────────────────
// Constants
// ───────────────────────────────────────────────────────────────────────────

const (
	// Net device class GUID — {4D36E972-E325-11CE-BFC1-08002BE10318}.
	// All network adapters live under this class.

	// SetupDiGetDeviceRegistryProperty key codes.
	spdrpService = 0x00000004 // SPDRP_SERVICE — driver service name (e.g. "Wintun")
	// Deprecated: SPDRP_FRIENDLYNAME on a WinTun adapter returns the device
	// class display name ("Wintun Userspace Tunnel"), not the connection
	// name. Reading it broke filter 1 in v0.9.9.1. Kept for potential
	// future diagnostics; do NOT use it as a primary filter.
	spdrpFriendlyName = 0x0000000C // SPDRP_FRIENDLYNAME — class name on Win7

	// Net device class GUID literal used in the registry path that holds
	// per-adapter NetConnectionID. Must match guidDevClassNet below.
	netClassGUIDLiteral = "{4D36E972-E325-11CE-BFC1-08002BE10318}"

	// Defensive cap on the device-instance-ID string length (UTF-16 chars).
	// Real-world IDs are <100 chars (e.g. "ROOT\\NET\\0001"); 1024 is a
	// generous safety bound.
	maxInstanceIDChars = 1024

	// DI_FUNCTION codes for SetupDiCallClassInstaller.
	difRemove = 0x00000005 // DIF_REMOVE — uninstall device

	// PnP device-node status / problem codes.
	dnStarted     = 0x00000008 // DN_STARTED — driver loaded & started
	cmProbPhantom = 24         // CM_PROB_PHANTOM — phantom (was removed)

	// SetupDiGetClassDevsW flags.
	digcfPresent = 0x00000002 // we DO NOT want this — phantoms are not "present"
	// We pass 0 to include phantoms.

	// Cap on per-invocation removals — defensive bound.
	maxRemovalsPerCall = 32

	// FriendlyName prefix we own.
	adapterNamePrefix = "singbox-tun"

	// Service name set by WinTun.
	wintunServiceName = "Wintun"

	// INVALID_HANDLE_VALUE — returned by SetupDiGetClassDevs on failure.
	invalidHandle = ^uintptr(0)
)

// guidDevClassNet — {4D36E972-E325-11CE-BFC1-08002BE10318}.
var guidDevClassNet = windows.GUID{
	Data1: 0x4d36e972,
	Data2: 0xe325,
	Data3: 0x11ce,
	Data4: [8]byte{0xbf, 0xc1, 0x08, 0x00, 0x2b, 0xe1, 0x03, 0x18},
}

// ───────────────────────────────────────────────────────────────────────────
// Structures (Win32 ABI)
// ───────────────────────────────────────────────────────────────────────────

// spDevInfoData — SP_DEVINFO_DATA. Size: 32 bytes on x64.
//
//	DWORD     cbSize         // 4
//	GUID      ClassGuid      // 16
//	DWORD     DevInst        // 4 (+ 4 padding on x64)
//	ULONG_PTR Reserved       // 8 on x64, 4 on x86
type spDevInfoData struct {
	cbSize    uint32
	classGUID windows.GUID
	devInst   uint32
	reserved  uintptr
}

// osVersionInfoW — RTL_OSVERSIONINFOW. Size: 4*5 + 128*2 = 276 bytes.
type osVersionInfoW struct {
	osVersionInfoSize uint32
	majorVersion      uint32
	minorVersion      uint32
	buildNumber       uint32
	platformID        uint32
	csdVersion        [128]uint16
}

// ───────────────────────────────────────────────────────────────────────────
// Version detection (cached after first call)
// ───────────────────────────────────────────────────────────────────────────

var (
	versionOnce sync.Once
	cachedIsWin7 bool
	cachedOSDesc string
)

// isWindows7 — true only when running on Windows 7 / Windows 7 SP1
// (major=6, minor=1). Cached after the first call. RtlGetVersion is the
// kernel's authoritative source; it ignores compatibility-mode shimming
// (unlike GetVersionEx).
func isWindows7() bool {
	versionOnce.Do(func() {
		var info osVersionInfoW
		info.osVersionInfoSize = uint32(unsafe.Sizeof(info))
		// NTSTATUS — 0 means STATUS_SUCCESS.
		ret, _, _ := procRtlGetVersion.Call(uintptr(unsafe.Pointer(&info)))
		if ret != 0 {
			// Call failed — assume not Win7 so we no-op.
			cachedOSDesc = "RtlGetVersion-failed"
			cachedIsWin7 = false
			return
		}
		cachedOSDesc = fmt.Sprintf("%d.%d.%d", info.majorVersion, info.minorVersion, info.buildNumber)
		cachedIsWin7 = info.majorVersion == 6 && info.minorVersion == 1
	})
	return cachedIsWin7
}

// ───────────────────────────────────────────────────────────────────────────
// Low-level helpers
// ───────────────────────────────────────────────────────────────────────────

// getRegistryPropertyW reads a UTF-16 string property from a device info node.
// Returns empty string on any failure (caller treats as "skip").
//
// Two-call pattern: first call probes the required buffer size, second call
// fills it. Required by SetupAPI for variable-length strings.
func getRegistryPropertyW(h uintptr, devInfo *spDevInfoData, prop uint32) string {
	var requiredSize uint32
	// First call — request size (buffer = nil, size = 0).
	procSetupDiGetDeviceRegistryPropertyW.Call(
		h,
		uintptr(unsafe.Pointer(devInfo)),
		uintptr(prop),
		0, // PropertyRegDataType — we don't care
		0, // PropertyBuffer
		0, // PropertyBufferSize
		uintptr(unsafe.Pointer(&requiredSize)),
	)
	if requiredSize == 0 {
		return ""
	}
	// Defensive cap — a friendly name longer than 4 KB is pathological.
	if requiredSize > 4096 {
		return ""
	}

	buf := make([]uint16, requiredSize/2+1)
	ret, _, _ := procSetupDiGetDeviceRegistryPropertyW.Call(
		h,
		uintptr(unsafe.Pointer(devInfo)),
		uintptr(prop),
		0,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(requiredSize),
		uintptr(unsafe.Pointer(&requiredSize)),
	)
	if ret == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// getDeviceInstanceID reads the device-instance-ID string (e.g.
// "ROOT\\NET\\0001") for a given SP_DEVINFO_DATA. Returns empty string on
// any failure. Used as the registry sub-key under
// HKLM\SYSTEM\CurrentControlSet\Control\Network\<class>\<instance>\Connection.
//
// Two-call pattern: probe required size, then fill.
func getDeviceInstanceID(h uintptr, devInfo *spDevInfoData) string {
	var requiredSize uint32
	// First call — request size (buffer = nil, size = 0).
	procSetupDiGetDeviceInstanceIdW.Call(
		h,
		uintptr(unsafe.Pointer(devInfo)),
		0, // DeviceInstanceId
		0, // DeviceInstanceIdSize
		uintptr(unsafe.Pointer(&requiredSize)),
	)
	if requiredSize == 0 || requiredSize > maxInstanceIDChars {
		return ""
	}
	buf := make([]uint16, requiredSize)
	ret, _, _ := procSetupDiGetDeviceInstanceIdW.Call(
		h,
		uintptr(unsafe.Pointer(devInfo)),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(requiredSize),
		uintptr(unsafe.Pointer(&requiredSize)),
	)
	if ret == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// getNetConnectionID reads the user-visible adapter connection name
// (NetConnectionID, e.g. "singbox-tun0") for a Net-class device by looking
// up the registry value:
//
//	HKLM\SYSTEM\CurrentControlSet\Control\Network\{4D36E972-...}\<instance>\Connection\Name
//
// This is what WinTun sets via WintunCreateAdapter — and it's what the
// v0.9.9 filter actually needs to match. SPDRP_FRIENDLYNAME on Win7
// returns the device-class display name ("Wintun Userspace Tunnel") and
// will never match "singbox-tun".
//
// Returns empty string on any failure (missing instance ID, key absent,
// value absent, wrong type). Empty string fails the prefix check
// downstream, so the adapter is silently skipped — the conservative
// behavior matches the rest of this file.
func getNetConnectionID(h uintptr, devInfo *spDevInfoData) string {
	instanceID := getDeviceInstanceID(h, devInfo)
	if instanceID == "" {
		return ""
	}
	subKey := `SYSTEM\CurrentControlSet\Control\Network\` + netClassGUIDLiteral + `\` + instanceID + `\Connection`
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, subKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	name, _, err := k.GetStringValue("Name")
	if err != nil {
		return ""
	}
	return name
}

// getDevNodeStatus reads status + problem-code for a device node.
// Returns ok=false on any failure (caller skips that adapter).
func getDevNodeStatus(devInst uint32) (status uint32, problem uint32, ok bool) {
	ret, _, _ := procCMGetDevNodeStatus.Call(
		uintptr(unsafe.Pointer(&status)),
		uintptr(unsafe.Pointer(&problem)),
		uintptr(devInst),
		0, // ulFlags
	)
	// CR_SUCCESS == 0.
	if ret != 0 {
		return 0, 0, false
	}
	return status, problem, true
}

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
		name := getNetConnectionID(h, &devInfo)
		if !aggressive && !strings.HasPrefix(name, adapterNamePrefix) {
			skipped++
			index++
			continue
		}

		// Filter 2: driver service name. Hard requirement in both modes
		// — without this guard aggressive cleanup would touch WireGuard
		// or any other non-WinTun network adapter, which is unsafe.
		service := getRegistryPropertyW(h, &devInfo, spdrpService)
		if !strings.EqualFold(service, wintunServiceName) {
			debuglog.DebugLog("ghost-tun cleanup: skip name=%q reason=service-mismatch service=%q", name, service)
			skipped++
			index++
			continue
		}

		status, problem, ok := getDevNodeStatus(devInfo.devInst)
		if !aggressive {
			// Phantom-only mode (original SPEC 065).
			if !ok {
				debuglog.DebugLog("ghost-tun cleanup: skip name=%q reason=status-readback-failed", name)
				skipped++
				index++
				continue
			}
			if status&dnStarted != 0 {
				debuglog.DebugLog("ghost-tun cleanup: skip name=%q reason=active(DN_STARTED) problem=%d", name, problem)
				skipped++
				index++
				continue
			}
			if problem != cmProbPhantom {
				debuglog.DebugLog("ghost-tun cleanup: skip name=%q reason=not-phantom(problem=%d)", name, problem)
				skipped++
				index++
				continue
			}
		} else {
			// Aggressive mode (v0.9.9.2): we already enforced service ==
			// "Wintun" above, so the only WinTun adapters that survive
			// to here are this app's leftovers OR a currently-active
			// adapter owned by another WinTun client (WireGuard etc).
			// Skip the active one to avoid stealing a running tunnel.
			// Phantom adapters from killed sing-box runs do NOT have
			// DN_STARTED set, so they pass through to DIF_REMOVE.
			if !ok {
				debuglog.DebugLog("ghost-tun cleanup: skip aggressive reason=status-readback-failed name=%q", name)
				skipped++
				index++
				continue
			}
			if status&dnStarted != 0 {
				debuglog.DebugLog("ghost-tun cleanup: skip aggressive reason=active(DN_STARTED) name=%q", name)
				skipped++
				index++
				continue
			}
			debuglog.DebugLog("ghost-tun cleanup: candidate aggressive name=%q status=0x%x problem=%d", name, status, problem)
		}

		debuglog.WarnLog("ghost-tun cleanup: removing name=%q service=%q", name, service)
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

// cleanupNLAProfiles removes Network Location Awareness profile +
// signature records left behind by destroyed singbox-tun adapters.
//
// Windows creates an NLA profile every time a new network is detected
// and NEVER garbage-collects them, even after the adapter goes away. A
// user with N start/stop cycles of sing-box accumulates N profile keys
// like:
//
//	HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\NetworkList\Profiles\{GUID}
//	  ProfileName = "singbox-tunK", "singbox-tunK  2", ..., "singbox-tunK  N"
//	  Description = "singbox-tunK"  ← ours, identification key (K = 0,1,2,...)
//
// Plus a matching entry per profile in
// `\NetworkList\Signatures\Unmanaged\<hash>` keyed by `ProfileGuid`. The
// "singbox-tun0 N" suffix increments forever because NLA dedups by
// ProfileName against the cached list. Without this cleanup, even after
// we fix device-node accumulation, the Choose Network Location dialog
// keeps showing fresh dedup numbers and the registry growth is monotonic.
//
// Match policy: registry value `Description` has prefix "singbox-tun"
// (matching `adapterNamePrefix`). sing-box names adapters
// singbox-tun0/1/2/... when several run in parallel or shift indices
// across restarts; Description mirrors the launcher-set adapter name
// exactly (NOT the dedup-suffixed `ProfileName`). The prefix is unique
// to us — no other application uses it. Other applications' profiles
// have their own Description string and are skipped.
//
// Returns (profilesRemoved, signaturesRemoved). All errors are logged at
// WarnLog and never propagated — the cleanup must never affect VPN
// behavior.
//
// Cross-version safe: no Win7 gate. NLA-cache accumulation is a
// universal Windows behavior (verified Win7/8/10/11). On Win7 the
// suffix is visible in the Choose Network Location dialog; on Win8+
// it bloats the registry silently. If a future Windows version
// restructures these keys, our strict `Description == "singbox-tun0"`
// match simply won't find anything and we return (0, 0) — fail-safe.
//
// Defensive `defer recover()` wraps the whole function: registry API
// calls today return Go errors and don't panic, but on a future or
// unusual Windows configuration we'd rather log + return zero than
// crash the launcher.
//
// Note: `registry.DeleteKey` deletes the value-only key; NLA profile and
// signature subkeys do not contain nested subkeys in practice (verified
// via the user-supplied .reg export), so a single-step delete is
// sufficient. If a future Windows version adds nested keys, the delete
// would fail with ERROR_KEY_HAS_CHILDREN and we'd log + skip — safe
// degradation.
func cleanupNLAProfiles() (profilesRemoved, signaturesRemoved int) {
	defer func() {
		if r := recover(); r != nil {
			debuglog.WarnLog("nla cleanup: recovered from panic: %v", r)
		}
	}()

	// Phase A: enumerate Profiles, collect GUIDs of matching entries,
	// delete profile keys. We collect first then delete because deleting
	// a key while iterating ReadSubKeyNames is a fast path to undefined
	// behavior; collect-then-delete is the conservative pattern.
	debuglog.WarnLog("nla cleanup: starting")

	// Phase A.1: read-only enumerate + match. Open root with READ only.
	// Mixing WRITE on the parent and QUERY_VALUE on children has been
	// observed to fail silently on Win7 — children get inherited handle
	// permissions and QUERY_VALUE may be refused. Separate read vs.
	// delete phases: read with KEY_READ (full subkey enumerate + value
	// query), delete with a freshly-opened KEY_WRITE handle.
	const profilesPath = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\NetworkList\Profiles`
	profReadKey, err := registry.OpenKey(registry.LOCAL_MACHINE, profilesPath, registry.READ)
	if err != nil {
		debuglog.WarnLog("nla cleanup: open Profiles (read): %v", err)
		return 0, 0
	}

	subNames, err := profReadKey.ReadSubKeyNames(-1)
	if err != nil {
		profReadKey.Close()
		debuglog.WarnLog("nla cleanup: enumerate Profiles: %v", err)
		return 0, 0
	}
	debuglog.WarnLog("nla cleanup: enumerated %d profile subkeys", len(subNames))

	// Matched-GUIDs set for Signatures cross-reference (case-preserving:
	// Windows stores GUIDs with mixed case, ProfileGuid value matches
	// subkey name exactly).
	matchedGUIDs := make(map[string]bool, len(subNames))

	for _, name := range subNames {
		sub, err := registry.OpenKey(profReadKey, name, registry.QUERY_VALUE)
		if err != nil {
			debuglog.DebugLog("nla cleanup: open profile %q: %v", name, err)
			continue
		}
		desc, _, descErr := sub.GetStringValue("Description")
		sub.Close()
		if descErr != nil {
			debuglog.DebugLog("nla cleanup: read Description for %q: %v", name, descErr)
			continue
		}
		if !strings.HasPrefix(desc, adapterNamePrefix) {
			debuglog.DebugLog("nla cleanup: skip profile %q (Description=%q does not match prefix)", name, desc)
			continue
		}
		debuglog.WarnLog("nla cleanup: matched profile %q (Description=%q)", name, desc)
		matchedGUIDs[name] = true
	}
	profReadKey.Close()

	if len(matchedGUIDs) == 0 {
		debuglog.WarnLog("nla cleanup: no matching profiles found in %d subkeys", len(subNames))
		return 0, 0
	}

	// Phase A.2: delete matched profiles. Re-open parent with WRITE.
	profWriteKey, err := registry.OpenKey(registry.LOCAL_MACHINE, profilesPath, registry.WRITE)
	if err != nil {
		debuglog.WarnLog("nla cleanup: open Profiles (write): %v", err)
		return 0, 0
	}
	for name := range matchedGUIDs {
		if delErr := registry.DeleteKey(profWriteKey, name); delErr != nil {
			debuglog.WarnLog("nla cleanup: delete profile %q: %v", name, delErr)
			continue
		}
		profilesRemoved++
	}
	profWriteKey.Close()
	debuglog.WarnLog("nla cleanup: deleted %d profile(s) of %d matched", profilesRemoved, len(matchedGUIDs))

	// Phase B: enumerate Signatures\Unmanaged, delete entries whose
	// ProfileGuid value matches one of the just-deleted profiles.
	const sigPath = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\NetworkList\Signatures\Unmanaged`
	sigReadKey, err := registry.OpenKey(registry.LOCAL_MACHINE, sigPath, registry.READ)
	if err != nil {
		debuglog.WarnLog("nla cleanup: open Signatures (read): %v", err)
		return profilesRemoved, 0
	}

	sigSubs, err := sigReadKey.ReadSubKeyNames(-1)
	if err != nil {
		sigReadKey.Close()
		debuglog.WarnLog("nla cleanup: enumerate Signatures: %v", err)
		return profilesRemoved, 0
	}
	debuglog.WarnLog("nla cleanup: enumerated %d signature subkeys", len(sigSubs))

	matchedSigs := make([]string, 0, len(matchedGUIDs))
	for _, name := range sigSubs {
		sub, err := registry.OpenKey(sigReadKey, name, registry.QUERY_VALUE)
		if err != nil {
			debuglog.DebugLog("nla cleanup: open signature %q: %v", name, err)
			continue
		}
		guid, _, guidErr := sub.GetStringValue("ProfileGuid")
		sub.Close()
		if guidErr != nil {
			debuglog.DebugLog("nla cleanup: read ProfileGuid for %q: %v", name, guidErr)
			continue
		}
		if !matchedGUIDs[guid] {
			continue
		}
		matchedSigs = append(matchedSigs, name)
	}
	sigReadKey.Close()

	if len(matchedSigs) == 0 {
		debuglog.WarnLog("nla cleanup: no matching signatures found")
		return profilesRemoved, 0
	}

	sigWriteKey, err := registry.OpenKey(registry.LOCAL_MACHINE, sigPath, registry.WRITE)
	if err != nil {
		debuglog.WarnLog("nla cleanup: open Signatures (write): %v", err)
		return profilesRemoved, 0
	}
	for _, name := range matchedSigs {
		if delErr := registry.DeleteKey(sigWriteKey, name); delErr != nil {
			debuglog.WarnLog("nla cleanup: delete signature %q: %v", name, delErr)
			continue
		}
		signaturesRemoved++
	}
	sigWriteKey.Close()
	debuglog.WarnLog("nla cleanup: deleted %d signature(s) of %d matched", signaturesRemoved, len(matchedSigs))

	return profilesRemoved, signaturesRemoved
}
