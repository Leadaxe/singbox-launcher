// Package models содержит модели данных визарда конфигурации.
//
// Файл wizard_model.go определяет WizardModel — чистую модель данных визарда без GUI зависимостей.
//
// WizardModel содержит только бизнес-данные (без Fyne виджетов):
//   - ParserConfig данные (ParserConfigJSON, ParserConfig) — источник истины для списка источников (Proxies)
//   - SourceURLs — поле ввода для добавления новых URL (кнопка Add); не источник истины для существующих источников
//   - Сгенерированные outbounds (GeneratedOutbounds, OutboundStats)
//   - Template данные (TemplateData)
//   - Правила маршрута: CustomRules (единый список); SelectedFinalOutbound; SelectableRuleStates не используется (027)
//   - Флаги состояния бизнес-операций (AutoParseInProgress)
//
// GUI-состояние (виджеты Fyne, UI-флаги) находится в presentation/GUIState.
//
// Используется в:
//   - presentation/presenter.go — WizardPresenter хранит модель и синхронизирует её с GUI
//   - business/*.go — все функции бизнес-логики работают с WizardModel
package models

import (
	"encoding/json"
	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/platform"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
)

// Константы, связанные с бизнес-логикой визарда.
const (
	// DefaultOutboundTag — тег outbound по умолчанию для правил маршрутизации.
	DefaultOutboundTag = "direct-out"
	// RejectActionName — название действия reject в правилах маршрутизации.
	RejectActionName = "reject"
	// RejectActionMethod — метод действия reject (drop).
	RejectActionMethod = "drop"
)

// OutboundStats содержит статистику по outbounds и endpoints для preview.
type OutboundStats struct {
	NodesCount           int
	EndpointsCount       int // WireGuard endpoint nodes
	LocalSelectorsCount  int
	GlobalSelectorsCount int
}

// WizardModel — модель данных визарда конфигурации.
//
// SPEC 052 phase 8 cleanup: канонический источник истины для списка
// подключений — `Sources []corestate.Source`. Старые поля
// `ParserConfig`/`ParserConfigJSON` остаются как ДЕРИВНЫЕ:
//   - `AsParserConfig()` — собирает `*config.ParserConfig` для парсера на лету
//     из `Sources` + `GlobalOutbounds` + `Defaults`.
//   - `ParserConfigJSON` — кэш сериализации того же на момент последнего
//     RefreshSerializedParserConfig (для JSON-editor вкладки и как
//     дешёвый fingerprint для stale-detection в ParseAndPreview).
//
// SourceNodeCount — счёт узлов одного источника для списка Sources.
type SourceNodeCount struct {
	// Total — сколько узлов дал парсер.
	Total int
	// Enabled — сколько из них попадёт в конфиг: без снятых пользователем
	// галок. Меньше Total означает, что часть выключена вручную, и
	// показывать надо оба числа — иначе «47» читается как потеря трёх.
	Enabled int
}

