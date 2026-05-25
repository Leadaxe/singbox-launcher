// Package build — File resolve_route.go (SPEC 056-R-N follow-up).
//
// Unified resolver для route section (rule_set + route.rules) — параллельно
// resolve_dns.go. Один источник истины для UI render'а и build emit'а.
//
// Тот же контракт что у ResolveDNS: pure func, возвращает структурированный
// view с meta-данными (Source/Active/Enabled). Build emit'ит то у чего
// Active && Enabled.
package build

import (
	"encoding/json"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/debuglog"
	"singbox-launcher/internal/outboundutil"
)

// RouteSource — discriminator происхождения route entry.
type RouteSource string

const (
	// RouteSourcePreset — pre-set bundled rule_set / routing rule.
	RouteSourcePreset RouteSource = "preset"

	// RouteSourceInline — user-defined inline rule (match-fields).
	RouteSourceInline RouteSource = "inline"

	// RouteSourceSrs — user-defined srs rule (local .srs file).
	RouteSourceSrs RouteSource = "srs"
)

// ResolvedRouteRuleSet — одна entry финального rule_set list'а.
type ResolvedRouteRuleSet struct {
	// Tag — финальный sing-box rule_set tag (с preset prefix для preset entries,
	// "user:<id>" для srs entries).
	Tag string

	// Body — готовое sing-box rule_set тело: {tag, type, format, rules|path|url}.
	// Для preset remote rule_set, если файл не cached, Body=nil + Skipped=true.
	Body map[string]interface{}

	// Source — preset|srs (inline не создаёт rule_set).
	Source RouteSource

	// PresetID/Label — только для Source=preset.
	PresetID    string
	PresetLabel string

	// SrsID — id user-srs rule (только для Source=srs).
	SrsID string

	// Skipped — true если rule_set не может быть эмитнут (remote .srs не cached
	// или srs cache miss). Build skip'ает; UI показывает с warning'ом.
	Skipped       bool
	SkippedReason string
}

// ResolvedRouteRule — одна entry финального route.rules[] list'а.
type ResolvedRouteRule struct {
	// Body — готовое sing-box route rule body после substitute + clean dangling.
	Body map[string]interface{}

	// Source — preset|inline|srs.
	Source RouteSource

	// PresetID/Label — только для Source=preset.
	PresetID    string
	PresetLabel string

	// InlineID/SrsID — для kind=inline/srs.
	InlineID string
	SrsID    string

	// Active — прошёл if/if_or (только для preset; inline/srs всегда true).
	Active bool

	// Enabled — state.Rules[i].Enabled (top-level toggle).
	Enabled bool

	// InactiveReason — UI tooltip для !Active (только preset).
	InactiveReason string
}

// ResolvedRoute — результат ResolveRoute().
type ResolvedRoute struct {
	RuleSets []ResolvedRouteRuleSet
	Rules    []ResolvedRouteRule
}

// ResolveRoute — единая точка резолва route section.
//
// Аргументы:
//   - state         — v6 state (Rules с preset/inline/srs)
//   - td            — TemplateData (presets с rule_set + routing rule)
//   - execDir       — для резолва local SRS paths (preset remote rule_set)
//   - srsCachedPaths — map[user-rule-id → path] для kind=srs
//
// Возвращает ResolvedRoute. RuleSets дедуплицированы по tag (first-wins);
// Rules в порядке state.Rules.
func ResolveRoute(
	state *corestate.State,
	td *template.TemplateData,
	execDir string,
	srsCachedPaths map[string]string,
) ResolvedRoute {
	var out ResolvedRoute
	if state == nil || td == nil {
		return out
	}

	presetByID := make(map[string]*template.Preset, len(td.Presets))
	for i := range td.Presets {
		presetByID[td.Presets[i].ID] = &td.Presets[i]
	}

	emittedTags := make(map[string]bool)

	for _, rule := range state.Rules {
		switch rule.Kind {
		case corestate.RuleKindPreset:
			resolvePresetRouteRule(&out, presetByID, rule, execDir, emittedTags)
		case corestate.RuleKindInline:
			resolveInlineRouteRule(&out, rule)
		case corestate.RuleKindSrs:
			resolveSrsRouteRule(&out, rule, srsCachedPaths, emittedTags)
		}
	}

	return out
}

