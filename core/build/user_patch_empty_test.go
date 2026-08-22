package build

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// Форма, открытая и сохранённая без изменений, не должна оставлять после
// себя USER-патч из пустых значений: он затирает пресетный фильтр и
// воспроизводит сам себя на каждом Save (найдено в живом state.json).
func TestUpsertUserPatchDropsAllEmptyPatch(t *testing.T) {
	base := []configtypes.OutboundUpdate{{
		Ref: "russian", Patch: map[string]interface{}{"filters": map[string]interface{}{"tag": "!/(🇷🇺)/i"}},
	}}
	empty := map[string]interface{}{
		"addOutbounds": []interface{}{}, "comment": "", "filters": map[string]interface{}{},
	}
	got := UpsertUserPatch(base, empty)
	if len(got) != 1 || got[0].Ref != "russian" {
		t.Fatalf("пустой патч должен быть отброшен, пресетный — сохранён: %+v", got)
	}

	// Существующий мусорный USER-патч снимается тем же путём.
	withJunk := append(base, configtypes.OutboundUpdate{Ref: configtypes.RefUser, Patch: empty})
	got = UpsertUserPatch(withJunk, empty)
	if len(got) != 1 {
		t.Fatalf("мусорный USER-патч должен уйти: %+v", got)
	}
}

// Осознанные значения остаются: false, 0 и явный null у auto — не «пусто».
func TestUpsertUserPatchKeepsMeaningfulZeroes(t *testing.T) {
	got := UpsertUserPatch(nil, map[string]interface{}{
		"disabled": false, "auto": nil, "filters": map[string]interface{}{},
	})
	if len(got) != 1 {
		t.Fatalf("патч с осмысленными значениями потерян: %+v", got)
	}
	p := got[0].Patch
	if _, ok := p["disabled"]; !ok {
		t.Fatalf("disabled:false выброшен: %+v", p)
	}
	if _, ok := p["auto"]; !ok {
		t.Fatalf("auto:null выброшен: %+v", p)
	}
	if _, ok := p["filters"]; ok {
		t.Fatalf("пустой filters должен быть выброшен: %+v", p)
	}
}

// Очистка вместе с реальной правкой — намеренная и сохраняется.
func TestUpsertUserPatchKeepsEmptyAlongsideRealChange(t *testing.T) {
	got := UpsertUserPatch(nil, map[string]interface{}{
		"filters": map[string]interface{}{}, "comment": "my note",
	})
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	if got[0].Patch["comment"] != "my note" {
		t.Fatalf("реальная правка потеряна: %+v", got[0].Patch)
	}
	// Пустой filters рядом с реальной правкой отброшен — пресетный фильтр
	// остаётся в силе. Чтобы СНЯТЬ его, есть Reset или осознанно заданный
	// другой фильтр.
	if _, ok := got[0].Patch["filters"]; ok {
		t.Fatalf("пустой filters не должен затирать пресет: %+v", got[0].Patch)
	}
}
