//go:build darwin || (windows && !386)

// File chain_probe.go — общее для послойной пробы цепочки (SPEC 110).
//
// Живёт в core, а не в одном из транспортов: локальный демон и удалённая
// машина спрашивают ядро одними и теми же RPC, и расхождение в том, какой
// тег отправлен или как истолкован ответ, дало бы разные диагнозы на одной
// и той же цепочке.
package core

import (
	"time"

	"singbox-launcher/api"
	"singbox-launcher/core/config"
)

// chainProbeTag — что мерить: префикс до позиции pos или всю цепочку.
//
// pos < 0 означает саму цепочку. Отрицательный индекс вместо отдельного
// метода потому, что для вызывающего это один и тот же вопрос «сколько
// стоит путь», и разводить его на две ветки в каждом транспорте незачем.
func chainProbeTag(chainTag string, pos int) string {
	if pos < 0 {
		return chainTag
	}
	return config.ChainLayerTag(chainTag, pos)
}

// chainProbeCallTimeout — дедлайн RPC: бюджет теста плюс запас.
//
// Запас нужен, чтобы дедлайн вызова НЕ срабатывал раньше пробы: иначе
// медленный хоп возвращал бы транспортную ошибку вместо своей честной
// цены, то есть ровно тот случай, ради которого пробу и смотрят.
func chainProbeCallTimeout() time.Duration {
	return time.Duration(api.GetPingTestTimeoutMs())*time.Millisecond + 5*time.Second
}
