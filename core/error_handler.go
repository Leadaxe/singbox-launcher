package core

import (
	"fmt"

	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/dialogs"
	"singbox-launcher/internal/locale"
)

// Длинные тексты локализации: ключ = английский текст (SPEC 111).
const (
	parserText  = "Parser failed:\n\n%s\n\nPlease check:\n1. Subscription URL is valid\n2. Network connection\n3. Check parser.log for details"
	startupText = "Failed to start sing-box:\n\n%s\n\nPlease check:\n1. config.json is valid\n2. sing-box executable exists\n3. Check logs for details"
)

// showErrorUI logs the error and shows it in the UI if available.
// category is used as a log prefix (e.g. "StartupError", "ParserError").
func (ac *AppController) showErrorUI(category string, err error) {
	debuglog.ErrorLog("%s: %v", category, err)
	if ac.hasUI() {
		dialogs.ShowError(ac.UIService.MainWindow, err)
	}
}

// ShowStartupError shows an error when sing-box fails to start.
func (ac *AppController) ShowStartupError(err error) {
	ac.showErrorUI("StartupError", fmt.Errorf("%s", locale.Tf(startupText, err.Error())))
}

// ShowParserError shows an error when parser fails.
func (ac *AppController) ShowParserError(err error) {
	ac.showErrorUI("ParserError", fmt.Errorf("%s", locale.Tf(parserText, err.Error())))
}
