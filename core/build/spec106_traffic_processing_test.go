package build

// Проверка переноса базовых правил в неотчуждаемый пресет (SPEC 106, D-050).
//
// Смысл теста: до переноса sniff/resolve собирались из params(prepend), а
// hijack-dns был зашит в config.route.rules — пользователь их не видел. После
// переноса все три живут в пресете traffic-processing. Собранный конфиг обязан
// остаться ЭКВИВАЛЕНТНЫМ: те же правила, в том же порядке, с теми же полями —
// меняется место объявления, а не поведение сети.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// loadRealTemplate читает боевой шаблон из bin/ — тест намеренно смотрит на
// него, а не на фикстуру: перенос правил менял именно этот файл.
func loadRealTemplate(t *testing.T) *template.TemplateData {
	t.Helper()
	path := filepath.Join("..", "..", "bin", "wizard_template.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("боевой шаблон недоступен: %v", err)
	}
	var td template.TemplateData
	if err := json.Unmarshal(raw, &td); err != nil {
		t.Fatalf("разбор шаблона: %v", err)
	}
	return &td
}

func findPreset(td *template.TemplateData, id string) *template.Preset {
	for i := range td.Presets {
		if td.Presets[i].ID == id {
			return &td.Presets[i]
		}
	}
	return nil
}

func TestTrafficProcessingPresetExists(t *testing.T) {
	td := loadRealTemplate(t)
	p := findPreset(td, "traffic-processing")
	if p == nil {
		t.Fatal("пресет traffic-processing отсутствует в шаблоне")
	}

	if p.IsSortable() {
		t.Error("пресет обязан быть несортируемым: его позиция часть инварианта")
	}
	if !p.Locked {
		t.Error("пресет обязан быть locked: пользователь не должен его выключать")
	}
	if p.OrderNum() != 0 {
		t.Errorf("номер на оси %d, ожидался 0 — sniff обязан быть первым правилом", p.OrderNum())
	}
	if !p.DefaultEnabled {
		t.Error("пресет обязан быть включён по умолчанию")
	}
	if len(p.Rules) != 3 {
		t.Fatalf("правил в пресете %d, ожидалось 3 (sniff, hijack-dns, resolve)", len(p.Rules))
	}
}

// TestBaseRulesNotDeclaredTwice — правила не должны остаться в прежних местах:
// иначе они попадут в конфиг дважды.
func TestBaseRulesNotDeclaredTwice(t *testing.T) {
	td := loadRealTemplate(t)

	var route map[string]interface{}
	if raw, ok := td.Config["route"]; ok {
		if err := json.Unmarshal(raw, &route); err != nil {
			t.Fatalf("разбор route: %v", err)
		}
	}
	if rules, ok := route["rules"].([]interface{}); ok && len(rules) > 0 {
		t.Errorf("config.route.rules непуст (%d правил) — базовые правила переехали в пресет", len(rules))
	}

	for _, p := range td.Params {
		if p.Name == "route.rules" {
			t.Error("param route.rules всё ещё объявлен — sniff/resolve задвоятся с пресетом")
		}
	}
}

// TestTrafficProcessingIsReseeded — неотчуждаемость держится re-seed'ом, а не
// флагом в состоянии: стёртый из state пресет обязан вернуться на сборке.
func TestTrafficProcessingIsReseeded(t *testing.T) {
	td := loadRealTemplate(t)
	specs := template.RuleOrderSpecs(td.Presets)

	// Состояние, где пресета нет вовсе — имитирует ручную правку state
	// или восстановление из бэкапа старой версии.
	rules := []corestate.Rule{
		{Kind: corestate.RuleKindInline, Enabled: true, Body: []byte(`{}`)},
	}
	out := corestate.NormalizeRuleOrder(rules, specs)

	var found bool
	for _, r := range out {
		if r.Kind == corestate.RuleKindPreset && r.Ref == "traffic-processing" {
			found = true
			if r.OrderNum == nil || *r.OrderNum != 0 {
				t.Errorf("пере-засеян с номером %v, ожидался 0", r.OrderNum)
			}
			if !r.Enabled {
				t.Error("пере-засеян выключенным")
			}
		}
	}
	if !found {
		t.Fatal("пресет не вернулся в список правил — неотчуждаемость не работает")
	}
	if out[0].Ref != "traffic-processing" {
		t.Errorf("первым идёт %q, а sniff обязан отработать до матчинга", out[0].Ref)
	}
}

// TestAxisAnchorsAreUnique — два пресета на одном номере делают порядок
// зависящим от порядка в файле, что ось как раз и должна исключать.
func TestAxisAnchorsAreUnique(t *testing.T) {
	td := loadRealTemplate(t)
	seen := make(map[int]string)
	for i := range td.Presets {
		p := &td.Presets[i]
		if p.Num == nil {
			continue // неразмеченные садятся в пользовательскую зону — это норма
		}
		if prev, dup := seen[*p.Num]; dup {
			t.Errorf("номер %d занят дважды: %q и %q", *p.Num, prev, p.ID)
		}
		seen[*p.Num] = p.ID
	}
}

// TestUserZoneIsFree — шаблонные якоря не должны залезать в зону
// пользовательских правил, иначе вставка правила вытесняет якорь.
func TestUserZoneIsFree(t *testing.T) {
	td := loadRealTemplate(t)
	for i := range td.Presets {
		p := &td.Presets[i]
		if p.Num == nil {
			continue
		}
		if *p.Num >= corestate.UserRuleNumStart && *p.Num <= corestate.UserRuleNumEnd {
			t.Errorf("пресет %q занял номер %d внутри пользовательской зоны [%d..%d]",
				p.ID, *p.Num, corestate.UserRuleNumStart, corestate.UserRuleNumEnd)
		}
	}
}
