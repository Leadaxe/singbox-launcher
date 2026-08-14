package services

import (
	"fmt"

	"singbox-launcher/api"
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
// OnProxySwitched) те же, что у SwitchProxy.
func (apiSvc *APIService) SwitchProxyVia(t ProxyTransport, group, proxyName string) error {
	if err := t.SwitchProxy(group, proxyName); err != nil {
		return fmt.Errorf("failed to switch proxy: %w", err)
	}
	apiSvc.SetActiveProxyName(proxyName)
	apiSvc.SetLastSelectedProxyForGroup(group, proxyName)
	if apiSvc.OnProxySwitched != nil {
		apiSvc.OnProxySwitched()
	}
	return nil
}

// wireTransport resolves the transport APIService itself should use: the
// override when set (daemon mode), else Clash HTTP with the current
// config.json endpoint. Returns an error when neither is available.
func (apiSvc *APIService) wireTransport() (ProxyTransport, error) {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	if apiSvc.transportOverride != nil {
		return apiSvc.transportOverride, nil
	}
	if !apiSvc.Enabled {
		return nil, fmt.Errorf("clash_api is disabled")
	}
	return ClashTransport{BaseURL: apiSvc.BaseURL, Token: apiSvc.Token}, nil
}
