package v5

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"singbox-launcher/core/config/configtypes"
)

// IDGenerator — функция генерации Source.ID. Production использует
// MakeULID; тесты подменяют детерминированным счётчиком для golden-тестов.
type IDGenerator func() string

// MigrateV4ToV5 преобразует v4-форму state.json в v5.
//
// Pure-функция, детерминирована относительно (input, gen). Если gen=nil,
// используется MakeULID (недетерминированный — для production-flow).
//
// Семантика:
//
//   - top-level meta заполняется из old.{Version=5, Comment, CreatedAt, UpdatedAt}
//   - config_params, custom_rules, vars, dns_options — копируются как есть
//   - rules_library_merged — выпилено (v5 не использует флаг)
//   - selectable_rule_states — выпилено
//   - id (top-level) — выпилен (snapshot-имена живут в имени файла)
//   - parser_config.parser.last_updated — выпилен (per-source meta теперь)
//
//   - parser_config.proxies[i] разворачивается в один или более Source:
//     • если old.source != "" и connections пустой → один Source(subscription)
//     • если len(connections) > 0 → один Source(server) на каждый URI
//     • если оба заданы → emit оба варианта (mixed legacy)
//
// MaxNodes (per-source) НЕ копируется из v4 (поля не было); 0 = inherit
// from Defaults.MaxNodes (DefaultMaxNodes=3000).
func MigrateV4ToV5(old *V4File, gen IDGenerator) *State {
	if old == nil {
		return nil
	}
	if gen == nil {
		gen = MakeULID
	}

	now := time.Now().UTC().Format(time.RFC3339)
	createdAt := old.CreatedAt
	if createdAt == "" {
		createdAt = now
	}
	updatedAt := old.UpdatedAt
	if updatedAt == "" {
		updatedAt = now
	}

	out := &State{
		Meta: MetaSection{
			Version:   SchemaVersion,
			Comment:   old.Comment,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		},
		Connections: ConnectionsSection{
			Sources:   migrateSources(old.ParserConfig.Proxies, gen),
			Outbounds: append([]configtypes.OutboundConfig(nil), old.ParserConfig.Outbounds...),
			Defaults: Defaults{
				Reload:   old.ParserConfig.Parser.Reload,
				MaxNodes: DefaultMaxNodes,
			},
		},
		ConfigParams: cloneConfigParams(old.ConfigParams),
		CustomRules:  cloneCustomRules(old.CustomRules),
		Vars:         cloneSettingVars(old.Vars),
		DNSOptions:   old.DNSOptions, // shallow-copy ок, миграция не мутирует
	}

	if out.ConfigParams == nil {
		out.ConfigParams = []ConfigParam{}
	}
	if out.CustomRules == nil {
		out.CustomRules = []CustomRule{}
	}
	if out.Connections.Sources == nil {
		out.Connections.Sources = []Source{}
	}
	if out.Connections.Outbounds == nil {
		out.Connections.Outbounds = nil
	}
	return out
}

// migrateSources разворачивает legacy ProxySource[] в новые Source[].
// Один input может породить несколько output'ов (mixed source+connections).
func migrateSources(proxies []configtypes.ProxySource, gen IDGenerator) []Source {
	out := make([]Source, 0, len(proxies))
	for _, ps := range proxies {
		// 1. type=subscription (если задан source URL)
		if ps.Source != "" {
			tag := buildTagSpec(ps.TagPrefix, ps.TagPostfix, ps.TagMask)
			s := Source{
				ID:                      gen(),
				Type:                    SourceTypeSubscription,
				Enabled:                 !ps.Disabled,
				URL:                     ps.Source,
				Skip:                    ps.Skip,
				Tag:                     tag,
				Outbounds:               ps.Outbounds,
				ExcludeFromGlobal:       ps.ExcludeFromGlobal,
				ExposeGroupTagsToGlobal: ps.ExposeGroupTagsToGlobal,
			}
			out = append(out, s)
		}

		// 2. type=server (один Source per URI в connections[])
		for j, uri := range ps.Connections {
			label := serverLabel(uri, j+1, ps.TagPrefix, ps.TagPostfix)
			s := Source{
				ID:                gen(),
				Type:              SourceTypeServer,
				Enabled:           !ps.Disabled,
				Label:             label,
				URI:               uri,
				ExcludeFromGlobal: ps.ExcludeFromGlobal,
			}
			out = append(out, s)
		}
	}
	return out
}

// buildTagSpec возвращает *TagSpec (или nil если все три поля пустые).
func buildTagSpec(prefix, postfix, mask string) *TagSpec {
	if prefix == "" && postfix == "" && mask == "" {
		return nil
	}
	return &TagSpec{
		Prefix:  prefix,
		Postfix: postfix,
		Mask:    mask,
	}
}

// serverLabel формирует label для Source(server) из:
//   - URI fragment (после #) — это «человекочитаемое имя» от провайдера;
//   - tag_prefix/tag_postfix — из v4 ProxySource (применялись на parsed
//     node tag; чтобы сохранить визуальную идентичность config.json
//     после миграции, переносим их в label).
//
// Если fragment пуст — fallback "server-N" (1-based).
//
// Variables substitution ({$tag}, {$server} ...) НЕ выполняется при
// миграции — только plain concat. Кейс с переменными в tag_prefix
// для connections-source встречается крайне редко.
func serverLabel(uri string, oneBasedIndex int, tagPrefix, tagPostfix string) string {
	base := extractFragment(uri)
	if base == "" {
		base = fmt.Sprintf("server-%d", oneBasedIndex)
	}
	if !strings.Contains(tagPrefix, "{$") {
		base = tagPrefix + base
	}
	if !strings.Contains(tagPostfix, "{$") {
		base = base + tagPostfix
	}
	return base
}

// extractFragment вытаскивает URL fragment (после #) и percent-decoded'ит.
// Не паникует на malformed URI — возвращает "".
func extractFragment(s string) string {
	hashAt := strings.Index(s, "#")
	if hashAt < 0 {
		return ""
	}
	frag := s[hashAt+1:]
	if frag == "" {
		return ""
	}
	if dec, err := url.QueryUnescape(frag); err == nil {
		return dec
	}
	return frag
}

// cloneConfigParams делает поверхностную копию (структуры — value-types).
func cloneConfigParams(in []ConfigParam) []ConfigParam {
	if in == nil {
		return nil
	}
	out := make([]ConfigParam, len(in))
	copy(out, in)
	return out
}

// cloneSettingVars — аналогично.
func cloneSettingVars(in []SettingVar) []SettingVar {
	if in == nil {
		return nil
	}
	out := make([]SettingVar, len(in))
	copy(out, in)
	return out
}

// cloneCustomRules — shallow-copy на уровне map'ов (Rule/Params/RuleSet
// не клонируем — миграция read-only).
func cloneCustomRules(in []CustomRule) []CustomRule {
	if in == nil {
		return nil
	}
	out := make([]CustomRule, len(in))
	copy(out, in)
	return out
}
