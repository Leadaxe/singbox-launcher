package backup

import (
	"encoding/json"
	"testing"

	"singbox-launcher/core/state"
)

// SPEC 113-C §1: перенумерация оси заканчивается пересортировкой массива.
// renumberImportedRules раздавал номера по возрастанию, но оставлял записи в
// том порядке, в каком они лежали в бэкапе — файл получался с номерами,
// противоречащими порядку записей, и всякий, кто читал позицию в слайсе,
// видел не то, что скажет маршрутизация.
func TestImportLeavesRulesSortedByAxis(t *testing.T) {
	n := func(v int) *float64 { f := float64(v); return &f }
	b := &Backup{LxBackup: FormatVersion, Rules: []Rule{
		{Kind: RuleInline, Name: "third", Num: n(9000), Match: json.RawMessage(`{}`)},
		{Kind: RuleInline, Name: "first", Num: n(10), Match: json.RawMessage(`{}`)},
		{Kind: RuleInline, Name: "second", Num: n(500), Match: json.RawMessage(`{}`)},
	}}
	dst := &state.State{}
	if _, err := Import(dst, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	got := make([]string, 0, len(dst.Rules))
	prev := -1 << 31
	for i, r := range dst.Rules {
		var body state.InlineBody
		if err := json.Unmarshal(r.Body, &body); err != nil {
			t.Fatalf("тело правила %d: %v", i, err)
		}
		got = append(got, body.Name)
		if r.OrderNum == nil {
			t.Fatalf("правило %q приехало без номера", body.Name)
		}
		if *r.OrderNum < prev {
			t.Fatalf("rules[%d] несёт номер %d после %d — массив не отсортирован по оси",
				i, *r.OrderNum, prev)
		}
		prev = *r.OrderNum
	}

	want := []string{"first", "second", "third"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("порядок записей после импорта = %v, ожидалось %v", got, want)
		}
	}
}

// Бэкап без num: неразмеченные уезжают в хвост, сохраняя взаимный порядок —
// разметку им раздаст MarkRuleOrder на первой загрузке, и она пойдёт от конца
// занятой части, а не поверх уже перенумерованных.
func TestImportPutsUnnumberedRulesLast(t *testing.T) {
	n := func(v int) *float64 { f := float64(v); return &f }
	b := &Backup{LxBackup: FormatVersion, Rules: []Rule{
		{Kind: RuleInline, Name: "no-num-a", Match: json.RawMessage(`{}`)},
		{Kind: RuleInline, Name: "numbered", Num: n(42), Match: json.RawMessage(`{}`)},
		{Kind: RuleInline, Name: "no-num-b", Match: json.RawMessage(`{}`)},
	}}
	dst := &state.State{}
	if _, err := Import(dst, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}

	got := make([]string, 0, len(dst.Rules))
	for _, r := range dst.Rules {
		var body state.InlineBody
		_ = json.Unmarshal(r.Body, &body)
		got = append(got, body.Name)
	}
	want := []string{"numbered", "no-num-a", "no-num-b"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("порядок после импорта = %v, ожидалось %v", got, want)
		}
	}
}
