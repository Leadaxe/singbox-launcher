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
		out, _, err := SubstituteVarsInJSONCanon(body, nil, nil, TargetSpec{GOOS: "linux", GOARCH: "amd64", Target: tc.target})
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
	out, _, err := SubstituteVarsInJSONCanon(body, nil, nil, TargetSpec{})
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
	out, _, err := SubstituteVarsInJSONCanon(body, nil, nil, TargetSpec{Target: TargetRemote})
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
		Vars   []TemplateVar              `json:"-"` // DecodeVarContainer
		Config map[string]json.RawMessage `json:"config"`
		Params []TemplateParam            `json:"params"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if vs, _, err := DecodeVarContainer(raw); err != nil {
		t.Fatalf("vars of %s: %v", path, err)
	} else {
		root.Vars = vs
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

	// Без gateway ключа нет вовсе (issue #160): значение, оставшееся в
	// неактивном поле, не должно ограничивать TUN одним интерфейсом. Гейт —
	// по роли (@gateway_mode), а не по платформе, поэтому оси по-прежнему
	// независимы и на macOS-шлюзе ключ остаётся (ниже).
	plain := renderBundledTemplate(t, TargetSpec{GOOS: "linux", GOARCH: "amd64", Target: TargetRemote},
		map[string]string{"tun": "true", "gateway_include_interface": "br-lan"})
	plainTun := tunInbound(t, plain)
	if v, has := plainTun["include_interface"]; has {
		t.Errorf("non-gateway config must not carry include_interface, got %#v", v)
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

// auto_redirect — ответ на issue #129: хост-файрвол Linux (firewalld) молча
// рубит локальный TCP-форвардер стека system; nftables-редирект ядра его
// обходит. Ключ эмитится ТОЛЬКО на Linux и только при auto_route: на других
// платформах ядро его не знает, без auto_route ядро отказывает. Default —
// true на этой машине, false на удалённой (роутеры: работающие деплои не
// трогаем).
func TestBundledTemplate_AutoRedirectLinuxOnly(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec TargetSpec
		vars map[string]string
		want interface{} // nil = ключа быть не должно
	}{
		{"linux local", TargetSpec{GOOS: "linux", GOARCH: "amd64", Target: TargetLocal}, nil, true},
		{"linux remote", TargetSpec{GOOS: "linux", GOARCH: "amd64", Target: TargetRemote}, nil, false},
		{"linux remote opted in", TargetSpec{GOOS: "linux", GOARCH: "amd64", Target: TargetRemote},
			map[string]string{"auto_redirect": "true"}, true},
		{"linux local, auto_route off", TargetSpec{GOOS: "linux", GOARCH: "amd64", Target: TargetLocal},
			map[string]string{"auto_route": "false"}, nil},
		{"darwin local", TargetSpec{GOOS: "darwin", GOARCH: "arm64", Target: TargetLocal}, nil, nil},
		{"windows local", TargetSpec{GOOS: "windows", GOARCH: "amd64", Target: TargetLocal}, nil, nil},
	} {
		tun := tunInbound(t, renderBundledTemplate(t, tc.spec, tc.vars))
		got, ok := tun["auto_redirect"]
		if tc.want == nil {
			if ok {
				t.Errorf("%s: auto_redirect must be absent, got %#v", tc.name, got)
			}
			continue
		}
		if !ok || got != tc.want {
			t.Errorf("%s: auto_redirect want %v, got %#v (present=%v)", tc.name, tc.want, got, ok)
		}
	}
}

// lx.sniff_guard — защита, которую с ядра 1.14.4-lx.34 включает тот, кто
// пишет конфиг (контракт 1.1.170): по умолчанию — обе секции и фильтр
// sni_mismatch, persist только по настройке; секция с методом off не
// пишется, блок без секций — тоже.
func TestBundledTemplate_SniffGuardBlock(t *testing.T) {
	spec := TargetSpec{GOOS: "darwin", GOARCH: "arm64", Target: TargetLocal}
	render := func(vars map[string]string) string {
		lx, _ := renderBundledTemplate(t, spec, vars)["lx"].(map[string]interface{})
		if _, old := lx["quic_sniff_decoy"]; old {
			t.Errorf("vars %v: lx.quic_sniff_decoy must never be written", vars)
		}
		guard, ok := lx["sniff_guard"]
		if !ok {
			return ""
		}
		out, _ := json.Marshal(guard) // ключи карты — по алфавиту
		return string(out)
	}
	const nodes = `"nodes":{"sni_mismatch":true}`
	const tls = `"tls":{"method":"record_fragment"}`
	const quic = `"quic":{"detour_type":{"socks":"never"},"method":"decoy_first_byte"}`
	cases := []struct {
		vars map[string]string
		want string
	}{
		{nil, "{" + nodes + "," + quic + "," + tls + "}"},
		{map[string]string{"sniff_guard_quic": "off"}, "{" + nodes + "," + tls + "}"},
		{map[string]string{"sniff_guard_tls": "off"}, "{" + nodes + "," + quic + "}"},
		{map[string]string{"sniff_guard_tls": "off", "sniff_guard_quic": "off"}, ""},
		// Выключенный фильтр пишется явным false: без ключа ядро берёт true.
		{map[string]string{"sniff_guard_sni_mismatch": "false"}, `{"nodes":{"sni_mismatch":false},` + quic + "," + tls + "}"},
		{
			map[string]string{"sniff_guard_persist": "true", "sniff_guard_tls": "delay"},
			`{` + nodes + `,"quic":{"detour_type":{"socks":"never"},"method":"decoy_first_byte","persist":true},"tls":{"method":"delay","persist":true}}`,
		},
		// Значения снятых переменных приманки на блок не влияют.
		{map[string]string{"quic_sniff_decoy": "detour", "quic_sniff_decoy_send": "always"}, "{" + nodes + "," + quic + "," + tls + "}"},
	}
	for _, c := range cases {
		if got := render(c.vars); got != c.want {
			t.Errorf("vars %v: lx.sniff_guard = %s, want %s", c.vars, got, c.want)
		}
	}
}

// lx.wg — буферы и подключённый сокет WireGuard ядра 1.14.4-lx.36 (контракт
// 1.1.183): ключ пишется только при значении, отличном от дефолта ядра;
// пустые buffers и wg не пишутся. Ключи сна (lx.wg.idle и старые плоские
// имена) десктоп не пишет никогда — сборка без with_lx_idle_suspend от них
// отказывается.
func TestBundledTemplate_WGBlock(t *testing.T) {
	spec := TargetSpec{GOOS: "linux", GOARCH: "arm64", Target: TargetRemote}
	render := func(vars map[string]string) string {
		lx, _ := renderBundledTemplate(t, spec, vars)["lx"].(map[string]interface{})
		wg, ok := lx["wg"]
		if !ok {
			return ""
		}
		if m, _ := wg.(map[string]interface{}); m != nil {
			for _, k := range []string{"idle", "idle_suspend", "idle_suspend_reachable", "idle_teardown", "lazy_build", "build_max", "build_overflow"} {
				if _, bad := m[k]; bad {
					t.Errorf("vars %v: lx.wg.%s must never be written by the desktop", vars, k)
				}
			}
		}
		out, _ := json.Marshal(wg)
		return string(out)
	}
	cases := []struct {
		vars map[string]string
		want string
	}{
		{nil, ""},
		{map[string]string{"wg_read_datagram_count": "128", "wg_send_by_mtu": "true", "wg_connected_socket": "false"}, ""},
		{map[string]string{"wg_read_datagram_count": "8"}, `{"buffers":{"read_datagram_count":8}}`},
		{map[string]string{"wg_send_by_mtu": "false"}, `{"buffers":{"send_by_mtu":false}}`},
		{map[string]string{"wg_connected_socket": "true"}, `{"connected_socket":true}`},
		{
			map[string]string{"wg_read_datagram_count": "1", "wg_send_by_mtu": "false", "wg_connected_socket": "true"},
			`{"buffers":{"read_datagram_count":1,"send_by_mtu":false},"connected_socket":true}`,
		},
	}
	for _, c := range cases {
		if got := render(c.vars); got != c.want {
			t.Errorf("vars %v: lx.wg = %s, want %s", c.vars, got, c.want)
		}
	}
}
