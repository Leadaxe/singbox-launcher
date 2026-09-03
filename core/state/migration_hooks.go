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
}

// MigrationHooks — набор реализаций, подставляемых пакетом config.
type MigrationHooks struct {
	MaterializeSubscription func(req MigrationSubRequest) (*MigrationSubResult, error)
	MaterializeServer       func(req MigrationServerRequest) (*MigrationServerResult, error)
}

var migrationHooks MigrationHooks

// SetMigrationHooks подставляет реализации (вызывается из init пакета
// config). Повторный вызов перезаписывает — последний импортированный
// комплект побеждает, что для единственного поставщика эквивалентно
// идемпотентности.
func SetMigrationHooks(h MigrationHooks) {
	migrationHooks = h
}
