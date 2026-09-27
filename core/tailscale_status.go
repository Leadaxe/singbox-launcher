// File tailscale_status.go — состояние tailnet у работающего ядра (SPEC 130).
//
// Источник — стрим SubscribeTailscaleStatus демона: он отдаёт статус ВСЕХ
// tailscale-endpoint'ов разом, с EndpointTag, первое сообщение сразу после
// подписки, дальше по событиям IPN-шины. Стрим один на процесс и живёт всё
// время коннекта — не открывается на время окна: колонка на вкладке
// Endpoints обязана быть актуальной без открытого окна, а NeedsLogin — то,
// о чём хочется знать, не заглядывая внутрь (решение владельца 16.09.2026).
//
// Диспетчер выбирает источник по цели (TailscaleTarget): Local — бэкенд
// своего ядра, Remote — транспорт машины из её выбора (не из APIService).
// Конвертер pb → доменный тип ОДИН на оба пути — локальный демон и роутер
// обязаны давать одинаковый диагноз (тот же довод, что у chainInfosFromPB).
//
// Только gRPC-бэкенды: legacy на Windows/Linux ходит в Clash API и статуса
// не получит — секция там не рисуется, как чейн-секция через
// ChainsAvailable(). Ограничение v1, записано в SPEC 130 §3.
package core

import (
	"context"
	"errors"

	"singbox-launcher/core/services"
)

// TailscaleStatus — алиас доменного типа из services (см. довод в
// services/tailscale_status.go: кеш и конвертер живут там из-за цикла).
type TailscaleStatus = services.TailscaleStatus

// tailscaleSource — бэкенд или транспорт, держащий кеш статуса tailnet.
//
// Кеш, а не запрос: стрим уже открыт, и UI читает последнее полученное.
// ok=false — статуса по этому тегу нет (стрим не поднялся, узла у ядра нет,
// либо ядро без with_tailscale).
type tailscaleSource interface {
	TailscaleStatus(tag string) (services.TailscaleStatus, bool)
	// TailscaleLive — пришёл ли хотя бы один кадр и не оборван ли стрим;
	// UI по нему отличает «ядро молчит» от «стрима нет».
	TailscaleLive() bool
}

// TailscaleTarget — цель ядра (CoreTarget): та же, что у пулов и цепочек.
type TailscaleTarget = CoreTarget

// TailscaleIn — цель области панели; у Remote — выбранная сейчас машина.
func TailscaleIn(scope services.ProxyScope) TailscaleTarget {
	return CoreIn(scope)
}

// tailscaleSource — источник статуса цели. Local глух к remote-override:
// только бэкенд своего ядра. Remote — транспорт машины из её ВЫБОРА
// (UIService.LxdMachineTransportFunc), а не то, что стоит в APIService: там
// при взгляде на вкладку Local стоит транспорт своего движка, а соединение с
// машиной принадлежит машине, не вкладке. Признак машины — выбор, а не режим
// бэкенда: на Windows-клиенте режим classic, а машина подключена.
func (ac *AppController) tailscaleSource(t TailscaleTarget) (tailscaleSource, bool) {
	if ac == nil {
		return nil, false
	}
	if t.Scope == services.ScopeRemote {
		tr, ok := ac.machineTransport(t)
		if !ok {
			return nil, false
		}
		src, ok := tr.(tailscaleSource)
		return src, ok
	}
	src, ok := ac.Backend().(tailscaleSource)
	return src, ok
}

// TailscaleAvailable — умеет ли источник цели отдавать статус tailnet.
// Только проверка типа: ленивый стрим удалённой машины не поднимается.
func (ac *AppController) TailscaleAvailable(t TailscaleTarget) bool {
	_, ok := ac.tailscaleSource(t)
	return ok
}

// TailscaleStatus — статус tailscale-endpoint'а по тегу у источника цели.
// ok=false — см. tailscaleSource.
func (ac *AppController) TailscaleStatus(t TailscaleTarget, tag string) (services.TailscaleStatus, bool) {
	if src, ok := ac.tailscaleSource(t); ok {
		return src.TailscaleStatus(tag)
	}
	return services.TailscaleStatus{}, false
}

// TailscaleLive — жив ли стрим статуса у источника цели.
func (ac *AppController) TailscaleLive(t TailscaleTarget) bool {
	if src, ok := ac.tailscaleSource(t); ok {
		return src.TailscaleLive()
	}
	return false
}

// TailscaleCoreRunning — работает ли ядро цели, чей статус tailnet
// показывается. Local — RunningState своего ядра. Remote — живость стрима
// машины: StartedService отдаёт его только работающему ядру, а при остановке
// или Deploy стрим рвётся вместе с инстансом (lxd_remote_transport.go,
// runResilientStream). Отдельного опроса здоровья машины ради этого не нужно.
func (ac *AppController) TailscaleCoreRunning(t TailscaleTarget) bool {
	if ac == nil {
		return false
	}
	if t.Scope == services.ScopeRemote {
		return ac.TailscaleLive(t)
	}
	return ac.RunningState != nil && ac.RunningState.IsRunning()
}

// tailscaleController — источник, умеющий команды tailnet (SPEC 148):
// выбор exit node на ходу, выход из аккаунта, проверка устройства. Умеют оба
// gRPC-пути: локальный демон и транспорт удалённой машины.
type tailscaleController interface {
	TailscaleSetExitNode(tag, stableID string) error
	TailscaleLogout(tag string) error
	TailscalePing(ctx context.Context, tag, peerIP string, onReply func(services.TailscalePingResult)) error
}

// errTailscaleNoControl — у источника цели нет команд tailnet.
var errTailscaleNoControl = errors.New("tailnet commands are not available for this core")

// tailscaleControl — контроллер ТОГО ЖЕ источника, что отдаёт статус этой
// области: команда не должна уйти в другое ядро, чем то, чьё состояние на
// экране.
func (ac *AppController) tailscaleControl(t TailscaleTarget) (tailscaleController, bool) {
	src, ok := ac.tailscaleSource(t)
	if !ok {
		return nil, false
	}
	c, ok := src.(tailscaleController)
	return c, ok
}

// TailscaleControlAvailable — есть ли команды tailnet у источника цели.
func (ac *AppController) TailscaleControlAvailable(t TailscaleTarget) bool {
	_, ok := ac.tailscaleControl(t)
	return ok
}

// TailscaleSetExitNode — выбор exit node на ходу; "" снимает выход.
func (ac *AppController) TailscaleSetExitNode(t TailscaleTarget, tag, stableID string) error {
	c, ok := ac.tailscaleControl(t)
	if !ok {
		return errTailscaleNoControl
	}
	return c.TailscaleSetExitNode(tag, stableID)
}

// TailscaleLogout — выход узла из аккаунта tailnet.
func (ac *AppController) TailscaleLogout(t TailscaleTarget, tag string) error {
	c, ok := ac.tailscaleControl(t)
	if !ok {
		return errTailscaleNoControl
	}
	return c.TailscaleLogout(tag)
}

// TailscalePing — проверка устройства tailnet, ответы в onReply.
func (ac *AppController) TailscalePing(ctx context.Context, t TailscaleTarget, tag, peerIP string, onReply func(services.TailscalePingResult)) error {
	c, ok := ac.tailscaleControl(t)
	if !ok {
		return errTailscaleNoControl
	}
	return c.TailscalePing(ctx, tag, peerIP, onReply)
}
