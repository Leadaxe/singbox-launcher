package ui

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"singbox-launcher/api"
	"singbox-launcher/core"
)

// serversListRowScrollbarGutterWidth — отступ справа внутри каждой строки списка прокси (после кнопок),
// чтобы полоса прокрутки списка не наезжала на Ping / Switch (а не поле снаружи скролла).
const serversListRowScrollbarGutterWidth = 10

// keyModifiers returns held keyboard modifiers (desktop); 0 on mobile or if driver has no support.
func keyModifiers() fyne.KeyModifier {
	d, ok := fyne.CurrentApp().Driver().(desktop.Driver)
	if !ok {
		return 0
	}
	return d.CurrentKeyModifiers()
}

// clashAPITestMaxAttempts / clashAPITestRetryInterval — повторы GET /version при проверке Clash API:
// диалог об ошибке только после исчерпания попыток (см. onTestAPIConnection).
const (
	clashAPITestMaxAttempts   = 5
	clashAPITestRetryInterval = 5 * time.Second
)

var pingAllConcurrencyOptions = []string{"1", "5", "10", "20", "50", "100"}

// reorderWithPinned moves special proxies to the top of the list while
// preserving relative order of the rest:
//   - "direct-out" (if present)
//   - currently active proxy (if set and different from direct-out)
func reorderWithPinned(ac *core.AppController, list []api.ProxyInfo) []api.ProxyInfo {
	if len(list) == 0 {
		return list
	}
	const directName = "direct-out"
	activeName := ac.GetActiveProxyName()

	hasDirect := false
	hasActive := false
	for i := range list {
		if list[i].Name == directName {
			hasDirect = true
		}
		if activeName != "" && list[i].Name == activeName {
			hasActive = true
		}
	}
	if !hasDirect && (!hasActive || activeName == "") {
		return list
	}

	result := make([]api.ProxyInfo, 0, len(list))
	used := make(map[string]struct{}, 2)

	if hasDirect {
		for i := range list {
			if list[i].Name == directName {
				result = append(result, list[i])
				used[directName] = struct{}{}
				break
			}
		}
	}
	if hasActive && activeName != directName {
		for i := range list {
			if list[i].Name == activeName {
				result = append(result, list[i])
				used[activeName] = struct{}{}
				break
			}
		}
	}
	for i := range list {
		if _, ok := used[list[i].Name]; ok {
			continue
		}
		result = append(result, list[i])
	}
	return result
}

// proxyClashTypeSkippedForShareExport skips selector/urltest/direct (routing outbounds), not leaf share links.
func proxyClashTypeSkippedForShareExport(p api.ProxyInfo) bool {
	switch strings.ToLower(strings.TrimSpace(p.ClashType)) {
	case "selector", "urltest", "direct":
		return true
	default:
		return false
	}
}
