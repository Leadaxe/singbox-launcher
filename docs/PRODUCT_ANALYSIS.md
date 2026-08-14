# Product analysis — singbox-launcher

**🌐 Language**: English | [Русский](PRODUCT_ANALYSIS.ru.md)

**One-liner:** "A declarative routing engine + a network profiler + a headless control plane on top of sing-box."

---

## Positioning

**A desktop platform for network routing and traffic analysis. 13 VPN protocols, enterprise-level configuration depth and API, with sing-box as the execution engine.**

Seven theses that describe the product:

1. **A VPN observability platform.** The Traffic Profiler + the Debug API give full visibility into traffic and state — within 30 seconds you can see which outbound the router picked, which DNS resolve produced which IP, and which rule the connection matched.

2. **Headless mode out of the box.** A bearer-auth HTTP API of 28 paths (27 protected + the unauthenticated `/ping`) with a documented MCP integration path (SPEC 038 §6.5). Suitable for DevOps scripts and AI agents: Claude reads `/state` over MCP, switches proxies and restarts the engine — without opening the UI.

3. **Configuration overlay.** State stores references to the community template plus the diff of the user's changes (SPEC 058). A template bump delivers updates automatically while personal edits ride on top. The dotfile-manager pattern (chezmoi / yadm) applied to VPN configs.

4. **Reliability first.** Auto-restart with a stability window (3 attempts × 180s), atomic writes (`stage → rename`), a per-source raw cache that falls back on failure, and power-event-aware HTTP transports.

5. **Self-service support through snapshots.** `GET /debug/snapshot` plus the "Copy snapshot" button in Diagnostics → one click packs template + state + cache + config into the clipboard for a bug report.

6. **A complete per-process traffic map with CNAME-chain reconstruction.** On par with Little Snitch ($59, macOS) / GlassWire ($39, Windows) — cross-platform, free, and integrated with the routing engine.

7. **Declarative routing for non-programmers.** Preset bundles (SPEC 053) with `if`/`if_or` conditions, local SRS rule-sets and typed vars (`enum`, `dns_server`, `outbound` with whitelists) turn writing pfSense-grade rules into checkboxes and dropdowns.

---

# User-facing capabilities

## Split connections / per-connection routing

Files: `internal/traffic/clash_connections.go`, `api/clash.go`, `ui/clash_api_tab.go`, `core/state/connections.go`, `bin/wizard_template.json`.

### The TUN inbound + declarative split tunneling

The template (`bin/wizard_template.json`) enables a TUN inbound with auto-route + auto-redirect (Linux) + `find_process: true` — the equivalent of a system VPN driver. Every packet goes through the sing-box routing engine with the rules from `state.rules[]`. The per-connection decision is **proxy / direct / block**, at the granularity of domain + CIDR + port + process (by name or path regex) + sniff (TLS SNI / HTTP Host).

### Real-time per-connection observability — the Clash API `/connections`

`internal/traffic/clash_connections.go` polls `http://127.0.0.1:9090/connections` once per second. For every connection it knows: the source process (path and name), the network/protocol, the host, the destination IP+port, the upload/download counters, the chain of outbound selectors, and the matched rule.

`ConnPoller` compares adjacent snapshots and emits three kinds of event:
- **Opened** — new conn-ids → emit `TCPOpen` / `UDPOpen`
- **Closed** — gone → emit `*Close` with a duration of `now - start`
- **Bytes** — the byte delta for the active ones

A stream of lifecycle events with per-process attribution.

### Routing-decision visibility

Every connection in `/connections` carries `chains: ["vless-server", "🇫🇮 Finland", "vpn-1"]` (the leaf→root outbound chain) plus `rule: "domain_suffix"` and `rulePayload: "example.com"` — the user sees which rule drove the routing decision and which chain of selectors the connection took. In the Traffic Profiler UI the chain expands in full: `Slack.app → api.slack.com → CNAME slack-edge.com → 104.16.x.x → via vpn-1 → 🇫🇮 Finland`. This is the level of pfSense / Untangle dashboards, tied to the originating process.

