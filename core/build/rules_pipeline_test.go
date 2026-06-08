package build

import (
	"encoding/json"
	"testing"
)

// TestParseTemplateDNSDefaults — парсинг template.dns_options.servers с required/enabled.
func TestParseTemplateDNSDefaults(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{"tag": "google_doh", "type": "https", "enabled": true}`),
		json.RawMessage(`{"tag": "cloudflare_doh", "type": "https", "enabled": false}`),
		json.RawMessage(`{"tag": "implicit", "type": "udp"}`),
		json.RawMessage(`{"tag": "local_dns_resolver", "type": "local", "required": true, "enabled": true}`),
		json.RawMessage(`{"tag": "broken", "type": "local", "required": true, "enabled": false}`),
	}
	defaults := ParseTemplateDNSDefaults(raw)
	if len(defaults) != 5 {
		t.Fatalf("count: %d", len(defaults))
	}
	if !defaults[0].Enabled || defaults[0].Required {
		t.Errorf("google_doh: Enabled=true,Required=false expected, got %+v", defaults[0])
	}
	if defaults[1].Enabled {
		t.Error("cloudflare_doh: Enabled=false expected")
	}
	if !defaults[2].Enabled {
		t.Error("implicit: default Enabled=true expected")
	}
	if !defaults[3].Required || !defaults[3].Enabled {
		t.Errorf("local_dns_resolver: Required=true,Enabled=true expected, got %+v", defaults[3])
	}
	// required + enabled=false → force Enabled=true (coherence fix).
	if !defaults[4].Required || !defaults[4].Enabled {
		t.Errorf("broken required+enabled=false: Enabled should be forced to true, got %+v", defaults[4])
	}
}

// TestValidateTemplateDNSServers — tag-uniqueness + required-enabled coherence warnings.
func TestValidateTemplateDNSServers(t *testing.T) {
	servers := []TemplateDNSServer{
		{Tag: "google_doh", Enabled: true, Raw: map[string]interface{}{"enabled": true}},
		{Tag: "google_doh", Enabled: true, Raw: map[string]interface{}{"enabled": true}},                                            // duplicate
		{Tag: "local_dns_resolver", Required: true, Enabled: true, Raw: map[string]interface{}{"required": true, "enabled": false}}, // coherence warn
	}
	warns := ValidateTemplateDNSServers(servers)
	if len(warns) != 2 {
		t.Errorf("expected 2 warnings (duplicate + coherence), got %d: %v", len(warns), warns)
	}
}
