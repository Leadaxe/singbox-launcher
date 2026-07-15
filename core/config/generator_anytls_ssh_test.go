package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

// Fake credentials. Verifies the full URI → sing-box anytls outbound JSON path —
// before the anytls branch existed the credential and session-pool tuning were
// silently dropped (only the shared TLS block survived).
func TestGenerateNodeJSON_AnyTLS(t *testing.T) {
	uri := "anytls://smokepass@anytls.example.test:8443?sni=anytls.example.test&insecure=1&idle_session_check_interval=30&idle_session_timeout=60&min_idle_session=2#anytls-smoke"
	node, err := subscription.ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	jsonStr, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("GenerateNodeJSON: %v", err)
	}
	for _, want := range []string{
		`"type":"anytls"`,
		`"server":"anytls.example.test"`,
		`"server_port":8443`,
		`"password":"smokepass"`,
		`"idle_session_check_interval":"30s"`,
		`"idle_session_timeout":"60s"`,
		`"min_idle_session":2`,
		`"server_name":"anytls.example.test"`,
		`"insecure":true`,
	} {
		if !strings.Contains(jsonStr, want) {
			t.Errorf("expected %s in JSON:\n%s", want, jsonStr)
		}
	}

	assertLastLineValidJSON(t, jsonStr, "anytls")
}

// Fake credentials. Verifies the full URI → sing-box ssh outbound JSON path —
// before the ssh branch existed user/password/host-key material were silently
// dropped, leaving a bare {tag,type,server,server_port} object.
func TestGenerateNodeJSON_SSH(t *testing.T) {
	uri := "ssh://deploy@ssh.example.test:2222?password=smokepass&host_key=ssh-ed25519%20AAAAC3fake&client_version=SSH-2.0-smoke#ssh-smoke"
	node, err := subscription.ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("ParseNode: %v", err)
	}
	jsonStr, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("GenerateNodeJSON: %v", err)
	}
	for _, want := range []string{
		`"type":"ssh"`,
		`"server":"ssh.example.test"`,
		`"server_port":2222`,
		`"user":"deploy"`,
		`"password":"smokepass"`,
		`"host_key":["ssh-ed25519 AAAAC3fake"]`,
		`"client_version":"SSH-2.0-smoke"`,
	} {
		if !strings.Contains(jsonStr, want) {
			t.Errorf("expected %s in JSON:\n%s", want, jsonStr)
		}
	}

	assertLastLineValidJSON(t, jsonStr, "ssh")
}

// assertLastLineValidJSON strips the leading comment line and trailing comma
// that GenerateNodeJSON adds and checks the object line parses as strict JSON.
func assertLastLineValidJSON(t *testing.T, jsonStr, scheme string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(jsonStr), "\n")
	jsonLine := strings.TrimSuffix(strings.TrimSpace(lines[len(lines)-1]), ",")
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(jsonLine), &obj); err != nil {
		t.Fatalf("%s object line must be valid JSON: %v\n%s", scheme, err, jsonLine)
	}
}
