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
	"singbox-launcher/core/build"
	"singbox-launcher/core/config/configtypes"
	corev6 "singbox-launcher/core/state/v6"
	wizardtemplate "singbox-launcher/core/template"
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

	// SPEC 053: sync ВСЕХ правил с сохранением порядка RuleOrder.
	// state.RulesV6 эмитится в том же порядке как UI Rules tab показывает
	// (включая drag-reordering). Build pipeline затем эмитит fragments
	// в config.json::route.rules[] в этом же порядке.
	wizardmodels.ReconcileRuleOrder(p.model)
	state.RulesV6 = wizardmodels.SyncRulesByOrderToStateRulesV6(
		p.model.RuleOrder, p.model.PresetRefs, p.model.CustomRules,
	)

	// SPEC 056-R-N: full DNS sync → flat servers[]/rules[] через kind discriminator.
	// Template DNS tag-set извлекаем из template.dns_options для split'а
	// model.DNSServers на kind=template vs kind=user.
	templateDNSTags := extractTemplateDNSTags(p.model.TemplateData)
	state.DNS = wizardmodels.SyncDNSFullToStateV6(
		p.model.DNSServers,
		p.model.DNSRulesText,
		p.model.DNSTemplateOverrides,
		templateDNSTags,
	)
	// Lifecycle sync: ensure preset-entries в state.DNS соответствуют активным
	// preset-ref'ам в state.RulesV6. Idempotent — добавит missing entries и удалит
	// orphan'ы. Это **единственная** точка где kind=preset entries создаются/удаляются.
	if p.model.TemplateData != nil {
		presetMap := wizardtemplate.PresetLiteMap(p.model.TemplateData.Presets)
		corev6.SyncDNSOptionsWithActivePresets(state.RulesV6, &state.DNS, presetMap)
	}
	// SPEC 056-R-N follow-up: apply UI toggle overrides для kind=preset entries.
	// Sync создал entries с дефолтом Enabled=true; юзерский toggle живёт в
	// PresetRefState (DNSServerEnabled/DNSRuleEnabled).
	applyPresetEnabledOverrides(&state.DNS, p.model.PresetRefs)

	// SPEC 057-R-N: lifecycle sync для outbounds. Preset add entries (с Ref)
	// и mode=update patches (в Updates стеке) синхронизируются с active
	// preset-ref'ами. Idempotent.
	//
	// **Важно — Sync на BOTH viewах:** state.Save() вызывает
	// syncConnectionsFromLegacy (core/state/adapter.go), который копирует
	// state.ParserConfig.Outbounds → state.Connections.Outbounds. Если
	// Sync'нуть только Connections — адаптер затрёт изменения. Sync'аем оба
	// view'а (или хотя бы ParserConfig — тогда адаптер скопирует корректную
	// версию в Connections).
	if p.model.TemplateData != nil {
		build.SyncOutboundsWithActivePresets(state.RulesV6, &state.Connections.Outbounds, p.model.TemplateData.Presets)
		build.SyncOutboundsWithActivePresets(state.RulesV6, &state.ParserConfig.ParserConfig.Outbounds, p.model.TemplateData.Presets)
	}

	// dns_options в state — только servers и rules; скаляры DNS — в state.vars (dns_*).
	state.DNSOptions = &wizardmodels.PersistedDNSState{
		Servers: append([]json.RawMessage(nil), p.model.DNSServers...),
		Rules:   wizardbusiness.PersistedDNSRulesForState(p.model.DNSRulesText),
	}

	if p.model.TemplateData != nil {
		// Sync model.SelectedFinalOutbound → SettingsVars["route_final"] before
		// emitting state.Vars. This is the canonical channel for `route.final`
		// (template uses `"final": "@route_final"`); old config_params channel
		// stays for backward-compat read on legacy state.json files but is no
		// longer written.
		if p.model.SelectedFinalOutbound != "" {
			if p.model.SettingsVars == nil {
				p.model.SettingsVars = make(map[string]string)
			}
			p.model.SettingsVars["route_final"] = p.model.SelectedFinalOutbound
		}
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
//
// Сейчас config_params не используется ни для одного параметра в новых
// state.json: route.final мигрирован в state.vars["route_final"] (template
// делает substitution через @route_final), DNS-параметры — в state.vars
// тоже (dns_default_domain_resolver и т.п.).
//
// Поле остаётся в state schema для backward-compat чтения старых state.json
// (см. restoreConfigParams) и для возможных будущих параметров, которые не
// уложатся в template-vars модель.
func (p *WizardPresenter) extractConfigParams() []wizardmodels.ConfigParam {
	return []wizardmodels.ConfigParam{}
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
	// SPEC 053: restore preset-ref правила (kind=preset из state.RulesV6).
	p.restorePresetRefs(stateFile)

	// SPEC 057-R-N: sync preset binding в model.GlobalOutbounds после load.
	// Если state без ref/updates (legacy) — добавит entries для active preset'ов.
	// Idempotent: повторный вызов с уже-synced state — noop.
	if p.model.TemplateData != nil {
		build.SyncOutboundsWithActivePresets(stateFile.RulesV6, &p.model.GlobalOutbounds, p.model.TemplateData.Presets)
		p.model.RefreshDerivedParserConfig()
	}

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

	// Heal-on-empty: state.json мог быть сохранён (баг до v0.9.0.1) с пустыми
	// connections.outbounds — Rebuild сгенерирует config.json без proxy-out /
	// auto-proxy-out селекторов, sing-box упадёт FATAL "default outbound not
	// found: proxy-out". Если state пустой, заполняем из template (как делает
	// loader.go LoadConfigFromFile на cold-start). Не трогаем не-пустой
	// state — пользователь мог явно отредактировать список.
	if len(p.model.GlobalOutbounds) == 0 && p.model.TemplateData != nil && p.model.TemplateData.ParserConfig != "" {
		var parsed configtypes.ParserConfig
		if err := json.Unmarshal([]byte(p.model.TemplateData.ParserConfig), &parsed); err != nil {
			debuglog.WarnLog("restoreParserConfig: heal-empty: failed to parse template parser_config: %v", err)
		} else if len(parsed.ParserConfig.Outbounds) > 0 {
			p.model.GlobalOutbounds = append([]configtypes.OutboundConfig(nil), parsed.ParserConfig.Outbounds...)
			debuglog.InfoLog("restoreParserConfig: heal-empty: seeded %d global outbounds from template (state had empty connections.outbounds)", len(p.model.GlobalOutbounds))
		}
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

// restorePresetRefs (SPEC 053) — восстанавливает model.PresetRefs из state.RulesV6
// и заполняет model.RuleOrder в порядке state.RulesV6 (так чтобы UI Rules tab
// после load показал правила в том же порядке как при save).
//
// Только kind=preset entries попадают в PresetRefs; kind=inline/srs остаются
// в CustomRules через restoreCustomRules + legacy view (см. parseV6).
func (p *WizardPresenter) restorePresetRefs(state *wizardmodels.WizardStateFile) {
	p.model.PresetRefs = wizardmodels.SyncStateRulesToPresetRefs(state.RulesV6)
	p.model.DNSTemplateOverrides = wizardmodels.SyncStateV6ToDNSOverrides(state.DNS)
	// SPEC 056-R-N follow-up: per-server/rule preset enabled overrides → PresetRefState fields.
	populatePresetEnabledFromState(p.model.PresetRefs, state.DNS)

	// Restore RuleOrder из state.RulesV6 (preserve порядок between save/load).
	// Fallback на дефолтную последовательность если state v5 (нет RulesV6).
	order := wizardmodels.RuleOrderFromStateRulesV6(state.RulesV6, p.model.PresetRefs, p.model.CustomRules)
	if len(order) == 0 {
		wizardmodels.RebuildRuleOrder(p.model)
	} else {
		p.model.RuleOrder = order
		// Reconcile в случае если в model.CustomRules / PresetRefs есть entries
		// которые не попали в order (могут быть после миграции v5→v6).
		wizardmodels.ReconcileRuleOrder(p.model)
	}
}

// restoreConfigParams восстанавливает config_params и vars в модель.
//
// route.final ищется в порядке приоритета:
//  1. state.vars["route_final"] (canonical, новые state.json)
//  2. state.config_params["route.final"] (legacy, для миграции v0.9.3-)
//  3. template default (fallback, чистый старт)
//
// Это автоматически мигрирует старые state.json: при следующем Save запись
// уйдёт в state.vars, а config_params останется чистым.
func (p *WizardPresenter) restoreConfigParams(stateFile *wizardmodels.WizardStateFile) {
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

	// route.final: vars[route_final] → legacy config_params[route.final] → template default.
	if v, ok := p.model.SettingsVars["route_final"]; ok && v != "" {
		p.model.SelectedFinalOutbound = v
	} else if legacy := p.findConfigParamValue(stateFile.ConfigParams, "route.final"); legacy != "" {
		p.model.SelectedFinalOutbound = legacy
		// Eager-write into vars so the next save persists in canonical channel
		// and the legacy config_params entry can be dropped.
		p.model.SettingsVars["route_final"] = legacy
	} else {
		p.model.SelectedFinalOutbound = p.getDefaultFinalOutbound()
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
