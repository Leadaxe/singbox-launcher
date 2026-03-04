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
	model                  *wizardmodels.WizardModel
	guiState               *GUIState
	templateLoader         wizardbusiness.TemplateLoader
	openRuleDialogs        map[int]fyne.Window
	openViewWindow         fyne.Window // single View (source servers) window
	openOutboundEditWindow fyne.Window // single Outbound Edit/Add window
	hasChanges             bool                                     // Отслеживает наличие несохранённых изменений
	createRulesTabFunc     func(*WizardPresenter) fyne.CanvasObject // Функция для создания вкладки Rules (устанавливается при инициализации)
}

// NewWizardPresenter создает новый презентер визарда.
func NewWizardPresenter(model *wizardmodels.WizardModel, guiState *GUIState, templateLoader wizardbusiness.TemplateLoader) *WizardPresenter {
	presenter := &WizardPresenter{
		model:           model,
		guiState:        guiState,
		templateLoader:  templateLoader,
		openRuleDialogs: make(map[int]fyne.Window),
		hasChanges:      false,
	}
	return presenter
}

// Model возвращает модель визарда.
func (p *WizardPresenter) Model() *wizardmodels.WizardModel {
	return p.model
}

// GUIState возвращает GUI-состояние визарда.
func (p *WizardPresenter) GUIState() *GUIState {
	return p.guiState
}

// SetCreateRulesTabFunc устанавливает функцию для создания вкладки Rules.
// Вызывается при инициализации визарда для возможности пересоздания вкладки после LoadState.
func (p *WizardPresenter) SetCreateRulesTabFunc(createRulesTab func(*WizardPresenter) fyne.CanvasObject) {
	p.createRulesTabFunc = createRulesTab
}

// ConfigServiceAdapter возвращает адаптер ConfigService.
func (p *WizardPresenter) ConfigServiceAdapter() wizardbusiness.ConfigService {
	ac := core.GetController()
	if ac == nil {
		return nil
	}
	return &wizardbusiness.ConfigServiceAdapter{CoreConfigService: ac.ConfigService}
}

// Controller возвращает AppController.
func (p *WizardPresenter) Controller() *core.AppController {
	return core.GetController()
}

// SafeFyneDo безопасно выполняет функцию в UI потоке Fyne.
// Проверяет, что window не nil, перед вызовом fyne.Do.
// Используется во всех методах презентера для безопасного обновления GUI из других горутин.
func SafeFyneDo(window fyne.Window, fn func()) {
	if window != nil {
		fyne.Do(fn)
	}
}

// UpdateUI безопасно обновляет UI элементы через SafeFyneDo.
// Инкапсулирует доступ к guiState.Window внутри презентера.
func (p *WizardPresenter) UpdateUI(fn func()) {
	SafeFyneDo(p.guiState.Window, fn)
}

// Child windows contract: see docs/WIZARD_CHILD_WINDOWS.md (register on open, unregister on close, UpdateChildOverlay, single instance for View/Edit).

// OpenViewWindow returns the open View (source servers) window if any.
func (p *WizardPresenter) OpenViewWindow() fyne.Window { return p.openViewWindow }

// SetViewWindow registers the View window; call ClearViewWindow when it closes.
func (p *WizardPresenter) SetViewWindow(w fyne.Window) { p.openViewWindow = w }

// ClearViewWindow unregisters the View window.
func (p *WizardPresenter) ClearViewWindow() { p.openViewWindow = nil }

// OpenOutboundEditWindow returns the open Outbound Edit/Add window if any.
func (p *WizardPresenter) OpenOutboundEditWindow() fyne.Window { return p.openOutboundEditWindow }

// SetOutboundEditWindow registers the Outbound Edit window; call ClearOutboundEditWindow when it closes.
func (p *WizardPresenter) SetOutboundEditWindow(w fyne.Window) { p.openOutboundEditWindow = w }

// ClearOutboundEditWindow unregisters the Outbound Edit window.
func (p *WizardPresenter) ClearOutboundEditWindow() { p.openOutboundEditWindow = nil }

// UpdateChildOverlay shows or hides the child-windows overlay based on open rule dialogs, View window, and Outbound Edit window.
func (p *WizardPresenter) UpdateChildOverlay() {
	if p.guiState == nil || p.guiState.ChildWindowsOverlay == nil {
		return
	}
	anyOpen := len(p.openRuleDialogs) > 0 || p.openViewWindow != nil || p.openOutboundEditWindow != nil
	if anyOpen {
		p.guiState.ChildWindowsOverlay.Show()
	} else {
		p.guiState.ChildWindowsOverlay.Hide()
	}
	p.guiState.ChildWindowsOverlay.Refresh()
}
