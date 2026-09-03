package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"singbox-launcher/api"
	"singbox-launcher/core/config"
	daemonpb "singbox-launcher/internal/daemonpb"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/lxdclient"
	"singbox-launcher/internal/traffic"
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

	// Кэш справочника устройств. Он опрашивается на каждой перерисовке
	// таблицы (раз в секунду), а меняется в масштабе часов — без кэша это
	// был бы HTTP-запрос к роутеру на каждый тик.
	clientsMu   sync.Mutex
	clientsAt   time.Time
	clientsMap  map[string]lxdclient.ClientInfo
	clientsErr  error
	clientsBusy bool
}

// clientsInfoTTL — срок жизни кэша справочника устройств.
//
// Совпадает с кэшем самого демона: свой TTL короче означал бы запросы, на
// которые демон всё равно отвечает из своего кэша — те же данные ценой
// лишнего похода к роутеру.
const clientsInfoTTL = time.Minute

// streamResubscribeDelay — пауза между попытками переподписки оборвавшегося
// стрима. Обрыв штатен: Deploy/Start/Stop пересоздают инстанс ядра, и
// server-side стрим StartedService умирает вместе с ним.
const streamResubscribeDelay = 2 * time.Second

// streamConn возвращает живое gRPC-соединение (ленивый dial под mutex'ом) —
// общий пролог всех Subscribe*.
func (t *LxdRemoteTransport) streamConn() (*grpc.ClientConn, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		conn, dialErr := t.client.DialGRPC()
		if dialErr != nil {
			return nil, fmt.Errorf("lxd remote: dial %s: %w", t.client.AddrString(), dialErr)
		}
		t.conn = conn
	}
	return t.conn, nil
}

