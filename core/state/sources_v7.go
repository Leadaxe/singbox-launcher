// File sources_v7.go — канонические типы дерева источников схемы v7
// (SPEC 118, этап 2; формы — PLAN §1.2, нормативный каркас).
//
// Принят плоский юнион с дискриминатором `kind` (PLAN §1.1в): один Go-struct
// на уровень, поля чужих kind'ов пустые/omitempty — ровно паттерн rules[] и
// dns_options (SPEC 053/056). Нелегальные комбинации полей закрывает
// normalizeSourceShape на Load и конструкторы.
//
// SPEC 118 W5: моста больше нет. Легаси-поля v6 (disabled_nodes, fold,
// exclude/expose, detour-тройня, локальные Направления, tag.mask, uri/
// config_json/chain-строки) в этом типе не существуют: их читает ТОЛЬКО вход
// миграции (legacySourceV6, migration_legacy_source.go) и выбрасывает шаг 8.
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
	// SourceKindUnsupported — запись тела, которую разобрать не удалось
	// (SPEC 116 W11). Живёт ТОЛЬКО внутри контейнера: корневым источником
	// такой узел не бывает — в корень его класть неоткуда.
	SourceKindUnsupported SourceKind = "unsupported"
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
}

// IsZero — нечего применять (все поля пустые).
func (t *TagPolicy) IsZero() bool {
	if t == nil {
		return true
	}
	return t.Prefix == "" && t.Postfix == ""
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

// Node — узел: kind ∈ {server, chain, auto, unsupported}. Живёт в корне
// sources[] (через embedded в Source) и в Folder.Nodes; kind=unsupported —
// только в контейнере.
//
// # Unsupported (SPEC 116 W11)
//
// Отбракованная запись тела не исчезает, а материализуется узлом
// kind=unsupported: он занимает СВОЮ позицию в nodes[], несёт `reason`
// (почему не разобрали) и ОБЯЗАТЕЛЬНЫЙ origin (raw = запись как пришла).
// Тела у него нет, detour неприменим, `enabled` всегда false и включение
// запрещено — собирать из него нечего. Эмиссия пропускает его ДО тег-машины
// (не потребляет ни {$num}, ни слот уникализации — старый движок такую запись
// узлом не считал вовсе), поэтому появление такого узла НЕ двигает финальные
// теги соседей. Целью NodeLink он быть не может — его просто нет среди узлов
// сборки.
type Node struct {
	Kind SourceKind `json:"kind"`
	// Tag — СЫРОЙ тег: идентичность в рамках контейнера, снятый ДО
	// тег-политики. На нём живут merge-ключ, enabled, detour и ссылки
	// NodeLink. У корневых узлов политики нет — финальный тег = сырой.
	Tag     string  `json:"tag"`
	Enabled bool    `json:"enabled"`
	Origin  *Origin `json:"origin,omitempty"`

	// Body — готовый sing-box outbound, чист от detour (никакого запекания
	// Outbound["detour"]).
	//
	//   - server: тело узла целиком (минус tag/detour);
	//   - chain:  настройки маршрута БЕЗ позиций — `type`, `idle_timeout`,
	//     `strip_evasion`, `strip`, `rewrite`. Позиции живут отдельным полем
	//     Hops: они ссылки (NodeLink), а не значения, и в теле им делать
	//     нечего — сборка подставляет их резолвом, как и всякую ссылку.
	//
	// У auto тела нет: группа целиком описана Group.
	Body json.RawMessage `json:"body,omitempty"`
	// Detour — server only; у kind=folder тем же ключом едет ОБЩИЙ detour
	// папки (одна json-точка на оба смысла — семантика по kind, как всё в
	// юнионе). У Chain и Auto detour не существует типом.
	Detour *NodeLink `json:"detour,omitempty"`
	// Hops — chain only: ближний хоп первым.
	Hops []NodeLink `json:"hops,omitempty"`
	// Group — auto only.
	Group *AutoGroup `json:"group,omitempty"`
	// Reason — unsupported only: почему запись не разобралась. Текст
	// парсера (английский, как и прочие per-record деградации): он же едет
	// в диагностику fetch, и переводить его на месте значило бы завести две
	// формулировки одной причины.
	Reason string `json:"reason,omitempty"`
}

// IsUnsupported — узел является нематериализованной записью тела.
func (n *Node) IsUnsupported() bool {
	return n != nil && n.Kind == SourceKindUnsupported
}

// NewUnsupportedNode — отбракованная запись тела как узел контейнера
// (SPEC 116 W11). Origin обязателен: без исходника пользователю не по чему
// узнать запись, а починить её — тем более.
func NewUnsupportedNode(tag, reason, originKind, originRaw string) Node {
	return Node{
		Kind:    SourceKindUnsupported,
		Tag:     tag,
		Enabled: false,
		Origin:  &Origin{Kind: originKind, Raw: originRaw},
		Reason:  reason,
	}
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
	// У узловых kind'ов (server/chain/auto) ULID тоже живёт: на него
	// ссылаются бэкап (SourceRef 0.11) и адресация UI-операций; ссылок на
	// узлы по нему в модели v7 больше нет — их место занял NodeLink.
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
	// (вердикт O2, SPEC 118): их пишет импорт бэкапа и миграция, когда
	// nodes[] ещё пусты и применять отметку не к чему.
	// Применяются на первом ДОСТОВЕРНОМ
	// fetch (MergeSubscriptionNodes) и стираются; при truncated-разборе
	// несматченные теги переживают fetch — узел мог остаться за капом.
	// Это не TTL-карта: поле живёт только между импортом и первым fetch.
	PendingDisabled []string `json:"pending_disabled,omitempty"`

	// Label — LEGACY-ВХОД, только на чтение (обкатка заход 3).
	//
	// Раньше это было второе имя узловых kind'ов рядом с тегом; поле формы
	// снято, новой записи поле не получает, и в канон v7 оно не пишется —
	// `json:"-"`, чтобы не уезжать обратно на диск. Читается там, где ещё
	// приходит: миграция v6→v7 (переносит его в Node.Tag / Name), загрузка
	// старых состояний и импорт бэкапов до канонизации W7.
	//
	// Имя узла в каноне — его ТЕГ (SPEC 112, [[node-identity-is-tag]]).
	Label string `json:"-"`
}

// NodeTagOrLabel — системный тег узла источника (server / chain / auto).
//
// В каноне v7 тег живёт в Node.Tag; откат на Label оставлен для узлов,
// у которых тег не проставлен вовсе (импорт бэкапа до канонизации W7,
// состояния до разделения ролей — там тег лежал именно в Label).
func (s Source) NodeTagOrLabel() string {
	if s.Tag != "" {
		return s.Tag
	}
	return s.Label
}

// SubMeta — метаданные подписки: заголовки провайдера, userinfo, announce.
//
// Канон v7 = прежний SubscriptionMeta МИНУС fetch-история и превью: история
// живёт в SubUpdateStatus (единственный дом диагностики), превью узлов
// упразднено вместе с ленивым кэшем — UI читает nodes[].
type SubMeta struct {
	// headers (HTTP response + inline #-comments в body первой строкой)
	ProfileTitle               string    `json:"profile_title,omitempty"`
	ProfileUpdateIntervalHours int       `json:"profile_update_interval_hours,omitempty"`
	SupportURL                 string    `json:"support_url,omitempty"`
	ProfileWebPageURL          string    `json:"profile_web_page_url,omitempty"`
	ContentDispositionFilename string    `json:"content_disposition_filename,omitempty"`
	UserInfo                   *UserInfo `json:"userinfo,omitempty"`

	// SPEC 061: provider announce headers (success **or** failure).
	ProviderAnnounce *ProviderAnnounce `json:"provider_announce,omitempty"`
}

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
			case SourceKindServer, SourceKindChain, SourceKindAuto, SourceKindUnsupported:
				if ws := normalizeNodeShape(n, fmt.Sprintf("%s/nodes[%d]", sourceShapeName(s), i)); len(ws) > 0 {
					warns = append(warns, ws...)
				}
			default:
				return warns, fmt.Errorf("state: source %s: node %q несёт неизвестный kind %q — файл от более новой схемы, обновите приложение", sourceShapeName(s), n.Tag, n.Kind)
			}
		}

	case SourceKindUnsupported:
		// Неразобранная запись живёт только внутри контейнера: её родил разбор
		// тела, а у корневого источника тела нет. В корне она бы ничего не
		// значила — и никакой fetch её оттуда не починил бы.
		return warns, fmt.Errorf("state: source %s: kind=unsupported легален только внутри контейнера", sourceShapeName(s))

	default:
		return warns, fmt.Errorf("state: source %s несёт неизвестный kind %q — файл от более новой схемы, обновите приложение", sourceShapeName(s), s.Kind)
	}
	return warns, nil
}

