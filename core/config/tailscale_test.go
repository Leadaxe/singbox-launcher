package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/state"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 122 §4 — приёмка одной задачи одним файлом: импорт+эмиссия узла
// tailnet, гейт ядра по пробе, пул кандидатов Направлений и (когда рядом
// лежит ядро с тегом) реальный `sing-box check` над собранным фрагментом.
//
// Тесты комплексные по замыслу: россыпь юнитов на каждую ветку ловила бы
// правки формулировок, а не поведение.

const tailscaleTestDoc = `{
  "endpoints": [ { "type": "tailscale", "tag": "ts-node", "auth_key": "tskey-auth-example", "accept_routes": true } ],
  "dns": { "servers": [ { "type": "tailscale", "tag": "ts-dns", "endpoint": "@self" } ],
           "rules":   [ { "domain_suffix": [".ts.net"], "server": "ts-dns" } ] },
  "route": { "rules": [ { "ip_cidr": ["100.64.0.0/10"], "outbound": "@self" } ] } }`

func testTailscaleNode(t *testing.T, tag string, body map[string]interface{}) *ParsedNode {
	t.Helper()
	if body == nil {
		body = map[string]interface{}{}
	}
	body["type"] = "tailscale"
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal tailscale body: %v", err)
	}
	var outbound map[string]interface{}
	if err := json.Unmarshal(raw, &outbound); err != nil {
		t.Fatalf("unmarshal tailscale body: %v", err)
	}
	return &ParsedNode{
		Tag: tag, Scheme: "tailscale", Outbound: outbound,
		EmitBody:    json.RawMessage(raw),
		SourceIndex: configtypes.UnsetSourceIndex,
	}
}

func withTailscaleProbe(t *testing.T, probe func() (bool, string)) {
	t.Helper()
	prev := TailscaleSupportProbe
	TailscaleSupportProbe = probe
	t.Cleanup(func() { TailscaleSupportProbe = prev })
}

func withTailscaleStateRoot(t *testing.T, root string) {
	t.Helper()
	prev := TailscaleStateDirRoot()
	SetTailscaleStateDirRoot(root)
	t.Cleanup(func() { SetTailscaleStateDirRoot(prev) })
}

// TestTailscaleEmittedAsEndpoint — §4 п. 1 и §2.1: документ разбирается,
// узел получает схему tailscale, эмитится в endpoints[] (а не в outbounds[]),
// и получает свой каталог состояния — по финальному тегу, с заменой
// недопустимых для имени каталога символов.
func TestTailscaleEmittedAsEndpoint(t *testing.T) {
	body, sections, err := ParseNodeDocument([]byte(tailscaleTestDoc))
	if err != nil {
		t.Fatalf("ParseNodeDocument: %v", err)
	}
	if !NodeBodyGoesToEndpoints(body) {
		t.Fatalf("tailscale body must go to endpoints[], body=%s", body)
	}
	if sections == nil || len(sections.DNSServers()) != 1 || len(sections.DNSRules()) != 1 || len(sections.Rules) != 1 {
		t.Fatalf("секции документа разобраны не полностью: %+v", sections)
	}
	// Ссылки на сам узел переписаны в @self — иначе переименование узла
	// оборвало бы связку.
	if got, _ := sections.DNSServers()[0].Body["endpoint"].(string); got != state.SelfPlaceholder {
		t.Errorf("ссылка DNS-сервера на узел = %q, ожидался %s", got, state.SelfPlaceholder)
	}
	ruleBody, err := sections.Rules[0].DecodeBody()
	if err != nil {
		t.Fatalf("тело правила секции: %v", err)
	}
	if got := ruleBody.(*state.InlineBody).Outbound; got != state.SelfPlaceholder {
		t.Errorf("цель правила секции = %q, ожидался %s", got, state.SelfPlaceholder)
	}

	withTailscaleStateRoot(t, "/opt/lx/bin/tailscale")
	node := testTailscaleNode(t, "ts node/1", map[string]interface{}{"auth_key": "tskey-auth-example"})
	outs, ep, err := EmitNodeJSONs(node)
	if err != nil {
		t.Fatalf("EmitNodeJSONs: %v", err)
	}
	if len(outs) != 0 {
		t.Errorf("узел tailscale уехал в outbounds[]: %v", outs)
	}
	if ep == "" {
		t.Fatal("узел tailscale не попал в endpoints[]")
	}
	var emitted map[string]interface{}
	if err := json.Unmarshal([]byte(ep), &emitted); err != nil {
		t.Fatalf("эмитированный endpoint не JSON: %v\n%s", err, ep)
	}
	if emitted["tag"] != "ts node/1" {
		t.Errorf("tag = %v, want %q", emitted["tag"], "ts node/1")
	}
	if got, want := emitted["state_directory"], "/opt/lx/bin/tailscale/ts_node_1"; got != want {
		t.Errorf("state_directory = %v, want %q", got, want)
	}

	// Явное значение пользователя эмиттер не перебивает.
	explicit := testTailscaleNode(t, "ts2", map[string]interface{}{"state_directory": "/my/own/dir"})
	_, ep2, err := EmitNodeJSONs(explicit)
	if err != nil {
		t.Fatalf("EmitNodeJSONs (explicit state_directory): %v", err)
	}
	if !strings.Contains(ep2, `"/my/own/dir"`) {
		t.Errorf("явный state_directory перебит эмиттером:\n%s", ep2)
	}

	// Голая эмиссия (тело канона, подпись, миграция) путь машины НЕ несёт.
	bare, err := GenerateEndpointJSONBare(node)
	if err != nil {
		t.Fatalf("GenerateEndpointJSONBare: %v", err)
	}
	if strings.Contains(bare, "state_directory") {
		t.Errorf("state_directory просочился в тело узла:\n%s", bare)
	}
}

