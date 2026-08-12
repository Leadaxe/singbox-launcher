package template

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// tunInbound находит tun-inbound по тегу. Индексом брать нельзя: на darwin
// enable_proxy_in=true по умолчанию, и mixed-инбаунд встаёт ПЕРЕД tun
// (params mode=prepend).
func tunInbound(t *testing.T, cfg map[string]interface{}) map[string]interface{} {
	t.Helper()
	ins, _ := cfg["inbounds"].([]interface{})
	for _, raw := range ins {
		in, ok := raw.(map[string]interface{})
		if ok && in["type"] == "tun" {
			return in
		}
	}
	t.Fatalf("no tun inbound in %v", cfg["inbounds"])
	return nil
}

// Эти тесты держат контракт SPEC 097 на ДВУХ уровнях:
//
//  1. движок — @runtime.target как строковый global в #if;
//  2. bin/wizard_template.json — что реально эмитится для local и remote.
//
// Второй уровень намеренно читает боевой шаблон, а не фикстуру: правки
// template'а — это код продукта, и «clash_api уехал на роутер» должно падать
// здесь, а не у пользователя на роутере.

// --- уровень 1: движок ------------------------------------------------------

func TestRuntimeTargetGlobal(t *testing.T) {
	body := []byte(`{"a":{"#if":{"and":[{"@runtime.target":"remote"}],"value":{"kept":1},"else":{"dropped":1}}}}`)
	for _, tc := range []struct {
		target   string
		wantKey  string
		otherKey string
	}{
		{TargetRemote, "kept", "dropped"},
		{TargetLocal, "dropped", "kept"},
	} {
		out, err := SubstituteVarsInJSON(body, nil, nil, TargetSpec{GOOS: "linux", GOARCH: "amd64", Target: tc.target})
		if err != nil {
			t.Fatalf("%s: %v", tc.target, err)
		}
		var m map[string]map[string]interface{}
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatal(err)
		}
		if _, ok := m["a"][tc.wantKey]; !ok {
			t.Errorf("target=%s: want key %q, got %v", tc.target, tc.wantKey, m["a"])
		}
		if _, ok := m["a"][tc.otherKey]; ok {
			t.Errorf("target=%s: key %q must be absent, got %v", tc.target, tc.otherKey, m["a"])
		}
	}
}

// Пустой Target нормализуется в local: состояния и вызывающие, ничего не
// знающие о SPEC 097, должны вести себя ровно как до него.
func TestZeroTargetIsLocal(t *testing.T) {
	if got := (TargetSpec{}).Normalized(); got.Target != TargetLocal || got.GOOS != runtime.GOOS {
		t.Fatalf("zero TargetSpec must normalize to local/runtime, got %+v", got)
	}
	body := []byte(`{"#if":{"and":[{"@runtime.target":"local"}],"value":{"ok":1}}}`)
	out, err := SubstituteVarsInJSON(body, nil, nil, TargetSpec{})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	_ = json.Unmarshal(out, &m)
	if _, ok := m["ok"]; !ok {
		t.Fatalf("zero target must satisfy target==local, got %v", m)
	}
}

// Bare-форма для runtime-globals запрещена (SPEC 067) — target не исключение.
// Это причина, по которой выбран строковый @runtime.target, а не bool
// @runtime.isRemote: типизировать globals не потребовалось.
func TestRuntimeTargetBareFormRejected(t *testing.T) {
	body := []byte(`{"#if":{"and":["@runtime.target"],"value":{"leaked":1}}}`)
	out, err := SubstituteVarsInJSON(body, nil, nil, TargetSpec{Target: TargetRemote})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	_ = json.Unmarshal(out, &m)
	if _, ok := m["leaked"]; ok {
		t.Fatalf("bare runtime global must evaluate false, got %v", m)
	}
}

// --- уровень 2: боевой шаблон ----------------------------------------------

