package state

import (
	"encoding/json"
	"fmt"
	"time"

	"singbox-launcher/internal/srstag"
)

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
//
// SPEC 118: v6 — легаси-формат. Секция connections читается в приватную
// v6-форму и переносится в плоский v7-корень СТРУКТУРНО (adoptConnectionsV6);
// легаси-значения уезжают сайдкаром legacySourceV6, который читает только
// миграция. Поверх переноса работает семантическая миграция
// (migrateLegacyStateToV7 — 8 шагов features/state.md: материализация
// nodes[], отметки, теги, хопы, тройня, fold→replace, отчёт потерь); снос
// легаси (шаг 8) идёт после успешной записи v7-файла.
func parseV6Legacy(data []byte, lc LoadContext) (*State, error) {
	var raw struct {
		Meta         MetaSection          `json:"meta"`
		Connections  ConnectionsSection   `json:"connections"`
		Rules        []Rule               `json:"rules"`
		Vars         []SettingVar         `json:"vars"`
		DNSOptions   DNSOptions           `json:"dns_options"`
		WarpAccounts *WarpAccountsSection `json:"warp_accounts"`
		// Legacy dev-shape (SPEC 053). Читаем для одноразовой in-place миграции.
		LegacyDNS json.RawMessage `json:"dns"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("state: parse v6 json: %w", err)
	}

	raw.Connections.adoptLegacyDirections()

	dnsOpts := raw.DNSOptions
	if dnsOpts.IsEmpty() && len(raw.LegacyDNS) > 0 {
		// Старый dev-shape → конвертим в новый flat layout.
		dnsOpts = legacyDevDNSToOptions(raw.LegacyDNS)
	}

	s := &State{
		Version:            raw.Meta.Version,
		Comment:            raw.Meta.Comment,
		Target:             raw.Meta.Target,
		TargetPlatform:     raw.Meta.TargetPlatform,
		TargetArch:         raw.Meta.TargetArch,
		Vars:               raw.Vars,
		Rules:              raw.Rules,
		DNS:                dnsOpts,
		WarpAccounts:       raw.WarpAccounts,
		RulesLibraryMerged: true,
	}
	if t, err := time.Parse(time.RFC3339, raw.Meta.CreatedAt); err == nil {
		s.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, raw.Meta.UpdatedAt); err == nil {
		s.UpdatedAt = t
	}

	// SPEC 108: прежние флаги подписки → Fold. Строго ДО переноса в v7 и
	// до построения legacy-вида: и то и другое читает уже мигрированное.
	migrateSourceFolds(&raw.Connections)

	// SPEC 118 (W1): структурный перенос v6-секции в плоский v7-корень.
	legacySources := adoptConnectionsV6(s, raw.Connections)

	// SPEC 118 (W2): семантическая миграция — СТРОГО до построения
	// legacy-видов (CustomRules и проекция обязаны видеть уже переписанные
	// ссылки и мигрированный канон).
	migrateLegacyStateToV7(s, 6, lc, legacySources)

	// Generate legacy CustomRules view for backward-compat UI (Phase 6 will use RulesV6 directly).
	s.CustomRules = legacyCustomRulesFromV6(s.Rules)

	syncLegacyFromCanonical(s)
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
			// Тег записи — content-addressed имя файла в bin/rule-sets/, тот же,
			// что даёт диалог правила и downloader. Без него запись невидима
			// для UI (GetSRSEntries отбрасывает записи без tag — кнопка
			// «✔️ srs» и гейт скачивания пропадали после переоткрытия) и
			// валит сборку «Итога»: MergeRouteSection отвечает «remote entry
			// missing tag» (репорт 1.5.4).
			rsRaw, _ := json.Marshal(map[string]interface{}{
				"tag":    srstag.TagFromURL(sb.SrsURL),
				"type":   "remote",
				"format": "binary",
				"url":    sb.SrsURL,
			})
			// SPEC 063 follow-up: edit dialog re-derives rule type через
			// DetermineRuleType(cr.Rule) и игнорирует stored cr.Type. Чтобы
			// SRS rule после re-open не превратилось в "Custom JSON",
			// в cr.Rule кладём rule_set placeholder с identity-based тегом
			// (тот же, что эмитит build/rules_pipeline.go: "user:" + StableRuleID).
			// Это чисто UI-hint — build path не использует cr.Rule для SRS,
			// он берёт state.Rules напрямую.
			tag := "user:" + StableRuleID(r)
			cr := CustomRule{
				Label:            sb.Name,
				Type:             RuleTypeSRS,
				Enabled:          r.Enabled,
				SelectedOutbound: sb.Outbound,
				HasOutbound:      true,
				RuleSet:          []json.RawMessage{rsRaw},
				Rule: map[string]interface{}{
					"rule_set": tag,
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
