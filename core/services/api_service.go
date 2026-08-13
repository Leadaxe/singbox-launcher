package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"singbox-launcher/api"
	"singbox-launcher/core/config"
	"singbox-launcher/internal/ctxutil"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// ProxyScope — чьи прокси описывает состояние: своё ядро или удалённая
// машина (SPEC 098).
type ProxyScope int

const (
	// ScopeLocal — локальное ядро (вкладка Local).
	ScopeLocal ProxyScope = iota
	// ScopeRemote — выбранная удалённая машина (вкладка Remote).
	ScopeRemote
)

// proxyScopeState — состояние списка прокси ОДНОЙ области.
//
// Всё, что зависит от того, чьё ядро мы сейчас показываем: сам список, выбор
// группы, активный узел, запомненные выборы и ошибки пинга. Хранится по
// экземпляру на область, поэтому Local и Remote не видят данных друг друга.
type proxyScopeState struct {
	SelectedClashGroup string
	ProxiesList        []api.ProxyInfo
	ActiveProxyName    string
	SelectedIndex      int
	// LastSelectedProxyByGroup maps selector group -> last selected proxy name.
	// This allows remembering the last proxy per selector (group) independently.
	LastSelectedProxyByGroup map[string]string
	// LastPingError maps proxy name -> last ping error message (for tooltip when button shows "Error").
	LastPingError map[string]string
}

func newProxyScopeState() *proxyScopeState {
	return &proxyScopeState{
		ProxiesList:              []api.ProxyInfo{},
		SelectedIndex:            -1,
		LastSelectedProxyByGroup: make(map[string]string),
		LastPingError:            make(map[string]string),
	}
}

// APIService manages Clash API interactions and proxy list management.
// It encapsulates all API-related state and operations to reduce AppController complexity.
type APIService struct {
	// Clash API configuration
	BaseURL string
	Token   string
	Enabled bool

	// Auto-load state
	AutoLoadInProgress bool
	AutoLoadMutex      sync.Mutex
	// AutoLoadGeneration растёт при каждом новом запуске автозагрузки.
	//
	// Retry-цикл живёт до ~100 секунд. Без поколения смена группы во время
	// его работы приводила к тому, что поздний ответ СТАРОГО цикла
	// перезаписывал список уже выбранной группы: в UI оставались узлы от
	// предыдущей. Цикл сверяет поколение перед каждой попыткой и перед
	// записью результата, и молча выходит, если его вытеснил новый запуск.
	AutoLoadGeneration uint64

	// Proxy list state (protected by StateMutex)
	StateMutex sync.RWMutex

	// scope — какой области принадлежит состояние, читаемое сейчас через
	// геттеры: ScopeLocal (своё ядро) или ScopeRemote (выбранная машина).
	//
	// SPEC 098: у вкладок Local и Remote независимые списки. Раньше поля
	// состояния были одни на всё приложение, и вкладки затирали данные друг
	// друга: перейдя на Remote, пользователь видел узлы локального ядра, пока
	// не нажмёт Start на сервере. Разделение виджетов этого не лечило —
	// данные-то оставались общими.
	scope ProxyScope
	// scopes хранит состояние КАЖДОЙ области отдельно. Активная выбирается
	// полем scope; переключение вкладки меняет его, а не перезаписывает
	// данные.
	scopes map[ProxyScope]*proxyScopeState

	// Dependencies (passed from AppController)
	ConfigPath            string
	RunningStateIsRunning func() bool
	OnProxiesUpdated      func() // Called when proxies are updated
	OnProxySwitched       func() // Called when proxy is switched

	// transportOverride — альтернативный транспорт proxy-операций
	// (daemon-режим: gRPC к lxd). nil = классический Clash HTTP.
	// Protected by StateMutex.
	transportOverride ProxyTransport
}