func renderBundledTemplate(t *testing.T, target TargetSpec, stateVars map[string]string) map[string]interface{} {
	t.Helper()
	// core/template → repo root.
	path := filepath.Join("..", "..", "bin", TemplateFileName)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("bundled template not readable (%v)", err)
	}
	var root struct {
		Vars   []TemplateVar              `json:"vars"`
		Config map[string]json.RawMessage `json:"config"`
		Params []TemplateParam            `json:"params"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	cfg, err := json.Marshal(root.Config)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ApplyTemplateWithVarsFor(cfg, root.Params, root.Vars, stateVars, raw, target)
	if err != nil {
		t.Fatalf("apply (%+v): %v", target, err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// clash_api — локальное допущение: на remote ядром управляет демон lxd
// (gRPC+REST, mTLS), а Clash API там некому и незачем слушать.
func TestBundledTemplate_ClashAPIOnlyLocal(t *testing.T) {
	local := renderBundledTemplate(t, TargetSpec{GOOS: "darwin", GOARCH: "arm64", Target: TargetLocal}, nil)
	if _, ok := local["experimental"].(map[string]interface{})["clash_api"]; !ok {
		t.Errorf("local must keep clash_api: %v", local["experimental"])
	}
	remote := renderBundledTemplate(t, TargetSpec{GOOS: "linux", GOARCH: "amd64", Target: TargetRemote}, nil)
	exp := remote["experimental"].(map[string]interface{})
	if _, ok := exp["clash_api"]; ok {
		t.Errorf("remote must drop clash_api entirely: %v", exp)
	}
	// cache_file живёт вне #if — он нужен обоим таргетам.
	if _, ok := exp["cache_file"]; !ok {
		t.Errorf("remote must keep cache_file: %v", exp)
	}
}

// find_process матчит трафик по процессу-владельцу. Критерий — РОЛЬ, не
// удалённость: у удалённого сервера/mac свои процессы есть и process_name-
// правила там работают; бессмыслен матчинг только для ТРАНЗИТНОГО трафика
// gateway. (Первая версия ветвила по target — ревью-проходка №2 исправила.)
func TestBundledTemplate_FindProcessOffOnlyForGateway(t *testing.T) {
	for _, tc := range []struct {
		name   string
		target string
		vars   map[string]string
		want   bool
	}{
		{"local", TargetLocal, nil, true},
		{"remote server", TargetRemote, nil, true},
		{"remote gateway", TargetRemote, map[string]string{"gateway_mode": "true"}, false},
	} {
		m := renderBundledTemplate(t, TargetSpec{GOOS: "linux", GOARCH: "amd64", Target: tc.target}, tc.vars)
		got := m["route"].(map[string]interface{})["find_process"]
		if got != tc.want {
			t.Errorf("%s: find_process want %v, got %#v", tc.name, tc.want, got)
		}
	}
}

// proxy_in_listen: 0.0.0.0 — только для gateway (клиенты из LAN). Для
// remote-СЕРВЕРА дефолт обязан остаться 127.0.0.1: mixed inbound без
// авторизации на 0.0.0.0 — это открытый прокси в интернет.
func TestBundledTemplate_ProxyListenWideOnlyForGateway(t *testing.T) {
	mixedListen := func(m map[string]interface{}) interface{} {
		for _, raw := range m["inbounds"].([]interface{}) {
			in := raw.(map[string]interface{})
			if in["type"] == "mixed" {
				return in["listen"]
			}
		}
		return nil
	}
	srv := renderBundledTemplate(t, TargetSpec{GOOS: "linux", GOARCH: "amd64", Target: TargetRemote},
		map[string]string{"enable_proxy_in": "true"})
	if got := mixedListen(srv); got != "127.0.0.1" {
		t.Errorf("remote server listen: want 127.0.0.1, got %#v", got)
	}
	gw := renderBundledTemplate(t, TargetSpec{GOOS: "linux", GOARCH: "amd64", Target: TargetRemote},
		map[string]string{"enable_proxy_in": "true", "gateway_mode": "true"})
	if got := mixedListen(gw); got != "0.0.0.0" {
		t.Errorf("gateway listen: want 0.0.0.0, got %#v", got)
	}
}

// Движковый контракт ForTargetIn: default_value видит user-vars, объявленные
// РАНЬШЕ по списку; forward-ссылка не резолвится (false), а не паникует.
func TestDefaultValueSeesEarlierVars(t *testing.T) {
	varsJSON := []byte(`[
		{"name": "flag", "type": "bool", "default_value": "true"},
		{"name": "addr", "type": "text",
		 "default_value": {"default": {"#if": {"and": ["@flag"], "value": "0.0.0.0", "else": "127.0.0.1"}}}},
		{"name": "early", "type": "text",
		 "default_value": {"default": {"#if": {"and": ["@late"], "value": "yes", "else": "no"}}}},
		{"name": "late", "type": "bool", "default_value": "true"}
	]`)
	var vars []TemplateVar
	if err := json.Unmarshal(varsJSON, &vars); err != nil {
		t.Fatal(err)
	}
	resolved := ResolveTemplateVarsFor(vars, nil, nil, LocalTarget())
	if got := resolved["addr"].Scalar; got != "0.0.0.0" {
		t.Errorf("addr must see earlier @flag=true: got %q", got)
	}
	// state-override перебивает дефолт зависимости.
	resolved = ResolveTemplateVarsFor(vars, map[string]string{"flag": "false"}, nil, LocalTarget())
	if got := resolved["addr"].Scalar; got != "127.0.0.1" {
		t.Errorf("addr must see overridden @flag=false: got %q", got)
	}
	// forward-ссылка на @late (объявлен ниже) — false-ветка, без паники.
	if got := resolved["early"].Scalar; got != "no" {
		t.Errorf("forward ref must resolve to else-branch: got %q", got)
	}
}

// Имя TUN-интерфейса задаётся на windows/linux и НЕ задаётся на macOS.
//
// Это условие стояло в шаблоне до SPEC 097, и снятие его сломало локальный
// запуск: macOS сам выдаёт utunN, а любое другое имя ядро отвергает
// («bad tun name: …»). SPEC 097 заменил литерал на var, сохранив условие.
func TestBundledTemplate_TunInterfaceNameByPlatform(t *testing.T) {
	for _, tc := range []struct {
		goos, target string
		want         interface{} // nil = поле не эмитится
	}{
		{"darwin", TargetLocal, nil},
		{"darwin", TargetRemote, nil},
		{"windows", TargetLocal, "singbox-tun0"},
		{"linux", TargetRemote, "lxd-tun0"},
	} {
		m := renderBundledTemplate(t, TargetSpec{GOOS: tc.goos, GOARCH: "amd64", Target: tc.target},
			map[string]string{"tun": "true"})
		tun := tunInbound(t, m)
		if got := tun["interface_name"]; got != tc.want {
			t.Errorf("%s/%s interface_name: want %v, got %#v", tc.goos, tc.target, tc.want, got)
		}
	}
}

// Ключевой роутерный кейс: wifi-с-VPN + обычный wifi. include_interface
// ограничивает TUN конкретными LAN-интерфейсами; без gateway-режима его быть
// не должно вовсе (иначе завернём весь трафик машины).
func TestBundledTemplate_GatewayIncludeInterface(t *testing.T) {
	gw := renderBundledTemplate(t, TargetSpec{GOOS: "linux", GOARCH: "arm64", Target: TargetRemote}, map[string]string{
		"tun":                       "true",
		"gateway_mode":              "true",
		"gateway_include_interface": "br-vpn\nbr-lan",
		"tun_interface_name":        "lxd-tun0",
		"tun_address":               "172.19.0.1/30",
		"tun_mtu":                   "1400",
	})
	tun := tunInbound(t, gw)
	inc, ok := tun["include_interface"].([]interface{})
	if !ok || len(inc) != 2 || inc[0] != "br-vpn" || inc[1] != "br-lan" {
		t.Fatalf("include_interface want [br-vpn br-lan], got %#v", tun["include_interface"])
	}
	// mtu обязан быть числом, а не строкой — sing-box не примет "1400".
	if tun["mtu"] != float64(1400) {
		t.Errorf("mtu must be numeric, got %#v", tun["mtu"])
	}

	// Без gateway ключ присутствует, но пустой: ядро трактует [] как
	// отсутствие поля (везде len(...)>0), поэтому фильтр не применяется.
	// Ключ постоянный намеренно — так роль и платформа остаются
	// независимыми осями (иначе include_interface пришлось бы вложить в
	// платформенную ветку и потерять на macOS).
	plain := renderBundledTemplate(t, TargetSpec{GOOS: "linux", GOARCH: "amd64", Target: TargetRemote},
		map[string]string{"tun": "true"})
	plainTun := tunInbound(t, plain)
	if inc, ok := plainTun["include_interface"].([]interface{}); !ok || len(inc) != 0 {
		t.Errorf("non-gateway include_interface must be empty, got %#v", plainTun["include_interface"])
	}

	// macOS-gateway: имени нет (ядро его не примет), а фильтр интерфейсов
	// работает — оси не связаны.
	mac := renderBundledTemplate(t, TargetSpec{GOOS: "darwin", GOARCH: "arm64", Target: TargetLocal},
		map[string]string{"tun": "true", "gateway_mode": "true", "gateway_include_interface": "en0"})
	macTun := tunInbound(t, mac)
	if _, hasName := macTun["interface_name"]; hasName {
		t.Errorf("macOS must not get interface_name: %v", macTun)
	}
	if inc, ok := macTun["include_interface"].([]interface{}); !ok || len(inc) != 1 || inc[0] != "en0" {
		t.Errorf("macOS gateway must keep include_interface, got %#v", macTun["include_interface"])
	}
}

// set_system_proxy на удалённой машине бессмыслен: там нет пользовательской
// сессии, чей системный прокси мы бы правили.
func TestBundledTemplate_SystemProxyNeverRemote(t *testing.T) {
	m := renderBundledTemplate(t, TargetSpec{GOOS: "darwin", GOARCH: "arm64", Target: TargetRemote},
		map[string]string{"tun": "false", "enable_proxy_in": "true"})
	for _, raw := range m["inbounds"].([]interface{}) {
		in := raw.(map[string]interface{})
		if in["type"] == "mixed" && in["set_system_proxy"] == true {
			t.Fatalf("remote must not set system proxy: %v", in)
		}
	}
}
