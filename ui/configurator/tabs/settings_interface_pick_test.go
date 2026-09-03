package tabs

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"singbox-launcher/core/netiface"
	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// resetInterfaceCacheForTest — тесты делят пакетные переменные, и кэш машины
// переживает границу теста: без сброса второй тест читал бы ответ первого.
func resetInterfaceCacheForTest(t *testing.T) {
	t.Helper()
	InvalidateRemoteInterfaceCache("")
	subscribeInterfaceHints(nil)
}

// waitRemoteInterfaces ждёт, пока фоновый запрос машины доедет до кэша.
//
// Опрос, а не колбэк: подписка одна на пакет, и тесты, ждущие её каждый
// по-своему, отбирали бы её друг у друга.
func waitRemoteInterfaces(t *testing.T, machineID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, _, loaded := cachedRemoteInterfaces(machineID); loaded {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("интерфейсы машины %q не приехали за отведённое время", machineID)
}

func TestInterfaceHintExplainsEmptyValue(t *testing.T) {
	// Пустое значение = ключ default_interface не пишется вовсе. Это штатный
	// режим, и подпись обязана говорить именно это, а не молчать и не пугать.
	got := interfaceHintFor(&wizardmodels.WizardModel{}, "", map[string]string{}, false)
	if got == "" || strings.Contains(got, "⚠") {
		t.Fatalf("подпись для пустого значения = %q, ожидалось нейтральное пояснение", got)
	}
}

func TestInterfaceHintDescribesKnownInterface(t *testing.T) {
	hints := map[string]string{"en0": "en0 — Wi-Fi (192.168.10.124)"}
	if got := interfaceHintFor(&wizardmodels.WizardModel{}, "en0", hints, false); got != hints["en0"] {
		t.Fatalf("подпись = %q, ожидалась расшифровка из списка", got)
	}
}

func TestInterfaceHintWarnsAboutUnknownLocalName(t *testing.T) {
	// Опечатка в имени = ядро стартует, но трафика нет. Молчать здесь нельзя.
	got := interfaceHintFor(&wizardmodels.WizardModel{}, "definitely-no-such-iface-42", map[string]string{}, false)
	if !strings.Contains(got, "⚠") {
		t.Fatalf("подпись = %q, ожидалось предупреждение", got)
	}
}

func TestInterfaceHintDoesNotWarnOnRemoteTarget(t *testing.T) {
	// Имя относится к другой машине: сверять его с локальными интерфейсами
	// нельзя, иначе валидное имя роутера показывалось бы как ошибка.
	m := &wizardmodels.WizardModel{Target: wizardtemplate.RemoteTarget("linux", "amd64")}
	got := interfaceHintFor(m, "eth0", map[string]string{}, false)
	if strings.Contains(got, "⚠") {
		t.Fatalf("подпись = %q, для remote-таргета предупреждать не о чем", got)
	}
}

// SPEC 113-E (M6): пока ответ машины в пути, подпись обязана говорить про
// загрузку, а не пугать «не могу проверить» тем, что через секунду приедет.
func TestInterfaceHintSaysLoadingWhilePending(t *testing.T) {
	m := &wizardmodels.WizardModel{Target: wizardtemplate.RemoteTargetFor("linux", "amd64", "home")}
	got := interfaceHintFor(m, "wan", map[string]string{}, true)
	if strings.Contains(got, "⚠") {
		t.Fatalf("подпись = %q, пока ответа нет предупреждать не о чем", got)
	}
	if got == interfaceHintFor(m, "wan", map[string]string{}, false) {
		t.Error("подпись «загрузка» обязана отличаться от подписи «проверить нечем»")
	}
}

// В sing-box route.auto_detect_interface перебивает route.default_interface:
// при включённой галке выбранное имя ядро молча игнорирует. Активный дропдаун
// в этот момент обманывает пользователя, поэтому строка обязана гаснуть.

func TestInterfacePickSuppressedByAutoDetect(t *testing.T) {
	on := map[string]wizardtemplate.ResolvedVar{autoDetectInterfaceVar: {Scalar: "true"}}
	if !interfacePickSuppressed(on) {
		t.Error("автоопределение включено — выбор интерфейса не имеет силы, поле обязано гаснуть")
	}
	off := map[string]wizardtemplate.ResolvedVar{autoDetectInterfaceVar: {Scalar: "false"}}
	if interfacePickSuppressed(off) {
		t.Error("автоопределение снято — поле обязано работать")
	}
}

func TestInterfacePickNotSuppressedWithoutTheVar(t *testing.T) {
	// Переменной нет в шаблоне = автоопределения нет. Гасить поле «на всякий
	// случай» значило бы отнять настройку у шаблона, который её не запрещал.
	if interfacePickSuppressed(map[string]wizardtemplate.ResolvedVar{}) {
		t.Error("без auto_detect_interface подавлять нечем")
	}
}

func TestInterfaceRowEnabledCombinesGateAndSuppression(t *testing.T) {
	// Состояние строки — конъюнкция: гейт шаблона И отсутствие подавления.
	// Ни одна из половин не должна уметь включить поле в одиночку.
	on := map[string]wizardtemplate.ResolvedVar{autoDetectInterfaceVar: {Scalar: "true"}}
	off := map[string]wizardtemplate.ResolvedVar{autoDetectInterfaceVar: {Scalar: "false"}}

	if interfaceRowEnabled(true, on) {
		t.Error("гейт открыт, но автоопределение включено — поле обязано быть выключено")
	}
	if interfaceRowEnabled(false, off) {
		t.Error("автоопределение снято, но гейт шаблона закрыт — поле обязано остаться выключенным")
	}
	if !interfaceRowEnabled(true, off) {
		t.Error("гейт открыт и автоопределение снято — поле обязано работать")
	}
}

// Подпись обязана объяснять, ПОЧЕМУ поле погашено. Прежняя строка «Traffic will
// go through this interface» под погашенным дропдауном — прямая ложь: трафик
// пойдёт через интерфейс, который выберет ядро.
func TestInterfaceHintExplainsSuppression(t *testing.T) {
	m := &wizardmodels.WizardModel{}
	hints := map[string]string{"en0": "en0 — Wi-Fi (192.168.10.124)"}

	got := interfaceHintForRow(m, "en0", hints, false, true)
	if got == hints["en0"] {
		t.Fatal("при подавлении подпись расшифровывает выбранное имя — она обещает то, чего не будет")
	}
	if got == "" {
		t.Fatal("погашенное поле обязано объяснять причину, а не молчать")
	}
	// А без подавления — обычное поведение, ничего не подменяем.
	if got := interfaceHintForRow(m, "en0", hints, false, false); got != hints["en0"] {
		t.Errorf("подпись = %q, без подавления ожидалась расшифровка", got)
	}
}

func TestInterfacePickLocalNamesAreBare(t *testing.T) {
	// В выпадающем списке SelectEntry лежит то, что уедет в конфиг дословно:
	// имя обязано быть чистым, без подписи вида «en0 — Wi-Fi (…)».
	names, hints, pending := interfacePickOptions(&wizardmodels.WizardModel{}, "")
	if pending {
		t.Error("локальный список читается синхронно, ждать нечего")
	}
	for _, n := range names {
		if strings.ContainsAny(n, " ()—") {
			t.Errorf("имя %q содержит оформление — оно уедет в конфиг как есть", n)
		}
		if hints[n] == "" {
			t.Errorf("для %q нет расшифровки", n)
		}
	}
}

// SPEC 113-F: чужой туннель — законный аплинк, и подпись обязана отличаться от
// обычной. Прежде «this is a tunnel — the core cannot use it as an uplink»
// врало пользователю с поднятым системным awg1: как раз может, и именно этого
// он хотел.
func TestInterfaceHintMarksForeignTunnel(t *testing.T) {
	plain := netiface.Iface{Name: "en0", FriendlyName: "Wi-Fi", Addrs: []string{"192.168.10.124"}, Up: true}
	tunnel := netiface.Iface{Name: "awg1", Addrs: []string{"10.7.0.2"}, Up: true, IsTunnel: true}

	if got := InterfaceHintText(plain); got != plain.Label() {
		t.Errorf("подпись обычного интерфейса = %q, ожидался чистый Label()", got)
	}
	got := InterfaceHintText(tunnel)
	if !strings.HasPrefix(got, tunnel.Label()) {
		t.Fatalf("подпись туннеля = %q, имя и адрес обязаны остаться на месте", got)
	}
	if got == tunnel.Label() {
		t.Fatal("туннель подписан как обычный интерфейс — о последствии никто не предупредил")
	}
	// Предупреждение, а не отказ: значок ⚠ здесь ставить нельзя, выбор законный.
	if strings.Contains(got, "⚠") {
		t.Errorf("подпись = %q, законный выбор помечен как ошибка", got)
	}
}

func TestInterfacePickListsForeignTunnelAsFit(t *testing.T) {
	// Сквозная проверка по живой машине: если туннель попал в List, то и
	// Fitness обязан звать его годным, и подпись — брать расшифровку, а не
	// ругаться. Разъехавшись, список и поле противоречили бы друг другу.
	names, hints, _ := interfacePickOptions(&wizardmodels.WizardModel{}, "")
	for _, n := range names {
		if fit := netiface.Fitness(n); !fit.Fit() {
			t.Errorf("Fitness(%q) = %v, но пикер его предлагает", n, fit)
		}
		if got := interfaceHintFor(&wizardmodels.WizardModel{}, n, hints, false); strings.Contains(got, "⚠") {
			t.Errorf("подпись предложенного %q = %q — пикер и поле разошлись", n, got)
		}
	}
}

func TestInterfacePickRemoteWithoutProviderIsEmpty(t *testing.T) {
	// Провайдер не установлен / машина не подключена: подсказок нет, и это
	// рабочее состояние — поле остаётся пригодным для ручного ввода.
	resetInterfaceCacheForTest(t)
	SetRemoteInterfaceProvider(nil)
	m := &wizardmodels.WizardModel{Target: wizardtemplate.RemoteTarget("linux", "amd64")}
	names, _, _ := interfacePickOptions(m, "")
	if len(names) != 0 {
		t.Fatalf("список = %v, ожидался пустой", names)
	}
}

func TestInterfacePickRemoteUsesProvider(t *testing.T) {
	resetInterfaceCacheForTest(t)
	SetRemoteInterfaceProvider(func(id string) ([]RemoteRawIface, bool) {
		if id != "home" {
			t.Errorf("провайдер получил machineID %q, ожидался home", id)
		}
		return []RemoteRawIface{
			{Name: "eth0", Up: true, Addrs: []string{"192.168.10.1/24"}},
			{Name: "wan", Up: true, Addrs: []string{"10.20.30.40/24"}},
		}, true
	})
	defer SetRemoteInterfaceProvider(nil)

	m := &wizardmodels.WizardModel{
		Target: wizardtemplate.RemoteTargetFor("linux", "amd64", "home"),
	}
	// Первый вызов только заводит запрос: сеть с UI-потока запрещена (M6).
	interfacePickOptions(m, "")
	waitRemoteInterfaces(t, "home")

	names, hints, pending := interfacePickOptions(m, "")
	if pending {
		t.Error("ответ уже в кэше — ждать нечего")
	}
	if len(names) != 2 || names[0] != "eth0" {
		t.Fatalf("список = %v, ожидались интерфейсы удалённой машины", names)
	}
	if hints["wan"] == "" {
		t.Error("расшифровка удалённого интерфейса потеряна")
	}
	// И подпись обязана брать её же, а не ругаться на «неизвестное имя».
	if got := interfaceHintFor(m, "wan", hints, false); got != hints["wan"] {
		t.Errorf("подпись = %q, ожидалась расшифровка от демона", got)
	}
}

// SPEC 113-E (M6), регресс: сбор строки НЕ ждёт машину.
//
// Провайдер здесь имитирует неотвечающий роутер (mTLS-дедлайн в десятки
// секунд). На старом коде interfacePickOptions звала его синхронно, и вызов
// висел бы вместе со всем UI-потоком.
func TestInterfacePickDoesNotBlockOnUnresponsiveMachine(t *testing.T) {
	resetInterfaceCacheForTest(t)
	release := make(chan struct{})
	SetRemoteInterfaceProvider(func(string) ([]RemoteRawIface, bool) {
		<-release
		return []RemoteRawIface{{Name: "wan", Up: true, Addrs: []string{"10.0.0.1/24"}}}, true
	})
	defer func() {
		close(release)
		SetRemoteInterfaceProvider(nil)
	}()

	m := &wizardmodels.WizardModel{
		Target: wizardtemplate.RemoteTargetFor("linux", "amd64", "stuck"),
	}
	done := make(chan struct{})
	go func() {
		names, _, pending := interfacePickOptions(m, "")
		if len(names) != 0 || !pending {
			t.Errorf("ряд обязан строиться из пустого кэша: names=%v pending=%v", names, pending)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("сбор строки ждал машину — это и есть заморозка вкладки Settings")
	}
}

// SPEC 113-E (M6), регресс: повторные refresh вкладки не плодят запросы к
// машине. На неотвечающем роутере их накапливалось по одному на отрисовку.
func TestInterfacePickSingleFlightPerMachine(t *testing.T) {
	resetInterfaceCacheForTest(t)
	var calls int32
	release := make(chan struct{})
	SetRemoteInterfaceProvider(func(string) ([]RemoteRawIface, bool) {
		atomic.AddInt32(&calls, 1)
		<-release
		return []RemoteRawIface{{Name: "wan", Up: true, Addrs: []string{"10.0.0.1/24"}}}, true
	})
	defer func() {
		close(release)
		SetRemoteInterfaceProvider(nil)
	}()

	m := &wizardmodels.WizardModel{
		Target: wizardtemplate.RemoteTargetFor("linux", "amd64", "busy"),
	}
	for i := 0; i < 5; i++ {
		interfacePickOptions(m, "")
	}
	// Даём заведённой горутине дойти до провайдера.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && atomic.LoadInt32(&calls) == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("запросов к машине = %d, ожидался ровно один", got)
	}
}

// SPEC 113-E (M6): провал запроса — такой же ответ на вопрос «что показывать»,
// как и успех. Без пробуждения подписки поле оставалось на «идёт загрузка» до
// следующей пересборки вкладки, а pending не давал сказать «проверить нечем».
func TestInterfacePickWakesSubscriberOnFailure(t *testing.T) {
	resetInterfaceCacheForTest(t)
	SetRemoteInterfaceProvider(func(string) ([]RemoteRawIface, bool) {
		return nil, false
	})
	defer SetRemoteInterfaceProvider(nil)

	woken := make(chan struct{}, 1)
	subscribeInterfaceHints(func() {
		select {
		case woken <- struct{}{}:
		default:
		}
	})
	defer subscribeInterfaceHints(nil)

	m := &wizardmodels.WizardModel{
		Target: wizardtemplate.RemoteTargetFor("linux", "amd64", "dead"),
	}
	if _, _, pending := interfacePickOptions(m, "wan"); !pending {
		t.Fatal("запрос только заведён — на этот момент ответа ещё нет")
	}
	select {
	case <-woken:
	case <-time.After(2 * time.Second):
		t.Fatal("подписка не разбужена отказом — подпись «загрузка» осталась бы висеть")
	}

	names, hints, pending := interfacePickOptions(m, "wan")
	if pending {
		t.Fatal("после провала ждать больше нечего: pending обязан сняться")
	}
	if len(names) != 0 {
		t.Fatalf("список = %v, при отказе подсказок нет", names)
	}
	// И подпись обязана перейти на «проверить нечем», а не на «загрузка».
	got := interfaceHintFor(m, "wan", hints, pending)
	if got == interfaceHintFor(m, "wan", hints, true) {
		t.Error("подпись после отказа не отличается от подписи «идёт загрузка»")
	}
}

// Отказ провайдера не стирает уже известное: показывать пусто там, где секунду
// назад были имена, значит сделать поле хуже, чем оно было.
func TestInterfacePickKeepsCacheOnProviderFailure(t *testing.T) {
	resetInterfaceCacheForTest(t)
	var ok int32 = 1
	SetRemoteInterfaceProvider(func(string) ([]RemoteRawIface, bool) {
		if atomic.LoadInt32(&ok) == 1 {
			return []RemoteRawIface{{Name: "wan", Up: true, Addrs: []string{"10.0.0.1/24"}}}, true
		}
		return nil, false
	})
	defer SetRemoteInterfaceProvider(nil)

	m := &wizardmodels.WizardModel{
		Target: wizardtemplate.RemoteTargetFor("linux", "amd64", "flaky"),
	}
	interfacePickOptions(m, "")
	waitRemoteInterfaces(t, "flaky")

	// Машина отвалилась.
	atomic.StoreInt32(&ok, 0)
	interfacePickOptions(m, "")
	// Ждём завершения неудачного запроса.
	time.Sleep(50 * time.Millisecond)

	names, _, _ := interfacePickOptions(m, "")
	if len(names) != 1 || names[0] != "wan" {
		t.Fatalf("известное затёрто отказом: %v", names)
	}
}