// NewAPIService creates and initializes a new APIService instance.
func NewAPIService(configPath string,
	runningStateIsRunning func() bool,
	onProxiesUpdated func(), onProxySwitched func()) (*APIService, error) {
	apiSvc := &APIService{
		ConfigPath:            configPath,
		RunningStateIsRunning: runningStateIsRunning,
		OnProxiesUpdated:      onProxiesUpdated,
		OnProxySwitched:       onProxySwitched,
		scope:                 ScopeLocal,
		scopes: map[ProxyScope]*proxyScopeState{
			ScopeLocal:  newProxyScopeState(),
			ScopeRemote: newProxyScopeState(),
		},
	}

	// Load Clash API configuration from config.json
	if base, tok, err := api.LoadClashAPIConfig(configPath); err != nil {
		debuglog.WarnLog("NewAPIService: Clash API config error: %v", err)
		apiSvc.BaseURL = ""
		apiSvc.Token = ""
		apiSvc.Enabled = false
	} else {
		apiSvc.BaseURL = base
		apiSvc.Token = tok
		apiSvc.Enabled = true
	}

	// Initialize SelectedClashGroup from config.
	//
	// Группа из локального config.json — свойство ЛОКАЛЬНОГО ядра, поэтому
	// заполняется только область ScopeLocal. У удалённой машины свои группы,
	// их отдаёт она сама (RemoteDaemonGroups).
	if apiSvc.Enabled {
		_, defaultSelector, err := config.GetSelectorGroupsFromConfig(configPath)
		if err != nil {
			debuglog.WarnLog("NewAPIService: Failed to get selector groups: %v", err)
			apiSvc.scopes[ScopeLocal].SelectedClashGroup = "proxy-out" // Default fallback
		} else {
			apiSvc.scopes[ScopeLocal].SelectedClashGroup = defaultSelector
			debuglog.DebugLog("NewAPIService: Initialized SelectedClashGroup: %s", defaultSelector)
		}
	}

	return apiSvc, nil
}

// stateLocked возвращает состояние активной области для ЧТЕНИЯ.
//
// Годится под RLock: обе области создаёт конструктор, поэтому обращение к
// карте здесь не мутирующее. Ленивое создание было бы записью под RLock, то
// есть гонкой на ровном месте — для записи есть mutableStateLocked.
//
// Пустое состояние возвращается только для APIService, собранного литералом
// (так делают тесты): читать из него можно, ронять приложение — нет.
func (apiSvc *APIService) stateLocked() *proxyScopeState {
	if st := apiSvc.scopes[apiSvc.scope]; st != nil {
		return st
	}
	return newProxyScopeState()
}

// mutableStateLocked возвращает состояние активной области для ЗАПИСИ,
// создавая его при необходимости. Вызывать только под StateMutex.Lock().
func (apiSvc *APIService) mutableStateLocked() *proxyScopeState {
	if apiSvc.scopes == nil {
		apiSvc.scopes = make(map[ProxyScope]*proxyScopeState, 2)
	}
	st := apiSvc.scopes[apiSvc.scope]
	if st == nil {
		st = newProxyScopeState()
		apiSvc.scopes[apiSvc.scope] = st
	}
	return st
}

// SetProxyScope переключает активную область (SPEC 098).
//
// Вызывается при смене вкладки: дальше все геттеры и сеттеры адресуют
// состояние ЭТОЙ области. Данные предыдущей остаются нетронутыми — вернувшись
// на вкладку, пользователь видит ровно то, что там было.
func (apiSvc *APIService) SetProxyScope(scope ProxyScope) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	apiSvc.scope = scope
}

// ProxyScopeOf возвращает активную область.
func (apiSvc *APIService) ProxyScopeOf() ProxyScope {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	return apiSvc.scope
}

