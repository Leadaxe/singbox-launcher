package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"singbox-launcher/core/config/configtypes"
)

// ErrNotFound — state-файл не существует. Вызывающий обычно интерпретирует
// это как «свежая установка», а не как ошибку.
var ErrNotFound = errors.New("state: file not found")

// Load читает state.json по пути path.
//
// Поведение:
//   - файл отсутствует → ErrNotFound;
//   - v5 (top-level "meta" с "version":5) → парсим напрямую;
//   - v2 / v3 / v4 (top-level "version") → legacy decode + auto-миграция в v5;
//   - неизвестная версия → ошибка «regenerate via wizard»;
//   - битый JSON → ошибка с понятным контекстом.
//
// SPEC 056-R-N: при загрузке v6 файла со старым дев-shape (`dns.template_servers`/
// `extra_servers`/`extra_rules`) `parseCurrent` читает его через legacyDevDNSToOptions
// fallback и конвертит in-memory в новый flat shape. На ближайшем Save файл
// перезаписывается в новом layout'е. Никакого backup'а не делаем — конверсия
// lossless (TestRoundTrip покрывает), v6 не релизился (только dev-state).
//
// Save после Load пишет либо v5, либо v6 в новом shape.
func Load(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("state: read %s: %w", path, err)
	}
	return Parse(data)
}

// Parse — Load из уже прочитанных байтов.
func Parse(data []byte) (*State, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("state: empty payload")
	}

	// Шаг 1: попытаться распознать формат — есть ли top-level "meta"?
	var probe struct {
		TopLevelVersion int `json:"version"`
		Meta            struct {
			Version int `json:"version"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("state: parse json: %w", err)
	}

	switch {
	case probe.Meta.Version >= 6:
		return parseCurrent(data)
	case probe.Meta.Version == 5:
		return parseV5Legacy(data)
	case probe.TopLevelVersion >= 2 && probe.TopLevelVersion <= 4:
		return parseLegacyAndMigrate(data)
	case probe.TopLevelVersion == 0 && probe.Meta.Version == 0:
		return nil, fmt.Errorf("state: unknown schema (neither legacy version nor meta.version present)")
	default:
		return nil, fmt.Errorf("state: unsupported version (top=%d, meta.version=%d) — regenerate via Configurator",
			probe.TopLevelVersion, probe.Meta.Version)
	}
}

// parseCurrent — прямой read canonical (v6) формата (SPEC 053 + SPEC 056-R-N).
//
// v6.State содержит:
//   - meta {version: 6, schema: "presets_v1", ...}
//   - connections (без изменений vs v5)
//   - rules[] с kind discriminator (preset/inline/srs)
//   - vars[] (глобальные template vars, включая dns_* scalars)
//   - dns_options { servers[{kind, ref|tag, enabled, ...body}], rules[...] }
//
// **Backward compat для старого дев-shape (SPEC 053):** если в JSON встречаем
// старую секцию `dns` (с template_servers/extra_servers/extra_rules), конвертим
// её через `legacyDevDNSToOptions` в новый flat shape in-memory. На ближайшем
// Save файл перезаписывается в новом layout'е. v6 не релизился, конверсия
// lossless — backup не нужен.
//
// **TODO (SPEC 056-R-N): удалить `legacyDevDNSToOptions` после release-cycle**
// когда все dev-state'ы перешли на новый shape.
//
// Для backward-compat UI callsite'ов (DNS tab пока на v5-моделях) генерируется
// legacy CustomRules view (preset-ref пропускается — UI Phase 6 покажет
// через новый dialog).
func parseCurrent(data []byte) (*State, error) {
	var raw struct {
		Meta        MetaSection        `json:"meta"`
		Connections ConnectionsSection `json:"connections"`
		Rules       []Rule             `json:"rules"`
		Vars        []SettingVar       `json:"vars"`
		DNSOptions  DNSOptions         `json:"dns_options"`
		// Legacy dev-shape (SPEC 053). Читаем для одноразовой in-place миграции.
		LegacyDNS json.RawMessage `json:"dns"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("state: parse v6 json: %w", err)
	}

	dnsOpts := raw.DNSOptions
	if dnsOpts.IsEmpty() && len(raw.LegacyDNS) > 0 {
		// Старый dev-shape → конвертим в новый flat layout.
		dnsOpts = legacyDevDNSToOptions(raw.LegacyDNS)
	}

	s := &State{
		Version:            raw.Meta.Version,
		Comment:            raw.Meta.Comment,
		Connections:        raw.Connections,
		Vars:               raw.Vars,
		Rules:              raw.Rules,
		DNS:                dnsOpts,
		RulesLibraryMerged: true,
	}
	if t, err := time.Parse(time.RFC3339, raw.Meta.CreatedAt); err == nil {
		s.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, raw.Meta.UpdatedAt); err == nil {
		s.UpdatedAt = t
	}

	// Generate legacy CustomRules view for backward-compat UI (Phase 6 will use RulesV6 directly).
	s.CustomRules = legacyCustomRulesFromV6(raw.Rules)

	syncLegacyFromConnections(s)
	normalizeNilSlices(s)
	return s, nil
}

