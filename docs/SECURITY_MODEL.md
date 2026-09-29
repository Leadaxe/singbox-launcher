# Security model

Russian version: [SECURITY_MODEL.ru.md](SECURITY_MODEL.ru.md). How to report a
vulnerability: [SECURITY.md](../SECURITY.md).

This document describes the current state: what the launcher protects, from
whom, and by which rules. The history of each decision lives in the specs
listed in [§12](#12-related-specs).

## 1. What is protected and from whom

**The attacker** is a program that runs under the user's own account with
ordinary rights: a malicious application, a script, a compromised dependency.
It can write every file the user can write and set the environment of the
processes the user starts.

**The boundary** is the step from the user's rights to higher ones: `root` on
macOS and Linux, an administrator or `SYSTEM` on Windows. The launcher needs
such rights for one task, the TUN interface, and for the daemon service.

**The goal**: a program with the user's rights must not gain higher rights
through the launcher. Whatever runs with higher rights must not be a file, a
path or an environment that the user's account can change.

**Outside the model:**

- An attacker who already has `root` or administrator rights.
- An attacker with the user's rights who only wants the user's own data:
  subscriptions, node keys and `config.json` belong to that account and are
  readable by its programs.
- Physical access to the machine.
- Traffic analysis and blocking of the protocols themselves: that is the core's
  field, see [sing-box-lx](https://github.com/Leadaxe/sing-box-lx).

## 2. Where higher rights are used

| Platform and mode | Who runs the core | What is executed |
|---|---|---|
| Any platform, proxy mode without TUN | the user | the launcher's core from the data folder |
| macOS, classic with TUN | `root`, through the system password prompt | the protected copy `/Library/PrivilegedHelperTools/sing-box-lxd` |
| macOS, daemon service | `root`, through launchd | the same protected copy |
| Windows x64/arm64, classic with TUN | an administrator, through UAC | the protected copy `<ProgramFiles>\sing-box-lxd\sing-box-lxd.exe` |
| Windows x64/arm64, daemon service | `LocalSystem` | the same protected copy |
| Windows 7 (`win7-32`) | an administrator, always | the launcher's core from the data folder; there is no protected copy in this build |
| Linux with TUN | the user | the launcher's core with file capabilities set by `setcap` |

The launcher's core lives in the data folder and belongs to the user. The
protected copy is made by the core itself (`lxd --service=copy` or
`lxd --service=install`), in a place only `root` or an administrator can write.

## 3. Rules for privileged code

These rules hold on every platform. A change that breaks one of them needs the
owner's decision recorded in a spec.

1. **Higher rights execute only protected files by absolute paths**: the
   protected copy of the core and system tools. Nothing from the data folder,
   the application bundle or `PATH`.
2. **The environment is not inherited.** The privileged process gets a fixed
   environment, not the launcher's one.
3. **No shell text is built from data.** Commands are passed as argv. Where a
   shell is needed, its body is a constant inside the launcher binary and paths
   come as positional arguments.
4. **The copy is checked before every privileged start**: the ownership chain
   of the path and the sha256 of the copy against the launcher's core. If the
   hash cannot be computed, there is no start.
5. **A privileged process does not write to the user's paths.** Its output goes
   to a protected folder; the launcher only reads it.
6. **Symbolic links are not followed.** A link, a wrong file type or a foreign
   owner in place of an expected file or folder is a refusal.
7. **The privileged process checks its own rights** before it touches anything
   and refuses with a message that names them.
8. **A refusal names its cause**: the link of the chain, the system error, the
   command that fixes it.
9. **Commands with `sudo` are shown, not run.** The user runs them in their own
   terminal and enters the password there.

### Rules for a change in privileged code

- A change of what runs with higher rights (argv, the shell body, the
  environment, the list of tools) is accepted after a live start with real
  higher rights on the target platform. Tests without such rights do not show
  a loss of rights: the effective and the real uid are equal there
  ([#138](https://github.com/Leadaxe/singbox-launcher/issues/138), SPEC 151).
- The implementation report lists what was started live and what was not.
- A new place where the launcher builds or runs a command is added to the
  audit table in SPEC 137 §12.

## 4. macOS

### 4.1 Protected copy

| What | Where | Owner and mode |
|---|---|---|
| Copy of the core | `/Library/PrivilegedHelperTools/sing-box-lxd` | `root:wheel 0755` |
| Install record | `/Library/PrivilegedHelperTools/sing-box-lxd.install.json` | `root:wheel 0644` |
| Service plist | `/Library/LaunchDaemons/com.leadaxe.sing-box-lxd.plist` | written by the core's install command |
| Log of the classic core under `root` | `/Library/Logs/sing-box-lxd/classic.log` | folder `root:wheel 0755`, file owned by the user, `0600` |

**Ownership chain**: `/Library` → `/Library/PrivilegedHelperTools` → the copy.
Each link is read with `Lstat` and must be: not a symbolic link, of the
expected type, owned by uid 0, not writable by group or others.

### 4.2 Classic start with TUN

The launcher asks for the password through the system prompt, once per
session, and runs:

```text
/usr/bin/env -i PATH=/usr/bin:/bin:/usr/sbin:/sbin /bin/bash -p -c '<constant body>'
  start-singbox-privileged <data>/bin <copy> config.json
  /Library/Logs/sing-box-lxd 0 <user uid> 2097152 0
```

- `env -i` cuts off the launcher's environment, including `PATH` and bash
  functions exported through `BASH_FUNC_<name>%%`.
- `-p` keeps the effective uid. The system prompt starts the tool with
  effective uid 0 and the user's real uid; bash without `-p` would drop the
  rights. It also disables the import of functions from the environment.
- The body first checks that it runs as `root`, then validates the user's uid
  (digits, 501 or higher, an existing account), prepares the log and starts the
  copy.

Stop and Kill run `/bin/kill` and `/usr/bin/pkill` directly, without a shell.
Turning TUN off needs no higher rights: the launcher removes leftovers with
its own uid.

The gate before the prompt gives one of the verdicts: no core, missing,
unsafe, outdated, ok. A refused gate shows a dialog with the command
(`lxd --service=copy`, or `--service=install` when the service is installed)
and does not ask for the password.

### 4.3 Daemon service

launchd runs the protected copy as `root`. The launcher checks the service
without `sudo` and shows its state: not installed, unsafe, outdated, not
running, process outdated, ok. An unsafe service is a loud warning and not a
block: the VPN keeps working until the user runs the shown command.

## 5. Windows

### 5.1 Rights

The Windows x64/arm64 build starts with the user's rights (`asInvoker`).
Higher rights are requested through UAC for TUN and for service commands. The
Windows 7 build always runs as an administrator.

### 5.2 Protected paths

| What | Where |
|---|---|
| Copy of the core | `<ProgramFiles>\sing-box-lxd\sing-box-lxd.exe` with `libcronet.dll` and the install record |
| Service data | `<ProgramData>\sing-box-lxd\state\` |
| Logs | `<ProgramData>\sing-box-lxd\logs\` (`lxd.log`, `classic.log`) |

**Protection check of a path:**

- The owner is `SYSTEM` or `Administrators`; for parent folders
  `TrustedInstaller` is allowed too.
- No other account has a right to write, delete or change the access list.
- A reparse point and an empty access list are a refusal.
- For the copy, every parent folder from the volume root is checked, and the
  volume must be a fixed NTFS disk. For the log, only
  `<ProgramData>\sing-box-lxd` and `logs\` are checked.

The copy is also compared with the launcher's core by sha256, together with
`libcronet.dll`.

### 5.3 Warning in place of a refusal

On Windows a failed protection check does not leave the user without a start.
The dialog shows the cause, the risk, the `icacls` command and the button
**Run anyway**. With it, until the launcher is closed:

- a failed copy check starts the launcher's core from the data folder with
  administrator rights;
- a failed log check sends the core's output to the launcher's own log.

The choice is kept in memory only. See accepted risks in [§10](#10-accepted-risks).

## 6. Linux

The launcher runs nothing as `root`. The core runs as the user with the file
capabilities `cap_net_admin` and `cap_net_bind_service`. The launcher checks
them with `getcap` and, when they are missing, shows the `sudo setcap` command
for the user to run. Writing to the file removes its capabilities, so replacing
the core does not give higher rights.

## 7. Daemon channel and remote machines

| Subject | State |
|---|---|
| Local address | loopback, the port is chosen by the install command (19091 and up) |
| Server identity | the client pins the SHA-256 of the server certificate |
| Client identity | its own ECDSA P-256 key and certificate; the key file is `0600`, its folder `0700` |
| Pairing | a one-time invite `address#fingerprint#code`; the code is spent by the first enrolment |
| Rights of a paired client | full control of the daemon: config, start and stop, resources |
| Revocation | on the machine itself: `sing-box lxd client remove`. Removing a machine in the launcher deletes the local keys and does not revoke the access |

A channel without TLS exists for a daemon on loopback; the launcher does not
downgrade a network channel to it on its own.

Details: [DAEMON_AND_REMOTE.md](DAEMON_AND_REMOTE.md).

## 8. Debug API

- Off by default; it starts only when enabled in the settings and a token is set.
- Listens on `127.0.0.1` only. There is no TLS and no LAN access; for remote
  access use an SSH tunnel.
- Every request except `GET /ping` needs `Authorization: Bearer <token>`. The
  token is 32 random bytes and is compared in constant time.
- The token gives full control of the launcher: state, config, start and stop,
  paired machines. Treat it as a password.

Details: [API.md](API.md).

## 9. Untrusted input and downloads

### 9.1 Subscriptions

A subscription is untrusted input.

- The response is limited to 10 MB and, by default, 3000 nodes.
- Node fields pass through the registry of the contract: keys outside the
  scheme are removed, `tag` and `type` are set by the build.
- A node whose `detour` would break the config is dropped.
- A tag becomes a folder name only after it is reduced to `[A-Za-z0-9._-]`.
- A response without a single valid node does not replace the nodes already
  stored.

### 9.2 Downloads

| What | Source | Check |
|---|---|---|
| Core | GitHub release of `Leadaxe/sing-box-lx`, the version is pinned in the launcher; on failure, the mirrors `ghfast.top` and `gh-proxy.com` | HTTPS, status, not an HTML page, size up to 100 MB; file names from the archive are reduced to the base name. No checksum and no signature |
| `wizard_template.json` | `raw.githubusercontent.com`, in a release build a pinned commit | HTTPS, parsing and validation of the template |
| Rule sets (SRS) | the URL from the template or the user's rules | HTTPS status only |
| Locales | `raw.githubusercontent.com`, a branch | HTTPS, valid JSON |
| `wintun.dll` | `www.wintun.net` | HTTPS |

The launcher does not update itself. It checks the latest version and opens
the release page in the browser. Every release carries `checksums.txt`.

### 9.3 What the launcher sends

- To a subscription server: `User-Agent` and, unless turned off, `X-Hwid`,
  `X-Device-OS`, `X-Ver-OS`, `X-Device-Model`. The HWID is a random UUID, not
  derived from the hardware. Sending is turned off in the settings, for all
  subscriptions or for one.
- There is no telemetry and no analytics.

### 9.4 Files of the user

| File | Mode |
|---|---|
| mTLS keys of the daemon client | `0600`, folder `0700` |
| LX Backup file | `0600` |
| `settings.json` (holds the Debug API token), `state.json`, `config.json`, the registry of remote machines | `0644` |

## 10. Accepted risks

Each item is a recorded decision, not an oversight.

| Risk | Where recorded |
|---|---|
| The core under higher rights reads the user's `config.json` and writes to the paths named there (cache, `log.output`) | SPEC 137 §8 |
| The `copy` and `install` commands run the launcher's core, a file of the user, under `sudo` or UAC. The user enters the password in their own terminal; the dialog shows both hashes | SPEC 137 §8, SPEC 136 |
| **Run anyway** on Windows runs a file, or writes to a folder, that a program of the user can replace | SPEC 150 §4 |
| The downloaded core is not verified by a checksum | SPEC 072 |
| The Debug API has no TLS | [API.md](API.md) |
| Removing a machine in the launcher does not revoke its access | [DAEMON_AND_REMOTE.md](DAEMON_AND_REMOTE.md) |
| The log of the core under `root` gets the core's stderr, including fragments of config errors. The file is `0600` and owned by the user | SPEC 137 §8 |

## 11. Known cases

| Case | What happened | Fix |
|---|---|---|
| [#137](https://github.com/Leadaxe/singbox-launcher/issues/137) | On Windows the log folder check refused the start because of an entry for Everyone on `C:\ProgramData` | SPEC 150: parents of the log folder are not checked; a warning with **Run anyway** |
| [#138](https://github.com/Leadaxe/singbox-launcher/issues/138) | On macOS `env -i` removed the variable that keeps bash from dropping rights; the start shell and the core ran without `root` since 2.1.0 | SPEC 151: `/bin/bash -p` and a check of rights in the body |

## 12. Related specs

| Spec | Subject |
|---|---|
| [068](../SPECS/068-Q-N-CODE_THREAT_MODEL/SPEC.md) | Threat model of the code: trust boundaries, catalogue of threats, findings |
| [136](../SPECS/136-F-N-DAEMON_SERVICE_ROOT_OWNED_CORE/SPEC.md) | macOS: the daemon service runs the protected copy |
| [137](../SPECS/137-F-N-CLASSIC_TUN_ROOT_OWNED_CORE/SPEC.md) | macOS: classic TUN runs the protected copy; audit of commands (§12) |
| [139](../SPECS/139-F-N-WINDOWS_ASINVOKER_ELEVATION/SPEC.md) | Windows: start with the user's rights, elevation for TUN |
| [141](../SPECS/141-F-N-DAEMON_MODE_WINDOWS/SPEC.md) | Windows: daemon service and protected copy |
| [150](../SPECS/150-B-C-WINDOWS_PRIVILEGED_GATE_WARNING/SPEC.md) | Windows: a warning in place of a refusal |
| [151](../SPECS/151-B-O-MACOS_PRIVILEGED_SHELL_NOT_ROOT/SPEC.md) | macOS: the start shell ran without `root` |
