package business

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config"
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

// renameNode — верхний узел-сервер с материализованным телом: SPEC 118 Т2,
// узел без тела собирать не из чего.
func renameNode(t *testing.T, id, tag, label, uri string, detour *corestate.NodeLink) corestate.Source {
	t.Helper()
	mat, err := config.MaterializeServerNode(uri, nil)
	if err != nil {
		t.Fatalf("материализация %q: %v", tag, err)
	}
	return corestate.Source{
		ID: id,
		Node: corestate.Node{
			Kind: corestate.SourceKindServer, Enabled: true, Tag: tag,
			Body:   mat.Body,
			Origin: &corestate.Origin{Kind: mat.OriginKind, Raw: mat.OriginRaw},
			Detour: detour,
		},
		Label: label,
	}
}

func modelWithLiveNodeRef(t *testing.T, hopTag string) *wizardmodels.WizardModel {
	return &wizardmodels.WizardModel{Sources: []corestate.Source{
		renameNode(t, "01WARP", hopTag, "WARP hop", renameHopURI, nil),
		renameNode(t, "01DEP", "🇳🇱 Proton NL", "Proton NL", renameDependentURI,
			&corestate.NodeLink{Tag: hopTag}),
	}}
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
	m := modelWithLiveNodeRef(t, "🔥🎭 WARP (MASQUE)")
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
	m := modelWithLiveNodeRef(t, "🔥🎭 WARP (MASQUE)")

	// Пользователь переименовал узел в форме источника; сохранение зовёт
	// ResetDetourNodeRefs со СТАРЫМ именем.
	m.Sources[0].Tag = "🔥🎭 WARP v2"
	affected := ResetDetourNodeRefs(m, "01WARP", "🔥🎭 WARP (MASQUE)")
	if len(affected) != 1 || affected[0] != "Proton NL" {
		t.Fatalf("затронутые источники = %v, ожидался [Proton NL]", affected)
	}
	if d := m.Sources[1].Detour; d != nil {
		t.Fatalf("ссылка обязана погаснуть в состоянии, осталось %+v", d)
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
	m := modelWithLiveNodeRef(t, "🔥🎭 WARP (MASQUE)")
	m.Sources[0].Tag = "🔥🎭 WARP v2" // переименовали, ссылку не тронули

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
