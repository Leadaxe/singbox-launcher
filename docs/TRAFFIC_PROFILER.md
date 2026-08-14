# Traffic Profiler — user guide

**🌐 Language**: English | [Русский](TRAFFIC_PROFILER.ru.md)

SPEC: [SPECS/059-F-N-TRAFFIC_PROFILER/SPEC.md](../SPECS/059-F-N-TRAFFIC_PROFILER/SPEC.md) ·
remote machines: [SPECS/099-F-C-REMOTE_TRAFFIC_PROFILER/SPEC.md](../SPECS/099-F-C-REMOTE_TRAFFIC_PROFILER/SPEC.md)

The Traffic Profiler is a live diagnostic tool that shows what your apps are actually doing on the network, who they're talking to, through which outbound, and whether the connection succeeded. Use it when "Slack works but Telegram doesn't go through VPN" or when you want a privacy audit of a closed application.

It is a **separate window** opened from the Diagnostics tab — not a configurator tab — so you can keep it side-by-side with the application you're investigating.

## When to use it

- **"Why does X bypass my VPN?"** — open the Profiler, start a recording targeting X, watch which outbound shows up next to each connection.
- **"Why does Y fail to load?"** — switch on Verbose logs, open Y in the app, look for ⚠ DnsTimeout / ⚠ TcpRstEarly markers on the corresponding rows.
- **Privacy audit** — open the Live tab without any recording and watch what every process on your machine is connecting to.
- **Discovering what an app needs allow-listed** — record a quick session, look at the Domains sub-tab, copy the list.

## Opening the window

Diagnostics tab → **Traffic Profiler** button. The button shows a ⚡ badge whenever a recording session is active, even if the window itself is closed (recording continues in the background and you can re-open the window any time to see it).

Re-clicking the button focuses the existing window rather than spawning a duplicate.

## Where the data comes from

The profiler joins two streams by connection ID. Which streams those are depends on
the engine driving the core:

| Engine | Connections | DNS |
|---|---|---|
| **Classic** (default) | Clash API `/connections` poller | parsed out of the sing-box log tail — the only DNS source available, hence the Verbose-logs toggle below |
| **Daemon** (macOS, `sing-box lxd`) | gRPC `SubscribeConnections` | gRPC `SubscribeDNSQueries` — a **structured** stream, so DNS arrives complete without switching the core to debug |

The switch is automatic: when the backend changes, the profiler's sources are
re-installed and the stale subscription is dropped. In daemon mode the config
carries no Clash API at all, so the Clash poller is not merely unused — it is
switched off rather than left knocking on a port that isn't there.

## Live tab

System-wide stream of every DNS / TCP / UDP event sing-box sees. New events appear at the top.

- Filter chips (DNS / DNS× / TCP / TCP· / UDP) hide/show event kinds.
- Search box matches domain, IP, process path, process name.
- Each row carries the process name when sing-box was able to attribute it (`route.find_process: true` is on by default in the bundled template).

If you see most rows without a process tag, your template likely has process detection off — a banner inside the window tells you so. Re-run the Configurator wizard and Save; the new template has `route.find_process: true`.

## Per-process recording

1. Switch to the **Per-process** tab inside the window.
2. Click **Pick process & START** — a dialog lists running processes (processes already producing traffic float to the top).
3. Pick the target; the dialog closes and recording begins. The button changes to **STOP**.

Sub-tabs that appear during a recording:

| Sub-tab | Purpose |
|---|---|
| **Live** | Newest-first stream of events attributed to the target. Pre-session events from the last 60 seconds are pulled in and marked 〽 ("backfilled"). |
| **Domains** | Aggregated unique domains, sorted by total bytes. Each row shows connection count, IP count, total up/down bytes. |
| **IPs** | Aggregated destination IPs — useful for raw-TCP connections without SNI. |
| **Connections** | Per-connection timeline with rule + outbound. |

⚠ icon means at least one connection had a diagnostic issue (DnsTimeout or TcpRstEarly).

Click **STOP** to finalize. The session moves to the "Saved sessions" ring (last 5 kept, FIFO). Saved sessions can be re-opened as read-only views or deleted.

## Verbose logs toggle

Top-left of the window: **Verbose logs (debug)** checkbox.

- ON: switches sing-box `log.level` to `debug` so DNS chain reconstruction (CNAME → A) becomes complete. Battery / CPU impact is small but non-zero — a warning is shown in the toolbar.
- OFF: reverts to `warn`.

**WARNING**: changing the log level reloads sing-box. **Active TCP/UDP connections will be reset** — open VoIP calls, downloads, file uploads will all break. The toolbar shows a confirmation dialog before applying.

Recording continues across the reload — the active session keeps capturing with the new conn-id space.

## Overflow menu (⋮ button)

- **Copy session JSON** — copies the current (or most recent completed) session as JSON to the clipboard. Useful for pasting into a bug report.
- **Export session JSON…** — saves to a file. Suggested filename: `traffic-<process>-<timestamp>.json`.
- **Clear completed sessions** — wipes the saved-sessions ring. Active session is preserved.
- **Help / about** — short summary.

## Profiling a remote machine

A paired machine (router, VPS, another Mac) gets its **own** profiler: its own
window, its own streams, its own ring buffer. Open it from the **More** block in the
machine's row on the **Remote** tab (**More → Traffic Profiler**); it needs a live
connection, since the source is that machine's gRPC stream.

Re-opening focuses the existing window instead of starting a second stream, and the
instance dies with its channel — on Disconnect and on machine removal. The local
profiler is untouched, so you can watch a router and your own machine side by side.

Two differences from the local view:

- **No per-process breakdown.** The per-process axis is replaced by a **per-client**
  one — traffic on a router comes from network devices, not from processes of this
  computer. `find_process` is off in a router's config for the same reason, so there
  would be nothing to attribute. Client names come from the machine's
  `GET /admin/clients-info`.
- **No log-tail fallback.** Everything arrives over gRPC (connections, DNS, status,
  plus `GetRules` / `GetOutbounds` for decoding) — the machine's `sing-box.log` lives
  on its own filesystem and its config has no Clash API by design.

> Traffic is not the only question a slow router raises. The **host telemetry**
> window (same row, CPU / memory / storage / network of the machine itself) answers
> "is the box overloaded" — the profiler describes the core, telemetry describes the
> machine. See [DAEMON_AND_REMOTE.md](DAEMON_AND_REMOTE.md).

## Limits & in-memory only

- One active recording at a time.
- Up to 5 completed sessions in a FIFO ring.
- Each session caps at 50 000 events or 3 hours sliding window (whichever first). The dropped-event counter shows in the status line.
- Sessions are wiped on app quit — **there is no persistence**. If you want to keep a session, export it.

## Troubleshooting

| Symptom | Cause / Fix |
|---|---|
| "Sing-box is not running" banner | Start sing-box from the Local tab first. The profiler keeps capturing what little arrives but the picture is incomplete. |
| Most events show no process | `route.find_process` is off in your config. Re-run the wizard and save — the default template has it on. |
| DNS events missing | Switch on Verbose logs — in classic mode sing-box only logs DNS at debug level. In daemon mode DNS arrives over a structured gRPC stream and needs no verbose toggle. |
| Window title timer not ticking | Reopen the window. The timer goroutine stops when the window is closed and restarts on Show. |
| "TCP RST early" on every connection to a domain | Likely firewall / TLS handshake fail at the destination. Try a different outbound / direct connection to compare. |