type WizardModel struct {
	// Sources — v5-canonical источники подписок и серверов (subscription/server).
	// Editing UI напрямую мутирует этот слайс; на Save переезжает в
	// state.connections.sources через corestate.Save.
	Sources []corestate.Source

	// GlobalOutbounds — глобальные группы (selector / urltest), которые
	// рендерятся в config.json `outbounds[]`. Источник истины для
	// outbounds-конфигуратора и парсера. Зеркалит state.connections.outbounds.
	GlobalOutbounds []configtypes.Direction

	// Defaults — глобальные defaults подключений (reload interval, max_nodes
	// fallback). Зеркалит state.connections.defaults.
	Defaults corestate.Defaults

	// WarpAccounts — кеш выданных Cloudflare регистраций WARP (nil = пусто).
	// Зеркалит state.warp_accounts. Диалог Add WARP переиспользует запись
	// вместо новой регистрации, поэтому MASQUE H2/H3 ложатся на один ключ.
	WarpAccounts *corestate.WarpAccountsSection

	// ParserConfigJSON — derived: кэш сериализации `AsParserConfig()` в
	// строку для JSON-editor виджета. Refresh в `RefreshSerializedParserConfig`
	// после любой мутации Sources/GlobalOutbounds. Не источник истины.
	ParserConfigJSON string

	// ParserConfig — derived: кэш `AsParserConfig()` для callsite'ов которые
	// не получают модель напрямую (preview cache, parser pipeline). Заполняется
	// `RefreshDerivedParserConfig`. Не источник истины.
	ParserConfig *config.ParserConfig

	// SourceURLs — текст в поле "Subscription URL or Direct Links" (ввод для кнопки Add); не используется для замены Proxies
	SourceURLs string

	// Сгенерированные outbounds и endpoints (WireGuard)
	GeneratedOutbounds []string
	GeneratedEndpoints []string
	OutboundStats      OutboundStats

	// Template данные
	TemplateData *wizardtemplate.TemplateData

	// Target (SPEC 097) — для какой машины готовится конфиг: local (эта
	// машина) или remote (сервер/роутер/другой mac) + платформа целевой
	// машины. Выбирается на шаге 0 визарда, сериализуется в state.meta.
	//
	// Zero value = «эта машина, local»: модель, созданная кодом, не знающим
	// о таргетах, ведёт себя ровно как до SPEC 097.
	Target wizardtemplate.TargetSpec

	// Правила (маршрут — только CustomRules; SelectableRuleStates не используется после 027)
	SelectableRuleStates  []*RuleState
	CustomRules           []*RuleState
	RulesLibraryMerged    bool // true после миграции/засева; сериализуется в state.json
	SelectedFinalOutbound string

	// PresetRefs — preset-ref правила (kind=preset). Хранятся параллельно
	// CustomRules (которые держат kind=inline/srs). При Save копируются в
	// state.Rules; на Load восстанавливаются из state.Rules.
	// Каждый элемент — {Ref, Enabled, Vars}.
	PresetRefs []*PresetRefState

	// DNSTemplateOverrides — overrides для template-defined DNS-серверов.
	// Map tag → enabled. Только tag'и где юзер изменил default_enabled.
	DNSTemplateOverrides map[string]bool

	// SPEC 057-R-N: preset outbound binding live в state.connections.outbounds[]
	// напрямую через `ref` field (см. configtypes.Direction.Ref + .Updates).
	// Display order = natural slice order — больше нет вспомогательной in-memory
	// карты OutboundDisplayOrder. Up/Down работают через swap в slice
	// (перетаскиванием), reorder автоматически персистится при сохранении.

	// SPEC 056-R-N follow-up: per-server toggle для preset-bundled DNS-серверов
	// и dns_rule живут внутри PresetRefState (DNSServerEnabled + DNSRuleEnabled).
	// Раньше были отдельные карты в model; теперь scope естественно привязан
	// к preset-ref instance — lifecycle автоматический.

	// RuleOrder — единый упорядоченный список slot'ов, определяющий порядок
	// отображения правил в Rules tab и порядок эмита в config.json::route.rules[].
	// См. rule_slot.go для подробностей.
	RuleOrder []RuleSlot

	// DNSUserRules — типизированный список user-defined DNS rules
	// (SPEC 062-F-N). Replaces JSON-string `DNSRulesText` для primary editing.
	// `DNSRulesText` остаётся для raw-JSON editor toggle (см. DNSUserRulesFromText
	// / DNSUserRulesToText в dns_user_rule.go).
	DNSUserRules []DNSUserRule

	// DNSRuleOrder — упорядоченный список slot'ов для DNS Rules секции в DNS tab
	// (SPEC 062-F-N). Зеркало RuleOrder для route rules. Каждый slot ссылается
	// либо на DNSUserRules[i], либо на PresetRefs[i].dns_rule. Определяет порядок
	// отображения и порядок эмита в state.DNS.Rules[].
	// См. dns_rule_slot.go.
	DNSRuleOrder []DNSRuleSlot

	// SettingsVars — переопределения вкладки Settings (name → value); пустое значение ключа = дефолт шаблона.
	SettingsVars map[string]string `json:"-"`

	// Флаги состояния бизнес-операций
	PreviewNeedsParse   bool
	AutoParseInProgress bool

	// Preview кеш для распарсенных нод (используется всеми Preview/View, включая вкладку Preview в Edit Outbound)
	PreviewNodes         []*config.ParsedNode
	PreviewNodesBySource map[int][]*config.ParsedNode

	// SourceNodeCounts — сколько узлов даст каждый источник (по индексу в
	// Sources): всего распарсено и сколько из них пойдёт в конфиг после
	// skip-фильтров и снятых галок.
	//
	// Кэш, а не вычисление на месте: разбор всех подписок занимает секунды
	// на живых конфигах (сотни узлов), а строка списка перерисовывается на
	// каждое движение мыши. nil = ещё не считали.
	SourceNodeCounts map[int]SourceNodeCount

	// PreviewCacheGeneration растёт на каждую инвалидцию кэша превью.
	// Фоновый счёт (EnsureSourceNodeCounts) снимает значение до разбора и
	// сверяет после: если источники менялись, пока шёл счёт, результат
	// относится к СТАРОМУ списку (ключи — индексы!) и выбрасывается, а не
	// пишется поверх — иначе счётчики съезжали на чужие строки.
	PreviewCacheGeneration int

	// PreviewIgnoredSectionsBySource — секции импортированного sing-box конфига,
	// которые парсер намеренно не читает (route/dns/inbounds/experimental),
	// по индексу источника (SPEC 094 A4). Показывается в превью, чтобы
	// пропущенное не выглядело как потеря данных. nil, если ни один источник
	// не отдал целый конфиг.
	PreviewIgnoredSectionsBySource map[int][]string

	// Мемо для GetAvailableOutbounds при чтении только из ParserConfigJSON (ParserConfig == nil); сброс в InvalidatePreviewCache.
	AvailableOutboundsMemoKey  string   `json:"-"`
	AvailableOutboundsMemoTags []string `json:"-"`

	// ExecDir — директория исполняемого файла (для путей к SRS и т.д.)
	ExecDir string

	// ResourceDir — каталог ресурсов машины, для которой собирается конфиг:
	// `<state_dir>/resources` её демона (SPEC 063). Пусто для local.
	//
	// Путь резолвит ядро НА ТОЙ СТОРОНЕ, поэтому в rule_set[].path для
	// удалённой машины должен уезжать он, а не ExecDir лаунчера: своего пути
	// на роутере нет, и ядро не нашло бы набор.
	ResourceDir string

	// MachineID — ID машины, для которой собирается конфиг (пусто для local).
	// Нужен, чтобы знать, в чей каталог .srs качать наборы.
	MachineID string

	// DNS tab (sing-box config.dns + route.default_domain_resolver)
	DNSServers []json.RawMessage
	// DNSLockedTags — УДАЛЕНО в SPEC unify. Lock-channel живёт в template
	// через `required: true` в dns_options.servers[]. См. wizardbusiness.DNSTagLocked.
	DNSRulesText string
	DNSFinal     string
	DNSStrategy  string
	// DNSIndependentCache — УДАЛЕНО: independent_cache deprecated в sing-box
	// 1.14.0 (кэш всегда per-transport). Поле снято из UI/model/emit.
	DefaultDomainResolver      string
	DefaultDomainResolverUnset bool // resolver explicitly omitted; omit route.default_domain_resolver in output
}

