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

func TestBuildTagsFromVersionOutput(t *testing.T) {
	cases := []struct {
		name         string
		output       string
		libAvailable bool
		wantKnown    bool   // теги прочитаны
		wantIssue    string // подстрока причины у with_naive_outbound; "" — причины нет
	}{
		{"lx core with libcronet", lxVersionOutput, true, true, ""},
		{"lx core without libcronet", lxVersionOutput, false, true, "libcronet"},
		{"core built without naive tag — no issue, the tag is simply absent", upstreamNoNaiveOutput, true, true, ""},
		{"static musl build needs no companion lib", muslStaticNaiveOutput, false, true, ""},
		{"no Tags line at all — tags unknown", "sing-box version 1.13.0\n", false, false, ""},
		{"garbage output — tags unknown", "flag provided but not defined", false, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tags, issues := buildTagsFromVersionOutput(c.output, c.libAvailable)
			if (tags != nil) != c.wantKnown {
				t.Errorf("tags = %v, want known=%v", tags, c.wantKnown)
			}
			issue := issues[naiveBuildTag]
			if c.wantIssue == "" && issue != "" {
				t.Errorf("issue = %q, want none", issue)
			}
			if c.wantIssue != "" && !strings.Contains(issue, c.wantIssue) {
				t.Errorf("issue = %q, want it to mention %q", issue, c.wantIssue)
			}
		})
	}
}
