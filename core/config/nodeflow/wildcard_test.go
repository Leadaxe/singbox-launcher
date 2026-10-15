package nodeflow

import (
	"testing"

	"singbox-launcher/core/config/registry"
)

// TestWildcardNestedStrict — норма wildcard "*" (контракт 1.1.122,
// REGISTRY_DESIGN §2): неописанные ключи объекта с "*" проходят насквозь,
// wildcard НЕ наследуется — описанный вложенный объект без своего "*"
// строг. Плюс эквивалентность прежнего fields_unchecked (openvpn-client:
// order ["*"]) байт в байт и узловой гейт по тегу сборки.
func TestWildcardNestedStrict(t *testing.T) {
	in := func() map[string]interface{} {
		return map[string]interface{}{
			"type":     "openconnect",
			"tag":      "forti",
			"server":   "fw.example.com:443",
			"flavor":   "fortinet",
			"x_future": "<keep&>",
			"tls": map[string]interface{}{
				"server_name":  "fw.example.com",
				"mca_key_path": "/k.pem",
			},
		}
	}

	// 1. Корень с "*" и tls с "*": всё проходит, кодов нет.
	res := SanitizeFrom("openconnect", SourceSingbox, in())
	if res.Drop != nil || len(res.Warnings) != 0 {
		t.Fatalf("tls.*: ждали проход без кодов, got drop=%+v warnings=%+v", res.Drop, res.Warnings)
	}
	body, err := Emit("openconnect", res.Clean)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	want := `{"server":"fw.example.com:443","flavor":"fortinet","tls":{"server_name":"fw.example.com","mca_key_path":"/k.pem"},"x_future":"<keep&>"}`
	if string(body) != want {
		t.Fatalf("эмиссия со слотом *:\n got %s\nwant %s", body, want)
	}

	// 2. Тот же корень, но tls БЕЗ своего "*": wildcard корня внутрь не
	// проникает — tls.mca_key_path получает unknown_key, x_future проходит.
	reg := registry.MustGet()
	b, ok := reg.Body("openconnect")
	if !ok {
		t.Fatal("нет схемы openconnect")
	}
	orig := b.Fields["tls"]
	strict := *orig
	strict.Order = nil
	strict.Fields = map[string]*registry.Field{}
	for _, name := range orig.Order {
		if name == registry.WildcardKey {
			continue
		}
		strict.Order = append(strict.Order, name)
		strict.Fields[name] = orig.Fields[name]
	}
	b.Fields["tls"] = &strict
	defer func() { b.Fields["tls"] = orig }()

	res = SanitizeFrom("openconnect", SourceSingbox, in())
	if res.Drop != nil {
		t.Fatalf("узел снят: %+v", res.Drop)
	}
	if len(res.Warnings) != 1 || res.Warnings[0].Code != "unknown_key" || res.Warnings[0].Path != "tls.mca_key_path" {
		t.Fatalf("tls без *: ждали один unknown_key на tls.mca_key_path, got %+v", res.Warnings)
	}
	if res.Clean["x_future"] != "<keep&>" {
		t.Fatalf("сквозной ключ корня потерян: %+v", res.Clean)
	}
	body, err = Emit("openconnect", res.Clean)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	want = `{"server":"fw.example.com:443","flavor":"fortinet","tls":{"server_name":"fw.example.com"},"x_future":"<keep&>"}`
	if string(body) != want {
		t.Fatalf("эмиссия строгого tls:\n got %s\nwant %s", body, want)
	}
	b.Fields["tls"] = orig

	// 3. Эквивалентность прежнего fields_unchecked: openvpn-client — order
	// ["*"], тело как написано, ключи по алфавиту, снят только tag и type.
	ovpn := map[string]interface{}{
		"type":     "openvpn-client",
		"tag":      "ovpn",
		"username": "user",
		"password": "pass",
		"servers":  []interface{}{map[string]interface{}{"server": "vpn.example.com", "server_port": float64(1194)}},
		"x_future": "<keep&>",
	}
	res = SanitizeFrom("openvpn-client", SourceSingbox, ovpn)
	if res.Drop != nil || len(res.Warnings) != 0 {
		t.Fatalf("openvpn-client: ждали проход без кодов, got drop=%+v warnings=%+v", res.Drop, res.Warnings)
	}
	body, err = Emit("openvpn-client", res.Clean)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}
	want = `{"password":"pass","servers":[{"server":"vpn.example.com","server_port":1194}],"username":"user","x_future":"<keep&>"}`
	if string(body) != want {
		t.Fatalf("openvpn-client тело изменено:\n got %s\nwant %s", body, want)
	}

	// 4. Узловой гейт: ядро без with_openvpn снимает узел, с тегом — нет.
	gateBody := map[string]interface{}{"username": "user"}
	if r := NodeCoreRefusal("openvpn-client", gateBody, CoreInfo{Version: "1.14.2-lx.6", Tags: []string{"with_quic"}}); r == nil || r.Code != "openvpn_core_unsupported" {
		t.Fatalf("ядро без with_openvpn: ждали openvpn_core_unsupported, got %+v", r)
	}
	if r := NodeCoreRefusal("openvpn-client", gateBody, CoreInfo{Version: "1.14.2-lx.6", Tags: []string{"with_openvpn"}}); r != nil {
		t.Fatalf("ядро с with_openvpn сняло узел: %+v", r)
	}
}
