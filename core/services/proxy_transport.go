package services

import (
	"fmt"

	"singbox-launcher/api"
	"singbox-launcher/internal/debuglog"
)

// ProxyTransport abstracts the wire used for proxy-group operations. The
// classic engine talks to the Clash HTTP API embedded in the running core;
// the daemon engine talks gRPC to the lxd daemon. APIService и Servers-tab
// ходят только через этот интерфейс, поэтому смена движка не трогает UI.
type ProxyTransport interface {
	// GroupProxies returns the proxies of a selector group plus the tag the
	// group has currently selected ("now").
	GroupProxies(group string) ([]api.ProxyInfo, string, error)
	// SwitchProxy selects a proxy inside a selector group.
	SwitchProxy(group, name string) error
	// Delay measures proxy latency in ms (URL-test against the ping URL).
	Delay(proxyName string) (int64, error)
}

// ClashTransport — классический транспорт поверх Clash HTTP API. Значения
// baseURL/token фиксируются на момент создания: caller (UI/APIService)
// строит транспорт непосредственно перед запросом, поэтому смена endpoint'а
// или override'а естественно даёт свежий транспорт.
type ClashTransport struct {
	BaseURL string
	Token   string
}

// NewClashTransport builds the classic Clash-HTTP transport.
func NewClashTransport(baseURL, token string) ClashTransport {
	return ClashTransport{BaseURL: baseURL, Token: token}
}

// GroupProxies implements ProxyTransport.
func (t ClashTransport) GroupProxies(group string) ([]api.ProxyInfo, string, error) {
	return api.GetProxiesInGroup(t.BaseURL, t.Token, group)
}

// SwitchProxy implements ProxyTransport.
func (t ClashTransport) SwitchProxy(group, name string) error {
	return api.SwitchProxy(t.BaseURL, t.Token, group, name)
}

// Delay implements ProxyTransport.
func (t ClashTransport) Delay(proxyName string) (int64, error) {
	return api.GetDelay(t.BaseURL, t.Token, proxyName)
}

// SetTransport устанавливает транспорт-override (daemon-режим). nil — вернуть
// классический Clash-путь.
func (apiSvc *APIService) SetTransport(t ProxyTransport) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	apiSvc.transportOverride = t
}

// TransportOverride возвращает установленный override (nil в classic-режиме).
func (apiSvc *APIService) TransportOverride() ProxyTransport {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	return apiSvc.transportOverride
}

// SwitchProxyVia переключает прокси через ЯВНО переданный транспорт, минуя
// wireTransport. Нужно для UI-слоя Servers-tab, где источник транспорта —
// EffectiveProxyTransport (учитывает remote-override SPEC 064, который
// APIService не видит). Побочные эффекты (active name, last-selected,
// OnProxySwitched) те же, что у SwitchProxy, но пишутся в область scope —
// панели, которая переключала, а не во вкладку, открытую к моменту ответа.
func (apiSvc *APIService) SwitchProxyVia(t ProxyTransport, scope ProxyScope, group, proxyName string) error {
	if err := t.SwitchProxy(group, proxyName); err != nil {
		debuglog.WarnLog("SwitchProxy: group=%q → %q failed: %v", group, proxyName, err)
		return fmt.Errorf("failed to switch proxy: %w", err)
	}
	// Логируем факт и адресата: без этого нельзя отличить «лаунчер не послал
	// команду» от «ядро её приняло, но соединения не разорвало» — а лечение у
	// этих случаев разное.
	debuglog.InfoLog("SwitchProxy: group=%q → %q", group, proxyName)
	apiSvc.SetActiveProxyNameIn(scope, proxyName)
	apiSvc.SetLastSelectedProxyForGroupIn(scope, group, proxyName)
	if apiSvc.OnProxySwitched != nil {
		apiSvc.OnProxySwitched()
	}
	return nil
}

// wireTransport resolves the transport APIService itself should use: the
// override when set (daemon mode), else Clash HTTP with the current
// config.json endpoint. Returns an error when neither is available.
func (apiSvc *APIService) wireTransport() (ProxyTransport, error) {
	t, _, err := apiSvc.wireTransportAndScope()
	return t, err
}

// wireTransportAndScope — wireTransport вместе с активной областью, снятые
// под ОДНОЙ блокировкой. Смена вкладки меняет обе величины (область и
// transportOverride), и ответ, полученный через этот транспорт, принадлежит
// именно этой области — даже если к моменту ответа на экране уже другая.
func (apiSvc *APIService) wireTransportAndScope() (ProxyTransport, ProxyScope, error) {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	if apiSvc.transportOverride != nil {
		return apiSvc.transportOverride, apiSvc.scope, nil
	}
	if !apiSvc.Enabled {
		return nil, apiSvc.scope, fmt.Errorf("clash_api is disabled")
	}
	return ClashTransport{BaseURL: apiSvc.BaseURL, Token: apiSvc.Token}, apiSvc.scope, nil
}