// TestTailscaleCoreGate — §4 п. 2 и §2.2: на ядре без with_tailscale узел
// выбрасывается с причиной, а остальной конфиг собирается.
func TestTailscaleCoreGate(t *testing.T) {
	const reason = "sing-box core is built without with_tailscale (need 1.14.0-lx.31 or newer)"

	cases := []struct {
		name    string
		probe   func() (bool, string)
		wantOut bool // узел tailscale остаётся в конфиге
		wantN   int  // SkippedTailscaleNodes
	}{
		{"без поддержки — выброшен", func() (bool, string) { return false, reason }, false, 1},
		{"с поддержкой — на месте", func() (bool, string) { return true, "" }, true, 0},
		{"пробы нет — не догадываемся", nil, true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withTailscaleProbe(t, tc.probe)
			withTailscaleStateRoot(t, "")

			pc := naiveDegradeParserConfig()
			nodes := []*ParsedNode{
				testSocksNode("socks-1"),
				testTailscaleNode(t, "ts-node", map[string]interface{}{"auth_key": "tskey-auth-example"}),
			}
			result, err := generateWithCanonicalNodes(t, pc, nodes, DirectionBuildOptions{})
			if err != nil {
				t.Fatalf("GenerateOutboundsFromParserConfig: %v", err)
			}
			if result.SkippedTailscaleNodes != tc.wantN {
				t.Errorf("SkippedTailscaleNodes = %d, want %d", result.SkippedTailscaleNodes, tc.wantN)
			}
			endpoints := strings.Join(result.EndpointsJSON, "\n")
			if got := strings.Contains(endpoints, "ts-node"); got != tc.wantOut {
				t.Errorf("узел в endpoints = %v, want %v:\n%s", got, tc.wantOut, endpoints)
			}
			// Остальной конфиг собран в любом случае — деградация снимает
			// узел, а не сборку.
			if !strings.Contains(strings.Join(result.OutboundsJSON, "\n"), "socks-1") {
				t.Errorf("соседний узел пропал из сборки:\n%v", result.OutboundsJSON)
			}
			if tc.wantN > 0 && !strings.Contains(result.SkippedTailscaleReason, "with_tailscale") {
				t.Errorf("SkippedTailscaleReason = %q, want вердикт пробы", result.SkippedTailscaleReason)
			}
		})
	}
}