### Split-tunneling policies

Typical preset bundles in the template: `ru-direct` (Russian traffic → direct, everything else → VPN), `ads-all` (block ad domains) and others. Declarative policy-based routing: "process X → direct, domain Y → block, everything else → VPN-EU". Configuration lives in the Configurator as checkboxes and dropdowns.

---

## Complex routing rules — firewall grade

Files: `core/state/rule_types.go`, `SPECS/053-F-N-PRESET_BUNDLES/SPEC.md`, `SPECS/018-F-C-CUSTOM_RULE_SUBSYSTEM_REFACTOR/SPEC.md`, `ui/configurator/tabs/rules_tab.go`, `bin/wizard_template.json`.

### The two-level rule model

**Level 1 — preset bundles** (SPEC 053, the `presets_v1` schema). `wizard_template.json` holds self-contained presets with:

- `vars` — parameters (types: `outbound | dns_server | enum | text | number | bool`) with UI controls (dropdown, checkbox, text entry)
- `rule_set[]` — local SRS rule-sets (inline or a remote URL); tags are prefixed `<preset_id>:<tag>`
- `dns_servers[]` — the preset's local DNS servers
- `rule` — a sing-box routing rule
- `dns_rule` — a sing-box dns rule
- `if` / `if_or` conditions — `if: ["use_yandex_dns"]` includes the fragment only when the listed bool vars are true

Every var has either `options` (a whitelist) or `select: "local"|"global"` (a scope shortcut). `default` is mandatory.

**Level 2 — user rules** (SPEC 018). Five typed constants:

- `ips` — IP/CIDR
- `urls` — domain / domain_suffix / domain_keyword / domain_regex
- `processes` — process_name or process_path_regex (with the Simple/Regex mode stored in `params`)
- `srs` — an SRS rule-set by URL (your own or one from the runetfreedom catalogue)
- `raw` — the raw JSON of a rule

### Supported matchers

From `bin/wizard_template.json` and `core/state/rule_types.go` (`Rule.Body` via `InlineBody.Match map[string]interface{}`):

- `domain`, `domain_suffix`, `domain_keyword`, `domain_regex`
- `ip_cidr`, `source_ip_cidr`
- `port`, `port_range`, `source_port`, `source_port_range`
- `network` (tcp/udp), `protocol`
- `process_name`, `process_path`, `process_path_regex`
- `package_name` (Android compatibility)
- `geosite`, `geoip` (through a rule_set with a remote URL)
- `rule_set` (local SRS binary files)
- `inbound`, `outbound`, `user`, `clash_mode`
- Composition: nested `rules` + `invert: true`

Per-rule action: `route(outbound)`, `route-options`, `reject`, `direct`.

### SRS — Sing-box Rule Sets

`core/state/rule_types.go::SrsBody{Name, SrsURL, Outbound}` + SPEC 014 + SPEC 020 (local download). The launcher:

- caches `.srs` files in `bin/rule-sets/`
- forbids `type: remote` in the final `config.json` (`convertRuleSetToLocalRequired()` in `core/build/route_merge.go`, SPEC 045 phase 9) — otherwise sing-box on a cold start tries to download the rule-set through a VPN proxy that isn't up yet → fatal. The invariant catches a real bug
- auto-downloads an SRS when the Configurator opens (if the file is missing)
- shows a ⚠ badge when an SRS file is missing

### Level of sophistication

Compared with pfSense (which offers `src/dst/proto/port` rules with `action: pass/block/reject` and an interface matcher), singbox-launcher adds:

- a sniff matcher (TLS SNI, HTTP Host)
- a process matcher on the path, with regex
- GeoIP/Geosite via SRS
- composite rules with `invert`
- a per-rule outbound chain through selectors (urltest / failover)
- a per-connection routing decision with full chain visibility (see the Split section)

sing-box is not a stateful firewall — it is an L4 routing engine, and the product stays inside those bounds deliberately.

---

## The log analyzer

A two-loop system with a typed multi-sink architecture.

