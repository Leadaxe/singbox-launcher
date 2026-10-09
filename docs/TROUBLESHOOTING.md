# Troubleshooting

Known problems and where their solutions live. Quick first checks are in the
[README troubleshooting table](../README.md#troubleshooting); build problems on
Linux are in [BUILD_LINUX.md](BUILD_LINUX.md#troubleshooting).

Russian version: [TROUBLESHOOTING.ru.md](TROUBLESHOOTING.ru.md).

## All platforms

### The window stops responding

After about 30 s of a frozen window the launcher writes `ui-freeze-<date-time>.txt` to the logs folder (Diagnostics → **Logs folder**).
Attach that file and the main log to the issue — it shows what the window was waiting for.

### A REALITY node shows "Error", the core log says `reality verification failed`

The test of VLESS/Trojan nodes with `security=reality` shows "Error" while `direct-out`
answers. On the **Core** tab of the logs window there are lines like
`open connection to … using outbound/…: reality verification failed`, and they arrive
fast (hundreds of milliseconds), with no timeout.

What it means: the server is reachable and the TLS handshake completed, but the server
did not recognise the client as its own. REALITY does not drop the connection in that
case; it silently serves the real certificate of the camouflage site (`sni` from the
link) — by design, so the server cannot be told apart by the shape of its refusal.
That is why all the causes below look the same in the log.

**System certificates and the "TLS root certificate store" setting have nothing to do
with it.** For REALITY the core checks its own signature and verifies the camouflage
certificate against the OS system store regardless of that setting. Outdated system
certificates would give a different error — `x509: certificate signed by unknown
authority`. If the node started working after changing that setting, something else
helped: the launcher rebuilt the config and restarted the core, or a temporary network
state passed (see item 5).

What to check, in order:

1. **The node's fingerprint.** Xray v26.9.8 and later accept only a ClientHello carrying
   the post-quantum `X25519MLKEM768` key share. `chrome` (and every `chrome_*`) carries
   it, and with the core shipped in launcher 1.6.2+ (sing-box-lx ≥ 1.14.1-lx.3) so do
   `firefox` (Firefox 148) and `safari` (Safari 26.3). `ios`, `android`, `edge`, `360`,
   `qq` do not and are rejected by such servers; `random` and `randomized` pass only
   some of the time. Quick check — set the node to `chrome`: if it works, this is the
   cause.
2. **Launcher and core version.** On an old core the same symptom comes from
   `firefox`/`safari` (before 1.6.2) and from the client-version cutoff `minClientVer`,
   which Xray enables by default since v26.7.11. Update the launcher — the core ships
   with it.
3. **Link parameters.** `pbk` (public key) and `sid` (short id) must match the server.
   If the link was reissued, the old one fails exactly this way.
4. **Clock.** The server may reject clients whose time differs too much. Turn on
   automatic time and time zone in the system.
5. **Network.** Test the same node through another network, e.g. a phone hotspot. An
   ISP may temporarily (for 1–2 minutes) break connections to a given address after a
   series of failed attempts; frequent back-to-back tests prolong that state, so pause
   between checks.

If the link works for someone else on a current version and not for you, the cause is
on your side (items 1, 2, 4, 5), not on the server.

## Linux

### A password is asked three times when the VPN starts, and once when it stops

Desktop with `systemd-resolved`: sing-box sets the DNS of the TUN interface through
`resolvectl`, and Polkit authorizes each of the four D-Bus actions separately.
`CAP_NET_ADMIN` on the binary does not help — the check happens in `systemd-resolved`.

A user-contributed recipe (a dedicated group plus a narrow Polkit rule for exactly these
four actions) is in [issue #126](https://github.com/Leadaxe/singbox-launcher/issues/126). It is a system-level change made by the
administrator; the launcher does not install it, and the project has not verified it on
other distributions.

## Windows

### Start shows “TUN needs administrator rights”

The launcher runs without administrator rights (no UAC prompt at start), and TUN
creates a network adapter and changes routes, which Windows allows only to
administrators. The dialog offers:

- **Restart as administrator** — one UAC prompt; the launcher restarts elevated with
  the same data folder (the window title ends with `(Administrator)`) and starts the
  VPN. On a standard user account Windows asks for an administrator's password, and
  the launcher then runs under that account. Declining the prompt keeps the dialog
  open.
- **Switch to proxy mode** — turns TUN off and the local proxy with the system proxy
  on (port `proxy_in_listen_port`, 7890 by default): browsers and most apps go
  through the VPN, the rest connect directly. Unavailable while the configurator is
  open — close it first.

To start elevated every time, use a shortcut with “Run as administrator”; Start with
Windows (Settings → Connection) always starts the launcher without rights.

### “Sing-Box appears to be already running”, and Kill says it needs administrator rights

sing-box was started by an elevated launcher that is gone (closed in Task Manager or
crashed), and a launcher without rights cannot stop it. Choose **Restart as
administrator** in the message; in the elevated launcher the same warning appears,
and **Kill Process** stops the core. Network cleanup (ghost adapters, NLA profiles,
orphan firewall rules) also runs only in an elevated launcher: without rights it is
skipped and logged as one INFO line.

### Data is in `%LOCALAPPDATA%` although `portable.txt` lies next to the program

The program folder is under `Program Files` (or `Windows`), where only administrators
can write, so the marker is ignored — **Settings → Storage → Mode** shows
`portable.txt ignored`. Data from an older version kept next to the program was
copied to `%LOCALAPPDATA%\singbox-launcher` on the first start; the old copy stays in
place. To keep data next to the program, extract the zip to a folder your account can
write to. **Remove all data…** without rights skips the old copy in `Program Files`
(*Requires administrator rights*), and since that copy stays, the next start
migrates it into `%LOCALAPPDATA%` again. Run the cleanup as administrator —
`"<exe>" -purge-data -yes` from an administrator command prompt — to remove it.

## Remote machines

### Save and Deploy succeed, but the machine keeps running the old rules

Version 2.0.0 only. The Configurator of a remote machine saves the state, shows
"Remote config exported", Deploy succeeds — yet the new rules have no effect on the machine.

Cause: before writing, the config is checked by the local core, and the rule-set (`.srs`)
paths in a remote machine's config point into that machine's file system. The check failed
with "no such file", and the previous build silently stayed on disk instead of the new
config — that is what Deploy then sent.

How to confirm: `bin/wizard_states/remote/<id>/config.json` is older than your last Save,
and the launcher log has `corereject: the core error does not name a node of ours` next to
the Save.

Fixed in versions after 2.0.0: for the duration of the check the paths are swapped for the
local copies of the rule sets, and a core rejection is shown as an error instead of being
lost. Workaround for 2.0.0: rename the machine's old `config.json` and press Save again —
with no previous file the config gets written. This has to be repeated before every Save.

## Daemon

The daemon (`sing-box lxd`) is the system service that runs the core — on this computer
in Daemon mode and on every remote machine (a router, a VPS). When it misbehaves, open
the **Service** window: ⚙ in the machine's row on the Remote tab, or Servers → ⚙ →
Local for this computer. It opens on the tab that needs attention (`✖ Not running`,
`⚠ Core`, `✖ Pairing`; Reference holds the paths) and fills in the commands below with
that machine's address and paths: ⧉ copies a command as is, ▶ opens Terminal with it —
for a remote machine wrapped in `ssh <target> '…'`. The SSH target is set in the
machine's Edit window (`root@<daemon host>` by default). The window's header turns green
by itself a few seconds after the daemon answers again.

This section holds the same recipes for when the launcher is not at hand.

> **These are default paths — yours may differ.** They are what the launcher assumes
> while the daemon has not reported its own. A hand-made OpenWrt install, for example,
> often keeps the binary in `/root/sing-box` (fork guide
> [§8.3](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.md#83-openwrt--procd-routers)).
> The **Reference** tab of the Service window shows the paths the daemon reported
> (`/admin/info`) and marks every assumed one `default` — trust those over this table.

The service is named `sing-box-lxd` everywhere (macOS launchd label
`com.leadaxe.sing-box-lxd`); the control channel listens on port `19091` unless
`daemon.json` says otherwise. On Linux the commands need root — as a non-root ssh user,
put `sudo` in front (the Service window does it for you). On Windows, run them in
PowerShell **as administrator**.

### Where things live

| | OpenWrt (procd) | Linux (systemd) | macOS (launchd) | Windows (SCM) |
|---|---|---|---|---|
| Core binary | `/usr/bin/sing-box` | `/usr/local/bin/sing-box` | `/Library/PrivilegedHelperTools/sing-box-lxd` | `C:\Program Files\sing-box-lxd\sing-box-lxd.exe` |
| Service | `/etc/init.d/sing-box-lxd` | `/etc/systemd/system/sing-box-lxd.service` | `/Library/LaunchDaemons/com.leadaxe.sing-box-lxd.plist` | SCM service `sing-box-lxd` |
| State dir | `/etc/sing-box-lxd/state` | `/var/lib/sing-box-lxd/state` | `/Library/Application Support/sing-box-lxd/state` | `C:\ProgramData\sing-box-lxd\state` |
| Log | `/tmp/lxd.log` | `/var/lib/sing-box-lxd/lxd.log` | `/Library/Application Support/sing-box-lxd/lxd.log` | `C:\ProgramData\sing-box-lxd\logs\lxd.log` |

Inside the state dir: `daemon.json` (settings), `last_good.json` (the last config that
started), `clients.json` (paired clients). On macOS and Windows the core binary is the
service's protected copy of the launcher's core, with its install record beside it
(`sing-box-lxd.install.json`); see [DAEMON_AND_REMOTE.md §2.1](DAEMON_AND_REMOTE.md#21-the-service-runs-a-root-owned-copy-of-the-core-spec-136).

`daemon.json` keys (fork guide
[§3](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.md#3-daemonjson--the-daemons-settings)):
`listen` (channel address, `"host:port"` or `{"address": [...], "port": N}`), `tls`
(mTLS on/off), `secret` (Bearer secret, the only gate when `tls` is `false`),
`log_file`, `log_max_size_mb` / `log_max_backups` / `log_max_age_hours` (1 / 1 / 24 by
default). **Change a setting by editing the file and restarting the service — never by
reinstalling.** To read it: `cat <state dir>/daemon.json` (macOS: `sudo cat`, Windows:
`Get-Content`).

### The daemon does not come up

Symptoms: the machine's row shows `✖ …` and a red dot on ⚙; the local header says the
daemon is not answering; `connection refused`, `i/o timeout`, `no route to host`. Go from
cheap to expensive and stop as soon as the daemon answers again.

| Step | OpenWrt (procd) | Linux (systemd) | macOS | Windows |
|---|---|---|---|---|
| 1. Is it running? | `/etc/init.d/sing-box-lxd status` | `systemctl status sing-box-lxd --no-pager` | `launchctl print system/com.leadaxe.sing-box-lxd` | `sc.exe query sing-box-lxd` |
| 2. Restart it | `/etc/init.d/sing-box-lxd restart` | `systemctl restart sing-box-lxd` | `sudo launchctl kickstart -k system/com.leadaxe.sing-box-lxd` | `Restart-Service -Name sing-box-lxd -Force` |
| 3. Read why | `tail -n 100 /tmp/lxd.log` | `tail -n 100 /var/lib/sing-box-lxd/lxd.log` | `sudo tail -n 100 '/Library/Application Support/sing-box-lxd/lxd.log'` | `Get-Content -Tail 100 'C:\ProgramData\sing-box-lxd\logs\lxd.log'` |
| Follow the log | `tail -f /tmp/lxd.log` | `tail -f /var/lib/sing-box-lxd/lxd.log` | `sudo tail -f '…/lxd.log'` | `Get-Content -Wait -Tail 50 '…\lxd.log'` |
| Who holds the port | `netstat -lnp \| grep :19091` | `ss -ltnp \| grep :19091` | `sudo lsof -nP -i :19091` | `netstat -ano \| findstr :19091` |

macOS: if `launchctl print` says the service is not found, it is installed but not
loaded — load it instead of reinstalling:
`sudo launchctl bootstrap system /Library/LaunchDaemons/com.leadaxe.sing-box-lxd.plist`.

What the log says:

- `lxd: refusing to run as …` — the service runs from an unprotected binary (macOS,
  Windows). Reinstall it from a protected copy: step 5.
- `bind: address already in use` — something else holds the port; see "Who holds the
  port", or change `listen` in `daemon.json`.
- A config error right after the core starts — go to step 4.

**4. The last config broke it?** Boot the last working one once, in the foreground —
the log stays on screen:

```sh
# OpenWrt
/etc/init.d/sing-box-lxd stop && /usr/bin/sing-box lxd --state-dir /etc/sing-box-lxd/state --config-force /etc/sing-box-lxd/state/last_good.json
# systemd
systemctl stop sing-box-lxd && /usr/local/bin/sing-box lxd --state-dir /var/lib/sing-box-lxd/state --config-force /var/lib/sing-box-lxd/state/last_good.json
# macOS
sudo launchctl bootout system/com.leadaxe.sing-box-lxd ; sudo /Library/PrivilegedHelperTools/sing-box-lxd lxd --state-dir '/Library/Application Support/sing-box-lxd/state' --config-force '/Library/Application Support/sing-box-lxd/state/last_good.json'
```

```powershell
# Windows (PowerShell as administrator)
sc.exe stop sing-box-lxd; & 'C:\Program Files\sing-box-lxd\sing-box-lxd.exe' lxd --state-dir 'C:\ProgramData\sing-box-lxd\state' --config-force 'C:\ProgramData\sing-box-lxd\state\last_good.json'
```

Then Ctrl-C and start the service again (`/etc/init.d/sing-box-lxd start`,
`systemctl start sing-box-lxd`, the macOS `bootstrap` command above, `sc.exe start
sing-box-lxd`).

**5. Nothing helps** — reinstall the core: [Updating the core on a
machine](#updating-the-core-on-a-machine) below (the same version is fine). Locally on
macOS and Windows: Service window → Core → **Install or update service**.

### Updating the core on a machine

When: the Service header says `Core 1.14.2-lx.11 (required 1.14.3-lx.14 ⚠)`, the machine's
⚙ has a yellow dot, or Deploy warns that the machine runs an older core. Such a core may
reject a config built for the newer one — it then rolls back to the last working config,
so the new rules simply do not take effect. Deploy only warns; it does not refuse.

**This computer (macOS, Windows).** Download the core on the dashboard's Core tab, then
run **Install or update service** (Service window → Core): it refreshes the protected
copy and restarts the service, keeping `daemon.json` and the paired clients.

**A remote macOS or Windows machine.** Run `lxd --service=install` on that machine itself
with the new core (fork guide
[§7](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.md#7-macos--automatic-installation) /
[§7a](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.md#7a-windows--automatic-installation)).

**A Linux machine (OpenWrt, systemd).** The recipe below has been run on an OpenWrt
router. The example is OpenWrt on `root@192.168.10.1`, `linux/arm64`, from
`1.14.2-lx.11` to `1.14.3-lx.14`; for systemd see the notes after it.

1. **Get the core for the machine's platform — on your computer, not the machine**
   (a router may lack the room to unpack it). Service window → Core → **Download**
   fetches the build for the machine's platform into
   `~/Downloads/sing-box-1.14.3-lx.14-linux-arm64` and checks it against the release's
   `SHA256SUMS`. By hand: download `sing-box-1.14.3-lx.14-linux-arm64.tar.gz` and
   `SHA256SUMS` from the [core release](https://github.com/Leadaxe/sing-box-lx/releases),
   compare `shasum -a 256` of the archive with its line in `SHA256SUMS`, unpack, and keep
   the `sing-box` binary. Asset names: `linux-arm64`, `linux-armv7`, `linux-amd64`,
   `linux-mips-softfloat`, `linux-mipsle-softfloat`.
2. **Upload it, streamed over ssh** — `scp` does not work on OpenWrt (there is no
   sftp-server):

   ```sh
   ssh root@192.168.10.1 'cat > /tmp/sing-box.new' < ~/Downloads/sing-box-1.14.3-lx.14-linux-arm64
   ```

   A non-standard ssh port: `ssh -p 2222 root@…`. `/tmp` is RAM; `df -h /tmp` shows
   whether the binary fits.
3. **Check it before swapping**, with the new binary against the current config:

   ```sh
   chmod +x /tmp/sing-box.new && sha256sum /tmp/sing-box.new && /tmp/sing-box.new version && /tmp/sing-box.new check -c /etc/sing-box-lxd/state/last_good.json
   ```

   Expect the sha256 shown in step 1, the new version with `with_lxd` among the tags, and
   no output from `check`. Anything else — stop here; the running service is untouched
   (`rm /tmp/sing-box.new`).
4. **Back up, swap, restart:**

   ```sh
   cp /usr/bin/sing-box /root/sing-box.1.14.2-lx.11.bak && /etc/init.d/sing-box-lxd stop && mv /tmp/sing-box.new /usr/bin/sing-box && chmod 755 /usr/bin/sing-box && /etc/init.d/sing-box-lxd start
   ```

   The backup goes to `/root` because `/tmp` does not survive a reboot. Router flash is
   small: delete the backup once the new core has proven itself.
5. **Check:** within a few seconds the Service header shows `1.14.3-lx.14 · started`;
   on the machine — `/etc/init.d/sing-box-lxd status`.
6. **Roll back** if the new core misbehaves:

   ```sh
   cp /root/sing-box.1.14.2-lx.11.bak /usr/bin/sing-box && /etc/init.d/sing-box-lxd restart
   ```

systemd: the binary is `/usr/local/bin/sing-box`, the service commands are
`systemctl stop|start|restart sing-box-lxd`, the backup sits next to the binary
(`/usr/local/bin/sing-box.1.14.2-lx.11.bak`), the check uses
`/var/lib/sing-box-lxd/state/last_good.json`. The `version` step matters on routers: a
binary for the wrong architecture or a glibc build on a musl router will not start at all.

**No service yet** (a fresh machine): the init script / unit with the machine's paths is
under Service window → Core → *Install from scratch*, and in the fork guide
([§8.2 systemd](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.md#82-systemd-a-regular-serverdesktop),
[§8.3 OpenWrt](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.md#83-openwrt--procd-routers)).
Enable it with `chmod +x /etc/init.d/sing-box-lxd && /etc/init.d/sing-box-lxd enable &&
/etc/init.d/sing-box-lxd start` or `systemctl daemon-reload && systemctl enable --now
sing-box-lxd`. On OpenWrt add the binary, the init script and the state dir to
`/etc/sysupgrade.conf`, or a firmware upgrade wipes them. A VPN Wi-Fi over the daemon:
[openwrt-vpn-ssid](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/openwrt-vpn-ssid.md).

**Removing the service** keeps the state: `/etc/init.d/sing-box-lxd disable &&
/etc/init.d/sing-box-lxd stop && rm /etc/init.d/sing-box-lxd` or `systemctl disable --now
sing-box-lxd && rm /etc/systemd/system/sing-box-lxd.service && systemctl daemon-reload`.
`rm -r <state dir>` deletes the paired clients, keys and last-good config as well — every
launcher will have to pair again.

### Pairing

Re-pair when the launcher reports `certificate changed` (the daemon's certificate no
longer matches the pinned fingerprint), `not paired`, `bad certificate` or `403`, after
the service was reinstalled with a wiped state dir, or when the ⚙ window's Pairing tab is
marked `✖`. Fork guide:
[§9](https://github.com/Leadaxe/sing-box-lx/blob/lx/docs-lx/lxd-daemon.md#9-pairing-a-client-the-same-on-every-os).

1. **Mint an invite on the machine itself** — operator commands answer only on loopback:

   ```sh
   # OpenWrt (systemd: /usr/local/bin/sing-box and /var/lib/sing-box-lxd/state)
   /usr/bin/sing-box lxd client add --name singbox-launcher --state-dir /etc/sing-box-lxd/state
   # macOS (the state dir is found automatically)
   sudo /Library/PrivilegedHelperTools/sing-box-lxd lxd client add --name singbox-launcher
   ```

   On Windows: `& 'C:\Program Files\sing-box-lxd\sing-box-lxd.exe' lxd client add --name
   singbox-launcher` in PowerShell as administrator. It prints `address#fingerprint#code`.
2. **Paste it into the launcher**: Service window → Pairing → **Pair** (a remote machine
   also: Edit → Re-pair). If the address in the invite is loopback, replace it with one
   reachable from this computer (`192.168.10.1:19091`) and keep the fingerprint and code.

Gotchas, all seen on a real router:

- The code lives in the daemon's memory: restarting the daemon between `client add` and
  Pair kills it (`enroll: no active enrollment code`) — mint a fresh one.
- If `listen` is a single LAN address, loopback is not listened on and `client add`
  cannot reach the daemon. Use the object form with both addresses:
  `{"address": ["192.168.10.1", "127.0.0.1"], "port": 19091}`, then restart.
- Who is trusted: `… lxd client list --state-dir …`; revoke: `… lxd client remove
  <name-or-fingerprint> --state-dir …`. Removing a machine from the launcher's list does
  **not** revoke its access — only `client remove` on the machine does.
- Plain mode (`"tls": false`, loopback/dev only): the only credential is the Bearer
  `secret` from `daemon.json`; enter it in the Pairing tab.

More on the Service window and the machine registry:
[DAEMON_AND_REMOTE.md](DAEMON_AND_REMOTE.md).
