//go:build windows
// +build windows

package platform

import (
	"strings"

	"golang.org/x/sys/windows/registry"

	"singbox-launcher/internal/debuglog"
)

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

	// IMPORTANT — KEY_WOW64_64KEY on every OpenKey under HKLM\SOFTWARE.
	//
	// On 64-bit Windows, a 32-bit process accessing HKLM\SOFTWARE\... is
	// silently redirected to HKLM\SOFTWARE\Wow6432Node\... by Wow64
	// Registry Redirection. The "\NetworkList" subtree is NOT in
	// Microsoft's list of Shared Keys, so it gets redirected. Real NLA
	// profiles live in the 64-bit view; the Wow6432Node view is empty.
	//
	// Discovered when a Win7-64 user ran our Win7-32 build: cleanup
	// logged "enumerated 0 profile subkeys" despite 20 profiles in the
	// registry. Adding KEY_WOW64_64KEY (= 0x100) forces every OpenKey
	// to target the 64-bit view. The flag is a no-op on native 32-bit
	// Windows (Win7-32), so the same binary works on both.
	const wowFlag = registry.WOW64_64KEY

	// Phase A.1: read-only enumerate + match. Open root with READ only.
	// Mixing WRITE on the parent and QUERY_VALUE on children has been
	// observed to fail silently on Win7 — children get inherited handle
	// permissions and QUERY_VALUE may be refused. Separate read vs.
	// delete phases: read with KEY_READ (full subkey enumerate + value
	// query), delete with a freshly-opened KEY_WRITE handle.
	const profilesPath = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\NetworkList\Profiles`
	profReadKey, err := registry.OpenKey(registry.LOCAL_MACHINE, profilesPath, registry.READ|wowFlag)
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
		sub, err := registry.OpenKey(profReadKey, name, registry.QUERY_VALUE|wowFlag)
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
	profWriteKey, err := registry.OpenKey(registry.LOCAL_MACHINE, profilesPath, registry.WRITE|wowFlag)
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
	sigReadKey, err := registry.OpenKey(registry.LOCAL_MACHINE, sigPath, registry.READ|wowFlag)
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
		sub, err := registry.OpenKey(sigReadKey, name, registry.QUERY_VALUE|wowFlag)
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

	sigWriteKey, err := registry.OpenKey(registry.LOCAL_MACHINE, sigPath, registry.WRITE|wowFlag)
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
