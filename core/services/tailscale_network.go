// File tailscale_network.go — чистая логика вкладки Network узла Tailscale
// (SPEC 148): сравнение записанного и действующего exit node, порядок
// устройств, результат проверки устройства. Без UI и без сети — чтобы
// покрываться тестом.
package services

import (
	"sort"
	"strings"
)

// ExitNodeDiff — как соотносятся записанный в теле узла `exit_node` и
// действующий выход по состоянию ядра (LxBox §581 раздел 5).
type ExitNodeDiff int

const (
	// ExitNodeSame — совпадают (оба пусты либо указывают на одно устройство):
	// знака нет, кнопки записи нет.
	ExitNodeSame ExitNodeDiff = iota
	// ExitNodeSelectedNotSaved — в узле выхода нет, на ходу выбран.
	ExitNodeSelectedNotSaved
	// ExitNodeClearedNotSaved — в узле выход записан, на ходу снят.
	ExitNodeClearedNotSaved
	// ExitNodeOtherNotSaved — записан один, на ходу выбран другой.
	ExitNodeOtherNotSaved
)

// CompareExitNode сравнивает записанное значение `exit_node` с действующим
// выходом. Ядро принимает в конфиге IP либо имя устройства
// (ipn.MaskedPrefs.SetExitNodeIP), поэтому совпадением считается любое из:
// адрес tailnet, HostName, DNS-имя (с точкой в конце и без).
func CompareExitNode(written string, effective *TailscalePeer) ExitNodeDiff {
	written = strings.TrimSpace(written)
	switch {
	case written == "" && effective == nil:
		return ExitNodeSame
	case written == "":
		return ExitNodeSelectedNotSaved
	case effective == nil:
		return ExitNodeClearedNotSaved
	case PeerMatchesExitValue(*effective, written):
		return ExitNodeSame
	}
	return ExitNodeOtherNotSaved
}

// PeerMatchesExitValue — указывает ли значение `exit_node` на это устройство.
func PeerMatchesExitValue(p TailscalePeer, value string) bool {
	v := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if v == "" {
		return false
	}
	for _, ip := range p.TailscaleIPs {
		if strings.EqualFold(ip, v) {
			return true
		}
	}
	if strings.EqualFold(p.HostName, v) {
		return true
	}
	dns := strings.ToLower(strings.TrimSuffix(p.DNSName, "."))
	if dns != "" && (dns == v || strings.SplitN(dns, ".", 2)[0] == v) {
		return true
	}
	return false
}

// ExitNodeValueFor — что писать в `exit_node` для выбранного устройства:
// первый адрес tailnet (однозначен и не зависит от MagicDNS), иначе имя.
func ExitNodeValueFor(p TailscalePeer) string {
	if len(p.TailscaleIPs) > 0 && p.TailscaleIPs[0] != "" {
		return p.TailscaleIPs[0]
	}
	return p.HostName
}

// ExitNodeOptions — устройства, годные выходом (`ExitNodeOption`), по имени.
func ExitNodeOptions(st TailscaleStatus) []TailscalePeer {
	var out []TailscalePeer
	for _, g := range st.UserGroups {
		for _, p := range g.Peers {
			if p.ExitNodeOption {
				out = append(out, p)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].HostName) < strings.ToLower(out[j].HostName)
	})
	return out
}

// SortDevices — порядок блока Devices: сначала устройства в сети, затем
// остальные; внутри по имени без учёта регистра. Вход не меняется.
func SortDevices(peers []TailscalePeer) []TailscalePeer {
	out := append([]TailscalePeer(nil), peers...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Online != out[j].Online {
			return out[i].Online
		}
		return strings.ToLower(out[i].HostName) < strings.ToLower(out[j].HostName)
	})
	return out
}

// TailscalePingResult — один ответ проверки устройства (StartTailscalePing).
type TailscalePingResult struct {
	LatencyMs      float64
	IsDirect       bool
	Endpoint       string
	DERPRegionCode string
	Error          string
}

// TailscalePingMaxReplies — проверка идёт до пяти ответов (LxBox §581 §7).
const TailscalePingMaxReplies = 5
