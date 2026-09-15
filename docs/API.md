# Debug API

**🌐 Language**: English | [Русский](API.ru.md)

A local HTTP API on `127.0.0.1`, bearer-auth, off by default. **Self-describing** (SPEC 078): `GET /` returns a manifest and `GET /help` the endpoint list, so an agent only needs the base URL and a token. Groups: discovery/info, state read, state write, actions, traffic profiler, snapshot. Used for automation (bash + curl), MCP wrappers for AI agents, CI/CD template validation, headless deployment, and capturing a full snapshot for a bug report (`/debug/snapshot`).

> Source of truth: the code in `core/debugapi/`. This document is a generated-style summary of the real handlers; SPEC 038 describes the original design and remains as a historical reference.

---

## TL;DR

```bash
# 1. Enable it in the UI: Settings → Debug API (localhost) → ✓
# 2. Copy the token: same screen, the "Copy token" button
# 3. Put it in the environment
export TOKEN="<paste-here>"
export API="http://127.0.0.1:9263"

# 4. Check
curl -s "$API/ping"                                    # → {"ok":true}    (no auth)
curl -s -H "Authorization: Bearer $TOKEN" "$API/version"
# → {"launcher":"v1.2.2","singbox":"1.14.0-lx.27-rc.6","api":"debugapi/v1"}
```

---

## Connecting

| What | Where |
|---|---|
| Bind | `127.0.0.1:<port>` — **hard-coded loopback**, cannot be moved onto the LAN |
| Default port | **9263** |
| Port override | `bin/settings.json` → `debug_api_port` (1024–65535, `0` = default) |
| Enable/disable | `bin/settings.json` → `debug_api_enabled` (UI: Settings → checkbox) |
| Bearer token | `bin/settings.json` → `debug_api_token` (UI: Settings → Debug API → Copy token) |
| Token regeneration | UI: **Settings → Debug API → "Regenerate"** (with a confirmation; rotates the token and restarts the listener). The alternative is deleting the key from `settings.json` and restarting the launcher |
| Comparison | `subtle.ConstantTimeCompare` (constant-time) |
| Header | `Authorization: Bearer <token>` |

The address is shown in Settings → Debug API next to the checkbox — a ready-to-copy `127.0.0.1:<port>` string.

---

## Discovery & info

The API is **self-describing** (SPEC 078): point an agent at the base URL with the token and it can read the surface itself.

| Method | Path | Auth | Response |
|---|---|---|---|
| GET | `/ping` | — | `{"ok":true}` |
| GET | `/` | ✓ | **Manifest** — `api`, `spec`, `launcher`, `core`, `auth`, `docs` (version-pinned link to this file), `hint`, `endpoints[]` (method/path/summary). |
| GET | `/help` | ✓ | `{"endpoints":[{method,path,summary,auth}, …]}` — just the endpoint list. |
| GET | `/version` | ✓ | `{"launcher":"v…","singbox":"1.14.0-lx.27-rc.6","api":"debugapi/v1"}` |

An authed request to any **unknown** path returns `404` with a `docs` pointer, so an agent that guessed wrong is nudged back to `/` and this file.

The Settings → Debug API screen has a **Copy API info** button that puts a *connection card* JSON on the clipboard (`base_url`, `token`, `launcher`, `core`, `auth`, `docs`, `hint`) — hand it to an agent and it has everything to connect from scratch.

```bash
curl -s "$API/ping"
curl -s -H "Authorization: Bearer $TOKEN" "$API/"       # manifest
curl -s -H "Authorization: Bearer $TOKEN" "$API/help"   # endpoint list
curl -s -H "Authorization: Bearer $TOKEN" "$API/version"
```

---

## State read

