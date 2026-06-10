package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

// Fake credentials. Verifies the full URI → sing-box tuic outbound JSON path.
func TestGenerateNodeJSON_Tuic(t *testing.T) {
	uri := "tuic://00000000-0000-0000-0000-000000000002:smokepass@tuic.example.test:443/?congestion_control=bbr&alpn=h3,spdy/3.1&udp_relay_mode=native&allow_insecure=1#tuic-smoke"
	node, err := subscription.ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	jsonStr, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("GenerateNodeJSON: %v", err)
	}
	for _, want := range []string{
		`"type":"tuic"`,
		`"server":"tuic.example.test"`,
		`"server_port":443`,
		`"uuid":"00000000-0000-0000-0000-000000000002"`,
		`"password":"smokepass"`,
		`"congestion_control":"bbr"`,
		`"udp_relay_mode":"native"`,
		`"server_name":"tuic.example.test"`,
		`"insecure":true`,
		`"alpn":["h3","spdy/3.1"]`,
	} {
		if !strings.Contains(jsonStr, want) {
			t.Errorf("expected %s in JSON:\n%s", want, jsonStr)
		}
	}

	// The emitted object line must be valid JSON.
	lines := strings.Split(strings.TrimSpace(jsonStr), "\n")
	jsonLine := strings.TrimSuffix(strings.TrimSpace(lines[len(lines)-1]), ",")
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(jsonLine), &obj); err != nil {
		t.Fatalf("tuic object line must be valid JSON: %v\n%s", err, jsonLine)
	}
}
