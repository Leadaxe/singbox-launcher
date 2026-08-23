// File explicit_patch_load_test.go — рубеж загрузки различает артефакт и
// осознанную очистку (SPEC 104 / корень проблемы с default).
package state

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

func userPatch(patch map[string]interface{}, explicit bool) configtypes.Direction {
	return configtypes.Direction{
		Tag: "proxy-out",
		Ref: configtypes.RefTemplate,
		Updates: []configtypes.OutboundUpdate{{
			Ref:      configtypes.RefUser,
			Patch:    patch,
			Explicit: explicit,
		}},
	}
}

// Артефакт pre-058 — тот самый, что выпускал российский трафик через
// российский узел. Флага у него нет: его писала старая версия.
func TestLoad_LegacyEmptyPatchDropped(t *testing.T) {
	obs := []configtypes.Direction{userPatch(map[string]interface{}{
		"addOutbounds": []interface{}{},
		"comment":      "",
		"filters":      map[string]interface{}{},
	}, false)}

	sanitizeOutboundRefs(&obs)

	if len(obs[0].Updates) != 0 {
		t.Fatalf("артефакт пережил загрузку и затрёт пресетный фильтр: %+v", obs[0].Updates)
	}
}

// Осознанная очистка выглядит так же, но помечена — снять её значило бы
// вернуть пользователю настройку, которую он убрал.
func TestLoad_ExplicitClearKept(t *testing.T) {
	obs := []configtypes.Direction{userPatch(map[string]interface{}{
		"preferredDefault": map[string]interface{}{},
	}, true)}

	sanitizeOutboundRefs(&obs)

	if len(obs[0].Updates) != 1 {
		t.Fatalf("осознанная очистка снята при загрузке: %+v", obs[0].Updates)
	}
	if _, ok := obs[0].Updates[0].Patch["preferredDefault"]; !ok {
		t.Errorf("содержимое патча потеряно: %+v", obs[0].Updates[0].Patch)
	}
}

// Патч с осмысленным значением не трогается независимо от флага.
func TestLoad_NonEmptyPatchAlwaysKept(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		obs := []configtypes.Direction{userPatch(map[string]interface{}{
			"label": "Моя Германия",
		}, explicit)}
		sanitizeOutboundRefs(&obs)
		if len(obs[0].Updates) != 1 {
			t.Errorf("explicit=%v: непустой патч снят: %+v", explicit, obs[0].Updates)
		}
	}
}
