# Daemon core engine and remote machines

**🌐 Language**: English | [Русский](DAEMON_AND_REMOTE.ru.md)

> Status: current for SPEC 096 (daemon core engine), 097 (remote config target),
> 098 (Local/Remote tabs, one profile per machine), 099 (machine traffic profiler),
> 100 (remote/daemon coverage in the Debug API).
>
> Companion documents:
> - **[ARCHITECTURE.md](ARCHITECTURE.md)** — layers, the `CoreBackend`/`ProxyTransport` seams.
> - **[TRAFFIC_PROFILER.md](TRAFFIC_PROFILER.md)** — the profiler, including the per-machine instance.
> - **[API.md](API.md)** — the launcher's Debug HTTP API (not to be confused with the
>   daemon's admin REST); see its `/remote/machines*`, `/daemon/*` and raw passthrough sections.

---

## 1. Two core engines

The launcher can drive the core in two ways. Nothing above the seam sees which one
is active: the UI, tray, keyboard shortcuts and Debug API reach the core only
through the active engine (`CoreBackend`).

| | **Classic** | **Daemon (lxd)** |
|---|---|---|
| Platforms | Windows, macOS, Linux | **macOS only** |
| Default | yes | no, opt-in |
| How the core lives | child process `sing-box run` | inside the long-lived system service `sing-box lxd` |
| Control plane | Clash HTTP API | gRPC (`daemon.StartedService`) + admin REST |
| Applying a config | kill + restart the process | the core is swapped in place, without restarting the service |
| Privileges | a password on every privileged TUN start | once, at install time; nothing afterwards |
| Quitting the launcher | brings the VPN down | **leaves the VPN running** by default |
| Core requirement | an ordinary fork build | a build with the `lxd` subcommand (`with_lx_command`) |

Implementation: `LegacyBackend` — the classic spawn; `DaemonBackend` — lxd. Proxy
group operations are abstracted behind the `ProxyTransport` seam (Clash HTTP for
classic, gRPC for daemon), so the server list is identical in both modes.

All daemon code and gRPC sit behind darwin build tags and never enter
`go.win7.mod` — the Win7 build compiles without them.

### 1.1 Where to switch it

**Local → ⚙ (connection settings) → LOCAL tab** — a Process / Daemon radio.
The radio expresses *intent*: choosing Daemon shows the command panel even before
pairing, and the engine actually switches once pairing is complete (the section's
status line reflects this). The engine can only be switched while the VPN is
stopped.

The **REMOTE** tab of the same window is the older remote Clash API override
(SPEC 064) — a separate thing from daemon mode.

---

## 2. Installing the service: sudo, in your own terminal

The launcher **never performs privileged operations itself**. It prepares a ready
sudo command and lets you copy it or open it in Terminal; you see the full output,
and sudo asks you.

| Operation | Command |
|---|---|
| Install the service | `sudo <path-to-sing-box> lxd --service=install` |
| Uninstall the service | `sudo <path-to-sing-box> lxd --service=uninstall` |
| Uninstall along with the daemon's data | the same `+ --purge` |
| Mint a fresh invite | `sudo <path-to-sing-box> lxd client add --name singbox-launcher` |

`--service=install` takes no parameters: it picks a free loopback port itself
(19091+, or keeps the address of an existing installation), generates the secret,
forces mTLS on, and prints a **one-time invite** at the end.

After that, starting/stopping the VPN and applying configs need no password.

---

## 3. Pairing (mTLS)

The channel to the daemon is mutually authenticated TLS. The client's credential is
its own certificate; pinning the server certificate is always mandatory.

An **invite** is a single line shaped like:

```
address#fingerprint#code
```

It is printed by the install command (the launcher extracts the invite from the
output of the privileged call and writes it to its own log with the invite redacted)
or by `lxd client add` — for re-pairing and for remote daemons. The invite is pasted
into the pairing field by hand.

