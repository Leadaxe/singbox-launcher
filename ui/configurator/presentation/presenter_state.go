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
	"time"

	"singbox-launcher/core"
	"singbox-launcher/core/build"
	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
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

	// SPEC 070 ADR-070-2: legacy state.CustomRules write удалён — поле снято
	// с core/state.State. marshalDisk сериализует только canonical state.Rules
	// (заполняется ниже через SyncRulesByOrderToStateRulesV6), а прежний
	// CustomRules-write был dead-on-save (не сериализовался) и dead-in-memory
	// (state отбрасывается после Save). UI legacy view берётся on-demand через
	// state.LegacyCustomRulesView при restore.

	// SPEC 053: sync ВСЕХ правил с сохранением порядка RuleOrder.
	// state.Rules эмитится в том же порядке как UI Rules tab показывает
	// (включая drag-reordering). Build pipeline затем эмитит fragments
	// в config.json::route.rules[] в этом же порядке.
	wizardmodels.ReconcileRuleOrder(p.model)
	state.Rules = wizardmodels.SyncRulesByOrderToStateRulesV6(
		p.model.RuleOrder, p.model.PresetRefs, p.model.CustomRules,
	)

	// SPEC 056-R-N: full DNS sync → flat servers[]/rules[] через kind discriminator.
	// Template DNS tag-set извлекаем из template.dns_options для split'а
	// model.DNSServers на kind=template vs kind=user.
	//
	// SPEC 062-F-N: rules portion теперь order-aware через model.DNSRuleOrder.
	// Reconcile сначала добавит slots для свежесозданных preset-ref'ов / user
	// rules (например preset включён через Rules tab → нужен slot для его
	// dns_rule). Затем SyncDNSByOrderToState обойдёт DNSRuleOrder и эмитит
	// rules в правильном порядке. Если DNSRuleOrder пуст (legacy state) —
	// fallback на DNSRulesText (через buildDNSRulesFromText внутри).
	templateDNSTags := wizardbusiness.ExtractTemplateDNSTags(p.model.TemplateData)
	wizardmodels.ReconcileDNSRuleOrder(p.model)
	state.DNS = wizardmodels.SyncDNSByOrderToState(
		p.model.DNSRuleOrder,
		p.model.PresetRefs,
		p.model.DNSUserRules,
		p.model.DNSServers,
		p.model.DNSRulesText,
		p.model.DNSTemplateOverrides,
		templateDNSTags,
	)
	// Lifecycle sync: ensure preset-entries в state.DNS соответствуют активным
	// preset-ref'ам в state.Rules. Idempotent — добавит missing entries и удалит
	// orphan'ы. Это **единственная** точка где kind=preset entries создаются/удаляются.
	if p.model.TemplateData != nil {
		presetMap := wizardtemplate.PresetLiteMap(p.model.TemplateData.Presets)
		corestate.SyncDNSOptionsWithActivePresets(state.Rules, &state.DNS, presetMap)
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
		build.SyncOutboundsWithActivePresets(state.Rules, &state.Connections.Outbounds, p.model.TemplateData.Presets)
		build.SyncOutboundsWithActivePresets(state.Rules, &state.ParserConfig.ParserConfig.Outbounds, p.model.TemplateData.Presets)
	}

	// SPEC 070 ADR-070-2: legacy state.DNSOptions write удалён — поле снято
	// с core/state.State. Canonical state.DNS (servers/rules) уже синхронизирован
	// выше через SyncDNSByOrderToState; DNS-scalars живут в state.vars (dns_*).
	// Прежний DNSOptions-write был dead-on-save (не сериализовался) и
	// dead-in-memory (state отбрасывается после Save).

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
	wizardbusiness.MaterializeSecretsIfNeeded(p.model)

	// Восстановление DNS вкладки (шаг 4b)
	p.restoreDNS(stateFile)

	// SPEC 053 removed the legacy template.selectable_rules library — rules now
	// live exclusively in state.rules[] (kind=preset/inline/srs). RulesLibraryMerged
	// + SelectableRuleStates are kept on the v4 disk struct for read-compat but
	// have no runtime effect; just zero the in-memory copies so nothing reads stale.
	p.model.RulesLibraryMerged = true
	p.model.SelectableRuleStates = nil
	// SPEC 070 ADR-070-2: legacy CustomRules больше не stored field — берём
	// on-demand projection из canonical stateFile.Rules.
	p.restoreCustomRules(corestate.LegacyCustomRulesView(stateFile))
	// Fill SelectedOutbound for any custom rules missing it (single-pass after restore).
	{
		opts := wizardbusiness.EnsureDefaultAvailableOutbounds(wizardbusiness.GetAvailableOutbounds(p.model))
		for _, rs := range p.model.CustomRules {
			wizardmodels.EnsureDefaultOutbound(rs, opts)
		}
	}
	// SPEC 053: restore preset-ref правила (kind=preset из state.Rules).
	p.restorePresetRefs(stateFile)

	// SPEC 058-R-N: migration direct→referenced shape. Legacy state.json (SPEC 057
	// и раньше) хранил template/preset-derived entries с full body inline; новый
	// shape — thin tag+ref. Migration однопроходная, lossless (Backup .pre-058.bak
	// создаётся на следующем Save). Idempotent.
	//
	// SPEC 057-R-N: sync preset binding после migration. Sync приведёт slice в
	// правильный referenced shape (drop stale, add missing, reorder updates).
	// Idempotent.
	if p.model.TemplateData != nil {
		// MigrateOutboundsToReferencedShape возвращает true если конвертировал
		// хоть один entry. Backup gate в Save проверяет outbounds.Ref напрямую,
		// флаг здесь не нужен. Rules нужны migration'у для computing merged_base
		// = template + active preset patches (чтобы USER patch не over-include
		// preset edits которые УЖЕ были materialized в legacy body).
		_ = build.MigrateOutboundsToReferencedShape(&p.model.GlobalOutbounds, stateFile.Rules, p.model.TemplateData)
		build.SyncOutboundsWithActivePresets(stateFile.Rules, &p.model.GlobalOutbounds, p.model.TemplateData.Presets)
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
	// На clean load дирти-флаг не нужен (раньше он зависел от
	// !hadRulesLibraryMerged — сигнал, что миграция SelectableRules сменила
	// shape; SPEC 053 убрал эту библиотеку, миграция мертва).
	p.MarkAsSaved()

	return nil
}