### The internal sink (for launcher logs)

`internal/debuglog/debuglog.go`: six levels (`Off/Error/Warn/Info/Verbose/Trace`), the `ErrorLog/WarnLog/InfoLog/DebugLog` factories, `StartTiming()` for measuring operations, and `LogTextFragment()` for smart truncated logging of large blocks. The constitution (`SPECS/CONSTITUTION.md §5`) requires new code paths to log start / success / error.

Through `SetInternalLogSink` the launcher's log channel is fanned out to the UI viewer in real time. Dev builds are verbose, release builds warn-only, decided automatically by the build branch.

### Log Viewer Window (SPEC 007)

`ui/log_viewer_window.go` (a singleton window). Three parallel tabs:

| Tab          | Source                                                                                | Level filter                  |
| ------------ | ------------------------------------------------------------------------------------- | ----------------------------- |
| **Internal** | the `debuglog.SetInternalLogSink` sink — every launcher call in real time             | yes, Error→Trace, at render   |
| **Core**     | a tail of `bin/logs/sing-box.log` (rotation-safe) auto-refreshed every 5s              | visually, by keyword parsing  |
| **API**      | the `api.writeLog()` sink — every `core/services/api_service.go` call to the Clash API | yes, by level                 |

An enterprise-style observability split: three concentric circles (launcher / API client / engine), each with its own pipeline, in one window.

### The log tailer with rotation detection

`internal/traffic/logtail.go` + `inode_unix.go` / `inode_windows.go`: inode/FileIndex-based rotation detection. Every read tick compares the identity of the current FD with the identity of the file at that path; a rename-rotate → reopen from the start; a truncate → seek to 0. `fsnotify` was excluded deliberately — it is noisy and misses the macOS rename edge cases. This is SRE-grade tailing (Vector / Filebeat territory).

### The log parser (event analyzer) — `internal/traffic/parser.go`

A deterministic parser for sing-box logs:

- five regexes: `reDNSExchanged`, `reDNSFailed`, `reRouterProcess`, `reRouterMatch`, `reInboundOut`
- `LogLine` — the structured representation of one line: `TS, Kind (EventDNSResolve|DNSFail|TCPOpen|TCPClose|UDPOpen|UDPClose|RouterMatch), ConnID, Domain, IP, CnameTarget, Port, ProcessPath, Rule, Outbound, FailReason`
- `connIDInner = [0-9A-Za-z._-]+` — catches both the numeric and uuid forms (sing-box changed the format)
- tolerant of an optional timestamp prefix (`reLeadingTS`) in three different time formats
- covered by `parser_test.go` against the log zoo in `testdata/sing-box-logs/`

The sing-box log stream is turned into typed `LogLine` events with a kind discriminator and fanned out through `Subscribe()` to the UI viewer and the Traffic Profiler at once. A cross-source join on `ConnID` with the Clash API `/connections` assembles the picture: "the Slack process went to api.slack.com through outbound vpn-1, matched by the domain_suffix rule, and the TLS handshake got an RST after 800ms". Grafana Tempo / Datadog APM-grade observability applied to a VPN stack.

---

## Traffic Profiler — the network analyzer window

Files: `SPECS/059-F-N-TRAFFIC_PROFILER/SPEC.md`, `internal/traffic/profiler.go`, `session.go`, `types.go`, `clash_connections.go`, `logtail.go`, `parser.go`, `ui/traffic/window.go`, `live_view.go`, `per_process_view.go`, `process_picker.go`, `toolbar.go`, `event_detail.go`.

### A singleton service, always on

`TrafficProfiler` (`internal/traffic/profiler.go`) is a singleton, started at app startup in `ui/traffic_bootstrap.go::startProfiler` and alive until quit. The 60s × 3000-event rolling buffer is always populated. The cost: roughly 50–200 lines/second of regex parsing in a background goroutine.

### Cross-source join

Two sources:

1. **The Clash API `/connections`** (polled every 1s) — the list of active connections with process metadata.
2. **A tail of sing-box.log** through the inode-rotation-safe tailer.

