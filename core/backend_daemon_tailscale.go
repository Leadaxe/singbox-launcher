//go:build darwin

// File backend_daemon_tailscale.go — стрим статуса tailnet локального
// демона (SPEC 130). Образец — superviseConnections: reconnect с backoff,
// сброс backoff только после реально полученного кадра (Subscribe* у grpc-go
// «успешен» и при лежащем демоне — ошибка всплывает в первом Recv).
package core

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"singbox-launcher/core/services"
	daemonpb "singbox-launcher/internal/daemonpb"
	"singbox-launcher/internal/debuglog"
)

// superviseTailscale держит подписку SubscribeTailscaleStatus и наполняет
// кеш. Первое сообщение приходит сразу после подписки — кеш заполнен с
// момента коннекта, а не с открытия окна. Ядро без with_tailscale или без
// tailscale-узлов: стрим либо не поднимается, либо молчит — кеш пуст,
// колонка показывает «—», как и без стрима вовсе.
func (b *DaemonBackend) superviseTailscale() {
	backoff := time.Second
	for {
		if b.ctx.Err() != nil {
			return
		}
		client, err := b.grpcClient()
		if err == nil {
			var stream grpc.ServerStreamingClient[daemonpb.TailscaleStatusUpdate]
			stream, err = client.SubscribeTailscaleStatus(b.ctx, &emptypb.Empty{})
			if err == nil && b.consumeTailscaleStream(stream) {
				backoff = time.Second
			}
		}
		if err != nil {
			debuglog.DebugLog("daemon.tailscale: stream unavailable: %v", err)
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

// consumeTailscaleStream читает стрим до обрыва; true — получен хотя бы один
// кадр. По выходу кеш помечается мёртвым, снимок остаётся (старый статус с
// возрастом честнее пустого экрана).
func (b *DaemonBackend) consumeTailscaleStream(stream grpc.ServerStreamingClient[daemonpb.TailscaleStatusUpdate]) bool {
	received := false
	defer b.tailscale.MarkDead()
	for {
		upd, err := stream.Recv()
		if err != nil {
			debuglog.DebugLog("daemon.tailscale: stream closed: %v", err)
			return received
		}
		received = true
		if !b.isActive() {
			continue
		}
		b.tailscale.Apply(upd)
	}
}

// TailscaleStatus implements tailscaleSource.
func (b *DaemonBackend) TailscaleStatus(tag string) (services.TailscaleStatus, bool) {
	return b.tailscale.Get(tag)
}

// TailscaleLive implements tailscaleSource.
func (b *DaemonBackend) TailscaleLive() bool {
	return b.tailscale.Live()
}
