package ui

import (
	"os"
	"sync"

	"singbox-launcher/core/config"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
)

// SPEC 113-E (M4, M5) — сбор списка selector-групп: чей это список и откуда
// его читать.
//
// Раньше сбор жил прямо в замыкании панели и выполнялся в потоке вызова. Из
// пяти путей вызова четыре приходят С UI-ПОТОКА (UpdateUI → ResetAPIStateFunc,
// колбэки успешного теста связи, клики по ↻ и по пульсу), а сбор ходит на диск
// (config.json) и в сеть (gRPC к демону машины). Недоступный роутер держал
// главный цикл на весь dial-дедлайн — это и был «зависший Stop».
//
// Здесь оставлена ЧИСТАЯ часть: только чтение источника, без единого виджета.
// Её и зовёт горутина; мутации виджетов панель делает сама через fyne.Do.

// selectorGroupsSnapshot — что удалось прочитать у источника групп.
type selectorGroupsSnapshot struct {
	// options — список групп. Пустой список — законное состояние: у машины
	// ядро ещё не поднялось, у Local нет config.json.
	options []string
	// defaultGroup — группа, которую источник считает первой/основной.
	defaultGroup string
	// clearAll — у панели нет собеседника вовсе (Remote без выбранной
	// машины): дропдаун обязан опустеть, а не показывать прежние группы.
	clearAll bool
	// err — ошибка чтения ЛОКАЛЬНОГО config.json. Ошибка удалённого источника
	// сюда не попадает: она уже залогирована, а список групп машины при ней
	// остаётся пустым (подставлять локальные нельзя — это чужое ядро).
	err error
}

// remoteGroupsSource — откуда берутся группы удалённой машины.
//
// Переменная, а не прямой вызов: тест обязан уметь поймать САМ ФАКТ обращения
// к машине из local-панели (M5), а живой RemoteDaemonGroups для этого пришлось
// бы поднимать вместе с gRPC-транспортом.
var remoteGroupsSource = RemoteDaemonGroups

// collectSelectorGroups читает группы того ядра, которое ведёт панель scope.
//
// Гейт по scope стоит ПЕРЕД обращением к удалённому демону намеренно (M5):
// RemoteDaemonGroups() — глобальное состояние приложения, и панель Local,
// спросив его первой, получала группы РОУТЕРА при подключённой машине. Это
// повтор класса «remote-override глобальный»: у локальной панели источник
// групп ровно один — локальный config.json.
//
// Функция ходит на диск и в сеть — звать её с UI-потока нельзя.
func collectSelectorGroups(scope services.ProxyScope, configPath string) selectorGroupsSnapshot {
	if scope == services.ScopeLocal {
		options, def, err := config.GetSelectorGroupsFromConfig(configPath)
		return selectorGroupsSnapshot{options: options, defaultGroup: def, err: err}
	}

	// Remote: собеседник — сам демон машины.
	remoteGroups, isRemote, groupsErr := remoteGroupsSource()
	if !isRemote {
		// Машина не выбрана: групп нет и быть не может. Оставить прежние
		// значило бы показывать группы отключённой машины как её собственные.
		return selectorGroupsSnapshot{clearAll: true}
	}
	snap := selectorGroupsSnapshot{options: remoteGroups}
	if len(remoteGroups) > 0 {
		snap.defaultGroup = remoteGroups[0]
	} else if groupsErr != nil {
		// Машина недоступна или ядро не запущено — список пуст, но локальные
		// группы подставлять нельзя: это чужое ядро.
		debuglog.WarnLog("clash_api_tab: remote groups unavailable: %v", groupsErr)
	}
	return snap
}

// logSelectorConfigErr объясняет неудачу чтения локального config.json.
//
// Отдельной функцией, потому что зовётся из двух мест (построение панели и
// фоновое перечитывание), а различие между «файла ещё нет» и «файл битый»
// стоит сохранять: первое — холодный старт, второе — настоящая поломка.
func logSelectorConfigErr(err error) {
	if err == nil {
		return
	}
	if os.IsNotExist(err) {
		// Cold-start: config.json ещё не существует (пользователь не нажал
		// Save). Не повод писать ERROR.
		debuglog.DebugLog("clash_api_tab: config.json not present yet (cold start): %v", err)
		return
	}
	debuglog.ErrorLog("clash_api_tab: failed to get selector groups: %v", err)
}

// selectorReloader сериализует перечитывание групп одной панели.
//
// Смысл — не «ускорить», а не дать одновременным вызовам разъехаться: Reset,
// клик по ↻ и колбэк успешного теста связи приходят пачкой, и без схлопывания
// панель получила бы три параллельных gRPC-запроса, чьи ответы применились бы
// в произвольном порядке. Схема та же, что у ping-all: пока сбор идёт,
// повторный запрос только помечает «нужен ещё один прогон», а не запускает
// второй.
type selectorReloader struct {
	mu      sync.Mutex
	running bool
	pending bool

	// collect — сбор данных (диск/сеть), выполняется в горутине.
	collect func() selectorGroupsSnapshot
	// apply — применение к виджетам; обязан сам уйти в fyne.Do.
	apply func(selectorGroupsSnapshot)
}

// Request просит перечитать группы. Возврат мгновенный: работа уходит в
// горутину, и ни один путь вызова не ждёт диска или сети.
func (r *selectorReloader) Request() {
	if r == nil || r.collect == nil {
		return
	}
	r.mu.Lock()
	if r.running {
		// Сбор уже идёт — его результат может оказаться снятым ДО причины
		// текущего вызова (сменилась машина, ядро перезапустилось), поэтому
		// нужен ещё один прогон, а не молчаливый отказ.
		r.pending = true
		r.mu.Unlock()
		return
	}
	r.running = true
	r.mu.Unlock()

	go r.loop()
}

func (r *selectorReloader) loop() {
	for {
		snap := r.collect()
		if r.apply != nil {
			r.apply(snap)
		}

		r.mu.Lock()
		if !r.pending {
			r.running = false
			r.mu.Unlock()
			return
		}
		r.pending = false
		r.mu.Unlock()
	}
}