// SelectedClashGroupIn / SetSelectedClashGroupIn адресуют КОНКРЕТНУЮ область,
// а не активную.
//
// Нужны конструкторам панелей: обе строятся на старте, когда активна
// ScopeLocal, и «активные» аксессоры записали бы группу удалённой машины в
// local-состояние, оставив remote пустым.
func (apiSvc *APIService) SelectedClashGroupIn(scope ProxyScope) string {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	if st := apiSvc.scopes[scope]; st != nil {
		return st.SelectedClashGroup
	}
	return ""
}

func (apiSvc *APIService) SetSelectedClashGroupIn(scope ProxyScope, group string) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	if apiSvc.scopes == nil {
		apiSvc.scopes = make(map[ProxyScope]*proxyScopeState, 2)
	}
	st := apiSvc.scopes[scope]
	if st == nil {
		st = newProxyScopeState()
		apiSvc.scopes[scope] = st
	}
	st.SelectedClashGroup = group
}

// ResetScope очищает состояние области — список, выбор и ошибки пинга.
//
// Нужен, когда область перестала быть осмысленной: машина отключена или
// снят её транспорт. Показывать при этом прежние узлы значило бы выдавать
// данные машины, с которой разговора уже нет.
func (apiSvc *APIService) ResetScope(scope ProxyScope) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	if apiSvc.scopes == nil {
		apiSvc.scopes = make(map[ProxyScope]*proxyScopeState, 2)
	}
	apiSvc.scopes[scope] = newProxyScopeState()
}

// SetProxiesList safely sets the proxies list with mutex protection.
func (apiSvc *APIService) SetProxiesList(proxies []api.ProxyInfo) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	apiSvc.mutableStateLocked().ProxiesList = proxies
}

// GetProxiesList safely gets a copy of the proxies list with mutex protection.
func (apiSvc *APIService) GetProxiesList() []api.ProxyInfo {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	// Return a copy to prevent external modifications
	result := make([]api.ProxyInfo, len(apiSvc.stateLocked().ProxiesList))
	copy(result, apiSvc.stateLocked().ProxiesList)
	return result
}

// SetActiveProxyName safely sets the active proxy name with mutex protection.
func (apiSvc *APIService) SetActiveProxyName(name string) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	apiSvc.mutableStateLocked().ActiveProxyName = name
}

// GetActiveProxyName safely gets the active proxy name with mutex protection.
func (apiSvc *APIService) GetActiveProxyName() string {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	return apiSvc.stateLocked().ActiveProxyName
}

// SetSelectedIndex safely sets the selected index with mutex protection.
func (apiSvc *APIService) SetSelectedIndex(index int) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	apiSvc.mutableStateLocked().SelectedIndex = index
}

// GetSelectedIndex safely gets the selected index with mutex protection.
func (apiSvc *APIService) GetSelectedIndex() int {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	return apiSvc.stateLocked().SelectedIndex
}

// SetLastSelectedProxyForGroup safely sets the last selected proxy name for a selector group with mutex protection.
func (apiSvc *APIService) SetLastSelectedProxyForGroup(group, name string) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	if apiSvc.mutableStateLocked().LastSelectedProxyByGroup == nil {
		apiSvc.mutableStateLocked().LastSelectedProxyByGroup = make(map[string]string)
	}
	apiSvc.mutableStateLocked().LastSelectedProxyByGroup[group] = name
}

// GetLastSelectedProxyForGroup safely gets the last selected proxy name for a selector group with mutex protection.
func (apiSvc *APIService) GetLastSelectedProxyForGroup(group string) string {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	if apiSvc.stateLocked().LastSelectedProxyByGroup == nil {
		return ""
	}
	return apiSvc.stateLocked().LastSelectedProxyByGroup[group]
}

// SetLastPingError stores the last ping error message for a proxy (for tooltip when button shows "Error").
func (apiSvc *APIService) SetLastPingError(proxyName, errMsg string) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	if apiSvc.mutableStateLocked().LastPingError == nil {
		apiSvc.mutableStateLocked().LastPingError = make(map[string]string)
	}
	if errMsg == "" {
		delete(apiSvc.mutableStateLocked().LastPingError, proxyName)
	} else {
		apiSvc.mutableStateLocked().LastPingError[proxyName] = errMsg
	}
}

