package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/outboundutil"
)

// stubSRSFile создаёт пустой bin/rule-sets/<tag>.srs внутри execDir.
// Возвращает абсолютный путь до файла. Используется в тестах SPEC 045 фаза 9
// (local-only emit) — реальное содержимое не нужно, только наличие.
func stubSRSFile(t *testing.T, execDir, tag string) string {
	t.Helper()
	dir := filepath.Join(execDir, constants.BinDirName, constants.RuleSetsDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, tag+".srs")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func unmarshalRoute(t *testing.T, raw json.RawMessage) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v\nraw: %s", err, raw)
	}
	return m
}

// TestMergeRouteSection_TemplateRulesPreserved — шаблонные rules / rule_set
// сохраняются (НЕ replace), даже если custom-rules пустые.
func TestMergeRouteSection_TemplateRulesPreserved(t *testing.T) {
	tmpl := json.RawMessage(`{
		"rules": [{"protocol":"dns","action":"hijack-dns"}],
		"rule_set": [{"tag":"ru","type":"inline"}],
		"final": "proxy-out"
	}`)
	got, err := MergeRouteSection(tmpl, RouteConfig{})
	if err != nil {
		t.Fatalf("MergeRouteSection: %v", err)
	}
	m := unmarshalRoute(t, got)

	rules, _ := m["rules"].([]interface{})
	if len(rules) != 1 {
		t.Errorf("template rules must remain: %v", rules)
	}
	rsets, _ := m["rule_set"].([]interface{})
	if len(rsets) != 1 {
		t.Errorf("template rule_set must remain: %v", rsets)
	}
	if got, _ := m["final"].(string); got != "proxy-out" {
		t.Errorf("template final must remain when cfg.FinalOutbound empty: %q", got)
	}
}

// TestMergeRouteSection_CustomRuleAppended — custom rule добавляется в массив,
// outbound материализуется на правиле.
func TestMergeRouteSection_CustomRuleAppended(t *testing.T) {
	cfg := RouteConfig{
		Rules: []RouteRule{
			{
				Enabled:     true,
				Outbound:    "vpn-1",
				PrimaryRule: map[string]interface{}{"domain_suffix": []string{"example.com"}},
			},
		},
	}
	got, err := MergeRouteSection(json.RawMessage(`{"rules":[]}`), cfg)
	if err != nil {
		t.Fatalf("MergeRouteSection: %v", err)
	}
	m := unmarshalRoute(t, got)
	rules, _ := m["rules"].([]interface{})
	if len(rules) != 1 {
		t.Fatalf("expected 1 custom rule, got %d", len(rules))
	}
	r := rules[0].(map[string]interface{})
	if r["outbound"] != "vpn-1" {
		t.Errorf("outbound should be vpn-1, got: %v", r["outbound"])
	}
}

// TestMergeRouteSection_DisabledRuleSkipped — disabled-правила не идут в out.
func TestMergeRouteSection_DisabledRuleSkipped(t *testing.T) {
	cfg := RouteConfig{
		Rules: []RouteRule{
			{Enabled: false, Outbound: "vpn-1", PrimaryRule: map[string]interface{}{"x": "1"}},
		},
	}
	got, err := MergeRouteSection(json.RawMessage(`{}`), cfg)
	if err != nil {
		t.Fatalf("MergeRouteSection: %v", err)
	}
	m := unmarshalRoute(t, got)
	if _, has := m["rules"]; has {
		t.Errorf("disabled rule must not produce rules key: %v", m)
	}
}

// TestMergeRouteSection_RejectAction — outbound=reject → action=reject, без outbound/method.
func TestMergeRouteSection_RejectAction(t *testing.T) {
	cfg := RouteConfig{
		Rules: []RouteRule{
			{
				Enabled:     true,
				Outbound:    "reject",
				PrimaryRule: map[string]interface{}{"domain": "ads.example"},
			},
		},
	}
	got, _ := MergeRouteSection(json.RawMessage(`{}`), cfg)
	m := unmarshalRoute(t, got)
	r := m["rules"].([]interface{})[0].(map[string]interface{})
	if r["action"] != "reject" {
		t.Errorf("action should be reject, got: %v", r["action"])
	}
	if _, has := r["outbound"]; has {
		t.Errorf("outbound must be removed for reject: %v", r)
	}
	if _, has := r["method"]; has {
		t.Errorf("method must NOT be set for plain reject: %v", r)
	}
}

