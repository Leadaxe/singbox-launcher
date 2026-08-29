// Package configtypes contains shared data types for configuration parsing.
// Extracted to its own package to break the circular dependency between
// core/config and core/config/subscription: both packages import configtypes
// for shared types, while core/config can now safely import subscription.
package configtypes

import (
	"encoding/json"
	"fmt"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"singbox-launcher/internal/constants"
)

// ParserConfigVersion is the current version of ParserConfig format
const ParserConfigVersion = 4

// canonicalGOOSName returns the canonical-case OS name used in our
// subscription request headers (`X-Device-OS`) and User-Agent.
//
// Form is `macOS` / `windows` / `linux` to match the Remnawave HWID docs
// (https://docs.rw/docs/features/hwid-device-limit/), which is the panel
// generation that actually parses these fields. Unknown GOOS (rare:
// `freebsd`, `netbsd`, …) falls through unchanged — better than masking
// it as "linux" and breaking eventual support if those builds ship.
func canonicalGOOSName(goos string) string {
	switch goos {
	case "darwin":
		return "macOS"
	case "windows":
		return "windows"
	case "linux":
		return "linux"
	default:
		return goos
	}
}

// BuildSubscriptionUserAgent returns the User-Agent string sent on every
// subscription fetch. Format follows the de-facto product/version (platform)
// convention used by Mozilla / v2rayNG / hiddify and required by HWID-binding
// panels (Remnawave / Marzneshin) which reject unknown clients like our
// previous `SubscriptionParserClient` and return 0-byte bodies.
//
// Examples:
//
//	LxBox/1.1.4 (desktop; macOS)
//	LxBox/1.1.4 (desktop; windows)
//	LxBox/1.1.4 (desktop; linux)
//
// Product brand token is "LxBox" with a "desktop" variant tag (distinguishes
// from the Android LxBox build). The UA must NOT contain a bare "singbox" (no
// hyphen): some subscription panels (Remnawave/Marzban-style) route the
// response body by a substring match on the User-Agent, and a bare "singbox"
// is matched as a non-sing-box client — the panel then serves a full
// client-config JSON instead of the base64/URI subscription list the launcher
// can ingest. The regression test in useragent_test.go guards against it.
//
// See SPEC 061-F-N §"Request headers" §1.
func BuildSubscriptionUserAgent() string {
	ver := strings.TrimSpace(constants.AppVersion)
	ver = strings.TrimPrefix(ver, "v")
	if ver == "" {
		ver = "unknown"
	}
	return fmt.Sprintf("LxBox/%s (desktop; %s)", ver, canonicalGOOSName(runtime.GOOS))
}

// MaxNodesPerSubscription limits the maximum number of nodes parsed from a single subscription
// This prevents memory issues with very large subscriptions
const MaxNodesPerSubscription = 3000

// ParserConfig represents the configuration structure from @ParserConfig block
// Clean structure for version 4 (legacy versions are migrated automatically)
type ParserConfig struct {
	ParserConfig struct {
		Version   int           `json:"version,omitempty"`
		Proxies   []ProxySource `json:"proxies"`
		Outbounds []Direction   `json:"outbounds"`
		Parser    struct {
			Reload      string `json:"reload,omitempty"`       // Интервал автоматического обновления
			LastUpdated string `json:"last_updated,omitempty"` // Время последнего обновления (RFC3339, UTC)
		} `json:"parser,omitempty"`
	} `json:"ParserConfig"`
}

