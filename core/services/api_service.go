package services

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"

	"singbox-launcher/api"
	"singbox-launcher/core/config"
)

// APIService manages Clash API interactions and proxy list management.
// It encapsulates all API-related state and operations to reduce AppController complexity.
type APIService struct {
	// Clash API configuration
	BaseURL            string
	Token              string
	Enabled            bool
	SelectedClashGroup string

	// Auto-load state
	AutoLoadInProgress bool
	AutoLoadMutex      sync.Mutex

	// Proxy list state (protected by StateMutex)
	StateMutex      sync.RWMutex
	ProxiesList     []api.ProxyInfo
	ActiveProxyName string
	SelectedIndex   int
	// LastSelectedProxyByGroup maps selector group -> last selected proxy name.
	// This allows remembering the last proxy per selector (group) independently.
	LastSelectedProxyByGroup map[string]string

	// Dependencies (passed from AppController)
	ConfigPath            string
	ApiLogFile            *os.File
	RunningStateIsRunning func() bool
	OnProxiesUpdated      func() // Called when proxies are updated
	OnProxySwitched       func() // Called when proxy is switched
}

// NewAPIService creates and initializes a new APIService instance.
func NewAPIService(configPath string, apiLogFile *os.File,
	runningStateIsRunning func() bool,
	onProxiesUpdated func(), onProxySwitched func()) (*APIService, error) {
	apiSvc := &APIService{
		ConfigPath:            configPath,
		ApiLogFile:            apiLogFile,
		RunningStateIsRunning: runningStateIsRunning,
		OnProxiesUpdated:      onProxiesUpdated,
		OnProxySwitched:       onProxySwitched,
	}

	// Load Clash API configuration from config.json
	if base, tok, err := api.LoadClashAPIConfig(configPath); err != nil {
		log.Printf("NewAPIService: Clash API config error: %v", err)
		apiSvc.BaseURL = ""
		apiSvc.Token = ""
		apiSvc.Enabled = false
	} else {
		apiSvc.BaseURL = base
		apiSvc.Token = tok
		apiSvc.Enabled = true
	}

	// Initialize SelectedClashGroup from config
	if apiSvc.Enabled {
		_, defaultSelector, err := config.GetSelectorGroupsFromConfig(configPath)
		if err != nil {
			log.Printf("NewAPIService: Failed to get selector groups: %v", err)
			apiSvc.SelectedClashGroup = "proxy-out" // Default fallback
		} else {
			apiSvc.SelectedClashGroup = defaultSelector
			log.Printf("NewAPIService: Initialized SelectedClashGroup: %s", defaultSelector)
		}
	}

	// Initialize API state fields
	apiSvc.SetProxiesList([]api.ProxyInfo{})
	apiSvc.SetSelectedIndex(-1)
	apiSvc.SetActiveProxyName("")
	apiSvc.LastSelectedProxyByGroup = make(map[string]string)

	return apiSvc, nil
}

// SetProxiesList safely sets the proxies list with mutex protection.
func (apiSvc *APIService) SetProxiesList(proxies []api.ProxyInfo) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	apiSvc.ProxiesList = proxies
}

// GetProxiesList safely gets a copy of the proxies list with mutex protection.
func (apiSvc *APIService) GetProxiesList() []api.ProxyInfo {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	// Return a copy to prevent external modifications
	result := make([]api.ProxyInfo, len(apiSvc.ProxiesList))
	copy(result, apiSvc.ProxiesList)
	return result
}

// SetActiveProxyName safely sets the active proxy name with mutex protection.
func (apiSvc *APIService) SetActiveProxyName(name string) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	apiSvc.ActiveProxyName = name
}

// GetActiveProxyName safely gets the active proxy name with mutex protection.
func (apiSvc *APIService) GetActiveProxyName() string {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	return apiSvc.ActiveProxyName
}

// SetSelectedIndex safely sets the selected index with mutex protection.
func (apiSvc *APIService) SetSelectedIndex(index int) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	apiSvc.SelectedIndex = index
}

// GetSelectedIndex safely gets the selected index with mutex protection.
func (apiSvc *APIService) GetSelectedIndex() int {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	return apiSvc.SelectedIndex
}

