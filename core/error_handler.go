package core

import (
	"fmt"
	"log"

	"singbox-launcher/internal/dialogs"
)

// ShowConfigError shows a config error banner in the UI
func (ac *AppController) ShowConfigError(message string) {
	if ac.UIService != nil && ac.UIService.MainWindow != nil {
		dialogs.ShowError(ac.UIService.MainWindow, fmt.Errorf("Configuration Error: %s", message))
	}
	log.Printf("ConfigError: %s", message)
}

// ShowStartupError shows an error when sing-box fails to start
func (ac *AppController) ShowStartupError(err error) {
	message := fmt.Sprintf("Failed to start sing-box:\n\n%s\n\nPlease check:\n1. config.json is valid\n2. sing-box executable exists\n3. Check logs for details", err.Error())
	if ac.UIService != nil && ac.UIService.MainWindow != nil {
		dialogs.ShowError(ac.UIService.MainWindow, fmt.Errorf(message))
	}
	log.Printf("StartupError: %v", err)
}

// ShowParserError shows an error when parser fails
func (ac *AppController) ShowParserError(err error) {
	message := fmt.Sprintf("Parser failed:\n\n%s\n\nPlease check:\n1. Subscription URL is valid\n2. Network connection\n3. Check parser.log for details", err.Error())
	if ac.UIService != nil && ac.UIService.MainWindow != nil {
		dialogs.ShowError(ac.UIService.MainWindow, fmt.Errorf(message))
	}
	log.Printf("ParserError: %v", err)
}

// ShowConfigValidationError shows an error when config validation fails
func (ac *AppController) ShowConfigValidationError(err error) {
	message := fmt.Sprintf("Config validation failed:\n\n%s\n\nPlease check config.json syntax and required fields.", err.Error())
	if ac.UIService != nil && ac.UIService.MainWindow != nil {
		dialogs.ShowError(ac.UIService.MainWindow, fmt.Errorf(message))
	}
	log.Printf("ConfigValidationError: %v", err)
}
