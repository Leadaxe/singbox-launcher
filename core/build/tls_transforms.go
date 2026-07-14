// Package build — tls_transforms.go: global anti-DPI TLS transforms applied to
// first-hop outbounds at config-build time (SPEC 092). Port of the LxBox
// post-step services/builder/post_steps/tls_transforms.dart.
//
// Two independent, opt-in transforms, driven by template vars:
//   - TLS fragment: split the ClientHello (tls.fragment) and/or every TLS record
//     (tls.record_fragment) across packets so a DPI can't match the SNI in one
//     shot. This is the single most effective ClientHello-based DPI bypass in
//     RU/TSPU networks.
//   - Mixed-case SNI: randomize the letter case of the SNI hostname labels. TLS
//     SNI is case-insensitive (RFC 6066), so servers accept it, but a
//     case-sensitive DPI blocklist match is defeated.
//
// Applied only to FIRST-HOP TLS outbounds — an outbound that itself dials the
// network (has an enabled tls block, no detour, and is a real proxy type). Inner
// hops of a detour chain and non-TLS/utility outbounds are left untouched.
package build

import (
	"encoding/json"
	"hash/fnv"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// TLSTransformOptions controls the anti-DPI TLS transforms. All default false
// (opt-in): an untouched config is byte-for-byte what it was before SPEC 092.
type TLSTransformOptions struct {
	Fragment              bool   // tls.fragment — split the ClientHello
	RecordFragment        bool   // tls.record_fragment — split every TLS record
	FragmentFallbackDelay string // tls.fragment_fallback_delay (duration, optional)
	MixedCaseSNI          bool   // randomize SNI letter case
}

// anyEnabled reports whether any transform is on. When false, ApplyTLSTransforms
// is a no-op and the outbounds are returned unchanged (zero diff).
func (o TLSTransformOptions) anyEnabled() bool {
	return o.Fragment || o.RecordFragment || o.MixedCaseSNI
}

// TLSTransformOptionsFromVars reads the transform toggles from the effective
// template var map. Recognized vars (all bool except the delay):
// tls_fragment, tls_record_fragment, tls_fragment_fallback_delay, tls_mixed_case_sni.
func TLSTransformOptionsFromVars(vars map[string]string) TLSTransformOptions {
	return TLSTransformOptions{
		Fragment:              varIsTrue(vars, "tls_fragment"),
		RecordFragment:        varIsTrue(vars, "tls_record_fragment"),
		FragmentFallbackDelay: vars["tls_fragment_fallback_delay"],
		MixedCaseSNI:          varIsTrue(vars, "tls_mixed_case_sni"),
	}
}

func varIsTrue(vars map[string]string, key string) bool {
	switch vars[key] {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

// ApplyTLSTransforms rewrites first-hop TLS outbounds in place-safe fashion: it
// parses each outbound JSON, applies the enabled transforms, and re-marshals.
// An outbound that does not qualify (no TLS, has detour, utility type, or bad
// JSON) is passed through unchanged. Returns a new slice; the input is not
// mutated. A no-op when no transform is enabled.
func ApplyTLSTransforms(outbounds []json.RawMessage, opts TLSTransformOptions) []json.RawMessage {
	if !opts.anyEnabled() || len(outbounds) == 0 {
		return outbounds
	}
	out := make([]json.RawMessage, len(outbounds))
	touched := 0
	for i, raw := range outbounds {
		var ob map[string]interface{}
		if err := json.Unmarshal(raw, &ob); err != nil {
			out[i] = raw // not an object — leave as-is
			continue
		}
		if applyTLSTransformToOutbound(ob, opts) {
			if reencoded, err := json.Marshal(ob); err == nil {
				out[i] = reencoded
				touched++
				continue
			}
		}
		out[i] = raw
	}
	if touched > 0 {
		debuglog.InfoLog("Build: applied TLS anti-DPI transforms to %d first-hop outbound(s) (fragment=%v record=%v mixed_case_sni=%v)",
			touched, opts.Fragment, opts.RecordFragment, opts.MixedCaseSNI)
	}
	return out
}

// isFirstHopTLSOutbound reports whether ob is a first-hop TLS outbound eligible
// for transforms: has an enabled tls block, no detour (it dials the network
// itself), and is not a utility/relay type where fragment is meaningless or
// harmful (direct/block/dns/selector/urltest/naive — naive manages its own
// TLS/HTTP2 stack).
func isFirstHopTLSOutbound(ob map[string]interface{}) (map[string]interface{}, bool) {
	if det, _ := ob["detour"].(string); det != "" {
		return nil, false
	}
	switch t, _ := ob["type"].(string); t {
	case "direct", "block", "dns", "selector", "urltest", "naive", "":
		return nil, false
	}
	tls, ok := ob["tls"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	if enabled, _ := tls["enabled"].(bool); !enabled {
		return nil, false
	}
	return tls, true
}

// applyTLSTransformToOutbound applies the enabled transforms to one outbound.
// Returns true if the outbound was modified.
func applyTLSTransformToOutbound(ob map[string]interface{}, opts TLSTransformOptions) bool {
	tls, ok := isFirstHopTLSOutbound(ob)
	if !ok {
		return false
	}
	changed := false
	if opts.Fragment {
		tls["fragment"] = true
		if opts.FragmentFallbackDelay != "" {
			tls["fragment_fallback_delay"] = opts.FragmentFallbackDelay
		}
		changed = true
	}
	if opts.RecordFragment {
		tls["record_fragment"] = true
		changed = true
	}
	if opts.MixedCaseSNI {
		if sni, _ := tls["server_name"].(string); sni != "" {
			if mixed := mixedCaseSNI(sni); mixed != sni {
				tls["server_name"] = mixed
				changed = true
			}
		}
	}
	return changed
}

// mixedCaseSNI randomizes the letter case of an SNI hostname deterministically:
// the case pattern is derived from an FNV hash of the whole hostname, so it is
// stable across rebuilds (no config churn / golden flake) yet differs per host
// and looks scrambled to a case-sensitive DPI blocklist. TLS SNI is
// case-insensitive (RFC 6066), so the server still matches.
//
// Punycode labels (xn-- prefix) are left untouched — their case is significant
// to the encoding, and flipping it would break resolution. An IP literal (no
// letters) passes through unchanged.
func mixedCaseSNI(host string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(host))
	seed := h.Sum64()

	labels := strings.Split(host, ".")
	for li, label := range labels {
		if strings.HasPrefix(strings.ToLower(label), "xn--") {
			continue // Punycode — do not touch
		}
		b := []byte(label)
		bit := 0
		for i := 0; i < len(b); i++ {
			c := b[i]
			isLetter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
			if !isLetter {
				continue
			}
			// One hash bit per letter drives upper/lower; mix in the label index
			// so identical labels in different positions still vary.
			up := (seed>>(uint(bit%64)))&1 == 1
			if (li & 1) == 1 {
				up = !up
			}
			if up {
				b[i] = c &^ 0x20 // to upper (ASCII)
			} else {
				b[i] = c | 0x20 // to lower (ASCII)
			}
			bit++
		}
		labels[li] = string(b)
	}
	return strings.Join(labels, ".")
}
