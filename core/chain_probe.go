//go:build darwin

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
	daemonpb "singbox-launcher/internal/daemonpb"
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

// chainInfosFromPB — перевод ответа ядра во внутренний тип.
func chainInfosFromPB(chains []*daemonpb.ChainState) []ChainInfo {
	out := make([]ChainInfo, 0, len(chains))
	for _, c := range chains {
		if c == nil {
			continue
		}
		positions := make([]ChainPositionInfo, 0, len(c.GetPositions()))
		for _, p := range c.GetPositions() {
			if p == nil {
				continue
			}
			info := ChainPositionInfo{
				Tag:         p.GetTag(),
				Now:         p.GetNow(),
				IsGroup:     p.GetIsGroup(),
				Transparent: p.GetTransparent(),
				Disabled:    p.GetDisabled(),
			}
			if cl := p.GetClone(); cl != nil {
				info.CloneState = cl.GetState()
				info.LastError = cl.GetLastError()
			}
			positions = append(positions, info)
		}
		out = append(out, ChainInfo{Tag: c.GetTag(), Positions: positions})
	}
	return out
}
