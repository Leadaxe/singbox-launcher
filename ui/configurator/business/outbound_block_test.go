package business

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/config/configtypes"
	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
)

// Цели, объявленные в шаблоне, обязаны попадать в выпадающий список правила.
//
// До исправления список собирался только из подписок и пресетов, и `block-out`
// — единственная цель «заблокировать соединение», которую объявляет шаблон, —
// в него не попадала: пользователь не мог выбрать блокировку, хотя outbound
// в конфиг уезжал и работал.
//
// Заготовки Направлений (`parser_config.outbounds[]`) целью становятся только
// записью состояния: удалённая пользователем `vpn ①` в конфиг не идёт, и
// выбор её в detour или правиле указал бы в пустоту.
func TestAvailableOutboundsIncludeTemplateGlobals(t *testing.T) {
	model := &wizardmodels.WizardModel{
		TemplateData: &wizardtemplate.TemplateData{
			ParserConfig: `{"ParserConfig":{"outbounds":[
				{"type":"selector","tag":"proxy-out"},
				{"type":"selector","tag":"vpn ①"}
			]}}`,
			Config: map[string]json.RawMessage{
				"outbounds": json.RawMessage(`[
					{"type":"direct","tag":"direct-out"},
					{"type":"block","tag":"block-out"}
				]`),
			},
		},
		GlobalOutbounds: []configtypes.Direction{{Tag: "proxy-out", Type: "selector"}},
	}

	got := GetAvailableOutbounds(model)

	if !containsTag(got, "block-out") {
		t.Errorf("в списке нет block-out — блокировку выбрать нечем: %v", got)
	}
	if !containsTag(got, "direct-out") {
		t.Errorf("в списке нет direct-out: %v", got)
	}
	if !containsTag(got, "proxy-out") {
		t.Errorf("пропало живое Направление proxy-out: %v", got)
	}
	if containsTag(got, "vpn ①") {
		t.Errorf("удалённая заготовка шаблона vpn ① предложена целью: %v", got)
	}
	// Псевдо-цели остаются: они не outbound'ы, а действия правила.
	for _, want := range []string{"reject", "drop"} {
		if !containsTag(got, want) {
			t.Errorf("пропала псевдо-цель %q: %v", want, got)
		}
	}
}

// Шаблон без outbound'ов не должен ронять сборку списка.
func TestAvailableOutboundsWithoutTemplate(t *testing.T) {
	got := GetAvailableOutbounds(&wizardmodels.WizardModel{})
	for _, want := range []string{"direct-out", "reject", "drop"} {
		if !containsTag(got, want) {
			t.Errorf("без шаблона пропало %q: %v", want, got)
		}
	}
}

// Дубль тега шаблона и умолчания не должен размножать запись.
func TestAvailableOutboundsNoDuplicates(t *testing.T) {
	model := &wizardmodels.WizardModel{
		TemplateData: &wizardtemplate.TemplateData{
			ParserConfig: `{"ParserConfig":{"outbounds":[{"type":"direct","tag":"direct-out"}]}}`,
		},
	}
	got := GetAvailableOutbounds(model)
	seen := map[string]int{}
	for _, tag := range got {
		seen[tag]++
	}
	if seen["direct-out"] != 1 {
		t.Errorf("direct-out встречается %d раз: %v", seen["direct-out"], got)
	}
}

func containsTag(list []string, tag string) bool {
	for _, t := range list {
		if t == tag {
			return true
		}
	}
	return false
}