// runResilientStream держит подписку живой до cancel.
//
// Deploy/Start/Stop ядра рвёт server-side стрим, и одноразовая горутина,
// умирая вместе с ним, оставляла профайлер/статус немыми до Disconnect/
// Connect машины. Теперь обрыв — не конец: onDrop() сбрасывает накопленное
// состояние (закрытые ядром соединения не должны отображаться живыми), пауза,
// переподписка. Единственный штатный выход — отмена ctx вызывающим.
//
// attempt открывает стрим и потребляет его до терминальной ошибки; ошибка
// первой же подписки равнозначна обрыву — ядро могло быть остановлено в
// момент открытия окна, и подписка догонит его на старте.
func runResilientStream(ctx context.Context, name string, attempt func() error, onDrop func()) {
	go func() {
		for {
			err := attempt()
			if onDrop != nil {
				onDrop()
			}
			if ctx.Err() != nil {
				return
			}
			debuglog.DebugLog("lxd remote %s: stream closed: %v — resubscribe in %s", name, err, streamResubscribeDelay)
			select {
			case <-ctx.Done():
				return
			case <-time.After(streamResubscribeDelay):
			}
		}
	}()
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

// rpcForURLTest — как rpc, но дедлайн не короче бюджета url-теста: при
// бюджете выше 25 секунд медленный узел возвращал бы транспортную ошибку
// вместо честной цифры, причём локальная проба той же цепочки при этом
// работала бы — расхождение транспортов на ровном месте.
func (t *LxdRemoteTransport) rpcForURLTest() (daemonpb.StartedServiceClient, context.Context, context.CancelFunc, error) {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return nil, nil, nil, err
	}
	budget := time.Duration(api.GetPingTestTimeoutMs())*time.Millisecond + 5*time.Second
	if budget <= lxdRemoteRPCTimeout {
		return client, ctx, cancel, nil
	}
	cancel()
	longCtx, longCancel := context.WithTimeout(context.Background(), budget)
	return client, longCtx, longCancel, nil
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
	// Пустая группа = список ещё не прочитан у машины (панель успела дёрнуть
	// refresh раньше). Спрашивать ядро про группу "" бессмысленно, а
	// «group "" not found» читается как поломка на той стороне.
	if strings.TrimSpace(group) == "" {
		return nil, "", errRemoteGroupUnknown
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
	client, ctx, cancel, err := t.rpcForURLTest()
	if err != nil {
		return 0, err
	}
	defer cancel()
	resp, err := client.URLTestOutbound(ctx, &daemonpb.URLTestOutboundRequest{
		OutboundTag: proxyName,
		Link:        api.GetPingTestURL(),
		// Timeout — миллисекунды (uint32); Interval у Subscribe* —
		// наносекунды, их легко перепутать.
		// Бюджет настраиваемый и единый во всех трёх транспортах: classic
		// GetDelay шлёт его же в query-параметре timeout.
		Timeout: uint32(api.GetPingTestTimeoutMs()),
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

// SubscribeGroupSelection следит за выбранным узлом группы (SPEC 097).
//
// Поток пушится ядром по событию перевыбора, а не по таймеру, поэтому окно
// узла отражает смену само — без опроса. Разовый GetGroups показал бы снимок
// на момент открытия и «замёрз» бы: у least_test перевыбор случается по
// результатам url-теста, то есть в любой момент.
//
// onSelected зовётся при КАЖДОМ кадре, включая повтор того же значения:
// фильтрацию оставляем вызывающему (у UI свой критерий «изменилось»).
// Пустой selected — валидное состояние (ядро не в STARTED), не ошибка.
//
// Возвращает функцию отмены: вызывающий обязан её позвать при закрытии окна,
// иначе стрим и горутина переживут его.
func (t *LxdRemoteTransport) SubscribeGroupSelection(group string, onSelected func(string)) (cancel func(), err error) {
	conn, err := t.streamConn()
	if err != nil {
		return nil, err
	}

	// Собственный контекст: стрим живёт, пока открыто окно, и не связан с
	// дедлайном одиночного RPC.
	ctx, cancelCtx := context.WithCancel(context.Background())
	runResilientStream(ctx, "groups", func() error {
		stream, serr := daemonpb.NewStartedServiceClient(conn).SubscribeGroups(ctx, &emptypb.Empty{})
		if serr != nil {
			return serr
		}
		for {
			groups, recvErr := stream.Recv()
			if recvErr != nil {
				return recvErr
			}
			for _, g := range groups.GetGroup() {
				if g.GetTag() == group {
					onSelected(g.GetSelected())
					break
				}
			}
		}
	}, nil)
	return cancelCtx, nil
}

// ChainPositionInfo / ChainInfo — состояние цепочки удалённого ядра
// (SPEC 110).
//
// Дублируют core.Chain*Info по той же причине, что PoolSlot ниже: core
// импортирует services, и обратная зависимость замкнула бы граф.
type ChainPositionInfo struct {
	Tag         string
	Now         string
	IsGroup     bool
	Transparent bool
	// Disabled — позиция выключена пользователем в рантайме (SPEC 075 ядра),
	// состояние живёт в cache-file удалённого ядра.
	Disabled   bool
	CloneState string
	LastError  string
}

type ChainInfo struct {
	Tag       string
	Positions []ChainPositionInfo
}

// Chains — состояние цепочек удалённого ядра (lx-RPC GetChains).
func (t *LxdRemoteTransport) Chains() ([]ChainInfo, error) {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return nil, err
	}
	defer cancel()
	list, err := client.GetChains(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("lxd remote GetChains: %w", err)
	}
	out := make([]ChainInfo, 0, len(list.GetChains()))
	for _, c := range list.GetChains() {
		if c == nil {
			continue
		}
		positions := make([]ChainPositionInfo, 0, len(c.GetPositions()))
		for _, p := range c.GetPositions() {
			if p == nil {
				continue
			}
			info := ChainPositionInfo{
				Tag:         p.GetTag(),
				Now:         p.GetNow(),
				IsGroup:     p.GetIsGroup(),
				Transparent: p.GetTransparent(),
				Disabled:    p.GetDisabled(),
			}
			if cl := p.GetClone(); cl != nil {
				info.CloneState = cl.GetState()
				info.LastError = cl.GetLastError()
			}
			positions = append(positions, info)
		}
		out = append(out, ChainInfo{Tag: c.GetTag(), Positions: positions})
	}
	return out, nil
}

// ProbeLayer — задержка префикса цепочки НА СТОРОНЕ роутера: меряется его
// канал, а не наш, как и обычный Delay выше.
//
// pos < 0 — сама цепочка целиком. Схема служебного тега принадлежит ядру и
// собирается общим config.ChainLayerTag, чтобы локальный и удалённый пути
// не разошлись.
func (t *LxdRemoteTransport) ProbeLayer(chainTag string, pos int) (int64, string, error) {
	client, ctx, cancel, err := t.rpcForURLTest()
	if err != nil {
		return 0, "", err
	}
	defer cancel()
	tag := chainTag
	if pos >= 0 {
		tag = config.ChainLayerTag(chainTag, pos)
	}
	resp, err := client.URLTestOutbound(ctx, &daemonpb.URLTestOutboundRequest{
		OutboundTag: tag,
		Link:        api.GetPingTestURL(),
		Timeout:     uint32(api.GetPingTestTimeoutMs()),
	})
	if err != nil {
		return 0, "", fmt.Errorf("lxd remote URLTestOutbound: %w", err)
	}
	// Ошибка ЯДРА приходит полем, а не gRPC-ошибкой: «хоп не поднялся» —
	// это диагноз для пользователя, и глотать его нельзя.
	return int64(resp.GetDelay()), resp.GetError(), nil
}

// ErrChainToggleUnsupported — удалённое ядро старше lx.28: метод объявлен
// в proto, реализации нет. Дубль core.ErrChainToggleUnsupported по той же
// причине, что и типы выше: core импортирует services, не наоборот.
var ErrChainToggleUnsupported = errors.New("core does not support chain position toggle — update the core")

// SetPositionEnabled — тумблер позиции цепочки на УДАЛЁННОМ ядре
// (SPEC 075 ядра). Состояние хранит его cache-file, не наш state.
//
// Бюджет как у URL-теста: включение позиции поднимает звено, а не только
// пишет флаг.
func (t *LxdRemoteTransport) SetPositionEnabled(chainTag string, pos int, enabled bool) (string, error) {
	client, ctx, cancel, err := t.rpcForURLTest()
	if err != nil {
		return "", err
	}
	defer cancel()
	resp, err := client.SetChainPositionEnabled(ctx, &daemonpb.SetChainPositionEnabledRequest{
		ChainTag: chainTag,
		Position: int32(pos),
		Enabled:  enabled,
	})
	if err != nil {
		if status.Code(err) == codes.Unimplemented {
			return "", ErrChainToggleUnsupported
		}
		return "", fmt.Errorf("lxd remote SetChainPositionEnabled: %w", err)
	}
	// Провал прогрева — диагноз узла, а не сбой вызова: едет данными.
	return resp.GetWarmupError(), nil
}

// PoolSlot — слот пула балансировщика удалённого ядра (SPEC 097).
//
// Дублирует core.PoolSlotInfo намеренно: core импортирует services, обратная
// зависимость замкнула бы граф. Вызывающий (core/backend.go) конвертирует.
type PoolSlot struct {
	Slot  uint32
	Tag   string
	Delay uint32
}

// PoolSlots — живой пул urltest-группы удалённого ядра (lx-RPC GetPool).
//
// Тот же RPC, что у локального демона: активный узел группы виден только
// работающему ядру, из конфига его не вычислить.
func (t *LxdRemoteTransport) PoolSlots(group string) ([]PoolSlot, error) {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return nil, err
	}
	defer cancel()
	pool, err := client.GetPool(ctx, &daemonpb.GetPoolRequest{GroupTag: group})
	if err != nil {
		return nil, fmt.Errorf("lxd remote GetPool: %w", err)
	}
	slots := make([]PoolSlot, 0, len(pool.GetSlots()))
	for _, s := range pool.GetSlots() {
		slots = append(slots, PoolSlot{Slot: s.GetSlot(), Tag: s.GetTag(), Delay: s.GetDelay()})
	}
	return slots, nil
}

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
	// Только РУЧНЫЕ группы (selector) — те, где узел выбирает пользователь.
	//
	// GetGroups отдаёт все группы ядра подряд, включая urltest (`auto-proxy-out`,
	// «Авто | Лучший сервер») и группы-обёртки вокруг отдельных узлов. В
	// дропдауне селектора им не место: выбор внутри urltest делает само ядро по
	// задержке, и подмена его руками ничего не решает.
	//
	// Тот же критерий, что у локальной стороны (GetSelectorGroupsFromConfig
	// отбирает outbounds с type=="selector"), — иначе список групп на Remote и
	// на Local означал бы разное.
	tags := make([]string, 0, len(groups.GetGroup()))
	for _, g := range groups.GetGroup() {
		if !strings.EqualFold(g.GetType(), "selector") {
			continue
		}
		if tag := g.GetTag(); tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags, nil
}

// errRemoteGroupUnknown — группа машины ещё не известна лаунчеру.
//
// Не ошибка машины: список групп читается у неё после соединения, и до этого
// момента спрашивать «дай узлы группы ""» нечего. Вызывающий показывает
// подсказку вместо диалога с сырым RPC-текстом.
var errRemoteGroupUnknown = errors.New("remote group is not known yet")

// IsRemoteGroupUnknown — проверка для UI-слоя.
func IsRemoteGroupUnknown(err error) bool { return errors.Is(err, errRemoteGroupUnknown) }

// SubscribeConnections открывает стрим соединений УДАЛЁННОЙ машины и
// возвращает источник снимков для traffic-профайлера (SPEC 059).
//
// Профайлер у каждой машины свой, как и список каналов: смотреть на свои
// соединения, когда открыт профайлер роутера, — то же самое, что показывать
// узлы чужого ядра под именем выбранной машины.
//
// Стрим живёт до вызова cancel (закрытия окна), поэтому контекст собственный,
// не связанный с дедлайном одиночного RPC.
func (t *LxdRemoteTransport) SubscribeConnections() (snapshot func(context.Context) (map[string]traffic.ClashConn, bool), cancel func(), err error) {
	conn, err := t.streamConn()
	if err != nil {
		return nil, nil, err
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	tracker := traffic.NewConnTracker()
	runResilientStream(ctx, "conns", func() error {
		stream, serr := daemonpb.NewStartedServiceClient(conn).SubscribeConnections(ctx,
			&daemonpb.SubscribeConnectionsRequest{
				// Интервал в НАНОСЕКУНДАХ (демон клампит снизу 200 мс). Прислать
				// сюда «1000», подумав про миллисекунды, — значит попросить тикер
				// раз в микросекунду и сжечь CPU на той стороне; так уже было.
				Interval: int64(time.Second),
			})
		if serr != nil {
			return serr
		}
		for {
			events, recvErr := stream.Recv()
			if recvErr != nil {
				return recvErr
			}
			// Reset = ядро перезапустилось (apply, Start/Stop): прежние
			// соединения мертвы, и держать их в снимке значило бы показывать
			// давно закрытые как живые.
			if events.GetReset_() {
				tracker.Reset()
			}
			for _, ev := range events.GetEvents() {
				tracker.ApplyEvent(ev)
			}
		}
		// Обрыв стрима: соединения, которые мы помним, больше не
		// подтверждены — Reset делает runResilientStream через onDrop.
	}, tracker.Reset)

	return tracker.Snapshot, cancelCtx, nil
}

// SubscribeDNSQueries открывает стрим DNS-запросов машины.
//
// includeAnswers=true обязателен: без ответов не восстановить CNAME-цепочку,
// а именно она отвечает на вопрос «куда на самом деле ушёл домен».
func (t *LxdRemoteTransport) SubscribeDNSQueries(onQuery func(DNSQuery)) (cancel func(), err error) {
	conn, err := t.streamConn()
	if err != nil {
		return nil, err
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	runResilientStream(ctx, "dns", func() error {
		stream, serr := daemonpb.NewStartedServiceClient(conn).SubscribeDNSQueries(ctx,
			&daemonpb.SubscribeDNSQueriesRequest{IncludeAnswers: true})
		if serr != nil {
			return serr
		}
		for {
			ev, recvErr := stream.Recv()
			if recvErr != nil {
				return recvErr
			}
			onQuery(DNSQueryFromProto(ev))
		}
	}, nil)
	return cancelCtx, nil
}

// --- Справочники и статус машины (SPEC 059, рецепт из lxd-grpc-api.md) ----

// RemoteRule — одно правило маршрутизации машины.
type RemoteRule struct {
	Type    string
	Payload string
	Action  string
}

// Rules возвращает таблицу правил ядра машины.
//
// Нужна, чтобы расшифровать Connection.rule: в соединении лежит строка
// правила, и без таблицы она читается как невнятный индекс.
func (t *LxdRemoteTransport) Rules() ([]RemoteRule, error) {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return nil, err
	}
	defer cancel()
	list, err := client.GetRules(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("lxd remote GetRules: %w", err)
	}
	out := make([]RemoteRule, 0, len(list.GetRules()))
	for _, r := range list.GetRules() {
		out = append(out, RemoteRule{
			Type:    r.GetType(),
			Payload: r.GetPayload(),
			Action:  r.GetAction(),
		})
	}
	return out, nil
}

// Outbounds возвращает теги outbound'ов машины — для резолва цепочек.
func (t *LxdRemoteTransport) Outbounds() ([]string, error) {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return nil, err
	}
	defer cancel()
	list, err := client.GetOutbounds(ctx, &emptypb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("lxd remote GetOutbounds: %w", err)
	}
	out := make([]string, 0, len(list.GetOutbounds()))
	for _, o := range list.GetOutbounds() {
		out = append(out, o.GetTag())
	}
	return out, nil
}

// StartedAt — момент запуска ядра машины (точка отсчёта uptime).
func (t *LxdRemoteTransport) StartedAt() (time.Time, error) {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return time.Time{}, err
	}
	defer cancel()
	resp, err := client.GetStartedAt(ctx, &emptypb.Empty{})
	if err != nil {
		return time.Time{}, fmt.Errorf("lxd remote GetStartedAt: %w", err)
	}
	ms := resp.GetStartedAt()
	if ms <= 0 {
		return time.Time{}, nil
	}
	return time.UnixMilli(ms), nil
}

// RemoteStatus — сводка ядра машины для шапки профайлера.
type RemoteStatus struct {
	Memory         uint64
	Goroutines     int32
	ConnectionsIn  int32
	ConnectionsOut int32
	Uplink         int64
	Downlink       int64
	UplinkTotal    int64
	DownlinkTotal  int64
}

// SubscribeStatus открывает стрим сводки ядра машины.
func (t *LxdRemoteTransport) SubscribeStatus(onStatus func(RemoteStatus)) (cancel func(), err error) {
	conn, err := t.streamConn()
	if err != nil {
		return nil, err
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	runResilientStream(ctx, "status", func() error {
		stream, serr := daemonpb.NewStartedServiceClient(conn).SubscribeStatus(ctx,
			// Наносекунды, как и у прочих Subscribe*.
			&daemonpb.SubscribeStatusRequest{Interval: int64(time.Second)})
		if serr != nil {
			return serr
		}
		for {
			st, recvErr := stream.Recv()
			if recvErr != nil {
				return recvErr
			}
			onStatus(RemoteStatus{
				Memory:         st.GetMemory(),
				Goroutines:     st.GetGoroutines(),
				ConnectionsIn:  st.GetConnectionsIn(),
				ConnectionsOut: st.GetConnectionsOut(),
				Uplink:         st.GetUplink(),
				Downlink:       st.GetDownlink(),
				UplinkTotal:    st.GetUplinkTotal(),
				DownlinkTotal:  st.GetDownlinkTotal(),
			})
		}
	}, nil)
	return cancelCtx, nil
}

// ClientsInfo — справочник устройств локальной сети машины, из кэша.
//
// Никогда не ходит по сети синхронно: зовётся из UI-потока на каждой
// перерисовке таблицы, и HTTP к роутеру там подвесил бы окно на время
// запроса. Просроченный кэш обновляется в фоне, а вызывающему сразу отдаётся
// то, что есть, — устаревшее имя устройства безобиднее замершего интерфейса.
//
// Второе значение — ok=false, пока справочник ни разу не получен: пустая карта
// тогда означает «ещё не знаем», а не «устройств нет».
func (t *LxdRemoteTransport) ClientsInfo() (map[string]lxdclient.ClientInfo, bool) {
	t.clientsMu.Lock()
	cur, at, busy := t.clientsMap, t.clientsAt, t.clientsBusy
	fresh := time.Since(at) < clientsInfoTTL
	if !fresh && !busy {
		// Флаг занятости не даёт запустить второе обновление, пока идёт
		// первое: таблица перерисовывается раз в секунду, и без него на
		// просроченном кэше стартовал бы запрос на каждый тик.
		t.clientsBusy = true
		go t.refreshClientsInfo()
	}
	t.clientsMu.Unlock()
	return cur, cur != nil
}

// refreshClientsInfo обновляет кэш справочника. Работает в своей горутине.
func (t *LxdRemoteTransport) refreshClientsInfo() {
	m, err := t.client.ClientsInfo()
	t.clientsMu.Lock()
	defer t.clientsMu.Unlock()
	t.clientsBusy = false
	t.clientsErr = err
	if err != nil {
		// Прошлую карту при ошибке НЕ выбрасываем: связь могла моргнуть, а
		// имена устройств от этого не перестали быть верными. Метку времени
		// тоже не двигаем — иначе следующая попытка отложилась бы на минуту.
		return
	}
	// Пустой ответ — это ответ: справочник может не знать ни одного устройства
	// (вне Linux молчат все провайдеры, кроме меток). Оставить nil значило бы
	// «ещё не получали», и кэш перезапрашивался бы вечно.
	if m == nil {
		m = map[string]lxdclient.ClientInfo{}
	}
	t.clientsMap = m
	t.clientsAt = time.Now()
}

// ClientsInfoError — последняя ошибка обновления справочника (nil, если всё
// хорошо). Демон старых версий эндпоинта не знает, и это не повод шуметь.
func (t *LxdRemoteTransport) ClientsInfoError() error {
	t.clientsMu.Lock()
	defer t.clientsMu.Unlock()
	return t.clientsErr
}

// SetClientLabel задаёт собственное имя устройства и сбрасывает кэш, чтобы
// имя появилось в таблице сразу, а не через минуту.
func (t *LxdRemoteTransport) SetClientLabel(key, name string) error {
	if err := t.client.SetClientLabel(key, name); err != nil {
		return err
	}
	t.invalidateClientsInfo()
	return nil
}

// DeleteClientLabel снимает собственное имя устройства.
func (t *LxdRemoteTransport) DeleteClientLabel(key string) error {
	if err := t.client.DeleteClientLabel(key); err != nil {
		return err
	}
	t.invalidateClientsInfo()
	return nil
}

func (t *LxdRemoteTransport) invalidateClientsInfo() {
	t.clientsMu.Lock()
	t.clientsAt = time.Time{}
	t.clientsMu.Unlock()
}

// HostInfo — снимок машины: CPU, память, термодатчики, диски, дескрипторы
// (GET /admin/host, SPEC 068 форка).
//
// Без кэша, в отличие от справочника устройств: там данные меняются в
// масштабе часов, а здесь смысл ровно в свежести. Темп задаёт вызывающий —
// проценты демон считает как дельту между ДВУМЯ соседними запросами, и
// частота опроса прямо определяет окно усреднения.
//
// Звать из горутины: это поход к роутеру.
func (t *LxdRemoteTransport) HostInfo() (lxdclient.HostInfo, error) {
	return t.client.Host()
}

// HostInterfaces — интерфейсы машины со счётчиками и скоростями
// (GET /admin/host/interfaces).
//
// Скорости появятся со второго вызова по той же причине, что и проценты CPU.
func (t *LxdRemoteTransport) HostInterfaces() (lxdclient.HostInterfaces, error) {
	return t.client.HostInterfaces()
}

// HostInterfacesWithin — тот же список, но со своим сроком ожидания вместо
// общего REST-дедлайна клиента.
//
// Для справочных потребителей (пикер интерфейсов в конфигураторе): там ответ
// нужен, только пока пользователь готов его ждать, а неотвечающая машина
// иначе держит очередь запроса десятки секунд (SPEC 113-E M6).
func (t *LxdRemoteTransport) HostInterfacesWithin(timeout time.Duration) (lxdclient.HostInterfaces, error) {
	return t.client.HostInterfacesWithin(timeout)
}

// CloseConnection обрывает одно соединение машины по его UUID.
func (t *LxdRemoteTransport) CloseConnection(id string) error {
	client, ctx, cancel, err := t.rpc()
	if err != nil {
		return err
	}
	defer cancel()
	if _, err := client.CloseConnection(ctx, &daemonpb.CloseConnectionRequest{Id: id}); err != nil {
		return fmt.Errorf("lxd remote CloseConnection: %w", err)
	}
	return nil
}
