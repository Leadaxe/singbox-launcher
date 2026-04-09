package business

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/wizard/models"
	wizardtemplate "singbox-launcher/ui/wizard/template"
)

// -----------------------------------------------------------------------------
// Public API — wizard DNS tab (state + template → model; model → sing-box)
// -----------------------------------------------------------------------------

// LoadPersistedWizardDNS copies dns_options from state.json into the model (servers + editor fields).
// Does not merge with the template; call ApplyWizardDNSTemplate after this.
func LoadPersistedWizardDNS(model *wizardmodels.WizardModel, p *wizardmodels.PersistedDNSState) {
	if model == nil || p == nil {
		return
	}
	model.DNSServers = append([]json.RawMessage(nil), p.Servers...)
	if len(p.Rules) > 0 {
		var objs []interface{}
		for _, raw := range p.Rules {
			var v interface{}
			if json.Unmarshal(raw, &v) != nil {
				continue
			}
			objs = append(objs, v)
		}
		model.DNSRulesText = DNSRulesToText(objs)
	} else {
		model.DNSRulesText = ""
	}
	// Скаляры strategy/final/cache/resolver — только state.vars (dns_*); см. MigrateDNSScalarsFromPersistedToSettingsVars + ApplyDNSVarsFromSettingsToModel.
}

