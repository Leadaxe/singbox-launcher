// Package netiface перечисляет сетевые интерфейсы машины для выбора
// исходящего аплинка (sing-box outbound.bind_interface / route.default_interface).
//
// Зачем отдельный пакет: список нужен и UI (выпадашка в Конфигураторе), и
// валидации сохранённого значения на сборке конфига. Держать его в UI значило
// бы, что headless-сборка не может проверить, существует ли выбранный
// интерфейс, и молча выпустит конфиг, который ядро отвергнет на старте.
package netiface

import (
	"fmt"
	"net"
	"sort"
	"strings"

	"singbox-launcher/internal/debuglog"
)

// Iface — интерфейс, пригодный на роль исходящего аплинка.
type Iface struct {
	// Name — системное имя (en0, eth0, Ethernet). Именно оно уезжает в конфиг.
	Name string
	// FriendlyName — человеческое имя сервиса, если платформа его отдаёт
	// («Wi-Fi», «USB 10/100/1000 LAN»). Пустое, если неизвестно.
	FriendlyName string
	// Addrs — юникастовые IP интерфейса, в порядке v4-затем-v6.
	Addrs []string
	// Up — интерфейс поднят (FlagUp && FlagRunning).
	Up bool

	// index — системный индекс интерфейса, только для стабильной сортировки.
	index int
}

// Label — подпись для выпадающего списка: «en0 — Wi-Fi (192.168.10.124)».
// Имя всегда идёт первым: это то, что реально уезжает в конфиг, и по нему
// пользователь сверяется с выводом ifconfig/ip.
func (i Iface) Label() string {
	var b strings.Builder
	b.WriteString(i.Name)
	if i.FriendlyName != "" && !strings.EqualFold(i.FriendlyName, i.Name) {
		b.WriteString(" — ")
		b.WriteString(i.FriendlyName)
	}
	if len(i.Addrs) > 0 {
		b.WriteString(" (")
		b.WriteString(i.Addrs[0])
		b.WriteString(")")
	}
	if !i.Up {
		b.WriteString(" [down]")
	}
	return b.String()
}

// tunnelPrefixes — юниксовые имена туннельных интерфейсов. Привязка аплинка к
// ним означала бы петлю: ядро отправляет исходящий пакет в собственный же TUN.
//
// Имя — только ОДИН из трёх признаков, и самый слабый: на Windows адаптер
// Wintun не называется ни singbox-tun0, ни Wintun. По логу реального
// пользователя (см. wintun_cleanup_windows_device.go) его NetConnectionID был
// «Подключение по локальной сети 2» — то есть неотличим от обычной сетевой
// карты. Поэтому ниже проверяются ещё флаг POINTOPOINT и собственный адрес TUN.
var tunnelPrefixes = []string{"utun", "tun", "tap", "ppp", "ipsec", "gif", "stf", "wg", "awg", "singbox"}

