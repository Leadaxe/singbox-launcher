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

	// IsTunnel — это ЧУЖОЙ туннель (системный WireGuard/AmneziaWG, IPsec,
	// utun чужого клиента) с собственным юникастовым адресом.
	//
	// SPEC 113-F: такой интерфейс — законный аплинк. Привязка к нему значит
	// «выйти в интернет через уже поднятый туннель», и ровно этого хочет
	// пользователь, у которого на роутере поднят awg1. Петля возможна только
	// через СОБСТВЕННЫЙ TUN ядра sing-box — он в список не попадает вовсе.
	//
	// Флаг несёт не запрет, а предупреждение: выбор честный, но последствие
	// нетривиальное (трафик уйдёт в чужой туннель, а не в физическую сеть), и
	// подпись под полем обязана это сказать.
	IsTunnel bool

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

// tunnelPrefixes — юниксовые имена туннельных интерфейсов.
//
// SPEC 113-F: имя туннеля НЕ означает запрета. Системный WireGuard/AmneziaWG
// (wg0, awg1 на роутере), IPsec, чужой utun — это уже поднятый аплинк, и
// привязка к нему легальна: трафик ядра уйдёт наружу через чужой туннель.
// Петля возможна только через СОБСТВЕННЫЙ TUN ядра sing-box, а его ловят
// другие признаки (собственная подсеть TUN и имя из конфига).
//
// Имя здесь нужно ровно для двух вещей: пометить чужой туннель флагом
// IsTunnel (чтобы подпись честно предупредила о последствии) и отсечь
// туннель БЕЗ адреса — мёртвый маршрут.
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

// isOwnTun — это ТОТ САМЫЙ TUN, который поднимает наше ядро. Привязка аплинка
// к нему = петля: ядро отправило бы исходящий пакет в собственный же вход.
//
// Два признака, и оба нужны. Адрес из tunSubnets ловит безымянный
// Wintun-адаптер на Windows: по логу реального пользователя его
// NetConnectionID был «Подключение по локальной сети 2» — неотличим от обычной
// сетевой карты (см. wintun_cleanup_windows_device.go). Имя из конфига ловит
// обратный случай — TUN, которому пользователь сдвинул адрес за пределы нашей
// /30, но оставил штатное имя (singbox-tun0 / lxd-tun0).
func isOwnTun(name string, addrs []net.IP) bool {
	if isOwnTunName(name) {
		return true
	}
	for _, ip := range addrs {
		if isTunnelAddr(ip) {
			return true
		}
	}
	return false
}

// isDeadTunnel — туннель, через который трафик не пойдёт: адреса нет.
//
// SPEC 113-E: POINTOPOINT считается признаком туннеля ТОЛЬКО у интерфейса без
// адреса. Флаг несут не одни туннели: PPPoE и мобильные WAN-модемы — это
// точка-точка по своей природе, и они же бывают единственным аплинком машины.
// Отсекая их, List() прятала настоящий аплинк, а диагностика bind_interface
// объявляла его «без IP-адреса», хотя адрес у него был.
//
// Туннель по имени, но без адреса, отсекается по той же причине, что и любой
// безадресный интерфейс: предлагать его значит предлагать мёртвый маршрут.
func isDeadTunnel(name string, flags net.Flags, addrs []net.IP) bool {
	if len(addrs) > 0 {
		return false
	}
	return hasTunnelName(name) || flags&net.FlagPointToPoint != 0
}

// isForeignTunnel — ЧУЖОЙ туннель с адресом: законный аплинк, но с пометкой.
//
// Собственный TUN сюда не попадает — его проверяют раньше и отсекают.
func isForeignTunnel(name string, addrs []net.IP) bool {
	return len(addrs) > 0 && hasTunnelName(name)
}

