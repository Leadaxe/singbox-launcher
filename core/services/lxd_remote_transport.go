package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"singbox-launcher/api"
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
	t.mu.Lock()
	if t.conn == nil {
		conn, dialErr := t.client.DialGRPC()
		if dialErr != nil {
			t.mu.Unlock()
			return nil, fmt.Errorf("lxd remote: dial %s: %w", t.client.AddrString(), dialErr)
		}
		t.conn = conn
	}
	conn := t.conn
	t.mu.Unlock()

	// Собственный контекст: стрим живёт, пока открыто окно, и не связан с
	// дедлайном одиночного RPC.
	ctx, cancelCtx := context.WithCancel(context.Background())
	stream, err := daemonpb.NewStartedServiceClient(conn).SubscribeGroups(ctx, &emptypb.Empty{})
	if err != nil {
		cancelCtx()
		return nil, fmt.Errorf("lxd remote SubscribeGroups: %w", err)
	}
	go func() {
		for {
			groups, recvErr := stream.Recv()
			if recvErr != nil {
				return // обрыв или отмена — окно закрылось либо ядро легло
			}
			for _, g := range groups.GetGroup() {
				if g.GetTag() == group {
					onSelected(g.GetSelected())
					break
				}
			}
		}
	}()
	return cancelCtx, nil
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
	t.mu.Lock()
	if t.conn == nil {
		conn, dialErr := t.client.DialGRPC()
		if dialErr != nil {
			t.mu.Unlock()
			return nil, nil, fmt.Errorf("lxd remote: dial %s: %w", t.client.AddrString(), dialErr)
		}
		t.conn = conn
	}
	conn := t.conn
	t.mu.Unlock()

	ctx, cancelCtx := context.WithCancel(context.Background())
	stream, err := daemonpb.NewStartedServiceClient(conn).SubscribeConnections(ctx,
		&daemonpb.SubscribeConnectionsRequest{
			// Интервал в НАНОСЕКУНДАХ (демон клампит снизу 200 мс). Прислать
			// сюда «1000», подумав про миллисекунды, — значит попросить тикер
			// раз в микросекунду и сжечь CPU на той стороне; так уже было.
			Interval: int64(time.Second),
		})
	if err != nil {
		cancelCtx()
		return nil, nil, fmt.Errorf("lxd remote SubscribeConnections: %w", err)
	}

	tracker := traffic.NewConnTracker()
	go func() {
		for {
			events, recvErr := stream.Recv()
			if recvErr != nil {
				// Обрыв стрима: соединения, которые мы помним, больше не
				// подтверждены — сбрасываем, иначе профайлер продолжил бы
				// показывать давно закрытые как живые.
				tracker.Reset()
				debuglog.DebugLog("lxd remote conns: stream closed: %v", recvErr)
				return
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
	}()

	return tracker.Snapshot, cancelCtx, nil
}

// dnsTypeCNAME — код записи CNAME в DNS (RFC 1035).
const dnsTypeCNAME = 5

// DNSQuery — одно событие DNS-плоскости удалённой машины (SPEC 018 форка).
//
// Структурный поток вместо разбора лога: приходит и сервер, который отвечал,
// и цепочка outbound'ов, и признак отказа — то, что из текстовой строки лога
// пришлось бы вытаскивать регулярками.
type DNSQuery struct {
	Domain  string
	Failed  bool
	Error   string
	Answers []string
	// CNAMEs — цепочка переадресаций, собранная из ответов типа CNAME.
	CNAMEs      []string
	DNSServer   string
	ProcessPath string
}

// SubscribeDNSQueries открывает стрим DNS-запросов машины.
//
// includeAnswers=true обязателен: без ответов не восстановить CNAME-цепочку,
// а именно она отвечает на вопрос «куда на самом деле ушёл домен».
func (t *LxdRemoteTransport) SubscribeDNSQueries(onQuery func(DNSQuery)) (cancel func(), err error) {
	t.mu.Lock()
	if t.conn == nil {
		conn, dialErr := t.client.DialGRPC()
		if dialErr != nil {
			t.mu.Unlock()
			return nil, fmt.Errorf("lxd remote: dial %s: %w", t.client.AddrString(), dialErr)
		}
		t.conn = conn
	}
	conn := t.conn
	t.mu.Unlock()

	ctx, cancelCtx := context.WithCancel(context.Background())
	stream, err := daemonpb.NewStartedServiceClient(conn).SubscribeDNSQueries(ctx,
		&daemonpb.SubscribeDNSQueriesRequest{IncludeAnswers: true})
	if err != nil {
		cancelCtx()
		return nil, fmt.Errorf("lxd remote SubscribeDNSQueries: %w", err)
	}

	go func() {
		for {
			ev, recvErr := stream.Recv()
			if recvErr != nil {
				debuglog.DebugLog("lxd remote dns: stream closed: %v", recvErr)
				return
			}
			q := DNSQuery{
				Domain:    ev.GetDomain(),
				Failed:    ev.GetFailed(),
				Error:     ev.GetError(),
				DNSServer: ev.GetDnsServer(),
			}
			if pi := ev.GetProcessInfo(); pi != nil {
				q.ProcessPath = pi.GetProcessPath()
			}
			for _, a := range ev.GetAnswers() {
				rdata := a.GetRdata()
				if rdata == "" {
					continue
				}
				// Type 5 = CNAME: из этих записей клиент и восстанавливает
				// цепочку переадресаций, отдельной сущностью она не приходит.
				if a.GetType() == dnsTypeCNAME {
					q.CNAMEs = append(q.CNAMEs, rdata)
					continue
				}
				q.Answers = append(q.Answers, rdata)
			}
			onQuery(q)
		}
	}()
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
	t.mu.Lock()
	if t.conn == nil {
		conn, dialErr := t.client.DialGRPC()
		if dialErr != nil {
			t.mu.Unlock()
			return nil, fmt.Errorf("lxd remote: dial %s: %w", t.client.AddrString(), dialErr)
		}
		t.conn = conn
	}
	conn := t.conn
	t.mu.Unlock()

	ctx, cancelCtx := context.WithCancel(context.Background())
	stream, err := daemonpb.NewStartedServiceClient(conn).SubscribeStatus(ctx,
		// Наносекунды, как и у прочих Subscribe*.
		&daemonpb.SubscribeStatusRequest{Interval: int64(time.Second)})
	if err != nil {
		cancelCtx()
		return nil, fmt.Errorf("lxd remote SubscribeStatus: %w", err)
	}
	go func() {
		for {
			st, recvErr := stream.Recv()
			if recvErr != nil {
				debuglog.DebugLog("lxd remote status: stream closed: %v", recvErr)
				return
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
	}()
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
