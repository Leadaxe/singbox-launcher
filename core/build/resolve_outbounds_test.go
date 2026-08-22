package build

import (
	"reflect"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/template"
)

// TestApplyOutboundUpdatePatch_Empty — пустой patch = noop.
func TestApplyOutboundUpdatePatch_Empty(t *testing.T) {
	target := configtypes.Direction{Tag: "x", Type: "selector"}
	out := applyOutboundUpdatePatch(target, nil, false)
	if out.Tag != "x" || out.Type != "selector" {
		t.Errorf("noop patch changed target: %+v", out)
	}
}

// TestApplyOutboundUpdatePatch_UserAddOutboundsReplace — USER patch заменяет
// addOutbounds целиком: снятый в форме тег (proxy-out) не должен доливаться
// обратно union'ом из базового списка.
func TestApplyOutboundUpdatePatch_UserAddOutboundsReplace(t *testing.T) {
	base := configtypes.Direction{
		Tag:          "Proxy group 2",
		Type:         "selector",
		AddOutbounds: []string{"direct-out", "proxy-out"},
	}
	patch := map[string]interface{}{
		"addOutbounds": []interface{}{"direct-out"},
	}
	out := applyOutboundUpdatePatch(base, patch, true)
	if want := []string{"direct-out"}; !reflect.DeepEqual(out.AddOutbounds, want) {
		t.Errorf("user patch must replace addOutbounds: got %v, want %v", out.AddOutbounds, want)
	}
}

// TestApplyOutboundUpdatePatch_UserAddOutboundsClear — пустой addOutbounds в
// USER patch = юзер снял все галки; список очищается, а не игнорируется.
func TestApplyOutboundUpdatePatch_UserAddOutboundsClear(t *testing.T) {
	base := configtypes.Direction{
		Tag:          "Proxy group 2",
		Type:         "selector",
		AddOutbounds: []string{"direct-out", "proxy-out"},
	}
	patch := map[string]interface{}{
		"addOutbounds": []interface{}{},
	}
	out := applyOutboundUpdatePatch(base, patch, true)
	if len(out.AddOutbounds) != 0 {
		t.Errorf("user patch with empty addOutbounds must clear list, got %v", out.AddOutbounds)
	}
}

// TestApplyOutboundUpdatePatch_PresetAddOutboundsUnion — preset patch сохраняет
// union-семантику: добавляет теги к базовым, ничего не удаляя.
func TestApplyOutboundUpdatePatch_PresetAddOutboundsUnion(t *testing.T) {
	base := configtypes.Direction{
		Tag:          "Proxy group 2",
		Type:         "selector",
		AddOutbounds: []string{"direct-out"},
	}
	patch := map[string]interface{}{
		"addOutbounds": []interface{}{"warp-out"},
	}
	out := applyOutboundUpdatePatch(base, patch, false)
	if want := []string{"direct-out", "warp-out"}; !reflect.DeepEqual(out.AddOutbounds, want) {
		t.Errorf("preset patch must union addOutbounds: got %v, want %v", out.AddOutbounds, want)
	}
}

// TestMergeOutboundUpdates_UserPatchRemovesAddOutbound — сквозной сценарий
// бага: referenced entry с USER patch, где юзер снял proxy-out; merged view
// не должен содержать снятый тег.
func TestMergeOutboundUpdates_UserPatchRemovesAddOutbound(t *testing.T) {
	ob := configtypes.Direction{
		Tag:          "Proxy group 2",
		Type:         "selector",
		AddOutbounds: []string{"direct-out", "proxy-out"},
		Updates: []configtypes.OutboundUpdate{
			{
				Ref: configtypes.RefUser,
				Patch: map[string]interface{}{
					"addOutbounds": []interface{}{"direct-out"},
				},
			},
		},
	}
	merged := MergeOutboundUpdates(ob, nil, template.TargetSpec{})
	if want := []string{"direct-out"}; !reflect.DeepEqual(merged.AddOutbounds, want) {
		t.Errorf("merged addOutbounds: got %v, want %v", merged.AddOutbounds, want)
	}
}