func copyBoolPtr(p *bool) *bool {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// ApplyWizardDNSTemplate reconciles dns.servers with the effective template (config.dns + dns_options),
// prepends missing type=local from config if needed, and fills empty auxiliary fields
// (rules, final, strategy, independent_cache, default_domain_resolver) from dns_options / config.dns.
//
// Typical use:
//   - After LoadPersistedWizardDNS (persisted row wins until reconcile merges tags with template).
//   - On a fresh model (no persistence): same call; reconcile builds the list from the template only.
func ApplyWizardDNSTemplate(model *wizardmodels.WizardModel) {
	if model == nil || model.TemplateData == nil {
		return
	}
	cfg := effectiveWizardConfig(model)
	dnsObj := dnsSectionFromConfig(cfg)
	optsMap := parseDNSOptionsMap(model.TemplateData.DNSOptionsRaw)

	reconcileDNSServers(model, dnsObj, optsMap)
	prependMissingLocalServers(model, dnsObj)
	fillDNSAuxiliaryIfEmpty(model, cfg, dnsObj, optsMap)
}

func effectiveWizardConfig(model *wizardmodels.WizardModel) map[string]json.RawMessage {
	if model == nil || model.TemplateData == nil {
		return nil
	}
	MaterializeClashSecretIfNeeded(model)
	config := model.TemplateData.Config
	if len(model.TemplateData.RawConfig) > 0 && (len(model.TemplateData.Params) > 0 || len(model.TemplateData.Vars) > 0) {
		effective, _, err := wizardtemplate.GetEffectiveConfig(
			model.TemplateData.RawConfig,
			model.TemplateData.Params,
			runtime.GOOS,
			model.TemplateData.Vars,
			model.SettingsVars,
			model.TemplateData.RawTemplate,
		)
		if err == nil {
			config = effective
		} else {
			debuglog.WarnLog("effectiveWizardConfig: GetEffectiveConfig: %v", err)
		}
	}
	return config
}

// DNSTagLocked is true for tags listed in template config.dns.servers (not editable / not deletable in UI).
func DNSTagLocked(model *wizardmodels.WizardModel, tag string) bool {
	if model == nil || model.DNSLockedTags == nil {
		return false
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false
	}
	_, ok := model.DNSLockedTags[tag]
	return ok
}

// -----------------------------------------------------------------------------
// Reconcile servers: config.dns order (locked) + dns_options-only tags + orphan saved tags
// -----------------------------------------------------------------------------

func reconcileDNSServers(model *wizardmodels.WizardModel, dnsObj map[string]interface{}, optsMap map[string]json.RawMessage) {
	saved := append([]json.RawMessage(nil), model.DNSServers...)
	byTag, order := indexDNSStateByTag(saved)

	optsByTag, optsOrder := dnsOptionsServersByTag(optsMap)

	locked := make(map[string]struct{})
	var out []json.RawMessage
	seen := make(map[string]struct{})

	if dnsObj != nil {
		if arr, ok := dnsObj["servers"].([]interface{}); ok {
			for _, s := range arr {
				row, ok := s.(map[string]interface{})
				if !ok {
					continue
				}
				tag := strings.TrimSpace(jsonString(row["tag"]))
				if tag == "" {
					continue
				}
				locked[tag] = struct{}{}
				raw := mergeLockedRow(row, optsByTag[tag], byTag[tag])
				if len(raw) == 0 {
					continue
				}
				out = append(out, raw)
				seen[tag] = struct{}{}
			}
		}
	}

	for _, tag := range optsOrder {
		if _, isLocked := locked[tag]; isLocked {
			continue
		}
		if _, done := seen[tag]; done {
			continue
		}
		if raw, ok := byTag[tag]; ok && len(raw) > 0 {
			out = append(out, raw)
		} else if opt := optsByTag[tag]; opt != nil {
			if b, err := json.Marshal(opt); err == nil {
				out = append(out, json.RawMessage(b))
			}
		}
		seen[tag] = struct{}{}
	}

	for _, tag := range order {
		if _, ok := seen[tag]; ok {
			continue
		}
		if raw, ok := byTag[tag]; ok && len(raw) > 0 {
			out = append(out, raw)
			seen[tag] = struct{}{}
		}
	}

	model.DNSServers = out
	model.DNSLockedTags = locked
}

func indexDNSStateByTag(servers []json.RawMessage) (byTag map[string]json.RawMessage, order []string) {
	byTag = make(map[string]json.RawMessage)
	for _, raw := range servers {
		tag := tagFromServerJSON(raw)
		if tag == "" {
			continue
		}
		if _, ok := byTag[tag]; !ok {
			order = append(order, tag)
		}
		byTag[tag] = raw
	}
	return byTag, order
}

func dnsOptionsServersByTag(optsMap map[string]json.RawMessage) (byTag map[string]map[string]interface{}, order []string) {
	byTag = make(map[string]map[string]interface{})
	if optsMap == nil {
		return byTag, order
	}
	raw, ok := optsMap["servers"]
	if !ok || len(raw) == 0 {
		return byTag, order
	}
	var arr []interface{}
	if json.Unmarshal(raw, &arr) != nil {
		return byTag, order
	}
	for _, s := range arr {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		tag := strings.TrimSpace(jsonString(m["tag"]))
		if tag == "" {
			continue
		}
		if _, has := byTag[tag]; !has {
			byTag[tag] = m
			order = append(order, tag)
		}
	}
	return byTag, order
}

// mergeLockedRow: при совпадении tag между config.dns.servers (скелет) и dns_options.servers —
// тело строки берётся из dns_options, если для строки включена галочка «в конфиг» (enabled в state или true по умолчанию в dns_options);
// иначе остаётся форма скелета из config.dns.
func mergeLockedRow(configRow map[string]interface{}, opt map[string]interface{}, stateRaw json.RawMessage) json.RawMessage {
	var st map[string]interface{}
	if len(stateRaw) > 0 {
		_ = json.Unmarshal(stateRaw, &st)
	}
	var userEnabled *bool
	if st != nil {
		if v, ok := st["enabled"]; ok {
			if b, ok := v.(bool); ok {
				userEnabled = &b
			}
		}
	}
	wantOptsBody := false
	if opt != nil {
		if userEnabled != nil {
			wantOptsBody = *userEnabled
		} else {
			wantOptsBody = dnsServerEnabledInWizard(opt)
		}
	}
	if wantOptsBody && opt != nil {
		m := shallowCopyMap(opt)
		if userEnabled != nil {
			m["enabled"] = *userEnabled
		}
		b, err := json.Marshal(m)
		if err != nil {
			return nil
		}
		return json.RawMessage(b)
	}
	if configRow == nil {
		if opt != nil {
			m := shallowCopyMap(opt)
			if userEnabled != nil {
				m["enabled"] = *userEnabled
			} else {
				m["enabled"] = false
			}
			b, err := json.Marshal(m)
			if err != nil {
				return nil
			}
			return json.RawMessage(b)
		}
		return nil
	}
	m := stripWizardOnlyServerFields(shallowCopyMap(configRow))
	if userEnabled != nil {
		m["enabled"] = *userEnabled
	} else if opt != nil {
		m["enabled"] = dnsServerEnabledInWizard(opt)
	} else {
		m["enabled"] = true
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return json.RawMessage(b)
}

// -----------------------------------------------------------------------------
// Local resolver from config.dns (if missing after reconcile)
// -----------------------------------------------------------------------------

func prependMissingLocalServers(model *wizardmodels.WizardModel, dnsObj map[string]interface{}) {
	if model == nil || dnsObj == nil {
		return
	}
	have := make(map[string]struct{})
	for _, raw := range model.DNSServers {
		var m map[string]interface{}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		if t, ok := m["tag"].(string); ok && t != "" {
			have[t] = struct{}{}
		}
	}
	arr, ok := dnsObj["servers"].([]interface{})
	if !ok {
		return
	}
	var prepend []json.RawMessage
	for _, s := range arr {
		m, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		tag, _ := m["tag"].(string)
		if typ != "local" || tag == "" {
			continue
		}
		if _, exists := have[tag]; exists {
			continue
		}
		b, err := json.Marshal(m)
		if err != nil {
			continue
		}
		prepend = append(prepend, json.RawMessage(b))
		have[tag] = struct{}{}
	}
	if len(prepend) == 0 {
		return
	}
	model.DNSServers = append(prepend, model.DNSServers...)
}

// -----------------------------------------------------------------------------
// Rules / final / strategy / cache / default resolver — only when model gaps
// -----------------------------------------------------------------------------

const dnsRulesPlaceholderMarker = `"rule_set":"example"`

func dnsRulesTextNeedsFill(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return true
	}
	return strings.Contains(s, dnsRulesPlaceholderMarker) && strings.Contains(s, `"server":"tag"`)
}

func fillDNSAuxiliaryIfEmpty(model *wizardmodels.WizardModel, cfg map[string]json.RawMessage, dnsObj map[string]interface{}, optsMap map[string]json.RawMessage) {
	if model == nil || model.TemplateData == nil {
		return
	}
	vd := model.TemplateData.Vars
	hasOpts := optsMap != nil

	if dnsRulesTextNeedsFill(model.DNSRulesText) {
		model.DNSRulesText = pickDNSRulesText(hasOpts, optsMap, dnsObj)
	}
	if !templateDeclaresDNSWizardVar(vd, wizardmodels.VarDNSFinal) && strings.TrimSpace(model.DNSFinal) == "" {
		model.DNSFinal = pickDNSFinal(hasOpts, optsMap, dnsObj)
	}
	if !templateDeclaresDNSWizardVar(vd, wizardmodels.VarDNSStrategy) && strings.TrimSpace(model.DNSStrategy) == "" {
		model.DNSStrategy = pickDNSStrategy(hasOpts, optsMap, dnsObj)
	}
	if !templateDeclaresDNSWizardVar(vd, wizardmodels.VarDNSIndependentCache) && model.DNSIndependentCache == nil {
		model.DNSIndependentCache = pickDNSIndependentCache(hasOpts, optsMap, dnsObj)
	}
	if !templateDeclaresDNSWizardVar(vd, wizardmodels.VarDNSDefaultDomainResolver) {
		fillDefaultDomainResolverIfEmpty(model, cfg, optsMap)
	}
}

func pickDNSRulesText(hasOpts bool, optsMap map[string]json.RawMessage, dnsObj map[string]interface{}) string {
	if hasOpts {
		if raw, ok := optsMap["rules"]; ok {
			var rules []interface{}
			if json.Unmarshal(raw, &rules) == nil {
				return DNSRulesToText(rules)
			}
			debuglog.DebugLog("pickDNSRulesText: dns_options.rules: invalid JSON")
		}
	}
	if dnsObj != nil {
		if rules, ok := dnsObj["rules"].([]interface{}); ok {
			return DNSRulesToText(rules)
		}
	}
	return ""
}

func pickDNSFinal(hasOpts bool, optsMap map[string]json.RawMessage, dnsObj map[string]interface{}) string {
	if hasOpts {
		if f := dnsOptsString(optsMap, "dns.final", "final"); f != "" {
			return f
		}
	}
	if dnsObj != nil {
		if f, ok := dnsObj["final"].(string); ok {
			return strings.TrimSpace(f)
		}
	}
	return ""
}

// pickDNSStrategy: сначала скелет config.dns.strategy, затем перекрытие dns_options.strategy шаблона (у второго приоритет).
// Сохранённый state: поле strategy уже в модели до ApplyWizardDNSTemplate; fill вызывает pick только если в модели пусто.
func pickDNSStrategy(hasOpts bool, optsMap map[string]json.RawMessage, dnsObj map[string]interface{}) string {
	base := ""
	if dnsObj != nil {
		base = jsonString(dnsObj["strategy"])
	}
	if hasOpts {
		if raw, ok := optsMap["strategy"]; ok && len(raw) > 0 {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				if t := strings.TrimSpace(s); t != "" {
					return t
				}
			}
		}
	}
	return base
}

