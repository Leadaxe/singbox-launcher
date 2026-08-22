// Package template содержит структуры и парсеры wizard_template.json.
//
// File preset_types.go — Go-типы preset bundles (SPEC 053).
//
// Preset — self-contained параметризованная конструкция в template:
// vars (типизированные переменные), rule_set'ы, dns_servers, routing rule,
// dns_rule. Tag'и rule_set/dns_servers ЛОКАЛЬНЫ внутри preset'а и при build
// автопрефиксируются `<preset_id>:<tag>` — глобального namespace тегов нет.
//
// state.json (v6) хранит на preset тонкую ссылку {kind: "preset", ref, vars}
// — match-поля живут в template, bump RequiredTemplateRef → юзеры
// автоматически получают обновлённые match-поля.
//
// См. SPECS/053-F-N-PRESET_BUNDLES/SPEC.md для полного описания.
package template

import (
	"encoding/json"

	"singbox-launcher/core/config/configtypes"
)

// Preset — параметризованный self-contained пресет в template.presets[].
type Preset struct {
	// ID — стабильный slug-идентификатор (`[a-z0-9_-]+`).
	// State.rules[] preset-ref ссылается через {ref: <id>}.
	ID string `json:"id"`

	// Label — название в UI Library/Rules tile.
	Label string `json:"label"`

	// Description — длинное описание (tooltip / library card).
	Description string `json:"description,omitempty"`

	// DefaultEnabled — рекомендация template'а: включён ли preset
	// на fresh install / при первом появлении после bump'а.
	// Не state; реальное состояние enable/disable в state.rules[].enabled.
	DefaultEnabled bool `json:"default_enabled,omitempty"`

	// Num — стартовая позиция на разреженной оси порядка (SPEC 106, D-051).
	// 0 у неотчуждаемой головы; шаблонные якоря идут с шагом 10, зона
	// пользовательских правил — 1000..1100. Значение попадает в
	// state.Rules[].OrderNum при первой разметке и дальше живёт в state:
	// пользователь двигает правило, номер пересчитывается.
	// Отсутствует в JSON → DefaultRuleNum (начало пользовательской зоны).
	Num *int `json:"num,omitempty"`

	// Sortable — можно ли двигать правило в UI. Отсутствует → true.
	//
	// false делает пресет НЕОТЧУЖДАЕМЫМ (D-050): такой пресет
	// (а) пере-засевается при каждой сборке, даже если его стёрли из state;
	// (б) не двигается перетаскиванием и не вытесняется соседями — его номер
	// часть инварианта. Пример — traffic-processing на позиции 0: sniff
	// обязан отработать до матчинга роутинга, иначе домен не извлечён.
	Sortable *bool `json:"sortable,omitempty"`

	// Locked — запрет выключения и удаления в UI. Отсутствует → false.
	// Роль отдельная от Sortable: Sortable управляет позицией и seed'ом,
	// Locked — доступностью тумблера и кнопки удаления.
	Locked bool `json:"locked,omitempty"`

	// Platforms — ОС где preset доступен. Пустой список = все платформы.
	// Loader фильтрует по runtime.GOOS — preset с несовпадающей платформой
	// не появляется в Library и не присутствует в TemplateData.Presets.
	Platforms []string `json:"platforms,omitempty"`

	// Vars — типизированные переменные preset'а (локальный scope).
	Vars []PresetVar `json:"vars,omitempty"`

	// RuleSet — определения rule_set'ов, локальные tag'и.
	// При build префиксуются `<preset_id>:<tag>`.
	RuleSet []PresetRuleSet `json:"rule_set,omitempty"`

	// DNSServers — bundled DNS-серверы preset'а, локальные tag'и.
	// При build префиксуются и фильтруются (только используемые попадают в config).
	DNSServers []PresetDNSServer `json:"dns_servers,omitempty"`

	// Rules — routing rules preset'а. Каждая entry — отдельный routing rule,
	// эмитится в порядке списка. Может содержать @varname плейсхолдеры,
	// `rule_set` ссылки на локальные tag'и (по имени без префикса). Каждая
	// rule имеет собственные `if`/`if_or` для conditional emit.
	//
	// SPEC 067 Phase 9: было одиночное Rule, теперь slice — позволяет
	// presets вроде split-all-traffic эмитить несколько rule (по одному per
	// outbound). Empty / nil — preset без routing rule (только rule_set /
	// dns_servers / outbounds), валидно.
	Rules []map[string]interface{} `json:"rules,omitempty"`

	// DNSRule — DNS-rule preset'а (одиночный). Опциональный, имеет свой `if`.
	DNSRule map[string]interface{} `json:"dns_rule,omitempty"`

	// DNSRules — упорядоченный список DNS-правил preset'а (SPEC 085.1). Нужен,
	// когда пресету требуется >1 правила (FakeIP: HTTPS/SVCB predefined-блок +
	// A/AAAA→fakeip). Все правила делят ОДИН toggle на пресет (state.DNS.Rules
	// несёт один Ref=<id>), эмитятся в порядке ПОСЛЕ singular DNSRule. Каждое
	// имеет свой `if`. Predefined/action-правило без server — валидно.
	DNSRules []map[string]interface{} `json:"dns_rules,omitempty"`

	// Outbounds — preset.outbounds[] (SPEC 056). Каждая entry — либо
	// mode="add" (новый outbound, требует Type), либо mode="update"
	// (патч existing outbound по Tag). См. PresetOutbound + comments в
	// core/build/preset_outbounds.go::ApplyPresetOutboundsToParserConfig.
	//
	// Архитектурно — это parser-format (зеркалит configtypes.Direction).
	// Build pipeline применяет их **до** native outbound generator'а:
	// типизированный pre-patch parserCfg.ParserConfig.Outbounds[] →
	// нативный generator сам делает options-flatten, filters/addOutbounds
	// резолв, comment-prefix. Никаких post-merge JSON-патчей или strip'ов.
	Outbounds []PresetOutbound `json:"outbounds,omitempty"`
}

