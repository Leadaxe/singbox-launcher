// File legacy_migration.go — v4 → v5 migration (private helper).
//
// Используется только Parse при чтении legacy v2/v3/v4 файлов. Результат
// migrateV4ToV5 — приватный diskStateV5; вызывающий разворачивает его в
// канонический State.
package state

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

// diskStateV5 — promoted private shape v5 (использовался ранее как v5.State).
// Поля идентичны top-level layout'у v5 файла. После SPEC 060 collapse доступен
// только внутри package state как промежуточная форма для migrateV4ToV5.
type diskStateV5 struct {
	Meta         metaSectionV5       `json:"meta"`
	Connections  ConnectionsSection  `json:"connections"`
	ConfigParams []ConfigParam       `json:"config_params"`
	CustomRules  []CustomRule        `json:"custom_rules"`
	Vars         []SettingVar        `json:"vars,omitempty"`
	DNSOptions   *LegacyDNSOptionsV5 `json:"dns_options"`
}

// metaSectionV5 — мета v5 (без поля Schema, в отличие от canonical MetaSection v6).
type metaSectionV5 struct {
	Version   int    `json:"version"`
	Comment   string `json:"comment,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// migrateV4ToV5 преобразует v4-форму state.json в v5.
//
// Pure-функция, детерминирована относительно (input, gen). Если gen=nil,
// используется MakeULID (недетерминированный — для production-flow).
func migrateV4ToV5(old *v4File, gen IDGenerator) *diskStateV5 {
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

	out := &diskStateV5{
		Meta: metaSectionV5{
			Version:   legacySchemaVersionV5,
			Comment:   old.Comment,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		},
		Connections: ConnectionsSection{
			Sources:   migrateLegacySources(old.ParserConfig.Proxies, gen),
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

// migrateLegacySources разворачивает legacy ProxySource[] в новые Source[].
// Один input может породить несколько output'ов (mixed source+connections).
func migrateLegacySources(proxies []configtypes.ProxySource, gen IDGenerator) []Source {
	out := make([]Source, 0, len(proxies))
	for _, ps := range proxies {
		// 1. type=subscription (если задан source URL)
		if ps.Source != "" {
			tag := buildLegacyTagSpec(ps.TagPrefix, ps.TagPostfix, ps.TagMask)
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
			label := legacyServerLabel(uri, j+1, ps.TagPrefix, ps.TagPostfix)
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

// buildLegacyTagSpec возвращает *TagSpec (или nil если все три поля пустые).
func buildLegacyTagSpec(prefix, postfix, mask string) *TagSpec {
	if prefix == "" && postfix == "" && mask == "" {
		return nil
	}
	return &TagSpec{
		Prefix:  prefix,
		Postfix: postfix,
		Mask:    mask,
	}
}

// legacyServerLabel формирует label для Source(server) из:
//   - URI fragment (после #) — это «человекочитаемое имя» от провайдера;
//   - tag_prefix/tag_postfix — из v4 ProxySource (применялись на parsed
//     node tag; чтобы сохранить визуальную идентичность config.json
//     после миграции, переносим их в label).
//
// Если fragment пуст — fallback "server-N" (1-based).
func legacyServerLabel(uri string, oneBasedIndex int, tagPrefix, tagPostfix string) string {
	base := extractLegacyFragment(uri)
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

// extractLegacyFragment вытаскивает URL fragment (после #) и percent-decoded'ит.
// Не паникует на malformed URI — возвращает "".
func extractLegacyFragment(s string) string {
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
