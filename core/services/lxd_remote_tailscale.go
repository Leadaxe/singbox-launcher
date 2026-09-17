// File lxd_remote_tailscale.go — стрим статуса tailnet удалённой машины
// (SPEC 130). Образец — SubscribeStatus: streamConn + runResilientStream.
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