// DisplayLabel returns the human-facing name for the preset: Label, or the ID
// as a fallback when Label is empty. Single source of truth for the build
// pipeline and the configurator UI (was duplicated as presetDisplayLabel).
func (p *Preset) DisplayLabel() string {
	if p.Label != "" {
		return p.Label
	}
	return p.ID
}

// presetGate — общее поле гейта #enable для типизированных фрагментов
// (SPEC 107). Хранится сырым: разбор и слияние с легаси if/if_or делает
// template.NormalizeGate в момент применения.
type presetGate struct {
	Enable json.RawMessage `json:"#enable,omitempty"`
}

// EnableRaw возвращает разобранное значение #enable (nil — ключа не было).
func (g presetGate) EnableRaw() interface{} {
	if len(g.Enable) == 0 {
		return nil
	}
	var v interface{}
	if err := json.Unmarshal(g.Enable, &v); err != nil {
		// Невалидный JSON — отдаём маркер, который движок отвергнет
		// (fail-closed): гейт не пройден, фрагмент не эмитится.
		return map[string]interface{}{"and": []interface{}{"@\u0000invalid"}}
	}
	return v
}

// PresetOutbound — entry preset.outbounds[] (SPEC 056).
//
// Поля Tag/Type/Options/Filters/AddOutbounds/PreferredDefault/Comment/Wizard
// зеркалят configtypes.Direction — намеренно, чтобы Phase 3 expand
// просто маппил поля в Direction без преобразований.
//
// Контрол-поля Mode/If/IfOr НЕ попадают в финальный config (используются
// только на этапе ExpandPresetOutbounds для разрешения какой outbound
// эмитить и в каком режиме).
type PresetOutbound struct {
	presetGate
	// Mode — режим применения:
	//   ""       → "add" (default)
	//   "add"    → добавить новый outbound (нужен Type)
	//   "update" → патчить existing outbound по Tag (Type запрещён)
	// Unknown values → loader strip'ает entry с warning.
	Mode string `json:"mode,omitempty"`

	// Tag — идентификатор outbound'а.
	//   mode=add    → tag нового outbound'а; collision с globals или earlier
	//                 preset → first wins + warning (identical body → silent skip)
	//   mode=update → tag existing outbound в parser_config.outbounds[];
	//                 если не найден → warning, no-op (no auto-create)
	// Required для обоих режимов.
	Tag string `json:"tag"`

	// Type — sing-box outbound type (selector / urltest / direct / shadowsocks / …).
	// Required для mode=add. Для mode=update запрещён (Tag/Type immutable —
	// loader strip'ает поле и warning'ает; в Phase 3 expand тоже dropped).
	Type string `json:"type,omitempty"`

	// Options — sing-box outbound options (nested object).
	// mode=add: установлен как есть.
	// mode=update: per-field replace в target.Options (нет глубокого merge —
	// заменяются только заданные ключи top-level).
	Options map[string]interface{} `json:"options,omitempty"`

	// Filters — фильтр нод для selector/urltest (native pipeline разруливает
	// в config.outbounds[].outbounds[] через filterNodesForSelector).
	// mode=add: установлен как есть.
	// mode=update: replace целиком (если задан).
	Filters map[string]interface{} `json:"filters,omitempty"`

	// AddOutbounds — дополнительные outbound-tag'и, добавляются к фильтру.
	// mode=add: установлен как есть.
	// mode=update: union с target.AddOutbounds (preserve order, dedupe).
	AddOutbounds []string `json:"addOutbounds,omitempty"`

	// PreferredDefault — приоритетный default для selector'а
	// (см. configtypes.Direction).
	// mode=add: установлен.
	// mode=update: replace целиком.
	PreferredDefault map[string]interface{} `json:"preferredDefault,omitempty"`

	// Comment — // prefix перед JSON entry (native pipeline печатает как
	// "// %s\n"). Не JSON-поле в финале.
	// mode=update: replace если задан непустой.
	Comment string `json:"comment,omitempty"`

	// Label — отображаемое имя Направления (SPEC 104). Пресет может как
	// создать направление с именем (mode=add), так и переименовать чужое
	// (mode=update, решение D-10).
	// mode=update: replace если задан непустой.
	Label string `json:"label,omitempty"`

	// Disabled — выключить направление (SPEC 104). Указатель: nil означает
	// «пресет об этом не высказывался», иначе mode=update всегда включал бы
	// направление, которое пользователь выключил сам.
	Disabled *bool `json:"disabled,omitempty"`

	// Auto — параметры парного `<tag>-auto` (SPEC 104).
	// mode=update: replace целиком, если задан.
	Auto *configtypes.DirectionAuto `json:"auto,omitempty"`

	// Wizard — UI metadata (hide / required). Не идёт в финальный config
	// (native pipeline стрипает). Можно задать в preset для override
	// template wizard-настроек при mode=update.
	Wizard interface{} `json:"wizard,omitempty"`

	// If — entry активна iff ВСЕ перечисленные bool vars true.
	// Семантика идентична PresetVar.If (cascade через vars_resolve.go).
	If []string `json:"if,omitempty"`

	// IfOr — активна iff ХОТЯ БЫ ОДНА из перечисленных bool vars true.
	IfOr []string `json:"if_or,omitempty"`
}

