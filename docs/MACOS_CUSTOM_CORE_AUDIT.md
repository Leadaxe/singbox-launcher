# macOS custom-core reliability audit

Audit of the reported macOS issues against the selected baseline. Every row states
what was **verified by reading code or live system state**, and distinguishes that
from what remains **unconfirmed**.

- Upstream: `https://github.com/Leadaxe/singbox-launcher`
- Baseline commit: `58f3eb47ed015c984b7d8fff3fd97f5494a793c8`
- Baseline == tag `v2.3.2` == current `upstream/main` (0 ahead / 0 behind)
- Fork: `https://github.com/Piggy-Cat-bit-shadow/singbox-launcher`
- Branch: `fix/macos-custom-core-reliability`

Because `v2.3.2` is the current tip of `main`, **none** of the reported issues have
been superseded upstream: there is no already-fixed case to skip.

## 0. Historical reproduction state vs current disk state

Two reported conditions were re-checked against live disk state. They no longer hold
**on disk today**, but that does not retire them: they describe states reached during
earlier reproduction, and they were resolved by manual intervention rather than by a
client-side fix. The client still needs a reliable display, replacement and sync flow
for the core — this section narrows *where* the work is, and does not cancel it.

| Report | Historical reproduction state | Current disk state (audited today) |
|---|---|---|
| 5.3 | the three core copies held **different versions**, so the UI showed one core while another ran | **all three byte-identical** — sha256 `501c9b5e98517dcda12ca4d6f291eb25ae1d98bfc6a58c16d29a2ff1595cb474`, all `1.15.0-jiejie-masquerade.5` |
| 5.4 | official `1.14.2-lx.4` was the running core and `check` failed with `unknown field "version"` | the launcher-visible core **is** the custom `1.15.0-jiejie-masquerade.5`; the official build is not on the run path |

Paths checked (all size `75783858`):

```text
/Applications/singbox-launcher.app/Contents/MacOS/bin/sing-box   501c9b5e…  (jie:staff)
~/Library/Application Support/singbox-launcher/bin/sing-box      501c9b5e…  (jie:staff)
/Library/PrivilegedHelperTools/sing-box-lxd                      501c9b5e…  (root:wheel)
```

How to read this:

- today's agreement is the **result of manual repair**, not of the client enforcing it.
  Nothing in the client guarantees it, so the divergence can recur on the next core
  update, reinstall, or partial copy;
- the divergences are still fully explained by the code audited below — `C1`, `C2` and
  `C4` are live defects that reproduce the *cause* of 5.3/5.4 whenever the copies drift
  apart again;
- therefore 5.3 and 5.4 remain in scope as **reliability** work (path display,
  replacement, root-copy sync), while the *currently observable* user-visible failures
  are `C1`/`C2` refusing the custom core and steering the user toward a reinstall.

The custom core's reported identity is confirmed live and matches §4 of the task:

```text
sing-box version 1.15.0-jiejie-masquerade.5
Environment: go1.25.5 darwin/arm64
Tags: with_gvisor,with_quic,with_utls,with_clash_api,with_naive_outbound,jiejie_client_macos,badlinkname,tfogo_checklinkname0
CGO: enabled
```

`with_naive_outbound` is present, confirming a `lite` core must not be substituted.

## 1. Issue → code location → present? → minimal fix → verification

### 5.1 macOS privileged shell loses elevation

| | |
|---|---|
| Location | `internal/platform/privileged_darwin.go:270-277` (`PrivilegedStartArgs`), body `:182-207` |
| Present? | **Applies to this exact construction.** The original report targets `/usr/bin/env -i PATH=… /bin/sh -c <body> <args>` under AEWP — which *is* what this baseline runs. |
| Minimal fix | Pass the shell `-pc` instead of `-c` so the effective root UID is retained by the shell and its children. |
| Verification | AEWP probe comparing `sh -c` vs `sh -pc`, recording real and effective UID for the shell and a child; `TestPrivilegedStartCommand` must stay green. |

