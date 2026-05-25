package traffic

import (
	"testing"
	"time"
)

func TestConnPoller_Diff_OpenCloseBytes(t *testing.T) {
	p := NewConnPoller(func() (string, string, bool) { return "", "", false }, nil)

	// Seed prev with one connection.
	p.prev = map[string]ClashConn{
		"abc": {ID: "abc", Upload: 100, Download: 200, Start: time.Now().Add(-3 * time.Second)},
	}
	now := time.Now()
	curr := map[string]ClashConn{
		// id "abc" disappeared → expect Closed
		// id "def" appeared → expect Opened
		"def": {ID: "def", Upload: 50, Download: 70, Start: now},
		// id "ghi" present in prev with byte change — we'll add to prev too
	}
	p.prev["ghi"] = ClashConn{ID: "ghi", Upload: 1000, Download: 2000}
	curr["ghi"] = ClashConn{ID: "ghi", Upload: 1500, Download: 2500}

	d := p.diff(curr, now)

	if len(d.Opened) != 1 || d.Opened[0].ID != "def" {
		t.Errorf("Opened: want [def], got %+v", d.Opened)
	}
	if len(d.Closed) != 1 || d.Closed[0].Conn.ID != "abc" {
		t.Errorf("Closed: want [abc], got %+v", d.Closed)
	}
	if d.Closed[0].Duration <= 0 {
		t.Errorf("Closed duration: want >0, got %v", d.Closed[0].Duration)
	}
	if len(d.Bytes) != 1 || d.Bytes[0].Conn.ID != "ghi" {
		t.Errorf("Bytes: want [ghi], got %+v", d.Bytes)
	}
	if d.Bytes[0].UpDelta != 500 || d.Bytes[0].DownDelta != 500 {
		t.Errorf("Bytes delta: want 500/500, got %d/%d", d.Bytes[0].UpDelta, d.Bytes[0].DownDelta)
	}
}

func TestClashConnMeta_PortInt(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"443", 443},
		{"", 0},
		{"notanumber", 0},
		{"0", 0},
	}
	for _, c := range cases {
		m := ClashConnMeta{DestinationPort: c.in}
		if got := m.PortInt(); got != c.want {
			t.Errorf("PortInt(%q): want %d got %d", c.in, c.want, got)
		}
	}
}