// legacyDevDNSToOptions — конверсия старого дев-shape `dns` (SPEC 053:
// template_servers / extra_servers / extra_rules) в новый flat `dns_options`
// (SPEC 056-R-N: servers[]/rules[] через kind discriminator).
//
// Используется только при чтении файлов которые шиппились в HEAD-of-develop
// между SPEC 053 и SPEC 056 — не для шиппнутых юзеров (v6 не релизился).
//
// Преобразование:
//   - dns.template_servers map → servers[] kind=template (tag, enabled)
//   - dns.extra_servers array  → servers[] kind=user (tag, enabled, ...body)
//   - dns.extra_rules array    → rules[] kind=user (enabled=true, ...body)
//   - kind=preset entries создаются позже через SyncDNSOptionsWithActivePresets
//     (см. caller в Load).
func legacyDevDNSToOptions(legacy json.RawMessage) DNSOptions {
	var raw struct {
		Strategy string `json:"strategy"`
		Final    string `json:"final"`
		// SPEC: independent_cache в JSON всё ещё парсим (legacy state read),
		// но в v6 DNSOptions не переносим — sing-box 1.14 deprecation.
		IndependentCache      bool   `json:"independent_cache"`
		DefaultDomainResolver string `json:"default_domain_resolver"`
		TemplateServers       map[string]struct {
			Enabled bool `json:"enabled"`
		} `json:"template_servers"`
		ExtraServers []map[string]interface{} `json:"extra_servers"`
		ExtraRules   []map[string]interface{} `json:"extra_rules"`
	}
	if err := json.Unmarshal(legacy, &raw); err != nil {
		return DNSOptions{}
	}
	_ = raw.IndependentCache // intentionally dropped on migration
	out := DNSOptions{
		Strategy:              raw.Strategy,
		Final:                 raw.Final,
		DefaultDomainResolver: raw.DefaultDomainResolver,
	}
	for tag, ovr := range raw.TemplateServers {
		out.Servers = append(out.Servers, DNSServer{
			Kind:    DNSServerKindTemplate,
			Tag:     tag,
			Enabled: ovr.Enabled,
		})
	}
	for _, body := range raw.ExtraServers {
		tag, _ := body["tag"].(string)
		enabled := true
		if v, ok := body["enabled"].(bool); ok {
			enabled = v
		}
		clean := make(map[string]interface{}, len(body))
		for k, v := range body {
			if k == "enabled" {
				continue
			}
			clean[k] = v
		}
		out.Servers = append(out.Servers, DNSServer{
			Kind:    DNSServerKindUser,
			Tag:     tag,
			Enabled: enabled,
			Body:    clean,
		})
	}
	for _, body := range raw.ExtraRules {
		clean := make(map[string]interface{}, len(body))
		for k, v := range body {
			if k == "enabled" {
				continue
			}
			clean[k] = v
		}
		out.Rules = append(out.Rules, DNSRule{
			Kind:    DNSRuleKindUser,
			Enabled: true,
			Body:    clean,
		})
	}
	return out
}

// legacyCustomRulesFromV6 — конвертирует Rules[] в legacy CustomRule view.
//
// Только kind=inline/srs конвертируются. kind=preset пропускается (не имеет
// сериализованных match-полей — они в template; UI Phase 6 будет работать
// с RulesV6 напрямую через новый edit dialog).
func legacyCustomRulesFromV6(rules []Rule) []CustomRule {
	out := make([]CustomRule, 0, len(rules))
	for _, r := range rules {
		body, err := r.DecodeBody()
		if err != nil {
			continue
		}
		switch r.Kind {
		case RuleKindInline:
			ib := body.(*InlineBody)
			cr := CustomRule{
				Label:            ib.Name,
				Enabled:          r.Enabled,
				SelectedOutbound: ib.Outbound,
				HasOutbound:      true,
				Rule:             cloneMap(ib.Match),
			}
			// Restore outbound в Rule для legacy build-pipeline (он ожидает rule.outbound).
			if cr.Rule == nil {
				cr.Rule = map[string]interface{}{}
			}
			cr.Rule["outbound"] = ib.Outbound
			out = append(out, cr)
		case RuleKindSrs:
			sb := body.(*SrsBody)
			rsRaw, _ := json.Marshal(map[string]interface{}{
				"type":   "remote",
				"format": "binary",
				"url":    sb.SrsURL,
			})
			cr := CustomRule{
				Label:            sb.Name,
				Type:             RuleTypeSRS,
				Enabled:          r.Enabled,
				SelectedOutbound: sb.Outbound,
				HasOutbound:      true,
				RuleSet:          []json.RawMessage{rsRaw},
				Rule: map[string]interface{}{
					"outbound": sb.Outbound,
				},
			}
			out = append(out, cr)
		case RuleKindPreset:
			// preset-ref пропускается в legacy view (UI Phase 6 покажет через новый dialog).
			// При сохранении preset-ref'ы сохраняются обратно в RulesV6 без потери.
		}
	}
	return out
}

