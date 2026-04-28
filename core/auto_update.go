package core

import (
	"time"

	"fyne.io/fyne/v2"

	"singbox-launcher/core/state"
	"singbox-launcher/internal/ctxutil"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/platform"
)

// Constants for auto-update configuration
const (
	autoUpdateMinInterval   = 10 * time.Minute // Minimum check interval
	autoUpdateRetryInterval = 20 * time.Second // Interval between retry attempts
	autoUpdateMaxRetries    = 2                // Maximum consecutive failed attempts
	autoUpdateDefaultReload = "4h"             // Default reload interval if not specified
)

// startAutoUpdateLoop runs a background goroutine that periodically checks and updates configuration
// Uses dynamic interval: max(10 minutes, parser.reload from config)
// Handles errors with retries (2 attempts, 20 seconds between retries)
// Resumes after successful manual update
func (ac *AppController) startAutoUpdateLoop() {
	debuglog.InfoLog("Auto-update: Starting auto-update loop")

	for {
		// Check if context is cancelled
		select {
		case <-ac.ctx.Done():
			debuglog.InfoLog("Auto-update: Context cancelled, stopping loop")
			return
		default:
		}

		// Check if auto-update is enabled
		if !ac.StateService.IsAutoUpdateEnabled() {
			if err := ctxutil.SleepWithContext(ac.ctx, 1*time.Minute); err != nil {
				return
			}
			continue
		}

		// Calculate check interval from config
		checkInterval, err := ac.calculateAutoUpdateInterval()
		if err != nil {
			debuglog.WarnLog("Auto-update: Failed to calculate interval: %v, using default", err)
			checkInterval = autoUpdateMinInterval
		}

		debuglog.DebugLog("Auto-update: Calculated interval: %v (min: %v)", checkInterval, autoUpdateMinInterval)

		if platform.IsSleeping() {
			if err := ctxutil.SleepWithContext(ac.ctx, 1*time.Minute); err != nil {
				return
			}
			continue
		}

		// Check if update is needed immediately (before waiting)
		requiredInterval := checkInterval
		needsUpdate, err := ac.shouldAutoUpdate(requiredInterval)
		if err != nil {
			debuglog.WarnLog("Auto-update: Failed to check if update needed: %v, skipping this check", err)
			// Don't stop auto-update on check errors, just skip this check and wait
		} else if needsUpdate {
			// Update is needed - check if already in progress
			ac.ParserMutex.Lock()
			updateInProgress := ac.ParserRunning
			ac.ParserMutex.Unlock()

			if !updateInProgress {
				debuglog.InfoLog("Auto-update: Update needed, attempting update...")
				success := ac.attemptAutoUpdateWithRetries(autoUpdateRetryInterval, autoUpdateMaxRetries)
				if success {
					// Success - error counter already reset in attemptAutoUpdateWithRetries
					ac.StateService.ResumeAutoUpdate()
					debuglog.InfoLog("Auto-update: Resumed after successful update")
					debuglog.InfoLog("Auto-update: Completed successfully, error counter reset")
				} else {
					// Failed after all retries - check if we reached max consecutive failures
					failedAttempts := ac.StateService.GetAutoUpdateFailedAttempts()
					if failedAttempts >= autoUpdateMaxRetries {
						ac.StateService.SetAutoUpdateEnabled(false)
						debuglog.WarnLog("Auto-update: Stopped after %d consecutive failed attempts", failedAttempts)
						fyne.Do(func() {
							if ac.hasUIWithApp() {
								dialogs.ShowAutoHideInfo(ac.UIService.Application, ac.UIService.MainWindow, "Auto-update", "Automatic configuration update stopped after 2 failed attempts. Use manual update.")
							}
						})
					}
				}
			} else {
				debuglog.DebugLog("Auto-update: Update already in progress, skipping")
			}
		} else {
			debuglog.DebugLog("Auto-update: Update not needed yet, will check again in %v", checkInterval)
		}

		// Wait for check interval before next check
		if err := ctxutil.SleepWithContext(ac.ctx, checkInterval); err != nil {
			return
		}
	}
}

