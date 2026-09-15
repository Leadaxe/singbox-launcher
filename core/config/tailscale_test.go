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
	"singbox-launcher/core/config/subscription"
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

// TestTailscaleCanonicalBundle — НОРМА СВЯЗКИ целиком, все четыре пункта
// NODE_SECTIONS.md §6 в одном тесте (норма принята с LxBox 14.09.2026).
//
// Тест один на четыре пункта намеренно: пункты — это одна договорённость, и
// россыпь мелких тестов на каждый развалилась бы на первой же правке
// формулировок, не поймав того, ради чего норма писалась — расхождения трёх
// входов между собой.
func TestTailscaleCanonicalBundle(t *testing.T) {
	// Норма (1): состав связки и ОДНО правило маршрута сразу с обоими
	// признаками. Разнесение на два правила — тихая потеря трафика при
	// FakeIP, поэтому число правил проверяется явно.
	t.Run("состав: одно правило с domain_suffix и обеими подсетями", func(t *testing.T) {
		sections, err := state.TailscaleCanonicalSections()
		if err != nil {
			t.Fatalf("TailscaleCanonicalSections: %v", err)
		}
		if n := len(sections.Rules); n != 1 {
			t.Fatalf("правил маршрута %d, норма (1) требует ровно одно", n)
		}
		body, err := sections.Rules[0].DecodeBody()
		if err != nil {
			t.Fatalf("тело правила: %v", err)
		}
		inline, ok := body.(*state.InlineBody)
		if !ok {
			t.Fatalf("правило связки вида %T, ожидался inline", body)
		}
		if inline.Outbound != state.SelfPlaceholder {
			t.Errorf("цель правила = %q, ожидался %s", inline.Outbound, state.SelfPlaceholder)
		}
		raw, err := json.Marshal(inline.Match)
		if err != nil {
			t.Fatalf("match правила: %v", err)
		}
		var match map[string]interface{}
		if err := json.Unmarshal(raw, &match); err != nil {
			t.Fatalf("match правила: %v", err)
		}
		// Имена не матчатся одним ip_cidr при FakeIP — суффикс обязан быть
		// в ТОМ ЖЕ правиле.
		if got := toStringList(match["domain_suffix"]); len(got) != 1 || got[0] != state.TailscaleMagicDNSSuffix {
			t.Errorf("domain_suffix правила = %v, ожидался [%s]", got, state.TailscaleMagicDNSSuffix)
		}
		gotCIDR := toStringList(match["ip_cidr"])
		wantCIDR := []string{state.TailscaleCGNATRange, state.TailscaleCGNATRange6}
		if strings.Join(gotCIDR, ",") != strings.Join(wantCIDR, ",") {
			t.Errorf("ip_cidr правила = %v, ожидались обе подсети tailnet %v", gotCIDR, wantCIDR)
		}

		// DNS-сервер и DNS-правило: тег сервера локален (`@{self}-dns`), и
		// правило метит именно в него.
		if n := len(sections.DNSServers()); n != 1 {
			t.Fatalf("DNS-серверов %d, ожидался один", n)
		}
		srv := sections.DNSServers()[0]
		if srv.Tag != state.TailscaleDNSServerTag() {
			t.Errorf("тег DNS-сервера = %q, ожидался %q", srv.Tag, state.TailscaleDNSServerTag())
		}
		if got, _ := srv.Body["type"].(string); got != "tailscale" {
			t.Errorf("тип DNS-сервера = %q, ожидался tailscale", got)
		}
		if got, _ := srv.Body["endpoint"].(string); got != state.SelfPlaceholder {
			t.Errorf("endpoint DNS-сервера = %q, ожидался %s", got, state.SelfPlaceholder)
		}
		if n := len(sections.DNSRules()); n != 1 {
			t.Fatalf("DNS-правил %d, ожидалось одно", n)
		}
		if got, _ := sections.DNSRules()[0].Body["server"].(string); got != state.TailscaleDNSServerTag() {
			t.Errorf("DNS-правило метит в %q, ожидался тег сервера связки", got)
		}
	})

	// Норма (1), вторая сторона: КОНСТРУКТОР отдаёт ту же связку. Разойдись
	// он с общей функцией — и узел из формы отличался бы от узла из конфига.
	t.Run("конструктор отдаёт ту же связку", func(t *testing.T) {
		canonical, err := state.TailscaleCanonicalSections()
		if err != nil {
			t.Fatalf("TailscaleCanonicalSections: %v", err)
		}
		// Документ конструктора собирается теми же фрагментами; проверяем,
		// что разбор документа даёт канонические записи байт-в-байт.
		frags := state.TailscaleBundleFragments()
		fromDoc, err := state.NodeSectionsFromSingbox(frags)
		if err != nil {
			t.Fatalf("NodeSectionsFromSingbox: %v", err)
		}
		want, _ := json.Marshal(canonical)
		got, _ := json.Marshal(fromDoc)
		if string(want) != string(got) {
			t.Errorf("связка конструктора разошлась с канонической:\n got=%s\nwant=%s", got, want)
		}
	})

	// Норма (2): голый узел получает связку по умолчанию, а НЕ голый —
	// остаётся со своей. Чужую связку подменять нельзя: это настройка.
	t.Run("голый узел получает связку, чужая не трогается", func(t *testing.T) {
		bare := state.ApplyTailscaleDefaultSections(true, nil)
		if bare.IsEmpty() {
			t.Fatal("голый узел tailnet остался без связки — норма (2) нарушена")
		}
		// Не-tailscale узел связку не получает никогда.
		if got := state.ApplyTailscaleDefaultSections(false, nil); !got.IsEmpty() {
			t.Errorf("узел не-tailnet получил связку tailnet: %+v", got)
		}
		// Своя связка пользователя переживает подстановку.
		own, err := state.NodeSectionsFromSingbox(state.SingboxNodeFragments{
			DNSServers: []json.RawMessage{json.RawMessage(
				`{"type":"tailscale","tag":"mine","endpoint":"@self"}`)},
		})
		if err != nil {
			t.Fatalf("своя связка: %v", err)
		}
		kept := state.ApplyTailscaleDefaultSections(true, own)
		if len(kept.DNSServers()) != 1 || kept.DNSServers()[0].Tag != "mine" {
			t.Errorf("своя связка пользователя перетёрта канонической: %+v", kept)
		}
	})

	// Норма (2), путь РОЖДЕНИЯ: голый узел из вставленного конфига приезжает
	// со связкой; §6 «снятие» проверяется тем, что ParseNodeDocument на
	// документе без dns/route отдаёт пустые секции — подстановки там нет.
	t.Run("импорт голого узла и снятие связки документом", func(t *testing.T) {
		res, err := subscription.ParseSingboxBody(
			`{"endpoints":[{"type":"tailscale","tag":"ts-bare","auth_key":"tskey-auth-x"}]}`,
			subscription.BodyKindSingboxConfig, nil)
		if err != nil {
			t.Fatalf("ParseSingboxBody: %v", err)
		}
		if len(res.Nodes) != 1 {
			t.Fatalf("узлов %d, ожидался один", len(res.Nodes))
		}
		if res.Nodes[0].Sections.IsEmpty() {
			t.Error("голый узел tailnet из конфига приехал без связки — норма (2)")
		}

		// Снятие: документ с телом и БЕЗ dns/route. Подстановка на этом пути
		// запрещена — иначе связку нельзя было бы убрать вовсе.
		_, sections, err := ParseNodeDocument(
			[]byte(`{"endpoints":[{"type":"tailscale","tag":"ts-bare","auth_key":"k"}]}`))
		if err != nil {
			t.Fatalf("ParseNodeDocument: %v", err)
		}
		if !sections.IsEmpty() {
			t.Errorf("документ без dns/route не снял связку: %+v", sections)
		}
	})

	// Норма (3): в многоузловом конфиге записи достаются tailscale-узлам по
	// ЯВНОЙ ссылке, а обычный узел связки не получает (правило «ровно один»).
	t.Run("многоузловой конфиг: tailnet по ссылке, остальные — нет", func(t *testing.T) {
		const cfg = `{
  "endpoints": [{"type":"tailscale","tag":"ts-a","auth_key":"k"}],
  "outbounds": [{"type":"socks","tag":"sock","server":"127.0.0.1","server_port":1080}],
  "dns": {
    "servers": [{"type":"tailscale","tag":"a-dns","endpoint":"ts-a"},
                {"type":"udp","tag":"sock-dns","server":"1.1.1.1","detour":"sock"}],
    "rules":   [{"domain_suffix":[".ts.net"],"server":"a-dns"},
                {"domain_suffix":[".example"],"server":"sock-dns"}]
  },
  "route": {
    "rules": [{"ip_cidr":["100.64.0.0/10"],"outbound":"ts-a"},
              {"domain_suffix":[".example"],"outbound":"sock"}]
  }
}`
		res, err := subscription.ParseSingboxBody(cfg, subscription.BodyKindSingboxConfig, nil)
		if err != nil {
			t.Fatalf("ParseSingboxBody: %v", err)
		}
		byTag := map[string]*ParsedNode{}
		for _, n := range res.Nodes {
			byTag[n.Tag] = n
		}
		ts := byTag["ts-a"]
		if ts == nil {
			t.Fatalf("узел ts-a не разобран, состав: %v", byTag)
		}
		if ts.Sections.IsEmpty() {
			t.Fatal("узел tailnet в многоузловом конфиге остался без своих записей — норма (3)")
		}
		// Достались именно ЕГО записи из КОНФИГА, а не каноническая связка
		// по умолчанию: сервер конфига назван `a-dns`, поэтому его тег после
		// переписи — `@{self}-a-dns`, тогда как у связки по умолчанию он
		// `@{self}-dns`. Без этой проверки норма (3) подтверждалась бы
		// подстановкой из нормы (2), и снятие исключения для многоузлового
		// конфига осталось бы незамеченным.
		raw := string(ts.Sections.Raw)
		if !strings.Contains(raw, state.SelfPlaceholderBraced+"-a-dns") {
			t.Errorf("узлу tailnet достались не его записи конфига (ожидался сервер %s-a-dns):\n%s",
				state.SelfPlaceholderBraced, raw)
		}
		// И не чужие: сервер соседа сюда попасть не должен.
		if strings.Contains(raw, "sock-dns") || strings.Contains(raw, ".example") {
			t.Errorf("узлу tailnet достались чужие записи:\n%s", raw)
		}
		if sock := byTag["sock"]; sock != nil && !sock.Sections.IsEmpty() {
			t.Errorf("обычный узел в многоузловом конфиге получил связку — норма (3): %s",
				sock.Sections.Raw)
		}
	})

	// Норма (4): узел tailnet из ПОДПИСКИ живёт, но помечен info — связку
	// подписка принести не может.
	t.Run("узел из подписки: info, узел на месте", func(t *testing.T) {
		pb, err := subscription.ParseSubscriptionBody(
			[]byte(`[{"type":"tailscale","tag":"ts-sub","auth_key":"tskey-auth-x"}]`), nil, 0)
		if err != nil {
			t.Fatalf("ParseSubscriptionBody: %v", err)
		}
		if len(pb.Entries) != 1 {
			t.Fatalf("записей %d, ожидалась одна", len(pb.Entries))
		}
		node := pb.Entries[0].Node
		var found bool
		for _, w := range node.Warnings {
			if w == subscription.WarnTailscaleFromSubscription {
				found = true
			}
		}
		if !found {
			t.Errorf("узел tailnet из подписки без пометки %q, предупреждения: %v",
				subscription.WarnTailscaleFromSubscription, node.Warnings)
		}
	})
}

// toStringList — список строк из значения тела правила (вход всегда
// []interface{} после round-trip через JSON).
func toStringList(v interface{}) []string {
	list, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, _ := item.(string)
		out = append(out, s)
	}
	return out
}