**Correction to an earlier draft of this audit.** An earlier version of this document
claimed the reported `-pc` fix was "obsolete for this revision" because the baseline no
longer passes `-c` to a script and instead uses `env -i … /bin/sh -c`. That reasoning
was wrong and is withdrawn:

- the original report targets **precisely** the `env -i PATH=… /bin/sh -c <body>`
  construction, not some older script-based variant;
- adding `env -i` does not by itself remove the shell's privilege drop. `env -i` only
  controls the *environment*; it says nothing about the credentials the shell runs
  with. Conflating "clean environment" with "retains elevation" is the error.

Historical evidence recorded before this audit, to be re-verified independently rather
than assumed:

- with the launcher's args changed from `-c` to `-pc` on this baseline, the integration
  test for the start command passed and the project built;
- `inbound/tun: started at utun7` appeared in the log;
- the process at `/Library/PrivilegedHelperTools/sing-box-lxd` was observed with **UID 0**.

Mechanism to test: the earlier symptom chain was that the privileged command could not
create `/Library/Logs/sing-box-lxd`, could not open `classic.log` there, and once the
log path was created manually the core failed to create the TUN with
`Connect: operation not permitted`. That is the signature of a shell whose **effective**
UID is not 0 while its real UID is — which `-p` (privileged mode) is designed to
preserve. This audit therefore treats 5.1 as an **open, well-supported** defect and does
not consider the fix obsolete.

Constraints on the fix (§7 Phase B): keep the ownership chain, SHA256 and the fixed
root-owned shell body; keep the clean environment and absolute system tool paths; keep
the log directory root-owned and the log file non-world-readable; do not run the whole
GUI as root, `chmod 777`, drop validation, or widen `sudoers`; do not turn a
user-writable script into a long-lived root entry point; and add any explicit UID check
only on the real privileged-start path so non-root and normal modes are unaffected.

#### Mechanism (verified against the local shell)

The fix is justified by the shell's own documented behaviour. `/bin/sh` on this machine
is `GNU bash, version 3.2.57(1)-release (arm64-apple-darwin26)`, and its builtin help
states for `-p`:

> `-p` — Turned on whenever the real and effective user ids do not match. Disables
> processing of the `$ENV` file and importing of shell functions. **Turning this option
> off causes the effective uid and gid to be set to the real uid and gid.**

This is the exact failure mode reported:

- AEWP makes the tool's **real** UID the invoking user (`501`) while the **effective**
  UID is `0`;
- with `-c` (privileged mode off) bash detects `ruid != euid` and deliberately sets the
  effective uid **back** to the real uid — dropping root for the shell and everything
  it spawns;
- with `-pc` (privileged mode on) that reset is suppressed, so the shell and its children
  keep `euid 0`.

This also explains why `env -i` is irrelevant to the problem: it sanitises the
environment, not the credentials, and the privilege drop is performed by the *shell*,
downstream of `env`.

Verified locally without root:

```text
$ /bin/sh --version
GNU bash, version 3.2.57(1)-release (arm64-apple-darwin26)

$ /bin/sh -pc 'echo ok'
ok            # -pc is accepted by this exact shell
```

Passing `-pc` is therefore compatible with the interpreter actually used, and the
existing `sh -n -c <body>` syntax check (`internal/platform/privileged_darwin_test.go:28`)
remains valid.

#### Probe status: blocked by the execution environment

An AEWP probe was built to confirm the real/effective UID split directly
(`.probe/aewp_uid_probe.c`, identity-only: it creates no TUN, modifies no route, stops
no proxy). Running it failed before execution:

```text
caller: real=501 eff=501
A: sh -c  (current launcher args): AuthorizationCreate failed -60008
B: sh -pc (proposed fix):           AuthorizationCreate failed -60008
```

`-60008` is `errAuthorizationInteractionNotAllowed`: the authorization prompt cannot be
presented from this harness. `SECURITYSESSIONID` is unset, and
`com.apple.SecurityAgent` is not reachable in the `gui/501` domain, so no dialog can be
raised regardless of user consent. This is an environment limitation, not a probe defect
and not a permission being withheld.

