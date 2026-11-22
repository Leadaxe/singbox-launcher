// File rule_identity_test.go — SPEC 063 unit tests for StableRuleID + sanitizeIDPart.
package state

import (
	"bytes"
	"encoding/json"
	"testing"
)

// SPEC 127 (state v8): имя — поле записи, тело — правило sing-box как есть.
func inlineTestRule(name string) Rule {
	return NewInlineRule(name, map[string]interface{}{"port": []int{443}}, "direct-out")
}

func srsTestRule(name, url string) Rule {
	return NewSrsRule(name, []string{url}, "reject")
}

// TestStableRuleID_AllKinds — для каждого kind возвращает корректную identity.
func TestStableRuleID_AllKinds(t *testing.T) {
	cases := []struct {
		name string
		rule Rule
		want string
	}{
		{
			name: "preset returns Ref",
			rule: NewPresetRule("ru-direct", nil),
			want: "ru-direct",
		},
		{
			name: "inline returns sanitized name",
			rule: inlineTestRule("Firefox VPN"),
			want: "Firefox-VPN",
		},
		{
			name: "srs returns sanitized name",
			rule: srsTestRule("Custom block list", "https://x/y.srs"),
			want: "Custom-block-list",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StableRuleID(tc.rule); got != tc.want {
				t.Errorf("StableRuleID: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestStableRuleID_EdgeCases — empty/unknown/undecodable → "unnamed".
func TestStableRuleID_EdgeCases(t *testing.T) {
	cases := []struct {
		name string
		rule Rule
		want string
	}{
		{
			name: "inline empty name",
			rule: inlineTestRule(""),
			want: "unnamed",
		},
		{
			name: "srs empty name",
			rule: srsTestRule("", "https://x"),
			want: "unnamed",
		},
		{
			name: "unknown kind",
			rule: Rule{Kind: "geosite", Body: json.RawMessage(`{}`)},
			want: "unnamed",
		},
		{
			// В v8 имя не в теле: битое тело идентичности не касается,
			// а безымянное правило по-прежнему "unnamed".
			name: "broken body, no name",
			rule: Rule{Kind: RuleKindInline, Body: json.RawMessage(`{ not json`)},
			want: "unnamed",
		},
		{
			name: "unicode/special chars sanitized to ASCII",
			rule: inlineTestRule("Firefox через VPN!"),
			// не-ASCII strip, "!" strip, пробелы → "-"; символы потеряны —
			// к id добавлен хэш имени.
			want: "Firefox--VPN-" + ruleNameHash("Firefox через VPN!"),
		},
		{
			name: "all special chars → fallback rule",
			rule: inlineTestRule("!@#"),
			want: "rule-" + ruleNameHash("!@#"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StableRuleID(tc.rule); got != tc.want {
				t.Errorf("StableRuleID: got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSanitizeIDPart — sanity для нижнего helper.
func TestSanitizeIDPart(t *testing.T) {
	cases := map[string]string{
		"Hello World":            "Hello-World",
		"Firefox через VPN":      "Firefox--VPN",
		"":                       "rule",
		"name with !@# symbols!": "name-with--symbols",
		"abc_DEF-123":            "abc_DEF-123",
		"!@#$%^":                 "rule",
	}
	for in, want := range cases {
		if got := sanitizeIDPart(in); got != want {
			t.Errorf("sanitizeIDPart(%q): got %q, want %q", in, got, want)
		}
	}
}

// TestStableRuleID_LossyNamesStayDistinct — имена, которые очистка сводила в
// один id (кириллица, знаки), различаются хэшем; имя без потерь держит прежний
// id; сохранённые ссылки на старые теги наборов переписываются.
func TestStableRuleID_LossyNamesStayDistinct(t *testing.T) {
	pairs := [][2]string{
		{"Реклама", "Трекеры"},
		{"Список 1", "Перечень 1"},
		{"Ads", "Ads!"},
	}
	for _, p := range pairs {
		a, b := StableRuleID(srsTestRule(p[0], "https://x/a.srs")), StableRuleID(srsTestRule(p[1], "https://x/b.srs"))
		if a == b {
			t.Errorf("%q and %q share the id %q", p[0], p[1], a)
		}
	}
	for name, want := range map[string]string{"Ads": "Ads", "My Ads": "My-Ads", "abc_DEF-123": "abc_DEF-123"} {
		if got := StableRuleID(srsTestRule(name, "https://x/a.srs")); got != want {
			t.Errorf("lossless name %q: id %q, want %q", name, got, want)
		}
	}
	if a, b := StableRuleID(srsTestRule("Реклама", "u")), StableRuleID(srsTestRule(" Реклама ", "u")); a[len(a)-8:] != b[len(b)-8:] {
		t.Errorf("edge spaces changed the hash: %q vs %q", a, b)
	}

	// Одно правило на старый id — ссылки переписываются.
	ads := NewSrsRule("Реклама", []string{"https://x/a.srs", "https://x/b.srs"}, "reject")
	newID := StableRuleID(ads)
	inline := NewInlineRule("Mix", map[string]interface{}{"rule_set": []string{"user:rule:2", "other"}}, "direct-out")
	rules := []Rule{ads, inline}
	dns := []DNSRule{
		{Kind: DNSRuleKindUser, Body: map[string]interface{}{"rule_set": "user:rule", "server": "a"}},
		{Kind: DNSRuleKindUser, Body: map[string]interface{}{"rule_set": []interface{}{"user:rule:2", "russian:ru-domains"}}},
	}
	if n := MigrateRuleSetTagRefs(rules, dns); n != 3 {
		t.Fatalf("rewritten refs: %d, want 3", n)
	}
	if got := dns[0].Body["rule_set"]; got != "user:"+newID {
		t.Errorf("dns[0] rule_set = %v", got)
	}
	if got := dns[1].Body["rule_set"].([]interface{}); got[0] != "user:"+newID+":2" || got[1] != "russian:ru-domains" {
		t.Errorf("dns[1] rule_set = %v", got)
	}
	if want := `"user:` + newID + `:2"`; !bytes.Contains(rules[1].Body, []byte(want)) {
		t.Errorf("inline body %s lacks %s", rules[1].Body, want)
	}
	if n := MigrateRuleSetTagRefs(rules, dns); n != 0 {
		t.Errorf("second pass rewrote %d refs", n)
	}

	// Два правила на один старый id — адресат неизвестен, ссылка не трогается.
	twins := []Rule{NewSrsRule("Реклама", []string{"https://x/a.srs"}, "reject"), NewSrsRule("Трекеры", []string{"https://x/b.srs"}, "reject")}
	ambiguous := []DNSRule{{Kind: DNSRuleKindUser, Body: map[string]interface{}{"rule_set": "user:rule"}}}
	if n := MigrateRuleSetTagRefs(twins, ambiguous); n != 0 || ambiguous[0].Body["rule_set"] != "user:rule" {
		t.Errorf("ambiguous ref rewritten: n=%d body=%v", n, ambiguous[0].Body)
	}
}
