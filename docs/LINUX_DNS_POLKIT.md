# Linux: passwordless DNS for TUN (systemd-resolved + Polkit)

On a Linux desktop running `systemd-resolved`, starting a TUN connection makes sing-box
configure DNS for the TUN interface through `resolvectl`. Each of those calls is a separate
D-Bus method on `systemd-resolved`, and Polkit authorizes each one on its own — so a single
VPN start produces **three password prompts**, and a stop produces **one more**.

The four Polkit actions involved:

| Action | When |
| --- | --- |
| `org.freedesktop.resolve1.set-dns-servers` | start |
| `org.freedesktop.resolve1.set-domains` | start |
| `org.freedesktop.resolve1.set-default-route` | start |
| `org.freedesktop.resolve1.revert` | stop |

Reported and solved by the issue author on openSUSE Tumbleweed (issue #126); the rule is
distribution-agnostic — it only needs Polkit with JavaScript rules support (`polkit >= 0.106`,
which is every current desktop distribution).

## Why the usual fixes do not help

- **`CAP_NET_ADMIN` on the sing-box binary.** The capability lets the process touch network
  interfaces directly, but `resolvectl` does not touch them: it asks `systemd-resolved` over
  D-Bus to do the work. The privilege check happens on the **service** side, in Polkit, and
  Polkit looks at who is asking, not at what capabilities they hold.
- **`systemd-resolve` / `systemd-network` groups.** These are service-account groups that own
  the daemon's own files and runtime state. They are not referenced by the `resolve1` Polkit
  policy, so adding yourself to them changes nothing.
- **`wheel`.** `wheel` membership means "may authenticate as an administrator" — Polkit will
  still ask for the password, it just accepts your own password instead of root's. The prompts
  stay.
- **Broad authorization groups** (openSUSE's `empower`, and similar distro-specific groups)
  do suppress the prompts, but they grant far more privilege than sing-box needs. Not
  recommended.

The launcher does **not** install this rule for you. It is a system-level configuration
change, so it stays a deliberate, documented administrator action.

## Install

### 1. Create the group and join it

```bash
sudo groupadd --system singbox-dns
sudo usermod --append --groups singbox-dns "$USER"
```

### 2. Install the Polkit rule

```bash
sudo tee /etc/polkit-1/rules.d/49-singbox-resolved.rules >/dev/null <<'EOF'
/*
 * Allow active local members of singbox-dns to configure per-link DNS
 * through systemd-resolved for sing-box TUN connections.
 */
polkit.addRule(function(action, subject) {
    var singBoxResolvedAction =
        action.id == "org.freedesktop.resolve1.set-dns-servers" ||
        action.id == "org.freedesktop.resolve1.set-domains" ||
        action.id == "org.freedesktop.resolve1.set-default-route" ||
        action.id == "org.freedesktop.resolve1.revert";

    if (singBoxResolvedAction &&
        subject.isInGroup("singbox-dns") &&
        subject.local &&
        subject.active) {
        return polkit.Result.YES;
    }
});
EOF

sudo chown root:root /etc/polkit-1/rules.d/49-singbox-resolved.rules
sudo chmod 0644 /etc/polkit-1/rules.d/49-singbox-resolved.rules
```

### 3. Log out and back in

Supplementary groups are attached to a session when it is created, so the new membership
reaches the launcher only after a **full** logout and login of the desktop session. Closing
and reopening the launcher is not enough.

## Verify

Confirm the group is in your session:

```bash
id
```

The output must list `singbox-dns`. If it does not, you are still in the old session — log out
again.

Confirm Polkit parsed the rule and knows the actions:

```bash
pkaction --action-id org.freedesktop.resolve1.set-dns-servers --verbose
```

A syntax error in the rules file is reported by the Polkit daemon:

```bash
journalctl -u polkit -n 50
```

Then start and stop the VPN. Both should complete without a single password prompt.

## Security notes

The rule is deliberately narrow:

- it applies only to members of the dedicated `singbox-dns` group — nobody is affected by
  default;
- it requires a **local, active** session (`subject.local && subject.active`), so it does not
  apply to SSH sessions or to a switched-away user;
- it permits exactly the four `systemd-resolved` actions the TUN lifecycle needs, and returns
  nothing (falls through to the default policy) for everything else;
- it grants no administrator rights and no general networking privileges — a member of the
  group can set per-link DNS, nothing more.

Authorize another user:

```bash
sudo usermod --append --groups singbox-dns USERNAME
```

## Uninstall

```bash
sudo rm /etc/polkit-1/rules.d/49-singbox-resolved.rules
sudo gpasswd --delete "$USER" singbox-dns
sudo groupdel singbox-dns
```

Log out and back in afterwards so the session drops the removed group. Polkit picks up the
removal of the rules file immediately — no daemon restart is needed.

## See also

- [docs/BUILD_LINUX.md](BUILD_LINUX.md) — building and running on Linux, `setcap` for TUN.
- Russian version: [LINUX_DNS_POLKIT.ru.md](LINUX_DNS_POLKIT.ru.md)
