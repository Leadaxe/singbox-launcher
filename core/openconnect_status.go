// File openconnect_status.go — состояние endpoint'ов openconnect у
// работающего ядра и ответы на челленджи второго фактора / SSO (SPEC 167
// §2.5). Образец — tailscale_status.go.
//
// Источник — стрим SubscribeOpenConnectStatus: статус всех openconnect-
// endpoint'ов разом, первый кадр сразу после подписки (висящий челлендж
// виден при открытии лаунчера). Канал есть только у gRPC-путей: локальный
// демон (DaemonBackend) и транспорт удалённой машины. В классическом режиме
// своего ядра (`sing-box run`) канала нет, и для Local здесь ничего не
// запускается; сервис api в конфиг не добавляется (решение владельца).
//
// Доменные типы, конвертер и кеш живут в services (цикл импорта, тот же
// довод, что у Tailscale); здесь — выбор источника по цели и уведомление UI
// о новом challenge.id.
package core

import (
	"errors"
	"sync"

	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
)

// OpenConnectStatus — алиас доменного типа из services.
type OpenConnectStatus = services.OpenConnectStatus

// OpenConnectTarget — цель ядра (CoreTarget). В уведомлении о челлендже
// у Remote MachineID заполнен: ответ уйдёт только той машине, чьё ядро
// спросило, даже если выбор машины сменили.
type OpenConnectTarget = CoreTarget

// openConnectSource — бэкенд или транспорт со стримом статуса openconnect.
type openConnectSource interface {
	OpenConnectStatus(tag string) (services.OpenConnectStatus, bool)
	OpenConnectLive() bool
	OpenConnectPending() []services.OpenConnectStatus
	OpenConnectSubmitForm(tag, challengeID string, values map[string]string) error
	OpenConnectSubmitBrowser(tag, challengeID, finalURL string) error
	OpenConnectCancel(tag, challengeID string) error
}

// openConnectNotifier — источник, которому колбэк ставит фасад (транспорт
// машины: он не знает своей цели). Локальный демон ставит его сам.
type openConnectNotifier interface {
	OpenConnectSetNotify(fn services.OpenConnectNotify)
}

// errOpenConnectUnavailable — у цели нет канала openconnect (классика,
// машина не подключена).
var errOpenConnectUnavailable = errors.New("openconnect sign-in channel is not available for this core")

// openConnectHooks — подписчики UI. Пакетный уровень: бэкенд стартует до
// UI, а лаунчер держит один AppController на процесс.
var openConnectHooks struct {
	mu        sync.Mutex
	challenge func(OpenConnectTarget, OpenConnectStatus)
	change    func(OpenConnectTarget)
}

// openConnectSource — источник цели и цель с конкретной машиной. Local —
// только daemon-режим своего ядра. Remote — транспорт машины из её выбора;
// MachineID "" дополняется выбранной сейчас машиной.
func (ac *AppController) openConnectSource(t OpenConnectTarget) (openConnectSource, OpenConnectTarget, bool) {
	if ac == nil {
		return nil, t, false
	}
	if t.Scope == services.ScopeRemote {
		if t.MachineID == "" && ac.UIService != nil && ac.UIService.LxdOverrideStateFunc != nil {
			if id, _, active := ac.UIService.LxdOverrideStateFunc(); active {
				t.MachineID = id
			}
		}
		tr, ok := ac.machineTransport(t)
		if !ok {
			return nil, t, false
		}
		// Колбэк — до первого чтения: чтение поднимает ленивый стрим, и
		// первый кадр с висящим челленджем не должен пройти мимо.
		if n, ok := tr.(openConnectNotifier); ok {
			bound := t
			n.OpenConnectSetNotify(func(fresh []services.OpenConnectStatus, changed bool) {
				dispatchOpenConnect(bound, fresh, changed)
			})
		}
		src, ok := tr.(openConnectSource)
		return src, t, ok
	}
	if ac.BackendMode() != BackendDaemon {
		return nil, t, false
	}
	src, ok := ac.Backend().(openConnectSource)
	return src, t, ok
}

// dispatchOpenConnect раздаёт новый снимок подписчикам UI. Зовётся с
// горутины стрима; переход в главный поток — забота UI.
func dispatchOpenConnect(t OpenConnectTarget, fresh []services.OpenConnectStatus, changed bool) {
	openConnectHooks.mu.Lock()
	onChallenge, onChange := openConnectHooks.challenge, openConnectHooks.change
	openConnectHooks.mu.Unlock()
	for _, st := range fresh {
		debuglog.DebugLog("openconnect: new challenge for %s (scope %v)", st.Tag, t.Scope)
		if onChallenge != nil {
			onChallenge(t, st)
		}
	}
	if changed && onChange != nil {
		onChange(t)
	}
}

