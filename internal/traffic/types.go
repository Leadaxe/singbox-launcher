// Package traffic implements SPEC 059 — TRAFFIC_PROFILER.
//
// It provides a always-on background pipeline that joins two data sources:
//
//  1. Clash API GET /connections (poll 1s) — gives the authoritative live
//     view of TCP/UDP connections, their bytes-up/down, process attribution
//     (when sing-box's route.find_process is on) and outbound chain.
//  2. sing-box.log (file tail with rotation handling) — gives DNS chain
//     reconstruction (CNAME → A record), router rule matches and the
//     `router: found process name` line which lets us attribute pre-resolve
//     events to the target process.
//
// The two streams are joined on the sing-box conn_id (the prefix
// `[<conn_id>]` that appears in most log lines and matches the `id` field in
// the Clash API response). The merged event stream feeds:
//   - A 60-second rolling buffer (for the pre-START backfill UX, §"Pre-
//     session backfill" in the SPEC).
//   - One active `Session` (when the user pressed ▶ START) plus up to five
//     completed sessions in a FIFO ring buffer.
//   - Live UI subscribers via `Subscribe()` returning a channel + unsubscribe
//     func — used by the Live system-wide view and the per-session Live
//     sub-tab.
//
// Everything is in-memory only — app quit wipes all sessions on purpose
// (LxBox parity, SPEC §"Что НЕ делаем"). No persistence path means we never
// have to worry about schema migrations for session JSON.
package traffic

import "time"

// EventKind classifies a TrafficEvent. The UI uses this both for the colored
// `DNS / DNS× / TCP / TCP· / UDP` row label and for the kind-filter chips on
// the Live view.
type EventKind string

const (
	// EventDNSResolve — sing-box log `dns: exchanged|cached <type> <domain> -> <ip|cname>`.
	// Multiple resolve events for the same conn_id form the CNAME chain
	// (final A-record terminates).
	EventDNSResolve EventKind = "DNSResolve"
	// EventDNSFail — sing-box log `dns: exchange failed for <host>: <reason>`.
	// We pair this with the IssueDnsTimeout classifier when the reason
	// matches "context deadline exceeded".
	EventDNSFail EventKind = "DNSFail"
	// EventTCPOpen — Clash API saw a new connection id (network=tcp).
	EventTCPOpen EventKind = "TCPOpen"
	// EventTCPClose — Clash API saw a connection id disappear (network=tcp).
	// Duration is filled in from `start` timestamp on the last snapshot.
	EventTCPClose EventKind = "TCPClose"
	// EventUDPOpen — Clash API saw a new connection id (network=udp).
	EventUDPOpen EventKind = "UDPOpen"
	// EventUDPClose — UDP connection disappeared.
	EventUDPClose EventKind = "UDPClose"
	// EventRouterMatch — sing-box log `router: match[<rule>] => route(<outbound>)`.
	// We don't surface this kind directly in the UI; it feeds the rule label
	// on the corresponding TCP/UDP event.
	EventRouterMatch EventKind = "RouterMatch"
)

// Confidence reflects how sure we are that an event belongs to the target
// process of a recording session — see SPEC §"Confidence levels".
type Confidence string

const (
	// ConfVerified — sing-box logged `router: found process name: <target>`
	// for the same conn_id. No room for doubt.
	ConfVerified Confidence = "verified"
	// ConfInferred — no per-conn process match but the destination IP was
	// resolved through a DNS query that *was* attributed to the target
	// within the last 10 seconds. Marked 〽 in the UI.
	ConfInferred Confidence = "inferred"
	// ConfUnattributed — no attribution strategy fired. Only shown in the
	// system-wide Live view, dimmed with a `?` marker.
	ConfUnattributed Confidence = "unattributed"
)

// IssueKind enumerates the connection diagnostics we surface as ⚠ markers.
// Kept short and locale-agnostic — LxBox §048 history demonstrates that
// auto-classifiers like "geoMismatch" or "unusualPort" are more noise than
// signal, so we resist the urge to add more.
type IssueKind string

const (
	// IssueDnsTimeout — `dns: exchange failed for <host>: ... context deadline exceeded`.
	IssueDnsTimeout IssueKind = "DnsTimeout"
	// IssueTcpRstEarly — TCP conn closed within 1s with 0 bytes both ways.
	// Indicates firewall RST, TLS handshake fail, block or unreachable.
	IssueTcpRstEarly IssueKind = "TcpRstEarly"
)

// ConnectionIssue is the one ⚠ marker attached to a TrafficEvent (or
// promoted into DomainStats.Issues when aggregating).
type ConnectionIssue struct {
	Kind        IssueKind
	Description string
}

// TrafficEvent is the unit of data flowing through the profiler. One event
// per DNS resolve / DNS fail / TCP open / TCP close / UDP open / UDP close /
// router match. Bytes counters are deltas-from-last-snapshot for the
// open/close events, *not* cumulative.
type TrafficEvent struct {
	TS             time.Time
	Kind           EventKind
	ConnID         string           // sing-box conn id; empty for events without one
	ProcessPath    string           // canonical executable path; empty if unattributed
	ProcessName    string           // short display name from Clash API metadata.process
	Confidence     Confidence       // verified / inferred / unattributed
	MatchedVia     string           // "router_log" / "prior_dns_10s" / "" — debug aid
	Domain         string           // sniffed/resolved hostname; empty for hostless
	CnameChain     []string         // accumulated CNAME chain ending in A-record IP
	IP             string
	Port           int
	Network        string           // "tcp" / "udp"
	OutboundChain  []string         // chains from Clash API; order is leaf→root
	Rule           string           // matched router rule name (if any)
	UpBytes        int64
	DownBytes      int64
	Duration       time.Duration    // only meaningful for *Close events
	Issues         []ConnectionIssue
	RawLogLine     string           // for diagnostics; not displayed by default
	Backfilled     bool             // true if copied from pre-session rolling buffer
}

// HasIssue returns true if the event carries an issue of the given kind.
// Convenience helper for tests and the issue chip widget.
func (e TrafficEvent) HasIssue(k IssueKind) bool {
	for _, iss := range e.Issues {
		if iss.Kind == k {
			return true
		}
	}
	return false
}
