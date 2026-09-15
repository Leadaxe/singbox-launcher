package business

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/configtypes"
	corestate "singbox-launcher/core/state"
	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// Ссылки на корневое имя — сквозной сценарий (NODE_LINK.md §6; решения
// владельца 15.09.2026): переименование верхнего узла ведёт за ним ссылки всех
// видов, удаление гасит их, переименование Направления и свёртки доходит до
// позиций цепочек в папках и detour членов папок. Проверяется и модель, и
// сборка: ссылка, переписанная в состоянии, обязана разрешиться в конфиге —
// иначе зависимый узел выпал бы fail-closed.

const renameHopURI = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@hop.example.com:443?encryption=none&security=tls&sni=hop.example.com#hop"

const renameDependentURI = "vless://c931381d-6324-4d53-ad4f-8cda48b30811@dep.example.com:443?encryption=none&security=tls&sni=dep.example.com#dep"

// renameNode — узел-сервер с материализованным телом: SPEC 118 Т2, узел без
// тела собирать не из чего.
func renameNode(t *testing.T, tag, uri string, detour *corestate.NodeLink) corestate.Node {
	t.Helper()
	mat, err := config.MaterializeServerNode(uri, nil)
	if err != nil {
		t.Fatalf("материализация %q: %v", tag, err)
	}
	return corestate.Node{
		Kind: corestate.SourceKindServer, Enabled: true, Tag: tag,
		Body:   mat.Body,
		Origin: &corestate.Origin{Kind: mat.OriginKind, Raw: mat.OriginRaw},
		Detour: detour,
	}
}

func renameChain(tag string, hops ...corestate.NodeLink) corestate.Node {
	return corestate.Node{
		Kind: corestate.SourceKindChain, Enabled: true, Tag: tag,
		Body: configtypes.ChainBody(&configtypes.SourceChain{}),
		Hops: hops,
	}
}

// rootNameModel — на верхний узел `hop` смотрит КАЖДЫЙ вид ссылки: detour
// корневого узла и члена папки, позиция цепочки в папке, опция и литеральное
// умолчание Направления, цель правила, route.final, detour DNS. Рядом —
// ссылки на Направление `vpn` и свёртку `pick` из тех же мест.
func rootNameModel(t *testing.T) *wizardmodels.WizardModel {
	work := corestate.Source{
		ID: "01FLD", Name: "Work",
		Node: corestate.Node{Kind: corestate.SourceKindFolder, Enabled: true},
		Nodes: []corestate.Node{
			renameNode(t, "exit", renameDependentURI, nil),
			renameNode(t, "m1", renameDependentURI, &corestate.NodeLink{Tag: "hop"}),
			renameNode(t, "m2", renameDependentURI, &corestate.NodeLink{Tag: "vpn"}),
			renameChain("fc", corestate.NodeLink{Tag: "hop"}, corestate.NodeLink{FolderID: "01FLD", Tag: "exit"}),
			renameChain("fd", corestate.NodeLink{Tag: "vpn"}, corestate.NodeLink{FolderID: "01FLD", Tag: "exit"}),
		},
	}
	fold := corestate.Source{
		ID: "01FOLD", Name: "Fold",
		Node:    corestate.Node{Kind: corestate.SourceKindFolder, Enabled: true},
		Replace: &corestate.FolderReplace{Mode: corestate.FolderReplaceBoth, Tag: "pick"},
		Nodes:   []corestate.Node{renameNode(t, "f1", renameDependentURI, nil)},
	}
	return &wizardmodels.WizardModel{
		Sources: []corestate.Source{
			{ID: "01HOP", Label: "WARP hop", Node: renameNode(t, "hop", renameHopURI, nil)},
			{ID: "01DEP", Label: "Proton NL", Node: renameNode(t, "dep", renameDependentURI, &corestate.NodeLink{Tag: "hop"})},
			work,
			fold,
			{ID: "01RC", Label: "Via pick", Node: renameChain("rc", corestate.NodeLink{Tag: "pick-auto"}, corestate.NodeLink{Tag: "dep"})},
		},
		GlobalOutbounds: []configtypes.Direction{
			{Tag: "vpn", Type: "selector", AddOutbounds: []string{"hop", "direct-out"},
				Options:          map[string]interface{}{"default": "hop"},
				PreferredDefault: map[string]interface{}{"tag": "hop"}},
			{Tag: "other", Type: "selector", AddOutbounds: []string{"pick"}},
			// Запись-ссылка на шаблон: тело в базе, ссылки — в USER-патче;
			// патч пресета пересобирает sync, и операции его не трогают.
			{Tag: "tmpl", Ref: configtypes.RefTemplate, Updates: []configtypes.OutboundUpdate{
				{Ref: "some-preset", Patch: map[string]interface{}{"addOutbounds": []interface{}{"hop"}}},
				{Ref: configtypes.RefUser, Patch: map[string]interface{}{
					"addOutbounds": []interface{}{"hop", "direct-out"},
					"options":      map[string]interface{}{"default": "hop"},
				}},
			}},
		},
		CustomRules: []*wizardmodels.RuleState{
			{Rule: wizardtemplate.TemplateSelectableRule{Label: "Work rule"}, SelectedOutbound: "hop"},
			{Rule: wizardtemplate.TemplateSelectableRule{Label: "Fold rule"}, SelectedOutbound: "pick"},
		},
		SelectedFinalOutbound: "hop",
		SettingsVars:          map[string]string{"route_final": "hop"},
		DNSServers: []json.RawMessage{
			json.RawMessage(`{"tag":"dns-proxy","type":"udp","server":"1.1.1.1","detour":"hop"}`),
		},
	}
}

