package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"singbox-launcher/api"
	daemonpb "singbox-launcher/internal/daemonpb"
	"singbox-launcher/internal/lxdclient"
)

// LxdRemoteTransport — ProxyTransport поверх gRPC-канала УДАЛЁННОГО демона
// `sing-box lxd` (SPEC 097).
//
// Зачем отдельно от daemonProxyTransport (core/backend_daemon_darwin.go): тот
// обслуживает демон, которым лаунчер сам управляет на ЭТОЙ машине — он привязан
// к жизненному циклу DaemonBackend (старт/стоп службы, supervise-стримы,
// state-dir). Здесь ничего этого нет: мы только клиент к чужому процессу на
// роутере/сервере, у которого свой жизненный цикл, и единственное, что нам
// нужно, — говорить те же RPC (GetGroups / SelectOutbound / URLTestOutbound).
//
// Реализует services.ProxyTransport, поэтому вкладка Servers работает с
// удалённым демоном ровно тем же кодом, что и с локальным ядром: смена
// endpoint'а не трогает UI (тот же шов, что SPEC 064 сделал для Clash-API).
//
// Транспорт независим от платформы: лаунчер на Windows может управлять
// linux-роутером. Демонный движок — macOS-only, но это про запуск СВОЕГО
// демона, а не про клиента к чужому.
type LxdRemoteTransport struct {
	client *lxdclient.Client

	mu   sync.Mutex
	conn *grpc.ClientConn
}

// NewLxdRemoteTransport строит транспорт к удалённому демону по готовой
// конфигурации подключения (адрес + пин сервера + клиентская identity).
func NewLxdRemoteTransport(cfg lxdclient.Config) *LxdRemoteTransport {
	return &LxdRemoteTransport{client: lxdclient.New(cfg)}
}

// lxdRemoteRPCTimeout — дедлайн одного вызова. Заметно больше локального
// (15s в daemonProxyTransport): канал до роутера может идти через сам VPN,
// и на плохой линии 15s даёт ложные таймауты на ровном месте.
const lxdRemoteRPCTimeout = 25 * time.Second

// Addr — адрес удалённого демона (для статус-бейджа и логов).
func (t *LxdRemoteTransport) Addr() string { return t.client.AddrString() }

// Close закрывает gRPC-соединение. Идемпотентен: повторный вызов и вызов на
// неподключённом транспорте безопасны.
func (t *LxdRemoteTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return nil
	}
	err := t.conn.Close()
	t.conn = nil
	return err
}

// rpc возвращает клиента StartedService с контекстом под один вызов.
//
// Соединение создаётся лениво и переиспользуется: grpc.NewClient не
// коннектится немедленно, поэтому «дозвон» происходит на первом RPC, а
// разрывы канала grpc-go лечит сам (reconnect с backoff внутри ClientConn).
func (t *LxdRemoteTransport) rpc() (daemonpb.StartedServiceClient, context.Context, context.CancelFunc, error) {
	t.mu.Lock()
	if t.conn == nil {
		conn, err := t.client.DialGRPC()
		if err != nil {
			t.mu.Unlock()
			return nil, nil, nil, fmt.Errorf("lxd remote: dial %s: %w", t.client.AddrString(), err)
		}
		t.conn = conn
	}
	conn := t.conn
	t.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), lxdRemoteRPCTimeout)
	return daemonpb.NewStartedServiceClient(conn), ctx, cancel, nil
}

// GroupProxies implements ProxyTransport через GetGroups.
func (t *LxdRemoteTransport) GroupProxies(group string) ([]api.ProxyInfo, string, error) {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return nil, "", err
	}
	defer cancel()
	groups, err := client.GetGroups(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, "", fmt.Errorf("lxd remote GetGroups: %w", err)
	}
	for _, g := range groups.GetGroup() {
		if g.GetTag() != group {
			continue
		}
		selected := g.GetSelected()
		proxies := make([]api.ProxyInfo, 0, len(g.GetItems()))
		for _, item := range g.GetItems() {
			info := api.ProxyInfo{
				Name:      item.GetTag(),
				ClashType: item.GetType(),
				Delay:     int64(item.GetUrlTestDelay()),
			}
			// GetGroups отдаёт выбор на уровне группы; Servers-tab рисует
			// маркер по per-node Now — разворачиваем (как локальный
			// daemon-транспорт).
			if item.GetTag() == selected {
				info.Now = selected
			}
			proxies = append(proxies, info)
		}
		return proxies, selected, nil
	}
	return nil, "", fmt.Errorf("lxd remote: group %q not found", group)
}

// SwitchProxy implements ProxyTransport через SelectOutbound.
func (t *LxdRemoteTransport) SwitchProxy(group, name string) error {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return err
	}
	defer cancel()
	if _, err := client.SelectOutbound(ctx, &daemonpb.SelectOutboundRequest{
		GroupTag: group, OutboundTag: name,
	}); err != nil {
		return fmt.Errorf("lxd remote SelectOutbound: %w", err)
	}
	return nil
}

// Delay implements ProxyTransport через URLTestOutbound (точечный URL-тест
// одного узла на СТОРОНЕ роутера — меряется его канал, а не наш).
func (t *LxdRemoteTransport) Delay(proxyName string) (int64, error) {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return 0, err
	}
	defer cancel()
	resp, err := client.URLTestOutbound(ctx, &daemonpb.URLTestOutboundRequest{
		OutboundTag: proxyName,
		Link:        api.GetPingTestURL(),
		// Timeout — миллисекунды (uint32); Interval у Subscribe* —
		// наносекунды. Всегда через time.Duration, чтобы не перепутать.
		Timeout: uint32((10 * time.Second).Milliseconds()),
	})
	if err != nil {
		return 0, fmt.Errorf("lxd remote URLTestOutbound: %w", err)
	}
	// Ядро сообщает неуспех теста полем error, а не gRPC-ошибкой: без этой
	// ветки недоступный узел показывался бы задержкой 0 мс, то есть «самым
	// быстрым» в списке.
	if msg := resp.GetError(); msg != "" {
		return 0, fmt.Errorf("%s", msg)
	}
	return int64(resp.GetDelay()), nil
}

// Убедимся на компиляции, что транспорт реализует интерфейс.
var _ ProxyTransport = (*LxdRemoteTransport)(nil)

// Groups возвращает список тегов selector-групп удалённого ядра. Нужен
// вкладке Servers, чтобы наполнить выпадающий список групп: локальный путь
// берёт их из config.json, а для чужой машины файла у нас нет.
func (t *LxdRemoteTransport) Groups() ([]string, error) {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return nil, err
	}
	defer cancel()
	groups, err := client.GetGroups(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("lxd remote GetGroups: %w", err)
	}
	tags := make([]string, 0, len(groups.GetGroup()))
	for _, g := range groups.GetGroup() {
		if tag := g.GetTag(); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags, nil
}
