package services

import (
	"sync"
	"time"
)

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

	// TemplateDirty is set when the wizard saved config changes but the parser
	// has not run since. The Update button decorates its label with "*" so
	// users see that the running config may lag the saved template.
	TemplateDirty      bool
	TemplateDirtyMutex sync.RWMutex

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

// IsTemplateDirty reports whether the wizard has committed template / state
// changes that the parser has not yet incorporated into the running config.
func (s *StateService) IsTemplateDirty() bool {
	s.TemplateDirtyMutex.RLock()
	defer s.TemplateDirtyMutex.RUnlock()
	return s.TemplateDirty
}

// SetTemplateDirty flags or clears the dirty-template marker.
// Setters of record: wizard Save (sets true), successful parser run (sets false).
func (s *StateService) SetTemplateDirty(dirty bool) {
	s.TemplateDirtyMutex.Lock()
	defer s.TemplateDirtyMutex.Unlock()
	s.TemplateDirty = dirty
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