// TestMergeRouteSection_DropAction — outbound=drop → action=reject, method=drop.
func TestMergeRouteSection_DropAction(t *testing.T) {
	cfg := RouteConfig{
		Rules: []RouteRule{
			{Enabled: true, Outbound: "drop", PrimaryRule: map[string]interface{}{"x": "1"}},
		},
	}
	got, _ := MergeRouteSection(json.RawMessage(`{}`), cfg)
	m := unmarshalRoute(t, got)
	r := m["rules"].([]interface{})[0].(map[string]interface{})
	if r["action"] != "reject" || r["method"] != "drop" {
		t.Errorf("drop semantics: want action=reject method=drop, got: %v", r)
	}
}

// TestMergeRouteSection_MultiRules — RouteRule.Rules (несколько подправил) → каждое добавляется.
func TestMergeRouteSection_MultiRules(t *testing.T) {
	cfg := RouteConfig{
		Rules: []RouteRule{
			{
				Enabled:  true,
				Outbound: "vpn-1",
				Rules: []map[string]interface{}{
					{"domain": "a"},
					{"domain": "b"},
				},
			},
		},
	}
	got, _ := MergeRouteSection(json.RawMessage(`{}`), cfg)
	m := unmarshalRoute(t, got)
	rules := m["rules"].([]interface{})
	if len(rules) != 2 {
		t.Errorf("expected 2 rules from RouteRule.Rules, got %d", len(rules))
	}
	for _, r := range rules {
		ro := r.(map[string]interface{})
		if ro["outbound"] != "vpn-1" {
			t.Errorf("each subrule must have outbound=vpn-1: %v", ro)
		}
	}
}

// TestMergeRouteSection_FinalOverride — cfg.FinalOutbound перезаписывает шаблон.
func TestMergeRouteSection_FinalOverride(t *testing.T) {
	tmpl := json.RawMessage(`{"final":"old"}`)
	cfg := RouteConfig{FinalOutbound: "new"}
	got, _ := MergeRouteSection(tmpl, cfg)
	m := unmarshalRoute(t, got)
	if got, _ := m["final"].(string); got != "new" {
		t.Errorf("FinalOutbound must override: got %q", got)
	}
}

// TestMergeRouteSection_OmitDefaultDomainResolver — ключ удаляется при флаге.
func TestMergeRouteSection_OmitDefaultDomainResolver(t *testing.T) {
	tmpl := json.RawMessage(`{"default_domain_resolver":"local"}`)
	cfg := RouteConfig{OmitDefaultDomainResolver: true}
	got, _ := MergeRouteSection(tmpl, cfg)
	m := unmarshalRoute(t, got)
	if _, has := m["default_domain_resolver"]; has {
		t.Errorf("default_domain_resolver must be removed when Omit=true: %v", m)
	}
}

// TestMergeRouteSection_DefaultDomainResolverOverride — переопределение значения.
func TestMergeRouteSection_DefaultDomainResolverOverride(t *testing.T) {
	tmpl := json.RawMessage(`{"default_domain_resolver":"old"}`)
	cfg := RouteConfig{DefaultDomainResolver: "new"}
	got, _ := MergeRouteSection(tmpl, cfg)
	m := unmarshalRoute(t, got)
	if got, _ := m["default_domain_resolver"].(string); got != "new" {
		t.Errorf("DefaultDomainResolver must override: got %q", got)
	}
}

// TestMergeRouteSection_RuleSetTemplatePreserved — шаблонные rule_sets идут вместе с custom.
func TestMergeRouteSection_RuleSetTemplatePreserved(t *testing.T) {
	tmpl := json.RawMessage(`{
		"rule_set": [{"tag":"tmpl","type":"inline"}]
	}`)
	cfg := RouteConfig{
		Rules: []RouteRule{
			{
				Enabled:  true,
				Outbound: "vpn",
				RuleSets: []json.RawMessage{
					json.RawMessage(`{"tag":"custom","type":"inline"}`),
				},
				PrimaryRule: map[string]interface{}{"x": "1"},
			},
		},
	}
	got, _ := MergeRouteSection(tmpl, cfg)
	m := unmarshalRoute(t, got)
	rsets := m["rule_set"].([]interface{})
	if len(rsets) != 2 {
		t.Errorf("expected template + custom rule_set: got %d", len(rsets))
	}
}

