package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

// Fake key material. Verifies the full URI → sing-box masque outbound JSON path —
// the wizard WARP (MASQUE) source goes through exactly this emitter, and before
// the masque branch existed the node was cut down to {tag,type,server,server_port},
// failing `sing-box check` with "masque: at least one of ip/ipv6 is required".
func TestGenerateNodeJSON_Masque(t *testing.T) {
	uri := "masque://FAKEPRIVKEYDER0000@162.159.198.2:443?address=172.16.0.2%2F32%2C2606%3A4700%3A110%3A8142%3A1%3A2%3A3%3A4%2F128&idle_timeout=5m&keep_alive=30s&mtu=1280&network=h3&profile=cloudflare&publickey=FAKESERVERPUBDER0000&sni=example.com#masque-smoke"
	node, err := subscription.ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	jsonStr, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("GenerateNodeJSON: %v", err)
	}
	for _, want := range []string{
		`"type":"masque"`,
		`"server":"162.159.198.2"`,
		`"server_port":443`,
		`"private_key":"FAKEPRIVKEYDER0000"`,
		`"public_key":"FAKESERVERPUBDER0000"`,
		`"ip":"172.16.0.2/32"`,
		`"ipv6":"2606:4700:110:8142:1:2:3:4/128"`,
		`"profile":"cloudflare"`,
		`"network":"h3"`,
		`"sni":"example.com"`,
		`"idle_timeout":"5m"`,
		`"keep_alive_period":"30s"`,
		`"mtu":1280`,
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
		t.Fatalf("masque object line must be valid JSON: %v\n%s", err, jsonLine)
	}
}
