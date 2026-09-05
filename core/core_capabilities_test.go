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

// SPEC 122: тот же гейт по тегу для tailscale. Политика та же — деградируем
// только по положительному свидетельству (строка Tags есть, тега в ней нет);
// любая неопределённость → «умеет».
func TestTailscaleVerdictFromVersionOutput(t *testing.T) {
	// lx.31: тег есть.
	const lx31Output = `sing-box version 1.14.0-lx.31

Environment: go1.25.12 darwin/amd64
Tags: with_gvisor,with_quic,with_wireguard,with_utls,with_clash_api,with_naive_outbound,with_awg,with_lx_chain,with_tailscale
Revision: deadbeef
CGO: disabled
`
	cases := []struct {
		name          string
		output        string
		wantSupported bool
		wantInReason  string
	}{
		{"core with with_tailscale", lx31Output, true, ""},
		{"upstream core also carries the tag", upstreamNoNaiveOutput, true, ""},
		{"lx core without the tag", lxVersionOutput, false, "with_tailscale"},
		{"lx core without the tag names the release", lxVersionOutput, false, "1.14.0-lx.31"},
		{"no Tags line at all — assume supported", "sing-box version 1.13.0\n", true, ""},
		{"garbage output — assume supported", "flag provided but not defined", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			supported, reason := tailscaleVerdictFromVersionOutput(c.output)
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

// SPEC 123: гейт полей AmneziaWG 3.x. Здесь одного тега мало — `with_awg`
// есть и у старых ядер, — поэтому вердикт складывается из тега И версии.
// Политика прежняя: деградируем только по положительному свидетельству.
func TestAWG3VerdictFromVersionOutput(t *testing.T) {
	const lx32Output = `sing-box version 1.14.0-lx.32

Environment: go1.25.12 darwin/amd64
Tags: with_gvisor,with_quic,with_wireguard,with_utls,with_clash_api,with_awg,with_lx_chain,with_tailscale
Revision: deadbeef
CGO: disabled
`
	const lx32rcOutput = `sing-box version 1.14.0-lx.32-rc.1

Environment: go1.25.12 darwin/amd64
Tags: with_gvisor,with_quic,with_wireguard,with_awg
Revision: deadbeef
CGO: disabled
`
	const lx31Output = `sing-box version 1.14.0-lx.31

Environment: go1.25.12 darwin/amd64
Tags: with_gvisor,with_quic,with_wireguard,with_awg,with_tailscale
Revision: deadbeef
CGO: disabled
`
	const upstream115Output = `sing-box version 1.15.0

Environment: go1.25.12 darwin/arm64
Tags: with_gvisor,with_quic,with_wireguard,with_utls,with_clash_api
Revision: f0cd3422
CGO: disabled
`
	cases := []struct {
		name          string
		output        string
		wantSupported bool
		wantInReason  string
	}{
		{"lx.32 — граница поддержки", lx32Output, true, ""},
		{"rc поверх lx.32 тоже умеет", lx32rcOutput, true, ""},
		{"lx.31 с with_awg — не умеет", lx31Output, false, "1.14.0-lx.32"},
		{"lx.31 — причина называет версию ядра", lx31Output, false, "1.14.0-lx.31"},
		{"upstream 1.15.0 без with_awg", upstream115Output, false, awgBuildTag},
		{"нет строки Tags — считаем, что умеет", "sing-box version 1.13.0\n", true, ""},
		{"мусор в выводе — считаем, что умеет", "flag provided but not defined", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			supported, reason := awg3VerdictFromVersionOutput(c.output)
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
