//go:build darwin

package core

import (
	"testing"

	daemonpb "singbox-launcher/internal/daemonpb"
)

func TestProtoConnToClash(t *testing.T) {
	c := &daemonpb.Connection{
		Id:            "conn-1",
		Network:       "tcp",
		Destination:   "1.2.3.4:443",
		Domain:        "example.com",
		UplinkTotal:   1000,
		DownlinkTotal: 2000,
		CreatedAt:     1_700_000_000_000, // ms
		ChainList:     []string{"proxy-out", "direct"},
		Rule:          "domain-suffix",
		ProcessInfo:   &daemonpb.ProcessInfo{ProcessPath: "/Applications/Foo.app/Contents/MacOS/foo"},
	}
	got := protoConnToClash(c)
	if got.ID != "conn-1" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.Upload != 1000 || got.Download != 2000 {
		t.Errorf("bytes = %d/%d", got.Upload, got.Download)
	}
	if got.Metadata.Host != "example.com" {
		t.Errorf("host = %q", got.Metadata.Host)
	}
	if got.Metadata.DestinationIP != "1.2.3.4" {
		t.Errorf("destIP = %q", got.Metadata.DestinationIP)
	}
	if got.Metadata.DestinationPort != "443" {
		t.Errorf("port = %q", got.Metadata.DestinationPort)
	}
	if got.Metadata.Process != "foo" {
		t.Errorf("process basename = %q, want foo", got.Metadata.Process)
	}
	if got.Start.IsZero() {
		t.Error("start time not parsed")
	}
	if len(got.Chains) != 2 {
		t.Errorf("chains = %v", got.Chains)
	}
}

func TestConnTrackerLifecycle(t *testing.T) {
	tr := newConnTracker()

	// До первого события — ok=false (пустой снимок не значит «всё закрылось»).
	if _, ok := tr.snapshot(nil); ok {
		t.Fatal("snapshot ok=true before any event")
	}

	// NEW добавляет.
	tr.mu.Lock()
	tr.live = true
	tr.mu.Unlock()
	tr.applyEvent(&daemonpb.ConnectionEvent{
		Type:       daemonpb.ConnectionEventType_CONNECTION_EVENT_NEW,
		Connection: &daemonpb.Connection{Id: "a", Destination: "1.1.1.1:80"},
	})
	snap, ok := tr.snapshot(nil)
	if !ok || len(snap) != 1 {
		t.Fatalf("after NEW: ok=%v len=%d", ok, len(snap))
	}

	// UPDATE обновляет байты.
	tr.applyEvent(&daemonpb.ConnectionEvent{
		Type:       daemonpb.ConnectionEventType_CONNECTION_EVENT_UPDATE,
		Connection: &daemonpb.Connection{Id: "a", Destination: "1.1.1.1:80", UplinkTotal: 500},
	})
	snap, _ = tr.snapshot(nil)
	if snap["a"].Upload != 500 {
		t.Errorf("after UPDATE upload = %d, want 500", snap["a"].Upload)
	}

	// CLOSED удаляет.
	tr.applyEvent(&daemonpb.ConnectionEvent{
		Type: daemonpb.ConnectionEventType_CONNECTION_EVENT_CLOSED,
		Id:   "a",
	})
	snap, _ = tr.snapshot(nil)
	if len(snap) != 0 {
		t.Errorf("after CLOSED len = %d, want 0", len(snap))
	}

	// reset очищает.
	tr.applyEvent(&daemonpb.ConnectionEvent{
		Type:       daemonpb.ConnectionEventType_CONNECTION_EVENT_NEW,
		Connection: &daemonpb.Connection{Id: "b", Destination: "2.2.2.2:80"},
	})
	tr.reset()
	snap, _ = tr.snapshot(nil)
	if len(snap) != 0 {
		t.Errorf("after reset len = %d, want 0", len(snap))
	}

	// markDead (обрыв стрима): снимок снова ok=false — источника нет,
	// застывшие данные не должны отдаваться как живые.
	tr.applyEvent(&daemonpb.ConnectionEvent{
		Type:       daemonpb.ConnectionEventType_CONNECTION_EVENT_NEW,
		Connection: &daemonpb.Connection{Id: "c", Destination: "3.3.3.3:80"},
	})
	tr.markDead()
	if _, ok := tr.snapshot(nil); ok {
		t.Error("snapshot ok=true after markDead")
	}
}