Consequence: the UID split is established here from the shell's documented and verified
semantics rather than by a live privileged run. The end-to-end privileged check (kernel
at UID 0 creating `utun`) remains §9 manual acceptance, to be run from the GUI where the
authorization dialog can be displayed — see the acceptance checklist in the delivery
report. It is not claimed as verified.

## 1a. Test status on this machine

The project's own CI test runner (`build/test_darwin.sh`, the script
`.github/workflows/ci.yml` invokes on `macos-latest`) was run on the working branch and
on the unmodified baseline. Both produce **exactly the same two failures**:

| Test | Cause | Related to this work? |
|---|---|---|
| `TestListProcesses_Smoke` | `fork/exec /bin/ps: operation not permitted` — the build sandbox blocks `/bin/ps` | No. Fails identically at baseline. |
| `TestCorpusBodiesPassSingboxCheck` | The corpus is checked against a sing-box binary; the one installed here is the custom core and is not built with `with_wireguard`, so `endpoints[0]: unknown endpoint type: wireguard` | No. Fails identically at baseline, and is a property of the core's build tags. |

Confirmed for the second failure directly:

```text
$ sing-box version | grep ^Tags:
Tags: with_gvisor,with_quic,with_utls,with_clash_api,with_naive_outbound,jiejie_client_macos,badlinkname,tfogo_checklinkname0
# no with_wireguard
```

No new failures are introduced by the changes: the failure set is identical before and
after. Upstream CI on the pull request sits in `action_required` because GitHub requires
a maintainer of the target repository to approve workflow runs from a fork; that approval
is outside this task's control and is deliberately not worked around.

### 5.2 Custom core rejected by the version-naming gate

| | |
|---|---|
| Location | **Two independent defects.** (a) `ui/core_dashboard_tab_status.go:338-346`; (b) `core/daemon_service_state.go:216-222` (`parseCoreBuild`) → `:193-201` → `:182-187` (`serviceCoreGate`), consumed by `core/classic_privileged.go:129-138`; message `core/daemon_manager.go:31-32,377-385` |
| Present? | **Confirmed — both root causes located exactly.** |
| Minimal fix | (a) replace exact string equality with numeric comparison so a newer custom core is not offered for downgrade; (b) stop gating the **classic TUN copy sync** behind a **daemon-protocol** version check. |
| Verification | Unit tests: non-`-lx` custom version yields a copy command for classic run while lxd/daemon stays gated; a numerically-newer custom version does not present "Reinstall". |

Defect (a) is a separate, independent mechanism from (b) and is the one that
actually renders the "Reinstall v1.14.2-lx.4" button. At
`ui/core_dashboard_tab_status.go:338` the installed version is compared for
**exact string equality** against `constants.RequiredCoreVersion` (`"1.14.2-lx.4"`,
`internal/constants/constants.go:191`):

```go
case installedVersion != required:
    tab.setSingboxState(installedVersion, locale.Tf("Reinstall v%s", required), -1)
```

Any custom-tagged core therefore always shows "Reinstall", *regardless of whether it
is newer*. The invariant this relies on is stated at `internal/constants/constants.go:187-188`:
the fork binary prints its full `X.Y.Z-lx.N` tag, so strict equality holds. A custom
tag breaks that assumption. Version *parsing* is not at fault — `core/core_version.go:45-58`
extracts `1.15.0-jiejie-masquerade.5` correctly; only the equality test fails.
Note the numeric threshold would in fact pass: `minCoreForRootOwnedService = "1.14.1-lx.12"`
(`core/daemon_service_state_darwin.go:49`) and `compareCoreBuilds` orders on base first
(`core/daemon_service_state.go:273-278`), so a hypothetical `1.15.0-lx.N` is accepted.
The rejection is purely the missing literal `-lx.` token.

Mechanism, precisely:

1. `parseCoreBuild` requires a literal `-lx.` substring (`core/daemon_service_state.go:219`, `i := strings.Index(v, "-lx.")`).
2. `1.15.0-jiejie-masquerade.5` contains no `-lx.` → returns `ok=false`.
3. `coreSupportsRootOwnedCopy` → `false` (`:194-197`).
4. `serviceCoreGate` → `*serviceCoreTooOldError` (`:182-187`).
5. `privilegedCopyCommandFor` returns **no command** (`core/classic_privileged.go:131-133`), so `privilegedCoreCopyGate` falls back to a core hint instead of a sync command (`:157-168`).
6. `DaemonServiceCoreHint` emits, verbatim (`core/daemon_manager.go:32`):

   > "The launcher core (1.15.0-jiejie-masquerade.5) is not a numbered sing-box-lx release, so the launcher cannot confirm it installs a root-owned service. Update the core first: Local tab → Download/Reinstall v1.14.2-lx.4, then install or update the service."

This is the reported behaviour: a custom core is refused and the UI steers the user
to overwrite it with official `1.14.2-lx.4` (`internal/constants/constants.go:191`).

Design intent vs defect: the comment at `core/daemon_service_state.go:190-192` shows
`ok=false` is deliberately a **safe default** meaning *"cannot determine capability"*.
The defect is that this unknown verdict is propagated as *"too old, reinstall"* and,
more importantly, that it also blocks **classic TUN**, which requires only that the
protected copy's SHA256 match the launcher core — a property already verified
independently by `checkPrivilegedCoreCopy` (`core/classic_privileged.go:60-120`).
This is exactly the classic/daemon conflation flagged in §5.2 of the task.

### 5.3 Three core paths can disagree

| | |
|---|---|
| Location | `internal/platform/singbox_exec_path.go:45-90` (`ResolveSingboxExecPath`), `:22-33` (`CoreResolution`, `Shadowed`); wrapper `core/services/file_service.go:156-174` |
| Present? | Discovery order confirmed. The reported *version divergence* does not currently exist (§0). |
| Minimal fix | Surface the resolution source and the shadowed copy in the UI instead of log-only. |
| Verification | Discovery tests across env / data / app / PATH, including portable mode. |

Authoritative order (`:38`, implemented `:58-89`):

```text
SINGBOX_LAUNCHER_CORE  →  <Data>/bin/sing-box  →  <App>/bin/sing-box  →  system PATH
```

`<Data>` **always** wins over `<App>` (`:76-82`), documented as intentional because
dev builds are placed in Data by hand. That is the real explanation of the reported
"saving into the app bundle still shows the official version": the app copy is
*shadowed*, not absent. `CoreResolution.Shadowed` already records this (`:29-32`)
but is described as being *"needed only for the log line at start"*, so the UI never
tells the user which copy is actually live. That is the narrow, real defect here.

Portable mode is not a separate discovery step: when `App == Data` the app entry is
blanked (`:51-52`) and the two collapse into one. `ResolveCore` stores the result as
`SingboxPath` / `CoreSource` / `ShadowedCorePath` (`core/services/file_service.go:156-174`),
while `SingboxBundledPath` = `<Data>/bin/sing-box` is the *install* target (`:55,120`).

`SINGBOX_LAUNCHER_CORE` is the only core-path env var (`internal/constants/constants.go:97`);
`SINGBOX_LAUNCHER_DATA_DIR` / `_LOG_DIR` relocate the data root (`:92-93`).

### 5.4 Running core incompatible with config

| | |
|---|---|
| Location | `core/rebuild.go:39` (`check -c`), `core/process_service.go:297` (`run -c`) |
| Present? | Not reproducible now (§0) — the custom core is installed and reads the config. |
| Minimal fix | The config check must use the **resolved** core, and the UI must report the version it actually checked. |
| Verification | Test that a failing candidate check blocks replacement; live acceptance that the displayed version is the one used for `check`. |

The reported `unknown field "version"` failure is a property of the official
`1.14.2-lx.4` core versus this config, so the durable fix is to guarantee that the
core used for `check` is the same one that will run — not to alter the config (§4:
the config must not be rewritten to mask a client bug).