// TestMergeOutboundUpdatesInPlace_DropsOrphanedTemplateRef — referenced entry,
// чей тег исчез из шаблона (например после переименования записи), не должен
// эмититься заглушкой с пустым type: sing-box на ней бракует весь конфиг
// («outbounds[N]: unknown outbound type: \"\"»). Запись дропается; соседние
// resolvable entries остаются.
func TestMergeOutboundUpdatesInPlace_DropsOrphanedTemplateRef(t *testing.T) {
	td := &template.TemplateData{
		ParserConfig: `{"ParserConfig":{"outbounds":[
			{"tag":"vpn ②","type":"selector"}
		]}}`,
	}
	pc := &configtypes.ParserConfig{}
	pc.ParserConfig.Outbounds = []configtypes.Direction{
		{Tag: "vpn ②", Ref: configtypes.RefTemplate},
		{Tag: "vpn ② – detour", Ref: configtypes.RefTemplate, Updates: []configtypes.OutboundUpdate{
			{Ref: configtypes.RefUser, Patch: map[string]interface{}{
				"addOutbounds": []interface{}{"direct-out"},
			}},
		}},
		{Tag: "direct-group", Type: "selector"},
	}
	MergeOutboundUpdatesInPlace(pc, td, template.TargetSpec{})

	tags := make([]string, 0, len(pc.ParserConfig.Outbounds))
	for _, ob := range pc.ParserConfig.Outbounds {
		tags = append(tags, ob.Tag)
		if ob.Type == "" {
			t.Errorf("outbound %q emitted with empty type", ob.Tag)
		}
	}
	if want := []string{"vpn ②", "direct-group"}; !reflect.DeepEqual(tags, want) {
		t.Errorf("kept outbounds: got %v, want %v", tags, want)
	}
}

// TestMergeOutboundUpdatesInPlace_NilTemplateKeepsLegacyBodies — td==nil
// (legacy SPEC 057 state): referenced entries с inline body не дропаются —
// body есть, type непустой.
func TestMergeOutboundUpdatesInPlace_NilTemplateKeepsLegacyBodies(t *testing.T) {
	pc := &configtypes.ParserConfig{}
	pc.ParserConfig.Outbounds = []configtypes.Direction{
		{Tag: "legacy-group", Type: "selector", Ref: configtypes.RefTemplate},
	}
	MergeOutboundUpdatesInPlace(pc, nil, template.TargetSpec{})
	if len(pc.ParserConfig.Outbounds) != 1 {
		t.Fatalf("legacy inline-body entry must survive td==nil merge, got %v", pc.ParserConfig.Outbounds)
	}
}

// TestDropDanglingNodeDetours — сценарий Proton NL: endpoint с
// detour_tag на несуществующую группу не должен валить весь конфиг на
// remote apply («dependency[Proton NL] not found»); висячий detour дропается
// fail-open, резолвящийся и отсутствующий — не трогаются.
func TestDropDanglingNodeDetours(t *testing.T) {
	finalTags := map[string]bool{"detour-group": true}
	entries := []string{
		"// Proton NL\n{\"detour\":\"missing-group\",\"tag\":\"Proton NL\",\"type\":\"wireguard\"}",
		`{"detour":"detour-group","tag":"ok-node","type":"vless"}`,
		`{"tag":"plain-node","type":"vless"}`,
		"not json at all",
	}
	got := dropDanglingNodeDetours(entries, finalTags, true)
	if len(got) != len(entries) {
		t.Fatalf("entry count changed: got %d, want %d", len(got), len(entries))
	}
	if strings.Contains(got[0], "missing-group") {
		t.Errorf("dangling detour must be dropped: %s", got[0])
	}
	if !strings.Contains(got[0], "// Proton NL") || !strings.Contains(got[0], `"tag":"Proton NL"`) {
		t.Errorf("comment prefix/tag must survive: %s", got[0])
	}
	for i := 1; i < len(entries); i++ {
		if got[i] != entries[i] {
			t.Errorf("entry %d must be untouched: got %s", i, got[i])
		}
	}
}