// ProxySource represents a proxy subscription source
type ProxySource struct {
	// ID — ULID источника (state.Source.ID), провезённый в сборочную форму.
	//
	// SPEC 112-A: ссылка на узел адресуется парой «source_id + identity-тег»,
	// поэтому генератору нужно соответствие «источник в ParserConfig → ULID».
	// Поле деривное: канонический владелец — Connections.Sources; сюда его
	// кладут ToProxySourceV4 / syncLegacyFromConnections (прямая проекция;
	// обратного синка нет — SPEC 117).
	//
	// Пусто у конфигов, собранных не из состояния (тесты, ручной JSON) —
	// резолв ссылок обязан это переживать (глобальный поиск по тегу).
	ID string `json:"id,omitempty"`
	// Label — подпись источника (state.Source.Label), провезённая в сборочную
	// форму ТОЛЬКО ради текстов диагностики.
	//
	// SPEC 112-A требует, чтобы сообщение о непойманной ссылке называло обе
	// стороны человеческими именами: «в подписке "AL: Liberty" не нашлось узла
	// …». До этого у сборки на руках был лишь URL подписки, и предупреждение
	// приходилось читать по адресу — а подпись пользователь как раз и правит,
	// чтобы источник узнавать. Ни на что, кроме сообщений, поле не влияет:
	// именем узла остаётся тег (SPEC 112).
	Label string `json:"label,omitempty"`
	// ProviderAnnounce — сообщение провайдера (заголовок `announce`) из
	// метаданных последнего фетча, провезённое в сборочную форму ТОЛЬКО ради
	// текстов диагностики — ровно как Label.
	//
	// SPEC 115: когда подписка отдаёт ноль узлов, лучший диагноз обычно уже
	// написан самим провайдером («⚠️ Произошла ошибка при получении подписки»);
	// наши синтезированные причины идут после него. Без этого поля разбор о
	// сообщении не знает: метаданные живут в state.Source.Meta, а разбору
	// достаётся только сборочная форма.
	//
	// ГРАНИЦА ДОВЕРИЯ: чужой текст. Он показывается как ДАННЫЕ, не
	// интерпретируется и ни на что в сборке не влияет; длина ограничена при
	// заполнении (state.ProviderAnnounce.AnnounceMessage).
	ProviderAnnounce string              `json:"-"`
	Source           string              `json:"source,omitempty"`
	Connections      []string            `json:"connections,omitempty"`
	Skip             []map[string]string `json:"skip,omitempty"`
	Outbounds        []Direction         `json:"outbounds,omitempty"`   // Local outbounds for this source (version 4)
	TagPrefix        string              `json:"tag_prefix,omitempty"`  // Prefix to add to all node tags from this source
	TagPostfix       string              `json:"tag_postfix,omitempty"` // Postfix to add to all node tags from this source
	TagMask          string              `json:"tag_mask,omitempty"`    // Mask to replace entire tag (ignores tag_prefix and tag_postfix if set)
	// ExcludeFromGlobal: when true, nodes from this source are omitted from the pool for global ParserConfig.outbounds (generation-time only).
	//
	// SPEC 108: свёрнутая подписка (Fold != nil) выставляет этот флаг сама
	// на проходе 0 — узлы уходят в свою группу и в общем списке им делать
	// нечего. Как самостоятельное поле он остаётся ради состояний, где
	// `exclude_from_global` стоял БЕЗ локальных групп: свёрткой это не
	// выражается (галка всегда создаёт группу), а ломать такой конфиг
	// незачем. UI его больше не выставляет.
	ExcludeFromGlobal bool `json:"exclude_from_global,omitempty"`
	// ExposeGroupTagsToGlobal: when true, tags of wizard-marked local outbounds are merged into each global outbound at generation time (SPEC 026).
	//
	// SPEC 108: тоже выставляется свёрткой на проходе 0. Прежний флаг
	// читается при загрузке состояния и разворачивается в Fold; UI его не
	// показывает.
	ExposeGroupTagsToGlobal bool `json:"expose_group_tags_to_global,omitempty"`
	// Chain: SPEC 110 — источник является цепочкой хопов. nil = обычный
	// источник (подписка или сервер).
	//
	// Материализуется в ОДИН узел с EmitRaw (как ручной config_json), а не
	// в группу: цепочка это маршрут, и остальной лаунчер видит её узлом —
	// её ловят фильтры Направлений, она переключается в Clash API.
	Chain *SourceChain `json:"chain,omitempty"`
	// Fold: SPEC 108 — свёртка подписки в одну группу вместо её узлов.
	// nil = не свёрнута. Локальные группы из неё разворачиваются на сборке
	// (config.PrepareSourceFolds), в состоянии не материализуются.
	Fold *SourceFold `json:"fold,omitempty"`
	// Disabled: quick on/off toggle exposed in the wizard Sources list.
	// When true, the parser pipeline skips this source entirely (no fetch,
	// no parse, no nodes generated). The source stays in the file so the
	// user can re-enable it without re-entering its URL / skip rules / etc.
	// Omit-when-default so legacy ParserConfig files (no field) are treated
	// as enabled, matching prior behavior.
	Disabled bool `json:"disabled,omitempty"`
	// DetourTag: SPEC 077 — tag of another outbound that this source's nodes
	// dial through (proxy chain). Empty = direct. Promoted to node.Outbound
	// ["detour"] for each non-WireGuard node at parse time; validated (dangling/
	// cycle/self → dropped) at generation time.
	DetourTag string `json:"detour_tag,omitempty"`
	// DetourNodeSourceID / DetourNodeTag — SPEC 112-A: ссылка на ОДИН узел,
	// через который дозваниваются узлы этого источника (альтернатива DetourTag,
	// который метит в группу). Ссылка — ОБЪЕКТ из двух частей:
	//
	//	DetourNodeSourceID — ULID источника-цели (ProxySource.ID);
	//	DetourNodeTag      — identity-тег узла ВНУТРИ этого источника
	//	                     (ParsedNode.IdentityTag, SPEC 112).
	//
	// Финальный конфиговый тег здесь НЕ хранится: он вычисляется на каждой
	// сборке (prefix/mask/uniquify), и хранимый тег протухал бы от смены
	// tag_prefix источника-цели — ровно тем же классом багов, что до SPEC 112
	// протухал контент-хеш.
	//
	// Резолв СТРОГИЙ (дополнение к SPEC 112-A): ссылка жива, только если
	// source_id найден И узел с таким identity-тегом в нём есть. Расхождение =
	// fail-closed. Честность за пределами сборки обеспечивает UI: операция,
	// меняющая идентичность узла (переименование node_tag), сама сбрасывает
	// ссылки на него и сообщает об этом пользователю.
	//
	// Пустой DetourNodeSourceID при непустом DetourNodeTag — переходная форма
	// (состояния dev-сборок между SPEC 112 и 112-A): тег трактуется как ФИНАЛЬНЫЙ
	// и ищется глобально, как раньше.
	//
	// Mutually exclusive with DetourTag (UI enforces; if both are set,
	// DetourNodeTag wins).
	DetourNodeSourceID string `json:"detour_node_source_id,omitempty"`
	DetourNodeTag      string `json:"detour_node_tag,omitempty"`
	// DetourNodeHash: УПРАЗДНЁННАЯ ссылка по контент-хешу (SPEC 101).
	//
	// Читается только ради миграции состояний, записанных до SPEC 112:
	// resolveNodeDetours опознаёт по нему узел (config.LegacyNodeIdentityHash)
	// и переписывает ссылку в пару DetourNodeSourceID+DetourNodeTag; не опознал —
	// берёт DetourNodeLabel как тег (label там и есть тег выбранного хопа), но
	// уже без source_id: узел-цель неизвестен. После миграции поле не пишется.
	DetourNodeHash string `json:"detour_node_hash,omitempty"`
	// DetourNodeLabel: display-only snapshot of the picked node's tag at pick
	// time, so the Source dialog can show a human label even when the node is
	// temporarily absent. Not used for resolution — except as the fallback of
	// the SPEC 112 migration described above.
	DetourNodeLabel string `json:"detour_node_label,omitempty"`
	// DisabledNodes: SPEC 094 D4 — per-node off switch, keyed by the node's
	// IDENTITY and valued with the unix time the mark was last confirmed.
	//
	// Identity is the node's raw provider tag, uniquified within the source and
	// taken before this source's tag_prefix / tag_mask (SPEC 112,
	// config.NodeIdentity). The tag is the name the provider manages the node
	// by: it survives the provider rotating the server behind that name, and
	// changing the source's tag policy does not move the marks.
	//
	// Was an emitted-JSON hash until SPEC 112; keys of that shape (64 lowercase
	// hex) are migrated to tag keys on the first parse of the source and are
	// never written again.
	//
	// The timestamp exists for garbage collection: a node that has been gone
	// from the subscription longer than the TTL drops out of the map, otherwise
	// it would grow without bound over a subscription's lifetime.
	DisabledNodes map[string]int64 `json:"disabled_nodes,omitempty"`
	// ConfigJSON: ручной sing-box outbound/endpoint объект (server-source).
	// Если задан, LoadNodesFromSource строит ноду из него (Connections не
	// парсятся), а генератор эмитит map passthrough — включая типы и поля,
	// которых лаунчер не знает. Tag и detour перештамповываются как обычно.
	ConfigJSON json.RawMessage `json:"config_json,omitempty"`

	// Canonical — ГОТОВЫЕ узлы канона v7 (state.Source.nodes[] либо body
	// корневого узла), спроецированные для сборки (SPEC 118 W4).
	//
	// Непусто → конвейер сборки НЕ зовёт парсер тела: узлы уже
	// материализованы (fetch W3 / миграция W2), эмиссия идёт из body, а
	// тег-политика применяется на эмиссии. Пусто → мостовой путь W1–W3
	// (парс raw-кэша) — он живёт до W5 ради состояний, ещё не прошедших
	// материализацию.
	//
	// `json:"-"`: проекция сборки, на диск не едет.
	Canonical *CanonicalSource `json:"-"`
}