Note how 5.2(a) drives this failure directly: the exact-equality check at
`ui/core_dashboard_tab_status.go:338` presents "Reinstall v1.14.2-lx.4" for the
working custom core, and following that action replaces the core that can read the
config with one that cannot. The two reports are therefore one causal chain, and
fixing the version gate removes the trap.

### 5.5 Port conflict causes a restart loop

| | |
|---|---|
| Location | `core/crash_handler.go:62-83` (`decideCrashAction`), `core/process_service.go:31,525,582` |
| Present? | **Confirmed by code** (the retry loop exists and cannot distinguish this error); the conflict itself is not currently reproducible. |
| Minimal fix | Classify the exit reason; treat `bind: address already in use` as deterministic and stop retrying. |
| Verification | Preflight test with an occupied loopback port; ensure the instance's own listener is not counted as a conflict. |

Live check:

```text
lsof -nP -iTCP:7890 -sTCP:LISTEN   → (none)
lsof -nP -iTCP:9090 -sTCP:LISTEN   → (none)
```

As the task notes, presence of a process is not proof it has taken over the network,
and `HakoMacExtension` would be missed by a `pgrep clash` name check — so conflict
detection must be based on the actual bind/listen state, with the kernel's own bind
error remaining authoritative.

### 5.6 Empty Clash API secret disables the API

| | |
|---|---|
| Location | `api/clash_config.go:50` (`if host == "" \|\| secret == ""`); consumers `core/services/api_service.go:122-131`, `:354-392` |
| Present? | **Confirmed — root cause located exactly.** |
| Minimal fix | Accept an empty secret on a loopback controller; treat authentication as a separate, explicit state. |
| Verification | Unit tests: non-empty secret, empty secret, missing section, invalid address, auth failure. |

Mechanism: the loader coerces two independent facts (controller address, secret)
into one fatal condition. A config that sing-box itself serves correctly —
`external_controller: 127.0.0.1:9090` with no secret, answering `/version` and
`/proxies` with HTTP 200 — is rejected, and `NewAPIService` responds by setting
`Enabled = false` (`core/services/api_service.go:126`).

That single assignment cascades into the reported symptoms:

- the Clash API is shown as disabled although the core serves it;
- selector-group initialisation is skipped because it is guarded by `if apiSvc.Enabled` (`:138`), so the node list is empty — the mechanism behind part of 5.7;
- `wireTransport` turns `Enabled=false` into `"clash_api is disabled"` (`core/services/proxy_transport.go:87-97`), so proxy load/switch abort;
- the tray omits the "Select Proxy" submenu entirely (`core/tray_menu.go:77-85`);
- the UI reports "❌ Clash API Off (Config Error)" (`ui/clash_api_tab.go:537-542`).

There is **no no-auth path anywhere**: all four request builders set the header
unconditionally — `api/clash_proxy.go:88`, `api/clash_switch.go:40`,
`api/clash_delay.go:170`, `api/clash_transport.go:81` — so the launcher can never
issue a tokenless request, and an unauthenticated loopback config is unreachable by
design.

Scope note: the launcher's own template always auto-fills the secret
(`core/template/vars_resolve.go:417-432` `MaybeGenerateSecrets`), so this path is
reached by hand-written or third-party configs — exactly the user's case.

### 5.7 Status display inconsistent

| | |
|---|---|
| Location | Footer: `ui/clash_api_tab.go:578-604` (`onResetAPIState`, line 600), triggered from `core/uiservice/ui_service.go:269-272`; lifecycle: `core/controller.go:173-184` (`RunningState`) |
| Present? | **Confirmed — two-source desync localised.** The empty node list additionally follows from 5.6. |
| Minimal fix | Derive the footer text from the same `RunningState` at render time instead of leaving imperatively-written stale text. |
| Verification | Test that a single state update makes panel and footer agree; Local/Remote isolation test. |