// SetLastSelectedProxyForGroup safely sets the last selected proxy name for a selector group with mutex protection.
func (apiSvc *APIService) SetLastSelectedProxyForGroup(group, name string) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	if apiSvc.LastSelectedProxyByGroup == nil {
		apiSvc.LastSelectedProxyByGroup = make(map[string]string)
	}
	apiSvc.LastSelectedProxyByGroup[group] = name
}

// GetLastSelectedProxyForGroup safely gets the last selected proxy name for a selector group with mutex protection.
func (apiSvc *APIService) GetLastSelectedProxyForGroup(group string) string {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	if apiSvc.LastSelectedProxyByGroup == nil {
		return ""
	}
	return apiSvc.LastSelectedProxyByGroup[group]
}

// GetSelectedClashGroup safely gets the selected Clash group.
func (apiSvc *APIService) GetSelectedClashGroup() string {
	apiSvc.StateMutex.RLock()
	defer apiSvc.StateMutex.RUnlock()
	return apiSvc.SelectedClashGroup
}

// SetSelectedClashGroup safely sets the selected Clash group.
func (apiSvc *APIService) SetSelectedClashGroup(group string) {
	apiSvc.StateMutex.Lock()
	defer apiSvc.StateMutex.Unlock()
	apiSvc.SelectedClashGroup = group
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

	log.Println("ReloadClashAPIConfig: Reloading Clash API configuration from config.json...")

	// Load Clash API configuration from config.json
	if base, tok, err := api.LoadClashAPIConfig(apiSvc.ConfigPath); err != nil {
		log.Printf("ReloadClashAPIConfig: Clash API config error: %v", err)
		apiSvc.BaseURL = ""
		apiSvc.Token = ""
		apiSvc.Enabled = false
		apiSvc.SelectedClashGroup = ""
		return fmt.Errorf("failed to reload Clash API config: %w", err)
	} else {
		oldEnabled := apiSvc.Enabled
		apiSvc.BaseURL = base
		apiSvc.Token = tok
		apiSvc.Enabled = true
		log.Printf("ReloadClashAPIConfig: Successfully reloaded - BaseURL: %s, Enabled: %v (was %v)", base, true, oldEnabled)
	}

	// Reload SelectedClashGroup from config if API is enabled
	if apiSvc.Enabled {
		_, defaultSelector, err := config.GetSelectorGroupsFromConfig(apiSvc.ConfigPath)
		if err != nil {
			log.Printf("ReloadClashAPIConfig: Failed to get selector groups: %v", err)
			// Keep existing SelectedClashGroup if we can't read new one
		} else {
			apiSvc.SelectedClashGroup = defaultSelector
			log.Printf("ReloadClashAPIConfig: Updated SelectedClashGroup: %s", defaultSelector)
		}
	}

	return nil
}

