package traffic

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LogLine is the parsed-but-not-yet-correlated representation of one
// sing-box.log line. The profiler joins LogLine events with Clash API
// snapshots on ConnID — see profiler.go.
//
// We keep this struct small and side-effect-free so the parser package can
// be exercised by golden tests without bringing in fsnotify or HTTP.
type LogLine struct {
	TS         time.Time
	Raw        string
	ConnID     string
	Kind       EventKind
	Domain     string  // for DNSResolve / DNSFail
	IP         string  // for DNSResolve (final A-record target) and TCP/UDP
	CnameTarget string // non-empty when DNSResolve produced a CNAME instead of A
	Port       int    // for inbound/tun outbound-connection line
	ProcessPath string // for `router: found process name: <path>` lines
	Rule       string // for `router: match[<rule>] => route(<outbound>)`
	Outbound   string // ditto
	FailReason string // for DNSFail
}

// Sing-box logs are not stable across minor versions — these regexes are
// intentionally permissive (whitespace, optional sub-bracket groups) and
// covered by parser_test.go against snapshots in testdata/sing-box-logs.
//
// Reference samples (sing-box 1.13.11):
//   `2026-05-24 12:34:15 INFO  [12345] dns: exchanged AAAA cdn.t-bank.ru. -> 2a02:...`
//   `2026-05-24 12:34:15 INFO  [12345] dns: cached A cdn.t-bank.ru. -> 81.222.127.186`
//   `2026-05-24 12:34:15 INFO  [12345] dns: exchange failed for certs.t-bank.ru: ... context deadline exceeded`
//   `2026-05-24 12:34:15 INFO  [12345] router: found process name: /Applications/Slack.app/Contents/MacOS/Slack`
//   `2026-05-24 12:34:15 INFO  [12345] router: match[domain_suffix=example.com] => route(vpn-1)`
//   `2026-05-24 12:34:15 INFO  [12345] inbound/tun[tun-in]: outbound connection to 1.2.3.4:443`
//
// The conn-id capture allows both numeric (`123`) and uuid-shape (`abcd-1234`)
// — sing-box has used both depending on build flags.
var (
	connIDInner = `([0-9A-Za-z._-]+)`

	reDNSExchanged = regexp.MustCompile(
		`\[` + connIDInner + `\]\s+dns:\s+(?:exchanged|cached)\s+([A-Z]+)\s+([^\s]+?)\.?\s+->\s+([^\s]+?)\.?\s*$`,
	)

	reDNSFailed = regexp.MustCompile(
		`\[` + connIDInner + `\]\s+dns:\s+exchange failed for\s+([^:]+):\s*(.+?)\s*$`,
	)

	reRouterProcess = regexp.MustCompile(
		`\[` + connIDInner + `\]\s+router:\s+found process name:\s+(.+?)\s*$`,
	)

	reRouterMatch = regexp.MustCompile(
		`\[` + connIDInner + `\]\s+router:\s+match\[(.+?)\]\s+=>\s+route\(([^)]+)\)\s*$`,
	)

	reInboundOut = regexp.MustCompile(
		`\[` + connIDInner + `\]\s+inbound/[^:]+:\s+outbound connection to\s+([^\s:]+):(\d+)\s*$`,
	)

	// Optional leading timestamp. sing-box config.log.timestamp=true emits a
	// `2026-05-24 12:34:15` prefix before the level word. We don't fail if
	// it's missing (tests cover both shapes).
	reLeadingTS = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?)\s+`)
)

// timestampLayouts the parser tries in order. The fractional-seconds variant
// covers `log.timestamp_format` overrides.
var timestampLayouts = []string{
	"2006-01-02 15:04:05",
	"2006-01-02 15:04:05.000",
	"2006-01-02 15:04:05.000000",
}

// ipLooksLikeIP returns true if s parses as either v4 (dotted) or v6 (has a
// `:`). The DNS resolved value is either an IP (terminal A/AAAA) or a CNAME
// — distinguishing the two lets the profiler build a proper CNAME chain.
func ipLooksLikeIP(s string) bool {
	if strings.Contains(s, ":") {
		return true // crude but enough for v6 vs domain
	}
	dots := strings.Count(s, ".")
	if dots != 3 {
		return false
	}
	for _, part := range strings.Split(s, ".") {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > 255 {
			return false
		}
	}
	return true
}

// ParseLogLine attempts every known pattern in order. Returns ok=false (and
// a zero-value LogLine) if nothing matched — the profiler then ignores the
// line. We never return errors: log content is best-effort source.
func ParseLogLine(line string) (LogLine, bool) {
	out := LogLine{Raw: line}

	if m := reLeadingTS.FindStringSubmatch(line); m != nil {
		for _, layout := range timestampLayouts {
			if t, err := time.ParseInLocation(layout, m[1], time.Local); err == nil {
				out.TS = t
				break
			}
		}
	}

	if m := reDNSExchanged.FindStringSubmatch(line); m != nil {
		out.Kind = EventDNSResolve
		out.ConnID = m[1]
		out.Domain = strings.TrimSuffix(m[3], ".")
		target := strings.TrimSuffix(m[4], ".")
		if ipLooksLikeIP(target) {
			out.IP = target
		} else {
			out.CnameTarget = target
		}
		return out, true
	}

	if m := reDNSFailed.FindStringSubmatch(line); m != nil {
		out.Kind = EventDNSFail
		out.ConnID = m[1]
		out.Domain = strings.TrimSpace(m[2])
		out.FailReason = strings.TrimSpace(m[3])
		return out, true
	}

	if m := reRouterProcess.FindStringSubmatch(line); m != nil {
		// We synthesize Kind=RouterMatch only for the actual rule line — this
		// one is consumed by the profiler's conn→process map, not emitted as
		// a TrafficEvent.
		out.ConnID = m[1]
		out.ProcessPath = strings.TrimSpace(m[2])
		return out, true
	}

	if m := reRouterMatch.FindStringSubmatch(line); m != nil {
		out.Kind = EventRouterMatch
		out.ConnID = m[1]
		out.Rule = strings.TrimSpace(m[2])
		out.Outbound = strings.TrimSpace(m[3])
		return out, true
	}

	if m := reInboundOut.FindStringSubmatch(line); m != nil {
		out.ConnID = m[1]
		out.IP = strings.TrimSpace(m[2])
		if p, err := strconv.Atoi(m[3]); err == nil {
			out.Port = p
		}
		return out, true
	}

	return out, false
}

// IsDNSTimeout returns true if the failure reason matches the patterns we
// classify as IssueDnsTimeout. We accept the common variants sing-box / Go
// stdlib produce: "context deadline exceeded", "i/o timeout", "timeout
// awaiting...". Kept liberal because a false positive is harmless (it just
// surfaces a ⚠ marker the user can dismiss).
func IsDNSTimeout(reason string) bool {
	r := strings.ToLower(reason)
	switch {
	case strings.Contains(r, "context deadline exceeded"):
		return true
	case strings.Contains(r, "i/o timeout"):
		return true
	case strings.Contains(r, "timeout"):
		return true
	}
	return false
}
