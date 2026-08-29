// File sources_v7.go — канонические типы дерева источников схемы v7
// (SPEC 118, этап 2; формы — PLAN §1.2, нормативный каркас).
//
// Принят плоский юнион с дискриминатором `kind` (PLAN §1.1в): один Go-struct
// на уровень, поля чужих kind'ов пустые/omitempty — ровно паттерн rules[] и
// dns_options (SPEC 053/056). Нелегальные комбинации полей закрывает
// normalizeSourceShape на Load и конструкторы.
//
// МОСТ (PLAN §6): пока сборка ходит через legacy-проекцию ProxySource,
// Source несёт рядом с каноном легаси-поля v6 (помечены TEMPORARY BRIDGE).
// Загрузка v6-состояния в W1 переносит их структурно, без семантической
// миграции (миграция — волна W2); adapter_source.go деривирует из них
// прежнюю ProxySource-форму, поэтому поведение build-путей не меняется.
package state

import (
	"encoding/json"
	"fmt"

	"singbox-launcher/core/config/configtypes"
)

// SourceKind — дискриминатор элемента sources[] (и узла внутри папки).
type SourceKind string

const (
	SourceKindServer       SourceKind = "server"
	SourceKindChain        SourceKind = "chain"
	SourceKindAuto         SourceKind = "auto"
	SourceKindFolder       SourceKind = "folder"
	SourceKindSubscription SourceKind = "subscription"
)

// TEMPORARY BRIDGE (SPEC 118 W1-W4), удаляется в W5: прежние имена
// дискриминатора. Строковые значения совпадают со старым SourceType
// («subscription»/«server»/«chain»), поэтому alias безопасен и держит
// callsite'ы UI/backup компилируемыми до их канонизации.
type SourceType = SourceKind

const (
	SourceTypeSubscription = SourceKindSubscription
	SourceTypeServer       = SourceKindServer
	SourceTypeChain        = SourceKindChain
)

// NodeLink — единая ссылка «через кого» (features/directions.md §6).
type NodeLink struct {
	// FolderID: "" → корневое пространство финальных тегов.
	FolderID string `json:"folder_id,omitempty"`
	// Tag — сырой тег узла папки (folderId задан) | финальный тег корня.
	Tag string `json:"tag"`
}

// Origin — происхождение узла. nil = создан руками с нуля.
type Origin struct {
	// Kind: "uri" | "wg_ini" | "json". kind=warp НЕ существует (warp-К1).
	Kind string `json:"kind"`
	// Raw — байт в байт; правится только явным Regen и merge-освежением.
	Raw string `json:"raw"`
	// SubURL — URL родной подписки. ВНУТРИ своей подписки пуст (контейнер и
	// есть связь); штампуется при копировании узла наружу (механика — этап 3).
	SubURL string `json:"sub_url,omitempty"`
}

const (
	OriginKindURI   = "uri"
	OriginKindWGIni = "wg_ini"
	OriginKindJSON  = "json"
)

// TagPolicy — косметика эмиссии: финальный тег = prefix + сырой тег + postfix.
// Переменные {$tag} {$scheme} {$protocol} {$server} {$port} {$label}
// {$comment} {$num} в prefix/postfix живут (RECON).
type TagPolicy struct {
	Prefix  string `json:"prefix,omitempty"`
	Postfix string `json:"postfix,omitempty"`

	// Mask — TEMPORARY BRIDGE (SPEC 118 W1-W4), удаляется в W5. В каноне v7
	// маски НЕТ (SPEC Т2); поле держит значение из v6-состояний до миграции
	// W2 (mask server/chain → Node.Tag, mask-шаблон подписки — warning) и
	// кормит legacy-парсер через мост.
	Mask string `json:"mask,omitempty"`
}

// TagSpec — TEMPORARY BRIDGE (SPEC 118 W1-W4), удаляется в W5: прежнее имя
// типа тег-политики (v6 «tag»-спека). Форма совпадает с TagPolicy, alias
// держит callsite'ы UI/backup компилируемыми.
type TagSpec = TagPolicy

