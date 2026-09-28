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
