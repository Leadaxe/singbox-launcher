// Package presentation содержит слой представления визарда конфигурации.
//
// Файл gui_state.go определяет GUIState - состояние GUI визарда (только Fyne виджеты).
//
// GUIState содержит только GUI-виджеты и UI-флаги состояния:
//   - Виджеты основного окна и табов (Entry, Label, Button, ProgressBar, Select и т.д.)
//   - Контейнеры и placeholder'ы для компоновки
//   - RuleWidget - структуры-обертки, связывающие виджеты Select с правилами из модели
//   - UI-флаги состояния операций (SaveInProgress и т.д.)
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
	ttwidget "github.com/dweymouth/fyne-tooltip/widget"
)

// RuleWidget связывает виджеты Select, Check и SRS button с правилом из модели.
type RuleWidget struct {
	Select    *widget.Select
	Checkbox  *widget.Check    // Может быть nil, если правило не имеет чекбокса
	SRSButton *ttwidget.Button // ⬇/🔄/✔️ для SRS: тот же виджет, что fynewidget.HoverForwardTTButton (см. TTWidget)
	RuleState interface{}      // *models.RuleState - используется interface{} чтобы избежать циклических зависимостей
}

// GUIState содержит только GUI-виджеты и UI-флаги состояния.
type GUIState struct {
	Window              fyne.Window
	ChildWindowsOverlay fyne.CanvasObject

	// Tab 1: Sources & ParserConfig
	SourceURLEntry    *widget.Entry
	ParserConfigEntry *widget.Entry
	ParseButton       *widget.Button

	// Template tab widgets
	TemplatePreviewEntry       *widget.Entry
	TemplatePreviewStatusLabel *widget.Label
	ShowPreviewButton          *widget.Button
	FinalOutboundSelect        *widget.Select
	RuleOutboundSelects        []*RuleWidget

	// Navigation buttons
	ReadButton        *widget.Button
	SaveAsButton      *widget.Button
	CloseButton       *widget.Button
	PrevButton        *widget.Button
	NextButton        *widget.Button
	SaveButton        *widget.Button
	SaveProgress      *widget.ProgressBar
	SavePlaceholder   *canvas.Rectangle
	SaveStatusLabel   *widget.Label // Status text left of Prev (e.g. "Building config...")
	ButtonsContainer  fyne.CanvasObject
	Tabs              *container.AppTabs
	RulesScroll       *container.Scroll
	RulesScrollOffset fyne.Position

	// Optional refresh for Sources list (set by CreateSourcesTab); called from SyncModelToGUI.
	RefreshSourcesList func()

	// Optional refresh for Outbounds configurator list (set by CreateOutboundsAndParserConfigTab).
	// Must run after ParserConfig/proxies change from Sources Edit, UpdateParserConfig, or tab switch.
	RefreshOutboundsConfiguratorList func()

	// DNS tab
	DNSRulesEntry            *widget.Entry
	DNSFinalSelect           *widget.Select
	DNSDefaultResolverSelect *widget.Select
	DNSStrategySelect        *widget.Select
	DNSIndependentCacheCheck *widget.Check
	RefreshDNSList           func()

	// RefreshSettingsFromModel пересобирает вкладку Settings из model.TemplateData.Vars (после LoadState / шаблона).
	RefreshSettingsFromModel func()

	// Last valid ParserConfig JSON for revert on validation error (e.g. on tab switch from Outbounds tab).
	LastValidParserConfigJSON string

	// UI-флаги состояния операций
	SaveInProgress          bool
	ParserConfigUpdating    bool
	UpdatingOutboundOptions bool
	// DNSSelectsProgrammatic: SetSelected в refreshDNSSelectsFromModel — не писать модель из OnChanged селектов.
	DNSSelectsProgrammatic bool

	// WizardWidgetsReady: после завершения applyWizardWidgetsFromModel (первый кадр SyncModelToGUI).
	// До этого MergeGUIToModel не трогает модель; SyncGUIToModel всё ещё может применяться (сохранение) с ветками «keep model» для пустых виджетов.
	WizardWidgetsReady bool

	// SourceURLsProgrammatic / DNSRulesProgrammatic: SetText из модели — не считать за правку пользователя.
	SourceURLsProgrammatic bool
	DNSRulesProgrammatic   bool
}
