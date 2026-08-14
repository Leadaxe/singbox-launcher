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
# → {"launcher":"v1.2.2","singbox":"1.14.0-lx.5","api":"debugapi/v1"}
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
| GET | `/version` | ✓ | `{"launcher":"v…","singbox":"1.14.0-lx.5","api":"debugapi/v1"}` |

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
| GET | `/state/outbounds/resolved` | `{"outbounds": []OutboundConfig}` — merged after SPEC 057/058 expansion (template + preset patches + user overrides) |
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

Every patch endpoint returns `{"ok":true,"diff_summary":["..."]}` on success. The write is synchronous through `state.Save` → atomic `.tmp + Rename`; there is **no per-path mutex** (it relies on the atomic write — concurrent PATCHes are safe from partial writes, but it is last-write-wins).

| Method | Path | Body | What it does |
|---|---|---|---|
| PATCH | `/state/rules` | `{"mode":"replace"\|"append", "rules":[]state.Rule}` | Replaces / appends rules. Each is validated via `r.DecodeBody()` (kind discriminator: preset/inline/srs). |
| PATCH | `/state/dns` | `state.DNSOptions` | Replaces the **whole** dns_options (servers + rules). Every server/rule is validated by its `kind`. **The body must contain `servers` and/or `rules`** — a keyless `{}` → `422` (a guard against silently wiping the entire section); state is left untouched. |
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

```bash
# Save a full snapshot for a bug report
curl -s -H "Authorization: Bearer $TOKEN" "$API/debug/snapshot" > snapshot-$(date +%Y%m%d-%H%M%S).json
```

Response shape:
```json
{
  "captured_at": "2026-05-28T12:00:00Z",
  "launcher_version": "v1.2.2",
  "singbox_version": "1.14.0-lx.5",
  "files": { "state.json": "...", "config.json": "...", "wizard_template.json": "..." },
  "missing": ["cache.json"],
  "errors": { "config.json": "read: permission denied" }
}
```

`missing` is an array, `errors` an object of `{file: message}`; empty fields are omitted entirely (omitempty).

---

## General rules

- **Auth header:** `Authorization: Bearer <token>` is required everywhere except `GET /ping`.
- **Content-Type:** `application/json` for every PATCH/POST that carries a body.
- **Errors:** `401` — missing/invalid bearer; `404` — resource not found; `405` — method not allowed; `409` — state conflict (traffic session); `422` — semantic validation failure; `500` — internal error.
- **Concurrency:** state writes go through an atomic `.tmp + Rename`; there is no per-resource mutex — concurrent PATCHes are safe from partial writes, but it is **last-write-wins**, not a merge.
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
| `core/debugapi/log_level_endpoint.go` | `/state/log-level` (level validation + core restart via `core.ApplyLogLevelAndReloadCore`) |
| `core/debugapi/traffic_endpoints.go` | All of `/traffic/*` |
| `core/debugapi/snapshot.go` | `/debug/snapshot` |
| `core/debugapi_wiring.go` | The bridge between Server and the controller (StartSingBox, StopSingBox, Update, Rebuild, PingAll) |
| `internal/locale/settings.go` | `debug_api_enabled`, `debug_api_port`, `debug_api_token` |
| `ui/settings_tab.go` | UI toggle / Copy token / port entry |

Design history (optional reading): [SPEC 038](../SPECS/038-F-C-DEBUG_API/SPEC.md), [IMPLEMENTATION_REPORT](../SPECS/038-F-C-DEBUG_API/IMPLEMENTATION_REPORT.md).