// IsZero — нечего применять (все поля пустые).
func (t *TagPolicy) IsZero() bool {
	if t == nil {
		return true
	}
	return t.Prefix == "" && t.Postfix == "" && t.Mask == ""
}

// AutoStrategy = configtypes.DirectionAuto — перенос, не изобретение
// (strategy-К1): 9 полей, включая TemplateInt-tolerance и трёхзначный
// interrupt.
type AutoStrategy = configtypes.DirectionAuto

// AutoGroup — провайдерская группа (kind=auto); groups-К2/К3 закрыты полями.
type AutoGroup struct {
	// GroupType: "selector" | "urltest"; импортированный selector остаётся
	// selector'ом.
	GroupType string `json:"group_type"`
	// Default — selector only; СЫРОЙ тег члена, обязан входить в состав.
	// Живёт здесь, а не в AutoStrategy: default member-зависим (strategy-К2),
	// а AutoStrategy переиспользуется твинами/replace, где default'а нет.
	Default  string       `json:"default,omitempty"`
	Members  []NodeLink   `json:"members"`
	Strategy AutoStrategy `json:"strategy,omitempty"`
}

const (
	AutoGroupSelector = "selector"
	AutoGroupURLTest  = "urltest"
)

// Node — узел: kind ∈ {server, chain, auto}. Живёт в корне sources[]
// (через embedded в Source) и в Folder.Nodes.
type Node struct {
	Kind SourceKind `json:"kind"`
	// Tag — СЫРОЙ тег: идентичность в рамках контейнера, снятый ДО
	// тег-политики. На нём живут merge-ключ, enabled, detour и ссылки
	// NodeLink. У корневых узлов политики нет — финальный тег = сырой.
	Tag     string  `json:"tag"`
	Enabled bool    `json:"enabled"`
	Origin  *Origin `json:"origin,omitempty"`

	// Body — server only: готовый sing-box outbound, чист от detour
	// (никакого запекания Outbound["detour"]).
	Body json.RawMessage `json:"body,omitempty"`
	// Detour — server only; у kind=folder тем же ключом едет ОБЩИЙ detour
	// папки (одна json-точка на оба смысла — семантика по kind, как всё в
	// юнионе). У Chain и Auto detour не существует типом.
	Detour *NodeLink `json:"detour,omitempty"`
	// Hops — chain only: ближний хоп первым.
	Hops []NodeLink `json:"hops,omitempty"`
	// Group — auto only.
	Group *AutoGroup `json:"group,omitempty"`
}

// FolderReplace — свёртка папки: объект, не узел.
type FolderReplace struct {
	// Mode: "manual" | "auto" | "both"; both → селектор + двойник "<tag>-auto".
	Mode string `json:"mode"`
	// Tag — явный тег замены (материализуется миграцией/пользователем).
	Tag string `json:"tag"`
	// Strategy — nil при manual.
	Strategy *AutoStrategy `json:"strategy,omitempty"`
}

const (
	FolderReplaceManual = "manual"
	FolderReplaceAuto   = "auto"
	FolderReplaceBoth   = "both"
)

