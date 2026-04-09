// Package presentation содержит слой представления визарда конфигурации.
//
// Файл presenter_sync.go содержит методы синхронизации данных между моделью и GUI:
//   - SyncModelToGUI / SyncModelToGUIInitial — модель → виджеты; Initial без пересоздания вкладки Rules
//   - SyncGUIToModel / MergeGUIToModel — GUI → модель (первый с MarkAsChanged при отличиях, второй только слияние)
//
// Эти методы обеспечивают двустороннюю синхронизацию между WizardModel и GUIState,
// что является ключевой частью архитектуры MVP.
//
// Синхронизация данных - это отдельная ответственность от других методов презентера.
// Методы синхронизации используются в разных местах (перед сохранением, при инициализации).
//
// Используется в:
//   - wizard.go — SyncModelToGUIInitial при первом открытии; SyncModelToGUI после LoadState / Read
//   - presenter_save.go — SyncGUIToModel в начале SaveConfig
//   - presenter_state.go — SyncGUIToModel в CreateStateFromModel перед сборкой state
//   - presenter_async.go, wizard.go, tabs/source_tab.go — MergeGUIToModel (смена вкладок, закрытие, парсинг preview без hasChanges)
package presentation

/*
Контракт синхронизации и флага «несохранённые изменения» (hasChanges)

  • Model → GUI: SyncModelToGUI → applyWizardWidgetsFromModel (через fyne.Do).
    В конце: WizardWidgetsReady = true и MarkAsSaved() — только после всех SetText/SetSelected,
    включая Final outbound. Иначе поздний SetSelected снова вызовет OnChanged и ложный hasChanges.

  • GUI → Model, два режима:
    – SyncGUIToModel()        — слить виджеты в p.model; при любом расхождении MarkAsChanged().
    – MergeGUIToModel()      — то же слияние без MarkAsChanged (смена вкладок, закрытие, parse preview).
    Пока WizardWidgetsReady == false, MergeGUIToModel сразу выходит (не трогаем модель при первом кадре).
    SyncGUIToModel при !WizardWidgetsReady всё же выполняется (сохранение не должно блокироваться) — с теми же
    по-полевым ветками «keep model», что и после ready.

  • Подавление ложных срабатываний при программной записи в виджеты:
    ParserConfigUpdating, DNSSelectsProgrammatic, UpdatingOutboundOptions,
    SourceURLsProgrammatic, DNSRulesProgrammatic — OnChanged/селекты не должны звать MarkAsChanged.

  • Поле ParserConfig (multi-line JSON): OnChanged вызывает MergeGUIToModel на каждый символ — намеренно
    (актуальный ParserConfigJSON и hasChanges для Save/табов). Тяжёлая работа вынесена в debounce
    RefreshOutboundOptions и мемо GetAvailableOutbounds по JSON (см. presenter_methods, business/outbound).

  • Пустой текст/выбор у Select до отрисовки: в syncGUIToModel специальные ветки «keep model»,
    см. dnsSelectReadLooksStale / dnsSelectOptionsMissingModelTag; Entry / strategy / Final outbound —
    internal/wizardsync (GuiTextAwaitingProgrammaticFill, FinalOutboundSelectReadLooksStale), юнит-тесты без Fyne.
*/
import (
	"encoding/json"
	"fmt"
	"strings"

	"fyne.io/fyne/v2/dialog"

	"singbox-launcher/core/config"
	"singbox-launcher/internal/locale"
	"singbox-launcher/internal/wizardsync"
	wizardbusiness "singbox-launcher/ui/wizard/business"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// SyncModelToGUI синхронизирует данные из модели в GUI.
// SourceURLEntry shows SourceURLs (input for Add only); source list is from ParserConfig.Proxies in refreshSourcesList.
//
// Всегда через fyne.Do: прямой вызов SetText/Refresh с потока, где создали окно, даёт зависания/краши при открытии визарда.
// Если сразу после SyncModelToGUI вызывается SyncGUIToModel (Save), DNS-виджеты могут быть ещё пустыми — см. защиту в SyncGUIToModel.
func (p *WizardPresenter) SyncModelToGUI() {
	p.syncModelToGUI(true)
}

// SyncModelToGUIInitial — первая синхронизация при открытии визарда: вкладка Rules уже
// построена в createWizardTabs; повторное RefreshRulesTabAfterLoadState даёт второй CreateRulesTab,
// лишний асинхронный RefreshOutboundOptions и ложный MarkAsChanged при закрытии без правок.
func (p *WizardPresenter) SyncModelToGUIInitial() {
	p.syncModelToGUI(false)
}

func (p *WizardPresenter) syncModelToGUI(recreateRulesTab bool) {
	p.UpdateUI(p.applyWizardWidgetsFromModel)

	if !recreateRulesTab {
		return
	}
	rulesTitle := locale.T("wizard.tab_rules")
	if p.createRulesTabFunc != nil && p.guiState.Tabs != nil {
		for _, tabItem := range p.guiState.Tabs.Items {
			if tabItem.Text == rulesTitle {
				p.RefreshRulesTabAfterLoadState()
				break
			}
		}
	}
}

// applyWizardWidgetsFromModel переносит p.model в виджеты. Порядок важен: MarkAsSaved и
// WizardWidgetsReady = true только в самом конце, после обновления Final outbound.
func (p *WizardPresenter) applyWizardWidgetsFromModel() {
	if p.guiState == nil {
		return
	}
	p.guiState.WizardWidgetsReady = false
	if p.guiState.SourceURLEntry != nil {
		p.guiState.SourceURLsProgrammatic = true
		p.guiState.SourceURLEntry.SetText(p.model.SourceURLs)
		p.guiState.SourceURLsProgrammatic = false
	}
	if p.guiState.ParserConfigEntry != nil {
		p.guiState.ParserConfigUpdating = true
		p.guiState.ParserConfigEntry.SetText(p.model.ParserConfigJSON)
		p.guiState.ParserConfigUpdating = false
		p.guiState.LastValidParserConfigJSON = p.model.ParserConfigJSON
	}
	if p.guiState.RefreshSourcesList != nil {
		p.guiState.RefreshSourcesList()
	}
	if p.guiState.RefreshOutboundsConfiguratorList != nil {
		p.guiState.RefreshOutboundsConfiguratorList()
	}
	if p.guiState.RefreshDNSList != nil {
		p.guiState.RefreshDNSList()
	}
	if p.guiState.DNSRulesEntry != nil {
		p.guiState.DNSRulesProgrammatic = true
		p.guiState.DNSRulesEntry.SetText(p.model.DNSRulesText)
		p.guiState.DNSRulesProgrammatic = false
	}
	p.refreshDNSSelectsFromModel()
	if p.guiState.FinalOutboundSelect != nil {
		p.guiState.UpdatingOutboundOptions = true
		options := wizardbusiness.EnsureDefaultAvailableOutbounds(wizardbusiness.GetAvailableOutbounds(p.model))
		p.guiState.FinalOutboundSelect.Options = options
		p.guiState.FinalOutboundSelect.SetSelected(p.model.SelectedFinalOutbound)
		p.guiState.FinalOutboundSelect.Refresh()
		p.guiState.UpdatingOutboundOptions = false
	}

	if p.guiState.RefreshSettingsFromModel != nil {
		p.guiState.RefreshSettingsFromModel()
	}

	p.guiState.WizardWidgetsReady = true
	p.MarkAsSaved()
}

func (p *WizardPresenter) refreshDNSSelectsFromModel() {
	if p.guiState != nil {
		p.guiState.DNSSelectsProgrammatic = true
		defer func() { p.guiState.DNSSelectsProgrammatic = false }()
	}
	tags := wizardbusiness.DNSEnabledTagOptions(p.model)
	if p.guiState.DNSFinalSelect != nil {
		p.guiState.DNSFinalSelect.Options = tags
		sel := p.model.DNSFinal
		if sel != "" && !stringSliceContains(tags, sel) && len(tags) > 0 {
			sel = tags[0]
			p.model.DNSFinal = sel
		}
		if sel != "" {
			p.guiState.DNSFinalSelect.SetSelected(sel)
		} else if len(tags) > 0 {
			p.guiState.DNSFinalSelect.SetSelected(tags[0])
			p.model.DNSFinal = tags[0]
		}
		p.guiState.DNSFinalSelect.Refresh()
	}
	if p.guiState.DNSDefaultResolverSelect != nil {
		opts := append([]string(nil), tags...)
		p.guiState.DNSDefaultResolverSelect.Options = opts
		sel := strings.TrimSpace(p.model.DefaultDomainResolver)
		if !stringSliceContains(tags, sel) {
			if len(tags) > 0 {
				sel = tags[0]
				p.model.DefaultDomainResolver = sel
				p.model.DefaultDomainResolverUnset = false
			} else {
				sel = ""
			}
		}
		if sel != "" {
			p.guiState.DNSDefaultResolverSelect.SetSelected(sel)
		}
		p.guiState.DNSDefaultResolverSelect.Refresh()
	}
	if p.guiState.DNSStrategySelect != nil {
		strOpts := []string{"ipv4_only", "ipv6_only", "prefer_ipv4", "prefer_ipv6"}
		if vd, ok := wizardtemplate.VarByName(p.model.TemplateData.Vars, wizardmodels.VarDNSStrategy); ok && strings.EqualFold(strings.TrimSpace(vd.Type), "enum") && len(vd.Options) > 0 {
			strOpts = append([]string(nil), vd.Options...)
		}
		p.guiState.DNSStrategySelect.Options = strOpts
		sel := strings.TrimSpace(p.model.DNSStrategy)
		if sel == "" && p.model.TemplateData != nil {
			sel = strings.TrimSpace(wizardtemplate.DisplaySettingValue(
				p.model.TemplateData.Vars,
				p.model.SettingsVars,
				p.model.TemplateData.RawTemplate,
				wizardmodels.VarDNSStrategy,
			))
		}
		if len(strOpts) > 0 && !stringSliceContains(strOpts, sel) {
			sel = strOpts[0]
		}
		if sel != "" {
			p.guiState.DNSStrategySelect.SetSelected(sel)
			p.model.DNSStrategy = sel
		}
		p.guiState.DNSStrategySelect.Refresh()
	}
	if p.guiState.DNSIndependentCacheCheck != nil {
		v := false
		if p.model.DNSIndependentCache != nil {
			v = *p.model.DNSIndependentCache
		}
		p.guiState.DNSIndependentCacheCheck.SetChecked(v)
	}
	wizardbusiness.SyncDNSModelToSettingsVars(p.model)
}

// RefreshDNSDependentSelectsOnly обновляет только селекты Final / resolver / strategy и галочку кэша,
// без пересборки списка строк серверов. Нужен после смены enabled у сервера: полный SyncModelToGUI
// на каждый клик по галочке пересоздаёт все строки и даёт сильные лаги и «зависание» вкладки.
func (p *WizardPresenter) RefreshDNSDependentSelectsOnly() {
	p.UpdateUI(func() {
		if p.guiState == nil {
			return
		}
		p.refreshDNSSelectsFromModel()
	})
}

// RefreshDNSListAndSelects пересобирает список серверов и DNS-селекты (после Add / Edit / Delete).
func (p *WizardPresenter) RefreshDNSListAndSelects() {
	p.UpdateUI(func() {
		if p.guiState == nil {
			return
		}
		if p.guiState.RefreshDNSList != nil {
			p.guiState.RefreshDNSList()
		}
		p.refreshDNSSelectsFromModel()
	})
}

// dnsSelectReadLooksStale: SyncGUIToModel до отработки fyne.Do — Select ещё без выбора, модель уже с тегом.
func dnsSelectReadLooksStale(widgetSelected, modelTag string, model *wizardmodels.WizardModel) bool {
	if strings.TrimSpace(widgetSelected) != "" {
		return false
	}
	mt := strings.TrimSpace(modelTag)
	if mt == "" {
		return false
	}
	return stringSliceContains(wizardbusiness.DNSEnabledTagOptions(model), mt)
}

func stringSliceContains(slice []string, s string) bool {
	for _, x := range slice {
		if x == s {
			return true
		}
	}
	return false
}

// dnsSelectOptionsMissingModelTag: выпадающий список ещё не обновлён под модель (async SyncModelToGUI).
func dnsSelectOptionsMissingModelTag(opts []string, modelTag string) bool {
	mt := strings.TrimSpace(modelTag)
	if mt == "" || len(opts) == 0 {
		return false
	}
	return !stringSliceContains(opts, mt)
}

// SyncGUIToModel переносит значения виджетов в p.model. При любом отличии от прежнего содержимого модели вызывает MarkAsChanged.
// Используйте перед сохранением state и там, где нужно зафиксировать правку пользователя.
func (p *WizardPresenter) SyncGUIToModel() {
	p.syncGUIToModel(true)
}

// MergeGUIToModel переносит значения виджетов в p.model без изменения hasChanges.
// Нужно при смене вкладок, перед проверкой «закрыть визард?», перед фоновым parse — чтобы модель была актуальной,
// но служебные расхождения виджет/модель не помечались как несохранённые.
func (p *WizardPresenter) MergeGUIToModel() {
	p.syncGUIToModel(false)
}

// MergeGUIToModelFromMainThread выполняет слияние GUI→модель без MarkAsChanged в потоке Fyne и ждёт завершения.
// Нужна из горутины Save после ensureOutboundsParsed: за время ожидания/парсинга пользователь мог править поля.
func (p *WizardPresenter) MergeGUIToModelFromMainThread() {
	if p.guiState == nil || p.guiState.Window == nil {
		p.MergeGUIToModel()
		return
	}
	done := make(chan struct{})
	p.UpdateUI(func() {
		p.syncGUIToModel(false)
		close(done)
	})
	<-done
}

func (p *WizardPresenter) syncGUIToModel(markDirty bool) {
	if p.guiState == nil {
		return
	}
	// До первого полного applyWizardWidgetsFromModel виджеты пустые: MergeGUIToModel не трогает модель
	// (переключение табов до отрисовки не должно затирать state и не вызывает MarkAsChanged).
	if !p.guiState.WizardWidgetsReady && !markDirty {
		return
	}
	ready := p.guiState.WizardWidgetsReady
	changed := p.syncGUIToModelSourceParserFinal(ready) || p.syncGUIToModelDNS(ready)
	if ready {
		wizardbusiness.SyncDNSModelToSettingsVars(p.model)
	}

	if changed && markDirty {
		p.MarkAsChanged()
	}
}

// syncGUIToModelSourceParserFinal переносит поля Sources / Parser / Final outbound (риск: затереть модель
// пустыми виджетами до конца applyWizardWidgetsFromModel или до SetSelected у Select).
func (p *WizardPresenter) syncGUIToModelSourceParserFinal(ready bool) bool {
	gs := p.guiState
	var changed bool

	if gs.SourceURLEntry != nil {
		newValue := gs.SourceURLEntry.Text
		if wizardsync.GuiTextAwaitingProgrammaticFill(ready, newValue, p.model.SourceURLs) {
			// ждём SetText из SyncModelToGUI
		} else if p.model.SourceURLs != newValue {
			p.model.SourceURLs = newValue
			changed = true
		}
	}
	if gs.ParserConfigEntry != nil {
		newValue := gs.ParserConfigEntry.Text
		if wizardsync.GuiTextAwaitingProgrammaticFill(ready, newValue, p.model.ParserConfigJSON) {
			// ждём SetText из SyncModelToGUI
		} else if p.model.ParserConfigJSON != newValue {
			p.model.ParserConfigJSON = newValue
			changed = true
		}
	}
	if gs.FinalOutboundSelect != nil {
		newValue := gs.FinalOutboundSelect.Selected
		opts := gs.FinalOutboundSelect.Options
		if wizardsync.FinalOutboundSelectReadLooksStale(ready, newValue, p.model.SelectedFinalOutbound, opts) {
			// keep model
		} else if p.model.SelectedFinalOutbound != newValue {
			p.model.SelectedFinalOutbound = newValue
			changed = true
		}
	}
	return changed
}

// syncGUIToModelDNS переносит вкладку DNS (риск: те же гонки fyne + рассинхрон Options селектов с моделью).
func (p *WizardPresenter) syncGUIToModelDNS(ready bool) bool {
	gs := p.guiState
	var changed bool

	if gs.DNSRulesEntry != nil {
		newValue := gs.DNSRulesEntry.Text
		if wizardsync.GuiTextAwaitingProgrammaticFill(ready, newValue, p.model.DNSRulesText) {
			// keep model
		} else if p.model.DNSRulesText != newValue {
			p.model.DNSRulesText = newValue
			changed = true
		}
	}
	if gs.DNSFinalSelect != nil {
		newValue := gs.DNSFinalSelect.Selected
		opts := gs.DNSFinalSelect.Options
		mf := strings.TrimSpace(p.model.DNSFinal)
		if dnsSelectOptionsMissingModelTag(opts, mf) {
			// keep model — список тегов ещё не из той же модели
		} else if dnsSelectReadLooksStale(newValue, mf, p.model) {
			// keep model
		} else if p.model.DNSFinal != newValue {
			p.model.DNSFinal = newValue
			changed = true
		}
	}
	if gs.DNSDefaultResolverSelect != nil {
		sel := gs.DNSDefaultResolverSelect.Selected
		opts := gs.DNSDefaultResolverSelect.Options
		mt := strings.TrimSpace(p.model.DefaultDomainResolver)
		if dnsSelectOptionsMissingModelTag(opts, mt) {
			// keep model — резолвер из state/шаблона ещё не попал в Options
		} else if sel != "" {
			if dnsSelectReadLooksStale(sel, mt, p.model) {
				// keep model
			} else if p.model.DefaultDomainResolver != sel || p.model.DefaultDomainResolverUnset {
				p.model.DefaultDomainResolver = sel
				p.model.DefaultDomainResolverUnset = false
				changed = true
			}
		}
	}
	if gs.DNSStrategySelect != nil {
		sel := gs.DNSStrategySelect.Selected
		if wizardsync.GuiTextAwaitingProgrammaticFill(ready, sel, p.model.DNSStrategy) {
			// выпадающий список ещё не получил SetSelected
		} else if p.model.DNSStrategy != sel {
			p.model.DNSStrategy = sel
			changed = true
		}
	}
	if gs.DNSIndependentCacheCheck != nil && ready {
		v := gs.DNSIndependentCacheCheck.Checked
		cur := false
		if p.model.DNSIndependentCache != nil {
			cur = *p.model.DNSIndependentCache
		}
		if cur != v {
			nv := v
			p.model.DNSIndependentCache = &nv
			changed = true
		}
	}
	return changed
}

// ValidateAndApplyParserConfigFromEntry parses ParserConfig from the entry, validates it,
// and on success updates model and LastValidParserConfigJSON; on error shows dialog and reverts entry.
// Call when leaving the Outbounds and ParserConfig tab so manual JSON edits are applied or reverted.
func (p *WizardPresenter) ValidateAndApplyParserConfigFromEntry() {
	if p.guiState.ParserConfigEntry == nil {
		return
	}
	text := strings.TrimSpace(p.guiState.ParserConfigEntry.Text)
	if text == "" {
		p.model.ParserConfigJSON = ""
		p.model.ParserConfig = nil
		wizardbusiness.InvalidatePreviewCache(p.model)
		p.guiState.LastValidParserConfigJSON = ""
		return
	}
	pc := &config.ParserConfig{}
	if err := json.Unmarshal([]byte(text), pc); err != nil {
		dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.outbounds.error_invalid_json"), err), p.guiState.Window)
		revert := p.guiState.LastValidParserConfigJSON
		p.guiState.ParserConfigUpdating = true
		p.guiState.ParserConfigEntry.SetText(revert)
		p.guiState.ParserConfigUpdating = false
		return
	}
	if err := wizardbusiness.ValidateParserConfig(pc); err != nil {
		dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.outbounds.error_invalid_config"), err), p.guiState.Window)
		revert := p.guiState.LastValidParserConfigJSON
		p.guiState.ParserConfigUpdating = true
		p.guiState.ParserConfigEntry.SetText(revert)
		p.guiState.ParserConfigUpdating = false
		return
	}
	serialized, err := wizardbusiness.SerializeParserConfig(pc)
	if err != nil {
		dialog.ShowError(fmt.Errorf("%s: %w", locale.T("wizard.outbounds.error_serialize"), err), p.guiState.Window)
		return
	}
	p.model.ParserConfig = pc
	p.model.ParserConfigJSON = serialized
	p.guiState.LastValidParserConfigJSON = serialized
	p.UpdateParserConfig(serialized)
	p.RefreshOutboundOptions()
	if p.guiState.RefreshSourcesList != nil {
		p.guiState.RefreshSourcesList()
	}
	p.model.PreviewNeedsParse = true
	wizardbusiness.InvalidatePreviewCache(p.model)
}

// ApplyParserConfigFromCurrentJSON replaces model.ParserConfig from model.ParserConfigJSON when JSON parses and validates,
// normalizes via SerializeParserConfig, and updates the Outbounds entry. Used when opening the Outbounds tab so the
// configurator list matches JSON after edits on Sources (local outbounds) or other tabs.
func (p *WizardPresenter) ApplyParserConfigFromCurrentJSON() {
	if p.guiState == nil {
		return
	}
	raw := strings.TrimSpace(p.model.ParserConfigJSON)
	if raw == "" {
		p.model.ParserConfig = nil
		return
	}
	var pc config.ParserConfig
	if err := json.Unmarshal([]byte(raw), &pc); err != nil {
		return
	}
	if err := wizardbusiness.ValidateParserConfig(&pc); err != nil {
		return
	}
	serialized, err := wizardbusiness.SerializeParserConfig(&pc)
	if err != nil {
		return
	}
	p.model.ParserConfig = &pc
	p.model.ParserConfigJSON = serialized
	p.guiState.LastValidParserConfigJSON = serialized
	if p.guiState.ParserConfigEntry != nil {
		p.guiState.ParserConfigUpdating = true
		p.guiState.ParserConfigEntry.SetText(serialized)
		p.guiState.ParserConfigUpdating = false
	}
}
