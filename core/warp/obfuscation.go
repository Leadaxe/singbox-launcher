package warp

import "strings"

// QuicParams configures AmneziaWG obfuscation for a WARP node. The junk knobs
// (jc/jmin/jmax) inject pre-handshake noise that breaks the DPI signature; the
// id/ip/ib masquerade sugar tells the sing-box-lx core (with_awg) to synthesize
// an i1 packet shaped like the named protocol. jc/jmin/jmax default to the
// LxBox values; ip defaults to "quic".
type QuicParams struct {
	JC   int    // junk packet count (default 4)
	JMin int    // junk min size (default 40)
	JMax int    // junk max size (default 70)
	IP   string // masquerade protocol: quic|dns|stun|sip (default quic)
	SNI  string // masquerade domain (default www.google.com); on-wire for dns/sip
	IB   string // browser fingerprint for ip=quic: chrome|firefox|curl (default chrome)
}

// DefaultQuicParams returns the LxBox default obfuscation profile.
func DefaultQuicParams() QuicParams {
	return QuicParams{JC: 4, JMin: 40, JMax: 70, IP: "quic", SNI: "www.google.com", IB: "chrome"}
}

// amneziaPreset returns the AmneziaWG fields that do NOT break the WireGuard
// handshake: s1=s2=0 (no magic prefix) and h1..h4=1..4 (standard WG message
// types) make init/response packets byte-identical to plain WireGuard, so
// Cloudflare still accepts them. jc/jmin/jmax add pre-handshake junk that
// scrambles the DPI signature. i1 is NOT set here — the core synthesizes it
// from id/ip/ib, and an explicit i1 alongside id/ip/ib is rejected.
func amneziaPreset(jc, jmin, jmax int) map[string]any {
	return map[string]any{
		"jc":   jc,
		"jmin": jmin,
		"jmax": jmax,
		"s1":   0,
		"s2":   0,
		"h1":   1,
		"h2":   2,
		"h3":   3,
		"h4":   4,
	}
}

// buildAWGFields builds the AmneziaWG obfuscation field set for the account from
// QuicParams. It mirrors LxBox buildAmneziaAwg: masquerade via id/ip/ib (the
// core expands them to i1), never an explicit i1. ib is emitted only for
// ip=quic (the only protocol that carries a browser fingerprint).
func buildAWGFields(p QuicParams) map[string]any {
	if p.JC == 0 && p.JMin == 0 && p.JMax == 0 {
		p = DefaultQuicParams()
	}
	fields := amneziaPreset(p.JC, p.JMin, p.JMax)

	ip := strings.TrimSpace(p.IP)
	if ip == "" {
		ip = "quic"
	}
	domain := strings.TrimSpace(p.SNI)
	if domain == "" {
		domain = "www.google.com"
	}
	fields["ip"] = ip
	fields["id"] = domain
	if ip == "quic" {
		ib := strings.TrimSpace(p.IB)
		if ib == "" {
			ib = "chrome"
		}
		fields["ib"] = ib
	}
	return fields
}