| Method | Path | Purpose |
|---|---|---|
| GET | `/state` | Live runtime snapshot: `{running, active_proxy, selected_group, singbox_version, subs_last_updated_unix}` |
| GET | `/proxies` | Proxy list (`[]api.ProxyInfo`) — from the current sing-box config |
| GET | `/state/full` | The whole `state.json` (after load + migrations) |
| GET | `/state/rules` | `{"rules":[]state.Rule}` — the SPEC 053 section |
| GET | `/state/dns` | The whole `state.DNSOptions` section (SPEC 056) |
| GET | `/state/dns/rules` | `{"text":"..."}` — **USER rules only**, as wizard text. Preset rules are excluded (they are toggle refs) |
| GET | `/state/outbounds/resolved` | `{"outbounds": []Direction}` — merged after SPEC 057/058 expansion (template + preset patches + user overrides); SPEC 104 fields `label`/`disabled`/`auto` included |
| GET | `/state/log-level` | `{level, is_set, default, effective, allowed}` — `level` is the raw `vars[log_level]` (`""` when unset), `effective` is what sing-box will actually use (when empty — `default`, i.e. `warn`) |

```bash
# What is selected right now
curl -s -H "Authorization: Bearer $TOKEN" "$API/state" | jq

# The full configuration
curl -s -H "Authorization: Bearer $TOKEN" "$API/state/full" > backup.json
```

**Errors:** `401` (no/bad bearer), `404` (state.json does not exist — fresh install), `500` (load/parse error).

---

## State write

Every patch endpoint returns `{"ok":true,"diff_summary":["..."]}` on success. The write is synchronous through `state.Save` → atomic `.tmp + Rename`; the whole load-modify-save cycle is **serialized by a mutex** (`stateMu` for `/state/*`, `settingsMu` for `/settings/*`), so two concurrent PATCHes cannot lose one side's edit. Remote machines get a **per-machine** mutex — patching two different machines does not queue.

| Method | Path | Body | What it does |
|---|---|---|---|
| PATCH | `/state/rules` | `{"mode":"replace"\|"append", "rules":[]state.Rule}` | Replaces / appends rules. Each is validated via `r.DecodeBody()` (kind discriminator: preset/inline/srs). |
| PATCH | `/state/dns` | `state.DNSOptions` | Replaces the **whole** `dns` section (servers + rules; state v8 — до v8 ключ назывался `dns_options`). Every server/rule is validated by its `kind`. **The body must contain `servers` and/or `rules`** — a keyless `{}` → `422` (a guard against silently wiping the entire section); state is left untouched. |
| PATCH | `/state/dns/rules` | `{"text":"..."}` | Replaces **USER rules only**; preset rules are preserved. `""` (empty text) wipes the user rules. |
| PATCH | `/state/log-level` | `{"level":"trace"\|"debug"\|"info"\|"warn"\|"error"\|"fatal"\|"panic"}` | Writes `vars[log_level]` → forces a `config.json` rebuild → **restarts sing-box** (active connections are dropped). Responds `202` + `{"ok":true,"level":"...","warning":"active connections reset"}` rather than the generic `{"ok":true,"diff_summary":[...]}`. The `level` field is required; an invalid level → `400` with the `allowed` list (the core is left alone). |

```bash
# Replace all rules with a single preset ref
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/state/rules" \
  -d '{"mode":"replace","rules":[{"kind":"preset","ref":"ru-direct","enabled":true,"body":{"vars":{}}}]}'

# Append one inline rule without touching the rest
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/state/rules" \
  -d '{"mode":"append","rules":[{"kind":"inline","enabled":true,
        "body":{"name":"Block Reddit","match":{"domain_suffix":["reddit.com"]},"outbound":"reject"}}]}'

# Patch the DNS rules text (same as the UI's Raw mode)
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/state/dns/rules" \
  -d '{"text":"{\"rules\":[{\"domain\":\"example.com\",\"server\":\"cf\"}]}"}'

# Raise logging to trace (drops active connections — the core restarts)
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/state/log-level" -d '{"level":"trace"}'
```

> `POST /traffic/verbose` is the boolean special case of the same handler: it only knows `debug` (`true`) and `warn` (`false`). For any other level use `PATCH /state/log-level`.

**Errors:** `400` (malformed JSON / unknown mode), `422` (semantic validation: unknown rule kind, unknown DNS server kind, body decode failure), `500` (load/save), `405` (method).

---

## Settings

`bin/settings.json` holds launcher-level preferences (a namespace separate from `state.json`). Changes are picked up on the fly: the subscription fetcher reads `LoadSubscriptionSettingsFunc` on every request, so a sing-box restart is NOT needed.

