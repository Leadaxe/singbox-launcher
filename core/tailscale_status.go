// File tailscale_status.go — состояние tailnet у работающего ядра (SPEC 130).
//
// Источник — стрим SubscribeTailscaleStatus демона: он отдаёт статус ВСЕХ
// tailscale-endpoint'ов разом, с EndpointTag, первое сообщение сразу после
// подписки, дальше по событиям IPN-шины. Стрим один на процесс и живёт всё
// время коннекта — не открывается на время окна: колонка на вкладке
// Endpoints обязана быть актуальной без открытого окна, а NeedsLogin — то,
// о чём хочется знать, не заглядывая внутрь (решение владельца 16.09.2026).
//
// Диспетчер повторяет схему ChainsAvailable/ChainFor: сначала транспорт
// удалённой машины (remote-override), потом бэкенд. Конвертер pb → доменный
// тип ОДИН на оба пути — локальный демон и роутер обязаны давать одинаковый
// диагноз (тот же довод, что у chainInfosFromPB).
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

// TailscaleAvailable — умеет ли текущий источник отдавать статус tailnet.
// Схема ChainsAvailable: сначала транспорт удалённой машины, потом бэкенд.
func (ac *AppController) TailscaleAvailable() bool {
	if ac.APIService != nil {
		if _, ok := ac.APIService.TransportOverride().(tailscaleSource); ok {
			return true
		}
	}
	_, ok := ac.Backend().(tailscaleSource)
	return ok
}

// TailscaleStatus — статус tailscale-endpoint'а по тегу у текущего
// источника. ok=false — см. tailscaleSource.
func (ac *AppController) TailscaleStatus(tag string) (services.TailscaleStatus, bool) {
	if src, ok := ac.tailscaleSource(); ok {
		return src.TailscaleStatus(tag)
	}
	return services.TailscaleStatus{}, false
}

// TailscaleLive — жив ли стрим статуса у текущего источника.
func (ac *AppController) TailscaleLive() bool {
	if src, ok := ac.tailscaleSource(); ok {
		return src.TailscaleLive()
	}
	return false
}

func (ac *AppController) tailscaleSource() (tailscaleSource, bool) {
	if ac.APIService != nil {
		if src, ok := ac.APIService.TransportOverride().(tailscaleSource); ok {
			return src, true
		}
	}
	src, ok := ac.Backend().(tailscaleSource)
	return src, ok
}

// tailscaleController — источник, умеющий команды tailnet (SPEC 148):
// выбор exit node на ходу, выход из аккаунта, проверка устройства. Сегодня —
// только локальный демон; удалённая машина отдаёт статус без команд.
type tailscaleController interface {
	TailscaleSetExitNode(tag, stableID string) error
	TailscaleLogout(tag string) error
	TailscalePing(ctx context.Context, tag, peerIP string, onReply func(services.TailscalePingResult)) error
}

// errTailscaleNoControl — у текущего источника нет команд tailnet.
var errTailscaleNoControl = errors.New("tailnet commands are not available for this core")

// tailscaleControl — контроллер ТОГО ЖЕ источника, что отдаёт статус:
// команда не должна уйти в другое ядро, чем то, чьё состояние на экране.
func (ac *AppController) tailscaleControl() (tailscaleController, bool) {
	src, ok := ac.tailscaleSource()
	if !ok {
		return nil, false
	}
	c, ok := src.(tailscaleController)
	return c, ok
}

// TailscaleControlAvailable — есть ли команды tailnet у текущего источника.
func (ac *AppController) TailscaleControlAvailable() bool {
	_, ok := ac.tailscaleControl()
	return ok
}

// TailscaleSetExitNode — выбор exit node на ходу; "" снимает выход.
func (ac *AppController) TailscaleSetExitNode(tag, stableID string) error {
	c, ok := ac.tailscaleControl()
	if !ok {
		return errTailscaleNoControl
	}
	return c.TailscaleSetExitNode(tag, stableID)
}

// TailscaleLogout — выход узла из аккаунта tailnet.
func (ac *AppController) TailscaleLogout(tag string) error {
	c, ok := ac.tailscaleControl()
	if !ok {
		return errTailscaleNoControl
	}
	return c.TailscaleLogout(tag)
}

// TailscalePing — проверка устройства tailnet, ответы в onReply.
func (ac *AppController) TailscalePing(ctx context.Context, tag, peerIP string, onReply func(services.TailscalePingResult)) error {
	c, ok := ac.tailscaleControl()
	if !ok {
		return errTailscaleNoControl
	}
	return c.TailscalePing(ctx, tag, peerIP, onReply)
}
