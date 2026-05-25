package traffic

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

// Limits for the rolling buffer — see SPEC §"Pre-session backfill".
const (
	rollingBufferWindow = 60 * time.Second
	rollingBufferMax    = 3000
	// dnsInferWindow — how long an IP→process attribution lasts for
	// "inferred" confidence (SPEC §"Confidence levels").
	dnsInferWindow = 10 * time.Second
)

// TrafficProfiler is the always-on singleton that joins Clash API
// /connections poll with sing-box.log tail, maintains a 60-second rolling
// buffer, and exposes Session lifecycle + Subscribe to the UI.
//
// Lifetime: created at app startup, runs until app quit. The Run goroutine
// drives both data sources; sessions are passive consumers via Append.
type TrafficProfiler struct {
	mu sync.Mutex

	// pipeline pieces (nil until Start)
	poller *ConnPoller
	tailer *LogTailer

	// rolling buffer of recent events (all processes)
	roll []TrafficEvent

	// active session + ring of completed
	active    *Session
	completed []*Session

	// cross-source join state
	connProcessMap map[string]string             // conn_id → process_path (from router log)
	dnsAccum       map[string][]string           // conn_id → CNAME chain (in arrival order)
	dnsByIP        map[string]dnsAttribution     // dest IP → recent DNS + process (for inferred attribution)

	// subscribers for live UI streaming
	subs map[int]chan TrafficEvent
	nextSub int

	// lifecycle hooks (window title timer / button label badge)
	onSessionChange func()

	stopCh chan struct{}

	// background context for poller/tailer
	bgCtx    context.Context
	bgCancel context.CancelFunc
}

// dnsAttribution remembers the most recent DNS query result so that a TCP
// open with no router process-match can be attributed to the same process
// via destination IP. Window = dnsInferWindow.
type dnsAttribution struct {
	ProcessPath string
	Domain      string
	At          time.Time
}

// NewTrafficProfiler creates the singleton. Caller is responsible for
// invoking Start exactly once.
func NewTrafficProfiler() *TrafficProfiler {
	return &TrafficProfiler{
		roll:           make([]TrafficEvent, 0, rollingBufferMax),
		connProcessMap: make(map[string]string),
		dnsAccum:       make(map[string][]string),
		dnsByIP:        make(map[string]dnsAttribution),
		subs:           make(map[int]chan TrafficEvent),
	}
}

// SetOnSessionChange registers a callback fired (off the profiler lock)
// whenever the active session starts/stops. UI uses this to update window
// title + Diagnostics button label.
func (p *TrafficProfiler) SetOnSessionChange(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onSessionChange = fn
}

// Start wires up the poller and tailer with the given config providers.
// Idempotent — second call is a no-op. Pass a real http.Client (e.g.
// api.getHTTPClient()) so we reuse the existing transport.
func (p *TrafficProfiler) Start(cfg ClashConfigProvider, logPath string, httpc HTTPClientLike) {
	p.mu.Lock()
	if p.poller != nil {
		p.mu.Unlock()
		return
	}
	p.bgCtx, p.bgCancel = context.WithCancel(context.Background())
	p.poller = NewConnPoller(cfg, asStdHTTP(httpc))
	p.tailer = NewLogTailer(logPath)
	p.mu.Unlock()

	go p.poller.Run(p.bgCtx)
	go p.tailer.Run(p.bgCtx)
	go p.runJoin(p.bgCtx)
}

// Stop tears down the goroutines. Mainly for tests; in production the app
// process exit handles cleanup.
func (p *TrafficProfiler) Stop() {
	p.mu.Lock()
	if p.bgCancel != nil {
		p.bgCancel()
		p.bgCancel = nil
	}
	p.mu.Unlock()
}

// runJoin is the central event loop. Reads from poller.Out and tailer.Out,
// produces TrafficEvents, fans out to rolling buffer + active session +
// subscribers.
func (p *TrafficProfiler) runJoin(ctx context.Context) {
	pollerCh := p.poller.Out()
	tailerCh := p.tailer.Out()
	for {
		select {
		case <-ctx.Done():
			return
		case delta, ok := <-pollerCh:
			if !ok {
				return
			}
			for _, e := range p.eventsFromPoller(delta) {
				p.dispatch(e)
			}
		case ll, ok := <-tailerCh:
			if !ok {
				return
			}
			for _, e := range p.eventsFromLogLine(ll) {
				p.dispatch(e)
			}
		}
	}
}

