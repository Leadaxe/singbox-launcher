// Package build — see route_merge.go for legacy merge of CustomRules.
//
// File preset_merge.go (SPEC 053) — дополнительный pass поверх MergeRouteSection,
// который append'ит fragments от active preset-ref правил в уже-смержанную
// route-секцию. Не трогает legacy путь — старые CustomRules продолжают идти
// через MergeRouteSection как раньше.
//
// Также подключает state.dns.template_servers overrides + bundled DNS-серверы
// от active presets в финальный dns.servers/dns.rules.
//
// Активируется когда state.Rules содержит хотя бы один preset-ref. Пустой
// список → noop (state.json остаётся в v5, поведение неизменно).
package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// Local stdlib wrappers used by srsTagFromURLLocal / convertPresetRuleSetRemoteToLocal.
// (Inline aliases — избавляют функции от длинных fully-qualified имен.)
func osStatLocal(p string) (os.FileInfo, error) { return os.Stat(p) }
func urlParseLocal(s string) (*url.URL, error)  { return url.Parse(s) }
func sha256SumLocal(b []byte) [sha256.Size]byte { return sha256.Sum256(b) }

// convertPresetRuleSetRemoteToLocal — резолвит remote rule_set из preset'а в
// type=local с path к скачанному файлу.
//
// Логика:
//   - Если type != "remote" → пробрасываем как есть (inline остаётся inline).
//   - Если type == "remote" + url — генерим content-addressed tag через
//     services.SRSTagFromURL, проверяем bin/rule-sets/<tag>.srs.
//   - Файл скачан → эмитим {tag (с preset-prefix), type:local, format, path}.
//   - Файла нет → возвращаем skip=true (caller пропустит и dangling-ref
//     cleanup уберёт ссылку из rule.rule_set).
//
// preset-prefix tag (`<preset_id>:<local_tag>`) уже задан в ExpandPreset и
// сохраняется — мы только меняем type/path/strip url.
func convertPresetRuleSetRemoteToLocal(rs map[string]interface{}, execDir string) (map[string]interface{}, bool) {
	typ, _ := rs["type"].(string)
	if typ != "remote" {
		return rs, false
	}
	url, _ := rs["url"].(string)
	if url == "" || execDir == "" {
		return rs, true // skip — нет данных для resolve
	}
	contentTag := srsTagFromURLLocal(url)
	if contentTag == "" {
		return rs, true
	}
	path := execDir + "/bin/rule-sets/" + contentTag + ".srs"
	if _, err := osStatLocal(path); err != nil {
		return rs, true // файл не скачан → skip
	}
	// Mutate copy: type=local, добавить path, удалить url/download_detour/update_interval.
	out := make(map[string]interface{}, len(rs))
	for k, v := range rs {
		switch k {
		case "url", "download_detour", "update_interval":
			// drop — это remote-specific поля
		case "type":
			out[k] = "local"
		default:
			out[k] = v
		}
	}
	out["path"] = path
	if _, has := out["format"]; !has {
		out["format"] = "binary"
	}
	return out, false
}

// cleanDanglingRuleSetInRule — для rule с `rule_set` ссылкой удаляет имена
// которых нет в emittedTags. Если после уборки массив пуст или строка ссылается
// на отсутствующий tag — rule отбрасывается (возвращает nil), если есть хоть
// один валидный ref — оставляет.
//
// Используется когда remote rule_set не скачан и пропущен — соответствующая
// ссылка в rule.rule_set должна исчезнуть, чтобы sing-box не упал на unknown tag.
func cleanDanglingRuleSetInRule(rule map[string]interface{}, emittedTags map[string]bool) map[string]interface{} {
	if rule == nil {
		return nil
	}
	ref, ok := rule["rule_set"]
	if !ok {
		return rule // нет rule_set → ничего убирать
	}
	out := make(map[string]interface{}, len(rule))
	for k, v := range rule {
		out[k] = v
	}
	switch v := ref.(type) {
	case string:
		if !emittedTags[v] {
			delete(out, "rule_set")
		}
	case []interface{}:
		kept := make([]interface{}, 0, len(v))
		for _, x := range v {
			if s, ok := x.(string); ok && emittedTags[s] {
				kept = append(kept, s)
			}
		}
		if len(kept) == 0 {
			delete(out, "rule_set")
		} else {
			out["rule_set"] = kept
		}
	}
	// Если после очистки rule остался только outbound/action (без rule_set
	// и без других match-полей) — rule пустой, не эмитим.
	hasMatchFields := false
	for k := range out {
		switch k {
		case "outbound", "action", "method", "if", "if_or":
			continue
		default:
			hasMatchFields = true
		}
	}
	if !hasMatchFields {
		return nil
	}
	return out
}

