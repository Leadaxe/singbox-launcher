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
// Также содержит утилиту SafeFyneDo для безопасного обновления GUI из других горутин.
// SafeFyneDo используется во всех методах презентера, которые обновляют Fyne виджеты.
//
// Презентер является единственной точкой взаимодействия между GUI (табы, виджеты) и бизнес-логикой,
// что обеспечивает четкое разделение ответственности и делает код тестируемым.
//
// Презентер - это центральный компонент архитектуры MVP (Model-View-Presenter).
// Он выделен в отдельный файл, так как является базовым структурным компонентом,
// на котором строятся все остальные методы презентера (разделены по файлам для организации кода).
//
// Используется в:
//   - wizard.go - создается при инициализации визарда и передается во все табы и диалоги
//   - tabs/*.go - все табы получают presenter для взаимодействия с моделью и бизнес-логикой
//   - dialogs/*.go - диалоги используют presenter для обновления модели и GUI
//   - presentation/*.go - все остальные файлы presentation расширяют функциональность WizardPresenter
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

// SafeFyneDo безопасно выполняет функцию в UI потоке Fyne.
// Проверяет, что window не nil, перед вызовом fyne.Do.
// Используется во всех методах презентера для безопасного обновления GUI из других горутин.
func SafeFyneDo(window fyne.Window, fn func()) {
	if window != nil {
		fyne.Do(fn)
	}
}
