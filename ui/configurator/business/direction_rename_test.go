package business

import (
	"encoding/json"
	"testing"

	corestate "singbox-launcher/core/state"

	"singbox-launcher/core/config/configtypes"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// renameTestModel — модель, где на тег `vpn-1` ссылается КАЖДЫЙ вид ссылки.
// Один тест на все шесть: пропущенный вид оставляет ссылку в никуда, и
// заметно это станет только на сборке конфига у пользователя.
func renameTestModel() *wizardmodels.WizardModel {
	m := &wizardmodels.WizardModel{
		GlobalOutbounds: []configtypes.Direction{
			{Tag: "vpn-1", Type: "selector", Auto: &configtypes.DirectionAuto{}},
			{Tag: "vpn-2", Type: "selector", AddOutbounds: []string{"vpn-1", "vpn-1-auto", "direct-out"}},
		},
		CustomRules: []*wizardmodels.RuleState{
			{SelectedOutbound: "vpn-1"},
			{SelectedOutbound: "vpn-1-auto"},
			{SelectedOutbound: "vpn-2"},
		},
		SelectedFinalOutbound: "vpn-1",
		SettingsVars:          map[string]string{"route_final": "vpn-1"},
		PresetRefs: []*wizardmodels.PresetRefState{
			{Ref: "russian", Enabled: true, Vars: map[string]string{"out": "vpn-1"}},
		},
		Sources: []corestate.Source{
			{Node: corestate.Node{Kind: corestate.SourceKindChain, Hops: []corestate.NodeLink{{Tag: "node-a"}, {Tag: "vpn-1"}}}},
		},
		DNSServers: []json.RawMessage{
			json.RawMessage(`{"tag":"dns-proxy","type":"udp","server":"1.1.1.1","detour":"vpn-1"}`),
			json.RawMessage(`{"tag":"dns-direct","type":"udp","server":"8.8.8.8","detour":"direct-out"}`),
		},
	}
	return m
}

func TestRenameDirectionRewritesEveryReference(t *testing.T) {
	m := renameTestModel()

	if n := RenameDirection(m, "vpn-1", "Германия"); n == 0 {
		t.Fatal("переименование не переписало ни одной ссылки")
	}

	if m.GlobalOutbounds[0].Tag != "Германия" {
		t.Errorf("сам тег не сменился: %q", m.GlobalOutbounds[0].Tag)
	}
	// Опции соседнего Направления: и сам тег, и его двойник.
	if got := m.GlobalOutbounds[1].AddOutbounds; got[0] != "Германия" || got[1] != "Германия-auto" {
		t.Errorf("addOutbounds не переписаны: %v", got)
	}
	if got2 := m.GlobalOutbounds[1].AddOutbounds[2]; got2 != "direct-out" {
		t.Errorf("чужая опция задета: %q", got2)
	}
	if m.CustomRules[0].SelectedOutbound != "Германия" {
		t.Errorf("цель правила не переписана: %q", m.CustomRules[0].SelectedOutbound)
	}
	if m.CustomRules[1].SelectedOutbound != "Германия-auto" {
		t.Errorf("цель правила на двойник не переписана: %q", m.CustomRules[1].SelectedOutbound)
	}
	if m.CustomRules[2].SelectedOutbound != "vpn-2" {
		t.Errorf("чужая цель задета: %q", m.CustomRules[2].SelectedOutbound)
	}
	if m.SelectedFinalOutbound != "Германия" || m.SettingsVars["route_final"] != "Германия" {
		t.Errorf("route.final не переписан: %q / %q", m.SelectedFinalOutbound, m.SettingsVars["route_final"])
	}
	if m.PresetRefs[0].Vars["out"] != "Германия" {
		t.Errorf("outbound-переменная пресета не переписана: %q", m.PresetRefs[0].Vars["out"])
	}
	if m.Sources[0].Hops[1].Tag != "Германия" {
		t.Errorf("позиция цепочки не переписана: %v", m.Sources[0].Hops)
	}
	if m.Sources[0].Hops[0].Tag != "node-a" {
		t.Errorf("чужая позиция задета: %v", m.Sources[0].Hops)
	}

	var srv map[string]any
	if err := json.Unmarshal(m.DNSServers[0], &srv); err != nil {
		t.Fatalf("DNS-сервер перестал разбираться: %v", err)
	}
	if srv["detour"] != "Германия" {
		t.Errorf("detour не переписан: %v", srv["detour"])
	}
	if srv["server"] != "1.1.1.1" || srv["tag"] != "dns-proxy" {
		t.Errorf("правка detour потеряла остальные поля: %v", srv)
	}
	if string(m.DNSServers[1]) != `{"tag":"dns-direct","type":"udp","server":"8.8.8.8","detour":"direct-out"}` {
		t.Errorf("сервер без ссылки переписан на ровном месте: %s", m.DNSServers[1])
	}
}

// SPEC 118 W5: локальных Направлений источника нет — переименование правит
// ОДНО canonical-место списка Направлений (GlobalOutbounds) плюс ссылки
// модели (хопы цепочек, detour источников, DNS). Проверяем, что ссылка
// detour источника тоже переписывается: повисшая цель = fail-closed, то есть
// источник молча выпал бы из конфига.
func TestRenameDirection_CanonicalOnly(t *testing.T) {
	m := renameTestModel()
	m.Sources = append(m.Sources, corestate.Source{
		Node: corestate.Node{
			Kind: corestate.SourceKindSubscription, Enabled: true,
			Detour: &corestate.NodeLink{Tag: "vpn-1"},
		},
		URL: "https://example.com/sub",
	})

	RenameDirection(m, "vpn-1", "vpn-9")

	if m.GlobalOutbounds[0].Tag != "vpn-9" {
		t.Errorf("canonical-тег не переименован: %q", m.GlobalOutbounds[0].Tag)
	}
	if got := m.GlobalOutbounds[1].AddOutbounds; got[0] != "vpn-9" || got[1] != "vpn-9-auto" {
		t.Errorf("ссылки в GlobalOutbounds не переписаны: %v", got)
	}
	if d := m.Sources[len(m.Sources)-1].Detour; d == nil || d.Tag != "vpn-9" {
		t.Errorf("ссылка detour источника не переписана: %+v", d)
	}
}

func TestRenameDirectionNoopCases(t *testing.T) {
	m := renameTestModel()
	if n := RenameDirection(m, "vpn-1", "vpn-1"); n != 0 {
		t.Errorf("переименование в то же имя не должно ничего трогать, got %d", n)
	}
	if n := RenameDirection(m, "vpn-1", "   "); n != 0 {
		t.Errorf("пустое имя не должно применяться, got %d", n)
	}
	if m.GlobalOutbounds[0].Tag != "vpn-1" {
		t.Errorf("тег испорчен no-op вызовом: %q", m.GlobalOutbounds[0].Tag)
	}
	if n := RenameDirection(nil, "a", "b"); n != 0 {
		t.Errorf("nil-модель, got %d", n)
	}
}

// Тег един для Направлений, узлов и служебных outbound'ов: два объекта под
// одним именем на сборке схлопнулись бы в один.
func TestDirectionTagTaken(t *testing.T) {
	m := renameTestModel()

	if !DirectionTagTaken(m, "vpn-2", "vpn-1") {
		t.Error("тег соседнего Направления обязан считаться занятым")
	}
	if !DirectionTagTaken(m, "direct-out", "vpn-1") {
		t.Error("служебный outbound обязан считаться занятым")
	}
	if !DirectionTagTaken(m, "vpn-1-auto", "") {
		t.Error("имя парной auto-группы занято её родителем")
	}
	if DirectionTagTaken(m, "vpn-1", "vpn-1") {
		t.Error("собственное имя записи не занято — Save без правки тега обычный сценарий")
	}
	if DirectionTagTaken(m, "совершенно-свободный", "vpn-1") {
		t.Error("свободный тег объявлен занятым")
	}
	if DirectionTagTaken(m, "", "vpn-1") {
		t.Error("пустой тег отсекается отдельной проверкой, не этой")
	}
}
