package traffic

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
)

// TestParseLogLine_KnownSamples runs the parser over the golden log file and
// asserts kind + key field for each line. If sing-box log format changes
// between releases, this test fails fast and pinpoints the regex to fix
// (rather than letting the profiler silently emit nothing).
//
// The fixture file `testdata/sing-box-logs/sample.log` is sanitized
// (no real user data) and checked into the repo via a `.gitignore`
// exception (`!internal/traffic/testdata/sing-box-logs/*.log`). The
// surrounding `*.log` rule still protects runtime logs under `bin/logs/`.
// The os.IsNotExist branch below is a belt-and-suspenders skip so the
// test degrades to SKIP (not FAIL) if the fixture ever goes missing again.
func TestParseLogLine_KnownSamples(t *testing.T) {
	path := filepath.Join("testdata", "sing-box-logs", "sample.log")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("fixture missing at %s — sanitized golden log expected to be in repo via .gitignore exception", path)
		}
		t.Fatalf("open sample log: %v", err)
	}
	defer func() { _ = f.Close() }()

	want := []struct {
		kind        EventKind
		connID      string
		domain      string
		ip          string
		cname       string
		processPath string
		rule        string
		outbound    string
		failReason  string
		port        int
	}{
		{kind: EventDNSResolve, connID: "12345", domain: "cdn.t-bank-app.ru", ip: "193.17.93.194"},
		{kind: EventDNSResolve, connID: "12345", domain: "cdn.t-bank-app.ru", ip: "2a02:6b8::1"},
		{kind: EventDNSResolve, connID: "12346", domain: "certs.t-bank-app.ru", cname: "eq09pc7nbi.a.trbcdn.net"},
		{kind: EventDNSResolve, connID: "12346", domain: "eq09pc7nbi.a.trbcdn.net", ip: "81.222.127.186"},
		{kind: EventDNSFail, connID: "12347", domain: "certs.t-bank-app.ru", failReason: "context deadline exceeded"},
		{kind: "", connID: "12348", processPath: "/Applications/Slack.app/Contents/MacOS/Slack"},
		{kind: EventRouterMatch, connID: "12348", rule: "domain_suffix=example.com", outbound: "vpn-1"},
		{kind: "", connID: "12348", ip: "1.2.3.4", port: 443},
		{kind: EventDNSResolve, connID: "12349", domain: "api.example.com", ip: "5.6.7.8"},
		{kind: "", connID: "12349", ip: "api.example.com", port: 443},
	}

	sc := bufio.NewScanner(f)
	idx := 0
	for sc.Scan() {
		line := sc.Text()
		if idx >= len(want) {
			t.Fatalf("more log lines than expectations at line %d: %q", idx, line)
		}
		got, ok := ParseLogLine(line)
		if !ok {
			t.Errorf("line %d not parsed: %q", idx, line)
			idx++
			continue
		}
		exp := want[idx]
		if got.Kind != exp.kind {
			t.Errorf("line %d kind: want %q got %q (%q)", idx, exp.kind, got.Kind, line)
		}
		if got.ConnID != exp.connID {
			t.Errorf("line %d conn: want %q got %q", idx, exp.connID, got.ConnID)
		}
		if exp.domain != "" && got.Domain != exp.domain {
			t.Errorf("line %d domain: want %q got %q", idx, exp.domain, got.Domain)
		}
		if exp.ip != "" && got.IP != exp.ip {
			t.Errorf("line %d ip: want %q got %q", idx, exp.ip, got.IP)
		}
		if exp.cname != "" && got.CnameTarget != exp.cname {
			t.Errorf("line %d cname: want %q got %q", idx, exp.cname, got.CnameTarget)
		}
		if exp.processPath != "" && got.ProcessPath != exp.processPath {
			t.Errorf("line %d processPath: want %q got %q", idx, exp.processPath, got.ProcessPath)
		}
		if exp.rule != "" && got.Rule != exp.rule {
			t.Errorf("line %d rule: want %q got %q", idx, exp.rule, got.Rule)
		}
		if exp.outbound != "" && got.Outbound != exp.outbound {
			t.Errorf("line %d outbound: want %q got %q", idx, exp.outbound, got.Outbound)
		}
		if exp.failReason != "" && got.FailReason != exp.failReason {
			t.Errorf("line %d failReason: want %q got %q", idx, exp.failReason, got.FailReason)
		}
		if exp.port != 0 && got.Port != exp.port {
			t.Errorf("line %d port: want %d got %d", idx, exp.port, got.Port)
		}
		idx++
	}
	if idx != len(want) {
		t.Errorf("expected %d lines, parsed %d", len(want), idx)
	}
}

func TestParseLogLine_Garbage(t *testing.T) {
	cases := []string{
		"",
		"foo bar baz",
		"2026-05-24 12:34:15 INFO  [42] proxy: starting", // not a known pattern
	}
	for _, c := range cases {
		if _, ok := ParseLogLine(c); ok {
			t.Errorf("garbage parsed unexpectedly: %q", c)
		}
	}
}

func TestIsDNSTimeout(t *testing.T) {
	yes := []string{
		"context deadline exceeded",
		"i/o timeout",
		"timeout awaiting response headers",
		"DNS query timeout (5s)",
	}
	no := []string{
		"server refused",
		"no such host",
		"network unreachable",
	}
	for _, r := range yes {
		if !IsDNSTimeout(r) {
			t.Errorf("want true for %q", r)
		}
	}
	for _, r := range no {
		if IsDNSTimeout(r) {
			t.Errorf("want false for %q", r)
		}
	}
}
