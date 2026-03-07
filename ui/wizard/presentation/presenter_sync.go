// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_sync.go содержит методы синхронизации данных между моделью и GUI:
//   - SyncModelToGUI - обновляет виджеты GUI из модели данных (SourceURLs, ParserConfigJSON, SelectedFinalOutbound)
//   - SyncGUIToModel - обновляет модель данных из виджетов GUI (обратная синхронизация)
//
// Эти методы обеспечивают двустороннюю синхронизацию между WizardModel и GUIState,
// что является ключевой частью архитектуры MVP.
//
// Синхронизация данных - это отдельная ответственность от других методов презентера.
// Методы синхронизации используются в разных местах (перед сохранением, при инициализации).
//
// Используется в:
//   - wizard.go - SyncModelToGUI вызывается при инициализации визарда для установки начальных значений
//   - presenter_save.go - SyncGUIToModel вызывается перед сохранением для получения актуальных данных
//   - presenter_async.go - SyncGUIToModel вызывается перед парсингом для получения актуальных данных
package presentation

import (
	"encoding/json"
	"fmt"
	"strings"

	"fyne.io/fyne/v2/dialog"

	"singbox-launcher/core/config"
	wizardbusiness "singbox-launcher/ui/wizard/business"
)

// SyncModelToGUI синхронизирует данные из модели в GUI.
// SourceURLEntry shows SourceURLs (input for Add only); source list is from ParserConfig.Proxies in refreshSourcesList.
func (p *WizardPresenter) SyncModelToGUI() {
	p.UpdateUI(func() {
		if p.guiState.SourceURLEntry != nil {
			p.guiState.SourceURLEntry.SetText(p.model.SourceURLs)
		}
		if p.guiState.ParserConfigEntry != nil {
			p.guiState.ParserConfigEntry.SetText(p.model.ParserConfigJSON)
			p.guiState.LastValidParserConfigJSON = p.model.ParserConfigJSON
		}
		if p.guiState.RefreshSourcesList != nil {
			p.guiState.RefreshSourcesList()
		}
		if p.guiState.FinalOutboundSelect != nil {
			options := wizardbusiness.EnsureDefaultAvailableOutbounds(wizardbusiness.GetAvailableOutbounds(p.model))
			p.guiState.FinalOutboundSelect.Options = options
			p.guiState.FinalOutboundSelect.SetSelected(p.model.SelectedFinalOutbound)
			p.guiState.FinalOutboundSelect.Refresh()
		}
	})

	// Пересоздаем вкладку Rules, если она уже создана (часть синхронизации модели с GUI)
	// Это обновит чекбоксы и селекторы правил в соответствии с текущим состоянием модели
	if p.createRulesTabFunc != nil && p.guiState.Tabs != nil {
		// Проверяем, существует ли вкладка Rules
		for _, tabItem := range p.guiState.Tabs.Items {
			if tabItem.Text == "Rules" {
				p.RefreshRulesTabAfterLoadState()
				break
			}
		}
	}
}

// SyncGUIToModel синхронизирует данные из GUI в модель.
// Устанавливает флаг изменений, если данные реально изменились.
func (p *WizardPresenter) SyncGUIToModel() {
	changed := false

	if p.guiState.SourceURLEntry != nil {
		newValue := p.guiState.SourceURLEntry.Text
		if p.model.SourceURLs != newValue {
			p.model.SourceURLs = newValue
			changed = true
		}
	}
	if p.guiState.ParserConfigEntry != nil {
		newValue := p.guiState.ParserConfigEntry.Text
		if p.model.ParserConfigJSON != newValue {
			p.model.ParserConfigJSON = newValue
			changed = true
		}
	}
	if p.guiState.FinalOutboundSelect != nil {
		newValue := p.guiState.FinalOutboundSelect.Selected
		if p.model.SelectedFinalOutbound != newValue {
			p.model.SelectedFinalOutbound = newValue
			changed = true
		}
	}

	// Устанавливаем флаг изменений, если данные изменились
	if changed {
		p.MarkAsChanged()
	}
}

// ValidateAndApplyParserConfigFromEntry parses ParserConfig from the entry, validates it,
// and on success updates model and LastValidParserConfigJSON; on error shows dialog and reverts entry.
// Call when leaving the Outbounds and ParserConfig tab so manual JSON edits are applied or reverted.
func (p *WizardPresenter) ValidateAndApplyParserConfigFromEntry() {
	if p.guiState.ParserConfigEntry == nil {
		return
	}
	text := strings.TrimSpace(p.guiState.ParserConfigEntry.Text)
	if text == "" {
		p.model.ParserConfigJSON = ""
		p.model.ParserConfig = nil
		wizardbusiness.InvalidatePreviewCache(p.model)
		p.guiState.LastValidParserConfigJSON = ""
		return
	}
	pc := &config.ParserConfig{}
	if err := json.Unmarshal([]byte(text), pc); err != nil {
		dialog.ShowError(fmt.Errorf("Invalid ParserConfig JSON: %w", err), p.guiState.Window)
		revert := p.guiState.LastValidParserConfigJSON
		p.guiState.ParserConfigUpdating = true
		p.guiState.ParserConfigEntry.SetText(revert)
		p.guiState.ParserConfigUpdating = false
		return
	}
	if err := wizardbusiness.ValidateParserConfig(pc); err != nil {
		dialog.ShowError(fmt.Errorf("Invalid ParserConfig: %w", err), p.guiState.Window)
		revert := p.guiState.LastValidParserConfigJSON
		p.guiState.ParserConfigUpdating = true
		p.guiState.ParserConfigEntry.SetText(revert)
		p.guiState.ParserConfigUpdating = false
		return
	}
	serialized, err := wizardbusiness.SerializeParserConfig(pc)
	if err != nil {
		dialog.ShowError(fmt.Errorf("Failed to serialize ParserConfig: %w", err), p.guiState.Window)
		return
	}
	p.model.ParserConfig = pc
	p.model.ParserConfigJSON = serialized
	p.guiState.LastValidParserConfigJSON = serialized
	p.UpdateParserConfig(serialized)
	p.RefreshOutboundOptions()
	if p.guiState.RefreshSourcesList != nil {
		p.guiState.RefreshSourcesList()
	}
	p.model.PreviewNeedsParse = true
	wizardbusiness.InvalidatePreviewCache(p.model)
}
