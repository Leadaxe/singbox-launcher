package nodeflow

import (
	"encoding/json"
	"testing"
)

// Контракт 1.1.99: у схемы с `fields_unchecked` (openvpn-client) тело едет
// как написано — без кодов, снимаются только tag и type.
func TestFieldsUncheckedOpenVPNClientPassesAsIs(t *testing.T) {
	in := map[string]interface{}{
		"type":     "openvpn-client",
		"tag":      "ovpn",
		"username": "user",
		"password": "pass",
		"servers":  []interface{}{map[string]interface{}{"server": "vpn.example.com", "server_port": float64(1194)}},
		"x_future": "<keep&>",
	}
	res := SanitizeFrom("openvpn-client", SourceSingbox, in)
	if res.Drop != nil {
		t.Fatalf("узел снят: %+v", res.Drop)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("предупреждения на теле без проверки: %+v", res.Warnings)
	}
	if _, ok := res.Clean["tag"]; ok {
		t.Fatal("tag остался в теле")
	}
	if _, ok := res.Clean["type"]; ok {
		t.Fatal("type остался в теле")
	}
	body, err := Emit("openvpn-client", res.Clean)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	want := `{"password":"pass","servers":[{"server":"vpn.example.com","server_port":1194}],"username":"user","x_future":"<keep&>"}`
	if string(body) != want {
		t.Fatalf("тело изменено:\n got %s\nwant %s", body, want)
	}
	var check map[string]interface{}
	if err := json.Unmarshal(body, &check); err != nil {
		t.Fatalf("не JSON: %v", err)
	}
}

// Ядро без with_openvpn снимает узел тем же гейтом, что tailscale.
func TestFieldsUncheckedOpenVPNClientCoreGate(t *testing.T) {
	body := map[string]interface{}{"username": "user"}
	r := NodeCoreRefusal("openvpn-client", body, CoreInfo{Version: "1.14.2-lx.6", Tags: []string{"with_quic"}})
	if r == nil || r.Code != "openvpn_core_unsupported" {
		t.Fatalf("ядро без with_openvpn: ждали openvpn_core_unsupported, got %+v", r)
	}
	if r := NodeCoreRefusal("openvpn-client", body, CoreInfo{Version: "1.14.2-lx.6", Tags: []string{"with_openvpn"}}); r != nil {
		t.Fatalf("ядро с with_openvpn сняло узел: %+v", r)
	}
}