// PresetVar — типизированная переменная preset'а.
//
// Все vars required (поля required/optional нет). Опциональность достигается
// через `if`/`if_or` на vars и фрагментах preset'а — переиспользует
// существующий механизм TemplateVar.If/IfOr (vars_resolve.go).
type PresetVar struct {
	presetGate
	// Name — идентификатор переменной, локальный в preset'е.
	// `@<name>` в rule_set/rule/dns_rule/dns_servers резолвится по этому имени.
	Name string `json:"name"`

	// Type — тип переменной:
	//   "outbound"   — picker outbound-тегов + "reject"/"drop" литералы
	//   "dns_server" — picker DNS-тегов (bundled / template / extras);
	//                  шаблон LxBox пишет "dns_servers" — читается как
	//                  синоним, нормализуется при загрузке
	//   "enum"       — dropdown {title, value}
	//   "text"       — text entry
	//   "number"     — numeric entry
	//   "bool"       — checkbox
	Type string `json:"type"`

	// Default — обязательное default-значение (для type=bool: "true"/"false").
	// Substitute использует если varsValues[name] пусто/отсутствует.
	//
	// Канон поля дефолта — `default_value` (TEMPLATE_LANG §6.1); `default`
	// читается как алиас, чтобы шаблоны обоих приложений грузились без правок.
	Default string `json:"default"`

	// DefaultValue — канонический алиас Default (разрыв N3). Значение
	// сливается в Default при загрузке, см. UnmarshalJSON.
	DefaultValue string `json:"default_value,omitempty"`

	// Ref — ССЫЛКА на глобальную переменную шаблона (SPEC 106, модель LxBox
	// §265). Значение живёт в глобальных vars, НЕ в теле правила; метаданные
	// (тип, options, заголовок) подтягиваются из объявления той переменной.
	//
	// Зачем: пресет показывает в своих настройках общую настройку, не заводя
	// её копию. Пример — стратегия разрешения имён: она же питает dns.strategy,
	// и вторая копия разъехалась бы с первой при первом же изменении.
	Ref string `json:"ref,omitempty"`

	// Required — пустое значение делает фрагменты пресета несобираемыми.
	// Пресет не выбрасывается целиком: правило, ссылающееся на пустую
	// обязательную переменную, выпадает с warning (Dropped-каскад §5.1).
	Required bool `json:"required,omitempty"`

	// Title — UI label (если пусто, используется Name).
	Title string `json:"title,omitempty"`

	// Tooltip — всплывающая подсказка.
	Tooltip string `json:"tooltip,omitempty"`

	// Options — варианты выбора (декодер зависит от Type):
	//   type=enum         → []OptionEntry (объектная форма с title/value)
	//   type=dns_server   → []string (whitelist tag'ов) — explicit options
	//   type=outbound     → []string (whitelist outbound-тегов + reject/drop)
	//   остальные         — игнорируется
	//
	// Парсинг по type через DecodeOptions.
	Options json.RawMessage `json:"options,omitempty"`

	// Select — shortcut для type=dns_server (mutually exclusive с Options):
	//   "local"  → только bundled tag'и preset'а
	//   "global" → все available (bundled ∪ template effective_enabled ∪ extras)
	//   default (omit) → "global"
	//
	// Для type=outbound поле игнорируется + warning (нет concept'а
	// "local outbounds" — outbound'ы всегда template-global).
	Select string `json:"select,omitempty"`

	// If — переменная активна iff ВСЕ перечисленные bool vars true.
	// Если активна false → var удаляется из varsMap, фрагменты с @<this> теряются
	// (через cascade или через свой собственный if на фрагменте).
	// Семантика идентична TemplateVar.If (vars_resolve.go).
	If []string `json:"if,omitempty"`

	// IfOr — активна iff ХОТЯ БЫ ОДНА из перечисленных bool vars true.
	IfOr []string `json:"if_or,omitempty"`
}