func pickDNSIndependentCache(hasOpts bool, optsMap map[string]json.RawMessage, dnsObj map[string]interface{}) *bool {
	if hasOpts {
		if raw, ok := optsMap["independent_cache"]; ok {
			var b bool
			if json.Unmarshal(raw, &b) == nil {
				return ptrBool(b)
			}
		}
	}
	if dnsObj != nil {
		if b, ok := dnsObj["independent_cache"].(bool); ok {
			return ptrBool(b)
		}
	}
	return nil
}

func ptrBool(b bool) *bool { return &b }

func fillDefaultDomainResolverIfEmpty(model *wizardmodels.WizardModel, cfg map[string]json.RawMessage, optsMap map[string]json.RawMessage) {
	if model == nil || model.DefaultDomainResolverUnset {
		return
	}
	if strings.TrimSpace(model.DefaultDomainResolver) != "" {
		return
	}
	if optsMap != nil {
		if dr := dnsOptsString(optsMap, "default_domain_resolver", "route.default_domain_resolver"); dr != "" {
			model.DefaultDomainResolver = dr
			return
		}
	}
	if model.TemplateData != nil {
		if dr := strings.TrimSpace(model.TemplateData.DefaultDomainResolver); dr != "" {
			model.DefaultDomainResolver = dr
			return
		}
	}
	rawRoute, ok := cfg["route"]
	if !ok || len(rawRoute) == 0 {
		return
	}
	var route map[string]interface{}
	if json.Unmarshal(rawRoute, &route) != nil {
		return
	}
	if dr := routeDefaultDomainResolver(route); dr != "" {
		model.DefaultDomainResolver = dr
	}
}

