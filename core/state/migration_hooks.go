// File migration_hooks.go — хуки материализации для миграции v6 → v7.
//
// Материализация (шаг 1 миграции) требует парсера подписок и эмиттеров
// outbound-JSON, а они живут в core/config{,/subscription}, которые сами
// импортируют state — прямой вызов дал бы цикл импорта. Используется тот же
// приём, что у subscription.NodeIdentityFunc: пакет config подставляет
// реализацию в init() (core/config/migrate_materialize.go), state зовёт её
// вслепую. nil-хук — парсер недоступен (тесты пакета state в изоляции):
// материализация не выполняется, миграция честно предупреждает.
package state

import "encoding/json"

// MigrationSubRequest — материализация одной подписки из raw-кэша.
type MigrationSubRequest struct {
	// SubID — ULID подписки (folderId членов Auto-групп).
	SubID string
	// Body — СЫРОЕ тело из bin/subscriptions/<id>.raw (декодирование
	// base64 — забота реализации хука, как у fetch-пути).
	Body []byte
	Skip []map[string]string
	// MaxNodes — уже разрешённый кап (подписка → дефолт → потолок 3000).
	MaxNodes int

	// Старая тег-машина (PLAN §5, шаги 4–5): финальные теги узлов считаются
	// политикой источника поверх ОБЩЕГО счётчика уникализации TagCounts,
	// который миграция ведёт по всем источникам в порядке v6.
	TagPrefix  string
	TagPostfix string
	TagMask    string
	TagCounts  map[string]int
}

// MigrationMaterializedNode — один материализованный узел подписки.
type MigrationMaterializedNode struct {
	// Node — канонический узел v7 (server с body/origin или auto с group).
	Node Node
	// FinalTag — финальный конфиг-тег по СТАРОЙ тег-машине: под ним узел
	// значился в прежних config.json, правилах и хопах.
	FinalTag string
	// LegacyHash — упразднённый контент-хэш SPEC 094/101 (пусто у групп):
	// по нему докручиваются legacy-64hex-ключи disabled-карты и
	// detour_node_hash.
	LegacyHash string
}

// MigrationSubResult — итог материализации подписки.
type MigrationSubResult struct {
	Nodes     []MigrationMaterializedNode
	Truncated bool
	Warnings  []string
}

// MigrationServerRequest — материализация body корневого server-источника
// из его URI либо ручного config_json.
type MigrationServerRequest struct {
	URI        string
	ConfigJSON json.RawMessage
}

// MigrationServerResult — body + происхождение корневого узла.
type MigrationServerResult struct {
	Body       json.RawMessage
	OriginKind string
	OriginRaw  string
	// LegacyHash — контент-хэш узла (адресат detour_node_hash).
	LegacyHash string
	// Warnings — коды деградаций, проставленных разбором этому узлу
	// (SPEC 131 W2b). Едут до самой записи узла: до этого они умирали в
	// материализации, и пользователь видел узел без следа того, что у него
	// сняли поле.
	Warnings []NodeWarning
}

// SanitizeBodyRequest — разовый пересчёт кодов по УЖЕ СОХРАНЁННОМУ телу
// (SPEC 131 W2c §3.5, ловушка Л3).
type SanitizeBodyRequest struct {
	// Body — тело узла из state как есть; схема определяется по его "type".
	Body json.RawMessage
	// OriginKind — вход, которым узел когда-то приехал (Origin.Kind:
	// "uri" | "wg_ini" | "json"); пусто, если происхождения у узла нет.
	//
	// Его читают правила значений, различающие, КТО сочинил значение: тело в
	// форме ядра человек или подписка написали сами, и лаунчер его не
	// переписывает — он предупреждает (потолок MTU у AmneziaWG). Без этого
	// поля исключение жило бы ровно до первой загрузки state: пересчёт кодов
	// увидел бы «вход неизвестен» и заклампил бы тело задним числом — то
	// есть настройка пользователя исчезала бы от перезапуска.
	OriginKind string
}

// SanitizeBodyResult — итог пересчёта.
type SanitizeBodyResult struct {
	// Body — тело после санитайзера. Непусто ТОЛЬКО если санитайзер что-то
	// снял или привёл: перезаписывать байты, в которых ничего не изменилось,
	// нельзя — это «менялось» у каждого узла на каждом апгрейде.
	Body json.RawMessage
	// Warnings — коды по этому телу; пустой (не nil) список = «считали, чисто».
	Warnings []NodeWarning
	// Drop — тело не проходит правила реестра совсем (ядро отвергло бы весь
	// конфиг). Узел при этом НЕ выбрасывается: он уже в состоянии, и
	// молчаливый снос чужого узла на апгрейде хуже, чем узел с кодами.
	Drop bool
}

// MigrationHooks — набор реализаций, подставляемых пакетом config.
type MigrationHooks struct {
	MaterializeSubscription func(req MigrationSubRequest) (*MigrationSubResult, error)
	MaterializeServer       func(req MigrationServerRequest) (*MigrationServerResult, error)
	// SanitizeBody — пересчёт кодов по сохранённому телу (без origin.raw).
	SanitizeBody func(req SanitizeBodyRequest) (*SanitizeBodyResult, error)
}

var migrationHooks MigrationHooks

// SetMigrationHooks подставляет реализации (вызывается из init пакета
// config). Повторный вызов перезаписывает — последний импортированный
// комплект побеждает, что для единственного поставщика эквивалентно
// идемпотентности.
func SetMigrationHooks(h MigrationHooks) {
	migrationHooks = h
}
