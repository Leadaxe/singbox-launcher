package debugapi

// Импорт в пустое состояние воспроизводит цели файла (релиз 1.6.0).
//
// Дефект: POST /backup/import на новой машине выключал правила с целью
// direct-out (backup_unknown_outbound) и не применял route.final на неё.
// Известных целей было два списка: декодер проверял цели правил ДО слияния
// набором «Направления приёмника + Направления, цепочки и свёртки файла», и ни
// он, ни список route.final не видели системных тегов шаблона приёмника. На
// пустом состоянии Направлений у приёмника нет — direct-out, block-out и
// endpoint шаблона оказывались «целью, которой не существует».

import (
	"encoding/json"
	"net/http"
	"testing"

	"singbox-launcher/core/backup"
	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

func TestBackupImportIntoEmptyKeepsRuleTargets(t *testing.T) {
	td := &template.TemplateData{
		ParserConfig: `{"ParserConfig":{"outbounds":[
			{"tag":"proxy-out","type":"selector","addOutbounds":["direct-out"]}
		]}}`,
		// Системные теги шаблона: outbound'ы и endpoint'ы секции config.
		Config: map[string]json.RawMessage{
			"outbounds": json.RawMessage(`[{"type":"direct","tag":"direct-out"},{"type":"block","tag":"block-out"}]`),
			"endpoints": json.RawMessage(`[{"type":"wireguard","tag":"warp-ep"}]`),
		},
		// Пресет объявляет свою цель (SPEC 129 Н2: значение необъявленного
		// имени приёмник снимает), умолчание другое — выбор файла едет.
		Presets: []template.Preset{{ID: "russian", Vars: []template.PresetVar{
			{Name: "out", Type: "outbound", Default: "proxy-out"},
		}}},
	}

	src := state.New()
	src.Directions = []configtypes.Direction{{
		Tag: "vpn-de", Type: "selector", AddOutbounds: []string{"direct-out"},
		Filters: map[string]interface{}{"tag": "/(DE)/i"},
		Auto:    &configtypes.DirectionAuto{URL: "https://cp.example-1.com/generate_204"},
	}}
	match := func(host string) map[string]interface{} {
		return map[string]interface{}{"domain_suffix": []interface{}{host}}
	}
	russian := presetRule("russian", map[string]string{"out": "direct-out"}, true)
	srs := state.NewSrsRule("by-srs", []string{"https://example-1.com/set.srs"}, "vpn-de")
	srs.Enabled = true
	src.Rules = []state.Rule{
		inlineRule("to-direct", match("direct.example-1.com"), "direct-out"),
		inlineRule("to-block-out", match("ads.example-1.com"), "block-out"),
		inlineRule("to-endpoint", match("warp.example-1.com"), "warp-ep"),
		inlineRule("to-reject", match("track.example-1.com"), "reject"),
		inlineRule("to-auto", match("fast.example-1.com"), "vpn-de-auto"),
		srs,
		inlineRule("ghost", match("ghost.example-1.com"), "vpn-9"),
		russian,
	}
	for i := range src.Rules {
		n := state.UserRuleNumStart + i
		src.Rules[i].Num = &n
	}
	src.Vars = []state.SettingVar{{Name: "route_final", Value: "direct-out"}}
	src.DNS.Servers = []state.DNSServer{{
		Kind: state.DNSServerKindUser, Tag: "home-dns", Enabled: true,
		Body: map[string]interface{}{"type": "udp", "server": "192.0.2.53", "detour": "vpn-de"},
	}}

	srcBase, _ := newTestServer(t, &fakeFacade{stateValue: src, templateValue: td})
	file, resp := fetchBackup(t, srcBase, "?format=1.0")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export status %d: %s", resp.StatusCode, file)
	}

	// Новая машина: state.json нет вовсе.
	fresh := &fakeFacade{stateLoadErr: state.ErrNotFound, templateValue: td}
	freshBase, _ := newTestServer(t, fresh)
	var out struct {
		OK       bool                `json:"ok"`
		Warnings []backupWarningView `json:"warnings"`
	}
	if status, raw := doJSON(t, authedReq(t, "POST", freshBase+"/backup/import", file), &out); status != http.StatusOK || !out.OK {
		t.Fatalf("import status %d: %s", status, raw)
	}
	got := fresh.savedState
	if got == nil {
		t.Fatal("импорт не сохранил состояние")
	}

	// Ровно одна мёртвая цель — та, которой нет ни в файле, ни у приёмника.
	unknown := 0
	for _, w := range out.Warnings {
		switch w.Code {
		case backup.WarnBackupUnknownOutbound:
			unknown++
			if w.Detail != "ghost → vpn-9" {
				t.Errorf("backup_unknown_outbound не о той цели: %q", w.Detail)
			}
		case backup.WarnBackupFinalDropped:
			t.Errorf("route.final на direct-out не применён: %+v", w)
		}
	}
	if unknown != 1 {
		t.Errorf("backup_unknown_outbound: %d, ожидался 1 (ghost): %+v", unknown, out.Warnings)
	}

	type want struct {
		enabled bool
		target  string
	}
	expect := map[string]want{
		"to-direct":    {true, "direct-out"},
		"to-block-out": {true, "block-out"},
		"to-endpoint":  {true, "warp-ep"},
		"to-reject":    {true, "reject"},
		"to-auto":      {true, "vpn-de-auto"},
		"by-srs":       {true, "vpn-de"},
		"ghost":        {false, "vpn-9"},
	}
	if len(got.Rules) != len(src.Rules) {
		t.Fatalf("правил после импорта %d, в файле %d", len(got.Rules), len(src.Rules))
	}
	for _, r := range got.Rules {
		if r.Kind == state.RuleKindPreset {
			if !r.Enabled || r.Vars["out"] != "direct-out" {
				t.Errorf("пресет %q: enabled=%v vars=%v — выбор цели пресета не доехал", r.Ref, r.Enabled, r.Vars)
			}
			continue
		}
		w, ok := expect[r.Name]
		if !ok {
			t.Errorf("лишнее правило %q", r.Name)
			continue
		}
		if r.Enabled != w.enabled || ruleTarget(t, r) != w.target {
			t.Errorf("правило %q: enabled=%v цель=%q, ожидалось enabled=%v цель=%q",
				r.Name, r.Enabled, ruleTarget(t, r), w.enabled, w.target)
		}
	}

	final := ""
	for _, v := range got.Vars {
		if v.Name == "route_final" {
			final = v.Value
		}
	}
	if final != "direct-out" {
		t.Errorf("route_final после импорта %q, ожидался direct-out", final)
	}
	if len(got.DNS.Servers) != 1 || got.DNS.Servers[0].Body["detour"] != "vpn-de" {
		t.Errorf("detour DNS-сервера не доехал: %+v", got.DNS.Servers)
	}

	// Тот же набор целей у файла 0.12 (прежние релизы): вход другой, список
	// известных целей тот же.
	legacy := []byte(`{
  "lx_backup": 1,
  "exported_by": {"app": "launcher", "version": "1.5.6", "platform": "darwin"},
  "exported_at": "2026-09-10T00:00:00Z",
  "rules": [
    {"kind": "inline", "name": "legacy-direct", "num": 1000, "outbound": "direct-out", "match": {"domain_suffix": ["a.example-1.com"]}},
    {"kind": "inline", "name": "legacy-block", "num": 1001, "outbound": "block-out", "match": {"domain_suffix": ["b.example-1.com"]}},
    {"kind": "inline", "name": "legacy-ghost", "num": 1002, "outbound": "vpn-9", "match": {"domain_suffix": ["c.example-1.com"]}}
  ],
  "route": {"final": "block-out"}
}`)
	fresh012 := &fakeFacade{stateLoadErr: state.ErrNotFound, templateValue: td}
	base012, _ := newTestServer(t, fresh012)
	var out012 struct {
		Warnings []backupWarningView `json:"warnings"`
	}
	if status, raw := doJSON(t, authedReq(t, "POST", base012+"/backup/import", legacy), &out012); status != http.StatusOK {
		t.Fatalf("import 0.12 status %d: %s", status, raw)
	}
	for _, r := range fresh012.savedState.Rules {
		if wantOn := r.Name != "legacy-ghost"; r.Enabled != wantOn {
			t.Errorf("0.12: правило %q enabled=%v, ожидалось %v (цель %q)", r.Name, r.Enabled, wantOn, ruleTarget(t, r))
		}
	}
	for _, w := range out012.Warnings {
		if w.Code == backup.WarnBackupFinalDropped {
			t.Errorf("0.12: route.final на системный тег шаблона не применён: %+v", w)
		}
	}
}

// ruleTarget — цель записи правила, как её видит вид записи.
func ruleTarget(t *testing.T, r state.Rule) string {
	t.Helper()
	body, err := r.DecodeBody()
	if err != nil {
		t.Fatalf("DecodeBody %q: %v", r.Name, err)
	}
	switch v := body.(type) {
	case *state.InlineBody:
		return v.Outbound
	case *state.SrsBody:
		return v.Outbound
	}
	return ""
}
