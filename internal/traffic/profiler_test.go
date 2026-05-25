package traffic

import (
	"testing"
	"time"
)

func TestProfiler_SessionLifecycle(t *testing.T) {
	p := NewTrafficProfiler()
	if p.ActiveSession() != nil {
		t.Fatal("new profiler should have no active session")
	}
	s, err := p.StartSession("/Apps/Slack.app", false)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if got := p.ActiveSession(); got != s {
		t.Errorf("ActiveSession mismatch")
	}
	// Double-start should fail.
	if _, err := p.StartSession("/Apps/Slack.app", false); err != ErrSessionAlreadyActive {
		t.Errorf("expected ErrSessionAlreadyActive, got %v", err)
	}
	done, err := p.StopSession()
	if err != nil {
		t.Fatalf("StopSession: %v", err)
	}
	if done != s {
		t.Errorf("StopSession returned different session")
	}
	if done.FinishedAt == nil {
		t.Errorf("session not finalized")
	}
	if p.ActiveSession() != nil {
		t.Errorf("active session not cleared")
	}
	comp := p.CompletedSessions()
	if len(comp) != 1 || comp[0] != s {
		t.Errorf("completed ring: %+v", comp)
	}
}

func TestProfiler_CompletedRingCapAt5(t *testing.T) {
	p := NewTrafficProfiler()
	for i := 0; i < 8; i++ {
		_, _ = p.StartSession("/x", false)
		_, _ = p.StopSession()
	}
	if got := len(p.CompletedSessions()); got != maxCompletedSessions {
		t.Errorf("want %d completed, got %d", maxCompletedSessions, got)
	}
}

func TestProfiler_RollingBufferBackfill(t *testing.T) {
	p := NewTrafficProfiler()
	now := time.Now()
	// Push two events into rolling buffer for /Apps/Slack and one for
	// /Apps/Other before starting a session.
	p.roll = []TrafficEvent{
		{TS: now.Add(-5 * time.Second), ProcessPath: "/Apps/Slack", IP: "1.1.1.1", Confidence: ConfVerified},
		{TS: now.Add(-3 * time.Second), ProcessPath: "/Apps/Other", IP: "2.2.2.2", Confidence: ConfVerified},
		{TS: now.Add(-1 * time.Second), ProcessPath: "/Apps/Slack", IP: "3.3.3.3", Confidence: ConfVerified},
	}
	s, err := p.StartSession("/Apps/Slack", false)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	evs := s.Events()
	if len(evs) != 2 {
		t.Fatalf("want 2 backfilled events, got %d", len(evs))
	}
	for _, e := range evs {
		if !e.Backfilled {
			t.Errorf("event not marked backfilled: %+v", e)
		}
	}
}

func TestProfiler_DispatchAttribution(t *testing.T) {
	p := NewTrafficProfiler()
	// Seed conn→process map; simulates a `router: found process name` log
	// arriving before the Clash poll for the same conn.
	p.connProcessMap["c1"] = "/Apps/Slack"

	_, _ = p.StartSession("/Apps/Slack", false)

	// Dispatch a DNS event via the eventsFromLogLine path: it should pick
	// up the process from the map.
	out := p.eventsFromLogLine(LogLine{
		TS:     time.Now(),
		Kind:   EventDNSResolve,
		ConnID: "c1",
		Domain: "example.com",
		IP:     "8.8.8.8",
	})
	if len(out) != 1 {
		t.Fatalf("want 1 event, got %d", len(out))
	}
	e := out[0]
	if e.ProcessPath != "/Apps/Slack" {
		t.Errorf("attribution failed: %+v", e)
	}
	if e.Confidence != ConfVerified {
		t.Errorf("want verified, got %s", e.Confidence)
	}

	// Dispatch into the session through dispatch().
	p.dispatch(e)
	if len(p.ActiveSession().Events()) == 0 {
		t.Errorf("event did not land in session")
	}
}

