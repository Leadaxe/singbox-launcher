package build

import (
	"testing"

	"singbox-launcher/core/template"
)

// helper: build TemplateData с указанным набором имён vars.
func tdWithVars(names ...string) *template.TemplateData {
	td := &template.TemplateData{}
	for _, n := range names {
		td.Vars = append(td.Vars, template.TemplateVar{Name: n})
	}
	return td
}

// TestApplyDNSScalarsToVars_NilTolerant — nil-аргументы безопасны.
func TestApplyDNSScalarsToVars_NilTolerant(t *testing.T) {
	ApplyDNSScalarsToVars(nil, DNSScalars{}, nil)
	ApplyDNSScalarsToVars(tdWithVars("dns_final"), DNSScalars{}, nil)
	ApplyDNSScalarsToVars(nil, DNSScalars{}, map[string]string{})
}

// TestApplyDNSScalarsToVars_OnlyDeclaredVars — vars НЕ объявленные в шаблоне
// — игнорируются (не пишем в map).
func TestApplyDNSScalarsToVars_OnlyDeclaredVars(t *testing.T) {
	// Шаблон НЕ объявляет dns_strategy.
	td := tdWithVars("dns_final")
	vars := map[string]string{}
	cfg := DNSScalars{Strategy: "ipv4_only", Final: "primary"}
	ApplyDNSScalarsToVars(td, cfg, vars)
	if _, has := vars["dns_strategy"]; has {
		t.Errorf("dns_strategy must NOT be set when not declared: %v", vars)
	}
	if v := vars["dns_final"]; v != "primary" {
		t.Errorf("dns_final: want primary, got %q", v)
	}
}

// TestApplyDNSScalarsToVars_StrategyEmptyDeletes — пустое значение → ключ удаляется.
func TestApplyDNSScalarsToVars_StrategyEmptyDeletes(t *testing.T) {
	td := tdWithVars("dns_strategy")
	vars := map[string]string{"dns_strategy": "ipv4_only"}
	ApplyDNSScalarsToVars(td, DNSScalars{Strategy: ""}, vars)
	if _, has := vars["dns_strategy"]; has {
		t.Errorf("empty Strategy must remove key: %v", vars)
	}
}

// TestApplyDNSScalarsToVars_IndependentCacheTristate — nil/true/false поведение.
func TestApplyDNSScalarsToVars_IndependentCacheTristate(t *testing.T) {
	td := tdWithVars("dns_independent_cache")

	// nil → удаление
	{
		vars := map[string]string{"dns_independent_cache": "true"}
		ApplyDNSScalarsToVars(td, DNSScalars{IndependentCache: nil}, vars)
		if _, has := vars["dns_independent_cache"]; has {
			t.Errorf("nil → delete: %v", vars)
		}
	}

	// true → "true"
	{
		v := true
		vars := map[string]string{}
		ApplyDNSScalarsToVars(td, DNSScalars{IndependentCache: &v}, vars)
		if vars["dns_independent_cache"] != "true" {
			t.Errorf("want true, got %q", vars["dns_independent_cache"])
		}
	}

	// false → "false"
	{
		v := false
		vars := map[string]string{}
		ApplyDNSScalarsToVars(td, DNSScalars{IndependentCache: &v}, vars)
		if vars["dns_independent_cache"] != "false" {
			t.Errorf("want false, got %q", vars["dns_independent_cache"])
		}
	}
}

// TestApplyDNSScalarsToVars_ResolverUnsetDeletes — Unset=true → удаление,
// даже если значение непустое.
func TestApplyDNSScalarsToVars_ResolverUnsetDeletes(t *testing.T) {
	td := tdWithVars("dns_default_domain_resolver")
	vars := map[string]string{"dns_default_domain_resolver": "old"}
	cfg := DNSScalars{DefaultDomainResolver: "newval", DefaultDomainResolverUnset: true}
	ApplyDNSScalarsToVars(td, cfg, vars)
	if _, has := vars["dns_default_domain_resolver"]; has {
		t.Errorf("Unset=true → delete despite value: %v", vars)
	}
}

// TestApplyDNSScalarsToVars_TrimsWhitespace — TrimSpace применяется к значению.
func TestApplyDNSScalarsToVars_TrimsWhitespace(t *testing.T) {
	td := tdWithVars("dns_final")
	vars := map[string]string{}
	ApplyDNSScalarsToVars(td, DNSScalars{Final: "  trimmed   "}, vars)
	if vars["dns_final"] != "trimmed" {
		t.Errorf("expected trimmed, got %q", vars["dns_final"])
	}
}

// TestApplyDNSScalarsToVars_ResolverWhitespaceOnlyDeletes — пробелы → пустое → удаление.
func TestApplyDNSScalarsToVars_ResolverWhitespaceOnlyDeletes(t *testing.T) {
	td := tdWithVars("dns_default_domain_resolver")
	vars := map[string]string{"dns_default_domain_resolver": "old"}
	ApplyDNSScalarsToVars(td, DNSScalars{DefaultDomainResolver: "   "}, vars)
	if _, has := vars["dns_default_domain_resolver"]; has {
		t.Errorf("whitespace-only → delete: %v", vars)
	}
}

// TestTemplateDeclaresVar_SkipsSeparators — отдельная проверка helper'а.
func TestTemplateDeclaresVar_SkipsSeparators(t *testing.T) {
	vars := []template.TemplateVar{
		{Separator: true, Name: "x"}, // separator с тем же именем не считается declaration'ом
		{Name: "y"},
	}
	if templateDeclaresVar(vars, "x") {
		t.Errorf("separator must not count as declaration")
	}
	if !templateDeclaresVar(vars, "y") {
		t.Errorf("'y' must be declared")
	}
}
