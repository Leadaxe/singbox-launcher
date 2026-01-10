// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_rules.go содержит методы для работы с правилами маршрутизации:
//   - RefreshRulesTab - обновляет содержимое таба Rules после изменений в модели
//   - OpenRuleDialogs - возвращает карту открытых диалогов редактирования правил
//
// Эти методы обеспечивают синхронизацию UI с моделью при изменении правил
// (добавление, удаление, редактирование пользовательских правил).
//
// Методы работы с правилами имеют отдельную ответственность от других методов презентера.
// RefreshRulesTab содержит логику поиска и обновления конкретного таба.
//
// Используется в:
//   - dialogs/add_rule_dialog.go - вызывает RefreshRulesTab после сохранения правила
//   - tabs/rules_tab.go - вызывает OpenRuleDialogs для проверки открытых диалогов
package presentation

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"

	"singbox-launcher/internal/debuglog"
)

// RefreshRulesTab refreshes the Rules tab content.
// createRulesTab is a function that creates the rules tab content.
func (p *WizardPresenter) RefreshRulesTab(createRulesTab func(*WizardPresenter) fyne.CanvasObject) {
	if p.guiState.Tabs == nil {
		debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, "RefreshRulesTab: Tabs is nil")
		return
	}

	// Find Rules tab
	var rulesTabItem *container.TabItem
	for _, tabItem := range p.guiState.Tabs.Items {
		if tabItem.Text == "Rules" {
			rulesTabItem = tabItem
			break
		}
	}

	if rulesTabItem == nil {
		debuglog.Log("ERROR", debuglog.LevelError, debuglog.UseGlobal, "RefreshRulesTab: Rules tab not found")
		return
	}

	// Create new content
	newContent := createRulesTab(p)

	// Update tab content
	rulesTabItem.Content = newContent
	p.guiState.Tabs.Refresh()
}

// OpenRuleDialogs returns the map of open rule dialogs.
func (p *WizardPresenter) OpenRuleDialogs() map[int]fyne.Window {
	return p.openRuleDialogs
}


