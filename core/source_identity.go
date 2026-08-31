// File source_identity.go — идентификация подписки в HTTP-запросе fetch'а.
//
// Переопределения живут в самой подписке (state.Source), а применяет их
// fetcher; этот файл — мостик между ними. Отдельный файл, потому что мостик
// нужен ДВУМ вызывающим: конвейеру обновления (core) и диагностическому
// Reload в окне источника (ui) — вторая копия правила «что чем перекрывать»
// разъехалась бы с первой.
package core

import (
	"singbox-launcher/core/config/subscription"
	"singbox-launcher/core/state"
)

// SourceIdentityOf — переопределения идентификации из записи подписки.
//
// Пустые поля означают «как в системе»: fetcher подставит глобальную
// настройку, а при её отсутствии — дефолт лаунчера.
func SourceIdentityOf(src *state.Source) subscription.SourceIdentity {
	if src == nil {
		return subscription.SourceIdentity{}
	}
	return subscription.SourceIdentity{
		UserAgent: src.UserAgent,
		HWID:      src.HWID,
		SendHWID:  src.SendHWID,
		HashModel: src.HashDeviceModel,
	}
}
