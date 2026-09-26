package backup

// Форма файла бэкапа 1.0 (SPEC 127 §6.0, норма — contract/docs/ONE_NAMESPACE.md).
//
// Главное отличие от 0.12: маппера больше нет. Записи файла — ТЕ ЖЕ ТИПЫ
// состояния (state.Source, state.Rule, state.DNSOptions), сериализованные
// своими struct-тегами. Бэкап стал сериализацией состояния (П1), и потому
// никакое поле не может «забыться в конверторе»: добавленное в состояние
// поедет в файл само.
//
// Тонкий слой остаётся ровно там, где у контракта уже есть своё имя или где
// поле по смыслу в файл не едет (перечень зафиксирован с LxBox 14.09.2026):
//
//   - directions[] — форма контракта (direction.schema.json): в ней живут
//     LxBox-поля label/ping_*, которых в модели лаунчера нет;
//   - fold — форма контракта (source_fold.schema.json), а не state-ный
//     `replace`: у свёртки в контракте уже есть имя;
//   - disabled{} — карта «тег → unix seconds» формы 0.12: у LxBox отметка
//     живёт с TTL, и менять её форму значило бы ломать обе стороны разом;
//   - identity{} — объект (он и в состоянии v8 объект, см.
//     core/state/subscription_identity.go);
//   - vars — только переносимые имена (registry/vars.json);
//   - route{final}, warp[] — как в 0.12.
//
// Что НЕ едет у подписки: nodes[]/meta/update_status (кэш и рантайм машины) и
// pending_disabled (её роль в файле играет disabled{}).

import (
	"encoding/json"

	"singbox-launcher/core/state"
)

// FormatVersion10 — маркер формата 1.0 в ключе lx_backup.
//
// Тот же int-маркер, что у 0.x (там 1): читатель различает форматы по нему
// одним сравнением, ещё до разбора тела. Строковой версии в корне нет
// намеренно — два способа сказать «какой это формат» разошлись бы.
const FormatVersion10 = 2

// Backup10 — корень файла 1.0. Порядок полей = порядок ключей в файле
// (encoding/json пишет struct по объявлению): файл читают и правят руками, и
// перестановка ключей между версиями лаунчера была бы шумом в diff'ах.
type Backup10 struct {
	LxBackup   int        `json:"lx_backup"`
	ExportedBy ExportedBy `json:"exported_by"`
	ExportedAt string     `json:"exported_at"`

	// Sources — записи состояния: union по kind (server | folder |
	// subscription | chain), у папки nodes[] внутри. Плоского servers[] с
	// полем folder, как в 0.12, здесь нет: папка с составом внутри выражает
	// владение, а плоский список — нет (ONE_NAMESPACE, принцип 3).
	Sources []Source10 `json:"sources,omitempty"`
	// Directions — форма контракта, единственное исключение из «как в
	// состоянии»: у сторон свои внутренние структуры Направлений.
	Directions []Direction `json:"directions,omitempty"`
	// Rules — записи состояния v8 как есть (kind/name/num/refs/vars/body).
	Rules []state.Rule `json:"rules,omitempty"`
	// DNS — секция состояния v8 как есть (strategy, final, servers[], rules[]).
	DNS *state.DNSOptions `json:"dns,omitempty"`
	// Vars — только переносимые имена.
	Vars map[string]string `json:"vars,omitempty"`
	// Route — финальный outbound; отдельным объектом, как в 0.12.
	Route *Route `json:"route,omitempty"`
	// Warp — регистрации WG/MASQUE сырым JSON, как в 0.12.
	Warp []json.RawMessage `json:"warp,omitempty"`

	// ruleGroups — адреса членов папок (индекс в Sources, индекс в Nodes),
	// у которых группа задана правилом отбора (`group.members_rule`, поле
	// стороны LxBox) без явного состава. Заполняет Parse по сырому файлу:
	// поля в типах нет (лаунчер групп по правилу не держит), а решение «такую
	// группу не ввозить» принимает разбор записи (decode10Source). Не
	// сериализуется.
	ruleGroups map[[2]int]bool
}