// AutoLoadProxies attempts to load proxies with retry intervals (1, 3, 7, 13, 17 seconds).
func (apiSvc *APIService) AutoLoadProxies(ctx context.Context) {
	// Check if already in progress
	apiSvc.AutoLoadMutex.Lock()
	if apiSvc.AutoLoadInProgress {
		apiSvc.AutoLoadMutex.Unlock()
		log.Printf("AutoLoadProxies: Already in progress, skipping")
		return
	}
	apiSvc.AutoLoadInProgress = true
	apiSvc.AutoLoadMutex.Unlock()

	if !apiSvc.Enabled {
		apiSvc.AutoLoadMutex.Lock()
		apiSvc.AutoLoadInProgress = false
		apiSvc.AutoLoadMutex.Unlock()
		log.Printf("AutoLoadProxies: Clash API is disabled, skipping")
		return
	}

	selectedGroup := apiSvc.GetSelectedClashGroup()
	if selectedGroup == "" {
		apiSvc.AutoLoadMutex.Lock()
		apiSvc.AutoLoadInProgress = false
		apiSvc.AutoLoadMutex.Unlock()
		log.Printf("AutoLoadProxies: No group selected, skipping")
		return
	}

	intervals := []time.Duration{1, 3, 3, 5, 5, 5, 5, 5, 10, 10, 10, 10, 15, 15}

	go func() {
		for attempt, interval := range intervals {
			// Check if context is cancelled
			select {
			case <-ctx.Done():
				log.Println("AutoLoadProxies: Stopped (context cancelled)")
				apiSvc.AutoLoadMutex.Lock()
				apiSvc.AutoLoadInProgress = false
				apiSvc.AutoLoadMutex.Unlock()
				return
			default:
			}

			// Wait for the interval (except first attempt)
			if attempt > 0 {
				select {
				case <-ctx.Done():
					log.Println("AutoLoadProxies: Stopped during wait (context cancelled)")
					apiSvc.AutoLoadMutex.Lock()
					apiSvc.AutoLoadInProgress = false
					apiSvc.AutoLoadMutex.Unlock()
					return
				case <-time.After(interval * time.Second):
					// Continue
				}
			}

			// Check if sing-box is running before attempting to connect
			if !apiSvc.RunningStateIsRunning() {
				log.Printf("AutoLoadProxies: Attempt %d/%d skipped - sing-box is not running", attempt+1, len(intervals))
				// Continue to next attempt
				continue
			}

			log.Printf("AutoLoadProxies: Attempt %d/%d to load proxies for group '%s'", attempt+1, len(intervals), selectedGroup)

			// Get current group (it might have changed)
			apiSvc.StateMutex.RLock()
			currentGroup := apiSvc.SelectedClashGroup
			baseURL := apiSvc.BaseURL
			token := apiSvc.Token
			apiSvc.StateMutex.RUnlock()

			if currentGroup == "" {
				log.Printf("AutoLoadProxies: Group cleared, stopping attempts")
				return
			}

			// Try to load proxies
			proxies, now, err := api.GetProxiesInGroup(baseURL, token, currentGroup, apiSvc.ApiLogFile)
			if err != nil {
				log.Printf("AutoLoadProxies: Attempt %d failed: %v", attempt+1, err)
				// Continue to next attempt
				continue
			}

			// Success - update proxies list
			fyne.Do(func() {
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
					log.Printf("AutoLoadProxies: Switching to saved proxy '%s' (current: '%s') for group '%s'", lastSelected, now, currentGroup)
					// Переключаемся на сохраненный прокси (мы уже в goroutine, дополнительная не нужна)
					if err := apiSvc.SwitchProxy(currentGroup, lastSelected); err != nil {
						log.Printf("AutoLoadProxies: Failed to switch to saved proxy '%s' for group '%s': %v", lastSelected, currentGroup, err)
						// Не критично, продолжаем с текущим прокси
					}
				}
			}

			log.Printf("AutoLoadProxies: Successfully loaded %d proxies for group '%s' on attempt %d", len(proxies), currentGroup, attempt+1)

			apiSvc.AutoLoadMutex.Lock()
			apiSvc.AutoLoadInProgress = false
			apiSvc.AutoLoadMutex.Unlock()
			return // Success, stop retrying
		}

		log.Printf("AutoLoadProxies: All %d attempts failed", len(intervals))
		apiSvc.AutoLoadMutex.Lock()
		apiSvc.AutoLoadInProgress = false
		apiSvc.AutoLoadMutex.Unlock()
	}()
}

// SwitchProxy switches to the specified proxy in the selected group.
func (apiSvc *APIService) SwitchProxy(group, proxyName string) error {
	baseURL, token, enabled := apiSvc.GetClashAPIConfig()
	if !enabled {
		return fmt.Errorf("Clash API is disabled")
	}

	err := api.SwitchProxy(baseURL, token, group, proxyName, apiSvc.ApiLogFile)
	if err != nil {
		return fmt.Errorf("failed to switch proxy: %w", err)
	}

	apiSvc.SetActiveProxyName(proxyName)
	// Сохраняем последний выбранный прокси для текущей группы для автоматического переключения при следующем старте
	apiSvc.SetLastSelectedProxyForGroup(group, proxyName)

	// Notify about proxy switch
	if apiSvc.OnProxySwitched != nil {
		apiSvc.OnProxySwitched()
	}

	return nil
}