There is one lifecycle source, `AppController.RunningState` (`core/controller.go:173-184`,
read via `IsRunning()` `:652-656`, written via `Set()` `:588-648`). Core Status, the
tray and the tab icon all read it. **The footer does not.**

- Core Status — `ui/core_dashboard_tab_status.go:89-142`, via `GetVPNButtonState()` → `RunningState.IsRunning()` (`core/controller.go:937`).
- Tray / icon — `core/tray_menu.go:56`, `ui/app.go:209` — same source.
- Footer — `panel.listStatusLabel` (`ui/clash_api_tab.go:161`, laid out at the bottom `:1969-1982`), written **imperatively** by `SetText` with no consultation of `RunningState`.

The reset that writes it is driven by `UIService.UpdateUI` (`core/uiservice/ui_service.go:269-272`),
called from `RunningState.Set` on every transition. On a successful start,
`RunningState.Set(true)` fires `UpdateUI`, the guard `!RunningStateIsRunning()` is
already true, so the reset is skipped and the footer keeps its previous text —
"Sing-box is stopped." (`ui/clash_api_tab.go:600`). Core Status is re-rendered to
Running via `UpdateCoreStatusFunc` (`core/controller.go:634-636`); **nothing in the
start-success path rewrites the footer**, so the contradiction persists until an
unrelated event (tab switch, Test, power resume) refreshes it.

It is aggravated because the `UIService` label slots are single-owner and repointed by
`ProxyListPanel.Activate` (`ui/clash_api_tab.go:118-128`) on tab switch, so a reset
can land in the panel that is not the subject of the state change.

Also relevant to the "node list is empty" symptom: `GET /proxies`
(`api/clash_proxy.go:66-205`) is only issued through `AutoLoadProxies`, which
short-circuits when `Enabled` is false — i.e. it is blocked by 5.6, not by a
separate fault. There is no periodic Clash-API liveness probe for the Local core:
`Enabled` is pure config parsing, and `GET /version`
(`api/clash_transport.go:65-102`) fires only on an explicit Test, tab switch or resume.

### 5.8 Automatic retry on deterministic errors

| | |
|---|---|
| Location | `core/crash_handler.go:62-83` (`decideCrashAction`), `core/process_service.go:31` (`restartAttempts = 3`), `:582` (flat 2 s delay), `:593-610` (180 s stability reset) |
| Present? | **Confirmed — the retry loop exists and provably cannot classify the error.** |
| Minimal fix | Add exit-reason classification; retry only transient failures, with bounded backoff and cancellation. |
| Verification | Tests that deterministic failures do not retry and transient ones are bounded. |

`decideCrashAction` inspects only three booleans — `stoppedByUser`,
`restartRequested`, `cleanExit` (`err == nil`) — and never looks at the exit code,
stderr or the core log:

```go
switch {
case stoppedByUser:     return actionStoppedByUser, 0
case restartRequested:  return actionUserRestart, 0
case cleanExit:         return actionClean, 0
default:
    inc := consecutiveCrashAttempts + 1
    if inc > maxAttempts { return actionMaxAttempts, 0 }
    return actionCrashRestart, inc
}
```

Bad config, `permission denied` and `address already in use` are therefore
indistinguishable from a transient crash: each burns one of the 3 attempts, then
shows "Sing-Box failed to restart after 3 attempts" (`core/process_service.go:570-571`).
This is exactly the reported "3 automatic restarts repeating the same error".

The pre-flight `sing-box check` gate (`core/rebuild_corereject.go:322-347`, aborting
before any process is spawned at `core/process_service.go:217-221`) only covers what
`check` reports on a candidate file; a runtime bind or permission failure at exec time
bypasses it and enters this loop. The neighbouring `corereject` classifier matches only
lines naming one of our own outbound/endpoint tags and explicitly pins
`"initialize inbound[0] tun: permission denied"` as not-matching
(`core/corereject/parse_test.go:78-82`), so it does not help here either.

