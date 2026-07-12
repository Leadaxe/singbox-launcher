package warp

import "strings"

// QuicParams configures AmneziaWG obfuscation for a WARP node (SPEC 084.2 — full
// field set). The junk knobs (jc/jmin/jmax) inject pre-handshake noise; s1-s4 are
// junk sizes (s1/s2 before the handshake init/response, s3/s4 padding on every
// transport packet); h1-h4 are the magic message headers (WARP must keep 1..4 or
// Cloudflare rejects the handshake). Masquerade id/ip/ib is the high-level sugar
// the core expands into i1; alternatively i1-i5 can carry explicit junk-packet
// tags (mutually exclusive with id/ip/ib).
//
// A zero-value QuicParams means "use the safe WARP default profile"
// (DefaultQuicParams). A preset or the configurator fills every field explicitly.
type QuicParams struct {
	JC   int // junk packet count (default 4)
	JMin int // junk min size (default 40)
	JMax int // junk max size (default 70)

	// s1-s4 junk sizes. WARP-safe defaults: s1=s2=0 (init/response byte-identical
	// to plain WG so Cloudflare accepts them), s3=s4=0 (no per-packet padding).
	S1, S2, S3, S4 int

	// h1-h4 magic headers. WARP requires the plain-WG message types 1..4.
	H1, H2, H3, H4 int

	// Masquerade sugar (expanded to i1 by the core). Mutually exclusive with I1-I5.
	IP  string // protocol: quic|dns|stun|sip (default quic)
	SNI string // masquerade domain (default www.google.com); on-wire for dns/sip
	IB  string // browser fingerprint for ip=quic: chrome|firefox|curl (default chrome)

	// Explicit junk-packet tags (advanced; e.g. "<b 0x...>", "<r 10>"). When any
	// is set, the masquerade id/ip/ib is NOT emitted (the core rejects both).
	I1, I2, I3, I4, I5 string
}

// DefaultQuicParams returns the safe WARP AmneziaWG profile: jc/jmin/jmax junk +
// masquerade over QUIC to www.google.com, with the plain-WG-compatible s/h set.
func DefaultQuicParams() QuicParams {
	return QuicParams{
		JC: 4, JMin: 40, JMax: 70,
		S1: 0, S2: 0, S3: 0, S4: 0,
		H1: 1, H2: 2, H3: 3, H4: 4,
		IP: "quic", SNI: "www.google.com", IB: "chrome",
	}
}

// isZero reports whether the params are an unset zero value (no field touched),
// in which case the safe default profile is used.
func (p QuicParams) isZero() bool {
	return p == QuicParams{}
}

// hasExplicitJunk reports whether any i1-i5 explicit tag is set.
func (p QuicParams) hasExplicitJunk() bool {
	return p.I1 != "" || p.I2 != "" || p.I3 != "" || p.I4 != "" || p.I5 != ""
}

// buildAWGFields builds the AmneziaWG obfuscation field map for the endpoint from
// QuicParams. Emits jc/jmin/jmax, s1-s4, h1-h4, then EITHER explicit i1-i5 OR the
// masquerade id/ip/ib sugar (never both — the core rejects the combination).
func buildAWGFields(p QuicParams) map[string]any {
	if p.isZero() {
		p = DefaultQuicParams()
	}
	// h1-h4 must be valid WG message types; treat an unset 0 as the plain default.
	h := func(v, def int) int {
		if v == 0 {
			return def
		}
		return v
	}
	fields := map[string]any{
		"jc":   nonNeg(p.JC, 4),
		"jmin": nonNeg(p.JMin, 40),
		"jmax": nonNeg(p.JMax, 70),
		"s1":   p.S1,
		"s2":   p.S2,
		"h1":   h(p.H1, 1),
		"h2":   h(p.H2, 2),
		"h3":   h(p.H3, 3),
		"h4":   h(p.H4, 4),
	}
	// s3/s4 add per-transport-packet padding; emit only when non-zero (keeps the
	// default WARP config byte-identical to before this change).
	if p.S3 != 0 {
		fields["s3"] = p.S3
	}
	if p.S4 != 0 {
		fields["s4"] = p.S4
	}

	if p.hasExplicitJunk() {
		// Advanced: explicit junk tags, no masquerade sugar.
		for k, v := range map[string]string{"i1": p.I1, "i2": p.I2, "i3": p.I3, "i4": p.I4, "i5": p.I5} {
			if v != "" {
				fields[k] = v
			}
		}
		return fields
	}

	// Masquerade sugar (default path).
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

func nonNeg(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}