func buildNodesByTag(t *testing.T, m *wizardmodels.WizardModel) map[string]map[string]interface{} {
	t.Helper()
	// SPEC 117 (Т2): одноразовая проекция canonical → legacy на входе генератора.
	res, err := config.GenerateOutboundsFromParserConfig(m.AsParserConfig(), map[string]int{}, nil,
		config.DirectionBuildOptions{BlockTag: "block-out", DirectTag: "direct-out"})
	if err != nil {
		t.Fatalf("сборка провалилась: %v", err)
	}
	out := map[string]map[string]interface{}{}
	for _, raw := range append(append([]string(nil), res.OutboundsJSON...), res.EndpointsJSON...) {
		start := strings.Index(raw, "{")
		if start < 0 {
			continue
		}
		var obj map[string]interface{}
		if json.Unmarshal([]byte(strings.TrimSuffix(strings.TrimSpace(raw[start:]), ",")), &obj) != nil {
			continue
		}
		if tag, _ := obj["tag"].(string); tag != "" {
			out[tag] = obj
		}
	}
	return out
}

func memberByTag(t *testing.T, src *corestate.Source, tag string) *corestate.Node {
	t.Helper()
	for i := range src.Nodes {
		if src.Nodes[i].Tag == tag {
			return &src.Nodes[i]
		}
	}
	t.Fatalf("в %q нет узла %q", src.Name, tag)
	return nil
}

func linkIs(l *corestate.NodeLink, folderID, tag string) bool {
	return l != nil && l.FolderID == folderID && l.Tag == tag
}

