package netiface

import (
	"net"
	"strings"
	"testing"
)

// SPEC 113-F: туннельное ИМЯ само по себе больше не отказ. Системный
// WireGuard/AmneziaWG — законный аплинк, и режется только мёртвый (без
// адреса) и собственный TUN ядра.
func TestDeadTunnelIsRejectedByName(t *testing.T) {
	for _, name := range []string{"utun0", "utun7", "tun0", "wg0", "awg1", "ppp0", "gif0", "stf0", "singbox-tun0"} {
		if !isDeadTunnel(name, 0, nil) {
			t.Errorf("isDeadTunnel(%q) = false, туннель без адреса — мёртвый маршрут", name)
		}
	}
	for _, name := range []string{"en0", "en9", "eth0", "Ethernet", "Wi-Fi", "bridge0"} {
		if isDeadTunnel(name, 0, nil) {
			t.Errorf("isDeadTunnel(%q) = true, обычный интерфейс отсеиваться не должен", name)
		}
	}
}

func TestDeadTunnelByPointToPointFlag(t *testing.T) {
	// Туннель без узнаваемого имени и без адреса ловится флагом.
	if !isDeadTunnel("weird0", net.FlagPointToPoint, nil) {
		t.Error("безадресный POINTOPOINT-интерфейс не распознан как мёртвый туннель")
	}
	if isDeadTunnel("weird0", net.FlagUp|net.FlagBroadcast, nil) {
		t.Error("обычный broadcast-интерфейс распознан как туннель")
	}
}

// SPEC 113-E: POINTOPOINT с адресом — это не туннель, а PPPoE или мобильный
// WAN-модем, и он бывает единственным аплинком машины. Раньше флаг рубил его
// из List() безусловно, и диагностика bind_interface объявляла его «без
// IP-адреса», хотя адрес у него был.
func TestPointToPointWithAddressIsNotTunnel(t *testing.T) {
	ip := []net.IP{net.ParseIP("100.64.7.9")}
	if isDeadTunnel("wwan0", net.FlagUp|net.FlagPointToPoint, ip) {
		t.Error("мобильный WAN с адресом отсеян как мёртвый туннель")
	}
	if isOwnTun("wwan0", ip) {
		t.Error("мобильный WAN опознан как собственный TUN ядра")
	}
	if isForeignTunnel("wwan0", ip) {
		t.Error("мобильный WAN помечен туннелем — имя у него не туннельное")
	}
	// Тот же интерфейс без адреса аплинком всё равно не годится — прежняя
	// ветка остаётся в силе.
	if !isDeadTunnel("wwan0", net.FlagUp|net.FlagPointToPoint, nil) {
		t.Error("POINTOPOINT без адреса обязан отсеиваться")
	}
}

// SPEC 113-F, суть решения: ЧУЖОЙ туннель с адресом — законный аплинк.
// Пользователь с поднятым системным awg1 на роутере хочет выйти именно через
// него, и прежний фильтр (резавший туннели скопом) отнимал у него эту
// возможность.
func TestForeignTunnelWithAddressIsAllowed(t *testing.T) {
	SetOwnTunNames()
	defer SetOwnTunNames()

	cases := []struct {
		name string
		addr string
	}{
		{"awg1", "10.7.0.2"},
		{"wg0", "10.9.0.3"},
		{"utun4", "100.64.1.5"},
		{"tun0", "10.8.0.6"},
	}
	for _, c := range cases {
		ips := []net.IP{net.ParseIP(c.addr)}
		if isOwnTun(c.name, ips) {
			t.Errorf("%q (%s) опознан как СОБСТВЕННЫЙ TUN — это чужой туннель", c.name, c.addr)
		}
		if isDeadTunnel(c.name, net.FlagUp|net.FlagPointToPoint, ips) {
			t.Errorf("%q (%s) отсеян как мёртвый, хотя адрес у него есть", c.name, c.addr)
		}
		if !isForeignTunnel(c.name, ips) {
			t.Errorf("%q (%s) не помечен туннелем — подпись промолчит о последствии", c.name, c.addr)
		}
	}
}

