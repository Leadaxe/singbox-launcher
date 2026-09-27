// File networks_direction.go — псевдо-направление NETWORKS на вкладке
// выбора узла (SPEC 148 §2, LxBox §579): узлы Tailscale без `exit_node`,
// которые не входят ни в одну группу выбора.
//
// Только вид: в конфиг и в состояние не пишется, выбора и замера нет. Состав
// считается из собранного config.json, по которому работает ядро.
package ui

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2/theme"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/platform"
)

// networksTone — цвет слова состояния в строке NETWORKS.
type networksTone int

const (
	networksToneNeutral networksTone = iota
	networksToneOK
	networksToneWarn
)

// networksRowState — слово на месте задержки (LxBox §579 раздел 3).
// Пустая строка — VPN выключен, состояния нет.
func networksRowState(running bool, st services.TailscaleStatus, ok bool) (string, networksTone) {
	if !running {
		return "", networksToneNeutral
	}
	if !ok {
		return locale.T("starting"), networksToneNeutral
	}
	switch st.BackendState {
	case services.TailscaleStateRunning:
		return locale.T("running"), networksToneOK
	case services.TailscaleStateNeedsLogin:
		return locale.T("sign-in needed"), networksToneWarn
	case services.TailscaleStateStopped:
		return locale.T("stopped"), networksToneWarn
	}
	if t := strings.TrimSpace(st.StateText); t != "" {
		return t, networksToneNeutral
	}
	return st.BackendState, networksToneNeutral
}

func networksToneColor(t networksTone) color.Color {
	switch t {
	case networksToneOK:
		return theme.Color(theme.ColorNameSuccess)
	case networksToneWarn:
		return theme.Color(theme.ColorNameWarning)
	}
	return theme.Color(theme.ColorNamePlaceHolder)
}

// networksOptionLabel — строка пункта NETWORKS в дропдауне. Пункты Select —
// строки, и направление пользователя с тегом NETWORKS совпало бы с
// псевдо-направлением: при совпадении к пункту добавляется неразрывный
// пробел, на глаз строка та же, а выбор различим.
func networksOptionLabel(groups []string) string {
	label := config.NetworksDirectionName
	for taken := true; taken; {
		taken = false
		for _, g := range groups {
			if g == label {
				label += " "
				taken = true
				break
			}
		}
	}
	return label
}

// networksRows — строки списка NETWORKS: тег узла, протокол tailscale.
func networksRows(tags []string) []api.ProxyInfo {
	out := make([]api.ProxyInfo, 0, len(tags))
	for _, t := range tags {
		out = append(out, api.ProxyInfo{Name: t, ClashType: "Tailscale"}) // l10n-exempt: protocol name
	}
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// networksPollInterval — раз в секунду: состав NETWORKS и перерисовка
// состояний (поток ядра чаще раза в секунду экран не перерисовывает).
const networksPollInterval = time.Second

// watchNetworksDirection раз в секунду отдаёт состав NETWORKS области scope:
// пусто, когда ядро области не работает. Конфиг — тот, по которому работает
// ядро области (effectiveNodeConfigPath: у Remote собранный конфиг
// выбранной машины); перечитывается только при смене пути, размера или
// времени изменения. Живёт всё время работы приложения, как и панель.
//
// Состояние ядра спрашивается ПОСЛЕ отбора: у Remote оно поднимает ленивый
// стрим статуса машины, и без узлов Tailscale в её конфиге открывать его
// незачем.
func watchNetworksDirection(ac *core.AppController, scope services.ProxyScope, onTick func(tags []string)) {
	var (
		lastPath string
		lastMod  time.Time
		lastSize int64
		cached   []string
		lastWhy  string
	)
	ticker := time.NewTicker(networksPollInterval)
	defer ticker.Stop()
	for range ticker.C {
		if platform.IsSleeping() || ac == nil || ac.FileService == nil {
			continue
		}
		tags, why := networksTagsNow(ac, scope, &lastPath, &lastMod, &lastSize, &cached)
		// Причина пишется при смене, а не каждый тик: по журналу видно,
		// какое условие держит пункт NETWORKS скрытым.
		if why != lastWhy {
			lastWhy = why
			name := "local"
			if scope == services.ScopeRemote {
				name = "remote"
			}
			debuglog.DebugLog("networks (%s): %s", name, why)
		}
		onTick(tags)
	}
}

// networksTagsNow — один тик watchNetworksDirection: состав и причина, по
// которой он таков (для журнала).
func networksTagsNow(ac *core.AppController, scope services.ProxyScope, lastPath *string, lastMod *time.Time, lastSize *int64, cached *[]string) ([]string, string) {
	// Без статуса tailnet (legacy-движок, машина не выбрана) состояние узла
	// не узнать — псевдо-направления нет.
	target := core.TailscaleIn(scope)
	if !ac.TailscaleAvailable(target) {
		return nil, "no tailnet status source"
	}
	path := effectiveNodeConfigPath(ac, scope)
	fi, err := os.Stat(path)
	if err != nil {
		return nil, "config file is missing"
	}
	if path != *lastPath || !fi.ModTime().Equal(*lastMod) || fi.Size() != *lastSize {
		fresh, rerr := config.GetNetworksNodeTagsFromConfig(path)
		if rerr != nil {
			debuglog.DebugLog("networks: config not read: %v", rerr)
			if path != *lastPath {
				return nil, "config not read"
			}
		} else {
			*cached = fresh
			*lastPath, *lastMod, *lastSize = path, fi.ModTime(), fi.Size()
		}
	}
	if len(*cached) == 0 {
		return nil, "no tailscale nodes without an exit in the config"
	}
	if !ac.TailscaleCoreRunning(target) {
		return nil, "core is not running or sent no tailnet status yet"
	}
	return *cached, fmt.Sprintf("%d nodes shown", len(*cached))
}

// tailscaleHasNoExit — узел Tailscale ядра области, у которого по состоянию
// ядра нет действующего exit node (SPEC 148 §4). У Remote статус читается
// только для строки типа tailscale: чтение поднимает ленивый стрим машины, и
// ради строк без узлов Tailscale открывать его незачем.
func tailscaleHasNoExit(ac *core.AppController, scope services.ProxyScope, proxy api.ProxyInfo) bool {
	if ac == nil {
		return false
	}
	if scope == services.ScopeRemote && !strings.EqualFold(proxy.ClashType, configtypes.SchemeTailscale) {
		return false
	}
	target := core.TailscaleIn(scope)
	if !ac.TailscaleCoreRunning(target) {
		return false
	}
	st, ok := ac.TailscaleStatus(target, proxy.Name)
	return ok && st.BackendState == services.TailscaleStateRunning && st.ExitNode == nil
}
