package core

import (
	"time"

	"singbox-launcher/core/events"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/ctxutil"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/platform"
)

// SPEC 052 phase 8 — event-driven per-source auto-update.
//
// Старая модель (одна попытка на ВСЕ подписки + 2 retry × 20 сек) заменена на:
//
//   - **Heartbeat 1 час**: на каждом тике пробег по `state.Connections.Sources[]`.
//     Для каждой enabled subscription смотрим `meta.last_fetched_at`. Если
//     `now - last_fetched_at >= effective_reload` (per-source override или
//     `defaults.reload`) → fetch только этого source через
//     `RefreshSingleSubscription(id)`. Свежие источники пропускаются.
//
//   - **Failure → 15-сек retry**: если RefreshSingleSubscription вернул
//     ошибку, планируем `time.AfterFunc(15s, retry)`. Один retry — не
//     рекурсивный; если retry тоже упал, ждём следующий heartbeat (1ч)
//     или VPN-event.
//
//   - **VPN-event trigger**: подписка на `events.VpnStateChanged` и
//     `events.ProxyActiveChanged` через event bus. На любое событие —
//     trigger fetch для source'ов с `last_status="err"` (досрочный retry,
//     не ждать heartbeat).
//
// Это устраняет паразитный апдейт-всех при старте (был bug — v4 поле
// parser.last_updated удалено в v5, всегда триггерило `shouldAutoUpdate=true`).
const (
	// autoUpdateHeartbeat — интервал между периодическими проверками
	// freshness всех source'ов. 1ч — достаточно частый, чтобы reagировать
	// на профильные `update.interval_hours` без burning сети.
	autoUpdateHeartbeat = 1 * time.Hour

	// autoUpdateRetryDelay — задержка перед единственным retry-ом для
	// одного source после failed fetch. 15s — пользователь успеет дождаться,
	// если ему важно; и не настолько долго, чтобы скрыть проблему.
	autoUpdateRetryDelay = 15 * time.Second

	// autoUpdateDefaultReload — fallback для source'ов без явного
	// `update.interval_hours` и без global `defaults.reload`.
	autoUpdateDefaultReload = 1 * time.Hour

	// autoUpdateEventCooldown — минимальный интервал между fetch-попытками
	// одного source через VPN-event handler (защита от storm'а при rapid
	// proxy-switch / VPN on-off). Manual Refresh и heartbeat — без cooldown'а
	// (юзер явно кликнул / редкое событие).
	autoUpdateEventCooldown = 5 * time.Second
)

// startAutoUpdateLoop запускает периодический heartbeat + подписку на
// VPN-event'ы. Goroutine живёт пока ac.ctx не cancelled.
func (ac *AppController) startAutoUpdateLoop() {
	debuglog.InfoLog("Auto-update: Starting per-source heartbeat loop (heartbeat=%v, retry=%v)",
		autoUpdateHeartbeat, autoUpdateRetryDelay)

	if ac.autoUpdateRetryTimers == nil {
		ac.autoUpdateRetryTimers = make(map[string]*time.Timer)
	}
	if ac.autoUpdateEventLastFetch == nil {
		ac.autoUpdateEventLastFetch = make(map[string]time.Time)
	}

	// Subscribe to VPN events — trigger immediate retry for failed source'ов.
	if ac.EventBus != nil {
		ac.EventBus.Subscribe(events.VpnStateChanged, func(_ events.Event) {
			ac.triggerRetryForFailedSources("vpn-state-changed")
		})
		ac.EventBus.Subscribe(events.ProxyActiveChanged, func(_ events.Event) {
			ac.triggerRetryForFailedSources("proxy-active-changed")
		})
	}

	// Первая проверка не моментально — даём UI стартануть и пользователю
	// увидеть текущие meta'ы; через короткую задержку запускаем initial sweep.
	if err := ctxutil.SleepWithContext(ac.ctx, 30*time.Second); err != nil {
		return
	}
	ac.runScheduledRefresh("startup")

	ticker := time.NewTicker(autoUpdateHeartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ac.ctx.Done():
			debuglog.InfoLog("Auto-update: Context cancelled, stopping loop")
			ac.cancelAllRetryTimers()
			return
		case <-ticker.C:
			if !ac.StateService.IsAutoUpdateEnabled() {
				debuglog.DebugLog("Auto-update: disabled by user, skipping heartbeat")
				continue
			}
			if platform.IsSleeping() {
				debuglog.DebugLog("Auto-update: system asleep, skipping heartbeat")
				continue
			}
			ac.runScheduledRefresh("heartbeat")
		}
	}
}

