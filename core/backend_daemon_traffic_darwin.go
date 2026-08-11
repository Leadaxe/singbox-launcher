//go:build darwin

package core

import (
	"context"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	daemonpb "singbox-launcher/internal/daemonpb"
	"singbox-launcher/internal/debuglog"
	tprof "singbox-launcher/internal/traffic"
)

// Traffic-источник daemon-режима: подписка на gRPC SubscribeConnections и
// поддержание накопительного снимка соединений, который traffic-профайлер
// потребляет через SnapshotFunc — тот же контракт, что у Clash-поллера
// (classic). Так график трафика работает без Clash API.

// connTracker держит текущее множество открытых соединений, наполняемое
// стримом SubscribeConnections (события NEW/UPDATE добавляют/обновляют,
// CLOSED удаляют, reset очищает всё). Снимок отдаётся профайлеру.
type connTracker struct {
	mu    sync.Mutex
	conns map[string]tprof.ClashConn
	// live — стрим хоть раз доставил данные; до этого snapshot возвращает
	// ok=false, чтобы профайлер не считал пустой снимок «всё закрылось».
	live bool
}

func newConnTracker() *connTracker {
	return &connTracker{conns: make(map[string]tprof.ClashConn)}
}

func (t *connTracker) applyEvent(ev *daemonpb.ConnectionEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch ev.GetType() {
	case daemonpb.ConnectionEventType_CONNECTION_EVENT_CLOSED:
		delete(t.conns, ev.GetId())
	default: // NEW, UPDATE
		if c := ev.GetConnection(); c != nil {
			t.conns[c.GetId()] = protoConnToClash(c)
		}
	}
}

func (t *connTracker) reset() {
	t.mu.Lock()
	t.conns = make(map[string]tprof.ClashConn)
	t.mu.Unlock()
}

// markDead переводит tracker в состояние «источника нет»: стрим оборвался,
// текущий снимок больше не отражает реальность. snapshot снова вернёт
// ok=false (профайлер сбросит diff-state), пока переподписка не доставит
// свежие данные. Без этого застывший снимок отдавался бы как живой всё
// время, пока демон недоступен.
func (t *connTracker) markDead() {
	t.mu.Lock()
	t.conns = make(map[string]tprof.ClashConn)
	t.live = false
	t.mu.Unlock()
}

// snapshot реализует traffic.SnapshotFunc: копия текущего множества.
// ok=false до первого события стрима.
func (t *connTracker) snapshot(_ context.Context) (map[string]tprof.ClashConn, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.live {
		return nil, false
	}
	out := make(map[string]tprof.ClashConn, len(t.conns))
	for k, v := range t.conns {
		out[k] = v
	}
	return out, true
}

// protoConnToClash конвертирует gRPC Connection в схему ClashConn, которую
// ждёт профайлер. Порт в ClashConnMeta — строкой (как в Clash-схеме).
func protoConnToClash(c *daemonpb.Connection) tprof.ClashConn {
	host, portStr := c.GetDomain(), ""
	dst := c.GetDestination() // "ip:port" или "host:port"
	if h, p, err := net.SplitHostPort(dst); err == nil {
		if host == "" {
			host = h
		}
		portStr = p
	}
	destIP := ""
	if ip := net.ParseIP(hostOnly(dst)); ip != nil {
		destIP = ip.String()
	}
	var start time.Time
	if ms := c.GetCreatedAt(); ms > 0 {
		start = time.UnixMilli(ms)
	}
	proc := ""
	if pi := c.GetProcessInfo(); pi != nil {
		proc = pi.GetProcessPath()
	}
	return tprof.ClashConn{
		ID:       c.GetId(),
		Upload:   c.GetUplinkTotal(),
		Download: c.GetDownlinkTotal(),
		Start:    start,
		Chains:   c.GetChainList(),
		Rule:     c.GetRule(),
		Metadata: tprof.ClashConnMeta{
			Network:         c.GetNetwork(),
			Host:            host,
			DestinationIP:   destIP,
			DestinationPort: portStr,
			ProcessPath:     proc,
			Process:         procBase(proc),
		},
	}
}

func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

func procBase(path string) string {
	if path == "" {
		return ""
	}
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}

// superviseConnections держит подписку SubscribeConnections и наполняет
// tracker; reconnect с backoff, как у status/logs (сброс backoff — только
// после реально полученного кадра: Subscribe* у grpc-go «успешен» даже при
// лежащем демоне, ошибка всплывает в первом Recv). Профайлер читает снимок
// через b.connTracker.snapshot (установлено как SnapshotFunc в EnsureProfiler).
func (b *DaemonBackend) superviseConnections() {
	backoff := time.Second
	for {
		if b.ctx.Err() != nil {
			return
		}
		client, err := b.grpcClient()
		if err == nil {
			var stream grpc.ServerStreamingClient[daemonpb.ConnectionEvents]
			stream, err = client.SubscribeConnections(b.ctx, &daemonpb.SubscribeConnectionsRequest{Interval: 1000})
			if err == nil && b.consumeConnStream(stream) {
				backoff = time.Second
			}
		}
		if err != nil {
			debuglog.DebugLog("daemon.conns: stream unavailable: %v", err)
		}
		select {
		case <-b.ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

// consumeConnStream читает стрим до обрыва; true — получен хотя бы один кадр.
// По выходу tracker помечается мёртвым: без источника снимок не «живой», и
// профайлер должен видеть ok=false, а не застывшее множество соединений.
// Свежая переподписка доставит полный снапшот заново (reset+NEW-события).
func (b *DaemonBackend) consumeConnStream(stream grpc.ServerStreamingClient[daemonpb.ConnectionEvents]) bool {
	received := false
	defer b.connTracker.markDead()
	for {
		batch, err := stream.Recv()
		if err != nil {
			debuglog.DebugLog("daemon.conns: stream closed: %v", err)
			return received
		}
		received = true
		if !b.isActive() {
			continue
		}
		if batch.GetReset_() {
			b.connTracker.reset()
		}
		b.connTracker.mu.Lock()
		b.connTracker.live = true
		b.connTracker.mu.Unlock()
		for _, ev := range batch.GetEvents() {
			b.connTracker.applyEvent(ev)
		}
	}
}

// ConnSnapshotFunc отдаёт SnapshotFunc для профайлера (traffic по gRPC).
func (b *DaemonBackend) ConnSnapshotFunc() tprof.SnapshotFunc {
	return b.connTracker.snapshot
}

// connSnapshotFuncAny реализует connSnapshotSource: тот же источник как any,
// чтобы core/backend.go не тянул зависимость на пакет traffic. UI приводит
// обратно к traffic.SnapshotFunc.
func (b *DaemonBackend) connSnapshotFuncAny() any {
	return b.ConnSnapshotFunc()
}