// CanonicalSource — проекция канонического источника v7 в сборочную форму
// (SPEC 118 W4, PLAN §4).
//
// Держит ровно то, что нужно эмиссии: готовые узлы, тег-политику папки и
// свёртку. Ссылки NodeLink уже здесь — их резолвит единый резолв
// (nodelink_resolve.go), а не частные поиски по тегам.
type CanonicalSource struct {
	// FolderID — ULID папки/подписки (адресат NodeLink.FolderID). Пусто у
	// корневого узла: его пространство тегов — корневое.
	FolderID string
	// IsContainer — источник является папкой/подпиской (у корневого узла
	// тег-политики нет, финальный тег = сырой).
	IsContainer bool
	// TagPrefix / TagPostfix — тег-политика контейнера (переменные живут).
	TagPrefix  string
	TagPostfix string
	// Nodes — узлы в порядке модели.
	Nodes []CanonicalNode
	// FolderDetour — общий detour папки: применяется к Server-узлам БЕЗ
	// личного, пропуская Chain и Auto (features/directions.md §7).
	FolderDetour *NodeLink
	// Replace — свёртка папки; nil = папка развёрнута поузлово.
	Replace *FolderReplace
}

// CanonicalNode — один готовый узел канона в сборочной форме.
type CanonicalNode struct {
	// Kind: "server" | "chain" | "auto" (значения state.SourceKind).
	Kind string
	// Tag — СЫРОЙ тег: идентичность в контейнере, вход тег-политики.
	Tag     string
	Enabled bool
	// Body — готовый sing-box outbound, чист от tag/detour (server only).
	Body json.RawMessage
	// OriginRaw / OriginKind — происхождение записи (диагностика, {$label}).
	OriginRaw  string
	OriginKind string
	// Detour — личный detour узла (server only).
	Detour *NodeLink
	// Hops — позиции цепочки, ближний первым (chain only).
	Hops []NodeLink
	// Group — провайдерская группа (auto only).
	Group *CanonicalAutoGroup
}

// CanonicalAutoGroup — провайдерская группа канона в сборочной форме.
type CanonicalAutoGroup struct {
	// GroupType: "selector" | "urltest".
	GroupType string
	// Default — сырой тег члена (selector only).
	Default string
	Members []NodeLink
	// Options — опции группы (url/interval/tolerance/…), уже раскрытые из
	// AutoStrategy в форму sing-box.
	Options map[string]interface{}
}

// NodeLink — ссылка «через кого» в сборочной форме (зеркало state.NodeLink;
// state сюда импортировать нельзя — цикл).
type NodeLink struct {
	// FolderID: "" → корневое пространство ФИНАЛЬНЫХ тегов.
	FolderID string
	// Tag — сырой тег узла папки | финальный тег корня.
	Tag string
}

// FolderReplace — свёртка папки в сборочной форме (зеркало
// state.FolderReplace).
type FolderReplace struct {
	// Mode: "manual" | "auto" | "both".
	Mode string
	// Tag — явный тег замены; both → двойник "<Tag>-auto".
	Tag string
	// Strategy — параметры авто-половины; nil при manual.
	Strategy *DirectionAuto
}

// Режимы FolderReplace (зеркало state.FolderReplace*).
const (
	FolderReplaceManual = "manual"
	FolderReplaceAuto   = "auto"
	FolderReplaceBoth   = "both"
)

