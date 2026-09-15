package build

// Лечение оси, сдвинутой импортом бэкапа выпущенных 1.5.3–1.5.6.
//
// Тот импорт перенумеровывал ось сплошь с 1000 (renumberImportedRules): голова
// traffic-processing (номер шаблона 0) и якоря ниже 1000 уезжали в
// пользовательскую зону. Импорт починен, а состояния, которые он сдвинул,
// остались у пользователей.

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// Раскладка — как после такого импорта: голова 1000, якорь 1001, свои правила
// 1002–1003, перехватчик 1004, и пресет, включённый уже ПОСЛЕ импорта со своим
// номером шаблона 980. У шаблона свои route.rules.
//
// Норма: несортируемый пресет встаёт на номер шаблона и при сборке, и при
// загрузке (NormalizeRuleOrder), sniff — первое правило конфига; номера и
// взаимный порядок всех остальных правил не меняются — сортируемый пресет,
// уехавший в 1000+, не трогается (тот же номер ставит перетаскивание);
// лечение идемпотентно и переживает Save → Load.
func TestShiftedAxisHeadPinnedToTemplate(t *testing.T) {
	num := func(v int) *int { return &v }
	notSortable := false
	presets := []template.Preset{
		{ID: "traffic-processing", Num: num(0), Sortable: &notSortable, DefaultEnabled: true,
			Rules: []map[string]interface{}{{"inbound": "tun-in", "action": "sniff"}}},
		{ID: "private-ips", Num: num(950),
			Rules: []map[string]interface{}{{"ip_is_private": true, "outbound": "direct-out"}}},
		{ID: "stop-http3", Num: num(980),
			Rules: []map[string]interface{}{{"network": []interface{}{"udp"}, "port": []interface{}{443}, "outbound": "drop"}}},
		{ID: "russian", Num: num(1120),
			Rules: []map[string]interface{}{{"domain_suffix": ".ru", "outbound": "direct-out"}}},
	}
	specs := template.RuleOrderSpecs(presets)

	preset := func(ref string, n int) state.Rule {
		r := state.NewPresetRule(ref, nil)
		r.Enabled = true
		r.Num = num(n)
		return r
	}
	inline := func(name string, n int) state.Rule {
		r := state.NewInlineRule(name,
			map[string]interface{}{"domain_suffix": []interface{}{name + ".example"}}, "direct-out")
		r.Enabled = true
		r.Num = num(n)
		return r
	}
	// Каждый проход получает свою копию: нормализация сортирует и размечает
	// переданный срез на месте.
	shifted := func() []state.Rule {
		return []state.Rule{
			preset("stop-http3", 980),
			preset("traffic-processing", 1000),
			preset("private-ips", 1001),
			inline("mine-a", 1002),
			inline("mine-b", 1003),
			preset("russian", 1004),
		}
	}

	axis := func(rules []state.Rule) []string {
		out := make([]string, 0, len(rules))
		for _, r := range rules {
			id := r.Name
			if r.Kind == state.RuleKindPreset {
				id = r.Ref
			}
			n := "-"
			if r.Num != nil {
				n = fmt.Sprint(*r.Num)
			}
			out = append(out, id+"@"+n)
		}
		return out
	}
	wantAxis := []string{
		"traffic-processing@0", "stop-http3@980", "private-ips@1001",
		"mine-a@1002", "mine-b@1003", "russian@1004",
	}

	// 1) Загрузка: голова на номере шаблона, остальные — где были.
	healed := state.NormalizeRuleOrder(shifted(), specs)
	if got := axis(healed); strings.Join(got, " ") != strings.Join(wantAxis, " ") {
		t.Fatalf("ось после нормализации %v, ожидалась %v", got, wantAxis)
	}
	if again := state.NormalizeRuleOrder(healed, specs); strings.Join(axis(again), " ") != strings.Join(wantAxis, " ") {
		t.Errorf("повторная нормализация сдвинула ось: %v", axis(again))
	}

	// 2) Save → Load → нормализация: вылеченная ось на диске не дрейфует.
	path := filepath.Join(t.TempDir(), "wizard_states", "state.json")
	st := state.New()
	st.Rules = healed
	if err := st.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := state.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := axis(state.NormalizeRuleOrder(loaded.Rules, specs)); strings.Join(got, " ") != strings.Join(wantAxis, " ") {
		t.Errorf("ось после Save → Load %v, ожидалась %v", got, wantAxis)
	}

	// 3) Сборка из СДВИНУТОГО состояния (файл ещё не пересохранён): sniff —
	// первое правило конфига, перед route.rules шаблона и перед пресетом,
	// включённым после импорта.
	route, err := MergePresetsIntoRoute(
		json.RawMessage(`{"rules":[{"domain_suffix":["template.example"],"outbound":"direct-out"}]}`),
		PresetMergeContext{Presets: presets, Rules: shifted()})
	if err != nil {
		t.Fatalf("MergePresetsIntoRoute: %v", err)
	}
	var parsed struct {
		Rules []map[string]interface{} `json:"rules"`
	}
	if err := json.Unmarshal(route, &parsed); err != nil {
		t.Fatalf("route: %v", err)
	}
	var got []string
	for _, r := range parsed.Rules {
		raw, _ := json.Marshal(r)
		s := string(raw)
		switch {
		case r["action"] == "sniff":
			got = append(got, "traffic-processing")
		case strings.Contains(s, `"network"`):
			got = append(got, "stop-http3")
		case r["ip_is_private"] == true:
			got = append(got, "private-ips")
		case strings.Contains(s, "template.example"):
			got = append(got, "template")
		case strings.Contains(s, `".ru"`):
			got = append(got, "russian")
		case strings.Contains(s, "mine-a"):
			got = append(got, "mine-a")
		case strings.Contains(s, "mine-b"):
			got = append(got, "mine-b")
		default:
			got = append(got, s)
		}
	}
	want := []string{"traffic-processing", "stop-http3", "template", "private-ips", "mine-a", "mine-b", "russian"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("route.rules %v, ожидалось %v", got, want)
	}
}