They are joined by conn-id (the `[12345]` prefix in the logs equals `id` in the Clash API). The profiler keeps:
- `connProcessMap[conn_id] → process_path` (from the `router: found process name` log line)
- `dnsAccum[conn_id] → []string` (accumulating the CNAME chain)
- `dnsByIP[dest_ip] → DNSAttribution` (a 10-second window for inferred attribution through DNS)

### Attribution with confidence levels

- `verified` — the sing-box log explicitly matched the process name by path
- `inferred` (〽) — a TCP connection to an IP that was resolved by a DNS query attributed to the target within the 10s window
- `unattributed` — nothing matched (shown only in the system-wide Live view)

### ⚠ Issue classification

Concrete diagnostic signals, not statistical anomalies:

- **DnsTimeout** — `dns: exchange failed for X: context deadline exceeded`
- **TcpRstEarly** — the TCP connection closed in under 1s with 0/0 bytes. A firewall RST / TLS failure / a block

Rejected as noisy: `geoMismatch`, `unusualPort`, `badLatency` — LxBox went down that road and ripped them out.

### Four per-process views

| Sub-tab          | What it shows                                                                                        |
| ---------------- | ---------------------------------------------------------------------------------------------------- |
| **Live**         | a newest-first event stream, colour-coded by kind                                                    |
| **Domains**      | aggregated unique domains, sorted by total bytes; tap → CNAME chain, all IPs, outbound chain, issues |
| **IPs**          | aggregated unique destinations; useful for hostless connections (raw TCP with no SNI sniff)          |
| **Connections**  | per-connection timeline (open/close); tap → CNAME chain, IPs, rule, outbound, issues                 |

### The verbose toggle, with revert

The 🔬 dbg button flips `vars[log_level]=debug`, performs an atomic config rebuild and calls `ProcessService.KillForRestart()` (the sing-box watcher restarts it automatically). Turning it OFF reverts. A confirmation dialog warns: "Reloading sing-box — active connections will reset".

### Pre-session backfill

A 60s × 3000-event rolling buffer covering ALL processes. On `▶ START`, the events from the last 60s that match the target are copied into the session marked 〽 backfilled. This solves the classic problem of "the user sees the problem and only then starts recording — losing the first seconds". An observability best-practice pattern (like the `bpftrace` history buffer).

### Lifecycle and edge cases

- Ring-buffer 5 completed sessions. Force-stop = wipe (in-memory only, no persist).
- A sing-box restart mid-session → partial CNAME chains are auto-finalized, the session continues in the new conn-id space, and `verbose_toggled_at` is recorded.
- A memory cap of 50000 events / a 3h sliding window → the `events_dropped` counter in the footer.
- Log rotation safe.
- Close the window → background capture continues (rolling buffer + clash polling); reopening shows the active session.
- Export the session JSON through the overflow menu (clipboard + file).

### Counterparts in the native ecosystem

| Native counterpart    | What they share                     | Where we do better                                                             |
| --------------------- | ----------------------------------- | ------------------------------------------------------------------------------ |
| Little Snitch (macOS) | per-process connections, host class | + cross-platform, + integrated with the VPN routing engine, + free             |
| Wireshark             | packet-level inspection             | L4 (proto/host/process) vs L2–L7 — different classes; ours integrates with routing |
| GlassWire (Windows)   | per-process traffic graph           | + cross-platform, + per-domain agg, + headless API                              |

Among VPN clients there is no counterpart: neither NekoBox, nor V2RayN, nor Clash Verge offers per-process attribution with CNAME-chain reconstruction and DNS-to-IP inferred matching.

---

## Subscription metadata — visibility into the provider's state

`SubscriptionMeta` (`core/state/connections.go`):