// Sentinel ref values for Direction (SPEC 058-R-N STATE_AS_TEMPLATE_DIFF).
//
// Outbound entries в state.connections.outbounds[] делятся на два класса:
//   - **Direct (прямые):** self-contained body. `Ref` пустой (поле отсутствует в JSON).
//   - **Referenced (ссылочные):** body живёт в template или preset, в state только tag + ref.
//
// Для ссылочных entries `Ref` принимает одно из двух значений:
//   - `RefTemplate` — body из `template.parser_config.outbounds[tag]`.
//   - `<preset_id>` — body из `template.presets[id].outbounds` (mode=add).
//
// Update-level (`OutboundUpdate.Ref`) — `<preset_id>` для preset patch'ей либо
// `RefUser` для пользовательского field-level diff поверх referenced body.
//
// Validation: preset.id regex `^[a-z0-9_-]+$` не пересекается с этими константами
// (UPPERCASE + `#`) by construction — collision невозможна.
const (
	RefTemplate = "#TEMPLATE#" // только в state.outbounds[].ref — referenced template entry
	RefUser     = "#USER#"     // только в state.outbounds[].updates[].ref — user patch
)

// Direction represents an outbound selector configuration.
//
// **Origin class (SPEC 058-R-N):**
//   - `Ref == ""` (поле отсутствует) — direct entry, body inline в state. Full ownership.
//   - `Ref == RefTemplate` — referenced template entry. Body live из
//     `template.parser_config.outbounds[tag]`; body-поля в state НЕ хранятся
//     (omitempty). USER edit становится field-level diff в `Updates[]` с ref=RefUser.
//   - `Ref == "<preset_id>"` — referenced preset add entry. Body live из
//     `template.presets[id].outbounds` (mode=add). USER edit аналогично через USER patch.
//
// **Updates stack (SPEC 057-R-N):** стек patches от `preset.outbounds[mode=update]`
// + опциональный USER patch. Merged body для emit вычисляется через ResolveOutbound /
// MergeOutboundUpdatesInPlace (base + apply updates в order; USER patch всегда последний).
//
// **Required:** template-only flag — указывает что outbound обязателен и
// не должен быть полностью удалён (UI блокирует Del, но Edit + Reset OK).
// В state.json приходит из миграции wizard.required (legacy) → required.
// В template — `required: true` на уровне outbound.
type Direction struct {
	Tag              string                 `json:"tag"`
	Type             string                 `json:"type,omitempty"`
	Options          map[string]interface{} `json:"options,omitempty"`
	Filters          map[string]interface{} `json:"filters,omitempty"`
	AddOutbounds     []string               `json:"addOutbounds,omitempty"`
	PreferredDefault map[string]interface{} `json:"preferredDefault,omitempty"`
	Comment          string                 `json:"comment,omitempty"`
	Required         bool                   `json:"required,omitempty"` // template-only marker (см. RequiredOutboundTags)

	// SPEC 104: поля Направления.
	//
	// Отображаемого имени у Направления НЕТ — имя ровно одно, Tag
	// (контракт 0.9.0). Прежнее поле Label снято намеренно: на тег
	// ссылаются правила, и второе имя означало, что в списке видно одно,
	// а в выпадашке целей — другое. Переименование = смена Tag вместе со
	// ссылками. Не возвращать; у узлов и пресетов Label законен — там
	// ссылочного тега в паре нет.

	// Disabled — направление не материализуется и не предлагается целью
	// правил. Именно Disabled, а не Enabled: нулевое значение bool должно
	// означать «включено», иначе запись без явного ключа читалась бы
	// выключенной (на этом уже спотыкалась первая реализация каналов,
	// которой пришлось заводить собственный UnmarshalJSON).
	Disabled bool `json:"disabled,omitempty"`

	// Auto — параметры парного `<tag>-auto` (urltest по узлам направления).
	// nil = двойника нет. Сам двойник в состоянии НЕ хранится: он
	// разворачивается на сборке, чтобы имя и состав не разъезжались с
	// родителем.
	Auto *DirectionAuto `json:"auto,omitempty"`

	// TwinOf / TwinTag — служебная связь Направления и его парной
	// urltest-группы (SPEC 104). Оба поля живут ТОЛЬКО во время сборки
	// (`json:"-"`): двойник разворачивается из Auto на каждом билде, и
	// хранить его в состоянии значило бы завести вторую сущность, которую
	// пришлось бы вручную держать синхронной с родителем.
	//
	// TwinOf непуст у самой auto-группы и указывает на родителя; TwinTag
	// непуст у родителя и указывает на группу.
	TwinOf  string `json:"-"`
	TwinTag string `json:"-"`

	// NoGroupMembers — запись НЕ принимает в состав групповые узлы
	// (selector/urltest из импортированного конфига).
	//
	// Живёт только во время сборки (`json:"-"`), как TwinOf/TwinTag. Причина
	// та же, что у твинов Направлений: авто-измеритель поверх чужой группы
	// мерил бы её текущий выбор, а не скорость сервера. Отдельный флаг, а не
	// переиспользование TwinOf: TwinOf меняет обработку записи в проходах
	// 1–3 целиком (исключение из пула, отказ от expose-кредита), а
	// авто-половина свёртки папки — самостоятельная локальная группа
	// (SPEC 118 W4, features/directions.md §5).
	NoGroupMembers bool `json:"-"`

	// SPEC 057/058-R-N: preset/template binding.
	Ref     string           `json:"ref,omitempty"`     // "" (direct) | "#TEMPLATE#" | "<preset_id>"
	Updates []OutboundUpdate `json:"updates,omitempty"` // стек patches: preset patches в rule order + опц. USER patch (всегда последний)
}