// TestTailscaleDirectionPool — §4 п. 5 и §2.3: узел tailnet попадает в пул
// кандидатов Направлений, только когда у него есть непустой exit_node.
func TestTailscaleDirectionPool(t *testing.T) {
	socks := testSocksNode("socks-1")
	socks.SourceIndex = 0
	plain := testTailscaleNode(t, "ts-plain", map[string]interface{}{"auth_key": "k"})
	plain.SourceIndex = 0
	withExit := testTailscaleNode(t, "ts-exit", map[string]interface{}{"auth_key": "k", "exit_node": "gw"})
	withExit.SourceIndex = 0
	blankExit := testTailscaleNode(t, "ts-blank", map[string]interface{}{"auth_key": "k", "exit_node": "   "})
	blankExit.SourceIndex = 0
	// Узел без известного источника подчиняется тому же правилу.
	orphan := testTailscaleNode(t, "ts-orphan", map[string]interface{}{"auth_key": "k"})
	orphan.SourceIndex = configtypes.UnsetSourceIndex

	proxies := []ProxySource{{Canonical: &configtypes.CanonicalSource{FolderID: "F1"}}}
	pool := FilterDirectionCandidatePool(
		[]*ParsedNode{socks, plain, withExit, blankExit, orphan}, proxies)

	got := map[string]bool{}
	for _, n := range pool {
		got[n.Tag] = true
	}
	for _, want := range []string{"socks-1", "ts-exit"} {
		if !got[want] {
			t.Errorf("узел %q обязан быть кандидатом Направления, пул: %v", want, got)
		}
	}
	for _, unwanted := range []string{"ts-plain", "ts-blank", "ts-orphan"} {
		if got[unwanted] {
			t.Errorf("узел %q без exit_node не должен быть кандидатом, пул: %v", unwanted, got)
		}
	}
}

// TestTailscaleConfigPassesSingboxCheck — §4 п. 1, последняя миля: собранный
// из документа фрагмент (endpoint + секции) действительно принимается ядром.
//
// Пропускается, когда рядом нет ядра с тегом with_tailscale: путь к нему
// задаётся переменной SINGBOX_TAILSCALE_BIN — в CI такого ядра нет, а
// подменять проверку сборкой без тега бессмысленно (она и должна падать).
func TestTailscaleConfigPassesSingboxCheck(t *testing.T) {
	bin := strings.TrimSpace(os.Getenv("SINGBOX_TAILSCALE_BIN"))
	if bin == "" {
		t.Skip("SINGBOX_TAILSCALE_BIN не задан — ядра с with_tailscale нет")
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skipf("ядро %s недоступно: %v", bin, err)
	}

	body, sections, err := ParseNodeDocument([]byte(tailscaleTestDoc))
	if err != nil {
		t.Fatalf("ParseNodeDocument: %v", err)
	}
	withTailscaleStateRoot(t, filepath.Join(t.TempDir(), "bin", "tailscale"))

	var outbound map[string]interface{}
	if err := json.Unmarshal(body, &outbound); err != nil {
		t.Fatalf("тело узла: %v", err)
	}
	const finalTag = "ts-node"
	node := &ParsedNode{Tag: finalTag, Scheme: "tailscale", Outbound: outbound, EmitBody: body}
	_, ep, err := EmitNodeJSONs(node)
	if err != nil {
		t.Fatalf("EmitNodeJSONs: %v", err)
	}

	// Записи хранимой формы обратно в куски sing-box (тем же переводом, что
	// рисует вкладку JSON), затем @self → финальный тег той же подстановкой,
	// что и на сборке.
	frags := state.NodeSectionsToSingbox(sections)
	subst := func(list []json.RawMessage) []json.RawMessage {
		out := make([]json.RawMessage, 0, len(list))
		for _, f := range list {
			out = append(out, json.RawMessage(state.SubstituteSelf(f, finalTag)))
		}
		return out
	}
	cfg := map[string]interface{}{
		"log":       map[string]string{"level": "warn"},
		"endpoints": []json.RawMessage{json.RawMessage(ep)},
		"outbounds": []map[string]string{{"type": "direct", "tag": "direct"}},
		"dns": map[string]interface{}{
			"servers": append([]json.RawMessage{json.RawMessage(`{"type":"local","tag":"local-dns"}`)},
				subst(frags.DNSServers)...),
			"rules": subst(frags.DNSRules),
			"final": "local-dns",
		},
		"route": map[string]interface{}{
			"default_domain_resolver": "local-dns",
			"final":                   "direct",
			"rules":                   subst(frags.RouteRules),
		},
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	out, err := exec.Command(bin, "check", "-c", path).CombinedOutput()
	if err != nil {
		t.Fatalf("sing-box check отверг конфиг: %v\n%s\n--- config ---\n%s", err, out, raw)
	}
}