// List возвращает интерфейсы, пригодные на роль аплинка: не loopback, не
// собственный TUN ядра, с хотя бы одним юникастовым адресом. Поднятые идут
// первыми, внутри группы — в порядке системного индекса (на macOS/Linux он
// отражает порядок появления, что близко к ожиданиям пользователя).
//
// SPEC 113-F: ЧУЖОЙ туннель с адресом остаётся в списке и помечается
// IsTunnel. Прежде утунели резались скопом, и пользователь с поднятым awg1 не
// мог выбрать его аплинком — хотя это ровно то, чего он хотел.
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
		// Проверки — ПОСЛЕ сбора адресов: и «наша подсеть TUN», и «туннель без
		// адреса» без них не вычисляются.
		if isOwnTun(si.Name, ips) || isDeadTunnel(si.Name, si.Flags, ips) {
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
			IsTunnel:     isForeignTunnel(si.Name, ips),
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
// ok=false означает «не годится в аплинки» — loopback, собственный TUN демона
// или нет адреса. Чужой туннель роутера (awg1, wg0 с адресом) годится и
// приезжает с IsTunnel=true: на роутере это самый частый осмысленный выбор.
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
	// и адрес. Собственный TUN демона (lxd-tun0 в подсети 172.16.0.0/30) ловят
	// оба признака; чужой туннель роутера остаётся с пометкой.
	if isOwnTun(name, ips) || isDeadTunnel(name, 0, ips) {
		return Iface{}, false
	}
	joined := append(v4, v6...)
	if len(joined) == 0 {
		return Iface{}, false
	}
	return Iface{Name: name, Addrs: joined, Up: up, IsTunnel: isForeignTunnel(name, ips)}, true
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
	// UnfitTunnel — СОБСТВЕННЫЙ TUN ядра или туннель без адреса. Первый
	// замкнул бы ядро само на себя, через второй трафик не пойдёт вовсе.
	//
	// SPEC 113-F: чужой туннель сюда больше не попадает — для него есть
	// UnfitFitTunnel. Прежде оба состояния звались одинаково, и подпись
	// «ядро не может выйти через него наружу» врала пользователю с поднятым
	// системным awg1: как раз может, и именно этого он и хотел.
	UnfitTunnel
	// UnfitNoAddress — интерфейс есть и не туннель, но адреса у него нет.
	UnfitNoAddress
	// UnfitFitTunnel — ГОДИТСЯ, но это чужой туннель с адресом.
	//
	// Подвид пригодности, а не отказа: выбор законный и ядро через него
	// выйдет. Отдельное значение нужно подписи — последствие нетривиальное
	// («трафик уйдёт в этот туннель, а не в физическую сеть»), и молчать о нём
	// значит оставить пользователя гадать, почему адрес на выходе чужой.
	UnfitFitTunnel
)

// Fit сообщает, годится ли интерфейс в аплинки. Оба «годных» состояния
// (обычный интерфейс и чужой туннель) отвечают да — иначе каждый вызывающий
// перечислял бы их сам и однажды забыл бы одно.
func (u Unfitness) Fit() bool {
	return u == UnfitFit || u == UnfitFitTunnel
}

// Fitness сообщает, годится ли интерфейс в аплинки, а если нет — по какой
// причине. Смотрит на систему СЕЙЧАС, тем же фильтром, что List.
//
// Два «годных» исхода: UnfitFit — обычный интерфейс, UnfitFitTunnel — чужой
// туннель с адресом. Оба попадают в List, и проверять пригодность нужно через
// Unfitness.Fit(), а не сравнением с UnfitFit: иначе законный туннель
// объявился бы негодным в одном месте и предлагался в другом.
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
		// Порядок тот же, что в List: проверки идут по собранным адресам,
		// иначе ни «наша подсеть TUN», ни «туннель без адреса» не вычислить.
		if isOwnTun(si.Name, ips) || isDeadTunnel(si.Name, si.Flags, ips) {
			return UnfitTunnel
		}
		if len(ips) == 0 {
			return UnfitNoAddress
		}
		if isForeignTunnel(si.Name, ips) {
			return UnfitFitTunnel
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
