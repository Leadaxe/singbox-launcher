// Package presentation содержит слой представления визарда конфигурации.
//
// Файл gui_state.go определяет GUIState - состояние GUI визарда (только Fyne виджеты).
//
// GUIState содержит только GUI-виджеты и UI-флаги состояния:
//   - Виджеты основного окна и табов (Entry, Label, Button, ProgressBar, Select и т.д.)
//   - Контейнеры и placeholder'ы для компоновки
//   - RuleWidget - структуры-обертки, связывающие виджеты Select с правилами из модели
//   - UI-флаги состояния операций (CheckURLInProgress, SaveInProgress)
//   - Флаги блокировки для предотвращения рекурсивных обновлений
//
// В отличие от WizardState, GUIState НЕ содержит бизнес-данных (ParserConfig, GeneratedOutbounds и т.д.).
// Бизнес-данные находятся в models.WizardModel, что позволяет разделить GUI и бизнес-логику.
//
// Связь между GUIState и WizardModel осуществляется через WizardPresenter,
// который синхронизирует данные между моделью и GUI.
//
// GUIState выделен в отдельный файл для четкого разделения ответственности:
// это часть рефакторинга от монолитного WizardState (который смешивал GUI и бизнес-данные)
// к архитектуре MVP, где GUI полностью отделен от бизнес-логики.
//
// Используется в:
//   - presentation/presenter.go - WizardPresenter хранит GUIState и обновляет его виджеты
//   - presentation/presenter_*.go - все методы презентера обновляют виджеты через GUIState
//   - wizard.go - создается при инициализации визарда и передается в презентер
package presentation

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// RuleWidget связывает виджет Select с правилом из модели.
type RuleWidget struct {
	Select    *widget.Select
	RuleState interface{} // *models.RuleState - используется interface{} чтобы избежать циклических зависимостей
}

// GUIState содержит только GUI-виджеты и UI-флаги состояния.
type GUIState struct {
	Window fyne.Window

	// Tab 1: Sources & ParserConfig
	SourceURLEntry      *widget.Entry
	URLStatusLabel      *widget.Label
	ParserConfigEntry   *widget.Entry
	OutboundsPreview    *widget.Entry
	CheckURLButton      *widget.Button
	CheckURLProgress    *widget.ProgressBar
	CheckURLPlaceholder *canvas.Rectangle
	CheckURLContainer   fyne.CanvasObject
	ParseButton         *widget.Button

	// Template tab widgets
	TemplatePreviewEntry       *widget.Entry
	TemplatePreviewStatusLabel *widget.Label
	ShowPreviewButton          *widget.Button
	FinalOutboundSelect        *widget.Select
	RuleOutboundSelects        []*RuleWidget

	// Navigation buttons
	CloseButton      *widget.Button
	PrevButton       *widget.Button
	NextButton       *widget.Button
	SaveButton       *widget.Button
	SaveProgress     *widget.ProgressBar
	SavePlaceholder  *canvas.Rectangle
	ButtonsContainer fyne.CanvasObject
	Tabs             *container.AppTabs

	// UI-флаги состояния операций
	CheckURLInProgress      bool
	SaveInProgress          bool
	ParserConfigUpdating    bool
	UpdatingOutboundOptions bool
}