| Method | Path | What it does |
|---|---|---|
| GET | `/settings/user-agent` | `{user_agent, default, effective}` — `user_agent` as stored (may be empty), `default` is what `BuildSubscriptionUserAgent()` returns, `effective` is what the next fetch will actually send |
| PATCH | `/settings/user-agent` | `{"user_agent":"..."}` — store a custom UA. `{"user_agent":""}` resets to the default. The field is required (omitting it → `400`) — otherwise a truncated request could wipe the value by accident |

```bash
# Read the current value + default + effective
curl -s -H "Authorization: Bearer $TOKEN" "$API/settings/user-agent" | jq

# Set the UA to v2rayN (for providers that reject our default)
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/settings/user-agent" \
  -d '{"user_agent":"v2rayN/7.5.0"}'

# Reset to the default
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/settings/user-agent" \
  -d '{"user_agent":""}'
```

**Errors:** `400` (malformed JSON / missing `user_agent` field), `500` (saving settings.json), `405` (method).

---

## Backup / transfer (SPEC 127)

The same thing the **Files** tab does with its *Export…* / *Import…* buttons: take a portable snapshot of the settings and apply one somewhere else. The payload is the LX Backup file itself — save the response to disk and the launcher or LxBox opens it unchanged.

Export writes **format 1.0** only: the file is the launcher state, so per-node rules, DNS sections and folders travel too. LxBox reads and writes the same format starting with 2.24.0, released together with launcher 1.6.0. **Import reads both 1.0 and the older 0.x files** (0.12 and before), and the caller never states the format.

| Method | Path | Purpose |
|---|---|---|
| GET | `/backup/formats` | `{"reads":[1,2],"writes":["1.0"],"default":"1.0"}` — `reads` are the `lx_backup` markers import understands, `writes` the format names `?format=` accepts |
| GET | `/backup/export` | The backup file (format 1.0) as the response body. Directions carry their merged body — template or preset plus patches, the same as `/state/outbounds/resolved`. `?format=` may be omitted or `1.0`; `?format=0.12` answers `400` (`format 0.12 is no longer written; import still reads it`). `?envelope=1` wraps the file as `{format, file_name, file, warnings}` |
| POST | `/backup/import` | Body = a backup file of format 1.0 or 0.x. Merges it into the state, saves, then rebuilds `config.json` |

Export losses are never silent: without the envelope the codes travel in the `X-Backup-Warnings` header as a JSON array; with `?envelope=1` they are the `warnings` field. The plain response also carries `Content-Disposition` with the same suggested filename the UI offers.

`POST /backup/import` **merges** — it does not replace (BACKUP.md §9): subscriptions match by URL, servers by what they connect to, folders by name, chains and Directions by tag. Routing rules are the one exception — the file replaces them wholly. The response reports what actually landed:

```json
{"ok":true,"format":"1.0","warnings":[{"code":"backup_unknown_outbound","detail":"Work → vpn-de"}],
 "applied":{"rules":7,"sources":4,"directions":1,"added_subscriptions":1,"updated_subscriptions":0,
            "added_servers":2,"skipped_servers":0,"added_folders":1,"updated_folders":0,
            "added_chains":1},
 "config_rebuilt":true}
```

```bash
# Snapshot this machine
curl -s -H "Authorization: Bearer $TOKEN" "$API/backup/export" -o lx-backup.json

# Codes of anything the format could not carry
curl -sD- -o /dev/null -H "Authorization: Bearer $TOKEN" "$API/backup/export" | grep -i x-backup-warnings

# Apply it on the other machine
curl -s -X POST -H "Authorization: Bearer $TOKEN" --data-binary @lx-backup.json "$API/backup/import" | jq
```

Machines paired through `/remote/*` mirror export and import: `GET /remote/machines/{id}/backup/export` (the same 1.0-only rule for `?format=`) and `POST /remote/machines/{id}/backup/import` act on that machine's wizard profile. The known SPEC 100 §3.3 limitation applies — the machine's `config.json` is rebuilt by its own wizard, so a remote import returns `config_rebuilt:false` and the deploy still needs the Save step in the UI.

On a **fresh install** (no `state.json` yet) import still works: the file describes the whole setting, so it is merged into a clean state and saved. Export in the same situation answers `404` — there is nothing to snapshot, and an empty file would misreport the machine.

