package build

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/template"
)

// Слоёный пирог «шаблон → пресет → пользователь» обязан пропускать новые
// поля Направления. Поле, не проведённое через все слои, молча теряется при
// сохранении шаблонной записи — на этом ловились предыдущие правки
// (см. SPEC 104 §11 T2).
func TestUserPatchCarriesDirectionFields(t *testing.T) {
	base := configtypes.Direction{Tag: "vpn-1", Type: "selector"}
	interrupt := true
	form := base
	form.Label = "Моя Германия"
	form.Disabled = true
	form.Auto = &configtypes.DirectionAuto{
		Mode:                      configtypes.AutoModeRoundRobin,
		Pool:                      3,
		InterruptExistConnections: &interrupt,
	}

	patch := OutboundFieldDiff(form, base)
	if patch == nil {
		t.Fatal("diff пуст: правка имени/двойника не дойдёт до state")
	}
	if patch["label"] != "Моя Германия" {
		t.Fatalf("label не в патче: %+v", patch)
	}
	if patch["disabled"] != true {
		t.Fatalf("disabled не в патче: %+v", patch)
	}
	if patch["auto"] == nil {
		t.Fatalf("auto не в патче: %+v", patch)
	}

	// Патч живёт в state как map (JSON round-trip) — проверяем именно так.
	raw, err := json.Marshal(patch)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var stored map[string]interface{}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	got := applyOutboundUpdatePatch(base, stored, true)
	if got.Label != "Моя Германия" {
		t.Fatalf("имя не применилось: %+v", got)
	}
	if !got.Disabled {
		t.Fatalf("выключение не применилось: %+v", got)
	}
	if got.Auto == nil || got.Auto.Mode != configtypes.AutoModeRoundRobin || got.Auto.Pool != 3 {
		t.Fatalf("двойник не применился: %+v", got.Auto)
	}
	if got.Auto.InterruptExistConnections == nil || !*got.Auto.InterruptExistConnections {
		t.Fatalf("трёхзначный interrupt потерян: %+v", got.Auto)
	}
}

// Обратный ход: пользователь снял имя и выключил двойник у направления,
// которое пришло из шаблона с именем. Нулевые значения обязаны записаться
// явно, иначе форма не сможет ничего отменить.
func TestUserPatchCanClearNameAndTwin(t *testing.T) {
	base := configtypes.Direction{
		Tag:   "vpn-1",
		Label: "VPN ①",
		Auto:  &configtypes.DirectionAuto{URL: "http://example.com"},
	}
	form := base
	form.Label = ""
	form.Auto = nil

	patch := OutboundFieldDiff(form, base)
	if patch == nil {
		t.Fatal("очистка должна давать непустой diff")
	}
	if v, ok := patch["label"]; !ok || v != "" {
		t.Fatalf("пустое имя должно писаться явно: %+v", patch)
	}
	if v, ok := patch["auto"]; !ok || v != nil {
		t.Fatalf("снятый двойник должен писаться явным null: %+v", patch)
	}

	raw, _ := json.Marshal(patch)
	var stored map[string]interface{}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := applyOutboundUpdatePatch(base, stored, true)
	if got.Label != "" {
		t.Fatalf("имя не снялось: %q", got.Label)
	}
	if got.Auto != nil {
		t.Fatalf("двойник не снялся: %+v", got.Auto)
	}
}

// Направление, выключенное пресетом, пользователь должен уметь включить
// обратно: `disabled:false` в патче — это значение, а не «ключа нет».
func TestUserPatchCanReEnableDirection(t *testing.T) {
	base := configtypes.Direction{Tag: "vpn-2", Disabled: true}
	got := applyOutboundUpdatePatch(base, map[string]interface{}{"disabled": false}, true)
	if got.Disabled {
		t.Fatalf("направление осталось выключенным: %+v", got)
	}
}

// Пресет с mode=update переименовывает и настраивает двойник; пустое имя у
// пресета означает «не переименовываю» и не должно стирать пользовательское.
func TestPresetUpdateRenamesButEmptyNameKeepsUserLabel(t *testing.T) {
	target := configtypes.Direction{Tag: "vpn-1", Label: "Моё имя"}

	renamed := applyOutboundUpdate(target, configtypes.Direction{Label: "ru VPN 🇷🇺"})
	if renamed.Label != "ru VPN 🇷🇺" {
		t.Fatalf("пресет не переименовал: %q", renamed.Label)
	}

	untouched := applyOutboundUpdate(target, configtypes.Direction{Filters: map[string]interface{}{"tag": "/x/i"}})
	if untouched.Label != "Моё имя" {
		t.Fatalf("пресет без label стёр имя пользователя: %q", untouched.Label)
	}
}

// Пресет умеет создать направление с именем и двойником (mode=add,
// решение D-10) — поля должны доехать через JSON-разворачивание.
func TestPresetAddCarriesDirectionFields(t *testing.T) {
	disabled := false
	preset := template.Preset{
		ID: "russian",
		Outbounds: []template.PresetOutbound{{
			Mode:     "add",
			Tag:      "ru VPN 🇷🇺",
			Type:     "selector",
			Label:    "ru VPN 🇷🇺",
			Disabled: &disabled,
			Auto: &configtypes.DirectionAuto{
				Mode: configtypes.AutoModeLeastTest,
				URL:  "http://cp.cloudflare.com/generate_204",
			},
		}},
	}
	entries, warns := ExpandPresetOutbounds(&preset, nil, template.LocalTarget())
	if len(warns) > 0 {
		t.Fatalf("неожиданные предупреждения: %+v", warns)
	}
	if len(entries) != 1 {
		t.Fatalf("ожидалась одна запись, got %d", len(entries))
	}
	got := entries[0].Config
	if got.Label != "ru VPN 🇷🇺" {
		t.Fatalf("имя не доехало: %+v", got)
	}
	if got.Auto == nil || got.Auto.URL == "" {
		t.Fatalf("двойник не доехал: %+v", got.Auto)
	}
	if got.Disabled {
		t.Fatalf("disabled=false из пресета прочитан как true: %+v", got)
	}
}

// Тело referenced-записи живёт в шаблоне/пресете: имя и двойник обязаны
// зачищаться вместе с остальным телом, иначе старое имя навсегда перебивало
// бы обновление шаблона.
func TestStripReferencedBodyClearsNameAndTwin(t *testing.T) {
	ob := &configtypes.Direction{
		Tag:   "proxy-out",
		Ref:   configtypes.RefTemplate,
		Label: "старое имя",
		Auto:  &configtypes.DirectionAuto{URL: "http://example.com"},
	}
	stripReferencedBody(ob)
	if ob.Label != "" || ob.Auto != nil {
		t.Fatalf("тело не зачищено: %+v", ob)
	}
	if ob.Tag != "proxy-out" || ob.Ref != configtypes.RefTemplate {
		t.Fatalf("идентичность записи потеряна: %+v", ob)
	}

	// Direct-запись (ref="") не трогаем — её тело и есть источник истины.
	direct := &configtypes.Direction{Tag: "vpn-1", Label: "Моё"}
	stripReferencedBody(direct)
	if direct.Label != "Моё" {
		t.Fatalf("direct-запись обеднена: %+v", direct)
	}
}