- `profile_title`, `support_url`, `profile_web_page_url`, `content_disposition_filename` — from the HTTP headers and the inline `#header:` on the body's first line (an LxBox-compatible contract)
- `UserInfo{UploadBytes, DownloadBytes, TotalBytes, ExpireUnix}` — the parsed `subscription-userinfo` (V2Board / Xboard formats)
- `LastStatus` (`ok`/`err`), `ErrorCount`, `LastErrorMsg`, `HTTPStatusCode`, `RawBodyBytes`
- `NodesCountFetched`, `Truncated` (when cut off at `max_nodes`), `PreviewNodes` (the first 50)
- `ProviderAnnounce` (SPEC 061) — providers send announce headers even on failure (HWID limit / quota exceeded / IP-bind violation) → the UI shows 📢 for success-with-announce and ⚠ for error-with-announce. The provider's URL is actionable in the UI

Per-subscription observability: "I have 23 GB left, valid until 2027-01-15, the last fetch failed on the HWID limit, and the provider said 'renew via https://...'".

---

# Technical capabilities

## Debug API — headless control plane

Files: `core/debugapi/server.go`, `state_endpoints.go`, `traffic_endpoints.go`, `snapshot.go`, `SPECS/038-F-C-DEBUG_API/SPEC.md`, `SPECS/050-F-N-DEBUG_API_STATE_MUTATIONS/SPEC.md`.

### The API surface — 28 paths (27 protected + the unauthenticated `/ping`) in 6 groups

| Group | What it covers |
| --- | --- |
| **Health & info** | an unauthenticated health check; the launcher / sing-box / API versions |
| **State (read)** | a snapshot of the current state, the active proxy, the selected group, the proxy list, the full state, the resolved outbounds |
| **State (write)** | rules / dns / dns-rules with replace and append modes, schema validation before the commit, a mutex per state path |
| **Actions** | start / stop / update-subs / ping-all / rebuild-config — synchronous triggers for every key action |
| **Traffic Profiler control** | start / stop / clear capture, live rolling-buffer snapshot, sessions list + export + drop, processes inventory, verbose log-level toggle |
| **Snapshot** | `/debug/snapshot` — template + state + cache + config in one JSON response |

The contract is versioned (`api: "debugapi/v1"`) and stays backwards compatible as it grows.

### The snapshot endpoint — a feature for the support workflow

`GET /debug/snapshot` (`core/snapshot/snapshot.go`) returns four files in a single HTTP call — template / state / cache / config — each as inline JSON, plus launcher_version / singbox_version / captured_at. Files absent from disk land in `Missing`; present but malformed JSON lands in `Errors`. The whole picture of a problem in one command: a bug report can be filed with `curl ... /debug/snapshot > snapshot.json` and the developer has the entire state.

The same `snapshot.Build()` is called by the "Copy snapshot" button on the Diagnostics tab — one source of truth.

### What makes it unique

No desktop client in this category offers a public, documented, scriptable HTTP API for driving the routing engine and the state.

### Use cases

- **Automation scripts**: "run update-subs against three different DNS configs and measure the latency" — plain bash + curl
- **MCP wrappers for AI agents**: Claude reads `/state` over MCP, switches proxies and restarts — without touching the UI (SPEC 038 §6.5)
- **Regression fixtures**: snapshot capture for reproducing problems
- **CI/CD**: validating new templates through `/action/rebuild-config`
- **Headless deployment**: start the launcher, script the setup, never open the UI

---

## Security aspects that were thought through

`core/debugapi/server.go`:

- binds strictly to `127.0.0.1` (no LAN, no 0.0.0.0)
- a bearer token with 32 bytes of entropy, `base64.RawURLEncoding` (43 characters)
- `crypto/subtle.ConstantTimeCompare` for the token check — protection against timing attacks over loopback
- mutating endpoints are POST/PATCH/DELETE only (protection against drive-by triggers from open web tabs)
- off by default; the token is generated on the first enable and preserved across OFF/ON
- the token never reaches the debug logs (`urlredact.RedactToken`)
- graceful shutdown with a 5s deadline

`core/state/`:

