package template

import (
	"encoding/json"
	"testing"
)

func TestSubstituteVarsInJSON_scalars(t *testing.T) {
	vars := []TemplateVar{
		{Name: "log_level", Type: "enum"},
		{Name: "tun_mtu", Type: "text"},
	}
	resolved := map[string]ResolvedVar{
		"log_level": {Scalar: "info"},
		"tun_mtu":   {Scalar: "1400"},
	}
	raw := json.RawMessage(`{"log":{"level":"@log_level"},"mtu":"@tun_mtu"}`)
	out, err := SubstituteVarsInJSON(raw, vars, resolved)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	log := m["log"].(map[string]interface{})
	if log["level"] != "info" {
		t.Fatalf("log.level: %v", log["level"])
	}
	if m["mtu"] != float64(1400) { // json.Unmarshal numbers default to float64
		t.Fatalf("mtu: %v want 1400", m["mtu"])
	}
}

func TestSubstituteVarsInJSON_bool(t *testing.T) {
	vars := []TemplateVar{{Name: "strict_route", Type: "bool"}, {Name: "auto", Type: "bool"}}
	resolved := map[string]ResolvedVar{
		"strict_route": {Scalar: "true"},
		"auto":         {Scalar: "false"},
	}
	raw := json.RawMessage(`{"strict_route":"@strict_route","auto":"@auto"}`)
	out, err := SubstituteVarsInJSON(raw, vars, resolved)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["strict_route"] != true {
		t.Fatalf("strict_route: %T %v want bool true", m["strict_route"], m["strict_route"])
	}
	if m["auto"] != false {
		t.Fatalf("auto: %T %v want bool false", m["auto"], m["auto"])
	}
}

func TestSubstituteVarsInJSON_proxyInListenPort(t *testing.T) {
	vars := []TemplateVar{{Name: "proxy_in_listen_port", Type: "text"}}
	resolved := map[string]ResolvedVar{
		"proxy_in_listen_port": {Scalar: "7890"},
	}
	raw := json.RawMessage(`{"listen_port":"@proxy_in_listen_port"}`)
	out, err := SubstituteVarsInJSON(raw, vars, resolved)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["listen_port"] != float64(7890) {
		t.Fatalf("listen_port: %T %v want number 7890", m["listen_port"], m["listen_port"])
	}
}

func TestSubstituteVarsInJSON_textList(t *testing.T) {
	vars := []TemplateVar{{Name: "addrs", Type: "text_list"}}
	resolved := map[string]ResolvedVar{
		"addrs": {List: []string{"10.0.0.1/32", "10.0.0.2/32"}},
	}
	raw := json.RawMessage(`{"address":["@addrs"]}`)
	out, err := SubstituteVarsInJSON(raw, vars, resolved)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	arr := m["address"].([]interface{})
	if len(arr) != 2 || arr[0] != "10.0.0.1/32" {
		t.Fatalf("address: %v", m["address"])
	}
}
