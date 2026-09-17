// File tailscale_status.go — состояние tailnet у работающего ядра: типы,
// конвертер pb → домен и кеш стрима (SPEC 130).
//
// Живёт в services, а не в core: транспорт удалённой машины (этот же пакет)
// держит свой стрим и кеш, а core импортирует services — обратная
// зависимость дала бы цикл. Конвертер ОДИН на оба пути: локальный демон и
// роутер обязаны давать одинаковый диагноз (довод chainInfosFromPB).
package services

import (
	"sync"
	"time"

	daemonpb "singbox-launcher/internal/daemonpb"
)

// TailscaleBackendState — состояние IPN-бэкенда словами ядра. Строки взяты
// из tailscale ipn.State.String(); StateText ядра НЕ используется: locale в
// gRPC-метаданных лаунчер не шлёт, и текст пришёл бы на дефолте ядра.
const (
	TailscaleStateNoState    = "NoState"
	TailscaleStateNeedsLogin = "NeedsLogin"
	TailscaleStateStopped    = "Stopped"
	TailscaleStateStarting   = "Starting"
	TailscaleStateRunning    = "Running"
)

// TailscalePeer — участник tailnet, как его видит ядро.
type TailscalePeer struct {
	HostName     string
	DNSName      string
	OS           string
	TailscaleIPs []string
	Online       bool
	// ExitNode — через этот пир сейчас идёт выход наружу.
	ExitNode bool
	// ExitNodeOption — пир анонсирует себя выходом (и это одобрено).
	ExitNodeOption bool
	Active         bool
	RxBytes        int64
	TxBytes        int64
	LastSeen       time.Time
	Expired        bool
	StableID       string
}

// TailscaleUserGroup — пиры одного пользователя tailnet.
type TailscaleUserGroup struct {
	LoginName   string
	DisplayName string
	Peers       []TailscalePeer
}

// TailscaleStatus — состояние одного tailscale-endpoint'а.
type TailscaleStatus struct {
	EndpointTag  string
	BackendState string
	// AuthURL — ссылка входа; непуста только при NeedsLogin у узла без
	// auth_key. Сегодня видна только в логе ядра, на роутере — нигде.
	AuthURL        string
	NetworkName    string
	MagicDNSSuffix string
	// KeyAuth — вход по auth_key (иначе интерактивный).
	KeyAuth bool
	// Self — этот узел; nil в NoState/NeedsLogin, когда адреса ещё нет.
	Self *TailscalePeer
	// ExitNode — пир, через который идёт выход; nil = выход не используется.
	ExitNode   *TailscalePeer
	UserGroups []TailscaleUserGroup
	// ReceivedAt — когда снимок пришёл; UI показывает возраст при обрыве.
	ReceivedAt time.Time
}

// PeersOnline / PeersTotal — сводка для короткой подписи.
func (s *TailscaleStatus) PeersOnline() (online, total int) {
	if s == nil {
		return 0, 0
	}
	for _, g := range s.UserGroups {
		for _, p := range g.Peers {
			total++
			if p.Online {
				online++
			}
		}
	}
	return online, total
}

