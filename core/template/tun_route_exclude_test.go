package template

import (
	"encoding/json"
	"testing"
)

// tunInboundFromTemplate резолвит вары шаблона с заданным state и возвращает
// TUN-инбаунд целиком (секция inbounds с гейтом @tun).
func tunInboundFromTemplate(t *testing.T, raw json.RawMessage, state map[string]string) map[string]interface{} {
	t.Helper()
	var root struct {
		Vars   []TemplateVar   `json:"vars"`
		Params []TemplateParam `json:"params"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatal(err)
	}
	var inbounds json.RawMessage
	for _, p := range root.Params {
		if p.Name != "inbounds" {
			continue
		}
		if deps := p.Gate().Deps(); len(deps) == 1 && deps[0] == "tun" {
			inbounds = p.Value
		}
	}
	if len(inbounds) == 0 {
		t.Fatal("template params: TUN inbounds section not found")
	}
	target := TargetSpec{GOOS: "linux", GOARCH: "amd64"}.Normalized()
	resolved := ResolveTemplateVarsFor(root.Vars, state, raw, target)
	out, err := SubstituteVarsInJSON(inbounds, root.Vars, resolved, target)
	if err != nil {
		t.Fatal(err)
	}
	var list []map[string]interface{}
	if err := json.Unmarshal(out, &list); err != nil || len(list) == 0 {
		t.Fatalf("inbounds is not a non-empty array of objects: %v", err)
	}
	return list[0]
}

// tun_address_exclude (text_list) → tun.route_exclude_address: пустое поле —
// пустой массив (ядро принимает), построчный ввод — по элементу на строку.
// Тест идёт по реальному bin/wizard_template.json: ловит рассинхрон между
// типом вара и формой подстановки в инбаунде (см. issue #97 для tun_address).
func TestTunRouteExcludeAddress(t *testing.T) {
	raw := loadShippedTemplate(t)
	cases := []struct {
		name  string
		state string
		want  []string
	}{
		{"empty", "", []string{}},
		{"two lines", "10.0.0.0/8\n\n fd00::/8 \n", []string{"10.0.0.0/8", "fd00::/8"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := tunInboundFromTemplate(t, raw, map[string]string{"tun": "true", "tun_address_exclude": c.state})
			got, ok := in["route_exclude_address"].([]interface{})
			if !ok {
				t.Fatalf("route_exclude_address: %T %v — want JSON array", in["route_exclude_address"], in["route_exclude_address"])
			}
			if len(got) != len(c.want) {
				t.Fatalf("route_exclude_address = %v, want %v", got, c.want)
			}
			for i, w := range c.want {
				if got[i] != w {
					t.Fatalf("route_exclude_address[%d] = %v, want %q", i, got[i], w)
				}
			}
			b, _ := json.Marshal(in)
			t.Logf("INBOUND %s", b)
		})
	}
}
