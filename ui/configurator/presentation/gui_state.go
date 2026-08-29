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
	SourceURLEntry *widget.Entry
	ParseButton    *widget.Button

	// Template tab widgets
	FinalOutboundSelect *widget.Select
	RuleOutboundSelects []*RuleWidget

	// Navigation buttons
	ReadButton        *widget.Button
	CloneFromButton   *widget.Button
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

	// RevealSource подсвечивает и прокручивает к строке источника с данным
	// ULID (SPEC 115 §3, переход «показать источник» из отчёта «Итога»).
	// Ставится CreateSourcesTab; nil до её постройки. Переключение самой
	// вкладки — забота вызывающего: список не знает, показан ли он.
	//
	// Неизвестный ULID — законный исход, а не ошибка: источник могли удалить
	// между сборкой и кликом по строке отчёта.
	RevealSource func(sourceID string)

	// RunFinalBuild запускает сборку в памяти для вкладки «Итог»
	// (SPEC 115 §1). Ставится CreateFinalTab; зовётся из обработчика смены
	// вкладок. Возврат мгновенный — сама сборка уходит в горутину, повторные
	// входы схлопываются.
	RunFinalBuild func()

	// Optional refresh for Outbounds configurator list (set by CreateDirectionsTab).
	// Must run after sources/directions change from Sources Edit or tab switch.
	RefreshOutboundsConfiguratorList func()

	// DNS tab
	DNSRulesEntry            *widget.Entry
	DNSFinalSelect           *widget.Select
	DNSDefaultResolverSelect *widget.Select
	DNSStrategySelect        *widget.Select
	RefreshDNSList           func()

	// RefreshSettingsFromModel пересобирает вкладку Settings из model.TemplateData.Vars (после LoadState / шаблона).
	RefreshSettingsFromModel func()

	// SettingsGates — индекс `переменная → строки`, подписанные на её
	// изменение (SPEC 107 §8.2). Заполняется при сборке вкладки Settings,
	// используется точечным пересчётом вместо полной пересборки.
	//
	// Тип — interface{}, чтобы presentation не зависел от пакета tabs
	// (реальный тип — *tabs.gateIndex); импорт в обратную сторону дал бы цикл.
	SettingsGates interface{}

	// RefreshTargetTabFromModel — полная пересборка вкладки Target (шаг 0).
	// Нужна, потому что состав вкладки зависит от таргета (поля платформы)
	// и от её собственных vars (gateway_mode → LAN-интерфейсы).
	RefreshTargetTabFromModel func()

	// UI-флаги состояния операций
	SaveInProgress          bool
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
