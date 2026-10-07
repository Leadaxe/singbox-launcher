// File tailscale_status.go — состояние tailnet у работающего ядра: типы,
// конвертер pb → домен и кеш стрима (SPEC 130).
//
// Живёт в services, а не в core: транспорт удалённой машины (этот же пакет)
// держит свой стрим и кеш, а core импортирует services — обратная
// зависимость дала бы цикл. Конвертер ОДИН на оба пути: локальный демон и
// роутер обязаны давать одинаковый диагноз (довод chainInfosFromPB).
package services

import (
	"context"
	"errors"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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

// TailscalePeerPath — через что идёт пир, вердикт ядра (SPEC 115 ядра,
// правило magicsock): CurAddr → direct даже у idle-пира, иначе PeerRelay →
// peer_relay, иначе узел хоть раз писал пиру → derp, иначе пусто. Active в
// вердикт не входит — это отдельный флаг «писали только что».
type TailscalePeerPath string

const (
	TailscalePathNone      TailscalePeerPath = ""
	TailscalePathDirect    TailscalePeerPath = "direct"
	TailscalePathPeerRelay TailscalePeerPath = "peer_relay"
	TailscalePathDERP      TailscalePeerPath = "derp"
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
	// KeyExpiry — срок ключа устройства; нулевое время — срока нет.
	KeyExpiry time.Time
	// ShareeNode — устройство чужой tailnet, расшаренное в эту.
	ShareeNode bool
	// Path и детали пути (SPEC 115 ядра). В потоке — снимок на момент
	// последнего события IPN-шины, смена пути событием не является; правда
	// на момент запроса — TailscaleStatusRPC.
	Path TailscalePeerPath
	// Endpoint — ip:port при direct; иначе пусто.
	Endpoint string
	// PeerRelay — ip:port:vni при peer_relay; иначе пусто.
	PeerRelay string
	// DERPRegionCode — домашний DERP-регион пира; отдаётся и при direct,
	// пуст, пока регион неизвестен.
	DERPRegionCode string
	// LastHandshake — нулевое время, если хендшейка не было.
	LastHandshake time.Time
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
	// StateText — текст состояния ядра; UI берёт его только для значения
	// BackendState, которого лаунчер не знает (SPEC 148 §2).
	StateText string
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
	// Health — предупреждения ipnstate.Status.Health (нет связи с DERP, exit
	// node offline и т.п.); пусто — жалоб нет.
	Health []string
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
		st := TailscaleEndpointStatusFromPB(ep, at)
		out[st.EndpointTag] = st
	}
	return out
}

// TailscaleEndpointStatusFromPB — один endpoint; тот же конвертер для кадра
// потока и ответа GetTailscaleStatus (SPEC 115 ядра): поля в сообщении
// общие, различается только свежесть.
func TailscaleEndpointStatusFromPB(ep *daemonpb.TailscaleEndpointStatus, at time.Time) TailscaleStatus {
	st := TailscaleStatus{
		EndpointTag:    ep.GetEndpointTag(),
		BackendState:   ep.GetBackendState(),
		StateText:      ep.GetStateText(),
		AuthURL:        ep.GetAuthURL(),
		NetworkName:    ep.GetNetworkName(),
		MagicDNSSuffix: ep.GetMagicDNSSuffix(),
		KeyAuth:        ep.GetKeyAuth(),
		Self:           tailscalePeerFromPB(ep.GetSelf()),
		ExitNode:       tailscalePeerFromPB(ep.GetExitNode()),
		Health:         ep.GetHealth(),
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
	return st
}

// ErrTailscalePathUnsupported — ядро без GetTailscaleStatus (≤ 1.14.2-lx.12-rc.1
// или без with_lx_command): показывать то, что даёт поток, без пути.
var ErrTailscalePathUnsupported = errors.New("core does not report tailscale peer paths (needs sing-box-lx 1.14.2-lx.12-rc.2)")

// TailscaleStatusRPC — свежий статус одного tailscale-endpoint'а
// (GetTailscaleStatus, SPEC 115 ядра): ядро перечитывает Status() на каждый
// вызов, поэтому путь пира здесь — правда на момент запроса, а не снимок
// потока. ok=false без ошибки — тега нет, это не tailscale или endpoint не
// запущен (NotFound / InvalidArgument / FailedPrecondition): вкладка Network
// такие состояния уже объясняет. Unimplemented → ErrTailscalePathUnsupported.
func TailscaleStatusRPC(ctx context.Context, client daemonpb.StartedServiceClient, tag string) (TailscaleStatus, bool, error) {
	resp, err := client.GetTailscaleStatus(ctx, &daemonpb.TailscaleStatusRequest{EndpointTag: tag})
	if err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unimplemented:
				return TailscaleStatus{}, false, ErrTailscalePathUnsupported
			case codes.NotFound, codes.InvalidArgument, codes.FailedPrecondition:
				return TailscaleStatus{}, false, nil
			}
			return TailscaleStatus{}, false, errors.New(st.Message())
		}
		return TailscaleStatus{}, false, err
	}
	out := TailscaleEndpointStatusFromPB(resp, time.Now())
	if out.EndpointTag == "" {
		out.EndpointTag = tag
	}
	return out, true, nil
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
		ShareeNode:     p.GetShareeNode(),
		Path:           tailscalePathFromPB(p.GetPath()),
		Endpoint:       p.GetEndpoint(),
		PeerRelay:      p.GetPeerRelay(),
		DERPRegionCode: p.GetDerpRegionCode(),
	}
	if hs := p.GetLastHandshake(); hs > 0 {
		peer.LastHandshake = time.Unix(hs, 0)
	}
	// LastSeen — unix-секунды; ноль = ядро не знает, оставляем нулевое время.
	if ls := p.GetLastSeen(); ls > 0 {
		peer.LastSeen = time.Unix(ls, 0)
	}
	if ke := p.GetKeyExpiry(); ke > 0 {
		peer.KeyExpiry = time.Unix(ke, 0)
	}
	return peer
}

// tailscalePathFromPB — enum ядра → строковый тип; неизвестное значение
// (ядро новее лаунчера) — как «пути нет», а не как попало.
func tailscalePathFromPB(p daemonpb.TailscalePeerPath) TailscalePeerPath {
	switch p {
	case daemonpb.TailscalePeerPath_TAILSCALE_PEER_PATH_DIRECT:
		return TailscalePathDirect
	case daemonpb.TailscalePeerPath_TAILSCALE_PEER_PATH_PEER_RELAY:
		return TailscalePathPeerRelay
	case daemonpb.TailscalePeerPath_TAILSCALE_PEER_PATH_DERP:
		return TailscalePathDERP
	}
	return TailscalePathNone
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
