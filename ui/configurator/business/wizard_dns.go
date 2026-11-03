package business

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/build"
	"singbox-launcher/core/state"
	"singbox-launcher/internal/debuglog"
	wizardmodels "singbox-launcher/ui/configurator/models"
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

// ApplyWizardDNSTemplate reconciles dns.servers with the effective template (config.dns + dns_options),
// prepends missing type=local from config if needed, and fills empty auxiliary fields
// (rules, final, strategy, default_domain_resolver) from dns_options / config.dns.
//
// Typical use:
//   - After LoadPersistedWizardDNS (persisted row wins until reconcile merges tags with template).
//   - On a fresh model (no persistence): same call; reconcile builds the list from the template only.
func ApplyWizardDNSTemplate(model *wizardmodels.WizardModel) {
	if model == nil || model.TemplateData == nil {
		return
	}
	// effectiveTemplate(..., true) материализует secrets перед resolve —
	// сохраняет поведение прежнего effectiveWizardConfig. Order игнорируем.
	cfg, _ := effectiveTemplate(model, true)
	dnsObj := dnsSectionFromConfig(cfg)
	optsMap := parseDNSOptionsMap(model.TemplateData.DNSOptionsRaw)

	reconcileDNSServers(model, dnsObj, optsMap)
	prependMissingLocalServers(model, dnsObj)
	fillDNSAuxiliaryIfEmpty(model, cfg, dnsObj, optsMap)
}

// DNSTagLocked — true для tag'ов с `required: true` в template.dns_options.servers[].
// Locked entries нельзя toggle'нуть в UI (чекбокс greyed).
//
// SPEC unify: семантика "toggle block" (только required). Для "edit/delete block"
// (любой template entry) используй DNSTagFromTemplate.
func DNSTagLocked(model *wizardmodels.WizardModel, tag string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false
	}
	for _, s := range templateDNSServersOf(model) {
		if t, _ := s["tag"].(string); t == tag {
			if r, ok := s["required"].(bool); ok && r {
				return true
			}
			return false
		}
	}
	return false
}

// DNSTagFromTemplate — true для любого tag'а присутствующего в
// template.dns_options.servers[]. Template-owned entries нельзя
// editировать: тело задаёт шаблон. Удалять — можно (SPEC 173, D-139:
// секция шаблона — каталог), кроме `required` и удерживаемых ссылкой.
//
// SPEC unify: семантика "edit block" (все template entries).
// Для "toggle block" (только required) используй DNSTagLocked.
func DNSTagFromTemplate(model *wizardmodels.WizardModel, tag string) bool {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return false
	}
	for _, s := range templateDNSServersOf(model) {
		if t, _ := s["tag"].(string); t == tag {
			return true
		}
	}
	return false
}

// DNSTemplateChoice — сервер каталога шаблона (SPEC 173).
type DNSTemplateChoice struct {
	Tag string
	// Title — описание сервера в шаблоне, без него — тег.
	Title string
	// Listed — строка сервера уже есть в списке: «Add from template» тогда
	// добавляет его копию своим сервером (DNSTemplateServerCopy).
	Listed bool
}

// DNSTemplateCatalog — весь каталог (`dns_options.servers` шаблона) в
// порядке шаблона с отметкой, есть ли сервер в списке модели: его
// показывает попап «Add from template».
func DNSTemplateCatalog(model *wizardmodels.WizardModel) []DNSTemplateChoice {
	have := make(map[string]bool, len(model.DNSServers))
	for _, raw := range model.DNSServers {
		have[tagFromServerJSON(raw)] = true
	}
	seen := map[string]bool{}
	var out []DNSTemplateChoice
	for _, s := range templateDNSServersOf(model) {
		tag, _ := s["tag"].(string)
		if tag = strings.TrimSpace(tag); tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		title, _ := s["description"].(string)
		if title = strings.TrimSpace(title); title == "" {
			title = tag
		}
		out = append(out, DNSTemplateChoice{Tag: tag, Title: title, Listed: have[tag]})
	}
	return out
}

// DNSTemplateServersMissing — серверы каталога, которых нет среди строк
// модели, в порядке шаблона: их возвращает «Add all defaults».
func DNSTemplateServersMissing(model *wizardmodels.WizardModel) []DNSTemplateChoice {
	var out []DNSTemplateChoice
	for _, c := range DNSTemplateCatalog(model) {
		if !c.Listed {
			out = append(out, c)
		}
	}
	return out
}

// DNSTemplateServerCopy дописывает в конец списка копию сервера каталога tag
// своим сервером (kind=user) и возвращает индекс новой строки. Тело — то,
// что сервер эмитит сейчас: переменные подставлены по значениям его записи
// (build.ResolveDNS — тот же резолвер, что у сборки), служебных полей
// шаблона нет. Тег — `<tag>_2`, `<tag>_3`…, свободный среди строк и каталога.
// Включённость копии — включена: её заводят, чтобы пользоваться.
func DNSTemplateServerCopy(model *wizardmodels.WizardModel, tag string) (int, bool) {
	if model == nil || model.TemplateData == nil {
		return -1, false
	}
	st := &state.State{DNS: state.DNSOptions{Servers: []state.DNSServer{{
		Kind:    state.DNSServerKindTemplate,
		Tag:     tag,
		Enabled: true,
		Vars:    model.DNSTemplateVars[tag],
	}}}}
	var body map[string]interface{}
	for _, srv := range build.ResolveDNS(st, model.TemplateData, PresetGlobalVars(model), model.Target).Servers {
		if srv.Source == build.DNSSourceTemplate && srv.Tag == tag {
			body = srv.Body
			break
		}
	}
	if body == nil {
		return -1, false
	}
	taken := map[string]bool{}
	for _, raw := range model.DNSServers {
		taken[tagFromServerJSON(raw)] = true
	}
	for _, s := range templateDNSServersOf(model) {
		if t, _ := s["tag"].(string); t != "" {
			taken[t] = true
		}
	}
	newTag := ""
	for n := 2; newTag == ""; n++ {
		if candidate := fmt.Sprintf("%s_%d", tag, n); !taken[candidate] {
			newTag = candidate
		}
	}
	out := make(map[string]interface{}, len(body)+1)
	for k, v := range body {
		if k == "vars" {
			continue
		}
		out[k] = v
	}
	out["tag"] = newTag
	out["enabled"] = true
	b, err := json.Marshal(out)
	if err != nil {
		return -1, false
	}
	model.DNSServers = append(model.DNSServers, json.RawMessage(b))
	return len(model.DNSServers) - 1, true
}