**Who owns what.** The daemon's home is its own state directory: `daemon.json`,
holding the listen address and the admin secret, lives there and is served over
`GET /admin/info`. The launcher keeps only its own client keypair. Each machine gets
**its own** pair: a certificate is the entire credential, and one shared key across
all devices would mean that revoking access on one router revokes it everywhere.

> A practical consequence: **removing a machine from the list in the launcher does
> not revoke access.** Revocation happens on the machine itself — the launcher warns
> about this when you remove one.

---

## 4. Remote machines

A remote machine is a router, a VPS or another Mac running its core under
`sing-box lxd`, with the launcher acting as its mTLS client.

### 4.1 The Remote tab

`Local` and `Remote` are built the same way — servers on the left, management on
the right. The difference is only what is managed: your own core, or a list of
other machines. A machine's row shows its name, platform, address and core state;
the same row carries **Configure** (the wizard rooted on its profile), Start/Stop,
**Restart ↻** (Stop + Start as one action, behind a confirmation — the machine's
VPN clients blink during the restart), **Deploy**, edit, remove, and a **More**
block (traffic profiler, host telemetry, resources, diagnostics).

The machine's node list heals itself. Right after Start the daemon already
reports "started", but the core inside is still coming up and returns no groups
yet — so groups are polled in the background with retries, and the auto-refresh
tick re-reads them whenever the group selection is empty. An empty list right
after Stop/Start is a transient state measured in seconds, not a reason to
Disconnect/Connect.

The key property: **picking a machine and picking "who are we building for" are the
same choice.** "Configure" opens the wizard rooted on that machine's profile, and
Deploy in the same row ships that machine's own config. The "built for one, deployed
to another" mistake is impossible by construction, not caught by validation.

### 4.2 The registry entry

The machine registry is `bin/remote-daemons.json`. An entry (`services.RemoteDaemon`)
carries:

| Field | Meaning |
|---|---|
| `id` | stable identifier (a slug of the name); also the directory name for the client keypair and the profile |
| `name` | human-readable name |
| `addr` | `host:port` of the control channel |
| `server_fingerprint` | SHA-256 pin of the server certificate; empty = plain h2c (a dev daemon on loopback) |
| `secret` | bearer secret; only needed by a plain-h2c daemon. Under mTLS the client certificate is the credential |
| `goos` / `goarch` | platform and architecture of the **machine** |

`goos`/`goarch` live here rather than in the wizard state because they are a
property of the machine, not of one of its settings. The row displays them, the
wizard reads them, generation builds a `TargetSpec` from them — one source of truth,
otherwise a way remains to build a config for an architecture other than the one
shown in the list.

### 4.3 One profile per machine — the on-disk layout

```
bin/
├── wizard_states/
│   ├── state.json                  — THIS machine's wizard state (historical layout)
│   ├── <name>.json                 — named local snapshots
│   └── remote/
│       └── <machine-id>/           — everything belonging to one machine
│           ├── state.json          — its wizard state
│           ├── config.json         — its built config
│           ├── srs/*.srs           — its rule-sets
│           └── subscriptions/*.raw — its subscription bodies
├── subscriptions/<id>.raw          — local subscription raw cache
└── rule-sets/*.srs                 — local SRS
```

Before SPEC 098 there was a single profile for all machines
(`wizard_states/remote/state.json` and `bin/remote-config.json`): configuring a
second machine silently overwrote the first. Existing installs migrate
automatically when **exactly one** machine is paired; with several, the old files
are left untouched and a warning is logged, because ownership cannot be determined.

### 4.4 Deploy

Deploy ships more than JSON. Before sending, the resources the config references are
collected — local rule-sets (`route.rule_set` entries of `type: local`) and
subscription bodies — and travel into the machine's resource store together with the
config (`services.CollectDeployResources`).

The config is adapted for the daemon before it is sent: relative paths the core
writes itself (`cache_file`) are made absolute against the daemon's directory (the
daemon starts with `cwd=/`), and `experimental.clash_api` is stripped — a machine's
config has no Clash API by design.