Hot reload is also confirmed **not implemented**: there is no `PUT /configs` request
anywhere, and no code assumes a 204 means "config applied". The only 204-tolerant
check is `api/clash_switch.go:56`, and it is for proxy switching. Config changes always
go through stop/start or a daemon re-apply, so §7 Phase D's caution about `PUT /configs`
being a no-op is satisfied by construction — but any new reload UI must state
"restart required" rather than claim the config was applied.

## 2. Security properties to preserve (do not weaken)

Verified as present at this baseline and kept by Phase B:

- only root-owned files at absolute paths are executed as root — the core copy and
  fixed system tools (`internal/platform/privileged_darwin.go:115-126`);
- `env -i` with a single `PATH` variable, so the user's environment (including
  `BASH_FUNC_*` function injection) cannot reach the root shell (`:120-126`);
- the start body is a compile-time constant, with paths passed as positional
  arguments (`:166-207`);
- the log directory is root-owned and the log file is handed to the launcher user
  `0600`, with symlink and ownership refusal (`:190-201`);
- the copy's ownership chain and SHA256 are verified before a privileged start
  (`core/classic_privileged.go:60-120`, `core/daemon_service_state.go:324-345`).

The existing test `TestPrivilegedStartCommand` already asserts the anti-injection
and refusal behaviour, and includes poisoned-environment checks
(`internal/platform/privileged_darwin_test.go:221-225`). It must remain green.

## 3. Root copy: verification coverage

The root-owned copy `/Library/PrivilegedHelperTools/sing-box-lxd` is **never created
by the launcher** — it only emits a `sudo` command and the core copies itself
(`core/classic_privileged.go:21-22,134-137`). Verification is read-only and needs no
root, in three layers:

1. **Ownership chain** — `checkRootOwnedChain` (`core/daemon_service_state.go:324-345`)
   walks `/Library` → `PrivilegedHelperTools` → file via `filepath.Rel`.
2. **Per-entry check** — `core/daemon_service_state_darwin.go:154-185`: rejects
   symlinks (`:164-165`), wrong type, non-regular file, wrong uid (`:178-180`) and any
   group/other write bit (`mode.Perm()&0o022 != 0`, `:181-183`).
3. **SHA256 equality** with the launcher core — `checkPrivilegedCoreCopy`
   (`core/classic_privileged.go:60-120`), failing closed if a hash cannot be computed.