// Source — элемент sources[]: узел ИЛИ папка/подписка. Node встроен — его
// поля инлайнятся в JSON (у kind=folder заняты kind/enabled/detour).
type Source struct {
	Node

	// === folder | subscription ===

	// ID — ULID; ЕДИНСТВЕННАЯ идентификация папки/подписки (NodeLink.folderId,
	// имена профильных директорий, адресация отчётов).
	//
	// TEMPORARY BRIDGE (SPEC 118 W1-W4), удаляется в W5: до смерти
	// detour-тройни ULID живёт и у узловых kind'ов (server/chain) — он
	// адресат ссылок DetourNodeSourceID (SPEC 112-A); normalizeSourceShape
	// его поэтому НЕ отбрасывает. Канонизация (id только у папки/подписки) —
	// вместе со сносом тройни.
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	TagPolicy *TagPolicy     `json:"tag_policy,omitempty"`
	Nodes     []Node         `json:"nodes,omitempty"`
	Replace   *FolderReplace `json:"replace,omitempty"`

	// === subscription only ===

	URL          string              `json:"url,omitempty"`
	Skip         []map[string]string `json:"skip,omitempty"`
	MaxNodes     int                 `json:"max_nodes,omitempty"`
	Update       *UpdateSpec         `json:"update,omitempty"`
	Meta         *SubMeta            `json:"meta,omitempty"`
	UpdateStatus *SubUpdateStatus    `json:"update_status,omitempty"`
	// PendingDisabled — одноразовые отметки выключения по сырым тегам
	// (вердикт O2, SPEC 118): их БУДЕТ писать импорт бэкапа (волна W7 —
	// сегодня импорт кладёт DisabledNodes-карту, её втягивает merge),
	// когда nodes[] ещё пусты и применять карту не к чему.
	// Применяются на первом ДОСТОВЕРНОМ
	// fetch (MergeSubscriptionNodes) и стираются; при truncated-разборе
	// несматченные теги переживают fetch — узел мог остаться за капом.
	// Это не TTL-карта: поле живёт только между импортом и первым fetch.
	PendingDisabled []string `json:"pending_disabled,omitempty"`

	// === TEMPORARY BRIDGE (SPEC 118 W1-W4), удаляется в W5 ===
	//
	// Легаси-поля v6: доезжают структурным переносом Load v6 → v7 (волна W1,
	// семантическая миграция — W2) и кормят adapter_source.go (проекцию
	// v7 → ProxySource), которой живут build-пути до W4. Save v7 пишет их
	// под прежними ключами, чтобы состояние не теряло данных до W2.

	// Label — отображаемое имя server/chain/subscription (тег — NodeTag/Tag).
	Label string `json:"label,omitempty"`
	// NodeTag — системный тег узла server/chain до миграции в Node.Tag
	// (шаг 3 миграции W2). Пусто → тег = Label (см. NodeTagOrLabel).
	NodeTag string `json:"node_tag,omitempty"`
	// URI — share-URI одиночного сервера (материализация в body — W2).
	URI string `json:"uri,omitempty"`
	// ConfigJSON — ручной sing-box outbound (server); переезжает в Body в W2.
	ConfigJSON json.RawMessage `json:"config_json,omitempty"`
	// Chain — строковые хопы цепочки; []NodeLink Hops — миграция W2 (шаг 4).
	Chain *configtypes.SourceChain `json:"chain,omitempty"`
	// Outbounds — локальные Направления источника (умирают в W5; произвольные
	// — warning миграции, fold-производные → Replace).
	Outbounds []configtypes.Direction `json:"outbounds,omitempty"`
	// ExcludeFromGlobal / ExposeGroupTagsToGlobal — флаги SPEC 108-эпохи.
	ExcludeFromGlobal       bool `json:"exclude_from_global,omitempty"`
	ExposeGroupTagsToGlobal bool `json:"expose_group_tags_to_global,omitempty"`
	// Fold — свёртка подписки; → Replace с материализацией тега (шаг 6, W2).
	Fold *configtypes.SourceFold `json:"fold,omitempty"`
	// Detour-тройня + tag-ссылка SPEC 077/112-A; → NodeLink (шаг 5, W2).
	DetourTag          string `json:"detour_tag,omitempty"`
	DetourNodeSourceID string `json:"detour_node_source_id,omitempty"`
	DetourNodeTag      string `json:"detour_node_tag,omitempty"`
	DetourNodeHash     string `json:"detour_node_hash,omitempty"`
	DetourNodeLabel    string `json:"detour_node_label,omitempty"`
	// DisabledNodes — карта выключенных узлов подписки (SPEC 094 D4);
	// → Node.Enabled=false по identity-ключам (шаг 2, W2).
	DisabledNodes map[string]int64 `json:"disabled_nodes,omitempty"`
}

