package traffic

import (
	"context"
	"net"
	"path/filepath"
	"sync"
	"time"

	daemonpb "singbox-launcher/internal/daemonpb"
)

// Трекер соединений поверх gRPC-стрима SubscribeConnections.
//
// Живёт здесь, а не в core/backend_daemon_*: тот файл darwin-only, потому что
// СВОЙ демон бывает только на macOS. Наблюдать за УДАЛЁННОЙ машиной можно с
// любой платформы — лаунчер на Windows управляет linux-роутером, — и трекер
// обязан собираться везде.

// ConnTracker держит текущее множество открытых соединений, наполняемое
// стримом: NEW/UPDATE добавляют и обновляют, CLOSED удаляет.
type ConnTracker struct {
	mu    sync.Mutex
	conns map[string]ClashConn
	// live — стрим хоть раз доставил данные. До этого Snapshot отдаёт
	// ok=false: пустой снимок до первого события профайлер принял бы за
	// «все соединения закрылись» и обнулил бы график.
	live bool
}

// NewConnTracker создаёт пустой трекер.
func NewConnTracker() *ConnTracker {
	return &ConnTracker{conns: make(map[string]ClashConn)}
}

// ApplyEvent применяет одно событие стрима.
//
// Это ДЕЛЬТА-протокол, а не повторяющийся снимок. Каждый тип события несёт
// своё, и путать их нельзя:
//
//   - NEW — полный объект соединения;
//   - UPDATE — ТОЛЬКО id и приращения трафика, connection всегда nil;
//   - CLOSED — id и closedAt; сам объект может отсутствовать.
//
// Отсюда два правила, на которых легко ошибиться. Пропускать событие с
// пустым connection нельзя: тогда счётчики трафика не растут вовсе, и все
// соединения выглядят молчащими. Удаление ключуется по id, а не по вложенному
// объекту, которого может не быть.
func (t *ConnTracker) ApplyEvent(ev *daemonpb.ConnectionEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.live = true

	switch ev.GetType() {
	case daemonpb.ConnectionEventType_CONNECTION_EVENT_CLOSED:
		delete(t.conns, ev.GetId())

	case daemonpb.ConnectionEventType_CONNECTION_EVENT_UPDATE:
		// Итоги ведём сами: сервер прислал только приращение с прошлого тика.
		// Нулевая дельта осмысленна — она означает «соединение передавало, а
		// сейчас нет», и по ней скорость в UI падает до нуля вместо того,
		// чтобы навсегда застыть на последнем ненулевом значении.
		cur, ok := t.conns[ev.GetId()]
		if !ok {
			return // UPDATE по неизвестному id — ждём NEW/reset
		}
		cur.Upload += ev.GetUplinkDelta()
		cur.Download += ev.GetDownlinkDelta()
		t.conns[ev.GetId()] = cur

	default: // NEW
		if c := ev.GetConnection(); c != nil {
			t.conns[c.GetId()] = ProtoConnToClash(c)
		}
	}
}

// Reset очищает состояние: ядро перезапустилось, прежние соединения мертвы.
func (t *ConnTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.conns = make(map[string]ClashConn)
	t.live = false
}

// Snapshot отдаёт копию текущего множества соединений.
func (t *ConnTracker) Snapshot(_ context.Context) (map[string]ClashConn, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.live {
		return nil, false
	}
	out := make(map[string]ClashConn, len(t.conns))
	for k, v := range t.conns {
		out[k] = v
	}
	return out, true
}

// ProtoConnToClash переводит соединение из gRPC в форму, которую понимает
// профайлер (та же, что приходит из Clash /connections).
func ProtoConnToClash(c *daemonpb.Connection) ClashConn {
	host, portStr := c.GetDomain(), ""
	dst := c.GetDestination() // "ip:port" либо "host:port"
	if h, p, err := net.SplitHostPort(dst); err == nil {
		if host == "" {
			host = h
		}
		portStr = p
	}
	destIP := ""
	if ip := net.ParseIP(hostOnlyPart(dst)); ip != nil {
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
	return ClashConn{
		ID:       c.GetId(),
		Upload:   c.GetUplinkTotal(),
		Download: c.GetDownlinkTotal(),
		Start:    start,
		// chainList + detourList: хвост транспортных обёрток (SPEC 017 форка)
		// в chainList не входит by design, а именно он отвечает на вопрос
		// «через что реально ушло соединение».
		Chains: appendDetour(c.GetChainList(), c.GetDetourList()),
		Rule:   c.GetRule(),
		Metadata: ClashConnMeta{
			SourceAddr:      c.GetSource(),
			Network:         c.GetNetwork(),
			Host:            host,
			DestinationIP:   destIP,
			DestinationPort: portStr,
			ProcessPath:     proc,
			Process:         ProcessBase(proc),
		},
	}
}

func hostOnlyPart(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

// ProcessBase — имя исполняемого файла из полного пути.
//
// На удалённой машине путь всегда в unix-форме, даже если лаунчер работает на
// Windows, поэтому filepath.Base местного разделителя тут мало: сначала
// пробуем срез по «/», а уже потом платформенный Base.
func ProcessBase(path string) string {
	if path == "" {
		return ""
	}
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return filepath.Base(path)
}

// appendDetour дописывает detour-хвост к цепочке outbound'ов, пропуская уже
// присутствующие звенья.
//
// Дубли реальны: часть обёрток попадает в обе коллекции, и без проверки
// цепочка в UI читалась бы как «vless → vless → tls».
func appendDetour(chain, detour []string) []string {
	if len(detour) == 0 {
		return chain
	}
	seen := make(map[string]struct{}, len(chain))
	for _, c := range chain {
		seen[c] = struct{}{}
	}
	out := chain
	for _, d := range detour {
		if _, dup := seen[d]; dup || d == "" {
			continue
		}
		out = append(out, d)
		seen[d] = struct{}{}
	}
	return out
}
