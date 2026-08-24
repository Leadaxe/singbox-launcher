# Sing-Box Launcher

**🌐 Language**: English | [Русский](README.ru.md)

[![GitHub](https://img.shields.io/badge/GitHub-Leadaxe%2Fsingbox--launcher-blue)](https://github.com/Leadaxe/singbox-launcher)
[![License](https://img.shields.io/badge/License-GPLv3-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.25%2B-blue)](https://golang.org/)
[![Version](https://img.shields.io/badge/version-1.4.2-blue)](https://github.com/Leadaxe/singbox-launcher/releases)

**Desktop platform for network routing and traffic analysis. 13 VPN protocols, configuration depth and API at enterprise level. Built on top of the [sing-box-lx](https://github.com/Leadaxe/sing-box-lx) fork (upstream sing-box + XHTTP transport + AmneziaWG 2.0) as execution engine — on every platform, including the Windows 7 32-bit legacy build. Drives its own core and, over a mutually-authenticated channel, the cores of remote machines — a router or a VPS — each with its own config.**

**Repository**: [https://github.com/Leadaxe/singbox-launcher](https://github.com/Leadaxe/singbox-launcher)

---

![Core dashboard and Config Wizard](docs/screenshots/01-hero-core-and-wizard.png)

## What it is

A cross-platform desktop client (Windows, macOS, Linux) that wraps sing-box and adds the entire surface around it: visual configurator, multi-subscription management, per-server switching with ping, network observability, declarative routing with preset bundles, a local HTTP API, and self-healing supervision.

Four layers that together define the product:

- **User layer** — one-button start/stop, subscription URL → working VPN flow, server picker with ping, declarative rules via checkboxes, Traffic Profiler window with per-process attribution.
- **Power layer** — Configurator with full sing-box rule semantics (CIDR, domain regex, process matching, sniff, GeoIP/Geosite via SRS), preset bundles with `if`/`if_or` conditions, DNS server selection with conditional rules.
- **Fleet layer** — the **Remote** tab manages other machines (router, VPS, another Mac) running the core as a daemon: each has its own wizard profile and built config, its own Start/Stop and Deploy, its own traffic profiler and host telemetry window.
- **Headless layer** — bearer-auth Debug API on `127.0.0.1`, ~30 local endpoints covering state read/write, action triggers, traffic capture control, and a one-shot snapshot endpoint for support workflows.

## Features

### Connectivity

- **13 connection protocols** — vless, vmess, trojan, shadowsocks, hysteria2, tuic, anytls, masque, ssh, socks / socks5, naive (https / quic), wireguard / AmneziaWG, plus Amnezia `vpn://` profiles.
- **XHTTP transport** — `type=xhttp` on vless/vmess/trojan nodes is parsed, generated into `config.json`, and round-tripped back to share URIs (no longer degraded to httpupgrade). Runs on the bundled sing-box-lx core.
- **AmneziaWG 2.0 (AWG2)** — obfuscation params on wireguard endpoints (`jc` / `jmin` / `jmax`, `s1`-`s4`, `h1`-`h4`, plus CPS packets `i1`-`i5`), parsed from both `wireguard://` and `awg://` URIs and emitted into `endpoints[]`. `h1`-`h4` accept AWG 2.0 randomization **ranges** (`lo-hi`) — the core picks a fresh in-range value on every handshake (core ≥ `1.14.0-lx.1-rc.17`). AWG endpoint MTU is auto-clamped to 1280.
- **Amnezia import** — paste an Amnezia `vpn://…` link (the `.vpn` file content) or the raw `[Interface]/[Peer]` text of a WireGuard/AmneziaWG `.conf` straight into Sources: the launcher decodes the profile / detects the conf blocks and imports them as regular WG/AWG endpoints.
- **Multiple sources per profile** — subscription URLs and direct links (`vless://`, `vmess://`, …) can be mixed in a single configuration.
- **Subscription provider compatibility** — first-class support for HWID-binding panels (Marzban, Marzneshin, Remnawave, NashVPN, V2Board / Xboard) via the canonical XTLS subscription-header protocol (`X-Hwid`, `X-Hwid-Limit`, `Announce`, `Subscription-Userinfo`).
- **Per-source raw cache** — last working subscription body preserved on fetch failure (no broken config when provider is down).
- **TUN inbound** — system-wide VPN driver on Windows / macOS / Linux with `auto-route`, `auto-redirect`, and `find_process` enabled by default.

### Core engines and remote machines

- **Two core engines behind one seam.** *Classic* (default, all platforms) spawns and supervises `sing-box run` and talks to it over the Clash API. *Daemon* (macOS) drives the core inside a long-lived system service (`sing-box lxd`) over gRPC + admin REST — the same in-process model the Android line uses. UI, tray, shortcuts and the Debug API all go through the active engine, so nothing above the seam knows which one is running.
- **Daemon mode benefits** — sudo once (the launcher prepares the install command for **your own** Terminal and never runs anything privileged itself), the VPN keeps running after you quit the launcher (opt-out toggle), config changes swap the core in place with subprocess validation and auto-rollback to the last working config, plus richer observability: live status, connections, core logs and a **balancer pool** view over gRPC.
- **Remote machines** — pair a router / VPS / another Mac over mTLS with a one-time invite (`address#fingerprint#code`). Each machine gets its own registry entry (name, platform, architecture, address), its own wizard profile and built `config.json`, and its own Start / Stop / Deploy — a config built for the router can no longer be deployed to the VPS.
- **Deploy delivers resources, not just JSON** — rule-sets and subscription bodies the machine's config references are shipped into its resource store alongside the config.
- **Host telemetry window** — per-machine CPU / memory / storage / network tables answering "why is the router slow" independently of the core's own traffic view.

### Routing & rules

- **Preset bundles** — community-maintained rule packs with typed variables, local SRS rule-sets, and conditional fragments (`if` / `if_or`). Toggle as checkboxes.
- **User rules** — five typed kinds: IP / CIDR, domain (suffix / keyword / regex), process (name or path regex), SRS URL, raw JSON.
- **17+ matchers** — domain, IP CIDR, ports, network, protocol, process, package name, GeoSite / GeoIP via SRS, composite rules with `invert`.
- **Per-rule outbound chains** through selectors (urltest / failover).
- **Hop chains** — a route through several hops in a row (`you → hop 1 → hop 2 → the site`), added from the ⋮ menu on *Sources* as a third source kind next to a subscription and a single server. A position may be a node, a subscription group or a Direction, so switching a group changes the path without a restart. The chain then behaves like any other server: Directions pick it up and an auto-select group measures the whole multi-hop route. The **Info** window probes it layer by layer, so the `(+N)` delta names the hop that costs the latency. Needs a core built with `with_lx_chain`; on an older one the chain simply does not appear and the launcher says which core you have.
- **Directions** — a named routing target with its own node filter and an optional auto-select twin. A rule points at the Direction, not at a node tag: provider tags are regenerated on every subscription update, so a node-targeted rule breaks the moment the provider renames it.
- **Fold a subscription into a group** — one checkbox instead of the old four flags: a fifty-node subscription arrives as one entry (selector, auto-select, or a selector with an auto-select default).
- **SRS auto-download** — missing-file `⚠` badge on Wizard open; engine never tries to fetch SRS over a not-yet-up VPN.

### DNS

- **DNS servers set up with a form, not raw JSON** — one form per kind that people actually use (UDP, TCP, DoT, DoH and a group), with the fields that kind needs and nothing else. The channel a query travels through is picked from the list of Directions. Raw JSON stays on its own tab for the types without a form (hosts, fakeip, dhcp, quic/h3).
- **DNS groups** — several resolvers behind one name: the group queries its members and takes the fastest answer, so one dead resolver no longer stalls name resolution.
- **Per-domain DNS rules** routing specific names to specific servers.
- **Resolve strategy**: `prefer_ipv4` / `prefer_ipv6` / `ipv4_only` / `ipv6_only`.

### Network observability

- **Traffic Profiler** — always-on capture with 60-second × 3000-event rolling buffer, per-process attribution, CNAME-chain reconstruction, DNS-to-IP inferred matching. A paired machine gets its own profiler instance and window (fed by its gRPC streams), so two machines can be watched side by side.
- **Issue classification** — `⚠ DnsTimeout` / `⚠ TcpRstEarly` surfacing concrete diagnostic signals.
- **Pre-session backfill** — last 60 seconds of matching events copied into a fresh recording session.
- **Three-stream Log Viewer** — Internal launcher logs / Core sing-box log file / Clash API client log, with level filter and log-rotation safety.
- **IP-check tools** — STUN (UDP) and HTTPS providers (2ip.ru and others) for external IP verification.

### Reliability

- **Auto-restart with stability window** — 3 attempts × 180 s, counter resets after stable operation; UI shows `[restart 2/3]` during recovery.
- **Atomic file writes** — `stage → rename` for config / state / settings; no half-written files on `kill -9` or power loss.
- **Power-event aware** — sleep / resume listener; HTTP requests don't hang after wake.
- **Configuration overlay** — state stores template references plus user diffs; template bumps deliver new defaults automatically while personal edits stay on top.
- **Auto-update subscriptions** — hourly heartbeat refreshes only stale sources; immediate retry on VPN-event with anti-storm cooldown.

### System integration

- **System tray** — start / stop, proxy switcher (when Clash API is on), open main window, exit. Active outbound mirrored in the tray.
- **Keyboard shortcuts** — `⌘R` / `Ctrl+R` reconnect (kill sing-box for restart), `⌘U` / `Ctrl+U` update subscriptions, `⌘P` / `Ctrl+P` ping all proxies.
- **CLI flags** — `-start` (auto-start VPN on launch), `-tray` (start minimized to tray). Useful for autostart, system services, and headless deployment.
- **Auto-loaders** — proxy list restored on every sing-box start; active outbound persists across restarts.
- **Share URI** — right-click any proxy in the server list (Local or Remote) → **Copy link** generates a share URI (`vless://`, `vmess://`, `trojan://`, `ss://`, `hysteria2://`, `wireguard://`) from the matching outbound in `config.json`.

### Power tools

- **Debug API** — local HTTP API (~30 local endpoints plus the `/remote/*` and `/daemon/*` groups; bearer-auth, off by default) for state read/write, action triggers, traffic capture control. See [Headless control plane](#headless-control-plane--debug-api).
- **Configurator** — 7-tab visual editor (Target / Sources / Directions / Rules / DNS / Settings / Files) with schema validation, named state snapshots, atomic save. The **Target** tab decides which machine the config is built for — this one, or a paired remote machine with its own OS/architecture and optional gateway role.
- **LX Backup** — carry settings between the desktop launcher and LxBox on your phone: *Settings → Backup* exports subscriptions, servers, rules, DNS and portable variables into one file and imports one back. Anything the other side has no place for travels along untouched, so a backup that passed through the phone does not come back impoverished.
- **Snapshot for support** — `GET /debug/snapshot` or **Copy snapshot** button packages template + state + cache + config into a single JSON for bug reports.
- **Verbose toggle** — `🔬 dbg` button in Traffic Profiler flips sing-box `log_level=debug` with atomic rebuild and revert.

### Distribution

- **Cross-platform** — Windows 10/11 (fully tested), Windows 7 via legacy build, macOS 11+ universal (Apple Silicon + Intel), Linux (build from source).
- **English and Russian UI** — the English text at the call site *is* the translation key (SPEC 111), so English lives in the code and `bin/locale/ru.json` carries the Russian catalogue.
- **Self-update** — pinned sing-box version auto-downloaded on mismatch; launcher self-update check at startup with notification (no silent installation).

## Quick start

1. Download from [GitHub Releases](https://github.com/Leadaxe/singbox-launcher/releases) and install (see [Installation](#installation)).
2. Open the app → **Local** tab → click **Download** to fetch the matching `sing-box` binary (and `wintun.dll` on Windows). The core is the `sing-box-lx` fork (XHTTP + AmneziaWG 2.0) from its GitHub Releases — on every platform, including the Windows 7 32-bit `legacy-windows-7` build (with a GitHub-proxy mirror fallback if GitHub is blocked).
3. Click **Wizard** → paste your subscription URL on the **Sources** tab → step through Directions / Rules / DNS / Settings / Files → **Save**.
4. Back on **Local** → **Start**. Servers are in the same tab's left column; monitor traffic via the **Traffic Profiler** button in Diagnostics.

### Command-line flags

```bash
singbox-launcher -start         # auto-start VPN on launch
singbox-launcher -tray          # start minimized to system tray
singbox-launcher -start -tray   # combined — headless autostart scenario
```

Useful for OS-level autostart (`LaunchAgents` / `Task Scheduler` / `systemd --user`) and for running the launcher as a background service that drives sing-box without showing a window.

## Feature tour

### Multi-subscription management

![Server switching across subscriptions](docs/screenshots/02-server-switching.png)

Add multiple subscription sources, each with its own update schedule and per-source raw cache. The **Local** tab's server column exposes selector groups (`proxy-out`, `vpn ①`, `ru VPN`, etc.) defined in your ParserConfig and shows per-server latency with one-click switching. Active outbound is mirrored in the system tray for quick swaps without opening the main window.

Per-source `SubscriptionMeta` surfaces upstream state — profile title, support URL, traffic usage (`UploadBytes` / `DownloadBytes` / `TotalBytes`), expiration date, last-fetch status, and provider announcements (`📢` on success-with-announce, `⚠` on error-with-announce — actionable URL in the UI).

**Share URI** — right-click any proxy row in the server list → first menu line shows the Clash API outbound type (lowercase: `vless`, `vmess`, `trojan`, `selector`, `direct`, …), then **Copy link** generates a share URI from the matching outbound in `config.json` (or from a WireGuard `endpoint[]` entry if the tag isn't an outbound). Convenient for moving a server to another device or sharing it with a teammate.

### Declarative routing with preset bundles

![Rules tab and Subscription identification](docs/screenshots/03-rules-and-hwid.png)

Two-level rule model:

- **Preset bundles** (community-maintained template) — self-contained rule packs with typed variables (`enum`, `dns_server`, `outbound` with whitelists), local SRS rule-sets, conditional fragments (`if` / `if_or`), and DNS-server definitions. Toggle as checkboxes. Includes ready-made bundles like `ru-direct` (Russian traffic → direct), `ads-all` (ad-block via SRS), Telegram routing, BitTorrent splitting, and more.
- **User rules** — your own rules with 5 typed kinds: IP/CIDR, domains (suffix/keyword/regex), processes (name or path regex), SRS URLs, raw JSON.

Matchers cover the full sing-box surface: domain (4 variants), IP CIDR, ports, network, protocol, process, package name, GeoSite / GeoIP via SRS, composite rules with `invert`, per-rule outbound chains through selectors (urltest / failover).

The **Subscription identification** section (right side) implements [SPEC 061](SPECS/061-F-N-SUBSCRIPTION_HEADER_PROTOCOL/SPEC.md) — the canonical XTLS / Remnawave subscription-header protocol. Random per-installation HWID, opt-out toggle, optional hash-based device model. Regenerate at any time.

### DNS configuration

![DNS configuration and About info](docs/screenshots/04-dns-configuration.png)

Per-DNS-server enable/disable, multiple transports (UDP / DoH / local), strategy (`prefer_ipv4` / `prefer_ipv6` / `ipv4_only` / `ipv6_only`), per-domain DNS rules pointing to specific servers (e.g. `domain=mysite.ru → test server`), default DNS resolver selection. SRS-based domain rules supported via the same library as routing rules.

### TUN, diagnostics, and power tools

![TUN settings and diagnostics tools](docs/screenshots/05-tun-settings-and-diagnostics.png)

TUN inbound (system VPN driver), MTU, stack selection (system / gvisor), TLS root certificate store, URLTest target and interval, custom proxy-in inbound — all in the wizard, no JSON editing required.

Power tools on the right:

- **Log window** — three parallel streams (Internal launcher logs / Core sing-box log file / Clash API client log), level filter, log rotation safe.
- **Logs / Config folder** — one-click open in Finder/Explorer.
- **Kill Sing-Box** — force restart; the supervisor handles the rest.
- **Traffic Profiler** — see below.
- **IP check services** — STUN (UDP), HTTPS IP-check providers (2ip.ru, etc.).
- **Debug API** — toggle the local HTTP API; bearer token generated on first enable and preserved across off/on cycles.

### Traffic Profiler

![Traffic Profiler event detail](docs/screenshots/06-traffic-profiler.png)

Always-on background capture with a 60-second × 3000-event rolling buffer. Two data sources joined by connection ID: Clash API `/connections` (per-process metadata from sing-box) and sing-box log tail (DNS resolves, router matches, outbound dials).

Per-process view has four sub-tabs:

- **Live** — newest-first event stream with color coding by kind.
- **Domains** — aggregated unique domains sorted by bytes; tap to see CNAME chain, all IPs, outbound chain, issues.
- **IPs** — useful for hostless connections (raw TCP without SNI sniff).
- **Connections** — per-connection timeline; tap to see rule, outbound, CNAME chain, issues.

Issue classification surfaces concrete problems: `DnsTimeout` (DNS resolver did not respond), `TcpRstEarly` (TCP closed <1s with 0/0 bytes — firewall RST / TLS fail / block). Verbose mode (🔬 dbg) toggles `log_level=debug` with atomic config rebuild and revert. Pre-session backfill copies the last 60 seconds of matching events into a new session, so you don't lose the first seconds of the problem you started recording for.

### System tray

The tray icon stays visible after the main window is closed and provides:

- **Start / Stop** sing-box.
- **Proxy switcher** — when Clash API is on, the active group's proxies appear as a submenu with current selection marked. Switching from the tray triggers the same path as switching from the server list.
- **Show main window** / **Exit**.

Combined with `-tray` CLI flag, this is the headless-style operating mode: launcher starts hidden, you control everything from the tray, the main window opens only when you need to configure.

Keyboard shortcuts (work regardless of which tab is focused, unless a text field is consuming input):

| Shortcut | Action |
| --- | --- |
| `⌘R` / `Ctrl+R` | Reconnect (kill sing-box for restart — supervisor auto-recovers) |
| `⌘U` / `Ctrl+U` | Update subscriptions |
| `⌘P` / `Ctrl+P` | Ping all proxies (same as the ping-all button above the server list) |

### Headless control plane — Debug API

Local HTTP API on `127.0.0.1`, bearer-auth, off by default. ~30 local endpoints in five groups (plus the `/remote/*`, `/daemon/*` and `/chains/*` groups when their capability is on):

| Group | Coverage |
| --- | --- |
| **Health & info** | health-check (no auth), launcher / sing-box / API version |
| **State read** | running state, active proxy, group, proxy list, full state, resolved outbounds |
| **State write** | rules / DNS / DNS-rules with `replace` and `append` modes, schema-validated before commit, mutex per state path |
| **Actions** | start / stop / update-subs / ping-all / rebuild-config — synchronous triggers |
| **Traffic Profiler control** | start / stop / clear / live snapshot / sessions list / export / drop / processes / verbose toggle |
| **Snapshot** | `/debug/snapshot` — template + state + cache + config as one JSON for bug reports |

Use cases: automation scripts (`bash` + `curl`), MCP wrappers for AI agents, CI/CD validation of new templates, headless deployment, regression fixtures. No public, documented, scriptable HTTP API of this scope exists in any other desktop sing-box client.

**Full reference + curl cookbook:** [`docs/API.md`](docs/API.md). Design notes: [SPEC 038](SPECS/038-F-C-DEBUG_API/SPEC.md).

Toggle in **Settings → Debug API (localhost)**. The same `snapshot.Build()` powers the **Copy snapshot** button in Diagnostics — one click to package the full state for a bug report.

### Auto-update and supervision

- **Process supervision** — 3 restart attempts, 180-second stability window resets the counter, graceful shutdown with 2-second deadline. UI shows `[restart 2/3]` during recovery. Same pattern as `systemd RestartSec` + `StartLimitBurst`.
- **Per-source heartbeat** (every hour) — refreshes only stale subscriptions, 15-second retry on failure, immediate retry on VPN-event with 5-second anti-storm cooldown.
- **Atomic writes** — `stage → rename` for config, state, settings, and per-source raw cache. Kill -9 or power loss never leaves a half-written file. A failed fetch never overwrites the last working cached subscription body.
- **Power-event aware** — sleep/resume listener; no HTTP request hangs after wake.
- **Self-update** — pinned sing-box version auto-downloaded on mismatch (SPEC 046); launcher self-update checks once at startup with a popup notification.

## Subscription provider compatibility

Verified to work with the canonical subscription-header protocol used by these panels:

| Panel | Subscription URL format | HWID-binding (SPEC 061) |
| --- | --- | --- |
| **Marzban** | `https://panel.example.com/sub/<token>` | yes — `X-Hwid` + `X-Hwid-Limit` |
| **Marzneshin** | `https://panel.example.com/sub/<token>` | yes |
| **Remnawave** | `https://panel.example.com/sub/<token>` | yes — canonical XTLS-format announce headers |
| **NashVPN** | `https://sub.example.com/<token>` | yes — provider returns empty body without HWID headers |
| **V2Board / Xboard** | `https://panel.example.com/api/v1/client/subscribe?token=<...>` | partial — `subscription-userinfo` only |
| **3X-UI / X-UI** | direct vless/vmess/trojan URIs | n/a |
| **Sing-box subscription** | any compatible `sing-box export` source | n/a |

User Agent: `singbox-launcher/<version> (<os> <arch>)`. Privacy controls in Settings:

- **Send device ID** — opt-out toggle for `X-Hwid`. If disabled, HWID-binding panels may refuse to serve the subscription.
- **Hash device model** — sends `hash(model)` instead of plain model string.
- **Device ID (HWID)** — random UUIDv4, not derived from hardware. Regenerate at any time.

## Requirements

### Windows

- **Recommended:** Windows 10 / 11 (x64).
- **Legacy:** Windows 7 (x86/x64) via separate build `singbox-launcher-<version>-win7-32.zip` with the sing-box-lx fork core (32-bit `windows-386-legacy-windows-7` build — **XHTTP + AmneziaWG 2.0 work on Win7 too**) and 32-bit `wintun.dll`.
- [sing-box-lx](https://github.com/Leadaxe/sing-box-lx/releases) — fork core (XHTTP + AmneziaWG 2.0) auto-downloaded via the Local tab on **all** Windows builds, including the Windows 7 32-bit `legacy-windows-7` asset.
- [WinTun](https://www.wintun.net/) (wintun.dll, MIT license) — auto-downloaded via the Local tab.

### macOS

- **Universal** (recommended): macOS 11+ (Big Sur), supports Apple Silicon and Intel.
- **Intel-only legacy build**: macOS 10.15+ (Catalina).
- [sing-box-lx](https://github.com/Leadaxe/sing-box-lx/releases) — fork core (XHTTP + AmneziaWG 2.0) auto-downloaded via the Local tab.
- **Daemon mode** (optional, macOS only) additionally needs a core built with the `lxd` subcommand (`with_lx_command`). The pinned `RequiredCoreVersion` ships it (the current value lives in `internal/constants/constants.go` — the single source of truth). See [docs/DAEMON_AND_REMOTE.md](docs/DAEMON_AND_REMOTE.md).

### Linux

Pre-built binaries are not distributed. Build from source — see [Building from source](#building-from-source). Help with testing on common distros is welcome — please open an issue with feedback.

## Installation

### Windows

1. Download from [Releases](https://github.com/Leadaxe/singbox-launcher/releases) — regular archive for Win 10/11, `singbox-launcher-<version>-win7-32.zip` for Windows 7.
2. Extract to any folder (e.g. `C:\Program Files\singbox-launcher`).
3. Run `singbox-launcher.exe`.
4. **Local** tab → **Download** to fetch `sing-box.exe`, then **Download wintun.dll** if needed.
5. Open **Wizard** → paste subscription URL → walk through tabs → **Save** → **Start**.

### macOS

#### Option 1: install script (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/Leadaxe/singbox-launcher/main/scripts/install-macos.sh | bash
```

Installs to `/Applications/`, removes quarantine attributes, fixes permissions, ensures compatibility with Apple Silicon and recent macOS. For a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/Leadaxe/singbox-launcher/main/scripts/install-macos.sh | bash -s -- v1.1.0
```

#### Option 2: manual install

1. Download the macOS ZIP from [Releases](https://github.com/Leadaxe/singbox-launcher/releases) and extract it.
2. Remove quarantine:
   ```bash
   xattr -cr "singbox-launcher.app" && chmod +x "singbox-launcher.app/Contents/MacOS/singbox-launcher"
   ```
3. Double-click `singbox-launcher.app`, or `open singbox-launcher.app` from terminal. If macOS still blocks the app, allow it via **System Settings → Privacy & Security → Open Anyway**.

### Linux

Build from source ([Building from source](#building-from-source)), then:

```bash
chmod +x singbox-launcher
./singbox-launcher
```

`sing-box` is auto-downloaded on first launch into `bin/`. If `sing-box` is on `PATH` (e.g. from a distro package), the launcher uses that binary instead.

## Folder layout

```
singbox-launcher/
├── bin/
│   ├── sing-box(.exe)             — engine, auto-downloaded
│   ├── wintun.dll                  — Windows only, auto-downloaded
│   ├── config.json                 — sing-box runtime config (derived view)
│   ├── wizard_template.json        — community template with preset bundles
│   ├── wizard_states/
│   │   ├── state.json              — this machine's wizard state
│   │   ├── <name>.json             — named state snapshots
│   │   └── remote/<machine-id>/    — one directory per paired machine:
│   │                                 state.json, config.json, srs/, subscriptions/
│   ├── subscriptions/<id>.raw      — per-source raw cache (SPEC 052)
│   ├── rule-sets/*.srs             — cached SRS rule-sets
│   └── logs/                       — sing-box.log + rotated history
└── singbox-launcher(.exe)
```

`bin/` layout is a stable contract — external tools (backup scripts, MCP servers, CI) can rely on it.

## Building from source

See platform-specific guides:

- **Windows** — [docs/BUILD_WINDOWS.md](docs/BUILD_WINDOWS.md) (Go 1.24+, GCC required, optional `rsrc` for icon)
- **macOS** — `./build/build_darwin.sh [universal|arm64|intel|catalina]`, optional `-i` to install/update in `/Applications`. With no build type it targets **this Mac's architecture** — the fast path for local work; pass `universal` for a release build
- **Linux** — [docs/BUILD_LINUX.md](docs/BUILD_LINUX.md) (Go 1.24+, OpenGL + X11 dev packages, or Docker build)

Quick reference:

```bash
git clone https://github.com/Leadaxe/singbox-launcher.git
cd singbox-launcher

# macOS — build for this Mac and install into /Applications
./build/build_darwin.sh -i

# macOS — universal binary (releases)
./build/build_darwin.sh -i universal

# Linux — build script with package check
./build/build_linux.sh

# Windows — see docs/BUILD_WINDOWS.md
build\build_windows.bat
```

## Tests

Use the centralized scripts in `build/` — they exclude GUI packages that require OpenGL on headless runners:

```bash
./build/test_linux.sh    # Linux
./build/test_darwin.sh   # macOS
build\test_windows.bat   # Windows
```

To run GUI tests locally, set `TEST_PACKAGE` manually inside the script or invoke `go test` directly on the desired path.

## Documentation

- **[docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)** — full project architecture map.
- **[SPECS/CONSTITUTION.md](SPECS/CONSTITUTION.md)** — architectural invariants.
- **[SPECS/](SPECS/)** — 90+ feature specs (HWID protocol, traffic profiler, debug API, preset bundles, state-as-template-diff, atomic writes, typed event bus, daemon core engine, remote machines, …).
- **[docs/API.md](docs/API.md)** — Debug API reference with a curl cookbook.
- **[docs/DAEMON_AND_REMOTE.md](docs/DAEMON_AND_REMOTE.md)** — daemon core engine, pairing, and remote-machine management.
- **[docs/WIZARD_TEMPLATE.md](docs/WIZARD_TEMPLATE.md)** — `wizard_template.json` syntax reference for VPN providers shipping a custom template.
- **[docs/ParserConfig.md](docs/ParserConfig.md)** — subscription parser configuration reference.
- **[docs/TRAFFIC_PROFILER.md](docs/TRAFFIC_PROFILER.md)** — Traffic Profiler internals and usage.
- **[docs/TEMPLATE_REFERENCE.md](docs/TEMPLATE_REFERENCE.md)** — `wizard_template.json` schema reference.

## Troubleshooting

| Symptom | First check |
| --- | --- |
| sing-box won't start | Download via **Local → Download**, then verify `config.json` exists. Check `bin/logs/sing-box.log`. |
| Wizard opens but Save fails | Inspect Internal log in **Log window**; schema validation error message is logged. |
| Server list is empty / disabled | sing-box is not running (the list is intentionally inert until the engine is up). |
| Subscription returns empty / errors | Check **Subscription identification** in Settings — HWID-binding panels need `Send device ID` enabled. Look at the ⚠ badge tooltip for provider announce. |
| TUN doesn't capture traffic (Linux/macOS) | TUN interface usually needs root: `sudo ./singbox-launcher` or `sudo setcap cap_net_admin+ep ./singbox-launcher` (Linux). |
| Win7 32-bit: tray icon shows but window is blank / empty frame | OpenGL 2.0 vs Fyne's 2.1+ requirement — see [docs/WIN7_OPENGL.md](docs/WIN7_OPENGL.md) for the Mesa3D drop-in fix. |
| Subscription auto-update silent | Open **Settings → Subscriptions** — confirm `Auto-update subscriptions` is on. Heartbeat is hourly; immediate retry fires on VPN-event. |
| Need full state for a bug report | **Diagnostics → Copy snapshot** packages template + state + cache + config into one JSON. |

## Contributing

For substantial features the project uses a spec-driven workflow — write a SPEC under `SPECS/` describing schema, phases, invariants, and acceptance criteria before code. See [AGENTS.md](AGENTS.md) (contributor guide) and [SPECS/README.md](SPECS/README.md) (closing-task checklist) for details.

Standard flow:

1. Fork the repository.
2. Branch off `develop` (`git checkout -b feature/your-feature`).
3. For non-trivial work, draft a SPEC and open it as a discussion first.
4. Commit, push, and open a Pull Request against `develop`.

Code style: `gofmt`, `golangci-lint`. Public functions should be documented. New paths should log start / success / error per `SPECS/CONSTITUTION.md §5`.

## License

GNU General Public License v3.0 — see [LICENSE](LICENSE).

Commercial licensing from Leadaxe is available for uses that are not compatible with GPLv3. Commercial terms are negotiated privately and are not published in this repository. Contact: [leadaxe@gmail.com](mailto:leadaxe@gmail.com). See [LICENSING.md](LICENSING.md).

## Acknowledgments

- [SagerNet/sing-box](https://github.com/SagerNet/sing-box) — the proxy engine.
- [Fyne](https://fyne.io/) — cross-platform UI framework.
- All project contributors.

## Support

- **Telegram channel** — [@singbox_launcher](https://t.me/singbox_launcher)
- **Issues** — [GitHub Issues](https://github.com/Leadaxe/singbox-launcher/issues)

---

*Independent project. Not affiliated with the upstream sing-box project.*