// SetOnOpenConnectChallenge — подписка UI на новый challenge.id. Колбэк
// зовётся с горутины стрима. Уже висящие челленджи своего ядра и выбранной
// машины отдаются сразу (стрим мог прийти раньше подписки UI).
func (ac *AppController) SetOnOpenConnectChallenge(fn func(OpenConnectTarget, OpenConnectStatus)) {
	openConnectHooks.mu.Lock()
	openConnectHooks.challenge = fn
	openConnectHooks.mu.Unlock()
	if fn == nil {
		return
	}
	for _, scope := range []services.ProxyScope{services.ScopeLocal, services.ScopeRemote} {
		src, t, ok := ac.openConnectSource(CoreIn(scope))
		if !ok {
			continue
		}
		pending := src.OpenConnectPending()
		if len(pending) == 0 {
			continue
		}
		go func(t OpenConnectTarget, pending []services.OpenConnectStatus) {
			for _, st := range pending {
				fn(t, st)
			}
		}(t, pending)
	}
}

// SetOnOpenConnectChange — подписка UI на смену видимого состояния узлов
// (строка списка: ожидание входа, ошибка). Зовётся с горутины стрима.
func SetOnOpenConnectChange(fn func(OpenConnectTarget)) {
	openConnectHooks.mu.Lock()
	openConnectHooks.change = fn
	openConnectHooks.mu.Unlock()
}

// OpenConnectAvailable — есть ли у цели канал openconnect.
func (ac *AppController) OpenConnectAvailable(t OpenConnectTarget) bool {
	_, _, ok := ac.openConnectSource(t)
	return ok
}

// OpenConnectTargetOf — цель с конкретной машиной (у Remote с MachineID ""
// — выбранной сейчас): ответ из диалога уйдёт ей, даже если выбор сменят.
func (ac *AppController) OpenConnectTargetOf(t OpenConnectTarget) OpenConnectTarget {
	_, resolved, _ := ac.openConnectSource(t)
	return resolved
}

// OpenConnectStatus — статус endpoint'а по тегу у источника цели.
// ok=false — канала нет, стрим не поднялся или тега у ядра нет.
func (ac *AppController) OpenConnectStatus(t OpenConnectTarget, tag string) (OpenConnectStatus, bool) {
	if src, _, ok := ac.openConnectSource(t); ok {
		return src.OpenConnectStatus(tag)
	}
	return OpenConnectStatus{}, false
}

// OpenConnectLive — жив ли стрим статуса у источника цели.
func (ac *AppController) OpenConnectLive(t OpenConnectTarget) bool {
	if src, _, ok := ac.openConnectSource(t); ok {
		return src.OpenConnectLive()
	}
	return false
}

// OpenConnectSubmitForm — ответ формы второго фактора.
func (ac *AppController) OpenConnectSubmitForm(t OpenConnectTarget, tag, challengeID string, values map[string]string) error {
	src, _, ok := ac.openConnectSource(t)
	if !ok {
		return errOpenConnectUnavailable
	}
	if err := src.OpenConnectSubmitForm(tag, challengeID, values); err != nil {
		debuglog.WarnLog("openconnect: submit form for %s failed: %v", tag, err)
		return err
	}
	debuglog.DebugLog("openconnect: form submitted for %s", tag)
	return nil
}

// OpenConnectSubmitBrowser — URL, на который шлюз вернул браузер после SSO.
func (ac *AppController) OpenConnectSubmitBrowser(t OpenConnectTarget, tag, challengeID, finalURL string) error {
	src, _, ok := ac.openConnectSource(t)
	if !ok {
		return errOpenConnectUnavailable
	}
	if err := src.OpenConnectSubmitBrowser(tag, challengeID, finalURL); err != nil {
		debuglog.WarnLog("openconnect: submit browser result for %s failed: %v", tag, err)
		return err
	}
	debuglog.DebugLog("openconnect: browser result submitted for %s", tag)
	return nil
}

// OpenConnectCancel — отмена челленджа: ядро через 1 с начнёт новый логин,
// шлюз пришлёт новый код.
func (ac *AppController) OpenConnectCancel(t OpenConnectTarget, tag, challengeID string) error {
	src, _, ok := ac.openConnectSource(t)
	if !ok {
		return errOpenConnectUnavailable
	}
	if err := src.OpenConnectCancel(tag, challengeID); err != nil {
		debuglog.WarnLog("openconnect: cancel challenge for %s failed: %v", tag, err)
		return err
	}
	debuglog.DebugLog("openconnect: challenge cancelled for %s", tag)
	return nil
}
