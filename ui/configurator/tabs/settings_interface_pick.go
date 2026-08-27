package tabs

import (
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/netiface"
	"singbox-launcher/internal/locale"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// newInterfaceHintLabel — строка-расшифровка под полем интерфейса. Курсив, как
// у прочих пояснений в конфигураторе; текст переписывается на каждый ввод.
func newInterfaceHintLabel() *widget.Label {
	l := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	l.Wrapping = fyne.TextWrapWord
	return l
}

// RemoteInterfaceProvider отдаёт интерфейсы удалённой машины по её ID.
//
// Хук, а не прямой вызов: перечисление живёт в пакете `ui` (там транспорт
// подключённой машины), а `ui` уже зависит от этого пакета — прямой вызов
// замкнул бы цикл импорта. `ui` подставляет реализацию при старте.
//
// ok=false означает «спросить не у кого» — машина не подключена, демон старый
// (ErrHostUnsupported) или не ответил. Для UI это не ошибка: список подсказок
// просто пуст, а поле остаётся полноценным для ручного ввода.
type RemoteInterfaceProvider func(machineID string) (names []string, hints map[string]string, ok bool)

var (
	remoteIfaceMu       sync.RWMutex
	remoteIfaceProvider RemoteInterfaceProvider
)

// SetRemoteInterfaceProvider устанавливает источник интерфейсов удалённых
// машин. Вызывается один раз из пакета `ui` при инициализации.
func SetRemoteInterfaceProvider(p RemoteInterfaceProvider) {
	remoteIfaceMu.Lock()
	defer remoteIfaceMu.Unlock()
	remoteIfaceProvider = p
}

func remoteInterfaces(machineID string) ([]string, map[string]string, bool) {
	remoteIfaceMu.RLock()
	p := remoteIfaceProvider
	remoteIfaceMu.RUnlock()
	if p == nil || strings.TrimSpace(machineID) == "" {
		return nil, nil, false
	}
	return p(machineID)
}

// SPEC 113-E (M6) — интерфейсы удалённой машины НЕ спрашиваются с UI-потока.
//
// Провайдер ходит по REST через mTLS с дедлайном в десятки секунд, а строка
// «interface» строилась на КАЖДЫЙ refresh вкладки Settings. Машина числится
// подключённой, но не отвечает (перезагрузился роутер, сменилась сеть) — и
// приложение висит целиком, хотя пользователь всего лишь открыл настройки.
//
// Отсюда кэш на машину: ряд строится сразу из того, что уже знаем (пусто —
// значит поле просто без подсказок, ручной ввод остаётся), а запрос уходит в
// фон. Повторные refresh не плодят параллельные запросы: пока запрос по машине
// в полёте, следующие только подписываются на его результат.

// remoteIfaceEntry — что известно про интерфейсы одной машины.
type remoteIfaceEntry struct {
	names  []string
	hints  map[string]string
	loaded bool // ответ уже приходил (пусть и пустой)
	inWork bool // запрос в полёте — второй не заводим
	// failed — последняя попытка спросить не удалась (машина не подключена,
	// демон старый, таймаут). Отдельно от loaded, потому что известное при
	// отказе НЕ затирается: с прежним кэшем поле продолжает жить как жило, а
	// без него подпись обязана перестать обещать загрузку.
	failed bool
}

var (
	remoteIfaceCacheMu sync.Mutex
	remoteIfaceCache   = map[string]*remoteIfaceEntry{}
	// ifaceHintsWaiter — строка поля, ждущая подсказок. Ровно одна: поле
	// «interface» в шаблоне одно, а вкладку Settings пересобирают целиком —
	// новая строка вытесняет прежнюю, и фоновый ответ не пишет в виджеты,
	// которых на экране уже нет.
	ifaceHintsWaiter func()
)

// subscribeInterfaceHints подписывает строку поля на «подсказки приехали»,
// вытесняя подписку прежней строки.
//
// Колбэк вызывается из фоновой горутины и обязан сам уйти в fyne.Do: этот
// пакет о потоке отрисовки ничего не знает.
func subscribeInterfaceHints(f func()) {
	remoteIfaceCacheMu.Lock()
	ifaceHintsWaiter = f
	remoteIfaceCacheMu.Unlock()
}

// notifyInterfaceHintsLoaded будит подписанную строку.
func notifyInterfaceHintsLoaded() {
	remoteIfaceCacheMu.Lock()
	f := ifaceHintsWaiter
	remoteIfaceCacheMu.Unlock()
	if f != nil {
		f()
	}
}

// init связывает догрузку человеческих имён локальных интерфейсов с теми же
// подписчиками: на macOS они приезжают из подпроцесса и позже самого списка.
func init() {
	netiface.SetFriendlyNamesLoadedHook(notifyInterfaceHintsLoaded)
}

// InvalidateRemoteInterfaceCache забывает известное про машину (или про все,
// если machineID пуст): переподключились к другой — прежние имена относятся к
// чужому железу.
func InvalidateRemoteInterfaceCache(machineID string) {
	remoteIfaceCacheMu.Lock()
	defer remoteIfaceCacheMu.Unlock()
	machineID = strings.TrimSpace(machineID)
	if machineID == "" {
		remoteIfaceCache = map[string]*remoteIfaceEntry{}
		return
	}
	delete(remoteIfaceCache, machineID)
}

// cachedRemoteInterfaces — что знаем про машину прямо сейчас, без единого
// сетевого вызова. loaded=false означает «ещё не спрашивали или ответа нет».
func cachedRemoteInterfaces(machineID string) (names []string, hints map[string]string, loaded bool) {
	remoteIfaceCacheMu.Lock()
	defer remoteIfaceCacheMu.Unlock()
	e := remoteIfaceCache[machineID]
	if e == nil {
		return nil, nil, false
	}
	return e.names, e.hints, e.loaded
}

// remoteInterfacesSettled — вопрос про машину закрыт: ответ приехал ЛИБО
// попытка провалилась.
//
// Отдельно от loaded, потому что состояния три, а не два: «ещё ждём» (подпись
// про загрузку), «знаем» (расшифровка) и «спросить не у кого» (подпись
// «проверить нечем»). Без этого третьего провал держал бы поле на «идёт
// загрузка» до следующей пересборки вкладки.
func remoteInterfacesSettled(machineID string) bool {
	remoteIfaceCacheMu.Lock()
	defer remoteIfaceCacheMu.Unlock()
	e := remoteIfaceCache[machineID]
	if e == nil {
		return false
	}
	return e.loaded || e.failed
}

// ensureRemoteInterfaces заводит фоновый запрос интерфейсов машины, если он
// ещё не идёт. Возврат мгновенный — ни диска, ни сети в потоке вызова.
func ensureRemoteInterfaces(machineID string) {
	machineID = strings.TrimSpace(machineID)
	if machineID == "" {
		return
	}
	remoteIfaceCacheMu.Lock()
	e := remoteIfaceCache[machineID]
	if e == nil {
		e = &remoteIfaceEntry{}
		remoteIfaceCache[machineID] = e
	}
	if e.inWork {
		// Single-flight: второй REST к той же машине ничего нового не узнает,
		// а на неотвечающей машине их накопилось бы по одному на refresh.
		remoteIfaceCacheMu.Unlock()
		return
	}
	e.inWork = true
	remoteIfaceCacheMu.Unlock()

	go func() {
		names, hints, ok := remoteInterfaces(machineID)
		remoteIfaceCacheMu.Lock()
		entry := remoteIfaceCache[machineID]
		if entry != nil {
			entry.inWork = false
			if ok {
				entry.names = names
				entry.hints = hints
				entry.loaded = true
				entry.failed = false
			} else {
				// ok=false — спросить не у кого (машина отключена, демон
				// старый, вышел срок). Прежде известное не затираем:
				// показывать пусто там, где секунду назад были имена, значит
				// сделать поле хуже, чем оно было. Но отметку о провале
				// ставим — иначе подпись под полем так и обещала бы загрузку.
				entry.failed = true
			}
		}
		remoteIfaceCacheMu.Unlock()
		// Будим подписку в ОБОИХ исходах: провал — такой же ответ на вопрос
		// «что показывать», как и успех. Без этого «Reading the machine's
		// interfaces…» висело до следующей пересборки вкладки (SPEC 113-E M6).
		notifyInterfaceHintsLoaded()
	}()
}

// WarmUpInterfaceHints прогревает источники подсказок для поля аплинка.
//
// Зовётся при сборке вкладки Settings ПРЯМО с UI-потока, и это безопасно:
// функция ничего не ждёт сама — оба тяжёлых пути уходят в фон внутри
// (ensureRemoteInterfaces заводит горутину, netiface.Warm — свою). В потоке
// отрисовки не остаётся ни сети, ни подпроцесса `networksetup`.
//
// Отсюда и требование к самой функции: любой новый источник подсказок обязан
// уходить в фон внутри себя, иначе вызов из строки вкладки заморозит её.
func WarmUpInterfaceHints(model *wizardmodels.WizardModel) {
	if model == nil {
		return
	}
	target := model.Target.Normalized()
	if target.IsRemote() {
		ensureRemoteInterfaces(target.MachineIDOrEmpty())
		return
	}
	// Локальный список сам по себе дешёв (один системный вызов), дорога
	// только расшифровка имён: на macOS это подпроцесс `networksetup`.
	netiface.Warm()
}

// interfacePickOptions собирает подсказки для поля выбора аплинка: чистые
// имена интерфейсов для выпадающего списка и карту «имя → расшифровка»
// для строки под полем.
//
// Имена и расшифровки разделены намеренно: SelectEntry подставляет выбранный
// пункт в поле дословно, поэтому в списке не может стоять «en0 — Wi-Fi (…)» —
// эта строка уехала бы в конфиг целиком.
//
// Пустой список подсказок — рабочее состояние, а не сбой: поле остаётся
// пригодным для ручного ввода.
//
// pending=true означает «про машину ещё не спрашивали или ответ в пути» — поле
// подпишет себя «загрузка…» вместо предупреждения о неизвестном имени.
func interfacePickOptions(model *wizardmodels.WizardModel, current string) (names []string, hints map[string]string, pending bool) {
	hints = map[string]string{}

	target := model.Target.Normalized()
	if target.IsRemote() {
		// Интерфейсы удалённой машины перечисляет её же демон: локальные
		// имена там значат другое железо. Спрашиваем ТОЛЬКО кэш — сам запрос
		// уходит в фон (M6).
		machineID := target.MachineIDOrEmpty()
		remoteNames, remoteHints, _ := cachedRemoteInterfaces(machineID)
		// pending — именно «ответа ЕЩЁ нет», а не «ответа нет». Провалившаяся
		// попытка вопрос закрывает: ждать больше нечего, и подпись переходит на
		// «проверить нечем».
		settled := remoteInterfacesSettled(machineID)
		ensureRemoteInterfaces(machineID)
		names = append(names, remoteNames...)
		for k, v := range remoteHints {
			hints[k] = v
		}
		return names, hints, !settled
	}

	for _, ifc := range netiface.ListOrEmpty() {
		names = append(names, ifc.Name)
		hints[ifc.Name] = ifc.Label()
	}
	return names, hints, false
}

// interfaceHintFor — строка под полем, объясняющая текущее значение.
//
// Состояния разведены, потому что и действия у них разные: пусто = штатный
// режим; известное имя = показать, что это за интерфейс; ответа ещё нет =
// сказать, что идёт загрузка; неизвестное = предупредить, но не мешать (машина
// может быть чужой или адаптер вынут).
//
// pending приходит из interfacePickOptions: подсказок нет ПОКА (запрос к машине
// в полёте) — предупреждать в этот момент значило бы пугать тем, что через
// секунду само рассосётся (SPEC 113-E).
func interfaceHintFor(model *wizardmodels.WizardModel, current string, hints map[string]string, pending bool) string {
	current = strings.TrimSpace(current)
	if current == "" {
		return locale.T("Traffic follows the system default route.")
	}
	if h, ok := hints[current]; ok {
		return h
	}
	if pending {
		return locale.T("Reading the machine's interfaces…")
	}
	if model.Target.Normalized().IsRemote() {
		// Не сверяем с локальными интерфейсами: имя относится к другой машине.
		return locale.T("Cannot verify: the machine's interfaces are unavailable.")
	}
	// SPEC 113-E: причина по фактам. Прежде любой существующий, но не попавший
	// в список интерфейс объявлялся «без IP-адреса» — для туннеля это ложь
	// (адрес у него есть), и совет получить адрес уводил не туда.
	switch netiface.Fitness(current) {
	case netiface.UnfitNoAddress:
		return "⚠ " + locale.T("no IP address — no traffic will go through it")
	case netiface.UnfitTunnel:
		return "⚠ " + locale.T("this is a tunnel — the core cannot use it as an uplink")
	case netiface.UnfitLoopback:
		return "⚠ " + locale.T("loopback — no traffic will leave the machine through it")
	case netiface.UnfitFit:
		// Годен, но подсказок нет: список интерфейсов брали до того, как его
		// подняли (или он вообще не запрашивался). Пугать нечем.
		return locale.T("Traffic will go through this interface.")
	}
	return "⚠ " + locale.T("not found on this machine")
}