// Имя собственного TUN приходит из config.json и сравнивается ТОЧНО: префикс
// «tun» не разделяет наш singbox-tun0 и чужой tun0, и именно на этом прежний
// фильтр рубил чужие туннели.
func TestOwnTunNameSeparatesOursFromForeign(t *testing.T) {
	SetOwnTunNames("singbox-tun0")
	defer SetOwnTunNames()

	ours := []net.IP{net.ParseIP("10.55.0.1")} // адрес сдвинут за пределы tunSubnets
	if !isOwnTun("singbox-tun0", ours) {
		t.Error("собственный TUN не опознан по имени из конфига")
	}
	if !isOwnTun("SingBox-Tun0", ours) {
		t.Error("сравнение имени обязано игнорировать регистр")
	}
	// Чужой туннель, чьё имя лишь начинается похоже, остаётся законным.
	if isOwnTun("singbox-tun0-peer", ours) {
		t.Error("сравнение по префиксу вернулось — чужой туннель объявлен нашим")
	}
	if isOwnTun("tun0", ours) {
		t.Error("чужой tun0 объявлен собственным TUN ядра")
	}
	// Пустой реестр (конфига нет) никого не прячет.
	SetOwnTunNames()
	if isOwnTun("singbox-tun0", ours) {
		t.Error("пустой реестр всё ещё прячет интерфейс")
	}
	// Пустые строки в реестр не попадают: иначе они совпали бы с чем угодно.
	SetOwnTunNames("", "   ")
	if isOwnTun("", nil) || isOwnTun("en0", nil) {
		t.Error("пустое имя попало в реестр собственных TUN")
	}
}

// SPEC 113-E: диагностика обязана называть настоящую причину. Прежде всякий
// существующий, но не прошедший фильтр интерфейс объявлялся «без IP-адреса» —
// для туннеля и для петли это прямая ложь.
func TestFitnessSeparatesReasons(t *testing.T) {
	if got := Fitness(""); got != UnfitUnknown {
		t.Errorf(`Fitness("") = %v, ожидалось UnfitUnknown`, got)
	}
	if got := Fitness("definitely-no-such-iface-42"); got != UnfitUnknown {
		t.Errorf("Fitness(несуществующий) = %v, ожидалось UnfitUnknown", got)
	}
	// Петля есть на любой машине под любым из двух имён.
	loopbackSeen := false
	for _, name := range []string{"lo0", "lo"} {
		if !Exists(name) {
			continue
		}
		loopbackSeen = true
		if got := Fitness(name); got != UnfitLoopback {
			t.Errorf("Fitness(%q) = %v, ожидалось UnfitLoopback", name, got)
		}
	}
	if !loopbackSeen {
		t.Skip("на этой машине нет ни lo0, ни lo — сверять не с чем")
	}
	// Всё, что List() отдал, обязано числиться годным: два ответа на один
	// вопрос разъехались бы, и поле противоречило бы выпадающему списку.
	// Годных исходов два — обычный интерфейс и чужой туннель (SPEC 113-F).
	for _, ifc := range ListOrEmpty() {
		got := Fitness(ifc.Name)
		if !got.Fit() {
			t.Errorf("Fitness(%q) = %v, но List() его предлагает", ifc.Name, got)
		}
		// И классы обязаны совпадать: List пометил туннелем — Fitness обязан
		// сказать то же, иначе подпись под полем опишет не тот интерфейс.
		if want := ifc.IsTunnel; want != (got == UnfitFitTunnel) {
			t.Errorf("Fitness(%q) = %v, но List().IsTunnel = %v", ifc.Name, got, want)
		}
	}
}

// Классификация — единый список исходов, и Fit() обязан покрывать ровно
// «годные». Без пина новый Unfit* легко забыть добавить в Fit() (или наоборот
// добавить лишний), и половина кода начнёт считать интерфейс годным, а
// половина — нет.
func TestUnfitnessFitCoversBothFitStates(t *testing.T) {
	fit := map[Unfitness]bool{UnfitFit: true, UnfitFitTunnel: true}
	for _, u := range []Unfitness{UnfitUnknown, UnfitFit, UnfitLoopback, UnfitTunnel, UnfitNoAddress, UnfitFitTunnel} {
		if got := u.Fit(); got != fit[u] {
			t.Errorf("Unfitness(%d).Fit() = %v, ожидалось %v", u, got, fit[u])
		}
	}
}

