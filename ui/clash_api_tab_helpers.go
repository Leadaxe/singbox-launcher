package ui

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"

	"singbox-launcher/api"
	"singbox-launcher/core"
)

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

// mergePreservingOrder накладывает свежий ответ машины на список, который уже
// стоит на экране, сохраняя ПОРЯДОК СТРОК и локальные пинги.
//
// Нужна авто-обновлению (раз в 5 секунд): пересобирать список из ответа как
// делает ручной Refresh нельзя. Порядок в ответе ядра не стабилен, и строка
// уезжала бы из-под курсора между наведением и кликом; пинги же машина не
// знает вовсе — они меряются локально и в ответе их нет, так что взятые из
// ответа нули стирали бы измеренное.
//
// Что реально применяется с машины: состав (пришедшие/ушедшие узлы), выбранный
// в группе узел (Now) и метаданные строки. Ушедшие узлы выпадают, новые
// дописываются в конец — так они заметны и не сдвигают уже видимые строки.
func mergePreservingOrder(current, fresh []api.ProxyInfo) []api.ProxyInfo {
	if len(current) == 0 {
		return fresh
	}
	byName := make(map[string]api.ProxyInfo, len(fresh))
	for _, p := range fresh {
		byName[p.Name] = p
	}

	result := make([]api.ProxyInfo, 0, len(fresh))
	kept := make(map[string]struct{}, len(fresh))
	// Сначала — узлы в том порядке, в каком они уже на экране.
	for _, old := range current {
		updated, ok := byName[old.Name]
		if !ok {
			// Узла больше нет на машине — выпадает из списка.
			continue
		}
		// Пинг локальный: он есть только у нас, из ответа его брать нечего.
		updated.Delay = old.Delay
		result = append(result, updated)
		kept[old.Name] = struct{}{}
	}
	// Затем — появившиеся узлы, в конец.
	for _, p := range fresh {
		if _, ok := kept[p.Name]; ok {
			continue
		}
		result = append(result, p)
	}
	return result
}