// normalizeNodeShape — та же проверка формы на уровне Node (узловые kind'ы).
// Server без body здесь НЕ деградируется: подписка, которую ещё ни разу не
// обновляли, легально приезжает с пустыми nodes[] (warning отчёта сборки —
// не отказ загрузки).
func normalizeNodeShape(n *Node, name string) []string {
	var warns []string
	drop := func(field string) {
		warns = append(warns, fmt.Sprintf("node %s (kind=%s): поле %q нелегально для этого kind — отброшено", name, n.Kind, field))
	}
	// Причина живёт только у неразобранной записи: у собравшегося узла ей
	// нечего объяснять, а показанная строкой «⚠ …» она соврала бы.
	if n.Kind != SourceKindUnsupported && n.Reason != "" {
		drop("reason")
		n.Reason = ""
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
		// Body у цепочки легален: там живут её настройки без позиций
		// (idle_timeout / strip / rewrite) — см. комментарий у Node.Body.
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
	case SourceKindUnsupported:
		// Собирать из неразобранной записи нечего: тела, маршрута и состава у
		// неё не существует типом. Включённой она тоже не бывает — иначе
		// «включено» обещало бы узел в конфиге, которого нет.
		if len(n.Body) > 0 {
			drop("body")
			n.Body = nil
		}
		if n.Detour != nil {
			drop("detour")
			n.Detour = nil
		}
		if len(n.Hops) > 0 {
			drop("hops")
			n.Hops = nil
		}
		if n.Group != nil {
			drop("group")
			n.Group = nil
		}
		if n.Enabled {
			drop("enabled")
			n.Enabled = false
		}
		if n.Origin == nil {
			// Исходник — единственное, что у такой записи есть. Без него узел
			// не рассказывает ни что это было, ни как это чинить.
			warns = append(warns, fmt.Sprintf("node %s: kind=unsupported без origin — исходник записи потерян", name))
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