- `type: remote` rule_sets are forbidden in the final `config.json` (SPEC 045 phase 9) — otherwise a cold start tries to download through a VPN proxy that isn't up yet, which is fatal
- atomic file writes (SPEC 041): `stage → rename` with `.tmp`/`.swap` suffixes — protection against zeroing out `config.json` / `settings.json` on kill -9 or power loss
- the per-source raw body is written atomically via `.tmp + Rename`; a failure never overwrites the last working `.raw` (SPEC 052)

Privacy:

- Constitution §6.3 — telemetry and implicit collection are forbidden
- The HWID UUID is random, not derived from a system serial; the user can regenerate it (SPEC 061)
- Opt-outs for sending the HWID and for hashing the device model live in Settings

---

# Architectural decisions

## State-as-template-diff (SPEC 058)

`SPECS/058-R-N-STATE_AS_TEMPLATE_DIFF/SPEC.md` + `core/state/rule_types.go`. State stores thin references to the template plus the diff of the user's changes. A `Rule` carries a `Kind` (`preset|inline|srs`), a `Ref` (for presets), an `ID` (for user rules) and a `Body` (the kind-specific payload). For a preset, `PresetBody.Vars` maps only the vars the user changed relative to the template default.

A template bump (new domains in the block list, new TLDs) reaches the user without any action on their part. The configuration-overlay pattern.

## Auto-update + supervision (SPEC 052)

### Process supervision with a stability window

`core/process_service.go`. Parameters: 3 restart attempts, a 180-second stability window, graceful shutdown with a 2-second deadline.

The `Monitor()` loop:

