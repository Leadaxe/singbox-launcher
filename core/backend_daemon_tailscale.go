//go:build darwin || (windows && !386)

// File backend_daemon_tailscale.go — стрим статуса tailnet локального
// демона (SPEC 130). Образец — superviseConnections: reconnect с backoff,
// сброс backoff только после реально полученного кадра (Subscribe* у grpc-go
// «успешен» и при лежащем демоне — ошибка всплывает в первом Recv).
package core

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// tailscaleCallTimeout — дедлайн разовых вызовов (выход, снятие exit node).
const tailscaleCallTimeout = 10 * time.Second

// TailscaleSetExitNode implements tailscaleController (SPEC 148 §5):
// выход переключается на ходу, тело узла не меняется. stableID "" снимает
// выход.
func (b *DaemonBackend) TailscaleSetExitNode(tag, stableID string) error {
	client, err := b.grpcClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(b.ctx, tailscaleCallTimeout)
	defer cancel()
	if _, err := client.SetTailscaleExitNode(ctx, &daemonpb.SetTailscaleExitNodeRequest{
		EndpointTag: tag, StableID: stableID,
	}); err != nil {
		return fmt.Errorf("daemon SetTailscaleExitNode: %w", err)
	}
	return nil
}

// TailscaleStatusNow implements tailscaleController (SPEC 158): свежий
// статус endpoint'а по запросу (GetTailscaleStatus, SPEC 115 ядра) — путь
// пиров на момент вызова, в отличие от кеша потока.
func (b *DaemonBackend) TailscaleStatusNow(tag string) (services.TailscaleStatus, bool, error) {
	client, err := b.grpcClient()
	if err != nil {
		return services.TailscaleStatus{}, false, err
	}
	ctx, cancel := context.WithTimeout(b.ctx, tailscaleCallTimeout)
	defer cancel()
	return services.TailscaleStatusRPC(ctx, client, tag)
}

// TailscaleLogout implements tailscaleController (SPEC 148 §3).
func (b *DaemonBackend) TailscaleLogout(tag string) error {
	client, err := b.grpcClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(b.ctx, tailscaleCallTimeout)
	defer cancel()
	if _, err := client.TailscaleLogout(ctx, &daemonpb.TailscaleLogoutRequest{EndpointTag: tag}); err != nil {
		return fmt.Errorf("daemon TailscaleLogout: %w", err)
	}
	return nil
}

// TailscalePing implements tailscaleController (SPEC 148 §7): каждый ответ
// ядра отдаётся в onReply; проверка идёт до отмены ctx, до
// services.TailscalePingMaxReplies ответов или до конца стрима.
func (b *DaemonBackend) TailscalePing(ctx context.Context, tag, peerIP string, onReply func(services.TailscalePingResult)) error {
	client, err := b.grpcClient()
	if err != nil {
		return err
	}
	stream, err := client.StartTailscalePing(ctx, &daemonpb.TailscalePingRequest{EndpointTag: tag, PeerIP: peerIP})
	if err != nil {
		return fmt.Errorf("daemon StartTailscalePing: %w", err)
	}
	for n := 0; n < services.TailscalePingMaxReplies; n++ {
		resp, rerr := stream.Recv()
		if rerr != nil {
			if errors.Is(rerr, io.EOF) || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("daemon StartTailscalePing: %w", rerr)
		}
		onReply(services.TailscalePingResult{
			LatencyMs:      resp.GetLatencyMs(),
			IsDirect:       resp.GetIsDirect(),
			Endpoint:       resp.GetEndpoint(),
			DERPRegionCode: resp.GetDerpRegionCode(),
			Error:          resp.GetError(),
		})
	}
	return nil
}