// legacyDNSOptionsFromV6 — УДАЛЁНА в SPEC 056-R-N.
//
// Старая функция материализовала v6.DNSConfig template_servers + extras в v5
// DNSOptions view для UI back-compat. Заменена прямым чтением `state.DNS`
// (v6.DNSOptions) — UI рендерит DNS tab из flat servers[]/rules[] напрямую,
// build pipeline читает через ctx.Preset.DNS. Никакого двойного view больше нет.
//
// Если UI код всё ещё ожидает state.DNSOptions — он использует legacy v5 path
// (для backward-compat с v5 файлами). В v6 path `state.DNSOptions` остаётся nil.

// cloneMap — shallow copy of map[string]interface{} for safe legacy view generation.
func cloneMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return nil
	}
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// parseV5Legacy — прямой read v5-формата (legacy). После SPEC 060 Phase 5
// canonical write всегда v6, но v5-файлы юзеров читаются здесь и нормализуются
// в State. На следующем Save перезаписываются в v6 shape.
func parseV5Legacy(data []byte) (*State, error) {
	var raw struct {
		Meta         metaSectionV5       `json:"meta"`
		Connections  ConnectionsSection  `json:"connections"`
		ConfigParams []ConfigParam       `json:"config_params"`
		CustomRules  []CustomRule        `json:"custom_rules"`
		Vars         []SettingVar        `json:"vars"`
		DNSOptions   *LegacyDNSOptionsV5 `json:"dns_options"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("state: parse v5 json: %w", err)
	}

	s := &State{
		Version:      raw.Meta.Version,
		Comment:      raw.Meta.Comment,
		Connections:  raw.Connections,
		ConfigParams: raw.ConfigParams,
		CustomRules:  raw.CustomRules,
		Vars:         raw.Vars,
		DNSOptions:   raw.DNSOptions,
		// Legacy флаги, которые v5 больше не сериализует — выставляем
		// дефолты, удобные UI-коду:
		RulesLibraryMerged: true,
	}
	if t, err := time.Parse(time.RFC3339, raw.Meta.CreatedAt); err == nil {
		s.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, raw.Meta.UpdatedAt); err == nil {
		s.UpdatedAt = t
	}

	// Заполняем legacy proxies-view из Connections для backward-compat
	// callsite'ов (UI source_tab, dashboard counters, parser).
	syncLegacyFromConnections(s)
	normalizeNilSlices(s)
	return s, nil
}

// parseLegacyAndMigrate — v2/v3/v4 → in-memory v5 (с legacy-view заполненной
// для обратной совместимости).
func parseLegacyAndMigrate(data []byte) (*State, error) {
	var raw rawLegacyFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("state: parse legacy json: %w", err)
	}

	// 1. Декодируем parser_config (поддерживает оба формата —
	//    "wrapped" v2 и "simplified" v3+).
	var pc configtypes.ParserConfig
	if err := decodeParserConfig(raw.ParserConfig, &pc); err != nil {
		return nil, err
	}

	// 2. Мигрируем selectable / custom rules как раньше.
	var selectable []SelectableRuleState
	if len(raw.SelectableRuleStates) > 0 {
		selectable = migrateSelectableRuleStates(raw.SelectableRuleStates)
	}
	var custom []CustomRule
	if len(raw.CustomRules) > 0 {
		custom = migrateCustomRules(raw.CustomRules)
	}

	// 3. Собираем v4-snapshot для v5-миграции.
	v4 := &v4File{
		Version:      raw.Version,
		ID:           raw.ID,
		Comment:      raw.Comment,
		CreatedAt:    raw.CreatedAt,
		UpdatedAt:    raw.UpdatedAt,
		ConfigParams: raw.ConfigParams,
		Vars:         raw.Vars,
		CustomRules:  custom,
		DNSOptions:   raw.DNSOptions,
		ParserConfig: v4ParserConfig{
			Version:   pc.ParserConfig.Version,
			Proxies:   pc.ParserConfig.Proxies,
			Outbounds: pc.ParserConfig.Outbounds,
			Parser: v4Parser{
				Reload:      pc.ParserConfig.Parser.Reload,
				LastUpdated: pc.ParserConfig.Parser.LastUpdated,
			},
		},
	}
	migrated := migrateV4ToV5(v4, nil) // production: ULID

	s := &State{
		Version:              SchemaVersion,
		ID:                   raw.ID,
		Comment:              raw.Comment,
		ParserConfig:         pc,
		Connections:          migrated.Connections,
		ConfigParams:         migrated.ConfigParams,
		Vars:                 migrated.Vars,
		SelectableRuleStates: selectable,
		CustomRules:          migrated.CustomRules,
		RulesLibraryMerged:   raw.RulesLibraryMerged,
		DNSOptions:           migrated.DNSOptions,
	}
	if t, err := time.Parse(time.RFC3339, raw.CreatedAt); err == nil {
		s.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, raw.UpdatedAt); err == nil {
		s.UpdatedAt = t
	}
	normalizeNilSlices(s)
	return s, nil
}

// rawLegacyFile — JSON-форма v2/v3/v4 для устойчивого декодирования.
type rawLegacyFile struct {
	Version              int                 `json:"version"`
	ID                   string              `json:"id,omitempty"`
	Comment              string              `json:"comment,omitempty"`
	CreatedAt            string              `json:"created_at"`
	UpdatedAt            string              `json:"updated_at"`
	ParserConfig         json.RawMessage     `json:"parser_config"`
	ConfigParams         []ConfigParam       `json:"config_params"`
	SelectableRuleStates json.RawMessage     `json:"selectable_rule_states"`
	CustomRules          json.RawMessage     `json:"custom_rules"`
	RulesLibraryMerged   bool                `json:"rules_library_merged"`
	DNSOptions           *LegacyDNSOptionsV5 `json:"dns_options"`
	Vars                 []SettingVar        `json:"vars"`
}

// decodeParserConfig — поддерживает два on-disk формата:
//
//  1. Упрощённый (v3+): {version, proxies, outbounds, parser}.
//  2. Старый (v2 и ранее): обёрнутый {"ParserConfig":{…}}.
func decodeParserConfig(raw json.RawMessage, dst *configtypes.ParserConfig) error {
	if len(raw) == 0 {
		return nil
	}

	var simplified struct {
		Version   int                          `json:"version"`
		Proxies   []configtypes.ProxySource    `json:"proxies"`
		Outbounds []configtypes.OutboundConfig `json:"outbounds"`
		Parser    struct {
			Reload      string `json:"reload,omitempty"`
			LastUpdated string `json:"last_updated,omitempty"`
		} `json:"parser,omitempty"`
	}
	if err := json.Unmarshal(raw, &simplified); err == nil && simplified.Proxies != nil {
		dst.ParserConfig.Version = simplified.Version
		dst.ParserConfig.Proxies = simplified.Proxies
		dst.ParserConfig.Outbounds = simplified.Outbounds
		dst.ParserConfig.Parser = simplified.Parser
		return nil
	}

	var legacy configtypes.ParserConfig
	if err := json.Unmarshal(raw, &legacy); err == nil {
		*dst = legacy
		return nil
	}

	return fmt.Errorf("state: parser_config: unsupported shape")
}

// migrateSelectableRuleStates — перенос как новых snapshots, так и старых
// (v2-эпоха) с вложенным rule.label.
func migrateSelectableRuleStates(raw json.RawMessage) []SelectableRuleState {
	var modern []SelectableRuleState
	if err := json.Unmarshal(raw, &modern); err == nil && (len(modern) == 0 || modern[0].Label != "") {
		return modern
	}

	var legacy []struct {
		Enabled          bool   `json:"enabled"`
		SelectedOutbound string `json:"selected_outbound"`
		Rule             struct {
			Label string `json:"label"`
		} `json:"rule"`
	}
	if err := json.Unmarshal(raw, &legacy); err == nil {
		out := make([]SelectableRuleState, 0, len(legacy))
		for _, x := range legacy {
			if x.Rule.Label == "" {
				continue
			}
			out = append(out, SelectableRuleState{
				Label:            x.Rule.Label,
				Enabled:          x.Enabled,
				SelectedOutbound: x.SelectedOutbound,
			})
		}
		return out
	}

	return nil
}

// migrateCustomRules — аналогично, для custom_rules.
func migrateCustomRules(raw json.RawMessage) []CustomRule {
	var modern []CustomRule
	if err := json.Unmarshal(raw, &modern); err == nil && (len(modern) == 0 || modern[0].Label != "") {
		return modern
	}

	var legacy []struct {
		Type             string `json:"type"`
		Enabled          bool   `json:"enabled"`
		SelectedOutbound string `json:"selected_outbound"`
		Rule             struct {
			Label           string                 `json:"label"`
			Description     string                 `json:"description"`
			Raw             map[string]interface{} `json:"raw"`
			DefaultOutbound string                 `json:"default_outbound"`
			HasOutbound     bool                   `json:"has_outbound"`
		} `json:"rule"`
	}
	if err := json.Unmarshal(raw, &legacy); err == nil {
		out := make([]CustomRule, 0, len(legacy))
		for _, x := range legacy {
			out = append(out, CustomRule{
				Label:            x.Rule.Label,
				Type:             x.Type,
				Enabled:          x.Enabled,
				SelectedOutbound: x.SelectedOutbound,
				Description:      x.Rule.Description,
				Rule:             x.Rule.Raw,
				DefaultOutbound:  x.Rule.DefaultOutbound,
				HasOutbound:      x.Rule.HasOutbound,
			})
		}
		return out
	}

	return nil
}

// normalizeNilSlices — приводим nil-slices к пустым для удобства callsite'ов.
func normalizeNilSlices(s *State) {
	if s.ConfigParams == nil {
		s.ConfigParams = []ConfigParam{}
	}
	if s.CustomRules == nil {
		s.CustomRules = []CustomRule{}
	}
	if s.Connections.Sources == nil {
		s.Connections.Sources = []Source{}
	}
	sanitizeOutboundRefs(&s.Connections.Outbounds)
	for i := range s.Connections.Sources {
		sanitizeOutboundRefs(&s.Connections.Sources[i].Outbounds)
	}
}

// sanitizeOutboundRefs валидирует позиционные правила для `ref` полей в outbound
// entries (SPEC 058-R-N). Лениво — дропает невалидные entries / updates и логирует
// в stderr, вместо fail-load. Это безопаснее против hand-edited state.json и
// forward-compat с будущими sentinel'ами.
//
// Правила:
//   - `outbounds[].ref`: принимаем "" (direct), RefTemplate (#TEMPLATE#), либо
//     любое непустое значение НЕ начинающееся на `#` (preset_id). Reject RefUser
//     и unknown #...# sentinel'ы.
//   - `outbounds[].updates[].ref`: принимаем RefUser (#USER#) или любое
//     непустое значение НЕ начинающееся на `#` (preset_id). Reject "",
//     RefTemplate, unknown #...# sentinel'ы.
func sanitizeOutboundRefs(outbounds *[]configtypes.OutboundConfig) {
	if outbounds == nil || *outbounds == nil {
		return
	}
	cleaned := (*outbounds)[:0]
	for _, ob := range *outbounds {
		if !validEntryRef(ob.Ref) {
			log.Printf("state: dropping outbound %q with invalid ref=%q (sentinel rules SPEC 058)", ob.Tag, ob.Ref)
			continue
		}
		if len(ob.Updates) > 0 {
			validUpdates := ob.Updates[:0]
			for _, u := range ob.Updates {
				if !validUpdateRef(u.Ref) {
					log.Printf("state: dropping update on outbound %q with invalid ref=%q", ob.Tag, u.Ref)
					continue
				}
				validUpdates = append(validUpdates, u)
			}
			ob.Updates = validUpdates
		}
		cleaned = append(cleaned, ob)
	}
	*outbounds = cleaned
}

// validEntryRef — допустимые значения state.outbounds[].ref:
// "", RefTemplate, или любая non-#-prefixed строка (preset_id).
func validEntryRef(ref string) bool {
	if ref == "" || ref == configtypes.RefTemplate {
		return true
	}
	if strings.HasPrefix(ref, "#") {
		// #USER# (patch-level only) или unknown sentinel
		return false
	}
	return true // preset_id; validation regex живёт в template loader
}

// validUpdateRef — допустимые значения state.outbounds[].updates[].ref:
// RefUser или любая non-#-prefixed строка (preset_id).
func validUpdateRef(ref string) bool {
	if ref == configtypes.RefUser {
		return true
	}
	if ref == "" || strings.HasPrefix(ref, "#") {
		return false
	}
	return true
}
