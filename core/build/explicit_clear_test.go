// File explicit_clear_test.go — осознанная очистка поля переживает Save
// (корень проблемы с `default: "proxy-out"`).
//
// До флага Explicit оба рубежа защиты судили по признаку «значение пустое»
// и не различали два разных случая с одинаковым содержимым:
//
//   - артефакт pre-058: форма отдавала пустыми поля, которых пользователь
//     не трогал → патч затирал пресетный фильтр;
//   - очистка: пользователь стёр поле → пустое значение и есть правка.
//
// Второй молча терялся: убранное умолчание не сохранялось, а в конфиг
// уезжала мёртвая ссылка, на которой ядро не стартует.
package build

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// Живой случай: у «vpn ②» шаблонное умолчание proxy-out, пользователь его
// убрал вместе с галкой состава.
func TestExplicitClear_PreferredDefaultSurvives(t *testing.T) {
	base := configtypes.Direction{
		Tag:              "vpn ②",
		Type:             "selector",
		AddOutbounds:     []string{"direct-out", "proxy-out"},
		PreferredDefault: map[string]interface{}{"tag": "/proxy-out/i"},
	}
	form := base
	form.PreferredDefault = nil
	form.AddOutbounds = []string{"direct-out"}

	patch := OutboundFieldDiff(form, base)
	updates := UpsertUserPatch(nil, patch, true)

	if len(updates) != 1 {
		t.Fatalf("правка потеряна: updates=%+v", updates)
	}
	if !updates[0].Explicit {
		t.Error("патч с очисткой не помечен Explicit — рубеж загрузки его снимет")
	}
	if _, ok := updates[0].Patch["preferredDefault"]; !ok {
		t.Errorf("очистка умолчания не записана: %+v", updates[0].Patch)
	}
}

// Патч без пустых значений пишется как раньше — без флага, байт-в-байт.
func TestExplicitClear_PlainEditNotMarked(t *testing.T) {
	base := configtypes.Direction{Tag: "vpn-1", Type: "selector", Label: "старое"}
	form := base
	form.Label = "новое"

	updates := UpsertUserPatch(nil, OutboundFieldDiff(form, base), true)
	if len(updates) != 1 {
		t.Fatalf("правка потеряна: %+v", updates)
	}
	if updates[0].Explicit {
		t.Error("обычная правка помечена Explicit — флаг должен стоять только на очистке")
	}
}

// Форма, открытая и закрытая без изменений, не оставляет патча вовсе.
func TestExplicitClear_NoopSaveWritesNothing(t *testing.T) {
	base := configtypes.Direction{
		Tag:     "proxy-out",
		Type:    "selector",
		Filters: map[string]interface{}{"tag": "!/(🇷🇺)/i"},
	}
	updates := UpsertUserPatch(nil, OutboundFieldDiff(base, base), true)
	if len(updates) != 0 {
		t.Errorf("Save без изменений оставил патч: %+v", updates)
	}
}

// Очистка одного поля вместе с правкой другого — тоже осознанный ввод.
func TestExplicitClear_MixedEditKeepsBoth(t *testing.T) {
	base := configtypes.Direction{
		Tag:     "proxy-out",
		Type:    "selector",
		Label:   "старое",
		Filters: map[string]interface{}{"tag": "!/(🇷🇺)/i"},
	}
	form := base
	form.Label = "новое"
	form.Filters = nil

	updates := UpsertUserPatch(nil, OutboundFieldDiff(form, base), true)
	if len(updates) != 1 || !updates[0].Explicit {
		t.Fatalf("смешанная правка: %+v", updates)
	}
	if updates[0].Patch["label"] != "новое" {
		t.Errorf("правка имени потеряна: %+v", updates[0].Patch)
	}
	if _, ok := updates[0].Patch["filters"]; !ok {
		t.Errorf("очистка фильтра потеряна: %+v", updates[0].Patch)
	}
}