// SRSTagFromURL — content-addressed tag (same logic as
// ui/configurator/dialogs.SRSTagFromURL — продублировано тут чтобы избежать
// импорта UI пакета в core). Используется как core/build, так и core (orphan
// GC через collectAllStageRuleSetTags). Должно быть вынесено в internal/srstag/
// если будет ещё одна копия.
func SRSTagFromURL(urlStr string) string { return srsTagFromURLLocal(urlStr) }

func srsTagFromURLLocal(urlStr string) string {
	u, err := urlParseLocal(urlStr)
	if err != nil {
		return ""
	}
	path := u.Path
	if path == "" {
		path = urlStr
	}
	if i := strings.LastIndex(path, "/"); i >= 0 {
		path = path[i+1:]
	}
	filename := strings.TrimSuffix(path, ".srs")
	if filename == "" {
		filename = "srs"
	}
	sum := sha256SumLocal([]byte(urlStr))
	hash8 := hex.EncodeToString(sum[:4])
	return filename + "-" + hash8
}

// PresetMergeContext — input для MergePresetsIntoRoute/DNS.
type PresetMergeContext struct {
	Presets        []template.Preset
	Rules          []state.Rule
	DNS            state.DNSOptions
	SrsCachedPaths map[string]string

	// ExecDir — для резолва local SRS paths (kind=srs / preset remote rule_set).
	ExecDir string

	// TemplateDNSDefaults — раскрытые dns_defaults.servers[] из template.
	// Используется для materialization template-серверов с применением
	// effective_enabled (от state.dns_options.servers[kind=template]).
	TemplateDNSDefaults []TemplateDNSServer
}

// MergePresetsIntoRoute — единый emit-путь route через ResolveRoute()
// (SPEC 056-R-N follow-up). Симметрично MergePresetsIntoDNS.
//
// Алгоритм: ResolveRoute → filter Enabled (Active всегда true для inline/srs;
// для preset уже отфильтровано ExpandPreset через if/if_or) → merge с уже
// эмитнутыми из template (dedup по tag).
//
// Skipped rule_sets (remote .srs не cached) пропускаются; dangling rule_set
// refs в routing rule уже очищены resolver'ом через cleanDanglingRuleSetInRule.
func MergePresetsIntoRoute(routeRaw json.RawMessage, ctx PresetMergeContext) (json.RawMessage, error) {
	if !hasAnyV6Rule(ctx.Rules) {
		return routeRaw, nil
	}

	var route map[string]interface{}
	if err := json.Unmarshal(routeRaw, &route); err != nil {
		return nil, fmt.Errorf("preset merge: parse route: %w", err)
	}

	rules, _ := route["rules"].([]interface{})
	ruleSets, _ := route["rule_set"].([]interface{})

	st := &state.State{Rules: ctx.Rules, DNS: ctx.DNS}
	tdVal := template.TemplateData{Presets: ctx.Presets}
	resolved := ResolveRoute(st, &tdVal, ctx.ExecDir, ctx.SrsCachedPaths)

	// Dedup по tag (template уже мог эмитить rule_sets).
	emittedTags := make(map[string]bool)
	for _, rs := range ruleSets {
		if m, ok := rs.(map[string]interface{}); ok {
			if tag, ok := m["tag"].(string); ok {
				emittedTags[tag] = true
			}
		}
	}

	// Emit rule_sets: skip Skipped, dedup.
	for _, rs := range resolved.RuleSets {
		if rs.Skipped {
			continue
		}
		if emittedTags[rs.Tag] {
			continue
		}
		ruleSets = append(ruleSets, rs.Body)
		emittedTags[rs.Tag] = true
	}

	// Emit rules: Active && Enabled. (Active=true для inline/srs always;
	// для preset — уже отфильтрован в ResolveRoute.)
	for _, r := range resolved.Rules {
		if !r.Active || !r.Enabled {
			continue
		}
		rules = append(rules, r.Body)
	}

	if len(rules) > 0 {
		route["rules"] = rules
	}
	if len(ruleSets) > 0 {
		route["rule_set"] = ruleSets
	}

	out, err := json.MarshalIndent(route, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("preset merge: marshal route: %w", err)
	}
	return out, nil
}

