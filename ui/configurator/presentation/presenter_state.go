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
	"time"

	"singbox-launcher/core"
	"singbox-launcher/core/build"
	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	"singbox-launcher/internal/constants"
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
//
// SPEC 115 §2: он же — сигнал инвалидации отчёта сборки. Отчёт описывает
// КОНКРЕТНУЮ конфигурацию, и после правки описывает уже не ту: пользователь
// чинит источник и смотрит, ушла ли пометка ⚠, а устаревший отчёт отвечает на
// этот вопрос неправильно. Заодно закрывается кнопка Save на вкладке «Итог» —
// сохранять то, чего не собирали, нельзя.
//
// Вешается именно сюда, а не на каждую правку по отдельности: MarkAsChanged —
// единственный сигнал «модель изменилась», через который проходят ВСЕ правки
// Мастера (источники, правила, DNS, настройки). Своя инвалидация у каждой
// формы разъехалась бы с первой же новой формой.
func (p *WizardPresenter) MarkAsChanged() {
	p.hasChanges = true
	config.ResetBuildReport()
	debuglog.DebugLog("MarkAsChanged: hasChanges set to true")
}

// MarkAsSaved сбрасывает флаг изменений.
func (p *WizardPresenter) MarkAsSaved() {
	p.hasChanges = false
	debuglog.DebugLog("MarkAsSaved: hasChanges reset to false")
}

// CreateStateFromModel создает state.State из текущей модели.
//
// SPEC 117 (W4): пишет ТОЛЬКО canonical — Connections (model.Sources,
// model.GlobalOutbounds, model.Defaults) + остальные секции. Legacy-проекция
// state.ParserConfig здесь не заполняется: она наполняется исключительно на
// Load (syncLegacyFromConnections), а Save её не читает.
func (p *WizardPresenter) CreateStateFromModel(comment, id string) *wizardmodels.WizardStateFile {
	// Синхронизируем GUI с моделью перед созданием состояния
	p.SyncGUIToModel()
	state := &wizardmodels.WizardStateFile{
		Version:   wizardmodels.WizardStateVersion,
		ID:        id,
		Comment:   comment,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	// SPEC 097: таргет — часть состояния, а не UI-момент. Без него визард
	// после перезапуска собрал бы remote-состоянию local-конфиг.
	tgt := p.model.Target.Normalized()
	if tgt.IsRemote() {
		state.Target = constants.ConfigTargetRemote
		state.TargetPlatform = tgt.GOOS
		state.TargetArch = tgt.GOARCH
	}
	state.Sources = append([]wizardmodels.Source(nil), p.model.Sources...)
	if len(p.model.GlobalOutbounds) > 0 {
		state.Directions = append([]configtypes.Direction(nil), p.model.GlobalOutbounds...)
	} else {
		state.Directions = []configtypes.Direction{}
	}
	state.Defaults = p.model.Defaults
	state.WarpAccounts = p.model.WarpAccounts

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
	// state.Rules эмитится в том же порядке как UI Rules tab показывает
	// (включая drag-reordering). Build pipeline затем эмитит fragments
	// в config.json::route.rules[] в этом же порядке.
	wizardmodels.ReconcileRuleOrder(p.model)
	state.Rules = wizardmodels.EmitStateRulesInAxisOrder(
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
	// SPEC 117: Sync ровно один — по canonical Connections.Outbounds.
	// Обратный синк Save удалён (W4): Save сериализует Connections как есть,
	// выравнивать проекцию больше не нужно.
	if p.model.TemplateData != nil {
		build.SyncOutboundsWithTemplate(state.Rules, &state.Directions, p.model.TemplateData.Presets, build.TemplateOutboundTags(p.model.TemplateData), p.model.Target)
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
		// Materialize DNS scalars (dns_strategy / dns_final / dns_default_domain_resolver)
		// from the model into SettingsVars before emitting state.Vars, so Save persists
		// them regardless of whether a prior GUI→model DNS sync ran. Without this, a
		// resolver/strategy/final change made right before Save (e.g. no tab switch) was
		// silently dropped, while server enable/disable persisted (that goes straight to
		// state.DNS.servers, not via a var).
		wizardbusiness.SyncDNSModelToSettingsVars(p.model)
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

	// SPEC 115 §2: загрузка состояния — самая крупная правка модели, какая
	// бывает, но через MarkAsChanged она не проходит (загруженное состояние
	// не считается несохранённым изменением). Без явного сброса отчёт
	// прошлой конфигурации раскрасил бы источники ЧУЖОГО состояния.
	config.ResetBuildReport()

	// Валидация шаблона (шаг 1)
	if p.model.TemplateData == nil {
		return fmt.Errorf("template data not available")
	}

	// SPEC 097: таргет восстанавливаем ПЕРВЫМ — от него зависят per-platform
	// дефолты vars, которые резолвятся ниже по этой же функции. Legacy-файл
	// без meta.target даёт zero TargetSpec → нормализуется в local.
	// Прежний таргет передаётся в восстановление: MachineID и каталоги машины
	// в файл не пишутся (они свойство записи реестра), и без переноса первый
	// же Load увёл бы Save в singleton-папку вместо профиля машины.
	p.model.Target = targetSpecFromStateMeta(stateFile, p.model.Target)

	// Восстановление parser_config (шаг 2)
	if err := p.restoreParserConfig(stateFile); err != nil {
		return err
	}

	// SPEC 052 phase 8: Sources уже выставлены в restoreParserConfig (canonical).

	// Step 3: SourceURLs is only the input field for "Add"; source of truth for existing sources is ParserConfig.Proxies.
	// Keep it empty on load so the field is for adding new URLs only; existing sources are shown from Proxies.
	p.model.SourceURLs = ""

	wizardmodels.MigrateSettingsVarsFromConfigParams(stateFile)
	// Многострочный tun_address (v4+v6 в одном поле) → два однострочных
	// поля. До restoreConfigParams: оно читает уже разведённые значения.
	wizardmodels.MigrateTunAddressSplit(stateFile)

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

	p.restoreCustomRules(stateFile.CustomRules)
	// Fill SelectedOutbound for any custom rules missing it (single-pass after restore).
	{
		opts := wizardbusiness.EnsureDefaultAvailableOutbounds(wizardbusiness.GetAvailableOutbounds(p.model))
		for _, rs := range p.model.CustomRules {
			wizardmodels.EnsureDefaultOutbound(rs, opts)
		}
	}
	// SPEC 053: restore preset-ref правила (kind=preset из state.Rules).
	p.restorePresetRefs(stateFile)

	// SPEC 108 (S5): цель, которой нет среди допустимых, — на direct.
	// Строго ПОСЛЕ restorePresetRefs: пресетные теги входят в множество
	// целей, и без них сброс убил бы живые правила.
	p.resetForeignRuleTargets()

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
		_ = build.MigrateOutboundsToReferencedShape(&p.model.GlobalOutbounds, stateFile.Rules, p.model.TemplateData, p.model.Target)
		build.SyncOutboundsWithTemplate(stateFile.Rules, &p.model.GlobalOutbounds, p.model.TemplateData.Presets, build.TemplateOutboundTags(p.model.TemplateData), p.model.Target)
		p.model.BumpRevision()
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
