package services

import (
	"sync"
	"time"

	"singbox-launcher/core/events"
	"singbox-launcher/core/state"
)

// DefaultAutoPingMaxProxies — soft cap for the auto-ping-after-connect storm.
// Above this many proxies the auto-ping is skipped: a single ping-all on a
// large subscription opens hundreds of connections and on Windows TUN that
// burst can starve in-flight game / app traffic during the connect window.
// Field report from a user with ~500 CIDR-derived nodes (2026-04-26) showed
// game logins freezing on 0.8.7+; 0.8.6 (which had no auto-ping) worked.
// 150 is the empirically suggested threshold (SPEC 039 §1.3). Manual «Test»
// button is unaffected — only the timer-driven path is gated.
const DefaultAutoPingMaxProxies = 150

// StateService manages application state including version caches and auto-update state.
// It encapsulates state management to reduce AppController complexity.
//
// Sing-box version is NOT cached here: the launcher pins it via
// constants.RequiredCoreVersion (SPEC 046). The cache below is only for the
// **launcher**'s own self-update check.
type StateService struct {
	// Launcher version check caching
	LauncherVersionCheckCache      string
	LauncherVersionCheckCacheTime  time.Time
	LauncherVersionCheckMutex      sync.RWMutex
	LauncherVersionCheckInProgress bool

	// Auto-update configuration
	AutoUpdateEnabled        bool
	AutoUpdateFailedAttempts int
	AutoUpdateMutex          sync.Mutex

	// Auto-ping proxies 5s after VPN connects (default on).
	AutoPingAfterConnect      bool
	AutoPingAfterConnectMutex sync.RWMutex

	// AutoPingMaxProxies — skip auto-ping when proxy count exceeds this.
	// 0 = no cap (always run). Default seeded from DefaultAutoPingMaxProxies.
	// Guarded by AutoPingAfterConnectMutex (same gate, same domain).
	AutoPingMaxProxies int

	// CacheStale (SPEC 045 phase 4.1) — proxy sources / skip / tag-prefix /
	// local outbounds list changed since the last successful parser+build run.
	// UI shows `*` on Update button. Cleared on successful RunUpdate.
	CacheStale      bool
	CacheStaleMutex sync.RWMutex

	// ConfigStale (SPEC 045 phase 4.1) — template fields (tun, dns, rules,
	// log_level, vars) changed since the last sing-box (re)start. Even after
	// config.json was rebuilt, the running sing-box process is still using the
	// old config until the user presses Restart. UI shows a marker on the
	// Restart button. Cleared on successful Restart.
	ConfigStale      bool
	ConfigStaleMutex sync.RWMutex

	// EventBus — optional, set by AppController when constructing the service.
	// If non-nil, dirty-marker mutators publish StateChanged events so UI can
	// react without needing direct callbacks. Nil-safe: every publish is
	// guarded by a nil-check.
	EventBus events.Bus

	// LastUpdateSucceededAt — timestamp последнего успешного прогона
	// RunParserProcess. Читается freshness-хинтом на Core Dashboard
	// («подписки: 2 ч назад»). In-memory, не персистится.
	LastUpdateSucceededAt time.Time
	LastUpdateMutex       sync.RWMutex
}

// NewStateService creates and initializes a new StateService instance.
func NewStateService() *StateService {
	return &StateService{
		AutoUpdateEnabled:        true,
		AutoUpdateFailedAttempts: 0,
		AutoPingAfterConnect:     true,
		AutoPingMaxProxies:       DefaultAutoPingMaxProxies,
	}
}

// IsAutoPingAfterConnectEnabled reports whether the controller should
// trigger an automatic ping-all 5s after sing-box starts running.
func (s *StateService) IsAutoPingAfterConnectEnabled() bool {
	s.AutoPingAfterConnectMutex.RLock()
	defer s.AutoPingAfterConnectMutex.RUnlock()
	return s.AutoPingAfterConnect
}

// SetAutoPingAfterConnectEnabled toggles the auto-ping-after-connect flag.
func (s *StateService) SetAutoPingAfterConnectEnabled(enabled bool) {
	s.AutoPingAfterConnectMutex.Lock()
	defer s.AutoPingAfterConnectMutex.Unlock()
	s.AutoPingAfterConnect = enabled
}

