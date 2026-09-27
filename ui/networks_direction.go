// File networks_direction.go — псевдо-направление NETWORKS на вкладке
// выбора узла (SPEC 148 §2, LxBox §579): узлы Tailscale без `exit_node`,
// которые не входят ни в одну группу выбора.
//
// Только вид: в конфиг и в состояние не пишется, выбора и замера нет. Состав
// считается из собранного config.json, по которому работает ядро.
package ui

import (
	"image/color"
	"os"
	"strings"
	"time"

	"fyne.io/fyne/v2/theme"

	"singbox-launcher/api"
	"singbox-launcher/core"
	"singbox-launcher/core/config"
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

// watchNetworksDirection раз в секунду отдаёт состав NETWORKS: пусто, когда
// ядро не работает. config.json перечитывается только при смене размера или
// времени изменения. Живёт всё время работы приложения, как и панель.
func watchNetworksDirection(ac *core.AppController, onTick func(tags []string)) {
	var (
		lastMod  time.Time
		lastSize int64
		cached   []string
	)
	ticker := time.NewTicker(networksPollInterval)
	defer ticker.Stop()
	for range ticker.C {
		if platform.IsSleeping() || ac == nil || ac.FileService == nil || ac.RunningState == nil {
			continue
		}
		var tags []string
		// Без статуса tailnet (legacy-движок) состояние узла не узнать —
		// псевдо-направления нет.
		if ac.RunningState.IsRunning() && ac.TailscaleAvailable() {
			path := ac.FileService.ConfigPath
			if fi, err := os.Stat(path); err == nil {
				if !fi.ModTime().Equal(lastMod) || fi.Size() != lastSize {
					fresh, rerr := config.GetNetworksNodeTagsFromConfig(path)
					if rerr != nil {
						debuglog.DebugLog("networks: config not read: %v", rerr)
					} else {
						cached = fresh
						lastMod, lastSize = fi.ModTime(), fi.Size()
					}
				}
				tags = cached
			}
		}
		onTick(tags)
	}
}

// tailscaleHasNoExit — узел Tailscale локального ядра, у которого по
// состоянию ядра нет действующего exit node (SPEC 148 §8). Только Local:
// чтение статуса удалённой машины подняло бы её ленивый стрим ради строки
// списка.
func tailscaleHasNoExit(ac *core.AppController, scope services.ProxyScope, tag string) bool {
	if ac == nil || scope != services.ScopeLocal || ac.RunningState == nil || !ac.RunningState.IsRunning() {
		return false
	}
	st, ok := ac.TailscaleStatus(tag)
	return ok && st.BackendState == services.TailscaleStateRunning && st.ExitNode == nil
}