// Source10 — запись sources[]: поля state.Source, кроме кэша и рантайма, плюс
// два поля формой контракта.
//
// Поля перечислены явно, а не получены встраиванием state.Source: снять
// ненужное тегом `json:"-"` поверх встроенной структуры нельзя — encoding/json
// не считает игнорируемое поле участником разрешения конфликта имён, и
// встроенное поле уезжает в файл как ни в чём не бывало (проверено: meta,
// update_status и pending_disabled всплывали в файле).
//
// Расхождение с состоянием ловит тест: он сверяет набор json-ключей этого
// типа с набором ключей state.Source, чтобы поле, добавленное в состояние,
// нельзя было забыть здесь.
//
// Порядок полей = порядок ключей файла: сперва общая часть узла (её несёт
// любой вид записи), затем поля контейнера, затем поля подписки, затем два
// контрактных.
type Source10 struct {
	// ── общее для всех видов (state.Node) ──
	Kind    state.SourceKind `json:"kind"`
	Tag     string           `json:"tag,omitempty"`
	Enabled bool             `json:"enabled"`
	Origin  *state.Origin    `json:"origin,omitempty"`
	Body    json.RawMessage  `json:"body,omitempty"`
	Detour  *state.NodeLink  `json:"detour,omitempty"`
	Hops    []state.NodeLink `json:"hops,omitempty"`
	Group   *state.AutoGroup `json:"group,omitempty"`
	// Service и Reason — признаки узла, приехавшего из чужой записи или не
	// разобранного вовсе. У корневого источника их не бывает, но у узлов
	// внутри папки бывают, и там едет state.Node целиком.
	Service bool   `json:"service,omitempty"`
	Reason  string `json:"reason,omitempty"`
	// Warnings — коды деградаций узла (контракт 1.1.0, SPEC 131). Пишутся,
	// чтобы ⚠ переехало «как было»; ЧИТАЮТСЯ, но в состояние не кладутся:
	// данные производные и при переносе не авторитетны (PARSING_PRINCIPLES §6) —
	// приёмник считает их сам по своему реестру. Поле объявлено, поэтому
	// неизвестным ключом оно не считается и импорт на него не ругается.
	// Исключение — узел-группа kind=auto (контракт 1.1.66): тела нет,
	// пересчитывать нечем, её записи кладутся в состояние как есть
	// (decode10Source).
	Warnings []state.NodeWarning `json:"warnings,omitempty"`
	Sections *state.NodeSections `json:"sections,omitempty"`

	// ── контейнер (папка | подписка) ──
	ID        string           `json:"id,omitempty"`
	Name      string           `json:"name,omitempty"`
	TagPolicy *state.TagPolicy `json:"tag_policy,omitempty"`
	// Nodes — состав ПАПКИ (её узлы — пользовательская настройка). У
	// подписки это кэш выдачи провайдера, и экспорт его обнуляет.
	Nodes []state.Node `json:"nodes,omitempty"`

	// ── подписка ──
	URL                string                      `json:"url,omitempty"`
	Identity           *state.SubscriptionIdentity `json:"identity,omitempty"`
	RelaysInDirections bool                        `json:"relays_in_directions,omitempty"`
	Skip               []map[string]string         `json:"skip,omitempty"`
	MaxNodes           int                         `json:"max_nodes,omitempty"`
	Update             *state.UpdateSpec           `json:"update,omitempty"`

	// ── формой контракта ──
	//
	// Disabled — отметки выключенных узлов подписки: «тег → unix seconds».
	// Значение у лаунчера всегда 0 (карта времён умерла вместе с TTL), но
	// ключ числовой — у LxBox по нему живёт очистка. В состоянии ту же роль
	// играют выключенные узлы кэша и PendingDisabled, которых в файле нет.
	Disabled map[string]int64 `json:"disabled,omitempty"`
	// Replace — свёртка источника в группу, ТОЙ ЖЕ формой, что в состоянии
	// (`{mode, tag, auto?}`, контракт 1.1.78): одно имя и одна форма в
	// state.json, в файле и у LxBox, поэтому конвертера у поля нет.
	Replace *state.FolderReplace `json:"replace,omitempty"`

	// Fold и FoldTag — ПРЕЖНЯЯ форма свёртки в файле 1.0 (контракт 1.0 —
	// 1.1.77): объект `fold{mode: select|auto|select_auto, auto?}` формы
	// source_fold.schema.json плюс явное имя группы ключом рядом. Писатель
	// их больше не пишет; читатель терпит (legacy-вход, BACKUP.md §2):
	// select → manual, select_auto → both, fold_tag → tag, а без fold_tag
	// имя выводится прежним позиционным деривативом (foldTag10). При
	// `replace` в той же записи эти ключи не читаются.
	Fold    *Fold  `json:"fold,omitempty"`
	FoldTag string `json:"fold_tag,omitempty"`
}

// source10ExcludedStateKeys — json-ключи state.Source, которых в файле 1.0
// нет намеренно. Список читает сверочный тест: любое ДРУГОЕ расхождение
// между состоянием и записью файла — забытое поле, а не решение.
var source10ExcludedStateKeys = map[string]string{
	"meta":             "рантайм fetch'а этой машины",
	"update_status":    "рантайм fetch'а этой машины",
	"pending_disabled": "внутренний буфер отметок; в файле его роль играет disabled{}",
}