func TestIsTunnelByOwnTunAddress(t *testing.T) {
	// Главный случай Windows: адаптер Wintun называется «Подключение по
	// локальной сети 2» и не имеет POINTOPOINT — узнать его можно только по
	// адресу из собственной подсети TUN лаунчера. Без этой ветки пользователь
	// выбрал бы TUN ядра как аплинк и получил петлю.
	if !isOwnTun("Подключение по локальной сети 2", []net.IP{net.ParseIP("172.16.0.1")}) {
		t.Error("TUN лаунчера по адресу 172.16.0.1 не распознан")
	}
	if !isOwnTun("Ethernet 3", []net.IP{net.ParseIP("fdfe:dcba:9876::1")}) {
		t.Error("TUN лаунчера по IPv6-адресу не распознан")
	}
	// Соседние подсети трогать нельзя: 172.16.0.4 уже вне /30.
	if isOwnTun("Ethernet", []net.IP{net.ParseIP("172.16.0.4")}) {
		t.Error("адрес вне подсети TUN ошибочно распознан как собственный TUN")
	}
	if isOwnTun("Ethernet", []net.IP{net.ParseIP("192.168.10.124")}) {
		t.Error("обычный LAN-адрес распознан как собственный TUN")
	}
}

func TestLabelPutsSystemNameFirst(t *testing.T) {
	// Имя уезжает в конфиг и сверяется с ifconfig — оно обязано быть первым,
	// иначе пользователь ищет в списке подпись, а вписывается другое.
	got := Iface{Name: "en0", FriendlyName: "Wi-Fi", Addrs: []string{"192.168.10.124"}, Up: true}.Label()
	if want := "en0 — Wi-Fi (192.168.10.124)"; got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}

func TestLabelWithoutFriendlyNameOrAddr(t *testing.T) {
	if got, want := (Iface{Name: "eth0", Up: true}).Label(), "eth0"; got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
	// Дублирующая подпись не удваивается: на Linux/Windows friendly == system.
	got := Iface{Name: "Ethernet", FriendlyName: "Ethernet", Addrs: []string{"10.0.0.2"}, Up: true}.Label()
	if want := "Ethernet (10.0.0.2)"; got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}

func TestLabelMarksDownInterface(t *testing.T) {
	got := Iface{Name: "en9", Addrs: []string{"169.254.1.1"}}.Label()
	if want := "en9 (169.254.1.1) [down]"; got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}

func TestListSkipsLoopbackAndOwnTun(t *testing.T) {
	// Прогон по живой машине: гарантий про конкретные имена нет, но ни один
	// loopback, ни собственный TUN, ни безадресный интерфейс попасть в выбор
	// не могут. Чужой туннель — может, и это теперь норма (SPEC 113-F).
	for _, ifc := range ListOrEmpty() {
		ips := make([]net.IP, 0, len(ifc.Addrs))
		for _, a := range ifc.Addrs {
			ips = append(ips, net.ParseIP(a))
		}
		if isOwnTun(ifc.Name, ips) {
			t.Errorf("List() вернул собственный TUN %q — это петля в выборе", ifc.Name)
		}
		if ifc.Name == "lo0" || ifc.Name == "lo" {
			t.Errorf("List() вернул loopback %q", ifc.Name)
		}
		if len(ifc.Addrs) == 0 {
			t.Errorf("List() вернул %q без адресов — мёртвый маршрут в выборе", ifc.Name)
		}
	}
}

func TestExistsRejectsEmptyAndUnknown(t *testing.T) {
	if Exists("") {
		t.Error(`Exists("") = true`)
	}
	if Exists("definitely-no-such-iface-42") {
		t.Error("Exists() = true для несуществующего имени")
	}
}