// runScheduledRefresh — пробег по source'ам, fetch только устаревших.
//
// Trigger source: "startup" / "heartbeat" / "vpn-state-changed" /
// "proxy-active-changed" — для логирования.
func (ac *AppController) runScheduledRefresh(trigger string) {
	statePath := platform.GetWizardStatePath(ac.FileService.ExecDir)
	s, err := state.Load(statePath)
	if err != nil {
		debuglog.DebugLog("Auto-update[%s]: state.Load failed: %v", trigger, err)
		return
	}

	now := time.Now().UTC()
	stale := 0
	skipped := 0
	for _, src := range s.Connections.Sources {
		if src.Type != state.SourceTypeSubscription || !src.Enabled || src.URL == "" {
			continue
		}
		if !sourceIsStale(&src, s.Connections.Defaults, now) {
			skipped++
			continue
		}
		stale++
		ac.refreshSourceWithRetry(src.ID, trigger)
	}
	debuglog.InfoLog("Auto-update[%s]: %d stale source(s) refreshed, %d skipped (fresh)",
		trigger, stale, skipped)
}

// sourceIsStale — true если `now - meta.last_fetched_at >= effective_reload`.
// Source без meta или с пустым LastFetchedAt считается stale.
func sourceIsStale(src *state.Source, defaults state.Defaults, now time.Time) bool {
	if src.Meta == nil || src.Meta.LastFetchedAt == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, src.Meta.LastFetchedAt)
	if err != nil {
		return true
	}
	effective := effectiveReload(src.Update, defaults.Reload)
	return now.Sub(t.UTC()) >= effective
}

// effectiveReload — выбирает интервал: per-source `update.interval_hours`,
// затем global `defaults.reload`, fallback `autoUpdateDefaultReload`.
func effectiveReload(update *state.UpdateSpec, defaultReload string) time.Duration {
	if update != nil && update.IntervalHours > 0 {
		return time.Duration(update.IntervalHours) * time.Hour
	}
	if defaultReload != "" {
		if d, err := time.ParseDuration(defaultReload); err == nil && d > 0 {
			return d
		}
	}
	return autoUpdateDefaultReload
}

// refreshSourceWithRetry — fetch + meta + raw для одного source.
// На failure планирует один retry через 15 секунд.
func (ac *AppController) refreshSourceWithRetry(sourceID, trigger string) {
	if ac.ConfigService == nil {
		return
	}
	debuglog.DebugLog("Auto-update[%s]: refreshing source %s", trigger, sourceID)
	_, err := ac.ConfigService.RefreshSingleSubscription(sourceID)
	if err == nil {
		ac.cancelRetryTimer(sourceID) // на success отменяем pending retry
		return
	}
	debuglog.WarnLog("Auto-update[%s]: refresh failed for %s: %v — scheduling retry in %v",
		trigger, sourceID, err, autoUpdateRetryDelay)
	ac.scheduleSourceRetry(sourceID)
}

// scheduleSourceRetry — таймер на 15s; повторно не рекурсирует
// (success/fail после single retry — оба ведут к ожиданию следующего
// heartbeat'а или VPN-event'а).
func (ac *AppController) scheduleSourceRetry(sourceID string) {
	ac.autoUpdateRetryMu.Lock()
	defer ac.autoUpdateRetryMu.Unlock()

	if ac.autoUpdateRetryTimers == nil {
		ac.autoUpdateRetryTimers = make(map[string]*time.Timer)
	}
	// Cancel existing timer for this source if any.
	if existing, ok := ac.autoUpdateRetryTimers[sourceID]; ok {
		existing.Stop()
	}
	ac.autoUpdateRetryTimers[sourceID] = time.AfterFunc(autoUpdateRetryDelay, func() {
		if ac.ConfigService == nil {
			return
		}
		debuglog.InfoLog("Auto-update[retry-15s]: refreshing source %s", sourceID)
		if _, err := ac.ConfigService.RefreshSingleSubscription(sourceID); err != nil {
			debuglog.WarnLog("Auto-update[retry-15s]: source %s still failing: %v "+
				"(next attempt at next heartbeat or VPN event)", sourceID, err)
		}
		// Cleanup map entry; не запускаем второй retry.
		ac.autoUpdateRetryMu.Lock()
		delete(ac.autoUpdateRetryTimers, sourceID)
		ac.autoUpdateRetryMu.Unlock()
	})
}