func TestRootNameRefs_RenameAndDeleteFollowEveryLink(t *testing.T) {
	m := rootNameModel(t)
	work, dep := &m.Sources[2], &m.Sources[1]

	// ── Переименование верхнего узла: все виды ссылок идут за ним ─────────
	m.Sources[0].Tag = "hop2"
	affected := RenameRootNodeRefs(m, "hop", "hop2")

	if !linkIs(dep.Detour, "", "hop2") || !linkIs(memberByTag(t, work, "m1").Detour, "", "hop2") {
		t.Errorf("detour корневого узла %+v и члена папки %+v не переписаны", dep.Detour, memberByTag(t, work, "m1").Detour)
	}
	if h := memberByTag(t, work, "fc").Hops; !linkIs(&h[0], "", "hop2") || !linkIs(&h[1], "01FLD", "exit") {
		t.Errorf("позиции цепочки в папке: %+v", h)
	}
	vpn := &m.GlobalOutbounds[0]
	if !reflect.DeepEqual(vpn.AddOutbounds, []string{"hop2", "direct-out"}) ||
		vpn.Options["default"] != "hop2" || vpn.PreferredDefault["tag"] != "hop2" {
		t.Errorf("опции/умолчания Направления: %v / %v / %v", vpn.AddOutbounds, vpn.Options, vpn.PreferredDefault)
	}
	tmpl := &m.GlobalOutbounds[2]
	userPatch, presetPatch := tmpl.Updates[1].Patch, tmpl.Updates[0].Patch
	if !reflect.DeepEqual(userPatch["addOutbounds"], []interface{}{"hop2", "direct-out"}) ||
		!reflect.DeepEqual(userPatch["options"], map[string]interface{}{"default": "hop2"}) {
		t.Errorf("USER-патч записи-ссылки: %v", userPatch)
	}
	if !reflect.DeepEqual(presetPatch["addOutbounds"], []interface{}{"hop"}) {
		t.Errorf("патч пресета тронут: %v", presetPatch)
	}
	if m.CustomRules[0].SelectedOutbound != "hop2" || m.SelectedFinalOutbound != "hop2" || m.SettingsVars["route_final"] != "hop2" {
		t.Errorf("цель правила %q, route.final %q / %q", m.CustomRules[0].SelectedOutbound, m.SelectedFinalOutbound, m.SettingsVars["route_final"])
	}
	if !strings.Contains(string(m.DNSServers[0]), `"detour":"hop2"`) {
		t.Errorf("detour DNS не переписан: %s", m.DNSServers[0])
	}
	for _, want := range []string{"Proton NL", "Work", "vpn", "tmpl", "Work rule", "route.final", "dns-proxy"} {
		if !strings.Contains(strings.Join(affected, "|"), want) {
			t.Errorf("задетые %v не называют %q", affected, want)
		}
	}

	// Сборка: переписанные ссылки разрешаются, обе цепочки папки — свои
	// outbound'ы (NODE_LINK.md §9.3 п. 2).
	nodes := buildNodesByTag(t, m)
	if got := nodes["dep"]; got == nil || got["detour"] != "hop2" {
		t.Errorf("узел с detour после переименования: %v", got)
	}
	if got := nodes["m1"]; got == nil || got["detour"] != "hop2" {
		t.Errorf("член папки с detour после переименования: %v", got)
	}
	if got := nodes["fc"]; got == nil || !reflect.DeepEqual(got["outbounds"], []interface{}{"hop2", "exit"}) {
		t.Errorf("цепочка fc: %v", got)
	}
	if got := nodes["fd"]; got == nil || !reflect.DeepEqual(got["outbounds"], []interface{}{"vpn", "exit"}) {
		t.Errorf("цепочка fd: %v", got)
	}

	// ── Направление и свёртка: позиции в папках и detour членов папок ─────
	RenameDirection(m, "vpn", "vpn2")
	if !linkIs(memberByTag(t, work, "m2").Detour, "", "vpn2") || !linkIs(&memberByTag(t, work, "fd").Hops[0], "", "vpn2") {
		t.Errorf("Направление: detour члена %+v, позиция цепочки в папке %+v",
			memberByTag(t, work, "m2").Detour, memberByTag(t, work, "fd").Hops)
	}
	fold := &m.Sources[3]
	before := *fold.Replace
	fold.Replace = &corestate.FolderReplace{Mode: corestate.FolderReplaceBoth, Tag: "pick2"}
	if n := RenameFoldRefs(m, &before, fold.Replace); n != 3 {
		t.Errorf("свёртка: переписано %d ссылок, ожидалось 3", n)
	}
	if m.CustomRules[1].SelectedOutbound != "pick2" || m.GlobalOutbounds[1].AddOutbounds[0] != "pick2" ||
		!linkIs(&m.Sources[4].Hops[0], "", "pick2-auto") {
		t.Errorf("свёртка: правило %q, опция %v, позиция %+v",
			m.CustomRules[1].SelectedOutbound, m.GlobalOutbounds[1].AddOutbounds, m.Sources[4].Hops)
	}

	// ── Тёзка: имя, которое носит ещё кто-то, операции не трогают ─────────
	twin := corestate.Source{ID: "01TWIN", Node: renameNode(t, "dep", renameHopURI, nil)}
	m.Sources = append(m.Sources, twin)
	m.Sources[1].Tag = "dep-renamed"
	if got := RenameRootNodeRefs(m, "dep", "dep-renamed"); got != nil || m.Sources[4].Hops[1].Tag != "dep" {
		t.Errorf("ссылка на живого тёзку переписана: задетые %v, позиция %+v", got, m.Sources[4].Hops[1])
	}
	m.Sources = m.Sources[:len(m.Sources)-1]

	// ── Удаление верхнего узла: ссылки гаснут вместе с ним ────────────────
	m.Sources = append(m.Sources[:0], m.Sources[1:]...)
	work, dep = &m.Sources[1], &m.Sources[0]
	affected = ClearRootNodeRefs(m, "hop2")

	if dep.Detour != nil || memberByTag(t, work, "m1").Detour != nil {
		t.Errorf("detour на удалённый узел не погас: %+v / %+v", dep.Detour, memberByTag(t, work, "m1").Detour)
	}
	if h := memberByTag(t, work, "fc").Hops; len(h) != 1 || !linkIs(&h[0], "01FLD", "exit") {
		t.Errorf("позиция удалённого узла осталась в цепочке: %+v", h)
	}
	if !reflect.DeepEqual(m.GlobalOutbounds[0].AddOutbounds, []string{"direct-out"}) ||
		!reflect.DeepEqual(m.GlobalOutbounds[2].Updates[1].Patch["addOutbounds"], []interface{}{"direct-out"}) {
		t.Errorf("опция удалённого узла осталась у Направления: %v / %v",
			m.GlobalOutbounds[0].AddOutbounds, m.GlobalOutbounds[2].Updates[1].Patch["addOutbounds"])
	}
	// Одиночные цели — как при удалении Направления: остаются и называются.
	if m.CustomRules[0].SelectedOutbound != "hop2" || m.SelectedFinalOutbound != "hop2" ||
		m.GlobalOutbounds[0].Options["default"] != "hop2" || m.GlobalOutbounds[0].PreferredDefault["tag"] != "hop2" ||
		!strings.Contains(string(m.DNSServers[0]), `"detour":"hop2"`) {
		t.Errorf("одиночные цели изменены удалением узла")
	}
	for _, want := range []string{"Proton NL", "Work", "vpn2", "tmpl", "Work rule", "route.final", "dns-proxy"} {
		if !strings.Contains(strings.Join(affected, "|"), want) {
			t.Errorf("удаление: задетые %v не называют %q", affected, want)
		}
	}
}