**Errors:** `400` (`?format=` other than `1.0`, empty body, not an LX Backup file, `lx_backup` newer than this build reads), `409` (the state file is written by a different schema major — SPEC 118 gate, the same as a `PATCH /state/*`), `422` (the file parsed but could not be merged), `404` (export only: no `state.json`), `500` (export only: the template could not be read — without it a Direction that refers to the template has no body to write), `405` (method).

---

## Actions

All are `POST`-only (`GET` → 405) and synchronous (they block until done). Success = `{"ok":true}`.

| Method | Path | What it does |
|---|---|---|
| POST | `/action/update-subs` | `ConfigService.UpdateConfigFromSubscriptions` — a synchronous re-fetch of every subscription |
| POST | `/action/start` | Starts sing-box (fire-and-forget) |
| POST | `/action/stop` | Stops sing-box (graceful, 2s deadline) |
| POST | `/action/ping-all` | Latency-tests every proxy. **Caveat:** a silent no-op when UIService is not initialized (a headless edge case) |
| POST | `/action/rebuild-config` | `RebuildConfigIfDirty` — rebuilds `config.json` when stale markers are present. Atomic `.tmp + Rename`. **Note:** the doc comment in the code promises `{"rebuilt":bool}` in the response, but the handler returns only `{"ok":true}` (pending) |

```bash
# Refresh subscriptions and rebuild the config
curl -s -X POST -H "Authorization: Bearer $TOKEN" "$API/action/update-subs"
curl -s -X POST -H "Authorization: Bearer $TOKEN" "$API/action/rebuild-config"

# Restart sing-box
curl -s -X POST -H "Authorization: Bearer $TOKEN" "$API/action/stop"
curl -s -X POST -H "Authorization: Bearer $TOKEN" "$API/action/start"
```

---

## Traffic Profiler (SPEC 059)

Control over the live DNS/TCP/UDP capture session and a view into the rolling buffer (the last 60 seconds; the `last` parameter is clamped to 10 minutes). The same subsystem as the **Traffic Profiler** window in Diagnostics.

| Method | Path | Purpose |
|---|---|---|
| GET | `/traffic/status` | State of the active session (recording, target, events_dropped, etc.) |
| GET | `/traffic/live?last=60s` | A snapshot of the rolling buffer. `last` is a Go duration (≤ 10 minutes, > 0). Returns `{events, cutoff_ts}` |
| POST | `/traffic/start` | Body `{"target":"<process_path>","verbose":<bool>}`. An empty target means system-wide. Verbose flips `log_level=debug` and restarts sing-box. **409** if a session is already active |
| POST | `/traffic/stop` | Finalizes the active session. **404** when there is none |
| POST | `/traffic/clear` | Wipes every completed session. Returns `{"cleared":N}` |
| GET | `/traffic/sessions` | Every session (completed + the active one, flagged `active:true`) |
| GET | `/traffic/sessions/{id}` | A full event dump for the session |
| DELETE | `/traffic/sessions/{id}` | Delete one. **409** if that session is active |
| GET | `/traffic/processes` | The distinct processes in the rolling buffer (for the UI dropdown) |
| GET | `/traffic/verbose` | The current sing-box `log_level` |
| POST | `/traffic/verbose` | Body `{"enabled":<bool>}`. Toggles `log_level=debug/warn`. **202 Accepted** (needs a sing-box reload); response: `{"ok":true,"level":"debug","warning":"active connections reset"}` |

```bash
# Record everything Firefox does for 10 seconds
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/traffic/start" -d '{"target":"/Applications/Firefox.app/Contents/MacOS/firefox","verbose":true}'
sleep 10
curl -s -X POST -H "Authorization: Bearer $TOKEN" "$API/traffic/stop" | jq .session.id
# → "01J…"

# Fetch the full session log
curl -s -H "Authorization: Bearer $TOKEN" "$API/traffic/sessions/01J…" > firefox_session.json

# A live snapshot of the last 30 seconds (without recording)
curl -s -H "Authorization: Bearer $TOKEN" "$API/traffic/live?last=30s" | jq '.events | length'
```