// -----------------------------------------------------------------------------
// JSON helpers
// -----------------------------------------------------------------------------

func parseDNSOptionsMap(raw json.RawMessage) map[string]json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	return m
}

func dnsSectionFromConfig(cfg map[string]json.RawMessage) map[string]interface{} {
	if cfg == nil {
		return nil
	}
	raw, ok := cfg["dns"]
	if !ok || len(raw) == 0 {
		return nil
	}
	var dnsObj map[string]interface{}
	if err := json.Unmarshal(raw, &dnsObj); err != nil {
		debuglog.WarnLog("dnsSectionFromConfig: %v", err)
		return nil
	}
	return dnsObj
}

func dnsOptsString(opts map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := opts[key]
		if !ok || len(raw) == 0 {
			continue
		}
		var s string
		if json.Unmarshal(raw, &s) != nil {
			continue
		}
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}

func jsonString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func tagFromServerJSON(raw json.RawMessage) string {
	var m map[string]interface{}
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	return strings.TrimSpace(jsonString(m["tag"]))
}

func shallowCopyMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return nil
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// -----------------------------------------------------------------------------
// Sing-box merge / validation / enabled tags (unchanged behaviour)
// -----------------------------------------------------------------------------

// dnsServerEnabledInWizard: missing or invalid "enabled" → true (same as sing-box: no such field).
func dnsServerEnabledInWizard(m map[string]interface{}) bool {
	v, ok := m["enabled"]
	if !ok || v == nil {
		return true
	}
	b, ok := v.(bool)
	if !ok {
		return true
	}
	return b
}

