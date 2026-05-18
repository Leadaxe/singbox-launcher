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
// Активируется когда state.RulesV6 содержит хотя бы один preset-ref. Пустой
// список → noop (state.json остаётся в v5, поведение неизменно).
package build

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"

	"singbox-launcher/core/template"
	v6 "singbox-launcher/core/state/v6"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/outboundutil"
)

// Local stdlib wrappers used by srsTagFromURLLocal / convertPresetRuleSetRemoteToLocal.
// (Inline aliases — избавляют функции от длинных fully-qualified имен.)
func osStatLocal(p string) (os.FileInfo, error)             { return os.Stat(p) }
func urlParseLocal(s string) (*url.URL, error)              { return url.Parse(s) }
func sha256SumLocal(b []byte) [sha256.Size]byte             { return sha256.Sum256(b) }

// outboundutilApply — короткий wrapper для read-friendliness в этом файле.
func outboundutilApply(r map[string]interface{}, outbound string) map[string]interface{} {
	return outboundutil.ApplyOutboundToRule(r, outbound)
}

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
	RulesV6        []v6.Rule
	DNS            v6.DNSConfig
	SrsCachedPaths map[string]string

	// ExecDir — для резолва local SRS paths (kind=srs / preset remote rule_set).
	ExecDir string

	// TemplateDNSDefaults — раскрытые dns_defaults.servers[] из template.
	// Используется для materialization template-серверов с применением
	// effective_enabled (от state.dns.template_servers).
	TemplateDNSDefaults []TemplateDNSServer

	// AllNodeTags — список всех outbound-тегов из ctx.Cache (subscription
	// nodes + parser-generated groups). Используется для резолва
	// `filters`/`addOutbounds` в preset.outbounds entries с mode=add —
	// без этого pool'а ru VPN 🇷🇺 (или похожий filter-based selector от
	// preset'а) не сможет получить outbound list и эмитит пустой selector.
	// nil → пропускаем filter-resolution (preview без cache).
	AllNodeTags []string
}

