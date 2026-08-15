package subscription

import (
	"testing"
)

func TestNodeFromManualConfigJSON_KnownType(t *testing.T) {
	node, err := NodeFromManualConfigJSON([]byte(`{
		"type": "shadowsocks",
		"tag": "ss-hand",
		"server": "1.2.3.4",
		"server_port": 8388,
		"method": "aes-256-gcm",
		"password": "secret"
	}`))
	if err != nil {
		t.Fatalf("NodeFromManualConfigJSON: %v", err)
	}
	// Известный sing-box тип нормализуется в каноническую схему лаунчера —
	// иначе feature-гейты (naive probe, wireguard→endpoints) её не узнают.
	if node.Scheme != "ss" {
		t.Errorf("scheme: got %q, want ss", node.Scheme)
	}
	if node.Tag != "ss-hand" || node.Server != "1.2.3.4" || node.Port != 8388 {
		t.Errorf("scalar fields: tag=%q server=%q port=%d", node.Tag, node.Server, node.Port)
	}
	if node.UUID != "secret" {
		t.Errorf("credential: got %q, want password value", node.UUID)
	}
	if !node.EmitRaw {
		t.Error("manual node must carry EmitRaw")
	}
}

func TestNodeFromManualConfigJSON_UnknownTypeKept(t *testing.T) {
	node, err := NodeFromManualConfigJSON([]byte(`{"type": "futureproto", "endpoint": "wss://x"}`))
	if err != nil {
		t.Fatalf("unknown type must be accepted (that is the point): %v", err)
	}
	if node.Scheme != "futureproto" {
		t.Errorf("scheme: got %q, want futureproto", node.Scheme)
	}
	// Пустой tag получает fallback: пустой тег в config.json валит check.
	if node.Tag == "" {
		t.Error("empty tag must get a fallback")
	}
}

func TestNodeFromManualConfigJSON_Errors(t *testing.T) {
	for name, raw := range map[string]string{
		"empty":        ``,
		"not_json":     `vless://uuid@host:443`,
		"missing_type": `{"server": "x"}`,
		"array":        `[{"type": "vless"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NodeFromManualConfigJSON([]byte(raw)); err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}