func hasTunnelName(name string) bool {
	n := strings.ToLower(name)
	for _, p := range tunnelPrefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

// tunSubnets — подсети, которые лаунчер отдаёт собственному TUN
// (tun_address / tun_address6 в wizard_template.json). Интерфейс с таким
// адресом — это наш туннель под любым именем, включая безымянный
// Wintun-адаптер на Windows.
//
// Совпадение по подсети, а не по точному адресу: пользователь мог сдвинуть
// tun_address в пределах той же сети, и /30 всё равно остаётся нашей.
var tunSubnets = []string{"172.16.0.0/30", "fdfe:dcba:9876::/126"}

// isTunnelAddr сообщает, принадлежит ли адрес подсети собственного TUN.
func isTunnelAddr(ip net.IP) bool {
	for _, cidr := range tunSubnets {
		_, netw, err := net.ParseCIDR(cidr)
		if err != nil || netw == nil {
			continue
		}
		if netw.Contains(ip) {
			return true
		}
	}
	return false
}

// isTunnel — три независимых признака туннеля. Имя ловит юниксовые utun/wg,
// адрес ловит наш собственный TUN на Windows, где имя ничего не говорит, а
// флаг POINTOPOINT — туннели без узнаваемого имени.
//
// SPEC 113-E: POINTOPOINT считается признаком туннеля ТОЛЬКО у интерфейса без
// адреса. Флаг несут не одни туннели: PPPoE и мобильные WAN-модемы — это
// точка-точка по своей природе, и они же бывают единственным аплинком машины.
// Отсекая их, List() прятала настоящий аплинк, а диагностика bind_interface
// объявляла его «без IP-адреса», хотя адрес у него был. Туннель без адреса
// ничего не теряет: аплинком он всё равно не годится.
func isTunnel(name string, flags net.Flags, addrs []net.IP) bool {
	if hasTunnelName(name) {
		return true
	}
	if flags&net.FlagPointToPoint != 0 && len(addrs) == 0 {
		return true
	}
	for _, ip := range addrs {
		if isTunnelAddr(ip) {
			return true
		}
	}
	return false
}

// List возвращает интерфейсы, пригодные на роль аплинка: не loopback, не
// туннель, с хотя бы одним юникастовым адресом. Поднятые идут первыми, внутри
// группы — в порядке системного индекса (на macOS/Linux он отражает порядок
// появления, что близко к ожиданиям пользователя).
//
// Интерфейс без адреса отбрасывается намеренно: воткнутый, но не получивший IP
// кабель — ровно тот случай, ради которого пользователь сюда и пришёл, и
// предлагать его как аплинк значит предлагать заведомо мёртвый маршрут.
func List() ([]Iface, error) {
	sys, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("enumerate interfaces: %w", err)
	}
	out := make([]Iface, 0, len(sys))
	for _, si := range sys {
		if si.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := si.Addrs()
		v4, v6 := []string{}, []string{}
		ips := make([]net.IP, 0, len(addrs))
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP == nil || ipn.IP.IsLinkLocalUnicast() {
				continue
			}
			ips = append(ips, ipn.IP)
			if ipn.IP.To4() != nil {
				v4 = append(v4, ipn.IP.String())
			} else {
				v6 = append(v6, ipn.IP.String())
			}
		}
		// Проверка туннеля — ПОСЛЕ сбора адресов: один из трёх её признаков
		// (собственная подсеть TUN) без них не вычисляется.
		if isTunnel(si.Name, si.Flags, ips) {
			continue
		}
		joined := append(v4, v6...)
		if len(joined) == 0 {
			continue
		}
		out = append(out, Iface{
			Name:         si.Name,
			FriendlyName: friendlyName(si.Name),
			Addrs:        joined,
			Up:           si.Flags&net.FlagUp != 0 && si.Flags&net.FlagRunning != 0,
			index:        si.Index,
		})
	}
	sort.SliceStable(out, func(a, b int) bool {
		if out[a].Up != out[b].Up {
			return out[a].Up
		}
		return out[a].index < out[b].index
	})
	return out, nil
}

// FromRemote собирает Iface из данных, снятых на ДРУГОЙ машине (демон lxd
// отдаёт имя, флаг up и список адресов строками).
//
// Отбор тот же, что для локальных: демон присылает всё подряд, включая lo и
// туннели, и оговаривает, что фильтрация — задача вызывающего. Дублировать
// правила на стороне UI нельзя: разъехавшись, они начнут предлагать для
// роутера то, что для своей машины запрещено.
//
// ok=false означает «не годится в аплинки» — loopback, туннель или нет адреса.
func FromRemote(name string, up bool, addrs []string) (Iface, bool) {
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, "lo") || strings.EqualFold(name, "lo0") {
		return Iface{}, false
	}
	v4, v6 := []string{}, []string{}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		// Адрес приходит строкой и может нести префикс ("192.168.1.1/24").
		s := strings.TrimSpace(a)
		if i := strings.IndexByte(s, '/'); i >= 0 {
			s = s[:i]
		}
		ip := net.ParseIP(s)
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		ips = append(ips, ip)
		if ip.To4() != nil {
			v4 = append(v4, ip.String())
		} else {
			v6 = append(v6, ip.String())
		}
	}
	// Флагов чужой машины у нас нет — POINTOPOINT не проверить, остаются имя
	// и адрес. Для linux-роутера имени достаточно: там туннели называются
	// предсказуемо (tun/wg/lxd-tun0 ловится префиксом "tun"/"wg"/…).
	if isTunnel(name, 0, ips) {
		return Iface{}, false
	}
	joined := append(v4, v6...)
	if len(joined) == 0 {
		return Iface{}, false
	}
	return Iface{Name: name, Addrs: joined, Up: up}, true
}