// calculateAutoUpdateInterval calculates the check interval: max(10 minutes, parser.reload)
// Returns the interval to use for checking if update is needed.
//
// Reads parser.reload из state.json (canonical source с SPEC 045).
// Если state.json отсутствует — fallback на default-интервал.
func (ac *AppController) calculateAutoUpdateInterval() (time.Duration, error) {
	statePath := platform.GetWizardStatePath(ac.FileService.ExecDir)
	s, err := state.Load(statePath)
	if err != nil {
		// state.json doesn't exist or can't be read — use default
		defaultDuration, _ := time.ParseDuration(autoUpdateDefaultReload)
		return maxDuration(autoUpdateMinInterval, defaultDuration), nil
	}

	reloadStr := s.ParserConfig.ParserConfig.Parser.Reload
	if reloadStr == "" {
		// Use default if not specified
		defaultDuration, _ := time.ParseDuration(autoUpdateDefaultReload)
		return maxDuration(autoUpdateMinInterval, defaultDuration), nil
	}

	// Parse reload string to duration
	reloadDuration, err := time.ParseDuration(reloadStr)
	if err != nil {
		debuglog.WarnLog("Auto-update: Failed to parse reload duration '%s': %v, using default", reloadStr, err)
		defaultDuration, _ := time.ParseDuration(autoUpdateDefaultReload)
		return maxDuration(autoUpdateMinInterval, defaultDuration), nil
	}

	// Return max(10 minutes, reload)
	return maxDuration(autoUpdateMinInterval, reloadDuration), nil
}

// maxDuration returns the maximum of two durations
func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// shouldAutoUpdate checks if configuration update is needed.
//
// SPEC 052: legacy `parser.last_updated` поле удалено; теперь свежесть
// определяется через per-source `Meta.LastFetchedAt`. Логика:
//   - state.json missing → fresh install, update needed
//   - нет enabled subscription source'ов → update НЕ нужен (нет работы)
//   - хоть один enabled source без meta или с last_fetched_at >= reload
//     назад → update needed
func (ac *AppController) shouldAutoUpdate(requiredInterval time.Duration) (bool, error) {
	statePath := platform.GetWizardStatePath(ac.FileService.ExecDir)
	s, err := state.Load(statePath)
	if err != nil {
		return true, nil
	}

	// Найти "самый свежий" last_fetched_at среди enabled subscription'ов;
	// если хоть один без meta — считаем что update нужен.
	var newestFetch time.Time
	hasEnabledSubs := false
	for _, src := range s.Connections.Sources {
		if src.Type != state.SourceTypeSubscription || !src.Enabled || src.URL == "" {
			continue
		}
		hasEnabledSubs = true
		if src.Meta == nil || src.Meta.LastFetchedAt == "" {
			// Хоть одна без meta → нужен fetch.
			return true, nil
		}
		t, perr := time.Parse(time.RFC3339, src.Meta.LastFetchedAt)
		if perr != nil {
			return true, nil
		}
		if t.After(newestFetch) {
			newestFetch = t
		}
	}
	if !hasEnabledSubs {
		// Нечего обновлять (нет enabled subscription'ов).
		return false, nil
	}

	// Update needed if самый свежий fetch старше required interval.
	elapsed := time.Since(newestFetch.UTC())
	debuglog.DebugLog("Auto-update: Checking if update needed (newest_fetch: %s, elapsed: %v, required: %v)", newestFetch.Format(time.RFC3339), elapsed, requiredInterval)

	// Check if elapsed >= required interval
	return elapsed >= requiredInterval, nil
}

// attemptAutoUpdateWithRetries attempts to update configuration with retries
// Returns true if update succeeded, false if all retries failed
func (ac *AppController) attemptAutoUpdateWithRetries(retryInterval time.Duration, maxRetries int) bool {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		debuglog.InfoLog("Auto-update: Attempting update (attempt %d/%d)", attempt, maxRetries)

		// Call UpdateConfigFromSubscriptions synchronously. The per-source
		// result is irrelevant for the auto-update retry loop — only the
		// error matters; toasts are not shown from this background path.
		_, err := ac.ConfigService.UpdateConfigFromSubscriptions()
		if err == nil {
			// Success - reset error counter
			ac.StateService.ResetAutoUpdateFailedAttempts()
			return true
		}

		// Error occurred - increment error counter
		ac.StateService.IncrementAutoUpdateFailedAttempts()
		currentAttempts := ac.StateService.GetAutoUpdateFailedAttempts()

		debuglog.WarnLog("Auto-update: Failed (attempt %d/%d, total consecutive failures: %d): %v", attempt, maxRetries, currentAttempts, err)

		if attempt < maxRetries {
			debuglog.DebugLog("Auto-update: Retrying in %v...", retryInterval)
			if err := ctxutil.SleepWithContext(ac.ctx, retryInterval); err != nil {
				return false
			}
		}
	}

	// All retries failed
	return false
}

// resumeAutoUpdate resumes automatic updates after successful manual update
// Should be called after successful UpdateConfigFromSubscriptions
func (ac *AppController) resumeAutoUpdate() {
	if ac.StateService != nil {
		ac.StateService.ResumeAutoUpdate()
		debuglog.InfoLog("Auto-update: Resumed after successful manual update")
	}
}