Delivery is the admin REST call `POST /admin/apply`: the daemon validates the config
**in a subprocess** before touching the running instance, and automatically rolls
back to the last working config if the new one fails to start. Start/Stop go through
`/admin`.

The whole "resources strictly before the config" chain is one function,
`services.RemoteRegistry.Deploy`: both the Deploy button and the Debug API
(SPEC 100) call it, so "deploying via the API works differently from the button"
is impossible by construction.

### 4.5 Observing a machine

| Tool | Source | What it shows |
|---|---|---|
| Proxy list | gRPC (`ProxyTransport`) | groups, node selection, latencies, balancer pool |
| Traffic profiler | gRPC streams (`SubscribeConnections`, `SubscribeDNSQueries`, `SubscribeStatus`) | connections and domains of the machine's **core**; a per-client breakdown instead of a per-process one |
| Host telemetry | admin REST | CPU, memory, storage, network of the **machine itself** |
| Resources | admin REST | the machine's resource store (rule-sets, subscription bodies) |

The profiler and the telemetry window are **one instance per machine**: two machines
must be openable side by side, which is exactly why one looks at these. Re-opening
focuses the existing window instead of starting a second stream; an instance dies
with its channel (on Disconnect and on machine removal).

gRPC subscriptions survive Deploy/Start/Stop. Recreating the core instance on the
machine tears down server-side streams — the transport treats that as a normal
event: it resets its state, waits a couple of seconds and resubscribes until the
subscription is cancelled (`runResilientStream`). The profiler and the status keep
showing live data without the Disconnect/Connect ritual.

A machine has no per-process breakdown and cannot have one: `find_process` is off in
a router's config because traffic comes from network devices, not from processes of
this computer.

---

## 5. Remote in the Debug API (SPEC 100)

The entire remote/daemon feature set is also available without the UI — through
the launcher's own Debug API (loopback + bearer). The full reference with
examples lives in [API.md](API.md); this section is about the principles.

- **Stateless addressing.** Every call names the machine explicitly —
  `/remote/machines/{id}/…`: registry and pairing, health, core
  Start/Stop/rollback, deploy, mirrors of the wizard state handles,
  observability, resources. The UI notion of the "active machine" (Connect on
  the Remote tab) does not affect working calls; the override itself is managed
  by the separate `/remote/ui*` handles and changes only what the Servers tab
  looks at.
- **Parity with the button.** Deploying via the API and via the Deploy button is
  the same `services.RemoteRegistry.Deploy` chain (§4.4).
- **Streams as snapshots.** `connections`, `dns/queries`, `logs` return a window
  (`?duration=…&max=…`), not an endless stream; SSE subscriptions are
  deliberately deferred.
- **Raw passthrough.** An arbitrary admin REST call (`…/raw/rest`) and an
  arbitrary gRPC call (`…/raw/grpc`, discovery via `GET /grpc/methods`) go
  **only** to the control channel of a paired machine, with its mTLS keys and
  pin from the registry. There are no requests to arbitrary addresses.
- **`/daemon/*` (darwin).** Status, pairing and engine of the local daemon, plus
  ready-made privileged command strings (`/daemon/commands`) — the API never
  executes them: "sudo only in your own terminal" applies here too.
- **The `capabilities` manifest.** `GET /` reports which groups this build has:
  Win7 — no remote at all, non-darwin — no `/daemon/*`.

---

## 6. Boundaries and requirements

- **Classic does not change.** The same spawn, the same Clash API, the same behavior.
- **Daemon is macOS-only.** All of its code sits behind darwin build tags.
- **The core must support `lxd`** (`with_lx_command`). The pinned
  `constants.RequiredCoreVersion` (currently `1.14.0-lx.26`) includes that build.
  Check the feature boundary by running the binary (`sing-box lxd --help`), not by
  release number.
- **A remote config has no Clash API** by design — hence the gRPC sources for both
  the node list and the profiler.