// NodeTagOrLabel — системный тег узла источника (server / chain).
//
// Порядок: канонический Node.Tag (заполняет миграция W2 либо v7-нативная
// запись) → легаси NodeTag → Label. Откат на Label — не удобство, а
// совместимость: состояния до разделения ролей несут тег именно в Label.
func (s Source) NodeTagOrLabel() string {
	if s.Tag != "" {
		return s.Tag
	}
	if s.NodeTag != "" {
		return s.NodeTag
	}
	return s.Label
}

// SubMeta — метаданные подписки: заголовки провайдера, userinfo, announce.
//
// Канон v7 = SubscriptionMeta МИНУС fetch-история и превью (PLAN §1.2);
// история переезжает в SubUpdateStatus. Поля ниже раздела TEMPORARY BRIDGE
// доживают в этом типе до W3 (перенос записи fetch-сервиса) / W5 (снос),
// чтобы структурный перенос v6 → v7 был без потерь и без правки callsite'ов.
type SubMeta struct {
	// headers (HTTP response + inline #-comments в body первой строкой)
	ProfileTitle               string    `json:"profile_title,omitempty"`
	ProfileUpdateIntervalHours int       `json:"profile_update_interval_hours,omitempty"`
	SupportURL                 string    `json:"support_url,omitempty"`
	ProfileWebPageURL          string    `json:"profile_web_page_url,omitempty"`
	ContentDispositionFilename string    `json:"content_disposition_filename,omitempty"`
	UserInfo                   *UserInfo `json:"userinfo,omitempty"`

	// === TEMPORARY BRIDGE (SPEC 118 W1-W4), удаляется в W5 ===
	// fetch-история: канонический дом — SubUpdateStatus; сюда пишет
	// существующий fetch-сервис до W3.
	URLAtFetch     string `json:"url_at_fetch,omitempty"`
	LastFetchedAt  string `json:"last_fetched_at,omitempty"` // RFC3339 UTC
	LastStatus     string `json:"last_status,omitempty"`     // "ok" | "err"
	ErrorCount     int    `json:"error_count,omitempty"`
	LastErrorMsg   string `json:"last_error_msg,omitempty"`
	HTTPStatusCode int    `json:"http_status_code,omitempty"`
	RawBodyBytes   int64  `json:"raw_body_bytes,omitempty"`

	// TEMPORARY BRIDGE: счёт/превью узлов — канон читает nodes[] (W4/W6).
	NodesCountFetched int      `json:"nodes_count_fetched,omitempty"`
	Truncated         bool     `json:"truncated,omitempty"`
	PreviewNodes      []string `json:"preview_nodes,omitempty"`

	// SPEC 061: provider announce headers (success **or** failure).
	ProviderAnnounce *ProviderAnnounce `json:"provider_announce,omitempty"`

	// LastErrorURL — снимок actionable-URL последней ошибки (см. SPEC 061).
	LastErrorURL string `json:"last_error_url,omitempty"`
}

// SubscriptionMeta — TEMPORARY BRIDGE (SPEC 118 W1-W4), удаляется в W5:
// прежнее имя типа метаданных подписки для callsite'ов UI/fetch-сервиса.
type SubscriptionMeta = SubMeta

// FetchWarning — per-record деградация fetch (skip-счётчики, битые записи,
// потерянные группы-члены). Источник данных отчёта сборки вместо parse-стадии
// (jsontab-К4); наполняется fetch-конвейером волны W3.
type FetchWarning struct {
	// Kind — вид деградации ("skip", "bad_record", "lost_group_member", ...).
	Kind string `json:"kind"`
	// Tag — сырой тег затронутого узла, если применимо.
	Tag string `json:"tag,omitempty"`
	// Message — человекочитаемая причина.
	Message string `json:"message,omitempty"`
	// Count — счётчик для агрегируемых видов (skip).
	Count int `json:"count,omitempty"`
}

