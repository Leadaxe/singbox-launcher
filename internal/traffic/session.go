package traffic

import (
	"sync"
	"time"
)

// Session caps to keep memory bounded — see SPEC §"Edge cases & limits".
const (
	maxEventsPerSession = 50000
	maxSessionAge       = 3 * time.Hour
	maxCompletedSessions = 5
)

// Session is one ▶ START..⏹ STOP recording. Lives in-memory; on app quit
// everything is wiped (LxBox parity).
type Session struct {
	mu sync.RWMutex

	ID             string
	TargetProcess  string         // canonical executable path the user picked
	StartedAt      time.Time
	FinishedAt     *time.Time
	WasVerbose     bool           // log.level was bumped to debug for this run
	VerboseToggleTimes []time.Time // mid-session toggles

	events       []TrafficEvent
	eventsDropped int           // counter shown in UI footer when events overflow

	// aggregated views — rebuilt lazily on AggregateDomains/IPs/Conns.
	// Kept inside the struct so the UI can hand back a snapshot pointer
	// without exposing internal locks.
}

// NewSession builds a fresh session with the given target. ID is the
// stringified StartedAt — we don't need anything more globally-unique
// (max 6 sessions live at once, and IDs are user-visible only in the
// "Saved sessions" list, where the start time is shown anyway).
func NewSession(target string, verbose bool) *Session {
	now := time.Now()
	return &Session{
		ID:            now.Format("20060102T150405"),
		TargetProcess: target,
		StartedAt:     now,
		WasVerbose:    verbose,
		events:        make([]TrafficEvent, 0, 1024),
	}
}

// Append adds one event. Overflow drops the oldest. Stale-by-age events
// at the head are also evicted before append (3h sliding window).
func (s *Session) Append(e TrafficEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-maxSessionAge)
	for len(s.events) > 0 && s.events[0].TS.Before(cutoff) {
		s.events = s.events[1:]
		s.eventsDropped++
	}
	if len(s.events) >= maxEventsPerSession {
		s.events = s.events[1:]
		s.eventsDropped++
	}
	s.events = append(s.events, e)
}

// Events returns a copy so callers can iterate without holding the lock.
// Cheap enough for ≤50k entries on UI refresh tick.
func (s *Session) Events() []TrafficEvent {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TrafficEvent, len(s.events))
	copy(out, s.events)
	return out
}

// EventsDropped is the counter the UI footer surfaces.
func (s *Session) EventsDropped() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.eventsDropped
}

// Finalize marks the session as finished. Idempotent.
func (s *Session) Finalize() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.FinishedAt == nil {
		t := time.Now()
		s.FinishedAt = &t
	}
}

// Duration returns the wall clock duration (in-progress sessions return
// time-since-start). Convenience for the ⚡ window title timer.
func (s *Session) Duration() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.FinishedAt != nil {
		return s.FinishedAt.Sub(s.StartedAt)
	}
	return time.Since(s.StartedAt)
}

// DomainStats is one row of the Domains sub-tab — aggregated unique
// domains across the session's events, sorted by total bytes in UI.
type DomainStats struct {
	Domain      string
	Connections int
	UpBytes     int64
	DownBytes   int64
	FirstSeen   time.Time
	LastSeen    time.Time
	IPs         []string
	Outbounds   []string
	Issues      []ConnectionIssue
	CnameChain  []string // first observed chain
}

// IPStats is one row of the IPs sub-tab.
type IPStats struct {
	IP          string
	Port        int
	Connections int
	UpBytes     int64
	DownBytes   int64
	Domain      string // first observed domain that resolved to this IP, if any
	Outbounds   []string
}

// ConnRecord is one row of the Connections sub-tab — timeline view.
type ConnRecord struct {
	ConnID    string
	Domain    string
	IP        string
	Port      int
	Network   string
	OpenedAt  time.Time
	ClosedAt  *time.Time
	UpBytes   int64
	DownBytes int64
	Outbounds []string
	Rule      string
	Issues    []ConnectionIssue
}

