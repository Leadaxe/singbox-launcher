package tabs

import (
	"strings"
	"testing"

	"singbox-launcher/core/netiface"
	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// Пикер вешается на ОДНУ переменную, а не на весь тип text_list: иначе список
// интерфейсов предлагался бы в любом списке строк — от доменов до портов.
func TestLANIfacePickAppliesOnlyToItsVar(t *testing.T) {
	if !lanIfacePickApplies("gateway_include_interface") {
		t.Error("поле LAN-интерфейсов осталось без пикера")
	}
	for _, other := range []string{"dns_user_rules", "tun_address", "", "gateway_mode"} {
		if lanIfacePickApplies(other) {
			t.Errorf("пикер интерфейсов навязан переменной %q", other)
		}
	}
}

// Формат поля — одно имя на строку: ровно так его читает подстановка шаблона
// (text_list разбивается по переводам строк).
func TestParseLANIfaceListSplitsByLines(t *testing.T) {
	got := parseLANIfaceList(" br-lan \n\n  lan2\nlan3 \n")
	want := []string{"br-lan", "lan2", "lan3"}
	if len(got) != len(want) {
		t.Fatalf("разбор = %v, ожидалось %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("разбор = %v, ожидалось %v", got, want)
		}
	}
	if len(parseLANIfaceList("")) != 0 || len(parseLANIfaceList("   \n  ")) != 0 {
		t.Error("пустое поле обязано разбираться в пустой список")
	}
}

func TestAppendLANIfaceWritesOneNamePerLine(t *testing.T) {
	if got, want := appendLANIface("", "br-lan"), "br-lan"; got != want {
		t.Errorf("дописывание в пустое поле = %q, want %q", got, want)
	}
	if got, want := appendLANIface("br-lan", "lan2"), "br-lan\nlan2"; got != want {
		t.Errorf("дописывание = %q, want %q", got, want)
	}
	// Хвостовой перевод строки (пользователь нажал Enter перед кликом) не
	// удваивается — иначе в поле копились бы пустые строки.
	if got, want := appendLANIface("br-lan\n", "lan2"), "br-lan\nlan2"; got != want {
		t.Errorf("дописывание после Enter = %q, want %q", got, want)
	}
	// Ручной ввод сохраняется как есть: пикер только дописывает.
	if got, want := appendLANIface("не-существует-пока", "lan2"), "не-существует-пока\nlan2"; got != want {
		t.Errorf("ручной ввод потерян: %q", got)
	}
}

func TestAppendLANIfaceDoesNotDuplicate(t *testing.T) {
	// Кнопка «+» уже вписанное не предлагает, но текст поля правится и руками
	// между двумя нажатиями — дубликат уехал бы в конфиг.
	if got, want := appendLANIface("br-lan\nlan2", "lan2"), "br-lan\nlan2"; got != want {
		t.Errorf("дубликат дописан: %q", got)
	}
	if got, want := appendLANIface("BR-LAN", "br-lan"), "BR-LAN"; got != want {
		t.Errorf("дубликат в другом регистре дописан: %q", got)
	}
	if got, want := appendLANIface("br-lan", "  "), "br-lan"; got != want {
		t.Errorf("пустое имя дописано: %q", got)
	}
}

// Список выбора не предлагает того, что уже в поле: клик по такому пункту либо
// ничего не делает, либо кладёт дубликат.
func TestLANIfaceCandidatesExcludeAlreadyAdded(t *testing.T) {
	resetInterfaceCacheForTest(t)
	SetRemoteInterfaceProvider(func(string) ([]RemoteRawIface, bool) {
		return []RemoteRawIface{
			{Name: "br-lan", Up: true, Addrs: []string{"192.168.10.1/24"}},
			{Name: "lan2", Up: true},
			{Name: "lan3", Up: false},
		}, true
	})
	defer SetRemoteInterfaceProvider(nil)

	m := &wizardmodels.WizardModel{
		Target: wizardtemplate.RemoteTargetFor("linux", "amd64", "router"),
	}
	lanIfaceCandidates(m, nil) // заводит фоновый запрос
	waitRemoteInterfaces(t, "router")

	names, hints, pending := lanIfaceCandidates(m, nil)
	if pending {
		t.Error("ответ уже в кэше — ждать нечего")
	}
	if len(names) != 3 {
		t.Fatalf("кандидаты = %v, ожидались все три порта (в том числе безадресные)", names)
	}
	if hints["br-lan"] == "" {
		t.Error("расшифровка порта потеряна")
	}

	names, _, _ = lanIfaceCandidates(m, []string{"lan2", "BR-LAN"})
	if len(names) != 1 || names[0] != "lan3" {
		t.Fatalf("кандидаты = %v, уже вписанные обязаны исключаться (в любом регистре)", names)
	}
}

// LAN-фильтр применяется к ТОМУ ЖЕ ответу демона, что и аплинковый: второй
// запрос к машине ради второго фильтра ничего нового не узнал бы.
func TestLANIfaceCandidatesShareUplinkCache(t *testing.T) {
	resetInterfaceCacheForTest(t)
	netiface.SetOwnTunNames("lxd-tun0")
	defer netiface.SetOwnTunNames()

	calls := 0
	SetRemoteInterfaceProvider(func(string) ([]RemoteRawIface, bool) {
		calls++
		return []RemoteRawIface{
			{Name: "lo", Up: true, Addrs: []string{"127.0.0.1/8"}},
			{Name: "wan", Up: true, Addrs: []string{"10.20.30.40/24"}},
			{Name: "awg1", Up: true, Addrs: []string{"10.7.0.2/24"}},
			{Name: "lxd-tun0", Up: true, Addrs: []string{"172.16.0.1/30"}},
			{Name: "lan2", Up: true},
		}, true
	})
	defer SetRemoteInterfaceProvider(nil)

	m := &wizardmodels.WizardModel{
		Target: wizardtemplate.RemoteTargetFor("linux", "amd64", "shared"),
	}
	interfacePickOptions(m, "")
	waitRemoteInterfaces(t, "shared")

	lanNames, _, _ := lanIfaceCandidates(m, nil)
	uplinkNames, _, _ := interfacePickOptions(m, "")

	if calls != 1 {
		t.Fatalf("запросов к машине = %d, оба фильтра обязаны жить на одном ответе", calls)
	}

	// Аплинк: чужой туннель законен, безадресный порт — нет.
	if !containsName(uplinkNames, "awg1") {
		t.Error("чужой туннель пропал из аплинков — SPEC 113-F")
	}
	if containsName(uplinkNames, "lan2") {
		t.Error("безадресный порт предложен как аплинк — это мёртвый маршрут")
	}
	// LAN: ровно наоборот.
	if containsName(lanNames, "awg1") {
		t.Error("туннель предложен как LAN-порт")
	}
	if !containsName(lanNames, "lan2") {
		t.Error("безадресный LAN-порт пропал — ради него поле и существует")
	}
	// Общее для обоих: петля и собственный TUN демона не предлагаются нигде.
	for _, list := range [][]string{lanNames, uplinkNames} {
		for _, bad := range []string{"lo", "lxd-tun0"} {
			if containsName(list, bad) {
				t.Errorf("%q попал в выбор", bad)
			}
		}
	}
	// А физический аплинк с адресом годится и туда, и туда.
	if !containsName(lanNames, "wan") || !containsName(uplinkNames, "wan") {
		t.Error("обычный интерфейс с адресом обязан быть в обоих списках")
	}
}

// Пока ответ машины в пути, пустой список — не «кандидатов нет».
func TestLANIfaceCandidatesPendingWhileMachineSilent(t *testing.T) {
	resetInterfaceCacheForTest(t)
	release := make(chan struct{})
	SetRemoteInterfaceProvider(func(string) ([]RemoteRawIface, bool) {
		<-release
		return nil, true
	})
	defer func() {
		close(release)
		SetRemoteInterfaceProvider(nil)
	}()

	m := &wizardmodels.WizardModel{
		Target: wizardtemplate.RemoteTargetFor("linux", "amd64", "slow"),
	}
	names, _, pending := lanIfaceCandidates(m, nil)
	if len(names) != 0 || !pending {
		t.Fatalf("names=%v pending=%v — до ответа список пуст и помечен ожиданием", names, pending)
	}
}

// Локальный путь идёт мимо кэша машины: имена там значат ДРУГОЕ железо.
func TestLANIfaceCandidatesLocalNamesAreBare(t *testing.T) {
	names, hints, pending := lanIfaceCandidates(&wizardmodels.WizardModel{}, nil)
	if pending {
		t.Error("локальный список читается синхронно, ждать нечего")
	}
	for _, n := range names {
		if strings.ContainsAny(n, "()") {
			t.Errorf("имя %q содержит оформление — оно уедет в поле как есть", n)
		}
		if !strings.HasPrefix(hints[n], n) {
			t.Errorf("подпись %q не начинается с имени %q", hints[n], n)
		}
	}
}

// В меню показываются подписи, а в поле уезжает имя: порядок обязан совпадать,
// иначе клик по «lan2 (…)» вписал бы соседний порт.
func TestLANIfacePickLabelsKeepOrderAndFallBackToName(t *testing.T) {
	names := []string{"br-lan", "lan2"}
	hints := map[string]string{"br-lan": "br-lan (192.168.10.1)"}
	got := lanIfacePickLabels(names, hints)
	if len(got) != 2 || got[0] != hints["br-lan"] || got[1] != "lan2" {
		t.Fatalf("подписи = %v, ожидались [расшифровка, голое имя]", got)
	}
}

func containsName(list []string, name string) bool {
	for _, n := range list {
		if n == name {
			return true
		}
	}
	return false
}