Gap relevant to §7 Phase C item 11: on macOS `daemonSetMismatch` is an explicit no-op
(`core/daemon_service_state_darwin.go:290-293` — *"on macOS the copy is a single file:
there are no other set members"*), so for the copy there is **no architecture check
and no version check** — identity is enforced solely by SHA256 equality plus the
ownership chain. Version is read from the sidecar for **display only**
(`readDaemonServiceSidecarVersion`, `:455-472`; `CopyVersion` documented as
*"display only"*, `core/daemon_service_state.go:97-98`). Phase C's requirement to
verify "hash, permissions, architecture and version" of the copy therefore has a
genuine, currently-unimplemented architecture component on macOS.

Because identity to the launcher core is byte equality, a custom core is *already*
fully acceptable to this gate — which is what makes the 5.2 version gate the sole
obstacle rather than a genuine capability problem for classic TUN.

Atomic replacement exists only for the launcher's own downloaded core, not the root
copy: `installBinary` (`core/core_downloader.go:703-762`) renames the old binary to
`<dest>.old` and restores it if `io.Copy` fails. It is not a true atomic swap (new
bytes are written in place via `os.Create`) and rollback does not fire on a hard kill
— relevant to §7 Phase C item 5, which asks for atomic update with backup and restore
on failure.

## 4. Still unconfirmed

Recorded as open, not as fixed:

- **5.1** whether AEWP retains the effective UID through `env`→`sh` on this baseline (requires a privileged probe that prompts for authorization);
- whether the root copy's missing **architecture** check (see §3) has any practical impact on this machine — all three copies are currently arm64 and byte-identical;
- the Naive IPv6 UDP and TUN-vs-mixed divergence of §6, which this document does
  **not** treat as a confirmed source defect. No config, DNS, FakeIP, selector or
  rule_set change is proposed, and no `bind_interface=en0` or IPv6-disabling
  workaround is applied.

## 5. Defect chains (summary)

The reported symptoms reduce to a small number of independent root causes:

| Root cause | Location | Symptoms it produces |
|---|---|---|
| C0 shell privilege drop: `sh -c` without `-p` resets euid to ruid under AEWP | `internal/platform/privileged_darwin.go:273` (`PrivilegedStartArgs`) | 5.1; no TUN (`operation not permitted`); cannot create the root log dir |
| C1 missing `-lx.` token → capability "unknown" treated as "too old", gating **classic TUN** behind a **daemon** check | `core/daemon_service_state.go:216-222,193-201,182-187` → `core/classic_privileged.go:131` | 5.2; no copy command offered for classic TUN; dialog degrades to hint + Close |
| C2 exact string equality against pinned `RequiredCoreVersion` | `ui/core_dashboard_tab_status.go:338` | 5.2 "Reinstall v1.14.2-lx.4"; drives 5.4 by replacing a working core |
| C3 empty secret treated as invalid config | `api/clash_config.go:50` → `core/services/api_service.go:126` | 5.6; API disabled; empty node list; no tray proxy menu |
| C4 copy shadowing is log-only, not surfaced in UI | `internal/platform/singbox_exec_path.go:29-32` | 5.3 confusion about which core is live |
| C5 footer text written imperatively, not derived from `RunningState` | `ui/clash_api_tab.go:600` via `core/uiservice/ui_service.go:269-272` | 5.7 Running-vs-stopped contradiction |
| C6 retry decision ignores exit reason | `core/crash_handler.go:62-83` | 5.8 three identical restarts on a deterministic error (including 5.5's port conflict) |

C0–C6 are independent: fixing C1 does not fix C2, C3, C5 or C6, so each needs its own
change and its own test. C3 is the single cause behind most of 5.6 and a direct cause
of part of 5.7, and C6 is the mechanism that turns 5.5's port conflict into a loop.

Note that a single reported symptom can have more than one contributing cause — 5.2 is
caused by **both** C1 (capability gate) and C2 (equality check), and neither fix alone
resolves it. Likewise 5.7's empty node list is a consequence of C3, while its
Running-vs-stopped contradiction is C5.

## 6. Implementation order (confirmed)

Each step is a confirmed defect with a contained, independently testable change:

1. **C3 (5.6)** — API config/auth state model; unblocks the API surface and the node list.
2. **C2 (5.2a)** — numeric version comparison instead of exact equality.
3. **C1 (5.2b)** — separate the classic-TUN copy gate from the daemon capability gate.
4. **C6 (5.8/5.5)** — exit-reason classification feeding the retry decision.
5. **C5 (5.7)** — derive the footer from `RunningState`.
6. **C4 (5.3)** — surface resolution source and shadowed copy in the UI.
7. **C0 (5.1)** — `-pc` in `PrivilegedStartArgs`, with the start-command test updated.

For C3, removing the empty-secret rejection is **not sufficient on its own**. The change
must also cover, together:

- request headers — omit `Authorization` entirely when the secret is empty, rather than
  sending `Bearer ` with an empty token;
- an explicit authentication-failure state, distinct from "not configured" and from
  "connection refused";
- Local/Remote scope isolation, so an API-state change in one scope does not clear or
  pollute the other;
- tests covering non-empty secret, empty secret, missing section and invalid address.

When the launcher auto-configures the API: generate a high-entropy secret from the
system random source and store it locally only; preserve an existing secret rather than
rotating it; bind `127.0.0.1`; and never print the secret or place it in a URL, process
argument, world-readable plist, log, or the repository.

Deferred pending explicit direction (needs privileged access or changes real network
state, and cannot be verified from a non-root test):

- root TUN and system-level network acceptance (§9 of the task);
- §6 Naive IPv6 UDP / TUN-vs-mixed diagnosis, which stays a separate report.
