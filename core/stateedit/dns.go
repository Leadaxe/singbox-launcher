package stateedit

import (
	"encoding/json"
	"fmt"
	"strings"

	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// Переменные состояния, в которых живут DNS-скаляры (core/build/sync_dns.go).
const (
	varDNSFinal    = "dns_final"
	varDNSResolver = "dns_default_domain_resolver"
)

// AddDNSServer добавляет пользовательский DNS-сервер (kind=user).
//
// Тег — метаданные записи: он берётся из srv.Tag, а при пустом — из
// body.tag, и из тела снимается. Тег обязан быть свободен среди
// dns.servers и серверов шаблона; `body.detour`, если задан, — известное
// корневое имя (KnownRuleTargets). template/preset так не заводятся:
// шаблонный сервер включается в секции dns целиком, пресетный приходит со
// своим правилом.
func AddDNSServer(st *state.State, td *template.TemplateData, srv state.DNSServer) error {
	switch srv.Kind {
	case "", state.DNSServerKindUser:
		srv.Kind = state.DNSServerKindUser
	default:
		return fieldErr("kind", "only kind=user servers can be added; template servers are switched in the whole dns section, preset servers follow their preset rule")
	}
	if len(srv.Body) == 0 {
		return fieldErr("body", "body is required: a sing-box DNS server")
	}
	if srv.Ref != "" {
		return fieldErr("ref", "ref is allowed only on kind=preset")
	}
	if len(srv.Vars) > 0 {
		return fieldErr("vars", "vars are allowed only on kind=template")
	}
	tag := strings.TrimSpace(srv.Tag)
	if bodyTag, ok := srv.Body["tag"].(string); ok {
		bodyTag = strings.TrimSpace(bodyTag)
		if tag != "" && bodyTag != "" && bodyTag != tag {
			return fieldErr("body.tag", "body.tag %q differs from tag %q", bodyTag, tag)
		}
		if tag == "" {
			tag = bodyTag
		}
		delete(srv.Body, "tag")
	}
	if tag == "" {
		return fieldErr("tag", "tag is required")
	}
	if st.DNS.FindServerByTag(tag) >= 0 || templateDNSTags(td)[tag] {
		return fieldErr("tag", "dns server tag %q is taken", tag)
	}
	if detour, _ := srv.Body["detour"].(string); detour != "" {
		if !KnownRuleTargets(st, td)[detour] {
			return fieldErr("body.detour", "unknown detour %q", detour)
		}
	}
	srv.Tag = tag
	st.DNS.Servers = append(st.DNS.Servers, srv)
	return nil
}

// DeleteDNSServerResult — исход удаления DNS-сервера.
type DeleteDNSServerResult struct {
	Tag string `json:"tag"`
	// FinalMovedTo — куда переехал dns_final, если он указывал на удалённый
	// сервер; "" при этом и FinalCleared — включённых серверов не осталось.
	FinalMovedTo    string `json:"final_moved_to,omitempty"`
	FinalCleared    bool   `json:"final_cleared,omitempty"`
	ResolverCleared bool   `json:"resolver_cleared,omitempty"`
	// Dangling — DNS-правила, чей `server` — удалённый тег (не правятся).
	Dangling []string `json:"dangling"`
}

// DeleteDNSServer удаляет пользовательский DNS-сервер. Был он dns_final —
// final переезжает на первый включённый сервер (нет такого — снимается);
// был dns_default_domain_resolver — резолвер снимается. Так же поступает
// Конфигуратор при удалении строки на вкладке DNS.
func DeleteDNSServer(st *state.State, tag string) (*DeleteDNSServerResult, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, fieldErr("tag", "tag is required")
	}
	idx := st.DNS.FindServerByTag(tag)
	if idx < 0 {
		return nil, fmt.Errorf("dns server %q: %w", tag, ErrNotFound)
	}
	if k := st.DNS.Servers[idx].Kind; k != state.DNSServerKindUser {
		return nil, fieldErr("tag", "dns server %q is kind=%s: only kind=user servers are deleted here", tag, k)
	}
	st.DNS.Servers = append(st.DNS.Servers[:idx], st.DNS.Servers[idx+1:]...)
	res := &DeleteDNSServerResult{Tag: tag, Dangling: []string{}}

	firstEnabled := ""
	for _, s := range st.DNS.Servers {
		if s.Enabled && s.Tag != "" && (s.Kind == state.DNSServerKindTemplate || s.Kind == state.DNSServerKindUser) {
			firstEnabled = s.Tag
			break
		}
	}
	finalHit := st.DNS.Final == tag
	if finalHit {
		st.DNS.Final = firstEnabled
	}
	kept := st.Vars[:0:0]
	for _, v := range st.Vars {
		switch {
		case v.Name == varDNSFinal && v.Value == tag:
			finalHit = true
			if firstEnabled == "" {
				continue
			}
			v.Value = firstEnabled
		case v.Name == varDNSResolver && v.Value == tag:
			res.ResolverCleared = true
			continue
		}
		kept = append(kept, v)
	}
	st.Vars = kept
	if st.DNS.DefaultDomainResolver == tag {
		st.DNS.DefaultDomainResolver = ""
		res.ResolverCleared = true
	}
	if finalHit {
		res.FinalMovedTo = firstEnabled
		res.FinalCleared = firstEnabled == ""
	}
	for i, rl := range st.DNS.Rules {
		if s, _ := rl.Body["server"].(string); s == tag {
			res.Dangling = append(res.Dangling, fmt.Sprintf("dns rule #%d server", i))
		}
	}
	return res, nil
}

