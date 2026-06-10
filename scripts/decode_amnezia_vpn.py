#!/usr/bin/env python3
"""Decode Amnezia vpn:// links and convert WG/AWG configs to awg:// URIs.

Format (reference: amnezia-vpn/config-decoder, mainwindow.cpp):
  vpn:// + base64url (alphabet -_, no padding)
  payload = qCompress(json, 8): 4-byte big-endian uncompressed length,
            then a plain zlib stream
  json    = full Amnezia profile: containers[], defaultContainer,
            hostName, dns1/dns2; the WG/AWG config itself is the
            [Interface]/[Peer] text inside a container's last_config

Usage:
  decode_amnezia_vpn.py vpn://...            decode link, print awg:// URI
  decode_amnezia_vpn.py profile.vpn          same, link read from file
  decode_amnezia_vpn.py tunnel.conf          convert a plain .conf instead
  decode_amnezia_vpn.py --json vpn://...     dump the decoded profile JSON

The awg:// URI goes to stdout; everything else goes to stderr.
MTU from the source config is dropped for AWG on purpose: Amnezia writes
1420, which breaks data flow (sendmsg: message too long); the launcher
defaults AWG endpoints to a safe 1280 when mtu is absent.
"""

import base64
import json
import sys
import zlib
from urllib.parse import quote

AWG_NUMERIC = ("jc", "jmin", "jmax", "s1", "s2", "s3", "s4",
               "h1", "h2", "h3", "h4")
AWG_STRINGS = ("i1", "i2", "i3", "i4", "i5")
INTERFACE_KEYS = {"privatekey", "address", "dns", "mtu", "listenport"}
PEER_KEYS = {"publickey", "presharedkey", "allowedips", "endpoint",
             "persistentkeepalive"}


def err(msg):
    print(msg, file=sys.stderr)


def decode_vpn_link(link):
    """vpn://<base64url> -> profile dict."""
    payload = link.strip()
    if payload.startswith("vpn://"):
        payload = payload[len("vpn://"):]
    payload = payload.replace("\n", "").replace("\r", "")
    payload += "=" * (-len(payload) % 4)
    raw = base64.urlsafe_b64decode(payload)
    if len(raw) < 5:
        raise ValueError("payload too short for qCompress format")
    expected = int.from_bytes(raw[:4], "big")
    data = zlib.decompress(raw[4:])
    if expected and len(data) != expected:
        err(f"warning: qCompress length header says {expected}, "
            f"got {len(data)} bytes")
    return json.loads(data.decode("utf-8"))


def find_ini_configs(node, out):
    """Recursively collect every string that looks like a WG .conf."""
    if isinstance(node, str):
        # last_config is a JSON string nested inside the profile, and its
        # raw text contains "[Interface]" too — so try JSON first
        if node.lstrip().startswith("{"):
            try:
                find_ini_configs(json.loads(node), out)
                return
            except (ValueError, RecursionError):
                pass
        if "[Interface]" in node:
            out.append(node)
    elif isinstance(node, dict):
        for v in node.values():
            find_ini_configs(v, out)
    elif isinstance(node, list):
        for v in node:
            find_ini_configs(v, out)


def parse_ini(text):
    """[Interface]/[Peer] text -> (interface dict, peer dict), keys lowered."""
    iface, peer = {}, {}
    section = None
    for line in text.splitlines():
        line = line.strip()
        if not line or line.startswith(("#", ";")):
            continue
        if line.startswith("["):
            section = line.strip("[]").lower()
            continue
        if "=" not in line:
            continue
        key, _, value = line.partition("=")
        key, value = key.strip().lower(), value.strip()
        if section == "interface":
            iface[key] = value
        elif section == "peer" and key not in peer:  # first peer wins
            peer[key] = value
    return iface, peer


def build_uri(iface, peer, name):
    missing = [k for k, d in (("privatekey", iface), ("endpoint", peer),
                              ("publickey", peer)) if k not in d]
    if missing:
        raise ValueError(f"config is missing required fields: {missing}")

    awg = {k: iface[k] for k in AWG_NUMERIC + AWG_STRINGS if iface.get(k)}
    scheme = "awg" if awg else "wireguard"

    params = [("publickey", peer["publickey"])]
    if iface.get("address"):
        params.append(("address", iface["address"].replace(" ", "")))
    params.append(("allowedips",
                   peer.get("allowedips", "0.0.0.0/0,::/0").replace(" ", "")))
    if peer.get("persistentkeepalive"):
        params.append(("keepalive", peer["persistentkeepalive"]))
    if peer.get("presharedkey"):
        params.append(("presharedkey", peer["presharedkey"]))
    if iface.get("dns"):
        params.append(("dns", iface["dns"].replace(" ", "")))
    if iface.get("listenport"):
        params.append(("listenport", iface["listenport"]))
    if iface.get("mtu"):
        if scheme == "awg":
            err(f"note: dropped MTU={iface['mtu']} (AWG needs <=1380; "
                "launcher will default to 1280)")
        else:
            params.append(("mtu", iface["mtu"]))
    params.extend((k, awg[k]) for k in AWG_NUMERIC + AWG_STRINGS if k in awg)

    query = "&".join(f"{k}={quote(v, safe='')}" for k, v in params)
    host_port = peer["endpoint"]
    key = quote(iface["privatekey"], safe="")
    return f"{scheme}://{key}@{host_port}?{query}#{quote(name, safe='')}"


def main():
    args = [a for a in sys.argv[1:] if a != "--json"]
    dump_json = "--json" in sys.argv[1:]
    if not args:
        err(__doc__)
        return 2
    src = args[0]

    if src.startswith("vpn://"):
        text = src
    else:
        with open(src, encoding="utf-8-sig") as f:
            text = f.read()

    if text.strip().startswith("vpn://"):
        profile = decode_vpn_link(text)
        if dump_json:
            print(json.dumps(profile, indent=2, ensure_ascii=False))
            return 0
        name = profile.get("description") or profile.get("hostName") or "awg"
        err(f"host: {profile.get('hostName')}  "
            f"defaultContainer: {profile.get('defaultContainer')}  "
            f"containers: {[c.get('container') for c in profile.get('containers', [])]}")
        configs = []
        find_ini_configs(profile, configs)
        if not configs:
            raise SystemExit("no [Interface]/[Peer] config found in profile; "
                             "rerun with --json to inspect it")
    else:
        configs, name = [text], src.rsplit("/", 1)[-1].rsplit(".", 1)[0]

    seen = set()
    for conf in configs:
        iface, peer = parse_ini(conf)
        uri = build_uri(iface, peer, name)
        if uri not in seen:
            seen.add(uri)
            print(uri)
    return 0


if __name__ == "__main__":
    sys.exit(main())