---

## Snapshot

| Method | Path | Purpose |
|---|---|---|
| GET | `/debug/snapshot` | `core.snapshot.Build()` — template + state + cache + config.json in a single JSON. Ideal for a bug report |
| GET | `/debug/goroutines` | `runtime.Stack(all)` — stack dump of every goroutine as `text/plain`, the same text Go prints on SIGQUIT, without stopping the process. Header `X-Goroutines` carries the count. For a frozen UI: `goroutine 1` is the Fyne/GLFW main loop |
| GET | `/debug/ui` | Fyne windows: canvas size, content type, focused widget and the **overlay stack** of each window (type, position, size, children). Capability `ui`. Fyne routes every click to the top overlay only, so a stale overlay makes a window ignore input while the process stays alive: this is where you see it |
| POST | `/debug/ui/overlays/clear` | Remove every canvas overlay in every window — unfreezes a window blocked by a stale overlay without restarting. Also closes any open dialog/popup. `504 ui loop unresponsive` means the Fyne main loop itself is blocked |

```bash
# Save a full snapshot for a bug report
curl -s -H "Authorization: Bearer $TOKEN" "$API/debug/snapshot" > snapshot-$(date +%Y%m%d-%H%M%S).json

# UI hung but the process is alive: dump goroutines, look at what goroutine 1 is waiting on
curl -s -H "Authorization: Bearer $TOKEN" "$API/debug/goroutines" > goroutines-$(date +%Y%m%d-%H%M%S).txt

# Window ignores clicks but the process is alive: is an overlay stuck on the canvas?
curl -s -H "Authorization: Bearer $TOKEN" "$API/debug/ui" | jq '.windows[] | {title, focused, overlays}'
# Yes → drop it and get the window back without a restart
curl -s -X POST -H "Authorization: Bearer $TOKEN" "$API/debug/ui/overlays/clear"
```

Response shape:
```json
{
  "captured_at": "2026-05-28T12:00:00Z",
  "launcher_version": "v1.2.2",
  "singbox_version": "1.14.0-lx.27-rc.6",
  "files": { "state.json": "...", "config.json": "...", "wizard_template.json": "..." },
  "missing": ["cache.json"],
  "errors": { "config.json": "read: permission denied" }
}
```

`missing` is an array, `errors` an object of `{file: message}`; empty fields are omitted entirely (omitempty).

---

## Remote machines (SPEC 100)

Full API wrapper over the remote lxd-machines registry (SPEC 096–099). Every
call addresses a machine explicitly — `/remote/machines/{id}/…`; there is no
"active machine" notion in the API. The `GET /` manifest carries
`capabilities` (`remote`/`daemon`/`raw_grpc`) so an agent knows up front which
groups this build exposes (Win7 builds ship without the remote group,
non-macOS without `/daemon/*`).

**Registry:**

| Method | Path | What it does |
|---|---|---|
| GET/POST | `/remote/machines` | List / pair `{invite, name?, addr?, secret?}` (invite is `addr#fingerprint#code`) |
| GET/PATCH/DELETE | `/remote/machines/{id}` | Get / update `{name?,addr?,goos?,goarch?}` / remove (response warns: access is NOT revoked on the daemon side) |
| POST | `/remote/machines/{id}/repair` | Re-pair `{invite, addr?, secret?}` with a fresh client key; the machine's profile is kept |
| POST | `/remote/machines/{id}/profile/copy-from` | Copy wizard profile `{source_id, overwrite?}`; existing state without `overwrite=true` → `409` |

**Core & deploy:**