// SubUpdateStatus — диагностика fetch подписки (SPEC Т2). UI и отчёт сборки
// читают её из состояния — ничего не перепарсивается. Наполняется
// fetch-конвейером W3; миграция W2 переносит сюда историю из SubMeta.
type SubUpdateStatus struct {
	URLAtFetch        string         `json:"url_at_fetch,omitempty"`
	LastAttemptAt     string         `json:"last_attempt_at,omitempty"` // RFC3339 UTC
	LastSuccessAt     string         `json:"last_success_at,omitempty"` // RFC3339 UTC
	LastStatus        string         `json:"last_status,omitempty"`     // "ok" | "err"
	ErrorCount        int            `json:"error_count,omitempty"`
	LastErrorMsg      string         `json:"last_error_msg,omitempty"`
	LastErrorURL      string         `json:"last_error_url,omitempty"`
	HTTPStatusCode    int            `json:"http_status_code,omitempty"`
	RawBodyBytes      int64          `json:"raw_body_bytes,omitempty"`
	NodesCountFetched int            `json:"nodes_count_fetched,omitempty"`
	Truncated         bool           `json:"truncated,omitempty"`
	Warnings          []FetchWarning `json:"warnings,omitempty"`
}

// ── Конструкторы ─────────────────────────────────────────────────

// NewServerSource — корневой server-узел с готовым body.
func NewServerSource(tag string, body json.RawMessage) Source {
	return Source{Node: Node{Kind: SourceKindServer, Tag: tag, Enabled: true, Body: body}}
}

// NewChainSource — корневая цепочка (ближний хоп первым).
func NewChainSource(tag string, hops []NodeLink) Source {
	return Source{Node: Node{Kind: SourceKindChain, Tag: tag, Enabled: true, Hops: hops}}
}

// NewAutoSource — корневая провайдерская группа.
func NewAutoSource(tag string, group *AutoGroup) Source {
	return Source{Node: Node{Kind: SourceKindAuto, Tag: tag, Enabled: true, Group: group}}
}

// NewFolderSource — пустая папка; ULID минтится сразу (единственная
// идентификация — SPEC Т2).
func NewFolderSource(name string) Source {
	return Source{
		Node: Node{Kind: SourceKindFolder, Enabled: true},
		ID:   MakeULID(),
		Name: name,
	}
}

// NewSubscriptionSource — подписка (папка с внешним владельцем состава).
func NewSubscriptionSource(name, url string) Source {
	return Source{
		Node: Node{Kind: SourceKindSubscription, Enabled: true},
		ID:   MakeULID(),
		Name: name,
		URL:  url,
	}
}

// ── Валидация формы ──────────────────────────────────────────────

