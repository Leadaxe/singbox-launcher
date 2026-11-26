//go:build darwin || (windows && !386)

package core

import (
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

// Трекер — общий с удалённой машиной (traffic.ConnTracker): стрим у своего
// демона и у чужого один и тот же, и вторая реализация здесь уже расходилась
// с протоколом — UPDATE несёт только приращения, без Connection, а она ждала
// Connection и суммы после первого NEW не росли.

// superviseConnections держит подписку SubscribeConnections и наполняет
// tracker; reconnect с backoff, как у status/logs (сброс backoff — только
// после реально полученного кадра: Subscribe* у grpc-go «успешен» даже при
// лежащем демоне, ошибка всплывает в первом Recv). Профайлер читает снимок
// через b.connTracker.Snapshot (установлено как SnapshotFunc в EnsureProfiler).
func (b *DaemonBackend) superviseConnections() {
	backoff := time.Second
	for {
		if b.ctx.Err() != nil {
			return
		}
		client, err := b.grpcClient()
		if err == nil {
			var stream grpc.ServerStreamingClient[daemonpb.ConnectionEvents]
			// Interval — наносекунды (time.Duration, конвенция sing-box/libbox):
			// голое число здесь означало бы наносекунды, а не миллисекунды, и
			// загоняло бы демон в 100% CPU. Демон клампит значения < 200мс.
			stream, err = client.SubscribeConnections(b.ctx, &daemonpb.SubscribeConnectionsRequest{
				Interval: int64(time.Second),
			})
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
// По выходу tracker сбрасывается (Reset): без источника снимок не «живой», и
// профайлер должен видеть ok=false, а не застывшее множество соединений.
// Свежая переподписка доставит полный снапшот заново (reset+NEW-события).
func (b *DaemonBackend) consumeConnStream(stream grpc.ServerStreamingClient[daemonpb.ConnectionEvents]) bool {
	received := false
	defer b.connTracker.Reset()
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
			b.connTracker.Reset()
		}
		// Кадр без событий тоже значит «источник жив»: пустой снимок после
		// reset — это «соединений нет», а не «данных нет».
		b.connTracker.MarkLive()
		for _, ev := range batch.GetEvents() {
			b.connTracker.ApplyEvent(ev)
		}
	}
}

// ConnSnapshotFunc отдаёт SnapshotFunc для профайлера (traffic по gRPC).
func (b *DaemonBackend) ConnSnapshotFunc() tprof.SnapshotFunc {
	return b.connTracker.Snapshot
}

// connSnapshotFuncAny реализует connSnapshotSource: тот же источник как any,
// чтобы core/backend.go не тянул зависимость на пакет traffic. UI приводит
// обратно к traffic.SnapshotFunc.
func (b *DaemonBackend) connSnapshotFuncAny() any {
	return b.ConnSnapshotFunc()
}