// DNSServerWizardEnabledRaw unmarshals one server entry; invalid JSON counts as enabled.
func DNSServerWizardEnabledRaw(raw json.RawMessage) bool {
	var m map[string]interface{}
	if json.Unmarshal(raw, &m) != nil {
		return true
	}
	return dnsServerEnabledInWizard(m)
}

func stripWizardOnlyServerFields(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		if k == "description" || k == "enabled" {
			continue
		}
		out[k] = v
	}
	return out
}

// PersistedDNSRulesForState builds dns_options.rules from the multiline editor text.
// On parse or marshal error returns nil (no rules key in state when empty).
func PersistedDNSRulesForState(rulesText string) []json.RawMessage {
	rulesText = strings.TrimSpace(rulesText)
	if rulesText == "" {
		return nil
	}
	parsed, err := ParseDNSRulesText(rulesText)
	if err != nil {
		return nil
	}
	var rules []json.RawMessage
	for _, r := range parsed {
		b, err := json.Marshal(r)
		if err != nil {
			return nil
		}
		rules = append(rules, json.RawMessage(b))
	}
	return rules
}

// DNSRulesToText formats dns.rules as a single JSON object: {"rules":[...]}.
func DNSRulesToText(rules []interface{}) string {
	if len(rules) == 0 {
		return ""
	}
	out := map[string]interface{}{
		"rules": rules,
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

func parseDNSRulesArray(arr []interface{}) ([]interface{}, error) {
	rules := make([]interface{}, 0, len(arr))
	for i, item := range arr {
		if _, ok := item.(map[string]interface{}); !ok {
			return nil, fmt.Errorf("rules[%d]: expected JSON object", i+1)
		}
		rules = append(rules, item)
	}
	return rules, nil
}

// ParseDNSRulesText parses DNS rules editor text.
// Preferred format: full JSON object {"rules":[...]}.
// For backward compatibility, it also accepts:
//   - plain JSON array [...]
//   - legacy multiline JSON (one object per line; # comments allowed)
func ParseDNSRulesText(text string) ([]interface{}, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}

	var root interface{}
	if err := json.Unmarshal([]byte(text), &root); err == nil {
		switch v := root.(type) {
		case map[string]interface{}:
			if rulesVal, ok := v["rules"]; ok {
				arr, ok := rulesVal.([]interface{})
				if !ok {
					return nil, fmt.Errorf(`field "rules": expected JSON array`)
				}
				return parseDNSRulesArray(arr)
			}
			// Single object is treated as one rule for convenience.
			return []interface{}{v}, nil
		case []interface{}:
			return parseDNSRulesArray(v)
		default:
			return nil, fmt.Errorf("expected JSON object or array")
		}
	}

	// Legacy fallback: one JSON object per line; # and blank lines are comments.
	lines := strings.Split(text, "\n")
	var rules []interface{}
	for lineNum, line := range lines {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		var obj interface{}
		if err := json.Unmarshal([]byte(s), &obj); err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNum+1, err)
		}
		m, isObj := obj.(map[string]interface{})
		if !isObj {
			return nil, fmt.Errorf("line %d: expected JSON object", lineNum+1)
		}
		rules = append(rules, m)
	}
	return rules, nil
}

