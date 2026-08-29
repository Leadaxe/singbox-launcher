package tabs

import (
	"strings"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"singbox-launcher/core/netiface"
	wizardtemplate "singbox-launcher/core/template"
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

// RemoteRawIface — интерфейс удалённой машины КАК ЕГО ОТДАЛ ДЕМОН, до всякого
// отбора.
//
// Сырой, а не отфильтрованный, потому что потребителей у одного ответа уже два
// и фильтры у них разные: аплинку нужны интерфейсы с адресом (включая чужие
// туннели), LAN-стороне — безадресные порты, но без туннелей вовсе. Отдавать
// готовый список значило бы либо второй REST к той же машине, либо один фильтр
// на две несовместимые роли.
type RemoteRawIface struct {
	Name  string
	Up    bool
	Addrs []string
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
type RemoteInterfaceProvider func(machineID string) (raw []RemoteRawIface, ok bool)

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

func remoteInterfaces(machineID string) ([]RemoteRawIface, bool) {
	remoteIfaceMu.RLock()
	p := remoteIfaceProvider
	remoteIfaceMu.RUnlock()
	if p == nil || strings.TrimSpace(machineID) == "" {
		return nil, false
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
//
// Хранится СЫРОЙ ответ демона, а не готовый список имён: у одного ответа два
// потребителя с разными фильтрами (аплинк и LAN-порты), и второй запрос к той
// же машине ради второго фильтра ничего нового не узнал бы.
type remoteIfaceEntry struct {
	raw    []RemoteRawIface
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

// cachedRemoteRaw — сырой ответ демона по машине, без единого сетевого вызова.
// loaded=false означает «ещё не спрашивали или ответа нет».
func cachedRemoteRaw(machineID string) (raw []RemoteRawIface, loaded bool) {
	remoteIfaceCacheMu.Lock()
	defer remoteIfaceCacheMu.Unlock()
	e := remoteIfaceCache[machineID]
	if e == nil {
		return nil, false
	}
	return e.raw, e.loaded
}

// cachedRemoteInterfaces — интерфейсы машины, годные в АПЛИНК, из кэша.
//
// Фильтр применяется на чтении, а не при записи в кэш: ответ демона один, а
// ролей у него две (см. remoteIfaceEntry.raw).
func cachedRemoteInterfaces(machineID string) (names []string, hints map[string]string, loaded bool) {
	raw, loaded := cachedRemoteRaw(machineID)
	names = make([]string, 0, len(raw))
	hints = make(map[string]string, len(raw))
	for _, r := range raw {
		ifc, ok := netiface.FromRemote(r.Name, r.Up, r.Addrs)
		if !ok {
			continue
		}
		names = append(names, ifc.Name)
		// Та же расшифровка, что для локальных: чужой туннель роутера (awg1) —
		// законный аплинк, но подпись обязана предупредить, что трафик уйдёт в
		// него, а не в физическую сеть (SPEC 113-F).
		hints[ifc.Name] = InterfaceHintText(ifc)
	}
	return names, hints, loaded
}

// cachedRemoteLANCandidates — интерфейсы машины, годные в LAN-порты
// (tun.include_interface), из ТОГО ЖЕ кэша.
func cachedRemoteLANCandidates(machineID string) (ifaces []netiface.Iface, loaded bool) {
	raw, loaded := cachedRemoteRaw(machineID)
	ifaces = make([]netiface.Iface, 0, len(raw))
	for _, r := range raw {
		ifc, ok := netiface.FromRemoteLAN(r.Name, r.Up, r.Addrs)
		if !ok {
			continue
		}
		ifaces = append(ifaces, ifc)
	}
	return ifaces, loaded
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
	if e.inWork || e.loaded {
		// Single-flight + кэш: второй REST к той же машине ничего нового не
		// узнает — и уже полученный ответ повторно не спрашиваем (оба
		// фильтра, аплинковый и LAN, живут на одном ответе демона; смена
		// машины забывает его через InvalidateRemoteInterfaceCache).
		// Провал (failed) кэшем не считается: следующий ensure — retry.
		remoteIfaceCacheMu.Unlock()
		return
	}
	e.inWork = true
	remoteIfaceCacheMu.Unlock()

	go func() {
		raw, ok := remoteInterfaces(machineID)
		remoteIfaceCacheMu.Lock()
		entry := remoteIfaceCache[machineID]
		if entry != nil {
			entry.inWork = false
			if ok {
				entry.raw = raw
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
		hints[ifc.Name] = InterfaceHintText(ifc)
	}
	return names, hints, false
}

// InterfaceHintText — расшифровка интерфейса для строки под полем.
//
// SPEC 113-F: у чужого туннеля к обычной подписи добавляется предупреждение.
// Он законный аплинк (ровно это и нужно тому, у кого поднят системный awg1),
// но трафик ядра уйдёт В НЕГО, а не в физическую сеть — и адрес на выходе
// будет его. Умолчать значит оставить пользователя гадать, почему выход не тот.
//
// Отдельная функция, а не метод Label(): Label живёт в core и о локали не
// знает, а сама подпись обязана переводиться.
//
// Экспортирована ради ОДНОГО источника подписи на оба пути: локальный список
// строится здесь, а удалённый — в пакете `ui` (там транспорт машины). Разъехавшись,
// они начали бы описывать один и тот же туннель по-разному.
func InterfaceHintText(ifc netiface.Iface) string {
	if !ifc.IsTunnel {
		return ifc.Label()
	}
	return ifc.Label() + " — " + locale.T("tunnel: traffic will go out through it")
}

// autoDetectInterfaceVar — переменная шаблона, несущая route.auto_detect_interface.
const autoDetectInterfaceVar = "auto_detect_interface"

// interfacePickSuppressed — выбор интерфейса не имеет силы, потому что включено
// автоопределение.
//
// В sing-box route.auto_detect_interface ПЕРЕБИВАЕТ route.default_interface:
// при включённом автоопределении ядро молча игнорирует выбранное имя. Активный
// дропдаун в этот момент — обман: пользователь выбирает то, что ни на что не
// влияет, и потом ищет причину «настройка не применилась» в чём угодно, кроме
// галки выше.
//
// Проверка живёт в коде, а не только в гейте шаблона (`#enable`), намеренно:
// приоритет — свойство САМОГО ЯДРА, а не редактируемой пользователем разметки.
// Шаблон лежит на диске у пользователя, переживает обновления лаунчера и может
// быть отредактирован руками — приоритет ядра от этого не исчезнет. Гейту
// шаблона это не мешает: он вычисляется отдельно, и оба условия сводятся через
// «и» (см. interfaceRowEnabled).
//
// Отсутствие переменной в шаблоне = автоопределения нет → не подавляем.
func interfacePickSuppressed(resolved map[string]wizardtemplate.ResolvedVar) bool {
	r, ok := resolved[autoDetectInterfaceVar]
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(r.Scalar), "true")
}

// interfaceRowEnabled — итоговое состояние строки выбора аплинка: гейт шаблона
// И отсутствие подавления автоопределением.
//
// Чистая функция, потому что здесь и только здесь живёт вся логика «активен ли
// пикер»: и первичная сборка вкладки, и реактивный пересчёт по клику зовут её,
// и разойтись им негде.
func interfaceRowEnabled(templateGate bool, resolved map[string]wizardtemplate.ResolvedVar) bool {
	return templateGate && !interfacePickSuppressed(resolved)
}

// interfaceHintForRow — подпись под полем с учётом подавления.
//
// Когда выбор подавлен, расшифровывать выбранное имя незачем и вредно:
// «Traffic will go through this interface» — прямая ложь при включённом
// автоопределении, трафик пойдёт через интерфейс, который выберет ядро. Поэтому
// подпись подменяется объяснением, ПОЧЕМУ поле погашено, — иначе пользователю
// пришлось бы догадываться самому.
func interfaceHintForRow(model *wizardmodels.WizardModel, current string, hints map[string]string, pending, suppressed bool) string {
	if suppressed {
		return locale.T("Auto-detect is on — the core picks the interface itself.")
	}
	return interfaceHintFor(model, current, hints, pending)
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
	case netiface.UnfitFitTunnel:
		// SPEC 113-F: чужой туннель — законный выбор, а не ошибка. Но
		// последствие нетривиальное: трафик ядра уйдёт в этот туннель, а не в
		// физическую сеть, и адрес на выходе будет его. Молчать об этом
		// значит оставить пользователя гадать, почему выход не тот.
		return locale.T("Tunnel — traffic will go out through it.")
	case netiface.UnfitFit:
		// Годен, но подсказок нет: список интерфейсов брали до того, как его
		// подняли (или он вообще не запрашивался). Пугать нечем.
		return locale.T("Traffic will go through this interface.")
	}
	return "⚠ " + locale.T("not found on this machine")
}