// AggregateDomains computes Domains sub-tab rows from the current event
// list. We rebuild on each UI refresh — simpler than maintaining indices,
// and at ≤50k events the cost is microseconds.
func (s *Session) AggregateDomains() []DomainStats {
	evs := s.Events()
	byDomain := make(map[string]*DomainStats)
	for _, e := range evs {
		dom := e.Domain
		if dom == "" {
			continue
		}
		ds, ok := byDomain[dom]
		if !ok {
			ds = &DomainStats{Domain: dom, FirstSeen: e.TS, LastSeen: e.TS, CnameChain: append([]string(nil), e.CnameChain...)}
			byDomain[dom] = ds
		}
		if e.TS.Before(ds.FirstSeen) {
			ds.FirstSeen = e.TS
		}
		if e.TS.After(ds.LastSeen) {
			ds.LastSeen = e.TS
		}
		ds.UpBytes += e.UpBytes
		ds.DownBytes += e.DownBytes
		switch e.Kind {
		case EventTCPOpen, EventUDPOpen:
			ds.Connections++
		}
		if e.IP != "" {
			ds.IPs = appendUnique(ds.IPs, e.IP)
		}
		for _, ob := range e.OutboundChain {
			ds.Outbounds = appendUnique(ds.Outbounds, ob)
		}
		for _, iss := range e.Issues {
			ds.Issues = append(ds.Issues, iss)
		}
	}
	out := make([]DomainStats, 0, len(byDomain))
	for _, ds := range byDomain {
		out = append(out, *ds)
	}
	return out
}

// AggregateIPs computes IPs sub-tab rows.
func (s *Session) AggregateIPs() []IPStats {
	evs := s.Events()
	byIP := make(map[string]*IPStats)
	for _, e := range evs {
		if e.IP == "" {
			continue
		}
		key := e.IP
		ips, ok := byIP[key]
		if !ok {
			ips = &IPStats{IP: e.IP, Port: e.Port, Domain: e.Domain}
			byIP[key] = ips
		}
		ips.UpBytes += e.UpBytes
		ips.DownBytes += e.DownBytes
		switch e.Kind {
		case EventTCPOpen, EventUDPOpen:
			ips.Connections++
		}
		if ips.Domain == "" && e.Domain != "" {
			ips.Domain = e.Domain
		}
		for _, ob := range e.OutboundChain {
			ips.Outbounds = appendUnique(ips.Outbounds, ob)
		}
	}
	out := make([]IPStats, 0, len(byIP))
	for _, ips := range byIP {
		out = append(out, *ips)
	}
	return out
}

// AggregateConns computes Connections sub-tab rows. Multiple events per
// conn_id collapse into one row.
func (s *Session) AggregateConns() []ConnRecord {
	evs := s.Events()
	byID := make(map[string]*ConnRecord)
	order := make([]string, 0)
	for _, e := range evs {
		if e.ConnID == "" {
			continue
		}
		r, ok := byID[e.ConnID]
		if !ok {
			r = &ConnRecord{ConnID: e.ConnID, Domain: e.Domain, IP: e.IP, Port: e.Port, Network: e.Network}
			byID[e.ConnID] = r
			order = append(order, e.ConnID)
		}
		switch e.Kind {
		case EventTCPOpen, EventUDPOpen:
			if r.OpenedAt.IsZero() || e.TS.Before(r.OpenedAt) {
				r.OpenedAt = e.TS
			}
		case EventTCPClose, EventUDPClose:
			t := e.TS
			r.ClosedAt = &t
		}
		r.UpBytes += e.UpBytes
		r.DownBytes += e.DownBytes
		if r.Domain == "" {
			r.Domain = e.Domain
		}
		if r.IP == "" {
			r.IP = e.IP
		}
		if r.Port == 0 {
			r.Port = e.Port
		}
		if r.Network == "" {
			r.Network = e.Network
		}
		if e.Rule != "" {
			r.Rule = e.Rule
		}
		for _, ob := range e.OutboundChain {
			r.Outbounds = appendUnique(r.Outbounds, ob)
		}
		for _, iss := range e.Issues {
			r.Issues = append(r.Issues, iss)
		}
	}
	out := make([]ConnRecord, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out
}

func appendUnique(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}
