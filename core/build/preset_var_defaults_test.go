package build

// Отсутствующая переменная шаблона берёт дефолт шаблона ОДНИМ правилом — и в
// секциях конфига (inbounds), и в теле пресета.
//
// Дефект 1.6.0: в состоянии после импорта бэкапа в пустое (новая машина) нет
// непереносимых `tun` и `enable_proxy_in`. Раздел inbounds собирался по
// дефолтам шаблона, а подстановка в правила пресета считала переменные
// неизвестными (false) — sniff и resolve уезжали с `inbound: []`, то есть без
// ограничения по inbound, и check это пропускал.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/muhammadmuzzammil1998/jsonc"

	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

const presetVarDefaultsTemplate = `{
  "parser_config": {"version": 4, "outbounds": []},
  "vars": [
    {"name": "tun", "type": "bool", "default_value": "true"},
    {"name": "enable_proxy_in", "type": "bool", "default_value": "false"},
    {"name": "resolve_strategy", "type": "text", "default_value": "ipv4_only"}
  ],
  "config": {
    "inbounds": [],
    "outbounds": [{"type": "direct", "tag": "direct-out"}],
    "route": {"rules": [], "final": "direct-out"}
  },
  "params": [
    {"name": "inbounds", "#enable": ["@tun"], "value": [{"type": "tun", "tag": "tun-in"}]},
    {"name": "inbounds", "#enable": ["@enable_proxy_in"], "mode": "prepend", "value": [{"type": "mixed", "tag": "proxy-in"}]}
  ],
  "presets": [
    {"id": "traffic-processing", "num": 0, "sortable": false, "default_enabled": true,
     "rules": [
       {"inbound": [{"#if": {"#and": ["@tun"], "#value": "tun-in"}}, {"#if": {"#and": ["@enable_proxy_in"], "#value": "proxy-in"}}],
        "action": "sniff"},
       {"inbound": [{"#if": {"#and": ["@tun"], "#value": "tun-in"}}, {"#if": {"#and": ["@enable_proxy_in"], "#value": "proxy-in"}}],
        "action": "resolve", "strategy": "@resolve_strategy"}
     ]}
  ]
}`

func TestMissingVarTakesTemplateDefaultInPresetsAndInbounds(t *testing.T) {
	td, err := template.ParseTemplateData([]byte(presetVarDefaultsTemplate))
	if err != nil {
		t.Fatalf("ParseTemplateData: %v", err)
	}

	buildWith := func(vars []state.SettingVar) (inbounds []string, sniff, resolve map[string]interface{}) {
		t.Helper()
		s := state.New()
		head := state.NewPresetRule("traffic-processing", nil)
		head.Enabled = true
		zero := 0
		head.Num = &zero
		s.Rules = []state.Rule{head}
		s.Vars = vars
		res, err := BuildConfig(BuildContext{
			Template: td,
			Vars:     stateVarsToMap(s),
			Cache:    &ParsedCache{},
			DNS:      dnsConfigFromState(s),
			Route:    routeConfigFromState(s),
			Target:   TargetSpecFromState(s),
			Preset:   presetContextFromState(s, td),
		})
		if err != nil {
			t.Fatalf("BuildConfig: %v", err)
		}
		var cfg struct {
			Inbounds []struct {
				Tag string `json:"tag"`
			} `json:"inbounds"`
			Route struct {
				Rules []map[string]interface{} `json:"rules"`
			} `json:"route"`
		}
		if err := json.Unmarshal(jsonc.ToJSON(res.ConfigJSON), &cfg); err != nil {
			t.Fatalf("config: %v\n%s", err, res.ConfigJSON)
		}
		for _, in := range cfg.Inbounds {
			inbounds = append(inbounds, in.Tag)
		}
		for _, r := range cfg.Route.Rules {
			switch r["action"] {
			case "sniff":
				sniff = r
			case "resolve":
				resolve = r
			}
		}
		if sniff == nil || resolve == nil {
			t.Fatalf("sniff/resolve не собраны:\n%s", res.ConfigJSON)
		}
		return inbounds, sniff, resolve
	}
	list := func(v interface{}) []string {
		items, _ := v.([]interface{})
		out := make([]string, 0, len(items))
		for _, it := range items {
			if s, ok := it.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	join := func(v []string) string { return "[" + strings.Join(v, " ") + "]" }

	cases := []struct {
		name        string
		vars        []state.SettingVar
		wantInbound []string
		wantResolve string
	}{
		// Новая машина: переменных нет — дефолты шаблона везде.
		{"state without vars", nil, []string{"tun-in"}, "ipv4_only"},
		// Сохранённые значения по-прежнему главнее дефолта.
		{"state with vars", []state.SettingVar{
			{Name: "tun", Value: "false"},
			{Name: "enable_proxy_in", Value: "true"},
			{Name: "resolve_strategy", Value: "prefer_ipv4"},
		}, []string{"proxy-in"}, "prefer_ipv4"},
	}
	for _, c := range cases {
		inbounds, sniff, resolve := buildWith(c.vars)
		if join(inbounds) != join(c.wantInbound) {
			t.Errorf("%s: inbounds %v, ожидалось %v", c.name, inbounds, c.wantInbound)
		}
		if got := list(sniff["inbound"]); join(got) != join(inbounds) {
			t.Errorf("%s: sniff.inbound %v, а inbounds конфига %v — правило и секция разрешили переменные по-разному", c.name, got, inbounds)
		}
		if got := list(resolve["inbound"]); join(got) != join(inbounds) {
			t.Errorf("%s: resolve.inbound %v, а inbounds конфига %v", c.name, got, inbounds)
		}
		if got, _ := resolve["strategy"].(string); got != c.wantResolve {
			t.Errorf("%s: resolve.strategy %q, ожидалось %q", c.name, got, c.wantResolve)
		}
	}
}