// OptionEntry — объектная форма option-варианта (для type=enum).
type OptionEntry struct {
	// Title — отображаемый в UI текст.
	Title string `json:"title"`

	// Value — машинное значение, подставляется в substitute.
	Value string `json:"value"`
}

// PresetRuleSet — определение rule_set внутри preset'а.
//
// Tag локален; при build префиксуется `<preset_id>:<tag>`. Может быть
// inline (rules в template) или remote (url для download'а в bin/rule-sets/).
type PresetRuleSet struct {
	presetGate
	// Tag — локальный tag, при build → `<preset_id>:<tag>`.
	Tag string `json:"tag"`

	// Type — "inline" | "remote".
	Type string `json:"type"`

	// Format — "domain_suffix" | "binary" | другие sing-box форматы.
	Format string `json:"format,omitempty"`

	// Rules — inline rule entries (для type=inline).
	Rules []map[string]interface{} `json:"rules,omitempty"`

	// URL — remote .srs URL (для type=remote). Скачивается через
	// content-addressed tag scheme `<filename>-<hash8(sha256(URL))>`.
	URL string `json:"url,omitempty"`

	// If/IfOr — условный rule_set (например geoip-ru под `if: ["geoip_enabled"]`).
	If   []string `json:"if,omitempty"`
	IfOr []string `json:"if_or,omitempty"`
}