// Режимы автовыбора в `DirectionAuto.Mode` (SPEC 088 + LxBox §208).
//
// Пусто эквивалентно AutoModeLeastTest: апстримный urltest, который пишется
// в конфиг бит-в-бит без лишних ключей.
const (
	AutoModeLeastTest  = "least_test"
	AutoModeRoundRobin = "round_robin"
)

// DirectionAuto — параметры парной urltest-группы направления.
//
// Пустые поля означают «берём из шаблона» (`group_templates.auto.options`),
// а не «ноль»: подстановка @urltest_* — задача движка шаблонов, и дублировать
// её значениями по умолчанию здесь значило бы завести вторую реализацию.
type DirectionAuto struct {
	Mode        string `json:"mode,omitempty"` // "" | least_test | round_robin
	URL         string `json:"url,omitempty"`
	Interval    string `json:"interval,omitempty"`
	IdleTimeout string `json:"idle_timeout,omitempty"`

	// Tolerance — число миллисекунд ЛИБО ссылка на переменную шаблона
	// ("@urltest_tolerance"): подстановка идёт после разворачивания
	// двойника, и до неё здесь лежит строка. Поэтому json.RawMessage, а не
	// int — иначе шаблон с `auto` просто не читался бы.
	// Указатель по той же причине, что PoolTolerance ниже.
	Tolerance *TemplateInt `json:"tolerance,omitempty"`

	// InterruptExistConnections — рвать ли живые соединения при смене лидера.
	// Указатель, потому что здесь важна ТРЁХЗНАЧНОСТЬ: nil = «шаблон решает»,
	// false = «пользователь выключил явно». Обычный bool не отличил бы
	// осознанное выключение от отсутствия настройки и всегда перебивал бы
	// шаблон.
	InterruptExistConnections *bool `json:"interrupt_exist_connections,omitempty"`

	// Поля пула (только Mode == round_robin, SPEC 088).
	Pool int `json:"pool,omitempty"`
	// Указатель, а не значение: `omitempty` на СТРУКТУРЕ Go не действует —
	// опускаются только нулевые скаляры, — и пустой TemplateInt уезжал в
	// патч как `"pool_tolerance": null`, читаясь как правка, которой
	// пользователь не делал. У указателя omitempty работает.
	PoolTolerance *TemplateInt `json:"pool_tolerance,omitempty"`
	StickyHash    []string     `json:"sticky_hash,omitempty"`
}

// Clone — глубокая копия (SPEC 118, хвост ревью W1): указатели
// Tolerance/PoolTolerance/InterruptExistConnections и слайс StickyHash не
// разделяются с оригиналом — рабочие буферы форм и материализация миграции
// обязаны владеть своими экземплярами. Без slices./maps. (go1.20-гард).
func (a *DirectionAuto) Clone() *DirectionAuto {
	if a == nil {
		return nil
	}
	c := *a
	c.Tolerance = a.Tolerance.Clone()
	c.PoolTolerance = a.PoolTolerance.Clone()
	if a.InterruptExistConnections != nil {
		b := *a.InterruptExistConnections
		c.InterruptExistConnections = &b
	}
	c.StickyHash = append([]string(nil), a.StickyHash...)
	return &c
}

// TemplateInt — целое число ЛИБО ссылка на переменную шаблона ("@name").
//
// Нужен там, где значение приходит из шаблона до подстановки переменных:
// в JSON это либо 100, либо "@urltest_tolerance", и обычный int спотыкается
// о вторую форму. Пустое значение означает «не задано».
type TemplateInt struct {
	raw json.RawMessage
}

// Clone — независимая копия (SPEC 118, хвост ревью W1): raw — слайс, и
// копия структуры по значению разделяла бы backing-массив с оригиналом;
// deep-copy рабочих буферов форм обязана владеть своими байтами.
func (t *TemplateInt) Clone() *TemplateInt {
	if t == nil {
		return nil
	}
	return &TemplateInt{raw: append(json.RawMessage(nil), t.raw...)}
}

// NewTemplateInt — значение из числа (форма редактора).
func NewTemplateInt(v int) *TemplateInt {
	if v == 0 {
		return nil
	}
	return &TemplateInt{raw: json.RawMessage(strconv.Itoa(v))}
}

// NewTemplateVar — значение-ссылка на переменную ("@urltest_tolerance").
func NewTemplateVar(name string) *TemplateInt {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	// Ссылка на переменную обязана нести «@»: без неё подстановка видит
	// обычную строку и оставляет её в конфиге как есть, а ядро бракует
	// «urltest_tolerance» на месте числа. Принимаем имя в любом виде —
	// вызывающие передают и «@urltest_tolerance», и «urltest_tolerance».
	// Голая «@» без имени — ссылка в никуда: подставлять нечего, а в конфиг
	// уехало бы «@». Пустое значение честнее: поле просто опустится.
	if strings.TrimPrefix(name, "@") == "" {
		return nil
	}
	if !strings.HasPrefix(name, "@") {
		name = "@" + name
	}
	quoted, err := json.Marshal(name)
	if err != nil {
		return nil
	}
	return &TemplateInt{raw: quoted}
}

// IsZero — значение не задано (поле опускается при сериализации).
func (t *TemplateInt) IsZero() bool { return t == nil || len(t.raw) == 0 }

// Int возвращает число; ok == false для ссылки на переменную или пустого
// значения. Вызывающий обязан различать: подставлять переменную здесь
// значило бы завести вторую реализацию движка шаблонов.
func (t *TemplateInt) Int() (int, bool) {
	if t == nil || len(t.raw) == 0 {
		return 0, false
	}
	var n int
	if err := json.Unmarshal(t.raw, &n); err != nil {
		return 0, false
	}
	return n, true
}