// MergePresetsIntoDNS — единый emit-путь DNS через ResolveDNS() (SPEC 056-R-N
// follow-up).
//
// Алгоритм: ResolveDNS → filter (Active && Enabled) → strip wizard fields →
// merge с уже эмитнутыми из template (dedup по tag).
//
// Если ResolveDNS вернёт пустой результат И в template dns nothing — noop.
//
// Dangling rule_set refs в kind=user dns rules чистятся через
// cleanDanglingDNSRule.
func MergePresetsIntoDNS(dnsRaw json.RawMessage, ctx PresetMergeContext) (json.RawMessage, error) {
	// ResolveDNS — единая точка резолва. Принимает state-like контекст
	// (RulesV6 + DNS), строит ResolvedDNS на лету.
	st := &state.State{Rules: ctx.Rules, DNS: ctx.DNS}
	tdVal := templateLikeFromCtx(ctx)
	resolved := ResolveDNS(st, &tdVal, nil)

	if len(resolved.Servers) == 0 && len(resolved.Rules) == 0 && !hasAnyV6Rule(ctx.Rules) {
		return dnsRaw, nil
	}

	var dns map[string]interface{}
	if len(dnsRaw) > 0 {
		_ = json.Unmarshal(dnsRaw, &dns)
	}
	if dns == nil {
		dns = make(map[string]interface{})
	}

	servers, _ := dns["servers"].([]interface{})
	dnsRules, _ := dns["rules"].([]interface{})

	// Dedup по tag (template уже эмитнул local_dns_resolver/direct_dns_resolver).
	emittedTags := make(map[string]bool)
	for _, s := range servers {
		if m, ok := s.(map[string]interface{}); ok {
			if tag, ok := m["tag"].(string); ok {
				emittedTags[tag] = true
			}
		}
	}

	// Emit servers: Active && Enabled, skip Source=Core (уже в template).
	for _, srv := range resolved.Servers {
		if !srv.Active || !srv.Enabled {
			continue
		}
		// (SPEC unify: больше нет CORE источника — required entries имеют
		// Source=template + Locked=true; они эмитятся как и любые template.)
		if srv.Tag != "" && emittedTags[srv.Tag] {
			continue
		}
		servers = append(servers, srv.Body)
		if srv.Tag != "" {
			emittedTags[srv.Tag] = true
		}
	}

	// Build emittedRuleSetTags для dangling-cleanup в DNS user rules.
	presetByID := make(map[string]*template.Preset, len(ctx.Presets))
	for i := range ctx.Presets {
		presetByID[ctx.Presets[i].ID] = &ctx.Presets[i]
	}
	emittedRuleSetTags := collectRuleSetTagsFromPresets(presetByID, ctx.Rules)

	// Emit rules: Active && Enabled. User → dangling cleanup; preset → as is.
	for _, dr := range resolved.Rules {
		if !dr.Active || !dr.Enabled {
			continue
		}
		switch dr.Source {
		case DNSSourcePreset:
			dnsRules = append(dnsRules, dr.Body)
		case DNSSourceUser:
			cleaned := cleanDanglingDNSRule(dr.Body, emittedRuleSetTags)
			if cleaned == nil {
				continue
			}
			dnsRules = append(dnsRules, cleaned)
		}
	}

	if len(servers) > 0 {
		dns["servers"] = servers
	}
	if len(dnsRules) > 0 {
		dns["rules"] = dnsRules
	}

	out, err := json.MarshalIndent(dns, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("preset merge dns: marshal: %w", err)
	}
	return out, nil
}

// templateLikeFromCtx — собирает временный TemplateData из PresetMergeContext
// для передачи в ResolveDNS. ctx содержит presets и (через TemplateDNSDefaults)
// раскрытые template DNS-серверы, но не config.dns.servers (core).
// Core минимум доступен через сам caller (он уже в dnsRaw).
func templateLikeFromCtx(ctx PresetMergeContext) template.TemplateData {
	// Восстанавливаем DNSOptionsRaw как JSON массив из ctx.TemplateDNSDefaults
	// (чтобы ResolveDNS мог пройтись по template library).
	var libRaw []map[string]interface{}
	for _, d := range ctx.TemplateDNSDefaults {
		libRaw = append(libRaw, d.Raw)
	}
	dnsOpt := map[string]interface{}{"servers": libRaw}
	raw, _ := json.Marshal(dnsOpt)
	td := template.TemplateData{
		Presets:       ctx.Presets,
		DNSOptionsRaw: raw,
	}
	return td
}

// collectRuleSetTagsFromPresets — set rule_set tag'ов от ВСЕХ enabled
// preset-ref'ов (после auto-prefix `<preset_id>:<local_tag>`).
//
// Используется в DNS rules dangling-cleanup: extra_rule с `rule_set` ссылкой
// должен матчиться с реально-эмитнутым rule_set tag'ом. Иначе sing-box упадёт
// на `start service: initialize DNS rule[N]: rule-set not found: <X>`.
func collectRuleSetTagsFromPresets(presetByID map[string]*template.Preset, rules []state.Rule) map[string]bool {
	tags := make(map[string]bool)
	for _, rule := range rules {
		if !rule.Enabled || rule.Kind != state.RuleKindPreset {
			continue
		}
		preset, ok := presetByID[rule.Ref]
		if !ok {
			continue
		}
		body, err := rule.DecodeBody()
		if err != nil {
			continue
		}
		pb := body.(*state.PresetBody)
		frags, _, ok := ExpandPreset(preset, pb.Vars)
		if !ok {
			continue
		}
		for _, rs := range frags.RuleSets {
			if t, _ := rs["tag"].(string); t != "" {
				tags[t] = true
			}
		}
	}
	return tags
}

