package core

// SPEC 129 §12.3 п. 1 — байт-в-байт: состояние, записанное до SPEC 129
// (значения переменных шаблонных DNS-серверов в корневых
// `vars.dns_<tag>_<var>`, пресеты с `vars`, равными умолчаниям), собирает тот
// же config.json и после того, как писатель привёл его к нормам записи, —
// а маршрут DNS при этом не теряется.
//
// Опасности, которые ловит тест: (1) сборка из файла без переноса в памяти
// брала бы умолчание сервера — google_udp шёл бы напрямую; (2) писатель,
// снявший корневое имя без переноса или записавший умолчание, менял бы
// конфиг после первого сохранения; (3) снятие ключа пресета, равного
// умолчанию, обязано не менять правило.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/muhammadmuzzammil1998/jsonc"

	"singbox-launcher/core/build"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/events"
	"singbox-launcher/core/services"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
	"singbox-launcher/internal/paths"
	"singbox-launcher/internal/platform"
)

// spec129Launcher — каталог лаунчера во временной папке: боевой шаблон и
// состояние на диске, сборка тем же путём, что кнопка Rebuild
// (RebuildConfigIfDirty, forced). Сети нет: ConfigService не заведён.
type spec129Launcher struct {
	td         *template.TemplateData
	ac         *AppController
	statePath  string
	configPath string
}

