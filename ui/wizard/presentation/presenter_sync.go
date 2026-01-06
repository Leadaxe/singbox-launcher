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
	wizardbusiness "singbox-launcher/ui/wizard/business"
)

// SyncModelToGUI синхронизирует данные из модели в GUI.
func (p *WizardPresenter) SyncModelToGUI() {
	SafeFyneDo(p.guiState.Window, func() {
		if p.guiState.SourceURLEntry != nil {
			p.guiState.SourceURLEntry.SetText(p.model.SourceURLs)
		}
		if p.guiState.ParserConfigEntry != nil {
			p.guiState.ParserConfigEntry.SetText(p.model.ParserConfigJSON)
		}
		if p.guiState.FinalOutboundSelect != nil {
			options := wizardbusiness.EnsureDefaultAvailableOutbounds(wizardbusiness.GetAvailableOutbounds(p.model))
			p.guiState.FinalOutboundSelect.Options = options
			p.guiState.FinalOutboundSelect.SetSelected(p.model.SelectedFinalOutbound)
			p.guiState.FinalOutboundSelect.Refresh()
		}
	})
}

// SyncGUIToModel синхронизирует данные из GUI в модель.
func (p *WizardPresenter) SyncGUIToModel() {
	if p.guiState.SourceURLEntry != nil {
		p.model.SourceURLs = p.guiState.SourceURLEntry.Text
	}
	if p.guiState.ParserConfigEntry != nil {
		p.model.ParserConfigJSON = p.guiState.ParserConfigEntry.Text
	}
	if p.guiState.FinalOutboundSelect != nil {
		p.model.SelectedFinalOutbound = p.guiState.FinalOutboundSelect.Selected
	}
}

