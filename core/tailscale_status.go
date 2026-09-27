// File tailscale_status.go — состояние tailnet у работающего ядра (SPEC 130).
//
// Источник — стрим SubscribeTailscaleStatus демона: он отдаёт статус ВСЕХ
// tailscale-endpoint'ов разом, с EndpointTag, первое сообщение сразу после
// подписки, дальше по событиям IPN-шины. Стрим один на процесс и живёт всё
// время коннекта — не открывается на время окна: колонка на вкладке
// Endpoints обязана быть актуальной без открытого окна, а NeedsLogin — то,
// о чём хочется знать, не заглядывая внутрь (решение владельца 16.09.2026).
//
// Диспетчер выбирает источник по области панели (services.ProxyScope):
// Local — бэкенд своего ядра, Remote — транспорт выбранной машины.
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

// Источник выбирается по ОБЛАСТИ панели, а не по тому, что сейчас стоит в
// APIService: remote-override глобальный, и при подключённой машине окно узла
// панели Local читало бы статус роутера, а команда ушла бы туда же. Область
// Local глуха к override (только бэкенд своего ядра), Remote — только
// транспорт выбранной машины; бэкенд своего ядра Remote не описывает.
// Признак машины — override, а не режим бэкенда: на Windows-клиенте режим
// classic, а машина подключена.
func (ac *AppController) tailscaleSource(scope services.ProxyScope) (tailscaleSource, bool) {
	if ac == nil {
		return nil, false
	}
	if scope == services.ScopeRemote {
		if ac.APIService == nil {
			return nil, false
		}
		src, ok := ac.APIService.TransportOverride().(tailscaleSource)
		return src, ok
	}
	src, ok := ac.Backend().(tailscaleSource)
	return src, ok
}

// TailscaleAvailable — умеет ли источник области отдавать статус tailnet.
// Только проверка типа: ленивый стрим удалённой машины не поднимается.
func (ac *AppController) TailscaleAvailable(scope services.ProxyScope) bool {
	_, ok := ac.tailscaleSource(scope)
	return ok
}

// TailscaleStatus — статус tailscale-endpoint'а по тегу у источника области.
// ok=false — см. tailscaleSource.
func (ac *AppController) TailscaleStatus(scope services.ProxyScope, tag string) (services.TailscaleStatus, bool) {
	if src, ok := ac.tailscaleSource(scope); ok {
		return src.TailscaleStatus(tag)
	}
	return services.TailscaleStatus{}, false
}

// TailscaleLive — жив ли стрим статуса у источника области.
func (ac *AppController) TailscaleLive(scope services.ProxyScope) bool {
	if src, ok := ac.tailscaleSource(scope); ok {
		return src.TailscaleLive()
	}
	return false
}

// TailscaleCoreRunning — работает ли ядро области, чей статус tailnet
// показывается. Local — RunningState своего ядра. Remote — живость стрима
// машины: StartedService отдаёт его только работающему ядру, а при остановке
// или Deploy стрим рвётся вместе с инстансом (lxd_remote_transport.go,
// runResilientStream). Отдельного опроса здоровья машины ради этого не нужно.
func (ac *AppController) TailscaleCoreRunning(scope services.ProxyScope) bool {
	if ac == nil {
		return false
	}
	if scope == services.ScopeRemote {
		return ac.TailscaleLive(scope)
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

// errTailscaleNoControl — у источника области нет команд tailnet.
var errTailscaleNoControl = errors.New("tailnet commands are not available for this core")

// tailscaleControl — контроллер ТОГО ЖЕ источника, что отдаёт статус этой
// области: команда не должна уйти в другое ядро, чем то, чьё состояние на
// экране.
func (ac *AppController) tailscaleControl(scope services.ProxyScope) (tailscaleController, bool) {
	src, ok := ac.tailscaleSource(scope)
	if !ok {
		return nil, false
	}
	c, ok := src.(tailscaleController)
	return c, ok
}

// TailscaleControlAvailable — есть ли команды tailnet у источника области.
func (ac *AppController) TailscaleControlAvailable(scope services.ProxyScope) bool {
	_, ok := ac.tailscaleControl(scope)
	return ok
}

// TailscaleSetExitNode — выбор exit node на ходу; "" снимает выход.
func (ac *AppController) TailscaleSetExitNode(scope services.ProxyScope, tag, stableID string) error {
	c, ok := ac.tailscaleControl(scope)
	if !ok {
		return errTailscaleNoControl
	}
	return c.TailscaleSetExitNode(tag, stableID)
}

// TailscaleLogout — выход узла из аккаунта tailnet.
func (ac *AppController) TailscaleLogout(scope services.ProxyScope, tag string) error {
	c, ok := ac.tailscaleControl(scope)
	if !ok {
		return errTailscaleNoControl
	}
	return c.TailscaleLogout(tag)
}

// TailscalePing — проверка устройства tailnet, ответы в onReply.
func (ac *AppController) TailscalePing(ctx context.Context, scope services.ProxyScope, tag, peerIP string, onReply func(services.TailscalePingResult)) error {
	c, ok := ac.tailscaleControl(scope)
	if !ok {
		return errTailscaleNoControl
	}
	return c.TailscalePing(ctx, tag, peerIP, onReply)
}
