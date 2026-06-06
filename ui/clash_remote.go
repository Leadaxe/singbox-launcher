// File clash_remote.go — SPEC 064: remote Clash API endpoint override for Servers tab.
//
// Раньше вкладка Servers была привязана к локальному sing-box: `baseURL/token`
// читались из `core.APIService.GetClashAPIConfig()`, который, в свою очередь,
// парсит локальный `bin/config.json`. Если sing-box локально не запущен — tab
// disabled.
//
// Этот файл вводит **RAM-only override**: юзер из gear-диалога в шапке таба
// может прописать произвольный (host, port, secret) и подключиться к remote
// Clash-API (sing-box на роутере, mihomo на VPS, другой инстанс лаунчера).
// После рестарта лаунчера override сбрасывается — explicitly ephemeral
// (см. SPEC 064 §«Целевая модель»).
//
// API:
//   - `SetRemoteOverride` / `ClearRemoteOverride` — atomic activate/deactivate.
//   - `GetRemoteOverride` — snapshot.
//   - `OnOverrideChanged` — register listener (status badge update, tab enable,
//     force-refresh).
//   - `EffectiveClashAPIConfig` — single resolver consulted by every callsite
//     in clash_api_tab.go. Returns (baseURL, token, enabled, remote).
//   - `CurrentGeneration` — atomic gen counter для drop-stale в refresh-goroutine'ах
//     (см. SPEC 064 §«Concurrency»).
//   - `NormalizeHost` — strip scheme prefix, reject `:`/`/`/IPv6 brackets.
package ui

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"singbox-launcher/core"
)

// RemoteOverride — ephemeral remote Clash-API endpoint.
//
// HTTP only (SPEC 064 explicit out-of-scope: HTTPS/WSS). Все три поля
// заполняются юзером из gear-dialog'а; reachability probe (3s GET /version)
// валидирует endpoint до активации.
type RemoteOverride struct {
	Host   string
	Port   int
	Secret string
}

var (
	remoteOverrideMu     sync.RWMutex
	remoteOverrideActive bool
	remoteOverrideValue  RemoteOverride

	// clashConfigGeneration — bump'ается на каждом Set/Clear. Refresh-goroutine'ы
	// вкладки (`onLoadAndRefreshProxies`, `pingAllProxies`, switch-proxy callback)
	// захватывают snapshot generation на старте и в `fyne.Do` callback'е проверяют:
	// если generation сместился — drop stale, не пишут в UI.
	//
	// Тот же паттерн что `pingAllGeneration` в `clash_api_tab.go`.
	clashConfigGeneration uint64

	overrideListenersMu sync.RWMutex
	overrideListeners   []func()
)

// GetRemoteOverride — snapshot текущего override'а. ok=false если override не active.
//
// Возвращает по-значению (struct copy), чтобы caller не мог случайно мутировать
// internal state. Безопасно для concurrent use.
func GetRemoteOverride() (RemoteOverride, bool) {
	remoteOverrideMu.RLock()
	defer remoteOverrideMu.RUnlock()
	if !remoteOverrideActive {
		return RemoteOverride{}, false
	}
	return remoteOverrideValue, true
}

// SetRemoteOverride — activate с новыми значениями. Bump'ает generation,
// notify'ит listeners за пределами lock'а.
//
// Безопасно вызывать из UI thread'а (fyne.Do callback) — listener-функции
// должны быть тонкие (типа «trigger tab refresh»); heavy work — в spawn'нутой
// goroutine.
func SetRemoteOverride(ov RemoteOverride) {
	remoteOverrideMu.Lock()
	remoteOverrideValue = ov
	remoteOverrideActive = true
	remoteOverrideMu.Unlock()
	atomic.AddUint64(&clashConfigGeneration, 1)
	notifyOverrideChanged()
}

// ClearRemoteOverride — deactivate (returns to local config). Bump'ает
// generation, notify'ит listeners.
func ClearRemoteOverride() {
	remoteOverrideMu.Lock()
	remoteOverrideValue = RemoteOverride{}
	remoteOverrideActive = false
	remoteOverrideMu.Unlock()
	atomic.AddUint64(&clashConfigGeneration, 1)
	notifyOverrideChanged()
}