func TestProfiler_IssueClassification_TcpRstEarly(t *testing.T) {
	p := NewTrafficProfiler()
	now := time.Now()
	d := ConnDelta{
		At: now,
		Closed: []ClashConnClosed{
			{
				Conn: ClashConn{
					ID:       "x",
					Upload:   0,
					Download: 0,
					Metadata: ClashConnMeta{Network: "tcp", Host: "evil.tld", DestinationIP: "9.9.9.9"},
				},
				Duration: 200 * time.Millisecond,
			},
		},
	}
	events := p.eventsFromPoller(d)
	if len(events) != 1 {
		t.Fatalf("want 1 close event, got %d", len(events))
	}
	e := events[0]
	if !e.HasIssue(IssueTcpRstEarly) {
		t.Errorf("want TcpRstEarly issue, got %+v", e.Issues)
	}
	if e.Kind != EventTCPClose {
		t.Errorf("want TCPClose, got %s", e.Kind)
	}
}

func TestProfiler_IssueClassification_DnsTimeout(t *testing.T) {
	p := NewTrafficProfiler()
	out := p.eventsFromLogLine(LogLine{
		TS:         time.Now(),
		Kind:       EventDNSFail,
		ConnID:     "z",
		Domain:     "example.com",
		FailReason: "DNS error: context deadline exceeded",
	})
	if len(out) != 1 {
		t.Fatalf("want 1 event, got %d", len(out))
	}
	if !out[0].HasIssue(IssueDnsTimeout) {
		t.Errorf("want DnsTimeout issue, got %+v", out[0].Issues)
	}
}

func TestProfiler_InferredAttribution_PriorDNS(t *testing.T) {
	p := NewTrafficProfiler()
	now := time.Now()
	// Seed a DNS-by-IP record from 2s ago for Slack.
	p.dnsByIP["1.2.3.4"] = dnsAttribution{
		ProcessPath: "/Apps/Slack",
		Domain:      "api.slack.com",
		At:          now.Add(-2 * time.Second),
	}
	// Clash poll says: new TCP open with no processPath. Should infer
	// Slack via the recent DNS.
	d := ConnDelta{
		At: now,
		Opened: []ClashConn{
			{ID: "tt", Metadata: ClashConnMeta{Network: "tcp", DestinationIP: "1.2.3.4", DestinationPort: "443"}},
		},
	}
	events := p.eventsFromPoller(d)
	if len(events) != 1 || events[0].ProcessPath != "/Apps/Slack" || events[0].Confidence != ConfInferred {
		t.Fatalf("inferred attribution failed: %+v", events)
	}
}

func TestProfiler_Snapshot(t *testing.T) {
	p := NewTrafficProfiler()
	now := time.Now()
	p.roll = []TrafficEvent{
		{TS: now.Add(-90 * time.Second), Domain: "old.com"},
		{TS: now.Add(-30 * time.Second), Domain: "mid.com"},
		{TS: now.Add(-5 * time.Second), Domain: "recent.com"},
	}
	out := p.Snapshot(60 * time.Second)
	if len(out) != 2 {
		t.Fatalf("Snapshot(60s) want 2, got %d", len(out))
	}
	if out[0].Domain != "mid.com" || out[1].Domain != "recent.com" {
		t.Errorf("Snapshot order: %+v", out)
	}
}

func TestProfiler_SubscribeUnsubscribe(t *testing.T) {
	p := NewTrafficProfiler()
	ch, unsub := p.Subscribe()
	p.dispatch(TrafficEvent{Domain: "x.com", TS: time.Now()})
	select {
	case e := <-ch:
		if e.Domain != "x.com" {
			t.Errorf("got wrong event: %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("did not receive event")
	}
	unsub()
	// dispatch after unsub — should not panic, may or may not deliver
	// (we may have buffered one); no assertion needed beyond no-panic.
	p.dispatch(TrafficEvent{Domain: "y.com", TS: time.Now()})
}
