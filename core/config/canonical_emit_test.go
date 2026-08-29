// Поведенческие критерии SPEC 118 §4.E (1–9): эмиссия из материализованных
// nodes[], единый резолв NodeLink, свёртка папки, пул кандидатов, гард тегов.
//
// Тесты гоняют ВЕСЬ конвейер генерации (GenerateOutboundsFromParserConfig) на
// канонической проекции: проверять эмиссию по кускам значило бы проверять не
// то, что реально уезжает в config.json.
package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// ── строительные леса ────────────────────────────────────────────────

// canonBody — тело server-узла в форме канона (ключи в порядке эмиттера,
// без tag и detour).
func canonBody(scheme, server string, port int) json.RawMessage {
	return json.RawMessage(`{"type":"` + scheme + `","server":"` + server + `","server_port":` + itoa(port) + `}`)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// canonServerNode — server-узел канона с происхождением-URI (подпись и вход
// тег-политики восстанавливаются из него, как на боевом пути).
func canonServerNode(rawTag, label, server string, port int) configtypes.CanonicalNode {
	return configtypes.CanonicalNode{
		Kind:       "server",
		Tag:        rawTag,
		Enabled:    true,
		Body:       canonBody("trojan", server, port),
		OriginKind: "uri",
		OriginRaw:  "trojan://pw@" + server + ":" + itoa(port) + "#" + label,
	}
}

// canonFolder — источник-папка (подписка) с тег-политикой.
func canonFolder(id, prefix, postfix string, nodes ...configtypes.CanonicalNode) ProxySource {
	return ProxySource{
		ID:    id,
		Label: id,
		Canonical: &configtypes.CanonicalSource{
			FolderID:    id,
			IsContainer: true,
			TagPrefix:   prefix,
			TagPostfix:  postfix,
			Nodes:       nodes,
		},
	}
}

// canonRoot — корневой узел (вне папки).
func canonRoot(id, tag string, node configtypes.CanonicalNode) ProxySource {
	node.Tag = tag
	return ProxySource{
		ID:        id,
		Label:     tag,
		Canonical: &configtypes.CanonicalSource{Nodes: []configtypes.CanonicalNode{node}},
	}
}

// runCanonicalBuild гоняет генерацию по каноническим источникам. loadNodes не
// зовётся вовсе — если он сработал, значит сборка полезла разбирать тела, и
// это провал критерия Т5.
func runCanonicalBuild(t *testing.T, proxies []ProxySource, directions []Direction) *OutboundGenerationResult {
	t.Helper()
	pc := &ParserConfig{}
	pc.ParserConfig.Version = ParserConfigVersion
	pc.ParserConfig.Proxies = proxies
	pc.ParserConfig.Outbounds = directions

	// SPEC Т5: парсер тел конвейеру сборки больше не передаётся вовсе —
	// сигнатура GenerateOutboundsFromParserConfig его не принимает, и
	// «сборка полезла разбирать тела» стало невыразимо по построению.
	res, err := GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil, DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("GenerateOutboundsFromParserConfig: %v", err)
	}
	return res
}

// emittedTags — теги всех эмитированных outbound'ов.
func emittedTags(res *OutboundGenerationResult) []string {
	var out []string
	for _, line := range res.OutboundsJSON {
		body := line
		if i := strings.Index(body, "{"); i >= 0 {
			body = body[i:]
		}
		body = strings.TrimRight(strings.TrimSpace(body), ",")
		var m map[string]interface{}
		if json.Unmarshal([]byte(body), &m) != nil {
			continue
		}
		if tag, _ := m["tag"].(string); tag != "" {
			out = append(out, tag)
		}
	}
	return out
}

