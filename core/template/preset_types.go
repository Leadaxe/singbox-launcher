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

	// Rule — routing rule preset'а. Может содержать @varname плейсхолдеры,
	// `rule_set` ссылки на локальные tag'и (по имени без префикса).
	Rule map[string]interface{} `json:"rule,omitempty"`

	// DNSRule — DNS-rule preset'а. Опциональный, имеет свой `if`.
	DNSRule map[string]interface{} `json:"dns_rule,omitempty"`

	// Outbounds — preset-emitted outbound groups (SPEC 055). Каждый элемент
	// — либо новый outbound (mode="add", default), либо патч существующего
	// (mode="update"). Tag user-facing, без префикса preset_id.
	Outbounds []PresetOutbound `json:"outbounds,omitempty"`
}

// PresetOutbound — определение outbound внутри preset'а (SPEC 055).
//
// Mode семантика:
//
//	"add" (default) — новый outbound; tag-collision = first-wins + warning.
//	"update"        — патч существующего outbound (из parser_config.outbounds
//	                  или ранее-эмитнутого preset'ом).
//
// При update'е поля сливаются по правилам:
//
//	filters       → replace whole map
//	addOutbounds  → union (append unique)
//	options.*     → replace per-field только тех что заданы
//	wizard.*      → replace per-field
//	type          → запрещено менять (drop + warning)
//	tag           → запрещено менять (drop)
//	comment       → replace
//
// Tag НЕ префиксуется preset_id (в отличие от rule_set/dns_servers) —
// outbounds это user-facing dropdown options, имена должны быть стабильны
// и понятны пользователю (`ru VPN 🇷🇺`, не `ru-inside:ru VPN 🇷🇺`).
type PresetOutbound struct {
	// Mode — "add" (default if empty) | "update". Любое другое значение
	// → loader warning, outbound пропускается.
	Mode string `json:"mode,omitempty"`

	// Tag — идентификатор outbound'а в финальном config.outbounds[].
	Tag string `json:"tag"`

	// Type — sing-box outbound type ("selector", "urltest", "shadowsocks",
	// "direct", "http", "socks", "wireguard", "tor", ...). Required для
	// mode="add". Для mode="update" должно быть пустым (иначе drop + warning).
	Type string `json:"type,omitempty"`

	// Options — outbound-type-specific опции (например {default, interrupt_exist_connections}
	// для selector / urltest). Содержимое может содержать @var-плейсхолдеры
	// для substitution через PresetVar.
	Options map[string]interface{} `json:"options,omitempty"`

	// Filters — фильтры для subscription proxies (например {"tag": "/(🇷🇺)/i"}).
	// При mode="update" — replace whole filters map.
	Filters map[string]interface{} `json:"filters,omitempty"`

	// AddOutbounds — список outbound тегов которые добавляются в selector/urltest
	// помимо отфильтрованных. При mode="update" — union с existing.
	AddOutbounds []string `json:"addOutbounds,omitempty"`

	// Comment — свободный комментарий (попадает в emit).
	Comment string `json:"comment,omitempty"`

	// Wizard — launcher-specific метаданные (wizard.required, ...).
	// Не нативное sing-box поле, обрабатывается parser_config builder'ом.
	Wizard map[string]interface{} `json:"wizard,omitempty"`

	// If/IfOr — условный outbound (например только при включённой var).
	// Семантика та же что для PresetRuleSet.If/IfOr.
	If   []string `json:"if,omitempty"`
	IfOr []string `json:"if_or,omitempty"`
}

// PresetVar — типизированная переменная preset'а.
//
// Все vars required (поля required/optional нет). Опциональность достигается
// через `if`/`if_or` на vars и фрагментах preset'а — переиспользует
// существующий механизм TemplateVar.If/IfOr (vars_resolve.go).
type PresetVar struct {
	// Name — идентификатор переменной, локальный в preset'е.
	// `@<name>` в rule_set/rule/dns_rule/dns_servers резолвится по этому имени.
	Name string `json:"name"`

	// Type — тип переменной:
	//   "outbound"   — picker outbound-тегов + "reject"/"drop" литералы
	//   "dns_server" — picker DNS-тегов (bundled / template / extras)
	//   "enum"       — dropdown {title, value}
	//   "text"       — text entry
	//   "number"     — numeric entry
	//   "bool"       — checkbox
	Type string `json:"type"`

	// Default — обязательное default-значение (для type=bool: "true"/"false").
	// Substitute использует если varsValues[name] пусто/отсутствует.
	Default string `json:"default"`

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
