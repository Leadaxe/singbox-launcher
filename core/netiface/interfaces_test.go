package netiface

import (
	"net"
	"strings"
	"testing"
)

func TestIsTunnelByName(t *testing.T) {
	// Привязка аплинка к собственному TUN ядра = петля, поэтому туннели
	// обязаны отсеиваться до попадания в выбор.
	for _, name := range []string{"utun0", "utun7", "tun0", "wg0", "awg1", "ppp0", "gif0", "stf0", "singbox-tun0"} {
		if !isTunnel(name, 0, nil) {
			t.Errorf("isTunnel(%q) = false, туннель обязан отсеиваться", name)
		}
	}
	for _, name := range []string{"en0", "en9", "eth0", "Ethernet", "Wi-Fi", "bridge0"} {
		if isTunnel(name, 0, nil) {
			t.Errorf("isTunnel(%q) = true, обычный интерфейс отсеиваться не должен", name)
		}
	}
}

func TestIsTunnelByPointToPointFlag(t *testing.T) {
	// Туннель без узнаваемого имени ловится флагом.
	if !isTunnel("weird0", net.FlagPointToPoint, nil) {
		t.Error("POINTOPOINT-интерфейс не распознан как туннель")
	}
	if isTunnel("weird0", net.FlagUp|net.FlagBroadcast, nil) {
		t.Error("обычный broadcast-интерфейс распознан как туннель")
	}
}

func TestIsTunnelByOwnTunAddress(t *testing.T) {
	// Главный случай Windows: адаптер Wintun называется «Подключение по
	// локальной сети 2» и не имеет POINTOPOINT — узнать его можно только по
	// адресу из собственной подсети TUN лаунчера. Без этой ветки пользователь
	// выбрал бы TUN ядра как аплинк и получил петлю.
	if !isTunnel("Подключение по локальной сети 2", net.FlagUp|net.FlagBroadcast,
		[]net.IP{net.ParseIP("172.16.0.1")}) {
		t.Error("TUN лаунчера по адресу 172.16.0.1 не распознан")
	}
	if !isTunnel("Ethernet 3", net.FlagUp, []net.IP{net.ParseIP("fdfe:dcba:9876::1")}) {
		t.Error("TUN лаунчера по IPv6-адресу не распознан")
	}
	// Соседние подсети трогать нельзя: 172.16.0.4 уже вне /30.
	if isTunnel("Ethernet", net.FlagUp|net.FlagBroadcast, []net.IP{net.ParseIP("172.16.0.4")}) {
		t.Error("адрес вне подсети TUN ошибочно распознан как туннель")
	}
	if isTunnel("Ethernet", net.FlagUp|net.FlagBroadcast, []net.IP{net.ParseIP("192.168.10.124")}) {
		t.Error("обычный LAN-адрес распознан как туннель")
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

func TestListSkipsLoopbackAndTunnels(t *testing.T) {
	// Прогон по живой машине: гарантий про конкретные имена нет, но ни один
	// loopback и ни один туннель попасть в выбор не может.
	for _, ifc := range ListOrEmpty() {
		if isTunnel(ifc.Name, 0, nil) {
			t.Errorf("List() вернул туннель %q", ifc.Name)
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
	rejected := []struct {
		name  string
		addrs []string
		why   string
	}{
		{"lo", []string{"127.0.0.1/8"}, "loopback"},
		{"lxd-tun0", []string{"172.16.0.1/30"}, "TUN демона"},
		{"tun0", []string{"10.8.0.1/24"}, "туннель"},
		{"wg0", []string{"10.9.0.1/24"}, "WireGuard"},
		{"eth2", nil, "нет адреса"},
		{"", []string{"1.2.3.4/24"}, "пустое имя"},
	}
	for _, c := range rejected {
		if _, ok := FromRemote(c.name, true, c.addrs); ok {
			t.Errorf("FromRemote(%q) принят, хотя это %s", c.name, c.why)
		}
	}

	for _, name := range []string{"eth0", "wan", "br-lan"} {
		if _, ok := FromRemote(name, true, []string{"192.168.1.1/24"}); !ok {
			t.Errorf("FromRemote(%q) отвергнут, хотя годится в аплинки", name)
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
