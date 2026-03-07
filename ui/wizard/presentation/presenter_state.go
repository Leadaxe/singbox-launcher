// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_state.go содержит методы для работы с сохранением и загрузкой состояний визарда:
//   - CreateStateFromModel - создает WizardStateFile из текущей модели
//   - SaveCurrentState - сохраняет текущее состояние в state.json
//   - SaveStateAs - сохраняет состояние под новым ID
//   - LoadState - загружает состояние в модель
//   - HasUnsavedChanges - проверяет наличие несохранённых изменений
//   - MarkAsChanged - устанавливает флаг изменений
//   - MarkAsSaved - сбрасывает флаг изменений
//
// Эти методы обеспечивают работу с состояниями визарда согласно спецификации:
//   - Сохранение состояния в state.json и именованные состояния
//   - Загрузка состояния из файла с восстановлением модели
//   - Отслеживание несохранённых изменений
//
// Используется в:
//   - wizard.go - при открытии визарда для проверки state.json
//   - dialogs/*.go - для сохранения/загрузки состояний через диалоги
package presentation

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"singbox-launcher/core"
	"singbox-launcher/core/services"
	"singbox-launcher/internal/debuglog"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardmodels "singbox-launcher/ui/wizard/models"
)

// HasUnsavedChanges проверяет наличие несохранённых изменений.
// hasChanges отслеживается как поле структуры WizardPresenter.
// Устанавливается в true при изменении данных через SyncGUIToModel (если данные реально изменились).
// Сбрасывается в false при сохранении состояния или загрузке нового состояния.
func (p *WizardPresenter) HasUnsavedChanges() bool {
	return p.hasChanges
}

// MarkAsChanged устанавливает флаг изменений.
func (p *WizardPresenter) MarkAsChanged() {
	p.hasChanges = true
	debuglog.DebugLog("MarkAsChanged: hasChanges set to true")
}

// MarkAsSaved сбрасывает флаг изменений.
func (p *WizardPresenter) MarkAsSaved() {
	p.hasChanges = false
	debuglog.DebugLog("MarkAsSaved: hasChanges reset to false")
}

