# REVIEW_EMIT — SPEC 134 pre-release review

**Scope:** reverse mapping of node bodies to share URIs (`core/config/linkmap/emit.go`), its registry attributes, and production Copy link callers.

## Result

No confirmed defects were found. The focused release suite and the full Copy-link corpus round trip pass. The checked cases include JSON-decoded `float64` values and `[]interface{}` peers, multi-peer WireGuard refusal, AWG/AWG3 fields, xhttp, REALITY, WS early data, Shadowsocks plugins, Hysteria2 obfuscation and multi-port ranges, empty credentials, IPv6, and Unicode/percent-escaped labels.

## Owner follow-up

`core/config/contract_emit_test.go` still contains the stale `dialerKeepAliveNotEmitted` allowance and comments claiming that Copy link drops dialer keep-alive fields. Current registry-driven emission preserves the fields (the `vless/dialer_keepalive` and `trojan/dialer_keepalive_disabled` corpus cases pass), so this allowance should be removed in the owner's broader contract-test cleanup. It is outside this review's permitted edit set.