// eventsFromPoller translates one ConnDelta into 0..N TrafficEvents. Bytes
// updates produce no event (would flood the UI) — instead, the next
// TCPClose carries final totals; live byte counters are read on demand
// from the rolling Conn snapshot if we ever need them. For MVP we emit on
// open + close only.
func (p *TrafficProfiler) eventsFromPoller(d ConnDelta) []TrafficEvent {
	out := make([]TrafficEvent, 0, len(d.Opened)+len(d.Closed))
	for _, c := range d.Opened {
		e := p.eventFromConn(c, d.At, false)
		if c.Metadata.Network == "udp" {
			e.Kind = EventUDPOpen
		} else {
			e.Kind = EventTCPOpen
		}
		out = append(out, e)

		// Update conn→process map (Clash already knows process via
		// find_process; we mirror so log-only consumers can attribute).
		if c.Metadata.ProcessPath != "" {
			p.mu.Lock()
			p.connProcessMap[c.ID] = c.Metadata.ProcessPath
			// And remember IP→process for the 10s inferred-attribution
			// window. Helps when sing-box can't attribute a follow-up
			// raw-IP connection.
			p.dnsByIP[c.Metadata.DestinationIP] = dnsAttribution{
				ProcessPath: c.Metadata.ProcessPath,
				Domain:      c.Metadata.Host,
				At:          d.At,
			}
			p.mu.Unlock()
		}
	}
	for _, cc := range d.Closed {
		e := p.eventFromConn(cc.Conn, d.At, true)
		if cc.Conn.Metadata.Network == "udp" {
			e.Kind = EventUDPClose
		} else {
			e.Kind = EventTCPClose
		}
		e.Duration = cc.Duration
		// Issue: TCP RST early — <1s, both byte counters 0.
		if e.Kind == EventTCPClose && cc.Duration > 0 && cc.Duration < time.Second &&
			cc.Conn.Upload == 0 && cc.Conn.Download == 0 {
			e.Issues = append(e.Issues, ConnectionIssue{
				Kind:        IssueTcpRstEarly,
				Description: "TCP closed within 1s with 0 bytes transferred",
			})
		}
		out = append(out, e)
	}
	return out
}

// eventFromConn builds the shared field-set for an open/close event.
func (p *TrafficProfiler) eventFromConn(c ClashConn, at time.Time, _ bool) TrafficEvent {
	e := TrafficEvent{
		TS:            at,
		ConnID:        c.ID,
		ProcessPath:   c.Metadata.ProcessPath,
		ProcessName:   c.Metadata.Process,
		Domain:        c.Metadata.Host,
		IP:            c.Metadata.DestinationIP,
		Port:          c.Metadata.PortInt(),
		Network:       c.Metadata.Network,
		OutboundChain: append([]string(nil), c.Chains...),
		Rule:          c.Rule,
		UpBytes:       c.Upload,
		DownBytes:     c.Download,
	}
	if e.ProcessPath != "" {
		e.Confidence = ConfVerified
		e.MatchedVia = "router_log"
	} else {
		// Try inferred via prior DNS-by-IP.
		p.mu.Lock()
		if att, ok := p.dnsByIP[e.IP]; ok && at.Sub(att.At) < dnsInferWindow {
			e.ProcessPath = att.ProcessPath
			e.Confidence = ConfInferred
			e.MatchedVia = "prior_dns_10s"
		} else {
			e.Confidence = ConfUnattributed
		}
		p.mu.Unlock()
	}
	// Attach CNAME chain we've accumulated for this conn.
	p.mu.Lock()
	if ch, ok := p.dnsAccum[c.ID]; ok && len(ch) > 0 {
		e.CnameChain = append([]string(nil), ch...)
	}
	p.mu.Unlock()
	return e
}