// GetLastPingError returns the last ping error message for a proxy, or empty string.
func (apiSvc *APIService) GetLastPingError(proxyName string) string {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	if apiSvc.stateLocked().LastPingError == nil {
		return ""
	}
	return apiSvc.stateLocked().LastPingError[proxyName]
}

// GetSelectedClashGroup safely gets the selected Clash group.
func (apiSvc *APIService) GetSelectedClashGroup() string {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	return apiSvc.stateLocked().SelectedClashGroup
}

// SetSelectedClashGroup safely sets the selected Clash group.
func (apiSvc *APIService) SetSelectedClashGroup(group string) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	apiSvc.mutableStateLocked().SelectedClashGroup = group
}

// GetClashAPIConfig safely gets Clash API configuration.
func (apiSvc *APIService) GetClashAPIConfig() (baseURL, token string, enabled bool) {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	return apiSvc.BaseURL, apiSvc.Token, apiSvc.Enabled
}

// ReloadClashAPIConfig reloads Clash API configuration from config.json file.
// This should be called when config might have changed (e.g., after wizard updates).
func (apiSvc *APIService) ReloadClashAPIConfig() error {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()

	debuglog.InfoLog("ReloadClashAPIConfig: Reloading Clash API configuration from config.json...")

	// Load Clash API configuration from config.json
	if base, tok, err := api.LoadClashAPIConfig(apiSvc.ConfigPath); err != nil {
		debuglog.WarnLog("ReloadClashAPIConfig: Clash API config error: %v", err)
		apiSvc.BaseURL = ""
		apiSvc.Token = ""
		apiSvc.Enabled = false
		// Локальный config.json описывает ЛОКАЛЬНОЕ ядро — правим только его
		// область. Через активную область перезагрузка, сделанная пока открыта
		// вкладка Remote, затёрла бы группу удалённой машины.
		apiSvc.scopes[ScopeLocal].SelectedClashGroup = ""
		return fmt.Errorf("failed to reload Clash API config: %w", err)
	} else {
		oldEnabled := apiSvc.Enabled
		apiSvc.BaseURL = base
		apiSvc.Token = tok
		apiSvc.Enabled = true
		debuglog.InfoLog("ReloadClashAPIConfig: Successfully reloaded - BaseURL: %s, Enabled: %v (was %v)", base, true, oldEnabled)
	}

	// Reload SelectedClashGroup from config if API is enabled
	if apiSvc.Enabled {
		_, defaultSelector, err := config.GetSelectorGroupsFromConfig(apiSvc.ConfigPath)
		if err != nil {
			debuglog.WarnLog("ReloadClashAPIConfig: Failed to get selector groups: %v", err)
			// Keep existing SelectedClashGroup if we can't read new one
		} else {
			apiSvc.scopes[ScopeLocal].SelectedClashGroup = defaultSelector
			debuglog.DebugLog("ReloadClashAPIConfig: Updated SelectedClashGroup: %s", defaultSelector)
		}
	}

	return nil
}