// CreateStateFromModel создает WizardStateFile из текущей модели.
func (p *WizardPresenter) CreateStateFromModel(comment, id string) *wizardmodels.WizardStateFile {
	// Синхронизируем GUI с моделью перед созданием состояния
	p.SyncGUIToModel()

	// Создаём состояние
	state := &wizardmodels.WizardStateFile{
		Version:   wizardmodels.WizardStateVersion,
		ID:        id,
		Comment:   comment,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// Копируем ParserConfig
	if p.model.ParserConfig != nil {
		state.ParserConfig = *p.model.ParserConfig
	}

	// Извлекаем config_params из модели
	state.ConfigParams = p.extractConfigParams()

	// Преобразуем SelectableRuleStates — сохраняем только label, enabled, selected_outbound
	state.SelectableRuleStates = make([]wizardmodels.PersistedSelectableRuleState, 0, len(p.model.SelectableRuleStates))
	for _, ruleState := range p.model.SelectableRuleStates {
		persisted := wizardmodels.ToPersistedSelectableRuleState(ruleState)
		state.SelectableRuleStates = append(state.SelectableRuleStates, persisted)
	}

	// Преобразуем CustomRules — сохраняем полную структуру
	state.CustomRules = make([]wizardmodels.PersistedCustomRule, 0, len(p.model.CustomRules))
	for _, ruleState := range p.model.CustomRules {
		persisted := wizardmodels.ToPersistedCustomRule(ruleState)
		state.CustomRules = append(state.CustomRules, persisted)
	}

	return state
}

// extractConfigParams извлекает параметры конфигурации из модели.
func (p *WizardPresenter) extractConfigParams() []wizardmodels.ConfigParam {
	params := make([]wizardmodels.ConfigParam, 0)

	// Добавляем route.final
	if p.model.SelectedFinalOutbound != "" {
		params = append(params, wizardmodels.ConfigParam{
			Name:  "route.final",
			Value: p.model.SelectedFinalOutbound,
		})
	} else if p.model.TemplateData != nil && p.model.TemplateData.DefaultFinal != "" {
		// Используем значение по умолчанию из шаблона
		params = append(params, wizardmodels.ConfigParam{
			Name:  "route.final",
			Value: p.model.TemplateData.DefaultFinal,
		})
	}

	if runtime.GOOS == "darwin" {
		v := "false"
		if p.model.EnableTunForMacOS {
			v = "true"
		}
		params = append(params, wizardmodels.ConfigParam{Name: "enable_tun_macos", Value: v})
	}

	return params
}

// SaveCurrentState сохраняет текущее состояние в state.json.
func (p *WizardPresenter) SaveCurrentState() error {
	debuglog.InfoLog("SaveCurrentState: called")
	// Синхронизируем GUI в модель перед сохранением
	p.SyncGUIToModel()

	state := p.CreateStateFromModel("", "")
	stateStore := p.getStateStore()

	ac := core.GetController()
	// Получаем путь к state.json для логирования
	statesDir := filepath.Join(ac.FileService.ExecDir, "bin", wizardbusiness.WizardStatesDir)
	statePath := filepath.Join(statesDir, wizardmodels.StateFileName)

	debuglog.InfoLog("SaveCurrentState: saving to state.json at %s", statePath)
	if err := stateStore.SaveCurrentState(state); err != nil {
		debuglog.ErrorLog("SaveCurrentState: failed to save: %v", err)
		return fmt.Errorf("failed to save current state: %w", err)
	}

	p.MarkAsSaved()
	debuglog.InfoLog("SaveCurrentState: state.json saved successfully to %s", statePath)
	return nil
}

// SaveStateAs сохраняет состояние под новым ID с комментарием.
func (p *WizardPresenter) SaveStateAs(comment, id string) error {
	// Валидация ID
	if err := wizardmodels.ValidateStateID(id); err != nil {
		return fmt.Errorf("invalid state ID: %w", err)
	}

	state := p.CreateStateFromModel(comment, id)
	stateStore := p.getStateStore()

	if err := stateStore.SaveWizardState(state, id); err != nil {
		return fmt.Errorf("failed to save state: %w", err)
	}

	p.MarkAsSaved()
	debuglog.InfoLog("SaveStateAs: state saved successfully with ID: %s", id)
	return nil
}

// LoadState загружает состояние в модель согласно детальной последовательности восстановления.
// Выполняет 9-шаговую последовательность восстановления WizardModel согласно спецификации.
func (p *WizardPresenter) LoadState(stateFile *wizardmodels.WizardStateFile) error {
	if stateFile == nil {
		return fmt.Errorf("state file cannot be nil")
	}

	timing := debuglog.StartTiming("loadState")
	defer timing.EndWithDefer()

	// Валидация шаблона (шаг 1)
	if p.model.TemplateData == nil {
		return fmt.Errorf("template data not available")
	}

	// Восстановление parser_config (шаг 2)
	if err := p.restoreParserConfig(stateFile); err != nil {
		return err
	}

	// Step 3: SourceURLs is only the input field for "Add"; source of truth for existing sources is ParserConfig.Proxies.
	// Keep it empty on load so the field is for adding new URLs only; existing sources are shown from Proxies.
	p.model.SourceURLs = ""

	// Восстановление config_params (шаг 4)
	p.restoreConfigParams(stateFile.ConfigParams)

	// Восстановление SelectableRuleStates (шаг 5)
	p.restoreSelectableRuleStates(stateFile.SelectableRuleStates)

	// Восстановление CustomRules (шаг 6)
	p.restoreCustomRules(stateFile.CustomRules)

	// Установка флага для парсинга (шаг 7)
	p.model.PreviewNeedsParse = true

	// Синхронизация GUI (шаг 8)
	// SyncModelToGUI() также пересоздаст вкладку Rules, если она уже создана
	p.SyncModelToGUI()

	// Обновляем опции outbound для правил (включая селекторы)
	p.RefreshOutboundOptions()

	// Сбрасываем флаг изменений
	p.MarkAsSaved()

	return nil
}

// restoreParserConfig восстанавливает parser_config из состояния (шаг 2).
func (p *WizardPresenter) restoreParserConfig(stateFile *wizardmodels.WizardStateFile) error {
	if stateFile.ParserConfig.ParserConfig.Proxies == nil {
		return fmt.Errorf("invalid parser_config: Proxies is nil")
	}

	p.model.ParserConfig = &stateFile.ParserConfig
	wizardbusiness.InvalidatePreviewCache(p.model)

	// Сериализуем parser_config в JSON строку
	parserConfigJSON, err := wizardbusiness.SerializeParserConfig(&stateFile.ParserConfig)
	if err != nil {
		return fmt.Errorf("failed to serialize parser_config: %w", err)
	}
	p.model.ParserConfigJSON = parserConfigJSON

	return nil
}

// restoreSelectableRuleStates восстанавливает SelectableRuleStates из состояния (шаг 5).
// Сопоставляет сохранённые состояния с правилами из шаблона по label.
// Правила без совпадения в шаблоне игнорируются (могли быть удалены из шаблона).
// Правила из шаблона без сохранённого состояния получают значения по умолчанию.
func (p *WizardPresenter) restoreSelectableRuleStates(persistedRules []wizardmodels.PersistedSelectableRuleState) {
	debuglog.DebugLog("restoreSelectableRuleStates: restoring %d rules", len(persistedRules))

	// Создаём индекс сохранённых состояний по label
	savedByLabel := make(map[string]wizardmodels.PersistedSelectableRuleState)
	for _, pr := range persistedRules {
		savedByLabel[pr.Label] = pr
		debuglog.DebugLog("restoreSelectableRuleStates: saved rule label=%s, enabled=%v, selected_outbound=%s", pr.Label, pr.Enabled, pr.SelectedOutbound)
	}

	// Для каждого правила из шаблона ищем сохранённое состояние
	templateRules := p.model.TemplateData.SelectableRules
	p.model.SelectableRuleStates = make([]*wizardmodels.RuleState, 0, len(templateRules))
	for i := range templateRules {
		rule := &templateRules[i]
		rs := &wizardmodels.RuleState{
			Rule: *rule,
		}

		if saved, ok := savedByLabel[rule.Label]; ok {
			rs.Enabled = saved.Enabled
			rs.SelectedOutbound = saved.SelectedOutbound
			debuglog.DebugLog("restoreSelectableRuleStates: matched rule label=%s, enabled=%v, selected_outbound=%s", rule.Label, saved.Enabled, saved.SelectedOutbound)
		} else {
			rs.Enabled = rule.IsDefault
			rs.SelectedOutbound = rule.DefaultOutbound
		}

		if !services.AllSRSDownloaded(p.model.ExecDir, rule.RuleSets) {
			rs.Enabled = false
		}

		p.model.SelectableRuleStates = append(p.model.SelectableRuleStates, rs)
	}
}

// restoreCustomRules восстанавливает CustomRules из состояния (шаг 6).
func (p *WizardPresenter) restoreCustomRules(persistedRules []wizardmodels.PersistedCustomRule) {
	p.model.CustomRules = make([]*wizardmodels.RuleState, 0, len(persistedRules))
	for i := range persistedRules {
		ruleState := persistedRules[i].ToRuleState()
		p.model.CustomRules = append(p.model.CustomRules, ruleState)
	}
}

// extractSourceURLsFromParserConfig derives a single string from model (p.model.ParserConfig.Proxies).
// Combines Source and Connections from all ProxySource; for display/export only, not used to overwrite config.
func (p *WizardPresenter) extractSourceURLsFromParserConfig() string {
	if p.model.ParserConfig == nil || len(p.model.ParserConfig.ParserConfig.Proxies) == 0 {
		return ""
	}

	lines := make([]string, 0)
	for _, proxySource := range p.model.ParserConfig.ParserConfig.Proxies {
		if proxySource.Source != "" {
			lines = append(lines, proxySource.Source)
		}
		lines = append(lines, proxySource.Connections...)
	}

	return strings.Join(lines, "\n")
}

// restoreConfigParams восстанавливает config_params и маппинг в модель.
func (p *WizardPresenter) restoreConfigParams(configParams []wizardmodels.ConfigParam) {
	// Ищем route.final в параметрах
	finalOutbound := p.findConfigParamValue(configParams, "route.final")

	// Используем значение из параметров, если задано, иначе fallback на шаблон
	if finalOutbound != "" {
		p.model.SelectedFinalOutbound = finalOutbound
	} else {
		p.model.SelectedFinalOutbound = p.getDefaultFinalOutbound()
	}

	if v := p.findConfigParamValue(configParams, "enable_tun_macos"); v != "" {
		p.model.EnableTunForMacOS = v == "true"
	}
}

// findConfigParamValue ищет значение параметра по имени.
// Возвращает пустую строку, если параметр не найден.
func (p *WizardPresenter) findConfigParamValue(configParams []wizardmodels.ConfigParam, name string) string {
	for _, param := range configParams {
		if param.Name == name {
			return param.Value
		}
	}
	return ""
}

// getDefaultFinalOutbound возвращает значение по умолчанию для final outbound из шаблона.
func (p *WizardPresenter) getDefaultFinalOutbound() string {
	if p.model.TemplateData != nil && p.model.TemplateData.DefaultFinal != "" {
		return p.model.TemplateData.DefaultFinal
	}
	return ""
}

// GetStateStore создает новый StateStore для работы с состояниями.
// Публичный метод для использования в диалогах и других компонентах.
//
// Примечание: FileServiceAdapter определён в business/file_service_adapter.go (без build tag).
// но это не проблема, так как весь визард компилируется с cgo.
func (p *WizardPresenter) GetStateStore() *wizardbusiness.StateStore {
	// FileServiceAdapter в business/file_service_adapter.go (без cgo tag)
	ac := core.GetController()
	fileServiceAdapter := &wizardbusiness.FileServiceAdapter{FileService: ac.FileService}
	return wizardbusiness.NewStateStore(fileServiceAdapter)
}

// getStateStore - приватный алиас для внутреннего использования в презентере.
func (p *WizardPresenter) getStateStore() *wizardbusiness.StateStore {
	return p.GetStateStore()
}