// cancelRetryTimer — отменяет pending retry для source (на success).
func (ac *AppController) cancelRetryTimer(sourceID string) {
	ac.autoUpdateRetryMu.Lock()
	defer ac.autoUpdateRetryMu.Unlock()
	if timer, ok := ac.autoUpdateRetryTimers[sourceID]; ok {
		timer.Stop()
		delete(ac.autoUpdateRetryTimers, sourceID)
	}
}

// cancelAllRetryTimers — на shutdown останавливаем всё, чтобы не было
// goroutine leak'а после ctx.Done().
func (ac *AppController) cancelAllRetryTimers() {
	ac.autoUpdateRetryMu.Lock()
	defer ac.autoUpdateRetryMu.Unlock()
	for id, timer := range ac.autoUpdateRetryTimers {
		timer.Stop()
		delete(ac.autoUpdateRetryTimers, id)
	}
}

// triggerRetryForFailedSources — на VPN-event пробегаемся по source'ам с
// `last_status="err"` и инициируем досрочный refresh.
//
// **Cooldown 5s per source**: rapid VPN switches / power events не должны
// генерировать N×fetches одной и той же подписки. Если последний event-
// triggered fetch < 5 сек назад — skip; остальные пути (manual Refresh,
// heartbeat) cooldown не затрагивает.
//
// Лёгкая операция (один state.Load + map look-ups + per-source goroutine
// внутри refreshSourceWithRetry); event handler в bus синхронный, так
// что мы не блокируем publisher'а — RefreshSingleSubscription идёт в
// фоне через AfterFunc'ы.
func (ac *AppController) triggerRetryForFailedSources(trigger string) {
	if ac.ConfigService == nil || ac.StateService == nil {
		return
	}
	if !ac.StateService.IsAutoUpdateEnabled() {
		return
	}
	statePath := platform.GetWizardStatePath(ac.FileService.ExecDir)
	s, err := state.Load(statePath)
	if err != nil {
		return
	}
	now := time.Now()
	for _, src := range s.Connections.Sources {
		if src.Type != state.SourceTypeSubscription || !src.Enabled || src.URL == "" {
			continue
		}
		if src.Meta == nil || src.Meta.LastStatus != "err" {
			continue
		}
		if !ac.eventCooldownAllow(src.ID, now) {
			debuglog.DebugLog("Auto-update[%s]: cooldown skip for source %s (<5s since last event-fetch)",
				trigger, src.ID)
			continue
		}
		// Запускаем в отдельной goroutine — event-bus handler не должен ждать сети.
		go ac.refreshSourceWithRetry(src.ID, trigger)
	}
}

// eventCooldownAllow возвращает true и обновляет timestamp если с
// последнего event-triggered fetch'а данного source прошло >= 5 сек;
// иначе false (skip).
//
// Использует ту же mutex что и retry timers — операции дешёвые,
// дополнительный mutex ради них ставить избыточно.
func (ac *AppController) eventCooldownAllow(sourceID string, now time.Time) bool {
	ac.autoUpdateRetryMu.Lock()
	defer ac.autoUpdateRetryMu.Unlock()
	if ac.autoUpdateEventLastFetch == nil {
		ac.autoUpdateEventLastFetch = make(map[string]time.Time)
	}
	last, ok := ac.autoUpdateEventLastFetch[sourceID]
	if ok && now.Sub(last) < autoUpdateEventCooldown {
		return false
	}
	ac.autoUpdateEventLastFetch[sourceID] = now
	return true
}

// resumeAutoUpdate — вызывается после успешного manual Update'а; сбрасывает
// retry timers и failed counter в StateService (legacy совместимость).
func (ac *AppController) resumeAutoUpdate() {
	if ac.StateService != nil {
		ac.StateService.ResumeAutoUpdate()
	}
	ac.cancelAllRetryTimers()
	debuglog.InfoLog("Auto-update: Resumed after successful manual update")
}
