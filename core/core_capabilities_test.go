package core

import (
	"strings"
	"testing"
)

// SPEC 044 feature-probe: the verdict must degrade naive ONLY on positive
// evidence (Tags line present and missing with_naive_outbound, or purego
// without libcronet). Any uncertainty → supported, so we never silently
// drop nodes on guesswork.

const lxVersionOutput = `sing-box version 1.14.0-lx.3

Environment: go1.25.5 windows/amd64
Tags: with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_clash_api,with_naive_outbound,with_purego,badlinkname,tfogo_checklinkname0,with_xhttp,with_awg,with_lx_command
Revision: deadbeef
CGO: disabled
`

const upstreamNoNaiveOutput = `sing-box version 1.12.13

Environment: go1.25.5 darwin/arm64
Tags: with_gvisor,with_quic,with_dhcp,with_wireguard,with_utls,with_acme,with_clash_api,with_tailscale
Revision: f0cd3422
CGO: disabled
`

const muslStaticNaiveOutput = `sing-box version 1.14.0-lx.3

Environment: go1.25.5 linux/amd64
Tags: with_gvisor,with_quic,with_naive_outbound,with_musl,with_xhttp,with_awg
Revision: deadbeef
CGO: enabled
`

func TestNaiveVerdictFromVersionOutput(t *testing.T) {
	cases := []struct {
		name          string
		output        string
		libAvailable  bool
		wantSupported bool
		wantInReason  string
	}{
		{"lx core with libcronet", lxVersionOutput, true, true, ""},
		{"lx core without libcronet", lxVersionOutput, false, false, "libcronet"},
		{"core built without naive tag", upstreamNoNaiveOutput, true, false, "with_naive_outbound"},
		{"static musl build needs no companion lib", muslStaticNaiveOutput, false, true, ""},
		{"no Tags line at all — assume supported", "sing-box version 1.13.0\n", false, true, ""},
		{"garbage output — assume supported", "flag provided but not defined", false, true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			supported, reason := naiveVerdictFromVersionOutput(c.output, c.libAvailable)
			if supported != c.wantSupported {
				t.Errorf("supported = %v, want %v (reason: %q)", supported, c.wantSupported, reason)
			}
			if c.wantInReason != "" && !strings.Contains(reason, c.wantInReason) {
				t.Errorf("reason = %q, want it to mention %q", reason, c.wantInReason)
			}
			if c.wantSupported && reason != "" {
				t.Errorf("reason = %q, want empty when supported", reason)
			}
		})
	}
}