// resolvePresetRouteRule — expand preset → append rule_sets + routing rule.
func resolvePresetRouteRule(
	out *ResolvedRoute,
	presetByID map[string]*template.Preset,
	rule corestate.Rule,
	execDir string,
	emittedTags map[string]bool,
) {
	p, ok := presetByID[rule.Ref]
	if !ok {
		debuglog.WarnLog("route resolve: preset ref %q not found in template", rule.Ref)
		return
	}
	body, err := rule.DecodeBody()
	if err != nil {
		debuglog.WarnLog("route resolve: decode preset body for %q: %v", rule.Ref, err)
		return
	}
	pb := body.(*corestate.PresetBody)
	frags, warns, ok := ExpandPreset(p, pb.Vars)
	for _, w := range warns {
		debuglog.WarnLog("route resolve: %s", w.String())
	}
	if !ok {
		return
	}
	presetLabel := presetDisplayLabel(p)

	// RuleSets из preset.RuleSet (после ExpandPreset уже substituted + prefixed).
	for _, rs := range frags.RuleSets {
		tag, _ := rs["tag"].(string)
		if tag == "" {
			continue
		}
		if emittedTags[tag] {
			continue
		}
		converted, skip := convertPresetRuleSetRemoteToLocal(rs, execDir)
		if skip {
			out.RuleSets = append(out.RuleSets, ResolvedRouteRuleSet{
				Tag:           tag,
				Source:        RouteSourcePreset,
				PresetID:      p.ID,
				PresetLabel:   presetLabel,
				Skipped:       true,
				SkippedReason: "remote .srs not cached",
			})
			continue
		}
		out.RuleSets = append(out.RuleSets, ResolvedRouteRuleSet{
			Tag:         tag,
			Body:        converted,
			Source:      RouteSourcePreset,
			PresetID:    p.ID,
			PresetLabel: presetLabel,
		})
		emittedTags[tag] = true
	}

	// Routing rule. Cleanup dangling refs (если remote rule_set skipped).
	if frags.RoutingRule != nil {
		cleaned := cleanDanglingRuleSetInRule(frags.RoutingRule, emittedTags)
		if cleaned != nil {
			out.Rules = append(out.Rules, ResolvedRouteRule{
				Body:        cleaned,
				Source:      RouteSourcePreset,
				PresetID:    p.ID,
				PresetLabel: presetLabel,
				Active:      true, // ExpandPreset уже отфильтровал по if/if_or
				Enabled:     rule.Enabled,
			})
		}
	}
}

// resolveInlineRouteRule — kind=inline → direct route rule, no rule_set.
func resolveInlineRouteRule(out *ResolvedRoute, rule corestate.Rule) {
	body, err := rule.DecodeBody()
	if err != nil {
		debuglog.WarnLog("route resolve: decode inline body: %v", err)
		return
	}
	ib := body.(*corestate.InlineBody)
	match := ib.Match
	if match == nil {
		match = map[string]interface{}{}
	}
	routeRule := make(map[string]interface{}, len(match)+1)
	for k, v := range match {
		routeRule[k] = v
	}
	routeRule = outboundutil.ApplyOutboundToRule(routeRule, ib.Outbound)
	out.Rules = append(out.Rules, ResolvedRouteRule{
		Body:     routeRule,
		Source:   RouteSourceInline,
		InlineID: rule.ID,
		Active:   true,
		Enabled:  rule.Enabled,
	})
}

// resolveSrsRouteRule — kind=srs → local rule_set (from cache) + route rule.
func resolveSrsRouteRule(
	out *ResolvedRoute,
	rule corestate.Rule,
	srsCachedPaths map[string]string,
	emittedTags map[string]bool,
) {
	body, err := rule.DecodeBody()
	if err != nil {
		debuglog.WarnLog("route resolve: decode srs body: %v", err)
		return
	}
	sb := body.(*corestate.SrsBody)
	path, hasCache := srsCachedPaths[rule.ID]
	tag := "user:" + rule.ID
	if !hasCache {
		out.RuleSets = append(out.RuleSets, ResolvedRouteRuleSet{
			Tag:           tag,
			Source:        RouteSourceSrs,
			SrsID:         rule.ID,
			Skipped:       true,
			SkippedReason: "srs file not cached",
		})
		debuglog.WarnLog("route resolve: srs rule %q skipped: no cached file", sb.Name)
		return
	}
	if !emittedTags[tag] {
		rs := map[string]interface{}{
			"tag":    tag,
			"type":   "local",
			"format": "binary",
			"path":   path,
		}
		out.RuleSets = append(out.RuleSets, ResolvedRouteRuleSet{
			Tag:    tag,
			Body:   rs,
			Source: RouteSourceSrs,
			SrsID:  rule.ID,
		})
		emittedTags[tag] = true
	}
	routeRule := map[string]interface{}{"rule_set": tag}
	routeRule = outboundutil.ApplyOutboundToRule(routeRule, sb.Outbound)
	out.Rules = append(out.Rules, ResolvedRouteRule{
		Body:    routeRule,
		Source:  RouteSourceSrs,
		SrsID:   rule.ID,
		Active:  true,
		Enabled: rule.Enabled,
	})
}

// ── Helper: silence unused json import (will be used by tests). ──
var _ = json.Unmarshal