// ListOrEmpty — List без ошибки: перечисление интерфейсов падает только при
// поломке системного стека, и в UI это должно означать пустой список, а не
// пустую вкладку настроек.
func ListOrEmpty() []Iface {
	list, err := List()
	if err != nil {
		debuglog.WarnLog("netiface: enumerate failed: %v", err)
		return nil
	}
	return list
}

// Names — только системные имена, в том же порядке, что List.
func Names() []string {
	list, err := List()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(list))
	for _, i := range list {
		names = append(names, i.Name)
	}
	return names
}

// Unfitness — почему существующий интерфейс не годится в аплинки.
//
// SPEC 113-E: раньше вызывающие знали ровно два состояния — «в списке» и «нет
// вовсе», — и всё остальное объявляли отсутствием IP-адреса. Для туннеля это
// прямая ложь: адрес у него есть, чинить нечего, а совет «получите адрес»
// уводит не туда. Диагностика обязана говорить по фактам.
type Unfitness int

const (
	// UnfitUnknown — интерфейса с таким именем на машине нет.
	UnfitUnknown Unfitness = iota
	// UnfitFit — интерфейс годится (есть в List).
	UnfitFit
	// UnfitLoopback — петля: трафик наружу через неё не пойдёт по определению.
	UnfitLoopback
	// UnfitTunnel — туннель (в том числе собственный TUN лаунчера): привязка
	// аплинка к нему замкнула бы ядро само на себя.
	UnfitTunnel
	// UnfitNoAddress — интерфейс есть и не туннель, но адреса у него нет.
	UnfitNoAddress
)

// Fitness сообщает, годится ли интерфейс в аплинки, а если нет — по какой
// причине. Смотрит на систему СЕЙЧАС, тем же фильтром, что List.
func Fitness(name string) Unfitness {
	name = strings.TrimSpace(name)
	if name == "" {
		return UnfitUnknown
	}
	sys, err := net.Interfaces()
	if err != nil {
		return UnfitUnknown
	}
	for _, si := range sys {
		if !strings.EqualFold(si.Name, name) {
			continue
		}
		if si.Flags&net.FlagLoopback != 0 {
			return UnfitLoopback
		}
		addrs, _ := si.Addrs()
		ips := make([]net.IP, 0, len(addrs))
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP == nil || ipn.IP.IsLinkLocalUnicast() {
				continue
			}
			ips = append(ips, ipn.IP)
		}
		// Порядок тот же, что в List: туннель проверяется по собранным
		// адресам, иначе признак «наша подсеть TUN» не вычислить.
		if isTunnel(si.Name, si.Flags, ips) {
			return UnfitTunnel
		}
		if len(ips) == 0 {
			return UnfitNoAddress
		}
		return UnfitFit
	}
	return UnfitUnknown
}

// Exists сообщает, есть ли на машине интерфейс с таким именем СЕЙЧАС —
// без фильтра по пригодности, чтобы валидация отличала «интерфейса нет вовсе»
// от «есть, но без адреса».
func Exists(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	sys, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, si := range sys {
		if strings.EqualFold(si.Name, name) {
			return true
		}
	}
	return false
}