// OnOverrideChanged — register callback. Goroutine-safe. Несколько листенеров
// поддерживается (порядок вызова не гарантирован semantically; listeners должны
// быть idempotent).
//
// Listeners вызываются за пределами override-lock'а после Set/Clear, чтобы
// избежать deadlock'а если listener в свою очередь зовёт `Get` / `Set`.
func OnOverrideChanged(cb func()) {
	if cb == nil {
		return
	}
	overrideListenersMu.Lock()
	defer overrideListenersMu.Unlock()
	overrideListeners = append(overrideListeners, cb)
}

// notifyOverrideChanged — internal helper, копирует listener-slice под RLock'ом
// и вызывает каждый колл-бек за пределами lock'а.
func notifyOverrideChanged() {
	overrideListenersMu.RLock()
	cbs := make([]func(), len(overrideListeners))
	copy(cbs, overrideListeners)
	overrideListenersMu.RUnlock()
	for _, cb := range cbs {
		cb()
	}
}

// CurrentGeneration — atomic load для drop-stale check'ов в refresh-goroutine'ах.
//
// Pattern (см. SPEC 064 §«Concurrency»):
//
//	go func() {
//	    gen := ui.CurrentGeneration()
//	    baseURL, token, _, _ := ui.EffectiveClashAPIConfig(ac)
//	    result, err := fetch(baseURL, token)
//	    fyne.Do(func() {
//	        if gen != ui.CurrentGeneration() { return } // override changed mid-flight
//	        renderResult(result, err)
//	    })
//	}()
func CurrentGeneration() uint64 {
	return atomic.LoadUint64(&clashConfigGeneration)
}

// EffectiveClashAPIConfig — single resolver consulted by every Servers-tab
// callsite вместо `ac.APIService.GetClashAPIConfig()` напрямую.
//
// Returns:
//   - baseURL — `http://<host>:<port>` (override) или local config baseURL
//   - token   — secret (override) или local config token
//   - enabled — true если caller имеет валидный (baseURL, token) для запросов
//   - remote  — true только если override active (для UI badge differentiation)
func EffectiveClashAPIConfig(ac *core.AppController) (baseURL, token string, enabled, remote bool) {
	if ov, ok := GetRemoteOverride(); ok {
		return fmt.Sprintf("http://%s:%d", ov.Host, ov.Port), ov.Secret, true, true
	}
	if ac == nil || ac.APIService == nil {
		return "", "", false, false
	}
	base, tok, en := ac.APIService.GetClashAPIConfig()
	return base, tok, en, false
}

// NormalizeHost — приводит юзер-ввод к чистому hostname'у.
//
// Преобразования:
//   - strip `http://` / `https://` префикса
//   - trim whitespace и trailing `/`
//
// Reject'ы (возвращают error):
//   - empty string после trim'а
//   - содержит `/` (path component) — юзер вставил URL целиком
//   - содержит `:` — юзер вставил `host:port`, должен использовать
//     отдельные поля
//   - начинается с `[` (IPv6 bracket form) — explicit out-of-scope в MVP
//     (SPEC 064 Out-of-scope)
func NormalizeHost(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("host is empty")
	}
	// Strip scheme prefix.
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "http://"):
		s = s[len("http://"):]
	case strings.HasPrefix(lower, "https://"):
		s = s[len("https://"):]
	}
	// Trim trailing slashes + whitespace.
	s = strings.TrimRight(s, "/ \t")
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("host is empty after normalization")
	}
	if strings.HasPrefix(s, "[") {
		return "", fmt.Errorf("IPv6 literals not supported yet")
	}
	if strings.Contains(s, "/") {
		return "", fmt.Errorf("host must not contain '/' — paste only hostname/IP, not URL")
	}
	if strings.Contains(s, ":") {
		return "", fmt.Errorf("host must not contain ':' — use the Port field separately")
	}
	return s, nil
}
