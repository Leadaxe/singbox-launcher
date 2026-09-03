package build

import "encoding/json"

// ParsedCache — in-memory результат парсинга подписок: готовые к вставке
// JSON-блоки sing-box outbound + WireGuard endpoint объекты.
//
// SPEC 052 phase 8: формат `bin/outbounds.cache.json` удалён;
// `core/outboundscache` package retired. Эта структура — pure-data carrier
// между Update/Rebuild (в `core/`) и BuildConfig (в `core/build/`).
//
// Заполняется одним из:
//   - `core.refreshSubscriptionsMetaAndCache` после успешного fetch'а
//     (Update path); парсер формирует `[]string`-блоки → `jsonStringsToRawMessages`
//     → этот тип.
//   - `core.buildSnapshotFromRawCache` при Rebuild без сети (читает
//     `bin/subscriptions/*.raw`).
//   - In-memory mode визарда: `business.inMemoryCacheFromModel` для preview.
type ParsedCache struct {
	// Outbounds — готовые JSON-блоки sing-box outbound (для @ParserSTART/@ParserEND).
	Outbounds []json.RawMessage

	// Endpoints — WireGuard endpoints (если используются).
	Endpoints []json.RawMessage

	// Warnings — non-fatal замечания парсера, о которых должен узнать
	// пользователь (например, деградированные naive-ноды на ядре без
	// with_naive_outbound). Caller (RebuildConfigIfDirty) присоединяет их
	// к Result.Validation.Warnings; BuildConfig это поле не читает.
	Warnings []string

	// NodeOrigins — финальный тег узла → источник, из которого он приехал
	// (SPEC 113-B). Граф-санитайзер видит только теги; выбросив узел за
	// висячий detour, он обязан назвать источник, у которого сломался
	// переход, — иначе исключение снова становится молчаливым. Пустая карта
	// не ошибка: узел тогда назовут собственным тегом.
	NodeOrigins map[string]NodeOrigin
}

// NodeOrigin — чей это узел: ULID источника и его человеческая подпись.
// Зеркалит config.NodeOrigin; своё определение здесь, потому что core/build
// про core/config не знает (и не должен: зависимость идёт в другую сторону).
type NodeOrigin struct {
	SourceID    string
	SourceLabel string
}

// SourceExclusion — источник, чьи узлы выброшены на последнем рубеже
// (SPEC 113-B). Та же тройка, что у config.SourceExclusion, плюс счётчик;
// вызывающий доливает эти записи в отчёт сборки поверх записей парсера.
//
// DroppedNodes (SPEC 115) — СКОЛЬКО узлов источника снято. Санитайзер работает
// по узлам, а отчёт группируется по источнику: у подписки на 500 узлов один
// сломанный переход снимает их все разом, и без числа пометка «источник
// пострадал» не отличает потерю одного узла от потери всей подписки.
type SourceExclusion struct {
	SourceID     string
	SourceLabel  string
	Reason       string
	DroppedNodes int
	// MissingTarget — тег цели detour, которой не оказалось в собранном
	// конфиге; пусто, когда узлы сняты по другой причине (кольцо ссылок).
	// Отдельным полем, а не разбором Reason: отчёт заводит на такую потерю
	// собственный вид записи (target_missing, SPEC 115 §2), и вытаскивать тег
	// обратно из человеческого текста было бы разбором собственного вывода.
	MissingTarget string
}