// eventsFromLogLine converts a parsed LogLine to 0..N TrafficEvents and
// updates internal join state (process map, CNAME accum).
func (p *TrafficProfiler) eventsFromLogLine(ll LogLine) []TrafficEvent {
	// Update join state first regardless of kind.
	if ll.ProcessPath != "" && ll.ConnID != "" {
		p.mu.Lock()
		p.connProcessMap[ll.ConnID] = ll.ProcessPath
		p.mu.Unlock()
	}

	switch ll.Kind {
	case EventDNSResolve:
		// Accumulate CNAME chain.
		if ll.CnameTarget != "" {
			p.mu.Lock()
			p.dnsAccum[ll.ConnID] = appendUnique(p.dnsAccum[ll.ConnID], ll.CnameTarget)
			p.mu.Unlock()
		}
		e := TrafficEvent{
			TS:         ll.TS,
			Kind:       EventDNSResolve,
			ConnID:     ll.ConnID,
			Domain:     ll.Domain,
			IP:         ll.IP,
			RawLogLine: ll.Raw,
		}
		p.fillAttribution(&e)
		// Remember IP→process for inferred attribution downstream.
		if e.ProcessPath != "" && ll.IP != "" {
			p.mu.Lock()
			p.dnsByIP[ll.IP] = dnsAttribution{
				ProcessPath: e.ProcessPath,
				Domain:      ll.Domain,
				At:          ll.TS,
			}
			p.mu.Unlock()
		}
		// Snapshot accumulated chain on the event.
		p.mu.Lock()
		if ch, ok := p.dnsAccum[ll.ConnID]; ok && len(ch) > 0 {
			e.CnameChain = append([]string(nil), ch...)
		}
		p.mu.Unlock()
		return []TrafficEvent{e}

	case EventDNSFail:
		e := TrafficEvent{
			TS:         ll.TS,
			Kind:       EventDNSFail,
			ConnID:     ll.ConnID,
			Domain:     ll.Domain,
			RawLogLine: ll.Raw,
		}
		if IsDNSTimeout(ll.FailReason) {
			e.Issues = append(e.Issues, ConnectionIssue{
				Kind:        IssueDnsTimeout,
				Description: ll.FailReason,
			})
		}
		p.fillAttribution(&e)
		return []TrafficEvent{e}

	case EventRouterMatch:
		// Not surfaced as its own row — feeds the rule field on later TCP
		// events. We do nothing else here.
		return nil
	default:
		// Process-name lines have empty Kind; consumed as join state above.
		return nil
	}
}

func (p *TrafficProfiler) fillAttribution(e *TrafficEvent) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if proc, ok := p.connProcessMap[e.ConnID]; ok {
		e.ProcessPath = proc
		e.Confidence = ConfVerified
		e.MatchedVia = "router_log"
		return
	}
	e.Confidence = ConfUnattributed
}

// dispatch appends to rolling buffer, active session (if match), and
// fans out to subscribers. Subscribers are non-blocking — dropping on a
// full channel rather than wedging the join loop is the right tradeoff.
func (p *TrafficProfiler) dispatch(e TrafficEvent) {
	p.mu.Lock()
	// Rolling buffer GC.
	cutoff := time.Now().Add(-rollingBufferWindow)
	for len(p.roll) > 0 && p.roll[0].TS.Before(cutoff) {
		p.roll = p.roll[1:]
	}
	if len(p.roll) >= rollingBufferMax {
		p.roll = p.roll[1:]
	}
	p.roll = append(p.roll, e)

	active := p.active
	// Snapshot subscriber list to fan out without holding lock.
	subs := make([]chan TrafficEvent, 0, len(p.subs))
	for _, ch := range p.subs {
		subs = append(subs, ch)
	}
	p.mu.Unlock()

	if active != nil && eventMatchesSession(e, active.TargetProcess) {
		active.Append(e)
	}
	for _, ch := range subs {
		select {
		case ch <- e:
		default:
			// Subscriber falling behind — drop.
		}
	}
}

// eventMatchesSession returns true if the event should be attributed to a
// session targeting the given process path. We accept verified and
// inferred matches; unattributed events are filed in the rolling buffer
// but not the session.
func eventMatchesSession(e TrafficEvent, target string) bool {
	if target == "" {
		return true // session with no target = system-wide; not our default but supported
	}
	if e.ProcessPath == "" {
		return false
	}
	return e.ProcessPath == target
}

// ============================================================
// Public session API
// ============================================================

// ErrSessionAlreadyActive is returned when StartSession is called while
// another session is in progress. The UI is responsible for asking the
// user to stop first.
var ErrSessionAlreadyActive = errors.New("traffic: a session is already active")

// StartSession opens a new recording for the given process. Pre-session
// backfill (last 60s of matching events from the rolling buffer) is
// copied into the new session with Backfilled=true.
func (p *TrafficProfiler) StartSession(target string, wasVerbose bool) (*Session, error) {
	p.mu.Lock()
	if p.active != nil {
		p.mu.Unlock()
		return nil, ErrSessionAlreadyActive
	}
	sess := NewSession(target, wasVerbose)

	// Pre-session backfill: copy rolling-buffer events that match the
	// target into the new session, marked Backfilled.
	for _, e := range p.roll {
		if eventMatchesSession(e, target) {
			e.Backfilled = true
			sess.events = append(sess.events, e) // direct, before publish (no lock contention)
		}
	}
	p.active = sess
	cb := p.onSessionChange
	p.mu.Unlock()

	if cb != nil {
		cb()
	}
	return sess, nil
}