// TestMergeRouteSection_OriginalRuleNotMutated — clone, не mutation in-place.
// Регрессия — сейчас тест ловит баг "если outbound=reject, то in-place правило шаблона
// тоже становится reject" если бы copy не было.
func TestMergeRouteSection_OriginalRuleNotMutated(t *testing.T) {
	original := map[string]interface{}{"domain_suffix": []string{"x"}}
	cfg := RouteConfig{
		Rules: []RouteRule{
			{Enabled: true, Outbound: "reject", PrimaryRule: original},
		},
	}
	_, _ = MergeRouteSection(json.RawMessage(`{}`), cfg)
	if _, has := original["action"]; has {
		t.Errorf("original PrimaryRule must NOT be mutated: %v", original)
	}
}

// TestApplyRouteOutbound_Direct — внутренний helper: явно проверим mapping.
func TestApplyRouteOutbound_Direct(t *testing.T) {
	cases := []struct {
		name     string
		in       map[string]interface{}
		outbound string
		check    func(t *testing.T, m map[string]interface{})
	}{
		{
			name:     "named outbound clears action",
			in:       map[string]interface{}{"action": "reject", "method": "drop"},
			outbound: "vpn-1",
			check: func(t *testing.T, m map[string]interface{}) {
				if m["outbound"] != "vpn-1" {
					t.Errorf("outbound: got %v", m["outbound"])
				}
				if _, has := m["action"]; has {
					t.Errorf("action must be cleared: %v", m)
				}
				if _, has := m["method"]; has {
					t.Errorf("method must be cleared: %v", m)
				}
			},
		},
		{
			name:     "empty outbound is no-op",
			in:       map[string]interface{}{"action": "reject"},
			outbound: "",
			check: func(t *testing.T, m map[string]interface{}) {
				if m["action"] != "reject" {
					t.Errorf("empty outbound must NOT touch existing fields: %v", m)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := shallowCopyStringMap(c.in)
			outboundutil.ApplyOutboundToRule(m, c.outbound)
			c.check(t, m)
		})
	}
}

// TestConvertRuleSetToLocalRequired_NoExecDir — без execDir → error
// (нельзя резолвить локальный путь).
func TestConvertRuleSetToLocalRequired_NoExecDir(t *testing.T) {
	in := json.RawMessage(`{"tag":"x","type":"remote","url":"http://..."}`)
	got, err := convertRuleSetToLocalRequired(in, "")
	if err == nil {
		t.Errorf("no execDir → expected error, got value: %v", got)
	}
}

// TestConvertRuleSetToLocalRequired_NonRemoteUntouched — local/inline не трогаем.
func TestConvertRuleSetToLocalRequired_NonRemoteUntouched(t *testing.T) {
	in := json.RawMessage(`{"tag":"x","type":"inline","rules":[]}`)
	got, err := convertRuleSetToLocalRequired(in, "/tmp")
	if err != nil {
		t.Fatalf("non-remote should not error: %v", err)
	}
	m := got.(map[string]interface{})
	if m["type"] != "inline" {
		t.Errorf("non-remote must be untouched: %v", m)
	}
}

// TestConvertRuleSetToLocalRequired_BadJSONReturnsError.
func TestConvertRuleSetToLocalRequired_BadJSONReturnsError(t *testing.T) {
	_, err := convertRuleSetToLocalRequired(json.RawMessage("not json"), "/tmp")
	if err == nil {
		t.Errorf("bad json → expected error")
	}
}

// TestConvertRuleSetToLocalRequired_RemoteFilePresent — remote entry со
// скачанным файлом переписывается на type:local + path. Это основной happy
// path SPEC 045 фаза 9: UI gate уже скачал файл, build pipeline эмитит local.
func TestConvertRuleSetToLocalRequired_RemoteFilePresent(t *testing.T) {
	execDir := t.TempDir()
	expectedPath := stubSRSFile(t, execDir, "ru-blocked-main")

	in := json.RawMessage(`{"tag":"ru-blocked-main","type":"remote","format":"binary","url":"https://example.com/x.srs"}`)
	got, err := convertRuleSetToLocalRequired(in, execDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	m := got.(map[string]interface{})
	if m["type"] != "local" {
		t.Errorf("type must be rewritten to local, got: %v", m["type"])
	}
	if m["format"] != "binary" {
		t.Errorf("format must be binary for local SRS, got: %v", m["format"])
	}
	if m["path"] != expectedPath {
		t.Errorf("path mismatch: want %q, got %q", expectedPath, m["path"])
	}
	if m["tag"] != "ru-blocked-main" {
		t.Errorf("tag must be preserved: %v", m["tag"])
	}
	// URL не должна попасть в emit — sing-box не читает её при type:local,
	// а наличие посторонних полей засоряет config.
	if _, ok := m["url"]; ok {
		t.Errorf("url field must be stripped from local emit: %v", m)
	}
}

// TestConvertRuleSetToLocalRequired_RemoteFileMissing — remote без скачанного
// файла → error (build не должен идти дальше). Safety net на случай если
// UI gate не сработал (manual delete файла, multi-stage переключение).
func TestConvertRuleSetToLocalRequired_RemoteFileMissing(t *testing.T) {
	execDir := t.TempDir()

	in := json.RawMessage(`{"tag":"ru-blocked-main","type":"remote","url":"https://example.com/x.srs"}`)
	_, err := convertRuleSetToLocalRequired(in, execDir)
	if err == nil {
		t.Fatal("expected error on missing file")
	}
	msg := err.Error()
	if !strings.Contains(msg, "ru-blocked-main") {
		t.Errorf("error must mention tag for actionable UX: %v", msg)
	}
	if !strings.Contains(msg, "Configurator") || !strings.Contains(msg, "Rules") {
		t.Errorf("error must point user to Configurator → Rules: %v", msg)
	}
}

// TestConvertRuleSetToLocalRequired_LocalFilePresent — local entry с
// существующим файлом эмитится как есть, без модификаций.
func TestConvertRuleSetToLocalRequired_LocalFilePresent(t *testing.T) {
	execDir := t.TempDir()
	path := stubSRSFile(t, execDir, "x")

	// json.Marshal handles backslash escaping (Windows: `C:\Users\...`).
	// Previous string-concat broke on Windows because `\U`, `\b`, etc.
	// are invalid JSON escape sequences.
	in, err := json.Marshal(map[string]interface{}{
		"tag":    "x",
		"type":   "local",
		"format": "binary",
		"path":   path,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	got, err := convertRuleSetToLocalRequired(in, execDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	m := got.(map[string]interface{})
	if m["type"] != "local" {
		t.Errorf("type must remain local: %v", m["type"])
	}
	if m["path"] != path {
		t.Errorf("path must be preserved as-is: want %q, got %q", path, m["path"])
	}
}

// TestConvertRuleSetToLocalRequired_LocalFileMissing — local entry с
// несуществующим path → error. Покрывает кейс state.json пришёл с другой
// машины с уже-local entry, но файла нет.
func TestConvertRuleSetToLocalRequired_LocalFileMissing(t *testing.T) {
	execDir := t.TempDir()
	missingPath := filepath.Join(execDir, "bin", "rule-sets", "missing.srs")

	// See note in _LocalFilePresent test re: Windows backslash escaping.
	in, err := json.Marshal(map[string]interface{}{
		"tag":    "missing",
		"type":   "local",
		"format": "binary",
		"path":   missingPath,
	})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	_, err = convertRuleSetToLocalRequired(in, execDir)
	if err == nil {
		t.Fatal("expected error on missing local file")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error must mention tag/path: %v", err)
	}
}

// TestConvertRuleSetToLocalRequired_LocalNoPath — local без path → error
// (нечего эмитить).
func TestConvertRuleSetToLocalRequired_LocalNoPath(t *testing.T) {
	execDir := t.TempDir()

	in := json.RawMessage(`{"tag":"x","type":"local","format":"binary"}`)
	_, err := convertRuleSetToLocalRequired(in, execDir)
	if err == nil {
		t.Fatal("expected error when local entry has no path")
	}
}

// TestConvertRuleSetToLocalRequired_RemoteNoTag — remote без tag → error
// (без tag не можем зарезолвить bin/rule-sets/<tag>.srs).
func TestConvertRuleSetToLocalRequired_RemoteNoTag(t *testing.T) {
	execDir := t.TempDir()

	in := json.RawMessage(`{"type":"remote","url":"https://example.com/x.srs"}`)
	_, err := convertRuleSetToLocalRequired(in, execDir)
	if err == nil {
		t.Fatal("expected error when remote entry has no tag")
	}
}

// TestMergeRouteSection_PropagatesRuleSetError — error из
// convertRuleSetToLocalRequired должен прорываться через MergeRouteSection,
// чтобы Caller (RebuildConfigIfDirty) не записал невалидный config.json.
func TestMergeRouteSection_PropagatesRuleSetError(t *testing.T) {
	execDir := t.TempDir() // empty bin/rule-sets/

	tmpl := json.RawMessage(`{"final":"proxy-out"}`)
	cfg := RouteConfig{
		ExecDir: execDir,
		Rules: []RouteRule{
			{
				Enabled:     true,
				Outbound:    "proxy-out",
				PrimaryRule: map[string]interface{}{"rule_set": "ru-blocked-main", "outbound": "proxy-out"},
				RuleSets: []json.RawMessage{
					json.RawMessage(`{"tag":"ru-blocked-main","type":"remote","url":"https://example.com/x.srs"}`),
				},
			},
		},
	}
	_, err := MergeRouteSection(tmpl, cfg)
	if err == nil {
		t.Fatal("expected error from MergeRouteSection when SRS file missing")
	}
	if !strings.Contains(err.Error(), "ru-blocked-main") {
		t.Errorf("error must mention failing tag: %v", err)
	}
}

// TestMergeRouteSection_LocalOnlyInvariant — happy path: remote SRS со
// скачанным файлом → в выходе route.rule_set[] нет ни одного type:remote.
// Гарантия что sing-box получит local-only config (SPEC 045 фаза 9 invariant).
func TestMergeRouteSection_LocalOnlyInvariant(t *testing.T) {
	execDir := t.TempDir()
	stubSRSFile(t, execDir, "ru-blocked-main")
	stubSRSFile(t, execDir, "ads-all")

	tmpl := json.RawMessage(`{
		"rule_set": [{"tag":"ru-domains","type":"inline"}],
		"final": "proxy-out"
	}`)
	cfg := RouteConfig{
		ExecDir: execDir,
		Rules: []RouteRule{
			{
				Enabled:     true,
				Outbound:    "proxy-out",
				PrimaryRule: map[string]interface{}{"rule_set": "ru-blocked-main", "outbound": "proxy-out"},
				RuleSets: []json.RawMessage{
					json.RawMessage(`{"tag":"ru-blocked-main","type":"remote","url":"https://example.com/r.srs"}`),
				},
			},
			{
				Enabled:     true,
				Outbound:    "reject",
				PrimaryRule: map[string]interface{}{"rule_set": "ads-all", "action": "reject"},
				RuleSets: []json.RawMessage{
					json.RawMessage(`{"tag":"ads-all","type":"remote","url":"https://example.com/a.srs"}`),
				},
			},
		},
	}
	got, err := MergeRouteSection(tmpl, cfg)
	if err != nil {
		t.Fatalf("MergeRouteSection: %v", err)
	}

	if strings.Contains(string(got), `"type":"remote"`) || strings.Contains(string(got), `"type": "remote"`) {
		t.Errorf("config must not contain any type:remote rule_set:\n%s", got)
	}

	m := unmarshalRoute(t, got)
	rsets, _ := m["rule_set"].([]interface{})
	if len(rsets) != 3 { // 1 inline + 2 remote→local
		t.Errorf("expected 3 rule_set entries (1 inline + 2 promoted to local), got %d:\n%s", len(rsets), got)
	}
}

// TestMergeRouteSection_FormattedOutputValid — для регресс-проверки: вывод
// — валидный JSON-объект с ожидаемой структурой.
func TestMergeRouteSection_FormattedOutputValid(t *testing.T) {
	cfg := RouteConfig{
		Rules: []RouteRule{
			{Enabled: true, Outbound: "vpn-1", PrimaryRule: map[string]interface{}{"x": "1"}},
		},
		FinalOutbound: "vpn-1",
	}
	got, err := MergeRouteSection(json.RawMessage(`{}`), cfg)
	if err != nil {
		t.Fatalf("MergeRouteSection: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(got)), "{") {
		t.Errorf("output not a JSON object: %s", got)
	}
	if !json.Valid(got) {
		t.Errorf("output not valid JSON: %s", got)
	}
}
