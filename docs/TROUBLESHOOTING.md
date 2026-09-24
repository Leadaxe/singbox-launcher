# Troubleshooting

Known problems and where their solutions live. Quick first checks are in the
[README troubleshooting table](../README.md#troubleshooting); build problems on
Linux are in [BUILD_LINUX.md](BUILD_LINUX.md#troubleshooting).

Russian version: [TROUBLESHOOTING.ru.md](TROUBLESHOOTING.ru.md).

## All platforms

### The window stops responding

After about 30 s of a frozen window the launcher writes `ui-freeze-<date-time>.txt` to the logs folder (Diagnostics → **Logs folder**).
Attach that file and the main log to the issue — it shows what the window was waiting for.

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
