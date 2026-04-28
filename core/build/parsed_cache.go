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
}

// IsEmpty — true, если cache не содержит ни одного outbound и ни одного endpoint.
// nil-receiver обрабатывается как пустой.
func (c *ParsedCache) IsEmpty() bool {
	if c == nil {
		return true
	}
	return len(c.Outbounds) == 0 && len(c.Endpoints) == 0
}