1. The process exited → if the user asked for a restart, restart without touching the counter.
2. The crash-attempt counter exceeded 3 → stop and show the "restart_failed" dialog.
3. Otherwise increment it and recover after a delay.
4. After a successful restart the goroutine waits out the stability window; if the process survives that long without crashing, the counter resets. The stability-window pattern (as in systemd's `RestartSec` + `StartLimitBurst`, kubelet, supervisord). The UI shows `[restart 2/3]` in the banner.

### The pre-start config rebuild

`process_service.go`: before every `Start`, `RebuildConfigIfDirty()` runs — if a wizard Save raised the `CacheStale` / `ConfigStale` markers, config.json is rebuilt from state + raw cache + template. On error it is best-effort: log it and start with the old config. The SPEC 045 invariant: state.json is the source of truth, config.json is a derived view, and the rebuild detects a stale state.

### The per-source heartbeat (SPEC 052 phase 8)

`core/auto_update.go`. Parameters: a 1-hour heartbeat, a 15-second retry delay, a default 1-hour reload interval, and a 5-second anti-storm cooldown for event triggers.

The algorithm:

1. Heartbeat: walk the list of enabled subscriptions and check each one's last fetch time. If its age exceeds the effective reload interval (per-source → global → a 1h fallback), refresh that subscription. Fresh sources are skipped.
2. On failure, retry after a delay. Not recursive — if the retry fails, wait for the heartbeat or an event.
3. VPN-event trigger: a subscription to `VpnStateChanged` + `ProxyActiveChanged` → an immediate retry for failed sources (with the anti-storm cooldown).
4. A state-level mutex serializes the `load → mutate → save` cycles.
5. The power-resume callback refreshes overdue sources after a wake.

### Self-update

`core/core_version.go` + `auto_update.go`:

- the sing-box version is pinned through `constants.RequiredCoreVersion` (SPEC 046) — on a mismatch it is re-downloaded automatically
- the template ref is pinned through `constants.RequiredTemplateRef` — CI ldflags inject a specific commit's SHA, so the template is vouched for by git
- launcher self-update — checked once at startup, with a popup for a new version (no auto-installation — the user decides)

---

## Layered + DI

`core` (the engine, Fyne-free) → `core/services/` (Fyne-free services) → `core/uiservice/` (the Fyne-dependent wrapper) → `ui` (Fyne).

The architectural invariant is fixed in `SPECS/CONSTITUTION.md §1.5`:

- the UI never reaches core/network directly
- the parser is deterministic and free of side effects
- platform-specific code is isolated in `internal/platform`

A contract, not a habit.

## The parse-and-build pipeline

Subscription parsing → normalization (`core/config/configtypes/`) → a three-pass outbound generator with a topological sort of the selectors (`core/config/outbound_generator.go`) → preset resolution (`core/state/`, SPEC 057/058) → an atomic write of `config.json` (SPEC 041) → `sing-box check` validation before the commit → a supervised launch (`core/process_service.go`).

## Typed State Engine

`core/state/`: `state.go`, `connections.go`, `rule_types.go`, `dns_options.go`, `diff.go`, `migration_v5_to_v6.go`, `legacy_migration.go`, `raw_cache.go`, `disk_v6.go`, `adapter.go`, `provider_announce.go`, `ulid.go`.

A full domain model with discriminators (`SourceType`, `RuleKind`, `DNSSource`), a chain of migrations (v2 → v3 → v4 → v5 → v6) and diffs (SPEC 058).

## Typed EventBus (SPEC 047)

`core/events/`. A `MemoryBus` with `Subscribe(kind, handler) Cancel` and typed payloads (`StateChanged`, `ConfigBuilt`, `VpnStateChanged`). The contract: a panic in one handler does not break delivery to the others.

The SRE features (auto-update on a VPN event, SPEC 052) and observability (the tab icon subscribes to `VpnStateChanged`) are decoupled from core.

## Atomic file writes (SPEC 041)

`core/config_service.go`, `internal/locale/settings.go`, `core/state/save.go`. The `stage → rename` pattern with `.tmp`/`.swap` suffixes. On macOS/Linux a POSIX rename is atomic; on Windows it goes through `MoveFileEx` (Go 1.22+).

## Power-events

`internal/platform/`. A sleep/resume listener with an `IsSleeping()` contract that the tray timers, AutoLoadProxies, HTTP requests (PowerContext, ErrPlatformInterrupt) and the proxy list subscribe to. SPEC 011 fixed "launcher freeze after sleep": network requests no longer hang after a resume.

## The bin layout as an ABI

`bin/` has a stable layout:

- `config.json` — sing-box runtime config (derived)
- `wizard_template.json` — the community template
- `wizard_states/<name>.json` — named state snapshots
- `subscriptions/<source_id>.raw` — per-source raw body cache (SPEC 052)
- `rule-sets/*.srs` — cached SRS files
- `logs/sing-box.log[.old]` — engine logs

The contract: external tools (backup scripts, MCP servers, CI) know where to look.

---

# Anti-features (deliberately absent)

1. An editor for sing-box config.json. The config is a derived view of state + template; editing it by hand is possible but not a first-class supported path.
2. Subscription CRUD over the API (SPEC 038 §5). The desktop wizard covers it fully; duplicating it over HTTP is extra surface for bugs. Mobile LxBox offers full CRUD; on desktop it is trimmed deliberately.
3. A log-streaming endpoint in the Debug API. There is no in-memory log ring in `debuglog`, and no streaming / SSE / WebSocket.
4. Packet capture / pcap. sing-box works at L4, and the launcher works on L4 events.
5. TLS fingerprinting (JA3/JA4). sing-box does not expose it.
6. Persisting Traffic Profiler sessions across a restart. In-memory only (as in LxBox §044).
7. Auth roles. The bearer token is all-powerful. SPEC 050: "whoever holds the token can do everything".
8. Remote access to the Debug API. 127.0.0.1 only; any remote access goes explicitly through adb-forward or an ssh tunnel.
9. Telemetry / implicit collection. Constitution §6.3.
10. Bundling sing-box / wintun. For licence hygiene the launcher downloads both binaries itself, at a pinned version (SPEC 046).
11. A single settings file. State is split: settings.json (language, the debug API token, theme), state.json (the wizard), config.json (the sing-box runtime), wizard_states/ (named snapshots), subscriptions/*.raw (the per-source cache), rule-sets/*.srs (the SRS cache), logs/. The split is intentional.

---

# Key files for navigation

- `docs/ARCHITECTURE.md` — the project map
- `SPECS/CONSTITUTION.md` — the architectural invariants