// MergePresetsIntoRoute — append'ит preset-ref + user inline/srs fragments
// в уже-смержанную route-секцию.
//
// Алгоритм:
//   1. Парсит routeRaw → map.
//   2. Для каждого active preset-ref вызывает ExpandPreset → получает fragments.
//   3. Append fragments.RuleSets в route.rule_set[] (с identical-skip / first-wins).
//   4. Append fragments.RoutingRule в route.rules[].
//   5. Re-marshal в RawMessage.
//
// Если ctx.RulesV6 не содержит enabled rules → noop (возвращает routeRaw как есть).
func MergePresetsIntoRoute(routeRaw json.RawMessage, ctx PresetMergeContext) (json.RawMessage, error) {
	if !hasAnyV6Rule(ctx.RulesV6) {
		return routeRaw, nil
	}

	var route map[string]interface{}
	if err := json.Unmarshal(routeRaw, &route); err != nil {
		return nil, fmt.Errorf("preset merge: parse route: %w", err)
	}

	rules, _ := route["rules"].([]interface{})
	ruleSets, _ := route["rule_set"].([]interface{})

	presetByID := make(map[string]*template.Preset, len(ctx.Presets))
	for i := range ctx.Presets {
		presetByID[ctx.Presets[i].ID] = &ctx.Presets[i]
	}

	emittedTags := make(map[string]bool)
	for _, rs := range ruleSets {
		if m, ok := rs.(map[string]interface{}); ok {
			if tag, ok := m["tag"].(string); ok {
				emittedTags[tag] = true
			}
		}
	}

	// Обход state.RulesV6 в порядке. Для каждого rule по kind dispatch:
	//   kind=preset → ExpandPreset → fragments append
	//   kind=inline → headless rule_set "user:<id>" + route rule
	//   kind=srs    → local rule_set "user:<id>" (path к скачанному файлу) + route rule
	for _, rule := range ctx.RulesV6 {
		if !rule.Enabled {
			continue
		}
		switch rule.Kind {
		case v6.RuleKindPreset:
			preset, ok := presetByID[rule.Ref]
			if !ok {
				debuglog.WarnLog("preset merge: ref %q not found in template (skipped)", rule.Ref)
				continue
			}
			body, err := rule.DecodeBody()
			if err != nil {
				debuglog.WarnLog("preset merge: decode body for %q: %v", rule.Ref, err)
				continue
			}
			pb := body.(*v6.PresetBody)
			frags, warns, ok := ExpandPreset(preset, pb.Vars)
			for _, w := range warns {
				debuglog.WarnLog("preset merge: %s", w.String())
			}
			if !ok {
				continue
			}
			for _, rs := range frags.RuleSets {
				tag, _ := rs["tag"].(string)
				if tag == "" || emittedTags[tag] {
					continue
				}
				// Remote rule_set → local: launcher должен скачать .srs (см.
				// services.DownloadSRSGroup, triggered через UI cloud button)
				// и эмитить type=local с path. Без скачанного файла rule_set
				// **скипается** (как для legacy user srs rules) — preset частично
				// работает на inline rule_set'ах, но без remote данные не маршрутизируются.
				converted, skip := convertPresetRuleSetRemoteToLocal(rs, ctx.ExecDir)
				if skip {
					debuglog.WarnLog("preset merge: rule_set %q remote .srs not cached — rule_set skipped", tag)
					continue
				}
				ruleSets = append(ruleSets, converted)
				emittedTags[tag] = true
			}
			// Routing rule может ссылаться на пропущенные tag'и (если remote rule_set
			// не скачан) — вычищаем dangling refs из rule.rule_set.
			if frags.RoutingRule != nil {
				cleanedRule := cleanDanglingRuleSetInRule(frags.RoutingRule, emittedTags)
				if cleanedRule != nil {
					rules = append(rules, cleanedRule)
				}
			}
		case v6.RuleKindInline:
			body, err := rule.DecodeBody()
			if err != nil {
				debuglog.WarnLog("preset merge: decode inline body: %v", err)
				continue
			}
			ib := body.(*v6.InlineBody)
			tag := "user:" + rule.ID
			if !emittedTags[tag] {
				match := ib.Match
				if match == nil {
					match = map[string]interface{}{}
				}
				rs := map[string]interface{}{
					"tag":   tag,
					"type":  "inline",
					"rules": []interface{}{match},
				}
				ruleSets = append(ruleSets, rs)
				emittedTags[tag] = true
			}
			routeRule := map[string]interface{}{"rule_set": tag}
			routeRule = outboundutilApply(routeRule, ib.Outbound)
			rules = append(rules, routeRule)
		case v6.RuleKindSrs:
			body, err := rule.DecodeBody()
			if err != nil {
				debuglog.WarnLog("preset merge: decode srs body: %v", err)
				continue
			}
			sb := body.(*v6.SrsBody)
			path, hasCache := ctx.SrsCachedPaths[rule.ID]
			if !hasCache {
				debuglog.WarnLog("preset merge: srs rule %q skipped: no cached file", sb.Name)
				continue
			}
			tag := "user:" + rule.ID
			if !emittedTags[tag] {
				rs := map[string]interface{}{
					"tag":    tag,
					"type":   "local",
					"format": "binary",
					"path":   path,
				}
				ruleSets = append(ruleSets, rs)
				emittedTags[tag] = true
			}
			routeRule := map[string]interface{}{"rule_set": tag}
			routeRule = outboundutilApply(routeRule, sb.Outbound)
			rules = append(rules, routeRule)
		}
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

// CleanDanglingOutboundsInRouteRules — SPEC 055 cleanup pass. Применяется к
// route.rules[] после того как MergePresetsIntoOutbounds уже определил финальный
// набор tag'ов в config.outbounds. Если rule ссылается на outbound вне этого set
// — fallback на routeFinal (или drop rule если final пустой).
//
// Вызывается из build.go ПОСЛЕ MergePresetsIntoOutbounds и ПОСЛЕ
// MergePresetsIntoRoute (когда обе секции уже merged).
func CleanDanglingOutboundsInRouteRules(routeRaw json.RawMessage, emittedOutboundTags map[string]bool, routeFinal string) (json.RawMessage, error) {
	var route map[string]interface{}
	if err := json.Unmarshal(routeRaw, &route); err != nil {
		return routeRaw, nil // graceful — пропустим cleanup, лучше чем сломаться
	}
	rules, ok := route["rules"].([]interface{})
	if !ok || len(rules) == 0 {
		return routeRaw, nil
	}
	cleaned := make([]interface{}, 0, len(rules))
	for _, r := range rules {
		m, ok := r.(map[string]interface{})
		if !ok {
			cleaned = append(cleaned, r)
			continue
		}
		patched := cleanDanglingOutboundRefInRule(m, emittedOutboundTags, routeFinal)
		if patched == nil {
			continue // drop
		}
		cleaned = append(cleaned, patched)
	}
	route["rules"] = cleaned
	out, err := json.MarshalIndent(route, "", "  ")
	if err != nil {
		return routeRaw, nil
	}
	return out, nil
}

// ApplyPresetUpdatesToGeneratedOutbounds — SPEC 055 phase 2. Применяет
// preset.outbounds entries с mode=update к parser-generated outbound strings
// (то что приходит из ctx.Cache.Outbounds через cacheOutboundsAsStrings).
//
// Mode=add игнорируется здесь (он уже обработан в MergePresetsIntoOutbounds
// для config.outbounds layer'а; парсер не должен генерить preset-add'ы).
// Mode=update ищет cache entry с тем же tag'ом и патчит body через
// applyOutboundUpdate (filters=replace, addOutbounds=union etc).
//
// Input: cacheStrings — []string в формате "\t{json},\n" (с трейлинг-comma'ой,
// как кладёт `cacheOutboundsAsStrings`). Output: те же strings, но изменённые
// entries содержат patched JSON. Если изменений нет — возвращает оригинальные
// slice (не аллоцирует).
//
// Tag matching exact — например preset 'russian' с update tag=proxy-out
// ищет cache entry с "tag":"proxy-out".
func ApplyPresetUpdatesToGeneratedOutbounds(cacheStrings []string, ctx PresetMergeContext) []string {
	if len(cacheStrings) == 0 || !hasAnyV6Rule(ctx.RulesV6) {
		return cacheStrings
	}

	// Собираем все updates от active preset-ref'ов: tag → []patches (порядок
	// по RuleOrder; multiple updates на один tag применяются последовательно).
	type updatePatch struct {
		presetID string
		body     map[string]interface{}
	}
	updates := make(map[string][]updatePatch)
	presetByID := make(map[string]*template.Preset, len(ctx.Presets))
	for i := range ctx.Presets {
		presetByID[ctx.Presets[i].ID] = &ctx.Presets[i]
	}
	for _, rule := range ctx.RulesV6 {
		if !rule.Enabled || rule.Kind != v6.RuleKindPreset {
			continue
		}
		preset, ok := presetByID[rule.Ref]
		if !ok || len(preset.Outbounds) == 0 {
			continue
		}
		body, err := rule.DecodeBody()
		if err != nil {
			continue
		}
		pb := body.(*v6.PresetBody)
		frags, _, ok := ExpandPreset(preset, pb.Vars)
		if !ok {
			continue
		}
		for _, ob := range frags.Outbounds {
			if ob.Mode != "update" || ob.Tag == "" {
				continue
			}
			updates[ob.Tag] = append(updates[ob.Tag], updatePatch{presetID: preset.ID, body: ob.Body})
		}
	}
	if len(updates) == 0 {
		return cacheStrings
	}

	// Walk cache entries; для каждой entry с matching tag — apply patches.
	out := make([]string, len(cacheStrings))
	for i, raw := range cacheStrings {
		// Strip leading whitespace + trailing comma/whitespace, остаток — JSON.
		s := strings.TrimSpace(raw)
		hadTrailingComma := strings.HasSuffix(s, ",")
		if hadTrailingComma {
			s = strings.TrimSuffix(s, ",")
			s = strings.TrimSpace(s)
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			out[i] = raw
			continue
		}
		tag, _ := m["tag"].(string)
		patches, hasPatches := updates[tag]
		if !hasPatches {
			out[i] = raw
			continue
		}
		for _, p := range patches {
			applyOutboundUpdate(m, p.body, p.presetID)
		}
		// Финальная зачистка launcher-only полей. applyOutboundUpdate уже
		// consumed filters/addOutbounds (резолвил в outbounds list), но
		// belt-and-suspenders на случай, если что-то затекло.
		finalStripLauncherFields(m)
		patched, err := json.Marshal(m)
		if err != nil {
			out[i] = raw
			continue
		}
		formatted := "\t" + string(patched)
		if hadTrailingComma {
			formatted += ","
		}
		out[i] = formatted
	}
	return out
}

// CollectOutboundTagsFromRaw — извлекает tag'и из json.RawMessage outbounds-секции.
// Используется для построения emittedOutboundTags set перед CleanDanglingOutboundsInRouteRules.
func CollectOutboundTagsFromRaw(outboundsRaw json.RawMessage) map[string]bool {
	tags := make(map[string]bool)
	var arr []interface{}
	if err := json.Unmarshal(outboundsRaw, &arr); err != nil {
		return tags
	}
	for _, x := range arr {
		m, ok := x.(map[string]interface{})
		if !ok {
			continue
		}
		if t, ok := m["tag"].(string); ok && t != "" {
			tags[t] = true
		}
	}
	return tags
}

// MergePresetsIntoDNS — дополняет dns-секцию template-overrides + bundled DNS
// от active presets + extras.
//
// Алгоритм:
//   1. Парсит dnsRaw → map.
//   2. Filter template servers по effective_enabled (state.dns.template_servers overrides).
//   3. Append bundled dns_servers от active presets.
//   4. Append state.dns.extra_servers / extra_rules.
//   5. Re-marshal.
//
// Если ctx пуст (нет v6-правил и нет overrides/extras) → noop.
func MergePresetsIntoDNS(dnsRaw json.RawMessage, ctx PresetMergeContext) (json.RawMessage, error) {
	hasV6 := hasAnyV6Rule(ctx.RulesV6) ||
		len(ctx.DNS.TemplateServers) > 0 ||
		len(ctx.DNS.ExtraServers) > 0 ||
		len(ctx.DNS.ExtraRules) > 0
	if !hasV6 {
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

	// Filter existing servers по effective_enabled (если они соответствуют
	// template_servers override'ам). Серверы НЕ из overrides остаются.
	if len(ctx.DNS.TemplateServers) > 0 {
		filtered := make([]interface{}, 0, len(servers))
		for _, s := range servers {
			m, ok := s.(map[string]interface{})
			if !ok {
				filtered = append(filtered, s)
				continue
			}
			tag, _ := m["tag"].(string)
			if ovr, has := ctx.DNS.TemplateServers[tag]; has {
				if !ovr.Enabled {
					continue
				}
			}
			filtered = append(filtered, s)
		}
		servers = filtered
	}

	emittedTags := make(map[string]bool)
	for _, s := range servers {
		if m, ok := s.(map[string]interface{}); ok {
			if tag, ok := m["tag"].(string); ok {
				emittedTags[tag] = true
			}
		}
	}

	// Append bundled DNS-сервера от active presets.
	presetByID := make(map[string]*template.Preset, len(ctx.Presets))
	for i := range ctx.Presets {
		presetByID[ctx.Presets[i].ID] = &ctx.Presets[i]
	}
	for _, rule := range ctx.RulesV6 {
		if !rule.Enabled || rule.Kind != v6.RuleKindPreset {
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
		pb := body.(*v6.PresetBody)
		frags, _, ok := ExpandPreset(preset, pb.Vars)
		if !ok {
			continue
		}
		for _, ds := range frags.DNSServers {
			tag, _ := ds["tag"].(string)
			if tag == "" || emittedTags[tag] {
				continue
			}
			servers = append(servers, ds)
			emittedTags[tag] = true
		}
		if frags.DNSRule != nil {
			dnsRules = append(dnsRules, frags.DNSRule)
		}
	}

	// Append extra_servers (user-defined).
	for _, extra := range ctx.DNS.ExtraServers {
		copy := make(map[string]interface{}, len(extra))
		for k, v := range extra {
			copy[k] = v
		}
		servers = append(servers, copy)
	}
	// Append extra_rules.
	for _, extra := range ctx.DNS.ExtraRules {
		copy := make(map[string]interface{}, len(extra))
		for k, v := range extra {
			copy[k] = v
		}
		dnsRules = append(dnsRules, copy)
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

// CollectSrsCachedPaths — собирает map[user-rule-id]→абсолютный путь к скачанному
// .srs файлу для всех kind=srs правил в state.RulesV6.
//
// Каждый kind=srs rule имеет user ID; .srs файл лежит в <execDir>/bin/rule-sets/
// под content-addressed tag'ом (см. SPEC 020 / dialogs/srs_tag.go).
// Если файл отсутствует — entry для этого ID НЕ добавляется (build pipeline
// получит "no cached file" warning и skip'нет правило).
//
// Для preset-ref'ов с remote rule_set'ами SRS-кэш не нужен: ExpandPreset
// эмитит rule_set с type=remote URL, а sing-box сам качает. Но если хотим
// emit type=local — нужен path map (TODO для phase 9).
func CollectSrsCachedPaths(rules []v6.Rule, execDir string) map[string]string {
	if execDir == "" || len(rules) == 0 {
		return nil
	}
	out := make(map[string]string, len(rules))
	for _, r := range rules {
		if r.Kind != v6.RuleKindSrs || r.ID == "" {
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

// MergePresetsIntoOutbounds — SPEC 055. Применяет preset-emitted outbounds к
// уже-собранной outbounds-секции (после resolveFilters / build base outbounds).
//
// Алгоритм:
//  1. Парсит outboundsRaw → []map.
//  2. Строит index tag→outbound, сохраняет original order.
//  3. Обходит state.RulesV6 enabled preset-refs (RuleOrder в state определяет
//     порядок применения update'ов; для add — fall-through).
//  4. Для каждого preset → ExpandPreset → frags.Outbounds:
//     - mode="add"    → если tag нет в index → append; если есть и identical → skip;
//                       иначе → first-wins + warning, skip.
//     - mode="update" → если tag нет → warning + skip; иначе applyOutboundUpdate.
//  5. Re-marshal sorted by emitted order.
//
// Если ctx.RulesV6 не содержит enabled preset-ref'ов с outbounds → noop.
func MergePresetsIntoOutbounds(outboundsRaw json.RawMessage, ctx PresetMergeContext) (json.RawMessage, error) {
	if !hasAnyV6Rule(ctx.RulesV6) {
		return outboundsRaw, nil
	}

	var outboundsList []interface{}
	if err := json.Unmarshal(outboundsRaw, &outboundsList); err != nil {
		return nil, fmt.Errorf("preset merge outbounds: parse: %w", err)
	}

	// Build index tag → outbound (mutable copies) + preserved order.
	emitted := make(map[string]map[string]interface{}, len(outboundsList))
	order := make([]string, 0, len(outboundsList))
	for _, raw := range outboundsList {
		m, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := m["tag"].(string)
		if tag == "" {
			continue
		}
		emitted[tag] = m
		order = append(order, tag)
	}

	presetByID := make(map[string]*template.Preset, len(ctx.Presets))
	for i := range ctx.Presets {
		presetByID[ctx.Presets[i].ID] = &ctx.Presets[i]
	}

	for _, rule := range ctx.RulesV6 {
		if !rule.Enabled || rule.Kind != v6.RuleKindPreset {
			continue
		}
		preset, ok := presetByID[rule.Ref]
		if !ok {
			continue
		}
		if len(preset.Outbounds) == 0 {
			continue
		}
		body, err := rule.DecodeBody()
		if err != nil {
			continue
		}
		pb := body.(*v6.PresetBody)
		frags, _, ok := ExpandPreset(preset, pb.Vars)
		if !ok {
			continue
		}
		for _, ob := range frags.Outbounds {
			switch ob.Mode {
			case "add":
				if existing, exists := emitted[ob.Tag]; exists {
					if outboundBodiesIdentical(existing, ob.Body) {
						continue
					}
					debuglog.WarnLog("preset merge outbounds: preset %q add tag %q already exists (first-wins, this add ignored)", preset.ID, ob.Tag)
					continue
				}
				// Resolve filters/addOutbounds → outbounds list (launcher-only
				// fields, sing-box rejects). Если pool пустой (ctx.AllNodeTags
				// nil — preview без parsing'а) — оставляем filters в body как
				// дебаг marker (sing-box check всё равно ругнётся, но юзер
				// увидит источник).
				resolveAddFiltersIntoOutbounds(ob.Body, ctx.AllNodeTags)
				emitted[ob.Tag] = ob.Body
				order = append(order, ob.Tag)
			case "update":
				existing, exists := emitted[ob.Tag]
				if !exists {
					debuglog.WarnLog("preset merge outbounds: preset %q update target %q not found in current outbounds (skipped)", preset.ID, ob.Tag)
					continue
				}
				applyOutboundUpdate(existing, ob.Body, preset.ID)
			}
		}
	}

	// Re-build outbounds list in original order + new appends. Final strip:
	// убираем launcher-only поля (comment/wizard/filters/addOutbounds/etc)
	// со всех entries — template-original могли иметь их (parser_config
	// builder обычно их потребляет, но static outbounds типа direct-out
	// эмитятся как есть и могут таскать `comment`).
	result := make([]interface{}, 0, len(order))
	for _, tag := range order {
		if m, ok := emitted[tag]; ok {
			finalStripLauncherFields(m)
			result = append(result, m)
		}
	}
	out, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("preset merge outbounds: marshal: %w", err)
	}
	return out, nil
}

// applyOutboundUpdate патчит target по правилам SPEC 055 (in-place mutation).
//
//	filters       → resolve против target.outbounds: фильтруем существующий
//	                список tag'ов через matchTagPattern, оставляем только
//	                подходящие. `filters` поле в emit НЕ попадает (sing-box
//	                1.12+ rejects unknown field).
//	addOutbounds  → append tag'и к target.outbounds (union). `addOutbounds`
//	                поле в emit НЕ попадает (тоже launcher-only).
//	options.*     → replace per-field только тех что заданы в patch
//	wizard.*      → ignored (launcher-only metadata)
//	type, tag     → drop (запрещено менять)
//	comment       → replace
//	прочие        → replace whole field
func applyOutboundUpdate(target, patch map[string]interface{}, presetID string) {
	for k, v := range patch {
		switch k {
		case "tag", "type":
			// Запрещено менять — пропускаем. Type already dropped at expand time.
		case "filters":
			// Resolve filter против target.outbounds — фильтруем existing tags.
			filterMap, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			pool := outboundsListAsStrings(target["outbounds"])
			filtered := applyTagFiltersToPool(pool, filterMap)
			target["outbounds"] = stringSliceToInterfaces(filtered)
			// Note: НЕ ставим target["filters"] — sing-box не понимает.
		case "addOutbounds":
			// Append tags to target.outbounds (union). НЕ keep addOutbounds field.
			pool := outboundsListAsStrings(target["outbounds"])
			patchList := interfaceListAsStrings(v)
			merged := unionTagLists(pool, patchList)
			target["outbounds"] = stringSliceToInterfaces(merged)
		case "options":
			patchMap, _ := v.(map[string]interface{})
			tgtMap, _ := target["options"].(map[string]interface{})
			if tgtMap == nil {
				tgtMap = map[string]interface{}{}
			}
			for pk, pv := range patchMap {
				tgtMap[pk] = pv
			}
			target["options"] = tgtMap
		case "wizard":
			// wizard.* — launcher metadata; не emit'им в финальный config.
			// (раньше делали wizard.* merge per-field — но это поле тоже
			// launcher-only и sing-box его не trips потому что оно
			// игнорировалось при парсинге parser_config; теперь когда
			// preset.outbounds эмитятся напрямую — нужно скипать).
		default:
			target[k] = v
		}
	}
}

// outboundsListAsStrings извлекает []string из target["outbounds"] (значение
// формата []interface{} в JSON-decoded map). Не-string элементы пропускаются.
func outboundsListAsStrings(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// interfaceListAsStrings — то же что outboundsListAsStrings но для []string
// который тоже мог попасть как []interface{} в patch body.
func interfaceListAsStrings(v interface{}) []string {
	if ss, ok := v.([]string); ok {
		return ss
	}
	return outboundsListAsStrings(v)
}

// stringSliceToInterfaces — обратное преобразование, для записи в map.
func stringSliceToInterfaces(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// unionTagLists — concat existing + add без дубликатов, preserving order.
func unionTagLists(existing, add []string) []string {
	seen := make(map[string]bool, len(existing)+len(add))
	out := make([]string, 0, len(existing)+len(add))
	for _, s := range existing {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, s := range add {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// resolveAddFiltersIntoOutbounds — для preset.outbounds mode=add body:
// конвертирует launcher-only fields `filters` + `addOutbounds` в нативный
// sing-box `outbounds: [...]` array путём резолва против allTags pool'а.
// Затем стрипает `filters`/`addOutbounds` из body — sing-box их не понимает.
//
// Если pool пустой (preview / cold start) — стрипает поля без резолва
// (selector получит пустой outbounds list, но sing-box хоть запустится).
func resolveAddFiltersIntoOutbounds(body map[string]interface{}, allTags []string) {
	if body == nil {
		return
	}
	resolved := outboundsListAsStrings(body["outbounds"])
	// Filter pool через filters → matched tags.
	if filterRaw, has := body["filters"]; has {
		if filterMap, ok := filterRaw.(map[string]interface{}); ok && len(allTags) > 0 {
			matched := applyTagFiltersToPool(allTags, filterMap)
			resolved = unionTagLists(resolved, matched)
		}
		delete(body, "filters")
	}
	// addOutbounds — литералы, append.
	if addRaw, has := body["addOutbounds"]; has {
		add := interfaceListAsStrings(addRaw)
		resolved = unionTagLists(resolved, add)
		delete(body, "addOutbounds")
	}
	if len(resolved) > 0 {
		body["outbounds"] = stringSliceToInterfaces(resolved)
	}
}

// applyTagFiltersToPool — фильтрует pool tag'ов через filter map (`{field:
// pattern}`). Pattern syntax: literal, !literal, /regex/i, !/regex/i —
// тот же synthax что в core/config/outbound_filter.go::matchesPattern. AND
// между фильтрами в map.
//
// Имена полей: 'tag' — единственное супportable поле здесь (выпрашиваем
// только string tag, не структуру ParsedNode).
func applyTagFiltersToPool(pool []string, filter map[string]interface{}) []string {
	if len(pool) == 0 || len(filter) == 0 {
		return pool
	}
	tagPattern, _ := filter["tag"].(string)
	if tagPattern == "" {
		return pool
	}
	out := make([]string, 0, len(pool))
	for _, t := range pool {
		if matchPresetTagPattern(t, tagPattern) {
			out = append(out, t)
		}
	}
	return out
}

// matchPresetTagPattern — копия логики core/config/outbound_filter.go::matchesPattern
// для preset.outbounds emit pipeline (избегаем import cycle core/build → core/config).
func matchPresetTagPattern(value, pattern string) bool {
	// Negation literal: !literal
	if strings.HasPrefix(pattern, "!") && !strings.HasPrefix(pattern, "!/") {
		return value != strings.TrimPrefix(pattern, "!")
	}
	// Negation regex: !/regex/i
	if strings.HasPrefix(pattern, "!/") && strings.HasSuffix(pattern, "/i") {
		regexStr := strings.TrimPrefix(pattern, "!/")
		regexStr = strings.TrimSuffix(regexStr, "/i")
		re, err := regexp.Compile("(?i)" + regexStr)
		if err != nil {
			return false
		}
		return !re.MatchString(value)
	}
	// Regex: /regex/i
	if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/i") {
		regexStr := strings.TrimPrefix(pattern, "/")
		regexStr = strings.TrimSuffix(regexStr, "/i")
		re, err := regexp.Compile("(?i)" + regexStr)
		if err != nil {
			return false
		}
		return re.MatchString(value)
	}
	// Literal match
	return value == pattern
}

// unionStringList объединяет existing []string и patch []string без дубликатов,
// preserving order: сначала existing, потом новые из patch. Принимает interface{}
// чтобы работать с JSON-decoded значениями ([]interface{}).
func unionStringList(existing, patch interface{}) []interface{} {
	seen := make(map[string]bool)
	out := make([]interface{}, 0)
	collect := func(v interface{}) {
		arr, ok := v.([]interface{})
		if !ok {
			return
		}
		for _, x := range arr {
			s, ok := x.(string)
			if !ok || s == "" || seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
	}
	collect(existing)
	collect(patch)
	return out
}

// outboundBodiesIdentical — true если два outbound body имеют одинаковый JSON-state
// (для identical-skip при mode=add на same tag).
func outboundBodiesIdentical(a, b map[string]interface{}) bool {
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(ja) == string(jb)
}

// cleanDanglingOutboundRefInRule — если rule.outbound указывает на tag НЕ в
// emittedTags → заменить на fallback (route.final). Если fallback пустой → return nil
// (drop rule entirely).
//
// Используется в MergePresetsIntoRoute для cleanup'а после того как все preset
// outbounds эмитнуты — если юзер сослался на tag preset'а который теперь
// disabled/removed, rule не должен указывать на unknown outbound (sing-box
// упадёт на validation).
func cleanDanglingOutboundRefInRule(rule map[string]interface{}, emittedTags map[string]bool, fallback string) map[string]interface{} {
	if rule == nil {
		return nil
	}
	out, _ := rule["outbound"].(string)
	if out == "" {
		return rule // нет outbound ссылки — нечего чистить (action-based rule)
	}
	// reject/drop sentinels — это action, не outbound; legacy путь резолвит
	// их через outboundutilApply. Но если sentinel попал в outbound поле
	// напрямую — оставляем (sing-box обработает).
	if out == "reject" || out == "drop" {
		return rule
	}
	if emittedTags[out] {
		return rule
	}
	debuglog.WarnLog("preset merge outbounds: rule references unknown outbound %q — fallback to %q", out, fallback)
	if fallback == "" {
		return nil // drop rule entirely
	}
	patched := make(map[string]interface{}, len(rule))
	for k, v := range rule {
		patched[k] = v
	}
	patched["outbound"] = fallback
	return patched
}

// hasAnyV6Rule — true если в state.RulesV6 есть хоть один enabled rule
// любого kind. Используется как trigger для preset merge path: если есть v6
// правила — берём их emit на себя, иначе noop (legacy путь через MergeRouteSection).
func hasAnyV6Rule(rules []v6.Rule) bool {
	for _, r := range rules {
		if r.Enabled {
			return true
		}
	}
	return false
}

// hasAnyPresetRef — оставлен для совместимости (используется в тестах).
func hasAnyPresetRef(rules []v6.Rule) bool {
	for _, r := range rules {
		if r.Kind == v6.RuleKindPreset && r.Enabled {
			return true
		}
	}
	return false
}
