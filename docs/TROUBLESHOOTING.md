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