// GetAutoPingMaxProxies returns the soft cap for auto-ping. 0 = no cap.
func (s *StateService) GetAutoPingMaxProxies() int {
	s.AutoPingAfterConnectMutex.RLock()
	defer s.AutoPingAfterConnectMutex.RUnlock()
	return s.AutoPingMaxProxies
}

// SetAutoPingMaxProxies updates the soft cap. Negative values clamp to 0
// (no cap); the caller is expected to pass values from settings.json.
func (s *StateService) SetAutoPingMaxProxies(n int) {
	if n < 0 {
		n = 0
	}
	s.AutoPingAfterConnectMutex.Lock()
	defer s.AutoPingAfterConnectMutex.Unlock()
	s.AutoPingMaxProxies = n
}

// IsCacheStale reports whether CacheStale marker is set (SPEC 045 phase 4.1).
// True means: state changes since last RunUpdate require parser re-run + rebuild.
func (s *StateService) IsCacheStale() bool {
	s.CacheStaleMutex.RLock()
	defer s.CacheStaleMutex.RUnlock()
	return s.CacheStale
}

// MarkCacheStale sets CacheStale=true. Idempotent — repeated calls without
// an intervening Clear are no-ops semantically (no event re-publish).
func (s *StateService) MarkCacheStale() {
	s.CacheStaleMutex.Lock()
	wasDirty := s.CacheStale
	s.CacheStale = true
	s.CacheStaleMutex.Unlock()
	if !wasDirty {
		s.publishStateChanged([]string{"update_dirty"})
	}
}

// ClearCacheStale sets CacheStale=false (called by RunUpdate on success).
func (s *StateService) ClearCacheStale() {
	s.CacheStaleMutex.Lock()
	wasDirty := s.CacheStale
	s.CacheStale = false
	s.CacheStaleMutex.Unlock()
	if wasDirty {
		s.publishStateChanged([]string{"update_dirty_cleared"})
	}
}

// IsConfigStale reports whether ConfigStale marker is set (SPEC 045 phase 4.1).
// True means: template-side changes since last sing-box restart; running
// process serves stale config until user presses Restart.
func (s *StateService) IsConfigStale() bool {
	s.ConfigStaleMutex.RLock()
	defer s.ConfigStaleMutex.RUnlock()
	return s.ConfigStale
}

// MarkConfigStale sets ConfigStale=true.
func (s *StateService) MarkConfigStale() {
	s.ConfigStaleMutex.Lock()
	wasDirty := s.ConfigStale
	s.ConfigStale = true
	s.ConfigStaleMutex.Unlock()
	if !wasDirty {
		s.publishStateChanged([]string{"restart_dirty"})
	}
}

// ClearConfigStale sets ConfigStale=false (called on successful sing-box restart).
func (s *StateService) ClearConfigStale() {
	s.ConfigStaleMutex.Lock()
	wasDirty := s.ConfigStale
	s.ConfigStale = false
	s.ConfigStaleMutex.Unlock()
	if wasDirty {
		s.publishStateChanged([]string{"restart_dirty_cleared"})
	}
}

// ApplyDiff (SPEC 045 phase 4.1) — atomically translates a state.Diff into
// dirty-marker flags following the canonical mapping:
//
//	Diff.AffectsParser()   → MarkCacheStale()
//	Diff.AffectsTemplate() → MarkConfigStale()
//
// Called by Configurator presenter after a successful state.Save. Idempotent:
// if Diff.IsEmpty() — no-op.
func (s *StateService) ApplyDiff(d state.Diff) {
	if d.IsEmpty() {
		return
	}
	if d.AffectsParser() {
		s.MarkCacheStale()
	}
	if d.AffectsTemplate() {
		s.MarkConfigStale()
	}
}

// publishStateChanged — внутренняя утилита; nil-safe относительно EventBus.
// Имена в `changed` — стабильные ярлыки доменов изменений, см.
// events.StateChangedPayload (раздел Changed).
func (s *StateService) publishStateChanged(changed []string) {
	if s.EventBus == nil {
		return
	}
	s.EventBus.Publish(events.Event{
		Kind:    events.StateChanged,
		Payload: events.StateChangedPayload{Changed: changed},
	})
}

