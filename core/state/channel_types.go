package state

// Channel — пользовательский канал роутинга (SPEC 087, порт LxBox §125/§267).
//
// Канал = именованная группа серверов под задачу: отбирает узлы regex-фильтром
// по итоговому tag'у ноды, переключается selector'ом, опционально имеет
// urltest-двойник (<tag>-auto) с round_robin/balancer. Один сервер входит в
// несколько каналов. Правила и route_final ссылаются на канал по Tag.
//
// ВАЖНО: Channels живут отдельным top-level полем state.State.Channels, НЕ внутри
// Connections.Outbounds — они НЕ зеркалятся в legacy ParserConfig.Outbounds и
// материализуются в selector/urltest только на build-time (снимает риск отката
// уровня a58a176). См. SPECS/087-R-N-CHANNELS_MODEL/SPEC.md.
type Channel struct {
	// Tag — immutable системный id (`vpn-1`..`vpn-10`). Автоген; юзер не правит.
	// `vpn-1` неудаляем и всегда enabled (IsRequired).
	Tag string `json:"tag"`

	// Label — отображаемое имя (VPN ①, …). Юзер правит.
	Label string `json:"label"`

	// Enabled — включён ли канал (материализуется в config только если true).
	Enabled bool `json:"enabled"`

	// IncludeDirect — добавить `direct-out` опцией в selector.
	IncludeDirect bool `json:"include_direct"`

	// IncludeBlock — добавить `block-out` опцией в selector (drop). В detour запрещён.
	IncludeBlock bool `json:"include_block"`

	// NodeFilter — regex по итоговому tag ноды. "" = все ноды пула.
	NodeFilter string `json:"node_filter"`

	// NodeFilterInvert — инверсия фильтра (ноды НЕ матчащие).
	NodeFilterInvert bool `json:"node_filter_invert"`

	// DefaultFilter — regex; первая matched нода → selector.default.
	DefaultFilter string `json:"default_filter"`

	// InterruptExistConnections — selector.interrupt_exist_connections.
	InterruptExistConnections bool `json:"interrupt_exist_connections"`

	// IsDetour — канал = detour-прослойка (цель для detour-серверов),
	// исключён из целей правил; block в detour запрещён.
	IsDetour bool `json:"detour"`

	// Auto — nil = urltest-двойник ВЫКЛ; иначе параметры двойника <tag>-auto.
	Auto *ChannelAuto `json:"auto"`
}

// ChannelAuto — параметры urltest-двойника канала (`<tag>-auto`).
type ChannelAuto struct {
	URL                       string `json:"url"`
	Interval                  string `json:"interval"`
	Tolerance                 int    `json:"tolerance"`
	IdleTimeout               string `json:"idle_timeout"`
	InterruptExistConnections bool   `json:"interrupt_exist_connections"`
	// Mode — "least_test" (деф, один лучший узел) | "round_robin" (пул + sticky).
	Mode     string           `json:"mode,omitempty"`
	Balancer *ChannelBalancer `json:"balancer,omitempty"`
}

// ChannelBalancer — параметры round_robin-балансировки (ядро SPEC 019).
type ChannelBalancer struct {
	Pool          int `json:"pool"`
	PoolTolerance int `json:"pool_tolerance"`
	// StickyHash — компоненты липкости (process/domain/source_ip/dest_ip/dest_port).
	// Отключение — ТОЛЬКО sentinel ["none"], НЕ []; см. sanitizeBalancerOptions.
	StickyHash []string `json:"sticky_hash"`
}

// AutoTag — производный tag urltest-двойника. В state не хранится.
func (c Channel) AutoTag() string { return c.Tag + "-auto" }

// IsRequired — vpn-1 неудаляем и всегда enabled (порт channel.dart:268).
func (c Channel) IsRequired() bool { return c.Tag == RequiredChannelTag }

// RequiredChannelTag — тег основного канала, который нельзя удалить/выключить.
const RequiredChannelTag = "vpn-1"
