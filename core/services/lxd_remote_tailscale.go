// File lxd_remote_tailscale.go — стрим статуса tailnet удалённой машины
// (SPEC 130) и команды tailnet (SPEC 148): выбор exit node на ходу, выход из
// аккаунта, проверка устройства. Образец стрима — SubscribeStatus:
// streamConn + runResilientStream; команд — DaemonBackend
// (core/backend_daemon_tailscale.go), те же RPC StartedService.
//
// Отличие от SubscribeStatus: тот открывается по требованию (профайлером), а
// этот живёт с транспортом от создания до Close — колонка на вкладке
// Endpoints и NeedsLogin обязаны быть видны без открытого окна. Стартует
// лениво при первом чтении, а не в конструкторе: конструктор зовётся и там,
// где статус никому не нужен (проверка связи, деплой), и открывать стрим к
// роутеру ради этого незачем.
package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"google.golang.org/protobuf/types/known/emptypb"

	daemonpb "singbox-launcher/internal/daemonpb"
)

// tailscaleStream — состояние ленивого стрима транспорта.
type tailscaleStream struct {
	once   sync.Once
	cancel context.CancelFunc
	cache  TailscaleStatusCache
}

// ensureTailscaleStream поднимает стрим один раз на транспорт. Ошибка
// streamConn не фатальна: кеш остаётся пустым, следующий вызов повторит
// попытку (once сбрасывать не нужно — runResilientStream сам переподпишется,
// а не поднявшийся streamConn означает, что и остальные RPC не работают).
func (t *LxdRemoteTransport) ensureTailscaleStream() {
	t.ts.once.Do(func() {
		conn, err := t.streamConn()
		if err != nil {
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		t.ts.cancel = cancel
		runResilientStream(ctx, "tailscale", func() error {
			stream, serr := daemonpb.NewStartedServiceClient(conn).SubscribeTailscaleStatus(ctx, &emptypb.Empty{})
			if serr != nil {
				return serr
			}
			for {
				upd, recvErr := stream.Recv()
				if recvErr != nil {
					return recvErr
				}
				t.ts.cache.Apply(upd)
			}
		}, t.ts.cache.MarkDead)
	})
}

// stopTailscaleStream гасит стрим; зовётся из Close.
func (t *LxdRemoteTransport) stopTailscaleStream() {
	if t.ts.cancel != nil {
		t.ts.cancel()
	}
}

// TailscaleStatus — статус tailscale-endpoint'а удалённого ядра из кеша
// (реализует core.tailscaleSource).
func (t *LxdRemoteTransport) TailscaleStatus(tag string) (TailscaleStatus, bool) {
	t.ensureTailscaleStream()
	return t.ts.cache.Get(tag)
}

// TailscaleLive — жив ли стрим статуса удалённого ядра.
func (t *LxdRemoteTransport) TailscaleLive() bool {
	t.ensureTailscaleStream()
	return t.ts.cache.Live()
}

// TailscaleSetExitNode — выбор exit node удалённого ядра на ходу
// (реализует core.tailscaleController); stableID "" снимает выход. Тело узла
// в конфиге машины не меняется — это делает Save choice.
func (t *LxdRemoteTransport) TailscaleSetExitNode(tag, stableID string) error {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return err
	}
	defer cancel()
	if _, err := client.SetTailscaleExitNode(ctx, &daemonpb.SetTailscaleExitNodeRequest{
		EndpointTag: tag, StableID: stableID,
	}); err != nil {
		return fmt.Errorf("lxd remote SetTailscaleExitNode: %w", err)
	}
	return nil
}

// TailscaleLogout — выход узла удалённого ядра из аккаунта tailnet.
func (t *LxdRemoteTransport) TailscaleLogout(tag string) error {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return err
	}
	defer cancel()
	if _, err := client.TailscaleLogout(ctx, &daemonpb.TailscaleLogoutRequest{EndpointTag: tag}); err != nil {
		return fmt.Errorf("lxd remote TailscaleLogout: %w", err)
	}
	return nil
}

// TailscalePing — проверка устройства tailnet через удалённое ядро: каждый
// ответ в onReply, до отмены ctx, до TailscalePingMaxReplies ответов или до
// конца стрима. Дедлайна у вызова нет — его задаёт ctx вызывающего (лист
// проверки живёт до закрытия).
func (t *LxdRemoteTransport) TailscalePing(ctx context.Context, tag, peerIP string, onReply func(TailscalePingResult)) error {
	conn, err := t.streamConn()
	if err != nil {
		return err
	}
	stream, err := daemonpb.NewStartedServiceClient(conn).StartTailscalePing(ctx, &daemonpb.TailscalePingRequest{EndpointTag: tag, PeerIP: peerIP})
	if err != nil {
		return fmt.Errorf("lxd remote StartTailscalePing: %w", err)
	}
	for n := 0; n < TailscalePingMaxReplies; n++ {
		resp, rerr := stream.Recv()
		if rerr != nil {
			if errors.Is(rerr, io.EOF) || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("lxd remote StartTailscalePing: %w", rerr)
		}
		onReply(TailscalePingResult{
			LatencyMs:      resp.GetLatencyMs(),
			IsDirect:       resp.GetIsDirect(),
			Endpoint:       resp.GetEndpoint(),
			DERPRegionCode: resp.GetDerpRegionCode(),
			Error:          resp.GetError(),
		})
	}
	return nil
}