// AutoLoadProxies attempts to load proxies across 14 retry attempts with
// escalating back-off (1,3,3, 5×5, 10×4, 15×2 seconds). Runs at most one
// instance at a time; the in-progress flag is always cleared on exit — the
// goroutine's defer covers every path, including the currentGroup=="" early
// return that previously leaked it (wedging auto-load until restart).
func (apiSvc *APIService) AutoLoadProxies(ctx context.Context) {
	// clearInProgress гасит флаг ТОЛЬКО если нас не вытеснил новый запуск —
	// иначе выходящий старый цикл сбросил бы флаг работающего нового.
	var myGenRef *uint64
	clearInProgress := func() {
		apiSvc.AutoLoadMutex.Lock()
		if myGenRef == nil || apiSvc.AutoLoadGeneration == *myGenRef {
			apiSvc.AutoLoadInProgress = false
		}
		apiSvc.AutoLoadMutex.Unlock()
	}

	// Новый запуск ВЫТЕСНЯЕТ предыдущий, а не отбрасывается сам.
	//
	// Раньше здесь стоял ранний return по AutoLoadInProgress, и смена группы
	// во время retry-цикла (до ~100 с) просто игнорировалась: пользователь
	// выбирал другую группу, а список продолжал жить от старой. Поколение
	// решает и это, и обратную гонку — поздний ответ вытесненного цикла.
	apiSvc.AutoLoadMutex.Lock()
	apiSvc.AutoLoadGeneration++
	myGen := apiSvc.AutoLoadGeneration
	apiSvc.AutoLoadInProgress = true
	apiSvc.AutoLoadMutex.Unlock()
	myGenRef = &myGen

	// isStale сообщает, вытеснен ли этот запуск более новым.
	isStale := func() bool {
		apiSvc.AutoLoadMutex.Lock()
		defer apiSvc.AutoLoadMutex.Unlock()
		return apiSvc.AutoLoadGeneration != myGen
	}

	if _, err := apiSvc.wireTransport(); err != nil {
		clearInProgress()
		debuglog.DebugLog("AutoLoadProxies: no proxy transport available (%v), skipping", err)
		return
	}

	selectedGroup := apiSvc.GetSelectedClashGroup()
	if selectedGroup == "" {
		clearInProgress()
		debuglog.DebugLog("AutoLoadProxies: No group selected, skipping")
		return
	}

	intervals := []time.Duration{1, 3, 3, 5, 5, 5, 5, 5, 10, 10, 10, 10, 15, 15}

	go func() {
		// Always release the in-progress flag, whichever path exits.
		defer clearInProgress()

		for attempt, interval := range intervals {
			// Check if context is cancelled
			select {
			case <-ctx.Done():
				debuglog.DebugLog("AutoLoadProxies: Stopped (context cancelled)")
				return
			default:
			}

			// Нас вытеснил более новый запуск (пользователь сменил группу) —
			// дальше работать бессмысленно, результат всё равно устарел.
			if isStale() {
				debuglog.DebugLog("AutoLoadProxies: Superseded by a newer run, stopping")
				return
			}

			if attempt > 0 {
				if err := ctxutil.SleepWithContext(ctx, interval*time.Second); err != nil {
					debuglog.DebugLog("AutoLoadProxies: Stopped during wait (context cancelled)")
					return
				}
			}

			// Раньше здесь стоял skip по RunningStateIsRunning(), и при любом
			// ядре, поднятом НЕ лаунчером, автозагрузка не выполнялась:
			// tracked PID = -1, флаг всегда false. Так ломалось переключение
			// групп и с чужим sing-box'ом (remote endpoint, SPEC 064), и с
			// локальным ядром, запущенным вручную, — API отвечал, а список
			// оставался от предыдущей группы.
			//
			// Отдельная проверка не нужна: недостижимый endpoint отсеется
			// ошибкой самого запроса ниже и уйдёт в следующий retry — тот же
			// результат, но без ложного пропуска.
			debuglog.DebugLog("AutoLoadProxies: Attempt %d/%d to load proxies for group '%s'", attempt+1, len(intervals), selectedGroup)

			// Get current group (it might have changed)
			currentGroup := apiSvc.GetSelectedClashGroup()

			if currentGroup == "" {
				debuglog.DebugLog("AutoLoadProxies: Group cleared, stopping attempts")
				return
			}

			if platform.IsSleeping() {
				debuglog.DebugLog("AutoLoadProxies: Skipping attempt - system sleeping")
				continue
			}

			// Транспорт строим на каждой попытке: endpoint/override могли
			// смениться пока retry-цикл спал.
			transport, terr := apiSvc.wireTransport()
			if terr != nil {
				debuglog.DebugLog("AutoLoadProxies: transport unavailable (%v)", terr)
				continue
			}

			// Try to load proxies
			proxies, now, err := transport.GroupProxies(currentGroup)
			if err != nil {
				if errors.Is(err, api.ErrPlatformInterrupt) {
					debuglog.DebugLog("AutoLoadProxies: Aborted (platform interrupt/sleep)")
				} else {
					debuglog.DebugLog("AutoLoadProxies: Attempt %d failed: %v", attempt+1, err)
				}
				continue
			}

			// Success - update proxies list.
			//
			// Поколение сверяем ЕЩЁ РАЗ перед записью: запрос летел по сети, и
			// за это время пользователь мог сменить группу. Без проверки
			// поздний ответ старого цикла затирал список уже выбранной группы.
			if isStale() {
				debuglog.DebugLog("AutoLoadProxies: Result for group '%s' dropped — superseded", currentGroup)
				return
			}
			fyne.Do(func() {
				if isStale() {
					return
				}
				apiSvc.SetProxiesList(proxies)
				apiSvc.SetActiveProxyName(now)

				// Notify about proxies update
				if apiSvc.OnProxiesUpdated != nil {
					apiSvc.OnProxiesUpdated()
				}
			})

			// Проверяем, есть ли сохраненный прокси для текущей группы, и переключаемся на него, если он отличается от текущего
			// Делаем это после обновления UI, чтобы не блокировать
			lastSelected := apiSvc.GetLastSelectedProxyForGroup(currentGroup)
			if lastSelected != "" && lastSelected != now {
				// Проверяем, что сохраненный прокси существует в списке
				proxyExists := false
				for _, proxy := range proxies {
					if proxy.Name == lastSelected {
						proxyExists = true
						break
					}
				}
				if proxyExists {
					debuglog.DebugLog("AutoLoadProxies: Switching to saved proxy '%s' (current: '%s') for group '%s'", lastSelected, now, currentGroup)
					// Переключаемся на сохраненный прокси (мы уже в goroutine, дополнительная не нужна)
					if err := apiSvc.SwitchProxy(currentGroup, lastSelected); err != nil {
						debuglog.WarnLog("AutoLoadProxies: Failed to switch to saved proxy '%s' for group '%s': %v", lastSelected, currentGroup, err)
						// Не критично, продолжаем с текущим прокси
					}
				}
			}

			debuglog.InfoLog("AutoLoadProxies: Successfully loaded %d proxies for group '%s' on attempt %d", len(proxies), currentGroup, attempt+1)
			return // Success, stop retrying
		}

		debuglog.WarnLog("AutoLoadProxies: All %d attempts failed", len(intervals))
	}()
}

// SwitchProxy switches to the specified proxy in the selected group.
func (apiSvc *APIService) SwitchProxy(group, proxyName string) error {
	transport, err := apiSvc.wireTransport()
	if err != nil {
		return err
	}

	if err := transport.SwitchProxy(group, proxyName); err != nil {
		debuglog.WarnLog("SwitchProxy: group=%q → %q failed: %v", group, proxyName, err)
		return fmt.Errorf("failed to switch proxy: %w", err)
	}
	// Логируем факт и адресата: без этого нельзя отличить «лаунчер не послал
	// команду» от «ядро её приняло, но соединения не разорвало» — а лечение у
	// этих случаев разное.
	debuglog.InfoLog("SwitchProxy: group=%q → %q", group, proxyName)

	apiSvc.SetActiveProxyName(proxyName)
	// Сохраняем последний выбранный прокси для текущей группы для автоматического переключения при следующем старте
	apiSvc.SetLastSelectedProxyForGroup(group, proxyName)

	// Notify about proxy switch
	if apiSvc.OnProxySwitched != nil {
		apiSvc.OnProxySwitched()
	}

	return nil
}