// Value возвращает значение как оно уедет в конфиг: число или строка-ссылка.
func (t *TemplateInt) Value() interface{} {
	if t == nil || len(t.raw) == 0 {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(t.raw, &v); err != nil {
		return nil
	}
	return v
}

func (t *TemplateInt) MarshalJSON() ([]byte, error) {
	if t == nil || len(t.raw) == 0 {
		return []byte("null"), nil
	}
	return t.raw, nil
}

func (t *TemplateInt) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		t.raw = nil
		return nil
	}
	t.raw = append(json.RawMessage(nil), data...)
	return nil
}

// AutoTag — тег парной urltest-группы направления.
//
// Формула зеркалит `group_templates.magic_nodes.auto.tpl`
// (`{parent_tag}-auto`) и LxBox `Channel.autoTag`; менять её нельзя — на
// совпадении держится поиск двойников в фильтрах и сортировках.
func (d Direction) AutoTag() string {
	if d.Tag == "" {
		return ""
	}
	return d.Tag + "-auto"
}

// DisplayName — имя для интерфейса. У Направления оно одно — Tag
// (контракт 0.9.0); метод оставлен точкой смысла «как показать
// Направление», чтобы вызовы не разъезжались по прямым d.Tag.
func (d Direction) DisplayName() string {
	return d.Tag
}

// IsEnabled — участвует ли направление в сборке и предлагается ли целью
// правил.
func (d Direction) IsEnabled() bool { return !d.Disabled }

// OutboundUpdate — одна запись в стеке `Direction.Updates` (SPEC 057/058-R-N).
//
// `Ref` принимает:
//   - `<preset_id>` — patch от активного preset'а (mode=update). Stale → drop через sync.
//   - `RefUser` — пользовательский field-level diff от merged_base. Один на outbound,
//     replace при каждом Save, всегда последний в order.
//
// Merged body вычисляется через ResolveOutbound:
// `merged = base; for each u in Updates: merged = applyOutboundUpdatePatch(merged, u.Patch)`.
type OutboundUpdate struct {
	Ref   string                 `json:"ref"`   // <preset_id> | RefUser
	Patch map[string]interface{} `json:"patch"` // patch fields (filters, options, addOutbounds, ...)

	// Explicit — патч записан формой ОСОЗНАННО: пользователь стёр поле, и
	// пустое значение здесь означает «убрать», а не «правки нет».
	//
	// Нужен, чтобы отличить очистку от артефакта. До SPEC 058 форма
	// отдавала пустыми поля, которых пользователь не трогал, и такой патч
	// затирал пресетные значения (russian → `!/(🇷🇺)/i` на proxy-out
	// выпускал российский трафик через российский узел). Оба рубежа защиты
	// судили по признаку «значение пустое» — и заодно съедали ЗАКОННУЮ
	// очистку: убранное умолчание не сохранялось, форма показывала
	// шаблонное значение снова.
	//
	// Различить по содержимому патча нельзя — оно у обоих случаев
	// одинаковое. Различает происхождение: этот флаг ставит только сегодняшняя
	// форма, а legacy-патчи его не имеют и чистятся как прежде.
	//
	// omitempty: патчи без очистки поля пишутся как раньше, байт-в-байт.
	Explicit bool `json:"explicit,omitempty"`
}

// IsReferenced возвращает true если entry — referenced (#TEMPLATE# или preset_id),
// false для direct (пустой Ref). Body для referenced live из template/preset.
func (oc *Direction) IsReferenced() bool {
	return oc.Ref != ""
}

// IsTemplateRef возвращает true если entry ссылается на template global outbound.
func (oc *Direction) IsTemplateRef() bool {
	return oc.Ref == RefTemplate
}

// IsPresetRef возвращает true если entry ссылается на preset add outbound.
func (oc *Direction) IsPresetRef() bool {
	return oc.Ref != "" && oc.Ref != RefTemplate
}

// UnsetSourceIndex means SourceIndex was not assigned; exclude_from_global must not apply.
const UnsetSourceIndex = -1

// SchemeGroup marks a ParsedNode that is an outbound group (selector/urltest)
// imported from a sing-box config (SPEC 094 A5).
//
// Such a node is an ORDINARY entry in the subscription's node list with no
// privileges: it does not appear in the wizard's Outbounds tab, routing rules
// never reference it, and the user does not configure its membership. That tab
// stays reserved for the launcher's own channels, which do drive routing.
//
// For sing-box it emits as a real selector/urltest inside the outbounds array;
// for the launcher it is just another node the user can pick.
const SchemeGroup = "group"

// GroupMembersKey is the ParsedNode.Outbound field holding the group's member
// tags. Mirrors sing-box's own "outbounds" field on selector/urltest.
const GroupMembersKey = "outbounds"

// ParsedJump is an optional first hop for Xray dialerProxy → sing-box detour (SOCKS, VLESS, …).
// Scheme empty means "socks" (backward compatibility). UUID/Flow are set for vless/vmess hops when GenerateNodeJSON needs them.
type ParsedJump struct {
	Tag      string
	Scheme   string // socks, vless, …
	Server   string
	Port     int
	UUID     string
	Flow     string
	Outbound map[string]interface{}
}