func TestFromRemoteFiltersLikeLocal(t *testing.T) {
	// Правила отбора обязаны совпадать с локальными: демон отдаёт всё подряд
	// и прямо оговаривает, что фильтрация — задача вызывающего.
	SetOwnTunNames("lxd-tun0")
	defer SetOwnTunNames()

	rejected := []struct {
		name  string
		addrs []string
		why   string
	}{
		{"lo", []string{"127.0.0.1/8"}, "loopback"},
		{"lxd-tun0", []string{"172.16.0.1/30"}, "TUN демона (адрес из нашей /30)"},
		{"lxd-tun0", []string{"10.99.0.1/24"}, "TUN демона (имя из конфига)"},
		{"tun9", nil, "туннель без адреса"},
		{"eth2", nil, "нет адреса"},
		{"", []string{"1.2.3.4/24"}, "пустое имя"},
	}
	for _, c := range rejected {
		if _, ok := FromRemote(c.name, true, c.addrs); ok {
			t.Errorf("FromRemote(%q) принят, хотя это %s", c.name, c.why)
		}
	}

	for _, name := range []string{"eth0", "wan", "br-lan"} {
		ifc, ok := FromRemote(name, true, []string{"192.168.1.1/24"})
		if !ok {
			t.Errorf("FromRemote(%q) отвергнут, хотя годится в аплинки", name)
			continue
		}
		if ifc.IsTunnel {
			t.Errorf("FromRemote(%q) помечен туннелем — это обычный интерфейс", name)
		}
	}
}

// SPEC 113-F, remote-путь: фильтрует ЛАУНЧЕР (демон отдаёт всё подряд, см.
// Client.HostInterfaces), поэтому чужой туннель роутера обязан доходить до
// пикера отсюда — иначе awg1 на RouteRich так и не появился бы в выборе.
func TestFromRemoteKeepsForeignTunnel(t *testing.T) {
	SetOwnTunNames("lxd-tun0")
	defer SetOwnTunNames()

	for _, name := range []string{"awg1", "wg0", "tun0"} {
		ifc, ok := FromRemote(name, true, []string{"10.7.0.2/24"})
		if !ok {
			t.Fatalf("FromRemote(%q) отвергнут — системный туннель роутера это законный аплинк", name)
		}
		if !ifc.IsTunnel {
			t.Errorf("FromRemote(%q).IsTunnel = false — подпись промолчит о последствии", name)
		}
	}
}

func TestFromRemoteStripsCIDRPrefix(t *testing.T) {
	// Демон присылает адрес строкой с префиксом; в подписи он не нужен.
	ifc, ok := FromRemote("eth0", true, []string{"192.168.10.1/24"})
	if !ok {
		t.Fatal("интерфейс отвергнут")
	}
	if got, want := ifc.Label(), "eth0 (192.168.10.1)"; got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}

func TestFromRemoteKeepsDownInterfaceWithAddress(t *testing.T) {
	// Лежачий интерфейс с адресом — валидный выбор (настроить заранее), но
	// пользователь должен видеть, что он сейчас не поднят.
	ifc, ok := FromRemote("eth1", false, []string{"10.0.0.2/24"})
	if !ok {
		t.Fatal("лежачий интерфейс с адресом отвергнут")
	}
	if !strings.Contains(ifc.Label(), "[down]") {
		t.Errorf("Label() = %q, ожидалась пометка [down]", ifc.Label())
	}
}

// LAN-сторона (tun.include_interface) отбирается МЯГЧЕ аплинковой: порт без
// адреса законен, туннель — нет вовсе.

func TestLANCandidateKeepsAddresslessPort(t *testing.T) {
	// Главный случай роутера: lan1/lan2 не несут собственного IP (адрес живёт
	// на мосту) или в них ещё никто не воткнулся. Для аплинка это мёртвый
	// маршрут, для LAN-стороны — штатное состояние, и прятать такой порт
	// значило бы отнять ровно то, ради чего поле и существует.
	for _, name := range []string{"lan1", "lan2", "eth3", "br-lan"} {
		if !lanCandidate(name, false, nil) {
			t.Errorf("lanCandidate(%q) = false, безадресный LAN-порт обязан предлагаться", name)
		}
	}
}

func TestLANCandidateRejectsLoopback(t *testing.T) {
	if lanCandidate("lo0", true, []net.IP{net.ParseIP("127.0.0.1")}) {
		t.Error("петля предложена как LAN-порт")
	}
	if lanCandidate("lo", true, nil) {
		t.Error("петля без адреса предложена как LAN-порт")
	}
}

