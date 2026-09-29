package state

import "testing"

// LxBox §578: поздний дефолтный пресет засевается один раз; отмеченный не
// возвращается; id, которого нет в шаблоне, не отмечается.
func TestSeedLateDefaultRules(t *testing.T) {
	specs := map[string]RuleOrderSpec{"tailscale": {Num: 945, Sortable: true, DefaultEnabled: true}}

	rules, marks := SeedLateDefaultRules(nil, specs, nil)
	if len(rules) != 1 || rules[0].Ref != "tailscale" || !rules[0].Enabled || *rules[0].Num != 945 {
		t.Fatalf("засев: %+v", rules)
	}
	if len(marks) != 1 || marks[0] != "tailscale" {
		t.Fatalf("отметки: %v", marks)
	}
	// Пользователь удалил пресет, отметка сохранена — не возвращается.
	if rules, _ := SeedLateDefaultRules(nil, specs, marks); len(rules) != 0 {
		t.Fatalf("удалённый пресет вернулся: %+v", rules)
	}
	// Правило уже есть (выключено пользователем) — не дублируется и не включается.
	off := NewPresetRule("tailscale", nil)
	if rules, marks := SeedLateDefaultRules([]Rule{off}, specs, nil); len(rules) != 1 || rules[0].Enabled || len(marks) != 1 {
		t.Fatalf("существующее правило: %+v %v", rules, marks)
	}
	// Шаблон без пресета — ничего не отмечено.
	if _, marks := SeedLateDefaultRules(nil, map[string]RuleOrderSpec{}, nil); len(marks) != 0 {
		t.Fatalf("отмечен отсутствующий в шаблоне id: %v", marks)
	}
}
