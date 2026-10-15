//go:build darwin || (windows && !386)

// File backend_daemon_openconnect.go — стрим статуса openconnect локального
// демона и ответы на челленджи второго фактора / SSO (SPEC 167 §2.5).
// Образец — superviseTailscale: переподписка с бэкоффом, сброс бэкоффа
// только после полученного кадра.
//
// Отличие: Unimplemented (ядро собрано без with_openconnect) — подписка
// останавливается насовсем для этого инстанса бэкенда. Новое ядро приходит
// со сменой бэкенда (newDaemonBackend), и супервизор стартует заново.
package core

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"singbox-launcher/core/services"
	daemonpb "singbox-launcher/internal/daemonpb"
	"singbox-launcher/internal/debuglog"
)

// superviseOpenConnect держит подписку SubscribeOpenConnectStatus и
// наполняет кеш. Первый кадр приходит сразу после подписки — висящий
// челлендж виден с момента коннекта (и при открытии лаунчера).
func (b *DaemonBackend) superviseOpenConnect() {
	b.openconnect.SetNotify(func(fresh []services.OpenConnectStatus, changed bool) {
		dispatchOpenConnect(CoreTarget{Scope: services.ScopeLocal}, fresh, changed)
	})
	backoff := time.Second
	for {
		if b.ctx.Err() != nil {
			return
		}
		client, err := b.grpcClient()
		if err == nil {
			var stream grpc.ServerStreamingClient[daemonpb.OpenConnectStatusUpdate]
			stream, err = client.SubscribeOpenConnectStatus(b.ctx, &emptypb.Empty{})
			if err == nil {
				var received bool
				received, err = b.consumeOpenConnectStream(stream)
				if received {
					backoff = time.Second
				}
			}
		}
		if isUnimplemented(err) {
			debuglog.InfoLog("daemon.openconnect: core has no openconnect status RPC, stream stopped")
			b.openconnect.MarkUnsupported()
			return
		}
		if err != nil {
			debuglog.DebugLog("daemon.openconnect: stream unavailable: %v", err)
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

// consumeOpenConnectStream читает стрим до обрыва; received — получен хотя
// бы один кадр, err — ошибка Recv (Unimplemented всплывает здесь).
func (b *DaemonBackend) consumeOpenConnectStream(stream grpc.ServerStreamingClient[daemonpb.OpenConnectStatusUpdate]) (bool, error) {
	received := false
	defer b.openconnect.MarkDead()
	for {
		upd, err := stream.Recv()
		if err != nil {
			debuglog.DebugLog("daemon.openconnect: stream closed: %v", err)
			return received, err
		}
		if !received {
			debuglog.DebugLog("daemon.openconnect: first status frame, %d endpoints", len(upd.GetEndpoints()))
		}
		received = true
		if !b.isActive() {
			continue
		}
		b.openconnect.Apply(upd)
	}
}

// OpenConnectStatus implements openConnectSource.
func (b *DaemonBackend) OpenConnectStatus(tag string) (services.OpenConnectStatus, bool) {
	return b.openconnect.Get(tag)
}

// OpenConnectLive implements openConnectSource.
func (b *DaemonBackend) OpenConnectLive() bool {
	return b.openconnect.Live()
}

// OpenConnectPending implements openConnectSource.
func (b *DaemonBackend) OpenConnectPending() []services.OpenConnectStatus {
	return b.openconnect.Pending()
}

// openConnectCallTimeout — дедлайн разовых вызовов Submit/Cancel.
const openConnectCallTimeout = 10 * time.Second

// OpenConnectSubmitForm implements openConnectSource.
func (b *DaemonBackend) OpenConnectSubmitForm(tag, challengeID string, values map[string]string) error {
	client, err := b.grpcClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(b.ctx, openConnectCallTimeout)
	defer cancel()
	debuglog.DebugLog("daemon.openconnect: submit form for %s (%d fields)", tag, len(values))
	return services.OpenConnectSubmitFormRPC(ctx, client, tag, challengeID, values)
}

// OpenConnectSubmitBrowser implements openConnectSource.
func (b *DaemonBackend) OpenConnectSubmitBrowser(tag, challengeID, finalURL string) error {
	client, err := b.grpcClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(b.ctx, openConnectCallTimeout)
	defer cancel()
	debuglog.DebugLog("daemon.openconnect: submit browser result for %s", tag)
	return services.OpenConnectSubmitBrowserRPC(ctx, client, tag, challengeID, finalURL)
}

// OpenConnectCancel implements openConnectSource.
func (b *DaemonBackend) OpenConnectCancel(tag, challengeID string) error {
	client, err := b.grpcClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(b.ctx, openConnectCallTimeout)
	defer cancel()
	debuglog.DebugLog("daemon.openconnect: cancel challenge for %s", tag)
	return services.OpenConnectCancelRPC(ctx, client, tag, challengeID)
}