func emittedObject(t *testing.T, res *OutboundGenerationResult, tag string) map[string]interface{} {
	t.Helper()
	for _, line := range res.OutboundsJSON {
		body := line
		if i := strings.Index(body, "{"); i >= 0 {
			body = body[i:]
		}
		body = strings.TrimRight(strings.TrimSpace(body), ",")
		var m map[string]interface{}
		if json.Unmarshal([]byte(body), &m) != nil {
			continue
		}
		if got, _ := m["tag"].(string); got == tag {
			return m
		}
	}
	return nil
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func joinWarnings(res *OutboundGenerationResult) string {
	return strings.Join(EmissionWarningTexts(res.EmissionWarnings), " | ")
}

// ── §4.E.1 — тег-политика на эмиссии ─────────────────────────────────

// Правка prefix меняет ФИНАЛЬНЫЕ теги и не трогает сырые: ни merge-ключи, ни
// enabled, ни ссылки (они адресуют сырой тег).
func TestEmitE1_TagPolicyChangesOnlyFinalTags(t *testing.T) {
	nodes := []configtypes.CanonicalNode{
		canonServerNode("NL-1", "NL-1", "nl.example", 443),
		canonServerNode("DE-2", "DE-2", "de.example", 443),
	}

	before := runCanonicalBuild(t, []ProxySource{canonFolder("F1", "[A] ", "", nodes...)}, nil)
	after := runCanonicalBuild(t, []ProxySource{canonFolder("F1", "[B] ", " •", nodes...)}, nil)

	if !hasTag(emittedTags(before), "[A] NL-1") || !hasTag(emittedTags(after), "[B] NL-1 •") {
		t.Fatalf("политика не применилась: before=%v after=%v", emittedTags(before), emittedTags(after))
	}
	// Сырые теги источника не тронуты — на них живут merge-ключ, enabled и
	// ссылки NodeLink.
	for _, n := range nodes {
		if n.Tag != "NL-1" && n.Tag != "DE-2" {
			t.Fatalf("сырой тег изменён эмиссией: %q", n.Tag)
		}
	}
}

// Выключенный узел не эмитится вовсе (роль прежней disabled-карты).
func TestEmitE1_DisabledNodeNotEmitted(t *testing.T) {
	off := canonServerNode("DE-2", "DE-2", "de.example", 443)
	off.Enabled = false
	res := runCanonicalBuild(t, []ProxySource{
		canonFolder("F1", "", "", canonServerNode("NL-1", "NL-1", "nl.example", 443), off),
	}, nil)
	tags := emittedTags(res)
	if hasTag(tags, "DE-2") {
		t.Errorf("выключенный узел уехал в конфиг: %v", tags)
	}
	if !hasTag(tags, "NL-1") {
		t.Errorf("включённый узел потерян: %v", tags)
	}
}

// ── §4.E.2 — detour fail-closed + исключение WireGuard ───────────────

func TestEmitE2_DanglingDetourDropsCarrier(t *testing.T) {
	carrier := canonServerNode("Tokyo", "Tokyo", "h.example", 443)
	carrier.Detour = &configtypes.NodeLink{Tag: "ghost"}

	res := runCanonicalBuild(t, []ProxySource{
		canonFolder("F1", "", "", canonServerNode("NL-1", "NL-1", "nl.example", 443)),
		canonRoot("S1", "Tokyo", carrier),
	}, nil)

	if hasTag(emittedTags(res), "Tokyo") {
		t.Error("носитель висячего detour остался в конфиге — трафик пошёл бы напрямую молча")
	}
	if !strings.Contains(joinWarnings(res), "Tokyo") {
		t.Errorf("деградация не названа пользователю: %v", res.EmissionWarnings)
	}
}

func TestEmitE2_DisabledDetourTargetDropsCarrier(t *testing.T) {
	target := canonServerNode("NL-1", "NL-1", "nl.example", 443)
	target.Enabled = false
	carrier := canonServerNode("Tokyo", "Tokyo", "h.example", 443)
	carrier.Detour = &configtypes.NodeLink{FolderID: "F1", Tag: "NL-1"}

	res := runCanonicalBuild(t, []ProxySource{
		canonFolder("F1", "", "", target, canonServerNode("SE-1", "SE-1", "se.example", 443)),
		canonRoot("S1", "Tokyo", carrier),
	}, nil)

	if hasTag(emittedTags(res), "Tokyo") {
		t.Error("выключенная цель detour: носитель обязан выпасть fail-closed")
	}
}

// Самоссылка — вырожденное кольцо длины 1, и исход тот же.
func TestEmitE2_SelfDetourDropsCarrier(t *testing.T) {
	a := canonServerNode("A", "A", "a.example", 443)
	a.Detour = &configtypes.NodeLink{Tag: "A"}
	res := runCanonicalBuild(t, []ProxySource{
		canonRoot("S1", "A", a),
		canonRoot("S2", "C", canonServerNode("C", "C", "c.example", 443)),
	}, nil)
	if hasTag(emittedTags(res), "A") {
		t.Errorf("самоссылка detour не поймана: %v", emittedTags(res))
	}
}

func TestEmitE2_DetourCycleIsFailClosed(t *testing.T) {
	a := canonServerNode("A", "A", "a.example", 443)
	a.Detour = &configtypes.NodeLink{Tag: "B"}
	b := canonServerNode("B", "B", "b.example", 443)
	b.Detour = &configtypes.NodeLink{Tag: "A"}

	res := runCanonicalBuild(t, []ProxySource{
		canonRoot("S1", "A", a),
		canonRoot("S2", "B", b),
		// Третий узел — чтобы сборка не свалилась в «узлов нет вовсе».
		canonRoot("S3", "C", canonServerNode("C", "C", "c.example", 443)),
	}, nil)

	tags := emittedTags(res)
	if hasTag(tags, "A") || hasTag(tags, "B") {
		t.Errorf("участники кольца detour остались в конфиге: %v", tags)
	}
	// Тексты эмиссии — английские ключи локали (SPEC 116 W12, фикс 2);
	// в тестах каталог не загружен, поэтому проверяется дефолт.
	if !strings.Contains(joinWarnings(res), "detour loop") {
		t.Errorf("кольцо не названо пользователю: %v", res.EmissionWarnings)
	}
}

// Исключение ядра: WireGuard detour не получает — правило модели.
func TestEmitE2_WireGuardTakesNoDetour(t *testing.T) {
	wg := configtypes.CanonicalNode{
		Kind:    "server",
		Tag:     "wg-1",
		Enabled: true,
		Body:    json.RawMessage(`{"type":"wireguard","address":["10.0.0.2/32"],"private_key":"k"}`),
		Detour:  &configtypes.NodeLink{Tag: "ghost"},
	}
	res := runCanonicalBuild(t, []ProxySource{
		canonRoot("S1", "wg-1", wg),
		canonRoot("S2", "C", canonServerNode("C", "C", "c.example", 443)),
	}, nil)

	// Узел жив (висячая ссылка его не уронила) и detour ему не проставлен.
	found := false
	for _, ep := range res.EndpointsJSON {
		if strings.Contains(ep, `"wg-1"`) {
			found = true
			if strings.Contains(ep, `"detour"`) {
				t.Error("WireGuard получил detour — правило модели нарушено")
			}
		}
	}
	if !found {
		t.Fatalf("wireguard-узел не эмитирован: %v", res.EndpointsJSON)
	}
}

// Папочный общий detour: Server-узлам без личного, пропуская Auto.
func TestEmitE2_FolderDetourSkipsGroups(t *testing.T) {
	auto := configtypes.CanonicalNode{
		Kind: "auto", Tag: "grp", Enabled: true,
		Group: &configtypes.CanonicalAutoGroup{
			GroupType: "urltest",
			Members:   []configtypes.NodeLink{{FolderID: "F1", Tag: "NL-1"}},
		},
	}
	src := canonFolder("F1", "", "", canonServerNode("NL-1", "NL-1", "nl.example", 443), auto)
	src.Canonical.FolderDetour = &configtypes.NodeLink{Tag: "Hop"}

	res := runCanonicalBuild(t, []ProxySource{
		src,
		canonRoot("S1", "Hop", canonServerNode("Hop", "Hop", "hop.example", 443)),
	}, nil)

	if obj := emittedObject(t, res, "NL-1"); obj == nil {
		t.Fatal("server-узел папки потерян")
	} else if obj["detour"] != "Hop" {
		t.Errorf("общий detour папки не применён к server-узлу: %v", obj["detour"])
	}
	if obj := emittedObject(t, res, "grp"); obj == nil {
		t.Fatal("Auto-узел папки потерян")
	} else if _, has := obj["detour"]; has {
		t.Error("общий detour папки применён к Auto — selector/urltest dial-полей не принимают")
	}
}

// ── §4.E.3 — цепочки: хоп на replace-тег и на верхний узел ───────────

func TestEmitE3_ChainHopsToReplaceTagAndRootNode(t *testing.T) {
	folder := canonFolder("F1", "", "",
		canonServerNode("NL-1", "NL-1", "nl.example", 443),
		canonServerNode("NL-2", "NL-2", "nl2.example", 443))
	folder.Canonical.Replace = &configtypes.FolderReplace{Mode: configtypes.FolderReplaceManual, Tag: "F1-pick"}

	chainNode := configtypes.CanonicalNode{
		Kind: "chain", Tag: "chain-1", Enabled: true,
		Hops: []configtypes.NodeLink{{Tag: "Hop"}, {Tag: "F1-pick"}},
	}
	chainSrc := ProxySource{
		ID: "C1", Label: "chain-1",
		Canonical: &configtypes.CanonicalSource{Nodes: []configtypes.CanonicalNode{chainNode}},
	}

	res := runCanonicalBuild(t, []ProxySource{
		folder,
		canonRoot("S1", "Hop", canonServerNode("Hop", "Hop", "hop.example", 443)),
		chainSrc,
	}, nil)

	obj := emittedObject(t, res, "chain-1")
	if obj == nil {
		t.Fatalf("цепочка не собралась: broken=%+v warnings=%v", res.BrokenChains, res.EmissionWarnings)
	}
	members, _ := obj["outbounds"].([]interface{})
	if len(members) != 2 || members[0] != "Hop" || members[1] != "F1-pick" {
		t.Errorf("позиции цепочки = %v, want [Hop F1-pick]", members)
	}
}

func TestEmitE3_DanglingChainHopIsFailClosed(t *testing.T) {
	chainNode := configtypes.CanonicalNode{
		Kind: "chain", Tag: "chain-1", Enabled: true,
		Hops: []configtypes.NodeLink{{Tag: "Hop"}, {Tag: "ghost"}},
	}
	res := runCanonicalBuild(t, []ProxySource{
		canonRoot("S1", "Hop", canonServerNode("Hop", "Hop", "hop.example", 443)),
		{ID: "C1", Label: "chain-1",
			Canonical: &configtypes.CanonicalSource{Nodes: []configtypes.CanonicalNode{chainNode}}},
	}, nil)

	if emittedObject(t, res, "chain-1") != nil {
		t.Error("цепочка с висячей позицией уехала в конфиг — маршрут без хопа это другой маршрут")
	}
	if len(res.BrokenChains) == 0 && !strings.Contains(joinWarnings(res), "ghost") {
		t.Errorf("потеря цепочки молчалива: broken=%+v warnings=%v", res.BrokenChains, res.EmissionWarnings)
	}
}

// ── §4.E.4 — Auto: фильтр enabled, пустая не эмитится, default ───────

func TestEmitE4_AutoFiltersDisabledMembers(t *testing.T) {
	off := canonServerNode("DE-2", "DE-2", "de.example", 443)
	off.Enabled = false
	auto := configtypes.CanonicalNode{
		Kind: "auto", Tag: "grp", Enabled: true,
		Group: &configtypes.CanonicalAutoGroup{
			GroupType: "urltest",
			Members: []configtypes.NodeLink{
				{FolderID: "F1", Tag: "NL-1"},
				{FolderID: "F1", Tag: "DE-2"},
			},
		},
	}
	res := runCanonicalBuild(t, []ProxySource{
		canonFolder("F1", "", "", canonServerNode("NL-1", "NL-1", "nl.example", 443), off, auto),
	}, nil)

	obj := emittedObject(t, res, "grp")
	if obj == nil {
		t.Fatal("группа не эмитирована")
	}
	members, _ := obj["outbounds"].([]interface{})
	if len(members) != 1 || members[0] != "NL-1" {
		t.Errorf("состав группы = %v, want [NL-1] (выключенный член отфильтрован)", members)
	}
	if !strings.Contains(joinWarnings(res), "DE-2") {
		t.Errorf("выпавший член не назван пользователю: %v", res.EmissionWarnings)
	}
}

func TestEmitE4_EmptyAutoNotEmittedWithWarning(t *testing.T) {
	auto := configtypes.CanonicalNode{
		Kind: "auto", Tag: "grp", Enabled: true,
		Group: &configtypes.CanonicalAutoGroup{
			GroupType: "urltest",
			Members:   []configtypes.NodeLink{{FolderID: "F1", Tag: "ghost"}},
		},
	}
	res := runCanonicalBuild(t, []ProxySource{
		canonFolder("F1", "", "", canonServerNode("NL-1", "NL-1", "nl.example", 443), auto),
	}, nil)

	if emittedObject(t, res, "grp") != nil {
		t.Error("пустая группа уехала в конфиг — ядро на ней не стартует")
	}
	if !strings.Contains(joinWarnings(res), "grp") {
		t.Errorf("потеря группы молчалива: %v", res.EmissionWarnings)
	}
}

func TestEmitE4_SelectorKeepsTypeAndDefaultDropsForeign(t *testing.T) {
	auto := configtypes.CanonicalNode{
		Kind: "auto", Tag: "grp", Enabled: true,
		Group: &configtypes.CanonicalAutoGroup{
			GroupType: "selector",
			Default:   "NL-1",
			Members:   []configtypes.NodeLink{{FolderID: "F1", Tag: "NL-1"}},
		},
	}
	res := runCanonicalBuild(t, []ProxySource{
		canonFolder("F1", "[P] ", "", canonServerNode("NL-1", "NL-1", "nl.example", 443), auto),
	}, nil)
	obj := emittedObject(t, res, "[P] grp")
	if obj == nil {
		t.Fatalf("selector не эмитирован: %v", emittedTags(res))
	}
	if obj["type"] != "selector" {
		t.Errorf("импортированный selector сменил тип: %v", obj["type"])
	}
	if obj["default"] != "[P] NL-1" {
		t.Errorf("default не переведён в финальный тег: %v", obj["default"])
	}

	// default вне состава — снимается с предупреждением.
	auto.Group.Default = "ghost"
	res2 := runCanonicalBuild(t, []ProxySource{
		canonFolder("F1", "[P] ", "", canonServerNode("NL-1", "NL-1", "nl.example", 443), auto),
	}, nil)
	obj2 := emittedObject(t, res2, "[P] grp")
	if obj2 == nil {
		t.Fatal("selector пропал после чужого default")
	}
	if _, has := obj2["default"]; has {
		t.Errorf("умолчание вне состава уехало в конфиг: %v", obj2["default"])
	}
	if !strings.Contains(joinWarnings(res2), "default") {
		t.Errorf("снятие умолчания молчаливо: %v", res2.EmissionWarnings)
	}
}

// ── §4.E.5 — FolderReplace ───────────────────────────────────────────

func replaceFixture(t *testing.T, mode string) *OutboundGenerationResult {
	t.Helper()
	auto := configtypes.CanonicalNode{
		Kind: "auto", Tag: "grp", Enabled: true,
		Group: &configtypes.CanonicalAutoGroup{
			GroupType: "urltest",
			Members:   []configtypes.NodeLink{{FolderID: "F1", Tag: "NL-1"}},
		},
	}
	folder := canonFolder("F1", "", "",
		canonServerNode("NL-1", "NL-1", "nl.example", 443),
		canonServerNode("NL-2", "NL-2", "nl2.example", 443),
		auto)
	folder.Canonical.Replace = &configtypes.FolderReplace{
		Mode: mode, Tag: "F1-pick", Strategy: &configtypes.DirectionAuto{Interval: "5m"},
	}
	return runCanonicalBuild(t, []ProxySource{folder}, nil)
}

func TestEmitE5_ReplaceManualCoversWholeFolder(t *testing.T) {
	res := replaceFixture(t, configtypes.FolderReplaceManual)
	obj := emittedObject(t, res, "F1-pick")
	if obj == nil {
		t.Fatalf("селектор замены не эмитирован: %v", emittedTags(res))
	}
	if obj["type"] != "selector" {
		t.Errorf("режим manual дал %v, want selector", obj["type"])
	}
	members, _ := obj["outbounds"].([]interface{})
	if len(members) != 3 {
		t.Errorf("пул замены не вся папка: %v", members)
	}
	// Узлы свёрнутой папки живут в outbounds и легальны целями ссылок.
	if !hasTag(emittedTags(res), "NL-1") {
		t.Error("узлы свёрнутой папки исчезли из конфига")
	}
}

func TestEmitE5_ReplaceAutoExcludesGroupNodes(t *testing.T) {
	res := replaceFixture(t, configtypes.FolderReplaceAuto)
	obj := emittedObject(t, res, "F1-pick")
	if obj == nil {
		t.Fatalf("авто-группа замены не эмитирована: %v", emittedTags(res))
	}
	if obj["type"] != "urltest" {
		t.Errorf("режим auto дал %v, want urltest", obj["type"])
	}
	members, _ := obj["outbounds"].([]interface{})
	for _, m := range members {
		if m == "grp" {
			t.Error("Auto-узел папки попал в авто-состав замены — измерялся бы чужой выбор")
		}
	}
	if len(members) != 2 {
		t.Errorf("авто-состав = %v, want [NL-1 NL-2]", members)
	}
}

func TestEmitE5_ReplaceBothMakesTwinDefault(t *testing.T) {
	res := replaceFixture(t, configtypes.FolderReplaceBoth)
	sel := emittedObject(t, res, "F1-pick")
	twin := emittedObject(t, res, "F1-pick-auto")
	if sel == nil || twin == nil {
		t.Fatalf("пара «селектор + двойник» не собралась: %v", emittedTags(res))
	}
	if twin["type"] != "urltest" {
		t.Errorf("двойник = %v, want urltest", twin["type"])
	}
	if sel["default"] != "F1-pick-auto" {
		t.Errorf("двойник не стал умолчанием селектора: %v", sel["default"])
	}
}

// ── §4.E.6 — пул кандидатов Направлений ──────────────────────────────

func TestEmitE6_PoolShowsReplaceTagInsteadOfFolderNodes(t *testing.T) {
	folder := canonFolder("F1", "", "",
		canonServerNode("NL-1", "NL-1", "nl.example", 443),
		canonServerNode("NL-2", "NL-2", "nl2.example", 443))
	folder.Canonical.Replace = &configtypes.FolderReplace{Mode: configtypes.FolderReplaceManual, Tag: "F1-pick"}

	dir := Direction{Tag: "proxy-out", Type: "selector"}
	res := runCanonicalBuild(t, []ProxySource{
		folder,
		canonRoot("S1", "Top", canonServerNode("Top", "Top", "top.example", 443)),
	}, []Direction{dir})

	obj := emittedObject(t, res, "proxy-out")
	if obj == nil {
		t.Fatalf("Направление не эмитировано: %v", emittedTags(res))
	}
	members, _ := obj["outbounds"].([]interface{})
	got := map[string]bool{}
	for _, m := range members {
		got[m.(string)] = true
	}
	if !got["F1-pick"] {
		t.Errorf("свёрнутая папка не представлена тегом замены: %v", members)
	}
	if got["NL-1"] || got["NL-2"] {
		t.Errorf("узлы свёрнутой папки остались в пуле: %v", members)
	}
	if !got["Top"] {
		t.Errorf("верхний узел выпал из пула: %v", members)
	}
}

func TestEmitE6_UnfoldedFolderShowsNodesUnderFinalTags(t *testing.T) {
	res := runCanonicalBuild(t, []ProxySource{
		canonFolder("F1", "[P] ", "", canonServerNode("NL-1", "NL-1", "nl.example", 443)),
	}, []Direction{{Tag: "proxy-out", Type: "selector"}})

	obj := emittedObject(t, res, "proxy-out")
	if obj == nil {
		t.Fatal("Направление не эмитировано")
	}
	members, _ := obj["outbounds"].([]interface{})
	if len(members) != 1 || members[0] != "[P] NL-1" {
		t.Errorf("несвёрнутая папка = %v, want [[P] NL-1] (финальные теги)", members)
	}
}

func TestEmitE6_TwinExcludesGroupCandidates(t *testing.T) {
	auto := configtypes.CanonicalNode{
		Kind: "auto", Tag: "grp", Enabled: true,
		Group: &configtypes.CanonicalAutoGroup{
			GroupType: "urltest",
			Members:   []configtypes.NodeLink{{FolderID: "F1", Tag: "NL-1"}},
		},
	}
	folder := canonFolder("F1", "", "", canonServerNode("NL-1", "NL-1", "nl.example", 443), auto)
	folderWithReplace := canonFolder("F2", "", "", canonServerNode("SE-1", "SE-1", "se.example", 443))
	folderWithReplace.Canonical.Replace = &configtypes.FolderReplace{Mode: configtypes.FolderReplaceManual, Tag: "F2-pick"}

	dir := Direction{Tag: "proxy-out", Type: "selector", Auto: &configtypes.DirectionAuto{Interval: "3m"}}
	res := runCanonicalBuild(t, []ProxySource{folder, folderWithReplace}, []Direction{dir})

	twin := emittedObject(t, res, "proxy-out-auto")
	if twin == nil {
		t.Fatalf("твин Направления не эмитирован: %v", emittedTags(res))
	}
	members, _ := twin["outbounds"].([]interface{})
	for _, m := range members {
		if m == "grp" {
			t.Error("твин принял Auto-узел — мерил бы чужой выбор, а не маршрут")
		}
		if m == "F2-pick" {
			t.Error("твин принял replace-тег — мерил бы чужой выбор, а не маршрут")
		}
	}
}

// ── §4.E.7 — единый гард занятости тегов ─────────────────────────────

func TestEmitE7_GuardCatchesDirectionVersusReplaceCollision(t *testing.T) {
	folder := canonFolder("F1", "", "", canonServerNode("NL-1", "NL-1", "nl.example", 443))
	folder.Canonical.Replace = &configtypes.FolderReplace{Mode: configtypes.FolderReplaceManual, Tag: "x"}

	res := runCanonicalBuild(t, []ProxySource{folder},
		[]Direction{{Tag: "x", Type: "selector", Auto: &configtypes.DirectionAuto{Interval: "3m"}}})

	joined := joinWarnings(res)
	if !strings.Contains(joined, `"x"`) || !strings.Contains(joined, "claimed twice") {
		t.Errorf("столкновение «Направление x + замена x» не названо: %v", res.EmissionWarnings)
	}
}

func TestEmitE7_GuardKnowsAllTagKinds(t *testing.T) {
	folder := canonFolder("F1", "", "", canonServerNode("NL-1", "NL-1", "nl.example", 443))
	folder.Canonical.Replace = &configtypes.FolderReplace{Mode: configtypes.FolderReplaceBoth, Tag: "F1-pick"}
	proxies := []ProxySource{folder, canonRoot("S1", "Top", canonServerNode("Top", "Top", "top.example", 443))}
	dirs := []Direction{{Tag: "proxy-out", Auto: &configtypes.DirectionAuto{}}}

	guard := BuildTagGuard(dirs, proxies, []string{"Top"}, []string{"direct-out", "block-out"})
	for _, want := range []string{"proxy-out", "proxy-out-auto", "F1-pick", "F1-pick-auto", "Top", "direct-out", "block-out"} {
		if !guard.Taken(want) {
			t.Errorf("гард не знает тега %q — переименование смогло бы его занять", want)
		}
	}
	if len(guard.Conflicts()) != 0 {
		t.Errorf("ложное столкновение: %v", guard.Conflicts())
	}

	// Известные цели ссылок = гард + addOutbounds.
	known := KnownTargetTags(guard, []Direction{{Tag: "proxy-out", AddOutbounds: []string{"extra-out"}}})
	if !hasTag(known, "F1-pick") || !hasTag(known, "extra-out") {
		t.Errorf("реестр известных целей неполон: %v", known)
	}
}

// ── §4.E.9 — выключенное Направление ─────────────────────────────────

func TestEmitE9_DisabledDirectionNotEmittedButKnown(t *testing.T) {
	proxies := []ProxySource{canonRoot("S1", "Top", canonServerNode("Top", "Top", "top.example", 443))}
	dirs := []Direction{
		{Tag: "live-out", Type: "selector"},
		{Tag: "paused-out", Type: "selector", Disabled: true},
	}
	res := runCanonicalBuild(t, proxies, dirs)

	tags := emittedTags(res)
	if hasTag(tags, "paused-out") {
		t.Errorf("выключенное Направление уехало в конфиг: %v", tags)
	}
	if !hasTag(tags, "live-out") {
		t.Errorf("включённое Направление потеряно: %v", tags)
	}

	// Тег остаётся в множестве известных целей — правила на него не
	// сбрасываются (SPEC §4.E.9).
	guard := BuildTagGuard(dirs, proxies, []string{"Top"}, nil)
	if !guard.Taken("paused-out") {
		t.Error("тег выключенного Направления выпал из гарда — правило было бы сброшено на direct")
	}
}

// ── Фикс-раунд W4: выключенный узел не сдвигает соседей ──────────────

// Выключенный узел ПРОХОДИТ тег-машину и выбрасывается после неё — ровно как
// в старом движке (filterDisabledNodes шёл после раздачи тегов). Иначе третий
// узел подписки [A, выключенный B, B] получил бы «B» вместо «B-2», и у
// пользователя разом протухли бы выборы в cache.db ядра и ссылки на финальный
// тег.
func TestEmitDisabledNodeStillConsumesUniqueSlot(t *testing.T) {
	off := canonServerNode("B", "B", "b1.example", 443)
	off.Enabled = false
	res := runCanonicalBuild(t, []ProxySource{
		canonFolder("F1", "", "",
			canonServerNode("A", "A", "a.example", 443),
			off,
			canonServerNode("B", "B", "b2.example", 443),
		),
	}, nil)

	tags := emittedTags(res)
	if !hasTag(tags, "B-2") {
		t.Errorf("выключенный узел не съел слот уникализации — теги сдвинулись против старого движка: %v", tags)
	}
	if hasTag(tags, "B") {
		t.Errorf("живой узел занял слот выключенного (%v): финальные теги разъехались со старым движком", tags)
	}
	// Выключенный именно выброшен, а не переименован: узла с адресом
	// выключенной записи в конфиге быть не должно.
	if obj := emittedObject(t, res, "B-2"); obj == nil || obj["server"] != "b2.example" {
		t.Errorf("под тегом B-2 оказался не тот узел: %v", obj)
	}
	if len(tags) != 2 {
		t.Errorf("ожидались ровно два узла (A и B-2), получено %v", tags)
	}
}

// {$num} тоже считается по узлам, прошедшим машину: выключенный номер
// потребляет (старый nodesFromThisSource рос до фильтрации).
func TestEmitDisabledNodeConsumesNumVariable(t *testing.T) {
	off := canonServerNode("mid", "mid", "mid.example", 443)
	off.Enabled = false
	res := runCanonicalBuild(t, []ProxySource{
		canonFolder("F1", "", " #{$num}",
			canonServerNode("first", "first", "a.example", 443),
			off,
			canonServerNode("last", "last", "c.example", 443),
		),
	}, nil)

	tags := emittedTags(res)
	if !hasTag(tags, "last #3") {
		t.Errorf("{$num} не учёл выключенный узел: %v", tags)
	}
}

// Тег-маска упразднена ВМЕСТЕ С ПОЛЕМ (SPEC 118 W5): в сборочной форме её
// больше нет, и warning о ней стал невыразим по построению. Проверки
// TestEmitRetiredMaskIsReported / TestEmitRootNodeTagIsNotReportedAsMask
// удалены вместе с предметом.


// ── Фикс-раунд W4: WG с висячим detour не молчит ─────────────────────

// WireGuard detour не применяется по правилу модели, поэтому носитель не
// роняется. Но если цель ещё и не существует, у пользователя настроены две
// неработающие вещи сразу — молчать нельзя.
func TestEmitWireguardDanglingDetourWarns(t *testing.T) {
	wg := configtypes.CanonicalNode{
		Kind:       "server",
		Tag:        "WG",
		Enabled:    true,
		Body:       json.RawMessage(`{"type":"wireguard","server":"wg.example","server_port":51820}`),
		OriginKind: "uri",
		OriginRaw:  "wireguard://wg.example:51820#WG",
		Detour:     &configtypes.NodeLink{Tag: "ghost"},
	}
	res := runCanonicalBuild(t, []ProxySource{
		canonFolder("F1", "", "", canonServerNode("NL-1", "NL-1", "nl.example", 443)),
		canonRoot("S1", "WG", wg),
	}, nil)

	// Носитель жив: detour для WG незначим, ронять его было бы наказанием
	// за неработающее поле. WireGuard едет в endpoints, не в outbounds.
	endpoints := strings.Join(res.EndpointsJSON, "\n")
	if !strings.Contains(endpoints, `"WG"`) {
		t.Fatalf("WG-узел выпал из конфига: endpoints=%v outbounds=%v", res.EndpointsJSON, emittedTags(res))
	}
	if strings.Contains(endpoints, `"detour"`) {
		t.Errorf("WG-узлу проштампован detour — ядро такого не принимает: %v", res.EndpointsJSON)
	}
	found := false
	for _, w := range EmissionWarningTexts(res.EmissionWarnings) {
		if strings.Contains(w, "WG") && strings.Contains(w, "ghost") {
			found = true
		}
	}
	if !found {
		t.Errorf("висячий detour у WG проглочен молча: %v", res.EmissionWarnings)
	}
}

// Разрешившаяся цель у WG — не повод шуметь: detour просто не применяется,
// про это говорят features/directions.md, а не отчёт каждой сборки.
func TestEmitWireguardResolvableDetourStaysQuiet(t *testing.T) {
	wg := configtypes.CanonicalNode{
		Kind:       "server",
		Tag:        "WG",
		Enabled:    true,
		Body:       json.RawMessage(`{"type":"wireguard","server":"wg.example","server_port":51820}`),
		OriginKind: "uri",
		OriginRaw:  "wireguard://wg.example:51820#WG",
		Detour:     &configtypes.NodeLink{FolderID: "F1", Tag: "NL-1"},
	}
	res := runCanonicalBuild(t, []ProxySource{
		canonFolder("F1", "", "", canonServerNode("NL-1", "NL-1", "nl.example", 443)),
		canonRoot("S1", "WG", wg),
	}, nil)

	for _, w := range EmissionWarningTexts(res.EmissionWarnings) {
		if strings.Contains(w, "wireguard") {
			t.Errorf("рабочая конфигурация названа деградацией: %q", w)
		}
	}
}