// AddDNSRule дописывает одно пользовательское DNS-правило sing-box в конец
// dns.rules и возвращает его индекс (тот, что видит GET /state/dns).
func AddDNSRule(st *state.State, rule map[string]interface{}) (int, error) {
	if len(rule) == 0 {
		return 0, fieldErr("rule", "rule is required: one sing-box DNS rule object")
	}
	server, _ := rule["server"].(string)
	action, _ := rule["action"].(string)
	if strings.TrimSpace(server) == "" && strings.TrimSpace(action) == "" {
		return 0, fieldErr("rule", "rule must carry server or action")
	}
	st.DNS.Rules = append(st.DNS.Rules, state.DNSRule{Kind: state.DNSRuleKindUser, Enabled: true, Body: rule})
	return len(st.DNS.Rules) - 1, nil
}

// DeleteDNSRule снимает DNS-правило по индексу в dns.rules. Правило пресета
// так не снимается: оно следует за своим правилом маршрутизации.
func DeleteDNSRule(st *state.State, index int) (state.DNSRule, error) {
	if index < 0 || index >= len(st.DNS.Rules) {
		return state.DNSRule{}, fmt.Errorf("dns rule #%d: %w", index, ErrNotFound)
	}
	rl := st.DNS.Rules[index]
	if rl.Kind != state.DNSRuleKindUser {
		return state.DNSRule{}, fieldErr("index", "dns rule #%d is kind=%s: preset dns rules follow their preset rule", index, rl.Kind)
	}
	st.DNS.Rules = append(st.DNS.Rules[:index], st.DNS.Rules[index+1:]...)
	return rl, nil
}

// templateDNSTags — теги DNS-серверов шаблона (dns_options.servers[].tag).
func templateDNSTags(td *template.TemplateData) map[string]bool {
	out := map[string]bool{}
	if td == nil || len(td.DNSOptionsRaw) == 0 {
		return out
	}
	var opts struct {
		Servers []struct {
			Tag string `json:"tag"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(td.DNSOptionsRaw, &opts); err != nil {
		return out
	}
	for _, s := range opts.Servers {
		if s.Tag != "" {
			out[s.Tag] = true
		}
	}
	return out
}