// ParsedNode represents a parsed proxy node with all extracted information.
// It contains protocol-specific fields (UUID, Flow, etc.) and the generated
// outbound configuration ready for JSON serialization.
type ParsedNode struct {
	Tag      string
	Scheme   string
	Server   string
	Port     int
	UUID     string
	Flow     string
	Label    string
	Comment  string
	Query    url.Values
	Outbound map[string]interface{}
	// Jump is set when the subscription node uses a chain (e.g. Xray dialerProxy → SOCKS before main outbound).
	//
	// Deprecated (SPEC 094 B1): holds only the first hop. Use Chain, which
	// carries the full path. Jump stays in sync with Chain[0] so existing
	// readers and state.json files written before SPEC 094 keep working.
	Jump *ParsedJump
	// SourceTag is the node's tag exactly as it appeared in an imported
	// sing-box config, before prefix/mask/uniquification (SPEC 094 A5).
	//
	// Imported selector/urltest groups list their members by that original
	// tag, so rebinding them to the final tags needs this untouched copy.
	// Empty for nodes that did not come from a sing-box config.
	SourceTag string
	// IdentityTag — идентичность узла (SPEC 112): сырой провайдерский тег,
	// уникализированный В ПРЕДЕЛАХ ИСТОЧНИКА, снятый ДО применения
	// tag_prefix / tag_postfix / tag_mask.
	//
	// Именно она держит отметки выключения (ProxySource.DisabledNodes) и
	// принадлежность узла между элементами Xray-массива. Содержимое узла
	// (server, port, ключи, SNI, транспорт) в идентичность НЕ входит:
	// провайдер вправе поменять сервер под тем же именем — это тот же узел.
	//
	// Пустая строка = «идентичности нет» (узел собран не парсером источника):
	// вызывающий обязан считать это отсутствием, а не общим ключом "".
	IdentityTag string
	// Chain is the ordered detour path from the nearest hop outwards
	// (SPEC 094 B1). Empty means the node dials directly.
	//
	// A node reaches its server through Chain[0] → Chain[1] → … → node, i.e.
	// the emitted outbound carries detour=Chain[0].Tag, and each hop carries
	// detour of the next one. Depth is capped at 8 hops.
	Chain []*ParsedNode
	// SourceIndex is the index into ParserConfig.proxies for this node; UnsetSourceIndex if unknown.
	SourceIndex int
	// EmitRaw marks a node built from a manual config_json (ProxySource.
	// ConfigJSON): the generator serializes Outbound as-is (tag/detour
	// restamped) instead of reassembling fields through the per-scheme
	// emitter — the whole point is carrying types and fields the emitter
	// does not know about.
	EmitRaw bool
	// EmitBody — ГОТОВОЕ тело узла из канона v7 (state.Node.Body): сборка
	// эмитит его как есть, только возвращая на места ключи tag и detour
	// (SPEC 118 W4, Т5 «сборка не читает тел подписок и не зовёт парсеры»).
	//
	// Порядок ключей внутри сохраняется байт-в-байт — это и есть то, что
	// эмиттер лаунчера написал в момент материализации; поэтому эмиссия из
	// nodes[] совпадает со старым движком, парсившим тело на каждой сборке.
	//
	// Приоритетнее EmitRaw и per-scheme-ветки: непусто → узел эмитится
	// отсюда. Outbound при этом ЗАПОЛНЕН (разбор тела в map) — его читают
	// фильтры Направлений, санитайзер и проверки цепочек.
	EmitBody json.RawMessage
	// CanonicalDetour — личный detour узла из канона v7 (NodeLink).
	// Резолвится единым резолвом на проходе 2; в body не запекается.
	CanonicalDetour *NodeLink
	// CanonicalGroupMembers / CanonicalGroupDefault — состав провайдерской
	// Auto-группы канона по ссылкам NodeLink (сырые теги своей папки).
	// Резолв на проходе 2 переписывает их в финальные теги членов.
	CanonicalGroupMembers []NodeLink
	CanonicalGroupDefault string
	// Warnings — коды деградаций, применённых к узлу при разборе
	// (SPEC 103, фаза 2). Словарь кодов — contract/registry/warnings.json.
	//
	// До этого деградация уходила только в debuglog: пользователь видел
	// «нода есть», но не знал, что у неё срезали обфускацию или заменили
	// отпечаток. Коды позволяют показать это в UI и сверять поведение
	// обоих приложений по общему корпусу, а не по тексту лога.
	//
	// Порядок не нормируется, дубли не хранятся: код отвечает на вопрос
	// «что случилось», а не «сколько раз».
	Warnings []string
}

// AddWarning помечает узел кодом деградации, не создавая дублей.
func (n *ParsedNode) AddWarning(code string) {
	if n == nil || code == "" {
		return
	}
	for _, existing := range n.Warnings {
		if existing == code {
			return
		}
	}
	n.Warnings = append(n.Warnings, code)
}

// SyncJumpFromChain refreshes the deprecated Jump field from Chain[0].
//
// SPEC 094 B1: Chain is the source of truth; Jump is kept alive so code paths
// and persisted state predating the chain model keep reading a valid first hop.
func (n *ParsedNode) SyncJumpFromChain() {
	if n == nil {
		return
	}
	if len(n.Chain) == 0 {
		n.Jump = nil
		return
	}
	hop := n.Chain[0]
	if hop == nil {
		n.Jump = nil
		return
	}
	n.Jump = &ParsedJump{
		Tag:      hop.Tag,
		Scheme:   hop.Scheme,
		Server:   hop.Server,
		Port:     hop.Port,
		UUID:     hop.UUID,
		Flow:     hop.Flow,
		Outbound: hop.Outbound,
	}
}

// AdoptLegacyJump rebuilds Chain from a legacy single-hop Jump.
//
// SPEC 094 B1 migration: state.json written before the chain model carries only
// Jump. Reading it must not lose the hop, so an empty Chain plus a non-nil Jump
// is promoted to a one-element Chain. No-op when Chain is already populated.
func (n *ParsedNode) AdoptLegacyJump() {
	if n == nil || len(n.Chain) > 0 || n.Jump == nil {
		return
	}
	n.Chain = []*ParsedNode{{
		Tag:         n.Jump.Tag,
		Scheme:      n.Jump.Scheme,
		Server:      n.Jump.Server,
		Port:        n.Jump.Port,
		UUID:        n.Jump.UUID,
		Flow:        n.Jump.Flow,
		Outbound:    n.Jump.Outbound,
		SourceIndex: UnsetSourceIndex,
	}}
}

