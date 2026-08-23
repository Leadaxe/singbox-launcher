// File explicit_clear_apply_test.go — очистка доезжает до итогового тела.
//
// Мало сохранить патч `preferredDefault: {}` — применение обязано УБРАТЬ
// шаблонное значение. Иначе флаг Explicit чинил бы хранение, а в конфиг
// по-прежнему уезжала мёртвая ссылка `default: "proxy-out"`, на которой
// ядро не стартует.
package build

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

func TestExplicitClear_AppliedToBody(t *testing.T) {
	base := configtypes.Direction{
		Tag:              "vpn ②",
		Type:             "selector",
		AddOutbounds:     []string{"direct-out", "proxy-out"},
		PreferredDefault: map[string]interface{}{"tag": "/proxy-out/i"},
		Filters:          map[string]interface{}{"tag": "!/(🔥|Proton)/i"},
	}
	patch := map[string]interface{}{
		"preferredDefault": map[string]interface{}{},
		"addOutbounds":     []interface{}{"direct-out"},
	}

	got := applyOutboundUpdatePatch(base, patch, true)

	if len(got.PreferredDefault) != 0 {
		t.Errorf("умолчание не убрано: %+v — в конфиг уедет мёртвая ссылка", got.PreferredDefault)
	}
	if len(got.AddOutbounds) != 1 || got.AddOutbounds[0] != "direct-out" {
		t.Errorf("состав = %v, ожидали [direct-out]", got.AddOutbounds)
	}
	// Фильтр пользователь не трогал — он обязан уцелеть.
	if got.Filters["tag"] != "!/(🔥|Proton)/i" {
		t.Errorf("фильтр затёрт правкой соседнего поля: %+v", got.Filters)
	}
}

// Очистка фильтра — тот же механизм, и она НЕ должна задевать умолчание.
func TestExplicitClear_FilterClearedIndependently(t *testing.T) {
	base := configtypes.Direction{
		Tag:              "proxy-out",
		Type:             "selector",
		Filters:          map[string]interface{}{"tag": "!/(🇷🇺)/i"},
		PreferredDefault: map[string]interface{}{"tag": "/de/i"},
	}
	got := applyOutboundUpdatePatch(base, map[string]interface{}{
		"filters": map[string]interface{}{},
	}, true)

	if len(got.Filters) != 0 {
		t.Errorf("фильтр не очищен: %+v", got.Filters)
	}
	if got.PreferredDefault["tag"] != "/de/i" {
		t.Errorf("умолчание задето: %+v", got.PreferredDefault)
	}
}
