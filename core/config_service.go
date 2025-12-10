// Package core provides core application logic including process management,
// configuration parsing, and service orchestration.
package core

import (
	"fmt"
	"log"

	"singbox-launcher/internal/dialogs"
)

// ConfigService encapsulates configuration parsing and update routines.
// It handles fetching subscriptions, parsing proxy nodes, generating JSON outbounds,
// and updating the configuration file. The service maintains separation of concerns
// by isolating all configuration-related operations from the main controller.
type ConfigService struct {
	ac *AppController
}

// NewConfigService constructs a ConfigService bound to the controller.
// The service requires an initialized AppController with valid ConfigPath.
func NewConfigService(ac *AppController) *ConfigService {
	return &ConfigService{ac: ac}
}

// RunParserProcess starts the internal configuration update process.
// Logic migrated from controller-level function without behavior changes.
func (svc *ConfigService) RunParserProcess() {
	ac := svc.ac
	// Проверяем, не запущен ли уже парсинг
	ac.ParserMutex.Lock()
	if ac.ParserRunning {
		ac.ParserMutex.Unlock()
		dialogs.ShowAutoHideInfo(ac.Application, ac.MainWindow, "Parser Info", "Configuration update is already in progress.")
		return
	}
	ac.ParserRunning = true
	ac.ParserMutex.Unlock()

	log.Println("RunParser: Starting internal configuration update...")
	// Ensure flag is reset after completion, even if there's an error
	defer func() {
		ac.ParserMutex.Lock()
		ac.ParserRunning = false
		ac.ParserMutex.Unlock()
	}()

	// Call internal parser to update configuration
	err := svc.UpdateConfigFromSubscriptions()

	// Обрабатываем результат
	if err != nil {
		log.Printf("RunParser: Failed to update config: %v", err)
		// Progress already updated in UpdateConfigFromSubscriptions with error status
		ac.ShowParserError(fmt.Errorf("failed to update config: %w", err))
	} else {
		log.Println("RunParser: Config updated successfully.")
		// Progress already updated in UpdateConfigFromSubscriptions with success status
		dialogs.ShowAutoHideInfo(ac.Application, ac.MainWindow, "Parser", "Config updated successfully!")
	}
}
