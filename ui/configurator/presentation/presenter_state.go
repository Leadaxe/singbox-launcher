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
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"singbox-launcher/core"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
	wizardbusiness "singbox-launcher/ui/configurator/business"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// HasUnsavedChanges проверяет наличие несохранённых изменений.
// hasChanges отслеживается как поле структуры WizardPresenter.
// Устанавливается в true через MarkAsChanged из табов и через SyncGUIToModel при расхождении виджетов с моделью (MergeGUIToModel флаг не трогает).
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

// CreateStateFromModel создает state.State из текущей модели.
//
// SPEC 052 phase 8: канонический список — model.Sources, model.GlobalOutbounds,
// model.Defaults. ParserConfig (legacy view) синхронизируется в core/state.Save
// через syncConnectionsFromLegacy, но здесь мы пишем напрямую в Connections —
// это короче и нет потери информации (Sources уже содержит все ID/Meta).
func (p *WizardPresenter) CreateStateFromModel(comment, id string) *wizardmodels.WizardStateFile {
	// Синхронизируем GUI с моделью перед созданием состояния
	p.SyncGUIToModel()

	// Создаём состояние с v5 layout: Connections — canonical, ParserConfig
	// — derived (заполняется через syncLegacyFromConnections при Load,
	// либо просто игнорируется на следующем сохранении).
	state := &wizardmodels.WizardStateFile{
		Version:   wizardmodels.WizardStateVersion,
		ID:        id,
		Comment:   comment,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	state.Connections.Sources = append([]wizardmodels.Source(nil), p.model.Sources...)
	if len(p.model.GlobalOutbounds) > 0 {
		state.Connections.Outbounds = append([]configtypes.OutboundConfig(nil), p.model.GlobalOutbounds...)
	} else {
		state.Connections.Outbounds = []configtypes.OutboundConfig{}
	}
	state.Connections.Defaults = p.model.Defaults

	// Заполняем legacy ParserConfig view ради совместимости тех тестов /
	// callsite'ов, что читают state.ParserConfig.ParserConfig.Proxies сразу
	// после CreateStateFromModel (без round-trip через диск).
	derivedPC := p.model.AsParserConfig()
	if derivedPC != nil {
		state.ParserConfig = *derivedPC
	}

	// Извлекаем config_params из модели
	state.ConfigParams = p.extractConfigParams()

	state.RulesLibraryMerged = p.model.RulesLibraryMerged
	state.SelectableRuleStates = nil

	// Преобразуем CustomRules — сохраняем полную структуру
	state.CustomRules = make([]wizardmodels.PersistedCustomRule, 0, len(p.model.CustomRules))
	for _, ruleState := range p.model.CustomRules {
		persisted := wizardmodels.ToPersistedCustomRule(ruleState)
		state.CustomRules = append(state.CustomRules, persisted)
	}

	// dns_options в state — только servers и rules; скаляры DNS — в state.vars (dns_*).
	state.DNSOptions = &wizardmodels.PersistedDNSState{
		Servers: append([]json.RawMessage(nil), p.model.DNSServers...),
		Rules:   wizardbusiness.PersistedDNSRulesForState(p.model.DNSRulesText),
	}

	if p.model.TemplateData != nil {
		for _, vd := range p.model.TemplateData.Vars {
			if vd.Separator {
				continue
			}
			if val, ok := p.model.SettingsVars[vd.Name]; ok {
				state.Vars = append(state.Vars, wizardmodels.PersistedSettingVar{Name: vd.Name, Value: val})
			}
		}
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

	// route.default_domain_resolver — в state.vars (dns_default_domain_resolver), не в config_params.

	return params
}

// SaveCurrentState сохраняет текущее состояние в state.json.
func (p *WizardPresenter) SaveCurrentState() error {
	debuglog.InfoLog("SaveCurrentState: called")
	// CreateStateFromModel вызывает SyncGUIToModel — не дублировать.
	state := p.CreateStateFromModel("", "")
	stateStore := p.GetStateStore()

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
	stateStore := p.GetStateStore()

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

	// SPEC 052 phase 8: Sources уже выставлены в restoreParserConfig (canonical).

	// Step 3: SourceURLs is only the input field for "Add"; source of truth for existing sources is ParserConfig.Proxies.
	// Keep it empty on load so the field is for adding new URLs only; existing sources are shown from Proxies.
	p.model.SourceURLs = ""

	wizardmodels.MigrateSettingsVarsFromConfigParams(stateFile)

	// Восстановление config_params и vars (шаг 4)
	p.restoreConfigParams(stateFile)
	wizardbusiness.MaterializeClashSecretIfNeeded(p.model)

	// Восстановление DNS вкладки (шаг 4b)
	p.restoreDNS(stateFile)

	hadRulesLibraryMerged := stateFile.RulesLibraryMerged
	wizardbusiness.ApplyRulesLibraryMigration(stateFile, p.model.TemplateData, p.model.ExecDir)
	p.model.RulesLibraryMerged = stateFile.RulesLibraryMerged
	p.model.SelectableRuleStates = nil
	p.restoreCustomRules(stateFile.CustomRules)
	wizardbusiness.EnsureCustomRulesDefaultOutbounds(p.model)

	// Установка флага для парсинга (шаг 7)
	p.model.PreviewNeedsParse = true

	// Синхронизация GUI (шаг 8)
	// SyncModelToGUI() также пересоздаст вкладку Rules, если она уже создана
	p.SyncModelToGUI()

	// Обновляем опции outbound для правил (включая селекторы)
	p.RefreshOutboundOptions()

	// SPEC 045 invariant: единственный writer state.json — Save визарда.
	// Миграция rules-library живёт в RAM; ApplyRulesLibraryMigration
	// идемпотентна — при следующем открытии без Save снова отработает
	// in-memory, на диск ничего не утечёт.
	//
	// Раньше тут стоял SaveWizardState под флагом !hadRulesLibraryMerged —
	// это и был баг утечки записи через "Get Free" (фрешный WizardStateFile
	// идёт с RulesLibraryMerged=false, миграция выставляет true, persist
	// затирал существующий state.json до того, как юзер успел нажать Save
	// или Cancel). После SPEC 045 миграция остаётся pure-data операцией.
	if !hadRulesLibraryMerged {
		p.MarkAsChanged()
	} else {
		p.MarkAsSaved()
	}

	return nil
}

// restoreParserConfig — SPEC 052 phase 8: переносит state.Connections в
// model.{Sources,GlobalOutbounds,Defaults} (canonical) и обновляет derived
// `ParserConfig`/`ParserConfigJSON` для UI/parser callsite'ов.
func (p *WizardPresenter) restoreParserConfig(stateFile *wizardmodels.WizardStateFile) error {
	// Sources canonical из v5 Connections.
	p.model.Sources = append([]wizardmodels.Source(nil), stateFile.Connections.Sources...)
	p.model.GlobalOutbounds = append([]configtypes.OutboundConfig(nil), stateFile.Connections.Outbounds...)
	p.model.Defaults = stateFile.Connections.Defaults

	// Validate: на свежей миграции должна быть хотя бы пустая slice.
	if p.model.Sources == nil {
		p.model.Sources = []wizardmodels.Source{}
	}
	if p.model.GlobalOutbounds == nil {
		p.model.GlobalOutbounds = []configtypes.OutboundConfig{}
	}

	// Refresh derived view (ParserConfig + ParserConfigJSON) для UI.
	p.model.RefreshDerivedParserConfig()
	wizardbusiness.InvalidatePreviewCache(p.model)
	return nil
}

// restoreCustomRules восстанавливает CustomRules из состояния (шаг 6).
func (p *WizardPresenter) restoreCustomRules(persistedRules []wizardmodels.PersistedCustomRule) {
	p.model.CustomRules = make([]*wizardmodels.RuleState, 0, len(persistedRules))
	for i := range persistedRules {
		ruleState := wizardmodels.PersistedCustomRuleToRuleState(&persistedRules[i])
		p.model.CustomRules = append(p.model.CustomRules, ruleState)
	}
}

// restoreConfigParams восстанавливает config_params и vars в модель.
func (p *WizardPresenter) restoreConfigParams(stateFile *wizardmodels.WizardStateFile) {
	configParams := stateFile.ConfigParams
	// Ищем route.final в параметрах
	finalOutbound := p.findConfigParamValue(configParams, "route.final")

	// Используем значение из параметров, если задано, иначе fallback на шаблон
	if finalOutbound != "" {
		p.model.SelectedFinalOutbound = finalOutbound
	} else {
		p.model.SelectedFinalOutbound = p.getDefaultFinalOutbound()
	}

	allowed := make(map[string]struct{})
	if p.model.TemplateData != nil {
		for _, vd := range p.model.TemplateData.Vars {
			if vd.Separator {
				continue
			}
			allowed[vd.Name] = struct{}{}
		}
	}
	p.model.SettingsVars = make(map[string]string)
	for _, x := range stateFile.Vars {
		if _, ok := allowed[x.Name]; !ok {
			continue // сироты: имя не из текущего шаблона (SPEC 032)
		}
		p.model.SettingsVars[x.Name] = x.Value // при дубликатах name в массиве JSON — последняя запись выигрывает
	}
}

// restoreDNS loads dns_options from state (if any) and merges with the current wizard_template.json.
func (p *WizardPresenter) restoreDNS(sf *wizardmodels.WizardStateFile) {
	if sf == nil {
		return
	}
	if p.model.TemplateData != nil {
		wizardbusiness.MigrateDNSScalarsFromPersistedToSettingsVars(sf.DNSOptions, p.model.SettingsVars, p.model.TemplateData.Vars)
	}
	if sf.DNSOptions != nil && sf.DNSOptions.ResolverUnset {
		p.model.DefaultDomainResolverUnset = true
		p.model.DefaultDomainResolver = ""
	}
	if sf.DNSOptions != nil {
		wizardbusiness.LoadPersistedWizardDNS(p.model, sf.DNSOptions)
	}
	// Старые state.json: тег только в config_params (до dns_* vars).
	if !p.model.DefaultDomainResolverUnset && strings.TrimSpace(p.model.DefaultDomainResolver) == "" {
		if dr := p.findConfigParamValue(sf.ConfigParams, "route.default_domain_resolver"); dr != "" {
			p.model.DefaultDomainResolver = dr
			p.model.DefaultDomainResolverUnset = false
		}
	}
	wizardbusiness.ApplyWizardDNSTemplate(p.model)
	wizardbusiness.ApplyDNSVarsFromSettingsToModel(p.model)
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
func (p *WizardPresenter) GetStateStore() *wizardbusiness.StateStore {
	ac := core.GetController()
	fileServiceAdapter := &wizardbusiness.FileServiceAdapter{FileService: ac.FileService}
	return wizardbusiness.NewStateStore(fileServiceAdapter)
}