// PresetDNSServer — bundled DNS-сервер preset'а.
//
// При build:
//   - Tag префиксуется `<preset_id>:<tag>`.
//   - Title стрипается (UI-only).
//   - Description пробрасывается в config.json как valid sing-box поле.
//   - Включается в emit только если выбран через @dns_server var ИЛИ
//     явно упомянут литералом в dns_rule.server.
type PresetDNSServer struct {
	presetGate
	// Tag — локальный tag, при build → `<preset_id>:<tag>`.
	Tag string `json:"tag"`

	// Type — sing-box DNS server type: "udp" | "https" | "tls" | "h3".
	Type string `json:"type"`

	// Server — IP или hostname.
	Server string `json:"server"`

	// ServerPort — порт (опционально, дефолт по типу).
	ServerPort int `json:"server_port,omitempty"`

	// Path — для https-type (URL path, например "/dns-query").
	Path string `json:"path,omitempty"`

	// TLS — TLS-конфигурация (server_name, enabled, ...).
	TLS map[string]interface{} `json:"tls,omitempty"`

	// Detour — outbound tag для forwarding'а. Может быть @varname.
	// Если резолвится в "direct-out" — ключ удаляется при emit (sing-box
	// резолвит через default_domain_resolver без forwarding'а).
	Detour string `json:"detour,omitempty"`

	// Title — UI label для picker'а; stripped at emit. Fallback на Tag если пусто.
	Title string `json:"title,omitempty"`

	// Description — valid sing-box DNS server field, пробрасывается в config.
	// Показывается в debug views / config inspection / logs.
	Description string `json:"description,omitempty"`

	// Inet4Range/Inet6Range — для type:"fakeip" (SPEC 085): CIDR-диапазоны,
	// из которых sing-box выдаёт синтетические A/AAAA-ответы. Пробрасываются в
	// config как есть.
	Inet4Range string `json:"inet4_range,omitempty"`
	Inet6Range string `json:"inet6_range,omitempty"`

	// If/IfOr — условный DNS-сервер.
	If   []string `json:"if,omitempty"`
	IfOr []string `json:"if_or,omitempty"`
}

// DecodeOptions парсит PresetVar.Options в зависимости от Type.
//
// Возвращает (enum []OptionEntry, tagList []string, ok). Один из двух
// результатов будет non-nil:
//   - type=enum                 → enum non-nil, tagList nil
//   - type=dns_server/outbound  → tagList non-nil, enum nil
//   - другие типы               → оба nil, ok=true (options не используется)
//   - parse error               → оба nil, ok=false
func (v *PresetVar) DecodeOptions() (enum []OptionEntry, tagList []string, ok bool) {
	if len(v.Options) == 0 || string(v.Options) == "null" {
		return nil, nil, true
	}

	switch v.Type {
	case "enum":
		var entries []OptionEntry
		if err := json.Unmarshal(v.Options, &entries); err != nil {
			// Fallback: попробуем []string (legacy form: title==value).
			var strs []string
			if err2 := json.Unmarshal(v.Options, &strs); err2 != nil {
				return nil, nil, false
			}
			for _, s := range strs {
				entries = append(entries, OptionEntry{Title: s, Value: s})
			}
		}
		return entries, nil, true

	case "dns_server", "outbound":
		var tags []string
		if err := json.Unmarshal(v.Options, &tags); err != nil {
			return nil, nil, false
		}
		return nil, tags, true

	default:
		// Options не применимо для bool/text/number — игнорируем.
		return nil, nil, true
	}
}

// UnmarshalJSON — слияние канонического `default_value` в `Default`
// (TEMPLATE_LANG §6.1, разрыв N3).
//
// Пресеты лаунчера исторически пишут `default`, мобильный шаблон — канонический
// `default_value`. Читаем обе формы, чтобы приём, придуманный в одном
// приложении, переносился в другое без правки шаблона. При наличии обеих
// побеждает каноническая.
// canonicalVarType сводит синонимы имён типов к каноническому написанию.
//
// Разрыв имён между приложениями: у нас тип picker'а DNS-тегов называется
// `dns_server`, в шаблоне LxBox — `dns_servers`. Одна сущность под двумя
// именами означала бы, что шаблон, написанный на той стороне, у нас
// молча теряет переменную (validateVar отвергает неизвестный тип и
// пропускает весь пресет). Канон — единственное число, как у соседнего
// `outbound`; множественное принимается бессрочно и обратно не пишется.
func canonicalVarType(t string) string {
	if t == "dns_servers" {
		return "dns_server"
	}
	return t
}

func (v *PresetVar) UnmarshalJSON(data []byte) error {
	type presetVarAlias PresetVar // без метода — иначе бесконечная рекурсия
	var a presetVarAlias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	if a.DefaultValue != "" {
		a.Default = a.DefaultValue
	}
	a.Type = canonicalVarType(a.Type)
	// Ref-переменная не несёт собственного имени: `{"ref": "x"}` значит
	// «показать глобальную x». Имя нужно для резолва @x внутри тела пресета.
	if a.Ref != "" && a.Name == "" {
		a.Name = a.Ref
	}
	*v = PresetVar(a)
	return nil
}
