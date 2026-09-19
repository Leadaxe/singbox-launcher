# Troubleshooting

Known problems and where their solutions live. Quick first checks are in the
[README troubleshooting table](../README.md#troubleshooting); build problems on
Linux are in [BUILD_LINUX.md](BUILD_LINUX.md#troubleshooting).

Russian version: [TROUBLESHOOTING.ru.md](TROUBLESHOOTING.ru.md).

## Linux

### A password is asked three times when the VPN starts, and once when it stops

Desktop with `systemd-resolved`: sing-box sets the DNS of the TUN interface through
`resolvectl`, and Polkit authorizes each of the four D-Bus actions separately.
`CAP_NET_ADMIN` on the binary does not help — the check happens in `systemd-resolved`.

A user-contributed recipe (a dedicated group plus a narrow Polkit rule for exactly these
four actions) is in [issue #126](https://github.com/Leadaxe/singbox-launcher/issues/126). It is a system-level change made by the
administrator; the launcher does not install it, and the project has not verified it on
other distributions.

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
