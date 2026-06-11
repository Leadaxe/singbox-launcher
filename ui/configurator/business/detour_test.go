package business

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/config/configtypes"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

const none = "(none)"

func modelWithOutbounds(t *testing.T, tags ...string) *wizardmodels.WizardModel {
	t.Helper()
	obs := make([]map[string]interface{}, 0, len(tags))
	for _, tag := range tags {
		obs = append(obs, map[string]interface{}{"tag": tag, "type": "selector"})
	}
	wrap := map[string]interface{}{
		"ParserConfig": map[string]interface{}{"outbounds": obs},
	}
	b, err := json.Marshal(wrap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return &wizardmodels.WizardModel{ParserConfigJSON: string(b)}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestDetourOptions_NoneFirstAndSelected(t *testing.T) {
	m := modelWithOutbounds(t, "proxy", "ru-vpn")
	opts, sel := DetourOptions(m, &configtypes.ProxySource{}, none)
	if len(opts) == 0 || opts[0] != none {
		t.Fatalf("first option must be %q, got %v", none, opts)
	}
	if sel != none {
		t.Errorf("empty DetourTag → selected %q, want %q", sel, none)
	}
	if !contains(opts, "proxy") || !contains(opts, "ru-vpn") {
		t.Errorf("available group tags must be offered, got %v", opts)
	}
}

func TestDetourOptions_ExcludesOwnGroups(t *testing.T) {
	m := modelWithOutbounds(t, "proxy", "ru-vpn")
	src := &configtypes.ProxySource{
		Outbounds: []configtypes.OutboundConfig{{Tag: "my-local-auto", Type: "urltest"}},
	}
	opts, _ := DetourOptions(m, src, none)
	if contains(opts, "my-local-auto") {
		t.Errorf("source's own local group must be excluded, got %v", opts)
	}
}

func TestDetourOptions_DanglingSelectionKept(t *testing.T) {
	m := modelWithOutbounds(t, "proxy")
	src := &configtypes.ProxySource{DetourTag: "ghost-group"} // not in available
	opts, sel := DetourOptions(m, src, none)
	if sel != "ghost-group" {
		t.Errorf("selected = %q, want the dangling tag", sel)
	}
	if !contains(opts, "ghost-group") {
		t.Errorf("dangling selection must stay visible/clearable, got %v", opts)
	}
}