// cleanDanglingDNSRule — для DNS rule entry: проверяет `rule_set` ссылки
// против validTags. Возвращает clean copy или nil (если rule станет пустым
// после очистки и его надо drop'нуть).
//
// Семантика (зеркало cleanDanglingRuleSetInRule для route):
//   - rule БЕЗ `rule_set` ссылки → keep (server-only rule валидно)
//   - `rule_set` string ∈ validTags → keep
//   - `rule_set` string ∉ validTags → удалить ключ; keep rule если остался
//     хотя бы один match-источник (server, domain*, ip_cidr, port, и т.п.)
//   - `rule_set` массив → filter dangling; пустой → удалить ключ
//   - rule без match-источников → drop entry целиком
//
// Pure: новый map, оригинал не мутируется.
func cleanDanglingDNSRule(rule map[string]interface{}, validTags map[string]bool) map[string]interface{} {
	if rule == nil {
		return nil
	}
	out := make(map[string]interface{}, len(rule))
	for k, v := range rule {
		out[k] = v
	}

	if ref, has := out["rule_set"]; has {
		switch v := ref.(type) {
		case string:
			if v != "" && !validTags[v] {
				delete(out, "rule_set")
			}
		case []interface{}:
			kept := make([]interface{}, 0, len(v))
			for _, x := range v {
				if s, ok := x.(string); ok && validTags[s] {
					kept = append(kept, s)
				}
			}
			if len(kept) == 0 {
				delete(out, "rule_set")
			} else {
				out["rule_set"] = kept
			}
		}
	}

	hasMatch := false
	for k := range out {
		switch k {
		case "server", "rule_set", "domain", "domain_suffix", "domain_keyword",
			"domain_regex", "ip_cidr", "source_ip_cidr", "port", "source_port",
			"network", "protocol", "inbound", "outbound", "process_name",
			"process_path", "process_path_regex", "rule_set_ip_cidr_match_source",
			"query_type", "client_subnet":
			hasMatch = true
		}
	}
	if !hasMatch {
		return nil
	}
	return out
}

// CollectSrsCachedPaths — собирает map[user-rule-id]→абсолютный путь к скачанному
// .srs файлу для всех kind=srs правил в state.Rules.
//
// Каждый kind=srs rule имеет user ID; .srs файл лежит в <execDir>/bin/rule-sets/
// под content-addressed tag'ом (см. SPEC 020 / dialogs/srs_tag.go).
// Если файл отсутствует — entry для этого ID НЕ добавляется (build pipeline
// получит "no cached file" warning и skip'нет правило).
//
// Для preset-ref'ов с remote rule_set'ами SRS-кэш не нужен: ExpandPreset
// эмитит rule_set с type=remote URL, а sing-box сам качает. Но если хотим
// emit type=local — нужен path map (TODO для phase 9).
func CollectSrsCachedPaths(rules []state.Rule, execDir string) map[string]string {
	if execDir == "" || len(rules) == 0 {
		return nil
	}
	out := make(map[string]string, len(rules))
	for _, r := range rules {
		if r.Kind != state.RuleKindSrs || r.ID == "" {
			continue
		}
		// Convention: user-defined srs rule cached as bin/rule-sets/<id>.srs.
		// Это упрощение — production использует content-addressed tag scheme.
		// Сейчас просто ставим path; если файла нет — MergePresetsIntoRoute
		// напишет warning и skip'нет.
		out[r.ID] = execDir + "/bin/rule-sets/" + r.ID + ".srs"
	}
	return out
}

// hasAnyV6Rule — true если в state.Rules есть хоть один enabled rule
// любого kind. Используется как trigger для preset merge path: если есть v6
// правила — берём их emit на себя, иначе noop (legacy путь через MergeRouteSection).
func hasAnyV6Rule(rules []state.Rule) bool {
	for _, r := range rules {
		if r.Enabled {
			return true
		}
	}
	return false
}

// hasAnyPresetRef — оставлен для совместимости (используется в тестах).
func hasAnyPresetRef(rules []state.Rule) bool {
	for _, r := range rules {
		if r.Kind == state.RuleKindPreset && r.Enabled {
			return true
		}
	}
	return false
}
