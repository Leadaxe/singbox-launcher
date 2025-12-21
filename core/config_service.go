// Package core provides core application logic including process management,
// configuration parsing, and service orchestration.
package core

import (
	"fmt"
	"log"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/parser"
	"singbox-launcher/core/config/subscription"
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

// updateParserProgress safely calls UpdateParserProgressFunc if it's not nil
func updateParserProgress(ac *AppController, progress float64, status string) {
	if ac.UpdateParserProgressFunc != nil {
		ac.UpdateParserProgressFunc(progress, status)
	}
}

// ProcessProxySource delegates to subscription.LoadNodesFromSource
func (svc *ConfigService) ProcessProxySource(proxySource config.ProxySource, tagCounts map[string]int, progressCallback func(float64, string), subscriptionIndex, totalSubscriptions int) ([]*config.ParsedNode, error) {
	return subscription.LoadNodesFromSource(proxySource, tagCounts, progressCallback, subscriptionIndex, totalSubscriptions)
}

// GenerateSelector delegates to config.GenerateSelector
func (svc *ConfigService) GenerateSelector(allNodes []*config.ParsedNode, outboundConfig config.OutboundConfig) (string, error) {
	return config.GenerateSelector(allNodes, outboundConfig)
}

// GenerateNodeJSON delegates to config.GenerateNodeJSON
func (svc *ConfigService) GenerateNodeJSON(node *config.ParsedNode) (string, error) {
	return config.GenerateNodeJSON(node)
}

// GenerateOutboundsFromParserConfig delegates to config.GenerateOutboundsFromParserConfig
func (svc *ConfigService) GenerateOutboundsFromParserConfig(
	parserConfig *config.ParserConfig,
	tagCounts map[string]int,
	progressCallback func(float64, string),
) (*config.OutboundGenerationResult, error) {
	// Create a wrapper function that matches the signature expected by config.GenerateOutboundsFromParserConfig
	loadNodesFunc := func(ps config.ProxySource, tc map[string]int, pc func(float64, string), idx, total int) ([]*config.ParsedNode, error) {
		return svc.ProcessProxySource(ps, tc, pc, idx, total)
	}
	return config.GenerateOutboundsFromParserConfig(parserConfig, tagCounts, progressCallback, loadNodesFunc)
}

// UpdateConfigFromSubscriptions delegates to config.UpdateConfigFromSubscriptions
func (svc *ConfigService) UpdateConfigFromSubscriptions() error {
	ac := svc.ac

	// Step 1: Extract configuration
	parserConfig, err := parser.ExtractParserConfig(ac.ConfigPath)
	if err != nil {
		updateParserProgress(ac, -1, fmt.Sprintf("Error: %v", err))
		return fmt.Errorf("failed to extract parser config: %w", err)
	}

	// Update progress: Step 1 completed
	updateParserProgress(ac, 5, "Parsed ParserConfig block")

	progressCallback := func(p float64, s string) {
		updateParserProgress(ac, p, s)
	}

	// Create a wrapper function that matches the signature expected by config.UpdateConfigFromSubscriptions
	loadNodesFunc := func(ps config.ProxySource, tc map[string]int, pc func(float64, string), idx, total int) ([]*config.ParsedNode, error) {
		return svc.ProcessProxySource(ps, tc, pc, idx, total)
	}

	err = config.UpdateConfigFromSubscriptions(ac.ConfigPath, parserConfig, progressCallback, loadNodesFunc)
	if err == nil {
		// Resume auto-update after successful update
		ac.resumeAutoUpdate()
	}
	return err
}