// RecordUpdateSuccess ставит timestamp последнего успешного прогона парсера.
// Используется freshness-хинтом на Core Dashboard.
func (s *StateService) RecordUpdateSuccess() {
	s.LastUpdateMutex.Lock()
	defer s.LastUpdateMutex.Unlock()
	s.LastUpdateSucceededAt = time.Now()
}

// GetCachedLauncherVersion safely gets the cached launcher version with mutex protection.
func (s *StateService) GetCachedLauncherVersion() string {
	s.LauncherVersionCheckMutex.RLock()
	defer s.LauncherVersionCheckMutex.RUnlock()
	return s.LauncherVersionCheckCache
}

// SetCachedLauncherVersion safely sets the cached launcher version with mutex protection.
func (s *StateService) SetCachedLauncherVersion(version string) {
	s.LauncherVersionCheckMutex.Lock()
	defer s.LauncherVersionCheckMutex.Unlock()
	s.LauncherVersionCheckCache = version
	s.LauncherVersionCheckCacheTime = time.Now()
}

// GetCachedLauncherVersionTime safely gets the cached launcher version time.
func (s *StateService) GetCachedLauncherVersionTime() time.Time {
	s.LauncherVersionCheckMutex.RLock()
	defer s.LauncherVersionCheckMutex.RUnlock()
	return s.LauncherVersionCheckCacheTime
}

// SetLauncherVersionCheckInProgress safely sets the launcher version check in progress flag.
func (s *StateService) SetLauncherVersionCheckInProgress(inProgress bool) {
	s.LauncherVersionCheckMutex.Lock()
	defer s.LauncherVersionCheckMutex.Unlock()
	s.LauncherVersionCheckInProgress = inProgress
}

// IsLauncherVersionCheckInProgress safely checks if launcher version check is in progress.
func (s *StateService) IsLauncherVersionCheckInProgress() bool {
	s.LauncherVersionCheckMutex.RLock()
	defer s.LauncherVersionCheckMutex.RUnlock()
	return s.LauncherVersionCheckInProgress
}

// IsAutoUpdateEnabled safely checks if auto-update is enabled.
func (s *StateService) IsAutoUpdateEnabled() bool {
	s.AutoUpdateMutex.Lock()
	defer s.AutoUpdateMutex.Unlock()
	return s.AutoUpdateEnabled
}

// SetAutoUpdateEnabled safely sets the auto-update enabled flag.
func (s *StateService) SetAutoUpdateEnabled(enabled bool) {
	s.AutoUpdateMutex.Lock()
	defer s.AutoUpdateMutex.Unlock()
	s.AutoUpdateEnabled = enabled
}

// GetAutoUpdateFailedAttempts safely gets the auto-update failed attempts count.
func (s *StateService) GetAutoUpdateFailedAttempts() int {
	s.AutoUpdateMutex.Lock()
	defer s.AutoUpdateMutex.Unlock()
	return s.AutoUpdateFailedAttempts
}

// IncrementAutoUpdateFailedAttempts safely increments the auto-update failed attempts count.
func (s *StateService) IncrementAutoUpdateFailedAttempts() {
	s.AutoUpdateMutex.Lock()
	defer s.AutoUpdateMutex.Unlock()
	s.AutoUpdateFailedAttempts++
}

// ResetAutoUpdateFailedAttempts safely resets the auto-update failed attempts count.
func (s *StateService) ResetAutoUpdateFailedAttempts() {
	s.AutoUpdateMutex.Lock()
	defer s.AutoUpdateMutex.Unlock()
	s.AutoUpdateFailedAttempts = 0
}

// ResumeAutoUpdate resumes automatic updates after successful manual update.
func (s *StateService) ResumeAutoUpdate() {
	s.AutoUpdateMutex.Lock()
	defer s.AutoUpdateMutex.Unlock()
	s.AutoUpdateFailedAttempts = 0
	if !s.AutoUpdateEnabled {
		s.AutoUpdateEnabled = true
	}
}
