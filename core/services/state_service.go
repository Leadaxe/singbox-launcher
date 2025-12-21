package services

import (
	"sync"
	"time"
)

// StateService manages application state including version caches and auto-update state.
// It encapsulates state management to reduce AppController complexity.
type StateService struct {
	// Version check caching
	VersionCheckCache      string
	VersionCheckCacheTime  time.Time
	VersionCheckMutex      sync.RWMutex
	VersionCheckInProgress bool

	// Launcher version check caching
	LauncherVersionCheckCache      string
	LauncherVersionCheckCacheTime  time.Time
	LauncherVersionCheckMutex      sync.RWMutex
	LauncherVersionCheckInProgress bool

	// Auto-update configuration
	AutoUpdateEnabled        bool
	AutoUpdateFailedAttempts int
	AutoUpdateMutex          sync.Mutex
}

// NewStateService creates and initializes a new StateService instance.
func NewStateService() *StateService {
	return &StateService{
		AutoUpdateEnabled:        true,
		AutoUpdateFailedAttempts: 0,
	}
}

// GetCachedVersion safely gets the cached version with mutex protection.
func (s *StateService) GetCachedVersion() string {
	s.VersionCheckMutex.RLock()
	defer s.VersionCheckMutex.RUnlock()
	return s.VersionCheckCache
}

// SetCachedVersion safely sets the cached version with mutex protection.
func (s *StateService) SetCachedVersion(version string) {
	s.VersionCheckMutex.Lock()
	defer s.VersionCheckMutex.Unlock()
	s.VersionCheckCache = version
	s.VersionCheckCacheTime = time.Now()
}

// GetCachedVersionTime safely gets the cached version time.
func (s *StateService) GetCachedVersionTime() time.Time {
	s.VersionCheckMutex.RLock()
	defer s.VersionCheckMutex.RUnlock()
	return s.VersionCheckCacheTime
}

// SetVersionCheckInProgress safely sets the version check in progress flag.
func (s *StateService) SetVersionCheckInProgress(inProgress bool) {
	s.VersionCheckMutex.Lock()
	defer s.VersionCheckMutex.Unlock()
	s.VersionCheckInProgress = inProgress
}

// IsVersionCheckInProgress safely checks if version check is in progress.
func (s *StateService) IsVersionCheckInProgress() bool {
	s.VersionCheckMutex.RLock()
	defer s.VersionCheckMutex.RUnlock()
	return s.VersionCheckInProgress
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