// NormalizeParserConfig normalizes ParserConfig structure:
// - Ensures version is set to ParserConfigVersion
// - Sets default reload to "4h" if not specified
// - Optionally updates last_updated timestamp (if updateLastUpdated is true)
func NormalizeParserConfig(parserConfig *ParserConfig, updateLastUpdated bool) {
	if parserConfig == nil {
		return
	}

	parserConfig.ParserConfig.Version = ParserConfigVersion

	if parserConfig.ParserConfig.Parser.Reload == "" {
		parserConfig.ParserConfig.Parser.Reload = "4h"
	}

	if updateLastUpdated {
		parserConfig.ParserConfig.Parser.LastUpdated = time.Now().UTC().Format(time.RFC3339)
	}
}

// SourceChain — параметры источника-цепочки (SPEC 110).
//
// Цепочка — это МАРШРУТ, а не точка выбора между маршрутами, поэтому она
// живёт источником рядом с подпиской и сервером, а не Направлением. Для
// остального лаунчера она узел: попадает в пул, отбирается фильтрами
// Направлений, переключается в Clash API.
//
// Соответствует `option.ChainOutboundOptions` форка ядра
// (`option/chain_lx.go`). Поля названы как в ядре везде, кроме Hops: рядом
// в состоянии лежат составы групп, и второе поле со словом «outbounds» и
// другим смыслом читалось бы как то же самое. В конфиг Hops эмитится под
// ключом `outbounds` — форма ядра неизменна.
type SourceChain struct {
	// Hops — позиции В ПОРЯДКЕ ПАКЕТА: [0] — первый хоп от клиента,
	// последний — тот, чей адрес видит цель. НЕ «кто через кого»: в detour
	// стрелка смотрит в обратную сторону, и перепутать их — значит собрать
	// работающий, но не тот маршрут (SPEC 110 T3).
	//
	// Ядро требует минимум двух позиций, непустых, без самоссылки и без
	// дублей (`protocol/chain/chain.go:85-100`) — нарушение любого условия
	// не даёт стартовать всему конфигу, а не одной цепочке.
	Hops []string `json:"hops,omitempty"`

	// IdleTimeout — простой, после которого звено без соединений удаляется.
	// Пусто = умолчание ядра (5m), "0s" = жить до остановки.
	IdleTimeout string `json:"idle_timeout,omitempty"`

	// StripEvasion — снимать ли у звеньев односторонние DPI-приёмы.
	// Указатель ради трёхзначности: nil = «умолчание ядра» (true),
	// false = «пользователь выключил явно». Обычный bool не отличил бы
	// одно от другого — та же причина, что у InterruptExistConnections.
	StripEvasion *bool `json:"strip_evasion,omitempty"`

	// Strip — патч к каталогу ядра поверх StripEvasion.
	//
	// Ключи только из ChainStripKeys: неизвестный ключ ядро считает ошибкой
	// старта, а не опечаткой, которую можно пропустить.
	Strip map[string]bool `json:"strip,omitempty"`

	// Rewrite — JSON merge-patch поверх опций узла, по типу outbound'а.
	// Правится на вкладке JSON: форму для произвольного патча по всем типам
	// протоколов не построить, а урезанная форма молча потеряла бы ключи,
	// которых не знает.
	Rewrite map[string]interface{} `json:"rewrite,omitempty"`
}

// ChainOutboundType — значение поля `type` в конфиге ядра.
const ChainOutboundType = "chain"

// Ключи каталога `strip` ядра (`protocol/chain/transform.go:24-27`).
//
// Список закрыт: ядро отвергает конфиг на неизвестном ключе, поэтому
// «на всякий случай» сюда добавлять нечего — новый ключ появляется только
// вместе с новой версией ядра.
const (
	ChainStripTLSFragment      = "tls.fragment"
	ChainStripMultiplexPadding = "multiplex.padding"
	ChainStripXHTTPPadding     = "xhttp.padding"
	ChainStripTLSUTLS          = "tls.utls"
)

// ChainStripKeys — каталог в порядке показа в форме. Снимаемые по умолчанию
// идут первыми, tls.utls последним: он единственный не снимается по
// умолчанию и единственный, снятие которого ломает reality-узлы (T4).
var ChainStripKeys = []string{
	ChainStripTLSFragment,
	ChainStripMultiplexPadding,
	ChainStripXHTTPPadding,
	ChainStripTLSUTLS,
}

// ChainStripDefault — снимается ли ключ при включённом strip_evasion.
// Копия каталога ядра; форма показывает по нему исходное состояние галок.
var ChainStripDefault = map[string]bool{
	ChainStripTLSFragment:      true,
	ChainStripMultiplexPadding: true,
	ChainStripXHTTPPadding:     true,
	ChainStripTLSUTLS:          false,
}

// HopsOrNil — позиции цепочки, безопасно для nil-приёмника.
//
// Нужен там, где Direction уже опознан цепочкой по типу, но Chain может
// оказаться пустым: тип и содержимое приходят из разных мест (тип — из
// шаблона или формы, содержимое — из состояния), и рассинхрон обязан
// кончаться нулём позиций, а не паникой на сборке конфига.
func (c *SourceChain) HopsOrNil() []string {
	if c == nil {
		return nil
	}
	return c.Hops
}

// StripEvasionEnabled — умолчание ядра: nil == true.
func (c *SourceChain) StripEvasionEnabled() bool {
	return c == nil || c.StripEvasion == nil || *c.StripEvasion
}