// NewWizardModel создает новую модель визарда с начальными значениями.
func NewWizardModel() *WizardModel {
	return &WizardModel{
		PreviewNeedsParse:    true,
		SettingsVars:         make(map[string]string),
		SelectableRuleStates: make([]*RuleState, 0),
		CustomRules:          make([]*RuleState, 0),
		GeneratedOutbounds:   make([]string, 0),
		GeneratedEndpoints:   make([]string, 0),
		Sources:              make([]corestate.Source, 0),
		GlobalOutbounds:      make([]configtypes.Direction, 0),
		Defaults:             corestate.Defaults{Reload: "4h", MaxNodes: corestate.DefaultMaxNodes},
	}
}

// AsParserConfig собирает legacy-форму parser config для парсера/preview
// из канонических Sources + GlobalOutbounds + Defaults.
//
// Каждый Source конвертится через v5.(*Source).ToProxySourceV4():
//   - subscription → ProxySource{Source, Skip, Outbounds, Tag*, Disabled, ...}
//   - server       → ProxySource{Connections:[URI], TagMask=Label, Disabled}
//
// Возвращаемый pointer указывает на свежий объект — caller может его
// мутировать (substitute placeholders) без побочных эффектов на модель.
func (m *WizardModel) AsParserConfig() *config.ParserConfig {
	if m == nil {
		return &config.ParserConfig{}
	}
	pc := &config.ParserConfig{}
	pc.ParserConfig.Version = configtypes.ParserConfigVersion
	pc.ParserConfig.Proxies = make([]configtypes.ProxySource, 0, len(m.Sources))
	for i := range m.Sources {
		pc.ParserConfig.Proxies = append(pc.ParserConfig.Proxies, m.Sources[i].ToProxySourceV4())
	}
	if len(m.GlobalOutbounds) > 0 {
		pc.ParserConfig.Outbounds = append([]configtypes.Direction(nil), m.GlobalOutbounds...)
	} else {
		pc.ParserConfig.Outbounds = []configtypes.Direction{}
	}
	pc.ParserConfig.Parser.Reload = m.Defaults.Reload
	return pc
}

// RefreshDerivedParserConfig — вызывается после мутации Sources/GlobalOutbounds
// для синхронизации деривных кэшей (`ParserConfig` + `ParserConfigJSON`).
// Идемпотентна; ошибки сериализации тихие (для JSON-editor display'а).
func (m *WizardModel) RefreshDerivedParserConfig() {
	if m == nil {
		return
	}
	m.ParserConfig = m.AsParserConfig()
	if data, err := json.MarshalIndent(map[string]interface{}{"ParserConfig": m.ParserConfig.ParserConfig}, "", "  "); err == nil {
		m.ParserConfigJSON = string(data)
	}
}

// SrsDir — каталог, куда качать .srs для текущего таргета (SPEC 098 §2.3).
// Пусто для local: там путь исторический, bin/rule-sets/.
func (m *WizardModel) SrsDir() string {
	if m == nil || m.MachineID == "" {
		return ""
	}
	return platform.GetRuleSetsDirFor(m.ExecDir, constants.ConfigTargetRemote, m.MachineID)
}