| Method | Path | What it does |
|---|---|---|
| GET | `/remote/machines/{id}/health` | `{reachable, core_status, active_sha, last_good_sha, …}` — comparing SHAs is the honest "did it land" check |
| POST | `/remote/machines/{id}/core/start` \| `stop` \| `rollback` | Core control (stop drops the VPN of the machine's clients — the API does not ask for confirmation) |
| GET | `/remote/machines/{id}/config/active` \| `built` | Running config fetched from the machine / locally built one |
| POST | `/remote/machines/{id}/deploy` | Resources → config (the same chain as the Deploy button). Optional body `{config:{…}}`. `422` = daemon rejected the config, running instance untouched |

**State (mirrors of `/state/*`):** `GET /remote/machines/{id}/state/full`,
`GET/PATCH …/state/rules`, `…/state/dns`, `…/state/dns/rules`,
`GET …/state/outbounds/resolved` — same contracts as the local endpoints.
**Limitation:** PATCH updates the machine's state, but its `config.json` is
still built only by the wizard (Configure → Save) — no programmatic rebuild yet.

**Backup (mirrors of `/backup/*`):** `GET /remote/machines/{id}/backup/export`,
`POST /remote/machines/{id}/backup/import` — the same contracts as the local
endpoints, acting on that machine's profile. The rebuild limitation above
applies: a remote import answers `config_rebuilt:false`.

**Observability:** `GET …/groups`, `GET …/proxies?group=`,
`POST …/proxies/switch {group,name}`, `POST …/proxies/delay {name}`,
`GET …/pool?group=`, `GET …/rules`, `GET …/outbounds`, `GET …/status`,
`GET/DELETE …/connections`, `DELETE …/connections/{conn_id}`,
`GET …/dns/queries?duration=5s&max=200`, `GET …/logs?duration=&max=`,
`GET …/host`, `GET …/host/interfaces`, `GET …/clients`,
`PUT/DELETE …/clients/{key}/label`. Stream sources are served as windows
(`duration` ≤ 60s, `max` ≤ 5000) — no SSE subscriptions in v1.

**Resource store:** `GET …/resources` (local vs machine overview),
`POST …/resources/sync`, `GET/PUT/DELETE …/resources/{name}`,
`POST …/resources/{name}/download`. `409` = the name is referenced by a live
config.

**UI-override (the Remote tab's Connect/Disconnect buttons):** regular remote
calls never touch the UI selection — these three endpoints control which
machine the launcher's Servers tab is pointed at.

| Method | Path | What it does |
|---|---|---|
| GET | `/remote/ui` | `{connected, machine_id, machine_name}`; `connected:false` = local core |
| POST | `/remote/machines/{id}/ui/connect` | Point the Servers tab at the machine. Health gate before switching: unreachable machine → `502`, override untouched. An idle core is not a failure (the response carries a `warning`) |
| POST | `/remote/ui/disconnect` | Return the Servers tab to the local core. Idempotent |

`503` on all three — the launcher runs headless or the UI is not created yet.

**Group errors:** `404` unknown machine / no built config; `409` conflict;
`422` config rejected by the daemon; `502` machine unreachable; `504` call
timeout.

```bash
# Pair with a router and list its nodes
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/remote/machines" -d '{"invite":"192.168.10.1:19091#3f9c…#Q7PLM2","name":"RouteRich"}'
curl -s -H "Authorization: Bearer $TOKEN" "$API/remote/machines/routerich/proxies?group=proxy-out" | jq

# Deploy and verify it landed
curl -s -X POST -H "Authorization: Bearer $TOKEN" "$API/remote/machines/routerich/deploy" | jq .config_sha
curl -s -H "Authorization: Bearer $TOKEN" "$API/remote/machines/routerich/health" | jq .active_sha
```

---

## Local daemon `/daemon/*` (macOS)

The group exists only in darwin builds (see `capabilities.daemon`). Core
start/stop under the daemon engine goes through the shared
`/action/start|stop` (the `CoreBackend` seam) — no dedicated endpoints.

| Method | Path | What it does |
|---|---|---|
| GET | `/daemon/status` | Pairing, service, reachability, core status, daemon passport |
| POST | `/daemon/pair` | `{invite, secret?}` — pair with the local daemon |
| POST | `/daemon/unpair` | Forget the pairing (keys, pin, secret) |
| PATCH | `/daemon/settings` | `{addr?, secret?}` |
| GET/POST | `/daemon/engine` | Core engine: `{"mode":"classic"\|"daemon"}`; POST while the VPN runs → `409` |
| GET | `/daemon/commands` | Ready-to-run sudo commands (install/uninstall/repair/kickstart/show_secret). **The API never executes them** — the "sudo only in your terminal" principle |

---

## Chains `/chains/*` (SPEC 110)

Gated exactly like `/daemon/*` — the group is registered only when the local
daemon is wired in (`capabilities.daemon`), because the layered probe rides the
daemon's gRPC plane.

| Method | Path | What it does |
|---|---|---|
| GET | `/chains` | Chain runtime state: positions, the node each one currently resolves to, clone status |
| POST | `/chains/{tag}/probe` | Layer-by-layer latency probe of one chain |

**Probe body** — `{repeat?, timeout_ms?, link?}`. Defaults and clamps:
`repeat` = 2 (max 10), `timeout_ms` = 15000 (max 120000), `link` falls back to
the UI's own ping URL so debug numbers stay comparable with the Servers tab.

`repeat` defaults to **2, not 1**: the first run brings the tunnels up (WG
handshake, QUIC session) and is inflated several-fold. The response marks it
`warm_up` rather than hiding it, so the discrepancy is explained instead of
looking like jitter. `timeout_ms` is **per position**, not for the whole run —
the point is to let a slow hop answer and show its real cost instead of being
cut off together with the rest.

**Response** carries `runs[]` (each with `layers[]`: `pos`, `tag`, `probe_tag`,
`transparent`, `delay_ms`, `error`), the per-hop `deltas[]`, and `worst`.

> **`probe_tag` is authoritative — do not reconstruct it.** A chain reserves the
> service tags `<chain>#<i>` for its own links (`config.ChainLayerTag`): `T#0` is
> the path up to and including position 0, `T#1` up to position 1, and so on.
> These exist only inside the running core — they are deliberately absent from
> `GetOutbounds` and the Clash API. Chain tags contain emoji and hashes, so the
> response hands you the exact string rather than a naming rule to re-implement.

**Status codes** map the core's gRPC errors: `501` — the core was built without
`with_lx_command` (the single most common cause), `502` Unavailable, `504`
DeadlineExceeded, `422` FailedPrecondition / InvalidArgument.

Layered probing also works for a **remote** machine
(`LxdRemoteTransport.ProbeLayer`) — measured **on the router's side**, not from
this host, so the numbers describe the router's channel to each hop.

---

## Raw passthrough

A tunnel to a **paired** daemon — a remote machine or the local one. Channel,
pin and credentials come from the registry/settings; these endpoints cannot
reach arbitrary addresses.

| Method | Path |
|---|---|
| POST | `/remote/machines/{id}/raw/rest` \| `/daemon/raw/rest` |
| POST | `/remote/machines/{id}/raw/grpc` \| `/daemon/raw/grpc` |
| GET | `/grpc/methods` — discovery of all `daemon.*` methods (kind, input, output) |

**REST:** body `{"method":"GET","path":"/admin/status","body":{…}|"body_base64":"…","content_type":"…"}`.
`path` must start with `/`; `body` and `body_base64` are mutually exclusive.
Response is `{"status":<daemon code>,"content_type":…,"body":{…}|"body_base64":"…"}`;
our HTTP status is always 200 — the daemon's status is data (otherwise you
could not tell "our" 404 from the daemon's 404).

**gRPC:** body `{"method":"/daemon.StartedService/URLTest","request":{…},"timeout":"15s","duration":"5s","max_events":100}`.
The method is resolved by name via protoregistry (no hand-written table — new
RPCs are picked up automatically after `internal/daemonpb` updates), JSON ↔
proto via protojson. Unary → `{"response":{…}}`; server-stream → a window
`{"events":[…],"truncated":bool}`; client/bidi streams → `501`.

```bash
# Any admin-REST request to a machine
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/remote/machines/routerich/raw/rest" -d '{"method":"GET","path":"/admin/info"}' | jq .body

# Any gRPC: URL-test a node on the machine's side
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/remote/machines/routerich/raw/grpc" \
  -d '{"method":"/daemon.StartedService/URLTestOutbound","request":{"outbound_tag":"JP-01","link":"https://cp.cloudflare.com","timeout":10000}}' | jq

# Core-log window over the stream
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
  "$API/remote/machines/routerich/raw/grpc" \
  -d '{"method":"/daemon.StartedService/SubscribeLog","duration":"3s","max_events":200}' | jq '.events | length'
```

---

## General rules

- **Auth header:** `Authorization: Bearer <token>` is required everywhere except `GET /ping`.
- **Content-Type:** `application/json` for every PATCH/POST that carries a body.
- **Errors:** `401` — missing/invalid bearer; `404` — resource not found; `405` — method not allowed; `409` — state conflict (traffic session); `422` — semantic validation failure; `500` — internal error.
- **Concurrency:** state writes go through an atomic `.tmp + Rename`, and the load-modify-save cycle is serialized by a mutex (`stateMu` / `settingsMu`; per-machine for remote state). Concurrent PATCHes to the same resource queue rather than overwrite each other.
- **Versioning:** the `api` field in `/version` is currently fixed at `debugapi/v1`. Breaking changes are planned as a `v2` namespace (`/v2/...`), with no auto-discovery for now.

---

## Use cases

- **Bash + curl scripts** — a health check in a systemd unit, a periodic subscription refresh from cron, asserting `running=true` after a deploy.
- **MCP wrappers for AI agents** — Claude / GPT / others can read `/state/full`, issue PATCHes, and trigger a rebuild. See [SPEC 038 §6.5](../SPECS/038-F-C-DEBUG_API/SPEC.md).
- **CI/CD template validation** — drop in a `wizard_template.json`, run the launcher headless, PATCH the state through the API, wait for the rebuild, read the generated `config.json`, run sing-box check over it.
- **Regression fixtures** — capture `/debug/snapshot` before and after a change and diff them.
- **Live observability** — `/traffic/live?last=10s` + `jq` is a realtime tail of connections without opening the UI.

---

## Limitations

- **Loopback only.** No TLS, no CORS, no LAN bind. For remote access use an ssh tunnel: `ssh -L 9263:127.0.0.1:9263 user@host`.
- **No streaming endpoints** (WebSocket / SSE). `/traffic/live?last=...` is a snapshot, not a subscription. For long-tail polling, take the rolling buffer in chunks.
- **No `GET /logs?tail=N`** — read the sing-box logs straight from `bin/logs/`.
- **No switch_proxy / list_groups / get_logs** — mentioned in SPEC 038 §183 as future work; not implemented.
- **Toggling verbose** restarts sing-box — active TCP connections are dropped. The response says so (`"warning":"active connections reset"`).
- **Token rotation** — the **Settings → Debug API → "Regenerate"** button (with a confirmation; rotates the token and restarts the listener). Without the UI: stop the launcher → delete `debug_api_token` from `bin/settings.json` → start the launcher → the token is regenerated on first enable.

---

## Source

| File | What's inside |
|---|---|
| `core/debugapi/server.go` | Routing, auth middleware, `/ping`, `/version`, `/state`, `/proxies`, `/action/*` |
| `core/debugapi/state_endpoints.go` | `/state/full`, `/state/rules`, `/state/dns`, `/state/dns/rules`, `/state/outbounds/resolved` |
| `core/debugapi/backup_endpoints.go` | `/backup/export`, `/backup/import`, `/backup/formats` and their `/remote/machines/{id}/backup/*` mirrors |
| `core/debugapi/log_level_endpoint.go` | `/state/log-level` (level validation + core restart via `core.ApplyLogLevelAndReloadCore`) |
| `core/debugapi/traffic_endpoints.go` | All of `/traffic/*` |
| `core/debugapi/snapshot.go` | `/debug/snapshot` |
| `core/debugapi/goroutines.go` | `/debug/goroutines` |
| `core/debugapi/ui_endpoints.go` + `core/debugapi_ui.go` | `/debug/ui`, `/debug/ui/overlays/clear` (Fyne inspector lives in core; wired via `EnableUI`) |
| `core/debugapi_wiring.go` | The bridge between Server and the controller (StartSingBox, StopSingBox, Update, Rebuild, PingAll) |
| `internal/locale/settings.go` | `debug_api_enabled`, `debug_api_port`, `debug_api_token` |
| `ui/settings_tab.go` | UI toggle / Copy token / port entry |

Design history (optional reading): [SPEC 038](../SPECS/038-F-C-DEBUG_API/SPEC.md), [IMPLEMENTATION_REPORT](../SPECS/038-F-C-DEBUG_API/IMPLEMENTATION_REPORT.md).