// TailscaleStatusesFromPB — перевод сообщения стрима во внутренний тип,
// по тегу endpoint'а. Экспортирован: тот же конвертер зовёт транспорт
// удалённой машины из пакета services.
func TailscaleStatusesFromPB(upd *daemonpb.TailscaleStatusUpdate, at time.Time) map[string]TailscaleStatus {
	out := make(map[string]TailscaleStatus, len(upd.GetEndpoints()))
	for _, ep := range upd.GetEndpoints() {
		if ep == nil || ep.GetEndpointTag() == "" {
			continue
		}
		st := TailscaleStatus{
			EndpointTag:    ep.GetEndpointTag(),
			BackendState:   ep.GetBackendState(),
			AuthURL:        ep.GetAuthURL(),
			NetworkName:    ep.GetNetworkName(),
			MagicDNSSuffix: ep.GetMagicDNSSuffix(),
			KeyAuth:        ep.GetKeyAuth(),
			Self:           tailscalePeerFromPB(ep.GetSelf()),
			ExitNode:       tailscalePeerFromPB(ep.GetExitNode()),
			ReceivedAt:     at,
		}
		for _, g := range ep.GetUserGroups() {
			if g == nil {
				continue
			}
			grp := TailscaleUserGroup{LoginName: g.GetLoginName(), DisplayName: g.GetDisplayName()}
			for _, p := range g.GetPeers() {
				if peer := tailscalePeerFromPB(p); peer != nil {
					grp.Peers = append(grp.Peers, *peer)
				}
			}
			st.UserGroups = append(st.UserGroups, grp)
		}
		out[st.EndpointTag] = st
	}
	return out
}

// tailscalePeerFromPB — один пир; nil на nil (Self пуст до входа).
func tailscalePeerFromPB(p *daemonpb.TailscalePeer) *TailscalePeer {
	if p == nil {
		return nil
	}
	peer := &TailscalePeer{
		HostName:       p.GetHostName(),
		DNSName:        p.GetDnsName(),
		OS:             p.GetOs(),
		TailscaleIPs:   p.GetTailscaleIPs(),
		Online:         p.GetOnline(),
		ExitNode:       p.GetExitNode(),
		ExitNodeOption: p.GetExitNodeOption(),
		Active:         p.GetActive(),
		RxBytes:        p.GetRxBytes(),
		TxBytes:        p.GetTxBytes(),
		Expired:        p.GetExpired(),
		StableID:       p.GetStableID(),
	}
	// LastSeen — unix-секунды; ноль = ядро не знает, оставляем нулевое время.
	if ls := p.GetLastSeen(); ls > 0 {
		peer.LastSeen = time.Unix(ls, 0)
	}
	return peer
}

// TailscaleStatusCache — кеш последнего снимка, общий для обоих gRPC-путей.
//
// Живёт в бэкенде (DaemonBackend) и в транспорте удалённой машины
// (LxdRemoteTransport): у каждого свой стрим, но правила хранения одни.
// Обрыв стрима кеш НЕ чистит — MarkDead только снимает «живость»: старый
// снимок с возрастом честнее пустого экрана, а свежая подписка перезапишет.
type TailscaleStatusCache struct {
	mu     sync.Mutex
	byTag  map[string]TailscaleStatus
	live   bool
	onSnap func()
}

// Apply — новый снимок целиком заменяет прежний (стрим отдаёт все
// endpoint'ы разом, ушедшие из ответа ушли и из ядра).
func (c *TailscaleStatusCache) Apply(upd *daemonpb.TailscaleStatusUpdate) {
	snap := TailscaleStatusesFromPB(upd, time.Now())
	c.mu.Lock()
	c.byTag = snap
	c.live = true
	cb := c.onSnap
	c.mu.Unlock()
	if cb != nil {
		cb()
	}
}

// MarkDead — стрим оборвался; снимок остаётся, живость снимается.
func (c *TailscaleStatusCache) MarkDead() {
	c.mu.Lock()
	c.live = false
	c.mu.Unlock()
}

// Get — статус по тегу. ok=false — тега в снимке нет (или снимка нет).
func (c *TailscaleStatusCache) Get(tag string) (TailscaleStatus, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	st, ok := c.byTag[tag]
	return st, ok
}

// Live — пришёл ли хотя бы один кадр и не оборван ли стрим.
func (c *TailscaleStatusCache) Live() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.live
}

// OnSnapshot — колбэк на каждый новый снимок (UI перерисовывает колонку).
func (c *TailscaleStatusCache) OnSnapshot(fn func()) {
	c.mu.Lock()
	c.onSnap = fn
	c.mu.Unlock()
}