func newSpec129Launcher(t *testing.T) *spec129Launcher {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "bin", "wizard_template.json"))
	if err != nil {
		t.Skipf("боевой шаблон недоступен: %v", err)
	}
	td, err := template.ParseTemplateData(raw)
	if err != nil {
		t.Fatalf("шаблон: %v", err)
	}
	execDir := t.TempDir()
	binDir := filepath.Join(execDir, "bin")
	if err := os.MkdirAll(filepath.Join(binDir, "wizard_states"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "wizard_template.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	bus := events.NewMemoryBus()
	ss := services.NewStateService()
	ss.EventBus = bus
	configPath := filepath.Join(binDir, "config.json")
	return &spec129Launcher{
		td: td,
		ac: &AppController{
			FileService:  &services.FileService{Layout: paths.Layout{Data: paths.DataDir(execDir)}, ConfigPath: configPath},
			StateService: ss,
			EventBus:     bus,
		},
		statePath:  platform.GetWizardStatePath(paths.DataDir(execDir)),
		configPath: configPath,
	}
}

func (l *spec129Launcher) rebuild(t *testing.T, stage string) []byte {
	t.Helper()
	if err := l.ac.RebuildConfigIfDirty(true); err != nil {
		t.Fatalf("%s: RebuildConfigIfDirty: %v", stage, err)
	}
	out, err := os.ReadFile(l.configPath)
	if err != nil {
		t.Fatalf("%s: config.json: %v", stage, err)
	}
	return out
}

// spec129ServerSource — корневой узел: без него proxy-out собирался бы пустым
// и выпадал из конфига, а с ним — и маршрут DNS через VPN.
func spec129ServerSource(body string) state.Source {
	src := state.NewServerSource("srv-1", json.RawMessage(body))
	src.ID = state.MakeULID()
	return src
}

func TestSpec129RecordVarsKeepConfigByteIdentical(t *testing.T) {
	l := newSpec129Launcher(t)
	td := l.td

	presetRule := func(ref string, num int, vars map[string]string) state.Rule {
		r := state.NewPresetRule(ref, vars)
		r.Enabled = true
		n := num
		r.Num = &n
		return r
	}
	// Форма до SPEC 129: запись сервера — {kind, tag, enabled}, значения —
	// корневыми склеенными именами. dns_ip у google_udp равен умолчанию,
	// у google_dot — нет; у safe_dns_dot профиль с доменным адресом.
	old := state.New()
	old.Sources = []state.Source{spec129ServerSource(`{"type":"socks","tag":"srv-1","server":"192.0.2.1","server_port":1080}`)}
	old.Directions = []configtypes.Direction{{Tag: "proxy-out", Ref: configtypes.RefTemplate}}
	old.DNS.Servers = []state.DNSServer{
		{Kind: state.DNSServerKindTemplate, Tag: "local_dns_resolver", Enabled: true},
		{Kind: state.DNSServerKindTemplate, Tag: "direct_dns_resolver", Enabled: true},
		{Kind: state.DNSServerKindTemplate, Tag: "google_udp", Enabled: true},
		{Kind: state.DNSServerKindTemplate, Tag: "google_dot", Enabled: true},
		{Kind: state.DNSServerKindTemplate, Tag: "safe_dns_dot", Enabled: true},
	}
	old.Vars = []state.SettingVar{
		// Секреты заданы: сгенерированные на каждой сборке заново, они
		// сделали бы два конфига разными без всякой причины.
		{Name: "clash_secret", Value: "spec129-clash-secret"},
		{Name: "proxy_in_password", Value: "spec129-proxy-password"},
		{Name: "dns_final", Value: "google_udp"},
		{Name: "dns_google_udp_outbound", Value: "proxy-out"},
		{Name: "dns_google_udp_dns_ip", Value: "8.8.8.8"},
		{Name: "dns_google_dot_dns_ip", Value: "8.8.4.4"},
		{Name: "dns_safe_dns_dot_safe_profile", Value: "dns.adguard-dns.com"},
		// Сервера cloudflare_dot в записях нет: перенос создаёт запись с
		// включённостью по умолчанию шаблона, и конфиг от этого не меняется.
		{Name: "dns_cloudflare_dot_dns_ip", Value: "1.0.0.1"},
		{Name: "route_final", Value: "proxy-out"},
	}
	old.Rules = []state.Rule{
		presetRule("private-ips", 950, map[string]string{"out": "direct-out"}),
		presetRule("local-lan-domains", 955, map[string]string{"out": "proxy-out", "stale": "x"}),
	}
	if err := old.Save(l.statePath); err != nil {
		t.Fatalf("Save старой формы: %v", err)
	}

	before := l.rebuild(t, "старая форма")

	// Маршрут DNS доехал до конфига из корневой формы: без переноса в памяти
	// google_udp взял бы умолчание direct-out, и detour не было бы вовсе.
	var cfg struct {
		DNS struct {
			Servers []map[string]interface{} `json:"servers"`
		} `json:"dns"`
	}
	if err := json.Unmarshal(jsonc.ToJSON(before), &cfg); err != nil {
		t.Fatalf("разбор конфига: %v", err)
	}
	servers := map[string]map[string]interface{}{}
	for _, srv := range cfg.DNS.Servers {
		if tag, _ := srv["tag"].(string); tag != "" {
			servers[tag] = srv
		}
	}
	if got := servers["google_udp"]; got["detour"] != "proxy-out" || got["server"] != "8.8.8.8" {
		t.Errorf("google_udp в конфиге %v, ожидались detour proxy-out и server 8.8.8.8", got)
	}
	if got := servers["google_dot"]; got["server"] != "8.8.4.4" {
		t.Errorf("google_dot в конфиге %v, ожидался server 8.8.4.4", got)
	}
	if got := servers["safe_dns_dot"]; got["server"] != "dns.adguard-dns.com" {
		t.Errorf("safe_dns_dot в конфиге %v, ожидался профиль AdGuard", got)
	}

	// Писатель (UI, импорт, экспорт, debug API — одна функция) приводит
	// состояние к нормам и сохраняет, как у пользователя.
	written, err := state.Load(l.statePath)
	if err != nil {
		t.Fatalf("Load старой формы: %v", err)
	}
	values := map[string]string{}
	for _, v := range written.Vars {
		values[v.Name] = v.Value
	}
	state.ApplyRecordVars(written, template.RecordVarDeclsFor(td, values, build.TargetSpecFromState(written)))
	if err := written.Save(l.statePath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	after := l.rebuild(t, "после записи")
	if !bytes.Equal(before, after) {
		t.Errorf("config.json изменился после записи состояния:\n--- до ---\n%s\n--- после ---\n%s", before, after)
	}

	// Форма после записи: значения — в записях, без умолчаний и сирот,
	// корневых склеенных имён нет.
	reloaded, err := state.Load(l.statePath)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, v := range reloaded.Vars {
		switch v.Name {
		case "dns_google_udp_outbound", "dns_google_udp_dns_ip", "dns_google_dot_dns_ip",
			"dns_safe_dns_dot_safe_profile", "dns_cloudflare_dot_dns_ip":
			t.Errorf("корневое %s пережило запись", v.Name)
		}
	}
	want := map[string]map[string]string{
		"google_udp":     {"outbound": "proxy-out"},
		"google_dot":     {"dns_ip": "8.8.4.4"},
		"safe_dns_dot":   {"safe_profile": "dns.adguard-dns.com"},
		"cloudflare_dot": {"dns_ip": "1.0.0.1"},
	}
	seen := map[string]bool{}
	for _, srv := range reloaded.DNS.Servers {
		if srv.Kind != state.DNSServerKindTemplate {
			continue
		}
		if seen[srv.Tag] {
			t.Errorf("запись %s задвоена", srv.Tag)
		}
		seen[srv.Tag] = true
		if !stringMapsEqual(srv.Vars, want[srv.Tag]) {
			t.Errorf("запись %s: vars %v, ожидалось %v", srv.Tag, srv.Vars, want[srv.Tag])
		}
	}
	if !seen["cloudflare_dot"] {
		t.Error("корневое имя сервера без записи не создало запись — значение потеряно")
	}
	for _, r := range reloaded.Rules {
		switch r.Ref {
		case "private-ips":
			if len(r.Vars) != 0 {
				t.Errorf("private-ips: vars %v — умолчание записано", r.Vars)
			}
		case "local-lan-domains":
			if !stringMapsEqual(r.Vars, map[string]string{"out": "proxy-out"}) {
				t.Errorf("local-lan-domains: vars %v, ожидалось только out=proxy-out", r.Vars)
			}
		}
	}
}

func stringMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// SPEC 129 §12.3 п. 3 — вторая линия fail-closed на сборке (Н10): шаблонный
// DNS-сервер, чей канал указывает на тег, которого в конфиге нет, не
// эмитится, и ни одна ссылка на него не уводит резолв мимо выбранного
// маршрута и не роняет старт ядра.
func TestSpec129DanglingDNSRouteFailsClosed(t *testing.T) {
	l := newSpec129Launcher(t)

	st := state.New()
	st.Sources = []state.Source{spec129ServerSource(
		`{"type":"socks","tag":"srv-1","server":"vpn.example-1.com","server_port":1080,"domain_resolver":"google_udp"}`)}
	st.Directions = []configtypes.Direction{{Tag: "proxy-out", Ref: configtypes.RefTemplate}}
	st.DNS.Servers = []state.DNSServer{
		{Kind: state.DNSServerKindTemplate, Tag: "local_dns_resolver", Enabled: true},
		{Kind: state.DNSServerKindTemplate, Tag: "direct_dns_resolver", Enabled: true},
		// Канал — Направление, которого больше нет (переименовали, удалили).
		{Kind: state.DNSServerKindTemplate, Tag: "google_udp", Enabled: true,
			Vars: map[string]string{"outbound": "vpn-gone"}},
		// Резолвер доменного адреса AdGuard — выпавший сервер.
		{Kind: state.DNSServerKindTemplate, Tag: "safe_dns_dot", Enabled: true,
			Vars: map[string]string{"safe_profile": "dns.adguard-dns.com", "outbound": "proxy-out"}},
	}
	st.DNS.Rules = []state.DNSRule{{
		Kind: state.DNSRuleKindUser, Enabled: true,
		Body: map[string]interface{}{"domain_suffix": []interface{}{".corp.example-1.com"}, "server": "google_udp", "strategy": "ipv4_only"},
	}}
	st.Vars = []state.SettingVar{
		{Name: "clash_secret", Value: "spec129-clash-secret"},
		{Name: "proxy_in_password", Value: "spec129-proxy-password"},
		{Name: "dns_final", Value: "google_udp"},
		{Name: "dns_default_domain_resolver", Value: "google_udp"},
		{Name: "route_final", Value: "proxy-out"},
	}
	if err := st.Save(l.statePath); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out := l.rebuild(t, "висячий канал")

	var cfg struct {
		DNS struct {
			Servers []map[string]interface{} `json:"servers"`
			Rules   []map[string]interface{} `json:"rules"`
			Final   *string                  `json:"final"`
		} `json:"dns"`
		Outbounds []map[string]interface{} `json:"outbounds"`
		Route     map[string]interface{}   `json:"route"`
	}
	if err := json.Unmarshal(jsonc.ToJSON(out), &cfg); err != nil {
		t.Fatalf("разбор конфига: %v", err)
	}
	emitted := map[string]map[string]interface{}{}
	for _, srv := range cfg.DNS.Servers {
		if tag, _ := srv["tag"].(string); tag != "" {
			emitted[tag] = srv
		}
	}
	if srv, ok := emitted["google_udp"]; ok {
		t.Errorf("google_udp с висячим каналом эмитирован: %v — резолв ушёл бы мимо выбранного маршрута", srv)
	}
	if got := emitted["safe_dns_dot"]; got["domain_resolver"] != l.td.DefaultDomainResolver {
		t.Errorf("safe_dns_dot.domain_resolver %v, ожидалась замена на %q", got["domain_resolver"], l.td.DefaultDomainResolver)
	}
	if cfg.DNS.Final != nil {
		t.Errorf("dns.final %q остался — без правила-заглушки запросы ушли бы на первый сервер", *cfg.DNS.Final)
	}
	var corp, last map[string]interface{}
	for _, r := range cfg.DNS.Rules {
		if suffix, ok := r["domain_suffix"].([]interface{}); ok && len(suffix) == 1 && suffix[0] == ".corp.example-1.com" {
			corp = r
		}
		last = r
	}
	if corp == nil || corp["action"] != "reject" || corp["server"] != nil || corp["strategy"] != nil {
		t.Errorf("правило на выпавший сервер %v, ожидалось action=reject без ключей маршрута", corp)
	}
	if last == nil || len(last) != 1 || last["action"] != "reject" {
		t.Errorf("последнее DNS-правило %v, ожидалась заглушка {action: reject}", last)
	}
	if got := cfg.Route["default_domain_resolver"]; got != l.td.DefaultDomainResolver {
		t.Errorf("route.default_domain_resolver %v, ожидалась замена на %q", got, l.td.DefaultDomainResolver)
	}
	for _, ob := range cfg.Outbounds {
		if ob["tag"] == "srv-1" && ob["domain_resolver"] != l.td.DefaultDomainResolver {
			t.Errorf("srv-1.domain_resolver %v, ожидалась замена на %q", ob["domain_resolver"], l.td.DefaultDomainResolver)
		}
	}

	// Форма, которую ядро примет: отдельный процесс `sing-box check`
	// установленного ядра (живое ядро он не трогает).
	bin := strings.TrimSpace(os.Getenv("SINGBOX_TEST_CORE"))
	if bin == "" {
		bin = "/Applications/singbox-launcher.app/Contents/MacOS/bin/sing-box"
	}
	if info, err := os.Stat(bin); err != nil || info.IsDir() {
		t.Logf("sing-box check пропущен: ядра %s нет", bin)
		return
	}
	if res, err := exec.Command(bin, "check", "-c", l.configPath).CombinedOutput(); err != nil {
		t.Errorf("sing-box check отверг конфиг: %v\n%s", err, res)
	}
}