// normalizeSourceShape — инварианты формы юниона на Load (PLAN §1.3):
// по kind обнуляет нелегальные КАНОНИЧЕСКИЕ поля (с warning), проверяет
// обязательные. Неизвестный (или пустой) kind — внятный отказ загрузки:
// файл от более нового мажора не должен сюда попасть, но защита обязана быть.
//
// Легаси-поля TEMPORARY BRIDGE намеренно не трогаются: их судьбу решает
// миграция W2, а до неё они кормят legacy-проекцию сборки.
func normalizeSourceShape(s *Source) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	var warns []string
	drop := func(field string) {
		warns = append(warns, fmt.Sprintf("source %s (kind=%s): поле %q нелегально для этого kind — отброшено", sourceShapeName(s), s.Kind, field))
	}

	switch s.Kind {
	case SourceKindServer, SourceKindChain, SourceKindAuto:
		// Узловые kind'ы: папочно-подписочных полей нет.
		if s.Name != "" {
			drop("name")
			s.Name = ""
		}
		if s.TagPolicy != nil {
			drop("tag_policy")
			s.TagPolicy = nil
		}
		if len(s.Nodes) > 0 {
			drop("nodes")
			s.Nodes = nil
		}
		if s.Replace != nil {
			drop("replace")
			s.Replace = nil
		}
		if s.URL != "" || len(s.Skip) > 0 || s.MaxNodes != 0 || s.Update != nil || s.Meta != nil || s.UpdateStatus != nil || len(s.PendingDisabled) > 0 {
			drop("url/skip/max_nodes/update/meta/update_status/pending_disabled")
			s.URL, s.Skip, s.MaxNodes, s.Update, s.Meta, s.UpdateStatus, s.PendingDisabled = "", nil, 0, nil, nil, nil, nil
		}
		if ws := normalizeNodeShape(&s.Node, sourceShapeName(s)); len(ws) > 0 {
			warns = append(warns, ws...)
		}

	case SourceKindFolder, SourceKindSubscription:
		// Папка/подписка: узловые поля Node заняты только kind/enabled/detour
		// (общий detour папки); tag у контейнера пуст.
		if s.Tag != "" {
			drop("tag")
			s.Tag = ""
		}
		if s.Origin != nil {
			drop("origin")
			s.Origin = nil
		}
		if len(s.Body) > 0 {
			drop("body")
			s.Body = nil
		}
		if len(s.Hops) > 0 {
			drop("hops")
			s.Hops = nil
		}
		if s.Group != nil {
			drop("group")
			s.Group = nil
		}
		if s.Kind == SourceKindFolder {
			if s.URL != "" || len(s.Skip) > 0 || s.MaxNodes != 0 || s.Update != nil || s.Meta != nil || s.UpdateStatus != nil || len(s.PendingDisabled) > 0 {
				drop("url/skip/max_nodes/update/meta/update_status/pending_disabled")
				s.URL, s.Skip, s.MaxNodes, s.Update, s.Meta, s.UpdateStatus, s.PendingDisabled = "", nil, 0, nil, nil, nil, nil
			}
		}
		for i := range s.Nodes {
			n := &s.Nodes[i]
			switch n.Kind {
			case SourceKindServer, SourceKindChain, SourceKindAuto:
				if ws := normalizeNodeShape(n, fmt.Sprintf("%s/nodes[%d]", sourceShapeName(s), i)); len(ws) > 0 {
					warns = append(warns, ws...)
				}
			default:
				return warns, fmt.Errorf("state: source %s: node %q несёт неизвестный kind %q — файл от более новой схемы, обновите приложение", sourceShapeName(s), n.Tag, n.Kind)
			}
		}

	default:
		return warns, fmt.Errorf("state: source %s несёт неизвестный kind %q — файл от более новой схемы, обновите приложение", sourceShapeName(s), s.Kind)
	}
	return warns, nil
}

// normalizeNodeShape — та же проверка формы на уровне Node (узловые kind'ы).
// Server без body здесь НЕ деградируется: в мостовую эпоху W1-W4 тело живёт
// в легаси-полях (URI/ConfigJSON/raw-кэш), правило битого фрагмента включится
// вместе с материализацией (W2/W4).
func normalizeNodeShape(n *Node, name string) []string {
	var warns []string
	drop := func(field string) {
		warns = append(warns, fmt.Sprintf("node %s (kind=%s): поле %q нелегально для этого kind — отброшено", name, n.Kind, field))
	}
	switch n.Kind {
	case SourceKindServer:
		if len(n.Hops) > 0 {
			drop("hops")
			n.Hops = nil
		}
		if n.Group != nil {
			drop("group")
			n.Group = nil
		}
	case SourceKindChain:
		if len(n.Body) > 0 {
			drop("body")
			n.Body = nil
		}
		if n.Detour != nil { // detour у Chain не существует (типом — SPEC Т2)
			drop("detour")
			n.Detour = nil
		}
		if n.Group != nil {
			drop("group")
			n.Group = nil
		}
	case SourceKindAuto:
		if len(n.Body) > 0 {
			drop("body")
			n.Body = nil
		}
		if n.Detour != nil { // detour у Auto не существует (типом — SPEC Т2)
			drop("detour")
			n.Detour = nil
		}
		if len(n.Hops) > 0 {
			drop("hops")
			n.Hops = nil
		}
		if n.Group == nil {
			warns = append(warns, fmt.Sprintf("node %s: kind=auto без group — группа не эмитится", name))
		}
	}
	return warns
}

// sourceShapeName — адрес источника для текстов warning'ов.
func sourceShapeName(s *Source) string {
	if s.ID != "" {
		return s.ID
	}
	if s.Tag != "" {
		return s.Tag
	}
	if s.Name != "" {
		return s.Name
	}
	return "<unnamed>"
}
