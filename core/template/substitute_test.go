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
	out, err := SubstituteVarsInJSON(raw, vars, resolved, "darwin", "amd64")
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
	out, err := SubstituteVarsInJSON(raw, vars, resolved, "darwin", "amd64")
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
	out, err := SubstituteVarsInJSON(raw, vars, resolved, "darwin", "amd64")
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
	out, err := SubstituteVarsInJSON(raw, vars, resolved, "darwin", "amd64")
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

// ---------------------------------------------------------------------------
// SPEC 067 Phase 1 — #if construct tests
// ---------------------------------------------------------------------------

func substituteHelper(t *testing.T, vars []TemplateVar, resolved map[string]ResolvedVar, body, goos, goarch string) map[string]interface{} {
	t.Helper()
	out, err := SubstituteVarsInJSON(json.RawMessage(body), vars, resolved, goos, goarch)
	if err != nil {
		t.Fatalf("substitute: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return m
}

// Walker semantics -----------------------------------------------------------

func TestIf_MapSpread_AndTrue_NoElse_Merges(t *testing.T) {
	vars := []TemplateVar{{Name: "flag", Type: "bool"}}
	resolved := map[string]ResolvedVar{"flag": {Scalar: "true"}}
	body := `{"type":"mixed","#if":{"and":["@flag"],"value":{"extra":"yes"}}}`
	m := substituteHelper(t, vars, resolved, body, "darwin", "amd64")
	if m["type"] != "mixed" {
		t.Fatalf("type: %v", m["type"])
	}
	if m["extra"] != "yes" {
		t.Fatalf("extra: %v want yes", m["extra"])
	}
	if _, has := m["#if"]; has {
		t.Fatalf("#if key should be removed")
	}
}

func TestIf_MapSpread_AndFalse_NoElse_RemovesKey(t *testing.T) {
	vars := []TemplateVar{{Name: "flag", Type: "bool"}}
	resolved := map[string]ResolvedVar{"flag": {Scalar: "false"}}
	body := `{"type":"mixed","#if":{"and":["@flag"],"value":{"extra":"yes"}}}`
	m := substituteHelper(t, vars, resolved, body, "darwin", "amd64")
	if m["type"] != "mixed" {
		t.Fatalf("type: %v", m["type"])
	}
	if _, has := m["extra"]; has {
		t.Fatalf("extra should not be present")
	}
	if _, has := m["#if"]; has {
		t.Fatalf("#if key should be removed")
	}
}

func TestIf_MapSpread_AndFalse_WithElse_MergesElse(t *testing.T) {
	vars := []TemplateVar{{Name: "flag", Type: "bool"}}
	resolved := map[string]ResolvedVar{"flag": {Scalar: "false"}}
	body := `{"type":"mixed","#if":{"and":["@flag"],"value":{"extra":"yes"},"else":{"extra":"no"}}}`
	m := substituteHelper(t, vars, resolved, body, "darwin", "amd64")
	if m["extra"] != "no" {
		t.Fatalf("extra: %v want no", m["extra"])
	}
	if _, has := m["#if"]; has {
		t.Fatalf("#if key should be removed")
	}
}

func TestIf_ArrayElement_OrTrue_Replaces(t *testing.T) {
	vars := []TemplateVar{{Name: "dark", Type: "bool"}}
	resolved := map[string]ResolvedVar{"dark": {Scalar: "true"}}
	body := `{"items":["always",{"#if":{"or":["@dark"],"value":"extra-dark","else":"extra-light"}},"regular"]}`
	m := substituteHelper(t, vars, resolved, body, "darwin", "amd64")
	arr := m["items"].([]interface{})
	if len(arr) != 3 {
		t.Fatalf("len: %d want 3", len(arr))
	}
	if arr[1] != "extra-dark" {
		t.Fatalf("arr[1]: %v want extra-dark", arr[1])
	}
}

func TestIf_ArrayElement_OrFalse_NoElse_Drops(t *testing.T) {
	vars := []TemplateVar{{Name: "dark", Type: "bool"}}
	resolved := map[string]ResolvedVar{"dark": {Scalar: "false"}}
	body := `{"items":["always",{"#if":{"or":["@dark"],"value":"extra-dark"}},"regular"]}`
	m := substituteHelper(t, vars, resolved, body, "darwin", "amd64")
	arr := m["items"].([]interface{})
	if len(arr) != 2 {
		t.Fatalf("len: %d want 2", len(arr))
	}
	if arr[0] != "always" || arr[1] != "regular" {
		t.Fatalf("items: %v", arr)
	}
}

func TestIf_ArrayElement_OrFalse_WithElse_Replaces(t *testing.T) {
	vars := []TemplateVar{{Name: "dark", Type: "bool"}}
	resolved := map[string]ResolvedVar{"dark": {Scalar: "false"}}
	body := `{"items":["always",{"#if":{"or":["@dark"],"value":"extra-dark","else":"extra-light"}},"regular"]}`
	m := substituteHelper(t, vars, resolved, body, "darwin", "amd64")
	arr := m["items"].([]interface{})
	if len(arr) != 3 {
		t.Fatalf("len: %d want 3", len(arr))
	}
	if arr[1] != "extra-light" {
		t.Fatalf("arr[1]: %v want extra-light", arr[1])
	}
}

func TestIf_Nested_OuterTrueInnerTrue(t *testing.T) {
	vars := []TemplateVar{
		{Name: "outer", Type: "bool"},
		{Name: "inner", Type: "bool"},
	}
	resolved := map[string]ResolvedVar{
		"outer": {Scalar: "true"},
		"inner": {Scalar: "true"},
	}
	body := `{"#if":{"and":["@outer"],"value":{"a":"x","#if":{"and":["@inner"],"value":{"b":"y"}}}}}`
	m := substituteHelper(t, vars, resolved, body, "darwin", "amd64")
	if m["a"] != "x" {
		t.Fatalf("a: %v", m["a"])
	}
	if m["b"] != "y" {
		t.Fatalf("b: %v", m["b"])
	}
}

// Expression language --------------------------------------------------------

func TestIf_Predicate_Equality(t *testing.T) {
	vars := []TemplateVar{{Name: "proto", Type: "text"}}
	cases := []struct {
		scalar string
		want   string
	}{
		{"vless", "matched"},
		{"trojan", "default"},
	}
	for _, tc := range cases {
		resolved := map[string]ResolvedVar{"proto": {Scalar: tc.scalar}}
		body := `{"v":"default","#if":{"and":[{"@proto":"vless"}],"value":{"v":"matched"}}}`
		m := substituteHelper(t, vars, resolved, body, "darwin", "amd64")
		if m["v"] != tc.want {
			t.Fatalf("scalar=%q got v=%v want %s", tc.scalar, m["v"], tc.want)
		}
	}
}

func TestIf_Predicate_NotEmpty_TextVar(t *testing.T) {
	vars := []TemplateVar{{Name: "hostname", Type: "text"}}
	for _, scalar := range []string{"foo", ""} {
		resolved := map[string]ResolvedVar{"hostname": {Scalar: scalar}}
		body := `{"x":"default","#if":{"and":[{"@hostname":"#notEmpty"}],"value":{"x":"matched"}}}`
		m := substituteHelper(t, vars, resolved, body, "darwin", "amd64")
		want := "default"
		if scalar != "" {
			want = "matched"
		}
		if m["x"] != want {
			t.Fatalf("scalar=%q got x=%v want %s", scalar, m["x"], want)
		}
	}
}

func TestIf_Predicate_In_TextList(t *testing.T) {
	vars := []TemplateVar{{Name: "proto", Type: "text"}}
	for _, scalar := range []string{"vless", "http"} {
		resolved := map[string]ResolvedVar{"proto": {Scalar: scalar}}
		body := `{"x":"default","#if":{"and":[{"@proto":{"#in":["vless","trojan"]}}],"value":{"x":"matched"}}}`
		m := substituteHelper(t, vars, resolved, body, "darwin", "amd64")
		want := "default"
		if scalar == "vless" {
			want = "matched"
		}
		if m["x"] != want {
			t.Fatalf("scalar=%q got x=%v want %s", scalar, m["x"], want)
		}
	}
}

func TestIf_Predicate_Matches_Regex(t *testing.T) {
	vars := []TemplateVar{{Name: "host", Type: "text"}}
	for _, scalar := range []string{"abc", "ABC123"} {
		resolved := map[string]ResolvedVar{"host": {Scalar: scalar}}
		body := `{"x":"default","#if":{"and":[{"@host":{"#matches":"^[a-z]+$"}}],"value":{"x":"matched"}}}`
		m := substituteHelper(t, vars, resolved, body, "darwin", "amd64")
		want := "default"
		if scalar == "abc" {
			want = "matched"
		}
		if m["x"] != want {
			t.Fatalf("scalar=%q got x=%v want %s", scalar, m["x"], want)
		}
	}
}

func TestIf_Predicate_Not_BoolVar(t *testing.T) {
	vars := []TemplateVar{{Name: "flag", Type: "bool"}}
	for _, scalar := range []string{"true", "false"} {
		resolved := map[string]ResolvedVar{"flag": {Scalar: scalar}}
		body := `{"x":"default","#if":{"and":[{"#not":"@flag"}],"value":{"x":"matched"}}}`
		m := substituteHelper(t, vars, resolved, body, "darwin", "amd64")
		want := "default"
		if scalar == "false" {
			want = "matched"
		}
		if m["x"] != want {
			t.Fatalf("scalar=%q got x=%v want %s", scalar, m["x"], want)
		}
	}
}

func TestIf_Predicate_Not_Nested_In(t *testing.T) {
	vars := []TemplateVar{{Name: "proto", Type: "text"}}
	for _, scalar := range []string{"http", "vless"} {
		resolved := map[string]ResolvedVar{"proto": {Scalar: scalar}}
		body := `{"x":"default","#if":{"and":[{"#not":{"@proto":{"#in":["vless","trojan"]}}}],"value":{"x":"matched"}}}`
		m := substituteHelper(t, vars, resolved, body, "darwin", "amd64")
		want := "default"
		if scalar == "http" {
			want = "matched"
		}
		if m["x"] != want {
			t.Fatalf("scalar=%q got x=%v want %s", scalar, m["x"], want)
		}
	}
}

func TestIf_Predicate_Not_DoubleNegation(t *testing.T) {
	vars := []TemplateVar{{Name: "flag", Type: "bool"}}
	for _, scalar := range []string{"true", "false"} {
		resolved := map[string]ResolvedVar{"flag": {Scalar: scalar}}
		body := `{"x":"default","#if":{"and":[{"#not":{"#not":"@flag"}}],"value":{"x":"matched"}}}`
		m := substituteHelper(t, vars, resolved, body, "darwin", "amd64")
		want := "default"
		if scalar == "true" {
			want = "matched"
		}
		if m["x"] != want {
			t.Fatalf("scalar=%q got x=%v want %s", scalar, m["x"], want)
		}
	}
}

// Runtime globals (@runtime.platform, @runtime.arch) ------------------------

func TestIf_Predicate_Platform_In(t *testing.T) {
	cases := []struct {
		goos string
		want string
	}{
		{"darwin", "matched"},
		{"linux", "matched"},
		{"windows", "default"},
	}
	for _, tc := range cases {
		body := `{"x":"default","#if":{"and":[{"@runtime.platform":{"#in":["darwin","linux"]}}],"value":{"x":"matched"}}}`
		m := substituteHelper(t, nil, map[string]ResolvedVar{}, body, tc.goos, "amd64")
		if m["x"] != tc.want {
			t.Fatalf("goos=%q got x=%v want %s", tc.goos, m["x"], tc.want)
		}
	}
}

func TestIf_Predicate_Arch_Equality(t *testing.T) {
	cases := []struct {
		goarch string
		want   string
	}{
		{"386", "matched"},
		{"amd64", "default"},
	}
	for _, tc := range cases {
		body := `{"x":"default","#if":{"and":[{"@runtime.arch":"386"}],"value":{"x":"matched"}}}`
		m := substituteHelper(t, nil, map[string]ResolvedVar{}, body, "windows", tc.goarch)
		if m["x"] != tc.want {
			t.Fatalf("goarch=%q got x=%v want %s", tc.goarch, m["x"], tc.want)
		}
	}
}

// TestIf_Predicate_Platform_BareString_Defensive — Phase 1 walker treats bare
// "@runtime.*" in and[] as false (validator catches at load in Phase 2).
func TestIf_Predicate_Platform_BareString_Defensive(t *testing.T) {
	body := `{"x":"default","#if":{"and":["@runtime.platform"],"value":{"x":"matched"}}}`
	m := substituteHelper(t, nil, map[string]ResolvedVar{}, body, "darwin", "amd64")
	if m["x"] != "default" {
		t.Fatalf("bare @runtime.platform predicate should be defensive-false; got x=%v", m["x"])
	}
}
