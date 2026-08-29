package business

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config"
	"singbox-launcher/core/config/subscription"
	corestate "singbox-launcher/core/state"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// SPEC 112-A, «Смена идентичности узла = сброс ссылок с предупреждением» —
// сквозной сценарий.
//
// Резолв ссылки на сборке строгий, поэтому переименование узла обязано
// сбрасывать ссылки на него ПРЯМО В МОМЕНТ правки. Проверяется оба конца:
// ссылка гаснет в состоянии, и следующая сборка проходит без fail-closed —
// зависимый источник остаётся в конфиге, просто без detour.

const renameHopURI = "vless://b831381d-6324-4d53-ad4f-8cda48b30811@hop.example.com:443?encryption=none&security=tls&sni=hop.example.com#hop"

const renameDependentURI = "vless://c931381d-6324-4d53-ad4f-8cda48b30811@dep.example.com:443?encryption=none&security=tls&sni=dep.example.com#dep"

func modelWithLiveNodeRef(hopTag string) *wizardmodels.WizardModel {
	m := &wizardmodels.WizardModel{Sources: []corestate.Source{
		{ID: "01WARP", Node: corestate.Node{Kind: corestate.SourceKindServer, Enabled: true},
			Label: "WARP hop", NodeTag: hopTag, URI: renameHopURI},
		{ID: "01DEP", Node: corestate.Node{Kind: corestate.SourceKindServer, Enabled: true},
			Label: "Proton NL", NodeTag: "🇳🇱 Proton NL", URI: renameDependentURI,
			DetourNodeSourceID: "01WARP", DetourNodeTag: hopTag, DetourNodeLabel: "WARP hop"},
	}}
	return m
}

func buildNodesByTag(t *testing.T, m *wizardmodels.WizardModel) map[string]map[string]interface{} {
	t.Helper()
	// SPEC 117 (Т2): одноразовая проекция canonical → legacy на входе генератора.
	res, err := config.GenerateOutboundsFromParserConfig(m.AsParserConfig(), map[string]int{}, nil,
		func(ps config.ProxySource, tc map[string]int, pc func(float64, string), idx, total int) ([]*config.ParsedNode, error) {
			return subscription.LoadNodesFromSource(ps, tc, pc, idx, total)
		},
		config.DirectionBuildOptions{BlockTag: "block-out", DirectTag: "direct-out"})
	if err != nil {
		t.Fatalf("сборка провалилась: %v", err)
	}
	out := map[string]map[string]interface{}{}
	for _, raw := range append(append([]string(nil), res.OutboundsJSON...), res.EndpointsJSON...) {
		obj := decodeEmittedForRenameTest(raw)
		if obj == nil {
			continue
		}
		if tag, _ := obj["tag"].(string); tag != "" {
			out[tag] = obj
		}
	}
	return out
}

func decodeEmittedForRenameTest(raw string) map[string]interface{} {
	start := strings.Index(raw, "{")
	if start < 0 {
		return nil
	}
	body := strings.TrimSuffix(strings.TrimSpace(raw[start:]), ",")
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		return nil
	}
	return obj
}

// Контроль: до переименования ссылка живая и detour в конфиге стоит.
func TestNodeRename_RefWorksBeforeRename(t *testing.T) {
	m := modelWithLiveNodeRef("🔥🎭 WARP (MASQUE)")
	nodes := buildNodesByTag(t, m)

	dep := nodes["🇳🇱 Proton NL"]
	if dep == nil {
		t.Fatalf("зависимый узел отсутствует; собрано %v", tagsOf(nodes))
	}
	if got, _ := dep["detour"].(string); got != "🔥🎭 WARP (MASQUE)" {
		t.Fatalf("detour = %q, ожидался тег хопа", got)
	}
}

// Переименование узла: ссылка сброшена в состоянии, сборка проходит без
// fail-closed — зависимый источник в конфиге, просто без detour.
func TestNodeRename_ResetsRefAndBuildStaysClean(t *testing.T) {
	m := modelWithLiveNodeRef("🔥🎭 WARP (MASQUE)")

	// Пользователь переименовал узел в форме источника; сохранение зовёт
	// ResetDetourNodeRefs со СТАРЫМ именем.
	m.Sources[0].NodeTag = "🔥🎭 WARP v2"
	affected := ResetDetourNodeRefs(m, "01WARP", "🔥🎭 WARP (MASQUE)")
	if len(affected) != 1 || affected[0] != "Proton NL" {
		t.Fatalf("затронутые источники = %v, ожидался [Proton NL]", affected)
	}
	if s := m.Sources[1]; s.DetourNodeSourceID != "" || s.DetourNodeTag != "" {
		t.Fatalf("ссылка обязана погаснуть в состоянии, осталось %+v", s)
	}

	nodes := buildNodesByTag(t, m)
	dep := nodes["🇳🇱 Proton NL"]
	if dep == nil {
		t.Fatalf("зависимый узел выпал fail-closed, хотя ссылка сброшена; собрано %v", tagsOf(nodes))
	}
	if _, has := dep["detour"]; has {
		t.Errorf("detour обязан отсутствовать после сброса ссылки: %v", dep["detour"])
	}
	if nodes["🔥🎭 WARP v2"] == nil {
		t.Errorf("переименованный узел отсутствует; собрано %v", tagsOf(nodes))
	}
}

// Без сброса та же сборка падает fail-closed — это и есть та честность,
// ради которой UI обязан сбрасывать ссылки сам.
func TestNodeRename_WithoutResetFailsClosed(t *testing.T) {
	m := modelWithLiveNodeRef("🔥🎭 WARP (MASQUE)")
	m.Sources[0].NodeTag = "🔥🎭 WARP v2" // переименовали, ссылку не тронули

	nodes := buildNodesByTag(t, m)
	if nodes["🇳🇱 Proton NL"] != nil {
		t.Fatalf("источник со сломанной ссылкой обязан выпасть fail-closed; собрано %v", tagsOf(nodes))
	}
}

func tagsOf(nodes map[string]map[string]interface{}) []string {
	out := make([]string, 0, len(nodes))
	for tag := range nodes {
		out = append(out, tag)
	}
	return out
}
