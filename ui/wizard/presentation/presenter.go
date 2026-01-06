// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter.go определяет WizardPresenter - презентер, который связывает GUI и бизнес-логику.
//
// WizardPresenter:
//   - Реализует UIUpdater для обновления GUI из бизнес-логики
//   - Хранит модель (WizardModel) и GUI-состояние (GUIState)
//   - Синхронизирует данные между моделью и GUI (SyncModelToGUI, SyncGUIToModel)
//   - Управляет открытыми диалогами (OpenRuleDialogs)
//   - Координирует вызовы бизнес-логики и обновление GUI
//
// Презентер является единственной точкой взаимодействия между GUI (табы, виджеты) и бизнес-логикой,
// что обеспечивает четкое разделение ответственности и делает код тестируемым.
package presentation

import (
	"fyne.io/fyne/v2"

	"singbox-launcher/core"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardmodels "singbox-launcher/ui/wizard/models"
)

// WizardPresenter связывает GUI и бизнес-логику визарда.
type WizardPresenter struct {
	model           *wizardmodels.WizardModel
	guiState        *GUIState
	controller      *core.AppController
	templateLoader  wizardbusiness.TemplateLoader
	openRuleDialogs map[int]fyne.Window
}

// NewWizardPresenter создает новый презентер визарда.
func NewWizardPresenter(model *wizardmodels.WizardModel, guiState *GUIState, controller *core.AppController, templateLoader wizardbusiness.TemplateLoader) *WizardPresenter {
	return &WizardPresenter{
		model:           model,
		guiState:        guiState,
		controller:      controller,
		templateLoader:  templateLoader,
		openRuleDialogs: make(map[int]fyne.Window),
	}
}

// Model возвращает модель визарда.
func (p *WizardPresenter) Model() *wizardmodels.WizardModel {
	return p.model
}

// GUIState возвращает GUI-состояние визарда.
func (p *WizardPresenter) GUIState() *GUIState {
	return p.guiState
}

// ConfigServiceAdapter возвращает адаптер ConfigService.
func (p *WizardPresenter) ConfigServiceAdapter() wizardbusiness.ConfigService {
	return &wizardbusiness.ConfigServiceAdapter{CoreConfigService: p.controller.ConfigService}
}

// Controller возвращает AppController.
func (p *WizardPresenter) Controller() *core.AppController {
	return p.controller
}