func TestLANCandidateRejectsEveryTunnel(t *testing.T) {
	SetOwnTunNames("singbox-tun0")
	defer SetOwnTunNames()

	addr := []net.IP{net.ParseIP("10.7.0.2")}
	cases := []struct {
		name  string
		addrs []net.IP
		why   string
	}{
		{"singbox-tun0", []net.IP{net.ParseIP("10.55.0.1")}, "СОБСТВЕННЫЙ TUN ядра по имени из конфига"},
		{"Подключение по локальной сети 2", []net.IP{net.ParseIP("172.16.0.1")}, "собственный TUN по адресу из нашей /30"},
		{"awg1", addr, "чужой туннель с адресом — для аплинка законен, для LAN нет"},
		{"wg0", addr, "чужой туннель с адресом"},
		{"tun0", addr, "чужой туннель с адресом"},
		{"utun4", nil, "туннель без адреса"},
		{"ipsec0", addr, "туннель с адресом"},
	}
	for _, c := range cases {
		if lanCandidate(c.name, false, c.addrs) {
			t.Errorf("lanCandidate(%q) = true, а это %s", c.name, c.why)
		}
	}
}

func TestLANCandidateRejectsEmptyName(t *testing.T) {
	if lanCandidate("   ", false, nil) {
		t.Error("интерфейс без имени предложен как LAN-порт")
	}
}

// Отбор LAN-стороны обязан быть НЕ строже аплинкового: всё, что годится в
// аплинк и не является туннелем, годится и в LAN-порты. Разъехавшись, два
// списка предлагали бы для одной машины взаимоисключающее.
func TestListLANCandidatesSupersetOfNonTunnelUplinks(t *testing.T) {
	lan := map[string]bool{}
	for _, ifc := range ListLANCandidatesOrEmpty() {
		lan[ifc.Name] = true
		if ifc.Name == "lo" || ifc.Name == "lo0" {
			t.Errorf("ListLANCandidates() вернул петлю %q", ifc.Name)
		}
		if hasTunnelName(ifc.Name) {
			t.Errorf("ListLANCandidates() вернул туннель %q", ifc.Name)
		}
	}
	for _, ifc := range ListOrEmpty() {
		if ifc.IsTunnel {
			continue // чужой туннель для LAN-стороны законно отсутствует
		}
		if !lan[ifc.Name] {
			t.Errorf("аплинк %q не попал в LAN-кандидаты, хотя туннелем не является", ifc.Name)
		}
	}
}

func TestFromRemoteLANKeepsAddresslessAndDropsTunnels(t *testing.T) {
	SetOwnTunNames("lxd-tun0")
	defer SetOwnTunNames()

	// Порт роутера без адреса — самый частый LAN-кандидат.
	if _, ok := FromRemoteLAN("lan2", true, nil); !ok {
		t.Error("FromRemoteLAN(lan2) отвергнут — безадресный LAN-порт обязан проходить")
	}
	// Лежачий тоже: настроить его нужно заранее.
	ifc, ok := FromRemoteLAN("lan3", false, nil)
	if !ok {
		t.Fatal("лежачий LAN-порт отвергнут")
	}
	if !strings.Contains(ifc.Label(), "[down]") {
		t.Errorf("Label() = %q, ожидалась пометка [down]", ifc.Label())
	}

	rejected := []struct {
		name  string
		addrs []string
		why   string
	}{
		{"lo", []string{"127.0.0.1/8"}, "петля"},
		{"lo0", nil, "петля"},
		{"lxd-tun0", []string{"10.99.0.1/24"}, "собственный TUN демона"},
		{"awg1", []string{"10.7.0.2/24"}, "чужой туннель"},
		{"wg0", nil, "туннель без адреса"},
		{"", []string{"1.2.3.4/24"}, "пустое имя"},
	}
	for _, c := range rejected {
		if _, ok := FromRemoteLAN(c.name, true, c.addrs); ok {
			t.Errorf("FromRemoteLAN(%q) принят, хотя это %s", c.name, c.why)
		}
	}

	// Мост с адресом — обычный LAN-кандидат, префикс из адреса срезается.
	br, ok := FromRemoteLAN("br-lan", true, []string{"192.168.10.1/24"})
	if !ok {
		t.Fatal("br-lan отвергнут")
	}
	if got, want := br.Label(), "br-lan (192.168.10.1)"; got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}
