package build

import (
	"reflect"
	"testing"

	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// TestRuleSetRemoteEmission — режим remote набора (SPEC 185 §2.4): правило
// kind=srs уезжает в конфиг как type:remote с url, update_interval и
// download_detour; общее значение из переменных действует, пока у правила нет
// своего; недоступный download_detour снимает набор вместе с правилом.
func TestRuleSetRemoteEmission(t *testing.T) {
	const url = "https://github.com/acme/lists/blob/main/ads.srs"
	newRule := func(name string, opts state.RuleSetOptions) state.Rule {
		r := state.NewSrsRule(name, []string{url}, "direct-out")
		r.Enabled = true
		r.SetRuleSetOptions(opts)
		return r
	}
	hours := func(n int) *int { return &n }
	resolve := func(rule state.Rule, globals map[string]string, finalTags map[string]bool) ResolvedRoute {
		st := &state.State{Rules: []state.Rule{rule}}
		return ResolveRouteChecked(st, &template.TemplateData{}, "", nil, template.LocalTarget(), globals, finalTags)
	}
	final := map[string]bool{"direct-out": true, "proxy-out": true}

	t.Run("rule options win", func(t *testing.T) {
		rule := newRule("ads", state.RuleSetOptions{Mode: state.SrsModeRemote, IntervalHours: hours(24), DownloadDetour: "proxy-out"})
		got := resolve(rule, map[string]string{state.VarSrsMode: state.SrsModeLocal}, final)
		if len(got.RuleSets) != 1 || got.RuleSets[0].Skipped || len(got.Rules) != 1 {
			t.Fatalf("resolved: %+v", got)
		}
		want := map[string]interface{}{
			"tag":             "ads:ads", // тег набора — имя правила и stem ссылки (§6.13)
			"type":            "remote",
			"format":          "binary",
			"url":             "https://raw.githubusercontent.com/acme/lists/main/ads.srs",
			"update_interval": "24h",
			"download_detour": "proxy-out",
		}
		if !reflect.DeepEqual(got.RuleSets[0].Body, want) {
			t.Fatalf("rule_set body:\n got %v\nwant %v", got.RuleSets[0].Body, want)
		}
	})

	t.Run("globals apply, zero interval is not written", func(t *testing.T) {
		globals := map[string]string{state.VarSrsMode: state.SrsModeRemote, state.VarSrsUpdateIntervalHours: "0"}
		got := resolve(newRule("ads", state.RuleSetOptions{}), globals, final)
		if len(got.RuleSets) != 1 || got.RuleSets[0].Skipped {
			t.Fatalf("resolved: %+v", got)
		}
		body := got.RuleSets[0].Body
		if body["type"] != "remote" {
			t.Fatalf("type = %v, want remote", body["type"])
		}
		if _, has := body["update_interval"]; has {
			t.Fatalf("update_interval written for interval 0: %v", body)
		}
		if _, has := body["download_detour"]; has {
			t.Fatalf("download_detour written though empty: %v", body)
		}
	})

	t.Run("local mode without a cached file drops the rule", func(t *testing.T) {
		got := resolve(newRule("ads", state.RuleSetOptions{}), nil, final)
		if len(got.Rules) != 0 || len(got.RuleSets) != 1 || !got.RuleSets[0].Skipped {
			t.Fatalf("resolved: %+v", got)
		}
	})

	t.Run("unavailable download_detour drops set and rule", func(t *testing.T) {
		rule := newRule("ads", state.RuleSetOptions{Mode: state.SrsModeRemote, DownloadDetour: "gone"})
		got := resolve(rule, nil, final)
		if len(got.Rules) != 0 || len(got.RuleSets) != 1 || !got.RuleSets[0].Skipped {
			t.Fatalf("resolved: %+v", got)
		}
	})
}
