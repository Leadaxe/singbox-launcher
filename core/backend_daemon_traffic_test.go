//go:build darwin || (windows && !386)

package core

import (
	"context"
	"io"
	"testing"

	"google.golang.org/grpc"

	daemonpb "singbox-launcher/internal/daemonpb"
	tprof "singbox-launcher/internal/traffic"
)

// connStreamStep — один кадр стрима и проверка снимка после того, как
// consumeConnStream его применил.
type connStreamStep struct {
	batch *daemonpb.ConnectionEvents
	check func(snap map[string]tprof.ClashConn, ok bool)
}

// fakeConnStream отдаёт кадры по одному. Проверка шага выполняется на входе
// в следующий Recv: к этому моменту предыдущий кадр уже применён, а стрим ещё
// не оборван (после обрыва трекер сбрасывается).
type fakeConnStream struct {
	grpc.ClientStream
	steps    []connStreamStep
	next     int
	snapshot tprof.SnapshotFunc
}

func (f *fakeConnStream) Recv() (*daemonpb.ConnectionEvents, error) {
	if f.next > 0 {
		if check := f.steps[f.next-1].check; check != nil {
			check(f.snapshot(context.Background()))
		}
	}
	if f.next == len(f.steps) {
		return nil, io.EOF
	}
	f.next++
	return f.steps[f.next-1].batch, nil
}

func connNew(id string, up, down int64) *daemonpb.ConnectionEvent {
	return &daemonpb.ConnectionEvent{
		Type: daemonpb.ConnectionEventType_CONNECTION_EVENT_NEW,
		Id:   id,
		Connection: &daemonpb.Connection{
			Id: id, Destination: "1.1.1.1:443", UplinkTotal: up, DownlinkTotal: down,
		},
	}
}

// closedNew — NEW недавно закрытого соединения, как его шлёт ядро при подписке.
func closedNew(id string, up, down int64) *daemonpb.ConnectionEvent {
	ev := connNew(id, up, down)
	ev.Connection.ClosedAt = 1_700_000_000_000
	return ev
}

// connUpdate — UPDATE как его шлёт ядро: id и приращения, без Connection.
func connUpdate(id string, up, down int64) *daemonpb.ConnectionEvent {
	return &daemonpb.ConnectionEvent{
		Type:          daemonpb.ConnectionEventType_CONNECTION_EVENT_UPDATE,
		Id:            id,
		UplinkDelta:   up,
		DownlinkDelta: down,
	}
}

func connBatch(reset bool, evs ...*daemonpb.ConnectionEvent) *daemonpb.ConnectionEvents {
	return &daemonpb.ConnectionEvents{Reset_: reset, Events: evs}
}

// TestDaemonConnStreamTotals гоняет через consumeConnStream кадры в том виде,
// в каком их шлёт ядро (daemon/started_service.go): NEW с полной записью,
// UPDATE только с приращениями, повторный NEW по тому же id с текущими суммами
// и приращением тика (SPEC 138 ядра), CLOSED по id, недавно закрытое
// соединение в кадре reset (NEW с closedAt).
func TestDaemonConnStreamTotals(t *testing.T) {
	ac := &AppController{}
	b := &DaemonBackend{ac: ac, connTracker: tprof.NewConnTracker()}
	ac.backend = b

	snapshot := b.ConnSnapshotFunc()
	if _, ok := snapshot(context.Background()); ok {
		t.Fatal("snapshot ok=true before the stream delivered anything")
	}

	wantTotals := func(stage string, up, down int64) func(map[string]tprof.ClashConn, bool) {
		return func(snap map[string]tprof.ClashConn, ok bool) {
			c, found := snap["a"]
			if !ok || !found {
				t.Errorf("%s: ok=%v found=%v", stage, ok, found)
				return
			}
			if c.Upload != up || c.Download != down {
				t.Errorf("%s: totals = %d/%d, want %d/%d", stage, c.Upload, c.Download, up, down)
			}
		}
	}

	// Повторный NEW: суммы записи равны накопленным (100+50+30 / 200+500+70),
	// приращение тика (30/70) лежит и в событии, и уже внутри сумм.
	reNew := connNew("a", 180, 770)
	reNew.UplinkDelta, reNew.DownlinkDelta = 30, 70
	reNew.Connection.Uplink, reNew.Connection.Downlink = 30, 70
	reNew.Connection.ChainList = []string{"node-2", "pool"}

	stream := &fakeConnStream{snapshot: snapshot, steps: []connStreamStep{
		{connBatch(true), func(snap map[string]tprof.ClashConn, ok bool) {
			if !ok || len(snap) != 0 {
				t.Errorf("empty reset frame: ok=%v len=%d, want live and empty", ok, len(snap))
			}
		}},
		{connBatch(false, connNew("a", 100, 200)), wantTotals("NEW", 100, 200)},
		{connBatch(false, connUpdate("a", 50, 500)), wantTotals("UPDATE", 150, 700)},
		{connBatch(false, reNew), func(snap map[string]tprof.ClashConn, ok bool) {
			wantTotals("repeated NEW", 180, 770)(snap, ok)
			if got := snap["a"].Chains; len(got) != 2 || got[0] != "node-2" {
				t.Errorf("repeated NEW: chains = %v, want the entry replaced", got)
			}
		}},
		{connBatch(false, connUpdate("a", 20, 30)), wantTotals("UPDATE after repeated NEW", 200, 800)},
		{connBatch(false, connUpdate("a", 0, 0), connUpdate("ghost", 5, 5)), func(snap map[string]tprof.ClashConn, ok bool) {
			wantTotals("zero UPDATE", 200, 800)(snap, ok)
			if _, found := snap["ghost"]; found {
				t.Error("UPDATE for an unknown id created an entry")
			}
		}},
		{connBatch(false, &daemonpb.ConnectionEvent{
			Type: daemonpb.ConnectionEventType_CONNECTION_EVENT_CLOSED, Id: "a",
		}), func(snap map[string]tprof.ClashConn, ok bool) {
			if !ok || len(snap) != 0 {
				t.Errorf("CLOSED: ok=%v len=%d, want live and empty", ok, len(snap))
			}
		}},
		{connBatch(false, connNew("b", 1, 2)), nil},
		{connBatch(true, connNew("c", 3, 4), closedNew("old", 7, 8)), func(snap map[string]tprof.ClashConn, ok bool) {
			if _, found := snap["b"]; found || len(snap) != 1 {
				t.Errorf("reset frame: snapshot = %v, want only c", snap)
			}
		}},
	}}

	if !b.consumeConnStream(stream) {
		t.Error("consumeConnStream = false, want true after received frames")
	}
	// Обрыв стрима: источника нет, застывший снимок не отдаётся как живой.
	if _, ok := snapshot(context.Background()); ok {
		t.Error("snapshot ok=true after the stream dropped")
	}
}
