package build

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// Открыть Направление и нажать Save, ничего не меняя, НЕ должно порождать
// USER-патч.
//
// Так уже ломался пресетный фильтр (пустой патч затирал `!/(🇷🇺)/i`), и так
// же ломалась автогруппа: форма не читала interrupt_exist_connections и
// idle_timeout — полей, которые задаёт шаблон и которых в форме нет. На
// Collect они пропадали, diff видел различие и записывал патч, хотя
// пользователь ничего не трогал.
func TestUnchangedAutoProducesNoPatch(t *testing.T) {
	base := configtypes.Direction{
		Tag:  "proxy-out",
		Type: "selector",
		Auto: &configtypes.DirectionAuto{
			URL:                       "@urltest_url",
			Interval:                  "@urltest_interval",
			Tolerance:                 configtypes.NewTemplateVar("urltest_tolerance"),
			InterruptExistConnections: boolPtr(true),
		},
	}

	// Форма вернула то же самое, включая непоказанные поля.
	form := base
	form.Auto = &configtypes.DirectionAuto{
		URL:                       "@urltest_url",
		Interval:                  "@urltest_interval",
		Tolerance:                 configtypes.NewTemplateVar("urltest_tolerance"),
		InterruptExistConnections: boolPtr(true),
	}

	if diff := OutboundFieldDiff(form, base); len(diff) > 0 {
		t.Errorf("непустой патч на неизменённой записи: %+v", diff)
	}
}

// Потеря непоказанного поля — это ИЗМЕНЕНИЕ, и оно обязано попасть в патч:
// иначе «пользователь выключил разрыв соединений» молча потерялось бы.
func TestDroppedInterruptFlagIsRealChange(t *testing.T) {
	base := configtypes.Direction{
		Tag: "proxy-out",
		Auto: &configtypes.DirectionAuto{
			URL:                       "@urltest_url",
			InterruptExistConnections: boolPtr(true),
		},
	}
	form := base
	form.Auto = &configtypes.DirectionAuto{
		URL:                       "@urltest_url",
		InterruptExistConnections: boolPtr(false),
	}
	if diff := OutboundFieldDiff(form, base); len(diff) == 0 {
		t.Error("настоящая правка не попала в патч")
	}
}

func boolPtr(b bool) *bool { return &b }