// MergeDNSSection overlays model DNS onto template dns JSON; preserves unknown dns keys from template.
func MergeDNSSection(templateDNS json.RawMessage, model *wizardmodels.WizardModel) (json.RawMessage, error) {
	var dnsObj map[string]interface{}
	if len(templateDNS) > 0 {
		if err := json.Unmarshal(templateDNS, &dnsObj); err != nil {
			return nil, fmt.Errorf("template dns: %w", err)
		}
	}
	if dnsObj == nil {
		dnsObj = make(map[string]interface{})
	}
	var servers []interface{}
	for _, raw := range model.DNSServers {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("dns server: %w", err)
		}
		if !dnsServerEnabledInWizard(m) {
			continue
		}
		servers = append(servers, stripWizardOnlyServerFields(m))
	}
	dnsObj["servers"] = servers
	rules, err := ParseDNSRulesText(model.DNSRulesText)
	if err != nil {
		return nil, err
	}
	dnsObj["rules"] = rules
	final := strings.TrimSpace(model.DNSFinal)
	if final == "" {
		final = firstEnabledDNSServerTag(model)
	}
	if final != "" {
		dnsObj["final"] = final
	} else {
		delete(dnsObj, "final")
	}
	if s := strings.TrimSpace(model.DNSStrategy); s != "" {
		dnsObj["strategy"] = s
	}
	if model.DNSIndependentCache != nil {
		dnsObj["independent_cache"] = *model.DNSIndependentCache
	}
	return json.Marshal(dnsObj)
}

func firstEnabledDNSServerTag(model *wizardmodels.WizardModel) string {
	if model == nil {
		return ""
	}
	for _, raw := range model.DNSServers {
		var o map[string]interface{}
		if json.Unmarshal(raw, &o) != nil {
			continue
		}
		if !dnsServerEnabledInWizard(o) {
			continue
		}
		if t, ok := o["tag"].(string); ok {
			return strings.TrimSpace(t)
		}
	}
	return ""
}

// ValidateDNSModel checks tags, final, and rules before save / preview.
func ValidateDNSModel(model *wizardmodels.WizardModel) error {
	if model == nil {
		return fmt.Errorf("model is nil")
	}
	if len(model.DNSServers) == 0 {
		return fmt.Errorf("at least one DNS server is required")
	}
	tags := make(map[string]struct{})
	enabledTags := make(map[string]struct{})
	enabledCount := 0
	for i, raw := range model.DNSServers {
		var m map[string]interface{}
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("DNS server %d: invalid JSON: %w", i+1, err)
		}
		tag, _ := m["tag"].(string)
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return fmt.Errorf("DNS server %d: missing tag", i+1)
		}
		if _, dup := tags[tag]; dup {
			return fmt.Errorf("duplicate DNS tag: %s", tag)
		}
		tags[tag] = struct{}{}
		if dnsServerEnabledInWizard(m) {
			enabledTags[tag] = struct{}{}
			enabledCount++
		}
	}
	if enabledCount == 0 {
		return fmt.Errorf("at least one enabled DNS server is required")
	}
	if model.DNSFinal != "" {
		if _, ok := enabledTags[model.DNSFinal]; !ok {
			return fmt.Errorf("dns.final %q must be an enabled server tag", model.DNSFinal)
		}
	}
	if model.DefaultDomainResolver != "" && !model.DefaultDomainResolverUnset {
		if _, ok := enabledTags[model.DefaultDomainResolver]; !ok {
			return fmt.Errorf("default domain resolver %q must be an enabled server tag", model.DefaultDomainResolver)
		}
	}
	rules, err := ParseDNSRulesText(model.DNSRulesText)
	if err != nil {
		return err
	}
	for i, r := range rules {
		rm, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		srvVal, ok := rm["server"]
		if !ok || srvVal == nil {
			continue
		}
		srv := dnsRuleServerTagString(srvVal)
		if srv == "" {
			continue
		}
		if _, ok := enabledTags[srv]; !ok {
			return fmt.Errorf("dns rule %d: server %q is missing or disabled", i+1, srv)
		}
	}
	return nil
}

func dnsRuleServerTagString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

func routeDefaultDomainResolver(route map[string]interface{}) string {
	v, ok := route["default_domain_resolver"]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// DNSEnabledTagOptions returns tags for enabled servers in list order.
// Выпадающие dns.final и route.default_domain_resolver показывают только эти теги: строка из скелета
// без галочки «в конфиг» в список не попадает; при включённой галочке тело может браться из dns_options (см. mergeLockedRow).
func DNSEnabledTagOptions(model *wizardmodels.WizardModel) []string {
	if model == nil {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, raw := range model.DNSServers {
		var m map[string]interface{}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		if !dnsServerEnabledInWizard(m) {
			continue
		}
		tag, _ := m["tag"].(string)
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}
