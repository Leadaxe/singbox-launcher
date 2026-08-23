// File default_in_members_test.go — `default` обязан входить в состав группы.
//
// Живой случай: шаблонная запись «vpn ②» объявляет `options.default =
// "proxy-out"` согласованно со своим `addOutbounds: [direct-out,
// proxy-out]`. Пользователь снял галку «proxy-out» — состав стал
// [direct-out], а `default` остался. Ядро отвергает ВЕСЬ конфиг:
//
//	start outbound/selector[vpn ②]: default outbound not found: proxy-out
package config

import (
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

func emitSelector(t *testing.T, d configtypes.Direction, nodes []*ParsedNode) string {
	t.Helper()
	info := map[string]*outboundInfo{}
	out, err := GenerateSelectorWithFilteredAddOutbounds(nodes, d, info, true, nil)
	if err != nil {
		t.Fatalf("эмиссия: %v", err)
	}
	return out
}

// Шаблонный `default` повис после того, как пользователь убрал его из
// состава: ключ снимается, группа остаётся рабочей.
func TestDefaultFromOptions_DroppedWhenNotInMembers(t *testing.T) {
	d := configtypes.Direction{
		Tag:          "vpn ②",
		Type:         "selector",
		AddOutbounds: []string{"direct-out"},
		Options:      map[string]interface{}{"default": "proxy-out"},
	}
	got := emitSelector(t, d, nil)
	if strings.Contains(got, `"default"`) {
		t.Fatalf("невалидный default уехал в конфиг — ядро не стартует:\n%s", got)
	}
	if !strings.Contains(got, `"direct-out"`) {
		t.Errorf("состав потерян: %s", got)
	}
}

// Валидный `default` из шаблона обязан сохраниться: снимать лишнее — не
// значит снимать всё.
func TestDefaultFromOptions_KeptWhenInMembers(t *testing.T) {
	d := configtypes.Direction{
		Tag:          "vpn ②",
		Type:         "selector",
		AddOutbounds: []string{"direct-out", "proxy-out"},
		Options:      map[string]interface{}{"default": "proxy-out"},
	}
	got := emitSelector(t, d, nil)
	if !strings.Contains(got, `"default":"proxy-out"`) {
		t.Errorf("валидный default снят: %s", got)
	}
}

// preferredDefault ищет среди отобранных узлов, а состав — это ещё и
// addOutbounds; проверка вхождения нужна и здесь.
func TestDefaultFromPreferred_MustBeInMembers(t *testing.T) {
	nodes := []*ParsedNode{{Tag: "🇩🇪 Frankfurt", Scheme: "socks"}}
	d := configtypes.Direction{
		Tag:              "vpn-1",
		Type:             "selector",
		Filters:          configtypes.SetDirectionFilterTag(nil, "Frankfurt", false),
		PreferredDefault: configtypes.SetDirectionFilterTag(nil, "Frankfurt", false),
	}
	got := emitSelector(t, d, nodes)
	if !strings.Contains(got, `"default":"🇩🇪 Frankfurt"`) {
		t.Errorf("умолчание по фильтру не выставлено: %s", got)
	}
}

// Ключ `default` не должен появиться дважды в одном объекте: один раз из
// вычисленного значения, второй — из options.
func TestDefault_NoDuplicateKey(t *testing.T) {
	nodes := []*ParsedNode{{Tag: "🇩🇪 Frankfurt", Scheme: "socks"}}
	d := configtypes.Direction{
		Tag:              "vpn-1",
		Type:             "selector",
		Filters:          configtypes.SetDirectionFilterTag(nil, "Frankfurt", false),
		PreferredDefault: configtypes.SetDirectionFilterTag(nil, "Frankfurt", false),
		AddOutbounds:     []string{"direct-out"},
		Options:          map[string]interface{}{"default": "direct-out"},
	}
	got := emitSelector(t, d, nodes)
	if n := strings.Count(got, `"default"`); n != 1 {
		t.Fatalf("ключ default встречается %d раз(а):\n%s", n, got)
	}
}