// AddDNSTemplateServer дописывает строку сервера каталога tag с умолчаниями
// шаблона: тело и включённость — как у засеянной строки, значения
// переменных — шаблона (выбор, оставшийся от удалённой записи, снят).
// Строка встаёт среди шаблонных в порядке каталога — там же, где её
// построила бы сверка. Возвращает false, если тега в каталоге нет или
// строка уже есть.
func AddDNSTemplateServer(model *wizardmodels.WizardModel, tag string) bool {
	catalog := templateDNSServersOf(model)
	pos := make(map[string]int, len(catalog))
	var body map[string]interface{}
	for i, s := range catalog {
		t, _ := s["tag"].(string)
		if _, dup := pos[t]; !dup {
			pos[t] = i
		}
		if t == tag && body == nil {
			body = s
		}
	}
	if body == nil {
		return false
	}
	at, last := -1, -1
	for i, raw := range model.DNSServers {
		t := tagFromServerJSON(raw)
		if t == tag {
			return false
		}
		p, ok := pos[t]
		if !ok {
			continue
		}
		last = i
		if at < 0 && p > pos[tag] {
			at = i
		}
	}
	switch {
	case at < 0 && last >= 0:
		at = last + 1
	case at < 0:
		at = 0
	}
	b, err := json.Marshal(body)
	if err != nil {
		return false
	}
	model.DNSServers = append(model.DNSServers, nil)
	copy(model.DNSServers[at+1:], model.DNSServers[at:])
	model.DNSServers[at] = json.RawMessage(b)
	delete(model.DNSTemplateOverrides, tag)
	delete(model.DNSTemplateVars, tag)
	return true
}

// ForgetDNSTemplateServer снимает выбор пользователя по серверу каталога,
// чья строка удалена (SPEC 173): включённость и значения переменных
// принадлежали записи и уходят вместе с ней.
func ForgetDNSTemplateServer(model *wizardmodels.WizardModel, tag string) {
	delete(model.DNSTemplateOverrides, tag)
	delete(model.DNSTemplateVars, tag)
}

// templateDNSServersOf — общий helper для DNSTagLocked / DNSTagFromTemplate.
// Парсит template.DNSOptionsRaw → servers list. Возвращает nil если template нет.
func templateDNSServersOf(model *wizardmodels.WizardModel) []map[string]interface{} {
	if model == nil || model.TemplateData == nil {
		return nil
	}
	if len(model.TemplateData.DNSOptionsRaw) == 0 {
		return nil
	}
	var dnsOpt struct {
		Servers []map[string]interface{} `json:"servers"`
	}
	if err := json.Unmarshal(model.TemplateData.DNSOptionsRaw, &dnsOpt); err != nil {
		return nil
	}
	return dnsOpt.Servers
}

// -----------------------------------------------------------------------------
// JSON helpers
// -----------------------------------------------------------------------------

// parseDNSOptionsMap decodes a DNSOptions blob as the top-level JSON OBJECT it
// is ({"servers":[...],"rules":[...],"strategy":...}), returning its fields as
// a key→raw map for by-key access (optsMap["servers"], ["rules"], ...).
//
// This is deliberately NOT the same as templateDNSServersOf, which unmarshals
// the SAME blob into a struct to pull out just the servers array. The two
// coexist on purpose — different callers want different slices of the object;
// they are not duplicate parsers to merge (SPEC 069 §5.4 — verified correct).
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

func routeDefaultDomainResolver(route map[string]interface{}) string {
	v, ok := route["default_domain_resolver"]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case map[string]interface{}:
		// Объектная форма ядра {server, disable_optimistic_cache, …}.
		s, _ := t["server"].(string)
		return strings.TrimSpace(s)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// -----------------------------------------------------------------------------
// Rules serialization (state ↔ editor text)
// -----------------------------------------------------------------------------

// PersistedDNSRulesForState builds dns_options.rules from the multiline editor text.
// On parse or marshal error returns nil (no rules key in state when empty).
func PersistedDNSRulesForState(rulesText string) []json.RawMessage {
	rulesText = strings.TrimSpace(rulesText)
	if rulesText == "" {
		return nil
	}
	parsed, err := build.ParseDNSRulesText(rulesText)
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

// DNSRulesToText — тонкий re-export `build.DNSRulesToText`. Сохранён
// под этим именем, чтобы не править все callsite'ы пакета (визард / тесты).
// Живая реализация — в `core/build/dns_merge.go` рядом с `ParseDNSRulesText`.
func DNSRulesToText(rules []interface{}) string {
	return build.DNSRulesToText(rules)
}
