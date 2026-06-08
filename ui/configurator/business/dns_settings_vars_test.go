package business

import (
	"testing"

	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

func TestMigrateDNSScalarsFromPersistedToSettingsVars_Idempotent(t *testing.T) {
	// SPEC: VarDNSIndependentCache УДАЛЕНО — sing-box 1.14 deprecation.
	vars := []wizardtemplate.TemplateVar{
		{Name: wizardmodels.VarDNSStrategy, Type: "enum"},
		{Name: wizardmodels.VarDNSFinal, Type: "text"},
		{Name: wizardmodels.VarDNSDefaultDomainResolver, Type: "text"},
	}
	st := map[string]string{}
	p := &wizardmodels.PersistedDNSState{
		Strategy:              "prefer_ipv6",
		Final:                 "google_doh",
		DefaultDomainResolver: "direct_dns_resolver",
	}

	MigrateDNSScalarsFromPersistedToSettingsVars(p, st, vars)
	if st[wizardmodels.VarDNSStrategy] != "prefer_ipv6" {
		t.Fatalf("strategy: %v", st)
	}
	MigrateDNSScalarsFromPersistedToSettingsVars(p, st, vars)
	if st[wizardmodels.VarDNSStrategy] != "prefer_ipv6" {
		t.Fatalf("second migrate should not overwrite")
	}
}

func TestMigrateDNSScalarsFromPersisted_DoesNotOverwriteExistingVar(t *testing.T) {
	vars := []wizardtemplate.TemplateVar{{Name: wizardmodels.VarDNSStrategy, Type: "enum"}}
	st := map[string]string{wizardmodels.VarDNSStrategy: "ipv4_only"}
	p := &wizardmodels.PersistedDNSState{Strategy: "prefer_ipv6"}
	MigrateDNSScalarsFromPersistedToSettingsVars(p, st, vars)
	if st[wizardmodels.VarDNSStrategy] != "ipv4_only" {
		t.Fatalf("got %q", st[wizardmodels.VarDNSStrategy])
	}
}