// StopSession finalizes the active session and pushes it into the ring of
// completed sessions (FIFO max 5). Returns the finalized session for the
// caller's convenience.
func (p *TrafficProfiler) StopSession() (*Session, error) {
	p.mu.Lock()
	if p.active == nil {
		p.mu.Unlock()
		return nil, errors.New("traffic: no active session")
	}
	sess := p.active
	sess.Finalize()
	p.active = nil
	p.completed = append(p.completed, sess)
	if len(p.completed) > maxCompletedSessions {
		p.completed = p.completed[len(p.completed)-maxCompletedSessions:]
	}
	cb := p.onSessionChange
	p.mu.Unlock()
	if cb != nil {
		cb()
	}
	return sess, nil
}

// ActiveSession returns the in-progress session or nil.
func (p *TrafficProfiler) ActiveSession() *Session {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.active
}

// CompletedSessions returns a snapshot of the ring (newest last).
func (p *TrafficProfiler) CompletedSessions() []*Session {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]*Session, len(p.completed))
	copy(out, p.completed)
	return out
}

// DeleteSession removes a finalized session from the ring. Active sessions
// can't be deleted (Stop first).
func (p *TrafficProfiler) DeleteSession(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.completed[:0]
	for _, s := range p.completed {
		if s.ID != id {
			out = append(out, s)
		}
	}
	p.completed = out
}

// ClearAll drops all completed sessions. Active is left alone.
func (p *TrafficProfiler) ClearAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.completed = nil
}

// Snapshot returns events from the rolling buffer within the last d.
// Used by the Live system-wide view to populate the initial scrollback
// when the user opens the window.
func (p *TrafficProfiler) Snapshot(d time.Duration) []TrafficEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	cutoff := time.Now().Add(-d)
	out := make([]TrafficEvent, 0, len(p.roll))
	for _, e := range p.roll {
		if !e.TS.Before(cutoff) {
			out = append(out, e)
		}
	}
	return out
}

// Subscribe returns a channel of new TrafficEvents and an unsubscribe
// function. The channel is buffered; a slow consumer simply drops.
func (p *TrafficProfiler) Subscribe() (<-chan TrafficEvent, func()) {
	ch := make(chan TrafficEvent, 256)
	p.mu.Lock()
	id := p.nextSub
	p.nextSub++
	p.subs[id] = ch
	p.mu.Unlock()
	unsub := func() {
		p.mu.Lock()
		delete(p.subs, id)
		p.mu.Unlock()
		// Don't close ch — Subscribe caller may still be ranging on it;
		// dropping the reference is enough for GC.
	}
	return ch, unsub
}

// ============================================================
// Helpers for the UI to walk processes seen in the rolling buffer
// (used by the "Filter by process" panel + process picker fallback).
// ============================================================

// ProcessSummary is what the picker UI shows in the "recently seen
// processes" hint section, when the OS-level enumeration is empty or the
// user wants to pick by activity.
type ProcessSummary struct {
	Path        string
	DisplayName string
	LastSeen    time.Time
	Events      int
}

// SeenProcesses returns processes attributed in the rolling buffer,
// newest-active first.
func (p *TrafficProfiler) SeenProcesses() []ProcessSummary {
	p.mu.Lock()
	defer p.mu.Unlock()
	by := make(map[string]*ProcessSummary)
	for _, e := range p.roll {
		if e.ProcessPath == "" {
			continue
		}
		ps, ok := by[e.ProcessPath]
		if !ok {
			ps = &ProcessSummary{Path: e.ProcessPath, DisplayName: e.ProcessName}
			by[e.ProcessPath] = ps
		}
		ps.Events++
		if e.TS.After(ps.LastSeen) {
			ps.LastSeen = e.TS
		}
	}
	out := make([]ProcessSummary, 0, len(by))
	for _, ps := range by {
		out = append(out, *ps)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeen.After(out[j].LastSeen)
	})
	return out
}

// FormatRecordingTitle is the window title with timer (⚡ Recording · mm:ss).
// Centralized so the live-update tick and the initial setup agree.
func FormatRecordingTitle(s *Session) string {
	if s == nil {
		return "Traffic Profiler"
	}
	d := s.Duration()
	m := int(d.Minutes())
	sec := int(d.Seconds()) % 60
	return fmt.Sprintf("Traffic Profiler ⚡ Recording · %02d:%02d", m, sec)
}
