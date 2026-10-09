package stateedit

import (
	"fmt"
	"sort"
	"strings"

	"singbox-launcher/core/build"
	"singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// directOutTag — системный прямой выход шаблона лаунчера; известен всегда,
// даже когда шаблон не прочитался (как умолчание импорта бэкапа).
const directOutTag = "direct-out"

// KnownRuleTargets — корневые имена, на которые законно метят правило,
// detour DNS-сервера и которые заняты для нового корневого узла: занятые
// имена состояния (state.TakenRootTags), Направления после слияния с
// шаблоном и пресетами с их `-auto` и опциями `include`, системные теги
// шаблона (`direct-out`, тег блокировки, outbound'ы и endpoint'ы `config`).
// Тот же состав, что у KnownRuleTargetTags Конфигуратора. td == nil —
// шаблон не прочитался: остаются имена состояния и умолчания системных.
func KnownRuleTargets(st *state.State, td *template.TemplateData) map[string]bool {
	known := state.TakenRootTags(st)
	add := func(tag string) {
		if tag = strings.TrimSpace(tag); tag != "" {
			known[tag] = true
		}
	}
	for _, d := range build.ResolveDirections(st.Directions, td, build.TargetSpecFromState(st)) {
		add(d.Tag)
		if d.Auto != nil {
			add(d.AutoTag())
		}
		for _, o := range d.AddOutbounds {
			add(o)
		}
	}
	add(directOutTag)
	if td == nil {
		add(template.DefaultDirectionBlockTag)
		return known
	}
	add(td.DirectionBlockTag())
	for _, t := range td.SystemOutboundTags() {
		add(t)
	}
	return known
}

// AddRule ставит одно правило на ось и возвращает его номер.
//
// Проверки — те, что не пропускает форма правила Конфигуратора: запись
// разбирается DecodeBody; у inline/srs тело несёт `outbound` или `action`, и
// цель `outbound` — известное корневое имя (KnownRuleTargets); у preset `ref`
// — пресет шаблона, которого ещё нет среди правил и который не голова оси.
//
// Номер без значения: у пресета — его якорь из шаблона (так пресет встаёт и
// из библиотеки UI, и при загрузке — MarkRuleOrder), у inline/srs —
// NextUserRuleNum. Массив остаётся отсортированным по оси. После пресета
// DNS-записи и Направления пресетов синхронизируются, как при сохранении в
// Конфигураторе.
func AddRule(st *state.State, td *template.TemplateData, r state.Rule) (int, error) {
	if _, err := r.DecodeBody(); err != nil {
		return 0, fieldErr("rule", "%s", err.Error())
	}
	var specs map[string]state.RuleOrderSpec
	if td != nil {
		specs = template.RuleOrderSpecs(td.Presets)
	}
	switch r.Kind {
	case state.RuleKindInline, state.RuleKindSrs:
		body, err := r.BodyMap()
		if err != nil {
			return 0, fieldErr("body", "body: %s", err.Error())
		}
		out, _ := body["outbound"].(string)
		action, _ := body["action"].(string)
		if strings.TrimSpace(out) == "" && strings.TrimSpace(action) == "" {
			return 0, fieldErr("body", "body must carry outbound or action")
		}
		if out != "" {
			if known := KnownRuleTargets(st, td); !known[out] {
				return 0, fieldErr("body.outbound", "unknown outbound %q; known: %s", out, strings.Join(sortedSet(known), ", "))
			}
		}
	case state.RuleKindPreset:
		if td == nil {
			return 0, fieldErr("ref", "template is not available: preset %q cannot be checked", r.Ref)
		}
		spec, ok := specs[r.Ref]
		if !ok {
			return 0, fieldErr("ref", "unknown preset %q", r.Ref)
		}
		if !spec.Sortable {
			return 0, fieldErr("ref", "preset %q is a fixed head of the rule axis; the loader seeds it itself", r.Ref)
		}
		for i := range st.Rules {
			if st.Rules[i].Kind == state.RuleKindPreset && st.Rules[i].Ref == r.Ref {
				return 0, fieldErr("ref", "preset %q is already in rules (num %d)", r.Ref, ruleNum(st.Rules[i]))
			}
		}
		if r.Num == nil {
			n := spec.Num
			r.Num = &n
		}
	}
	if r.Num == nil {
		n := state.NextUserRuleNum(st.Rules)
		r.Num = &n
	}
	if *r.Num < state.MinSortableRuleNum {
		return 0, fieldErr("num", "num must be >= %d: lower positions belong to the fixed head of the axis", state.MinSortableRuleNum)
	}
	num := *r.Num
	st.Rules = state.SortRulesByNum(append(st.Rules, r))
	if r.Kind == state.RuleKindPreset {
		syncPresets(st, td)
	}
	return num, nil
}

// RuleSelector — какое правило удалить: ровно одно из полей.
type RuleSelector struct {
	Num  *int
	Name string
	Ref  string
}

// DeleteRule снимает ровно одно правило. Совпало несколько — *AmbiguousError
// с их номерами (удалять тогда по num); ни одного — ErrNotFound. Голову оси
// (несортируемый пресет) удалить нельзя: загрузчик досеет её снова.
func DeleteRule(st *state.State, td *template.TemplateData, sel RuleSelector) (state.Rule, error) {
	set := 0
	if sel.Num != nil {
		set++
	}
	if sel.Name != "" {
		set++
	}
	if sel.Ref != "" {
		set++
	}
	if set != 1 {
		return state.Rule{}, fieldErr("selector", "exactly one of num, name, ref is required")
	}
	var hits []int
	for i := range st.Rules {
		r := &st.Rules[i]
		switch {
		case sel.Num != nil && r.Num != nil && *r.Num == *sel.Num,
			sel.Name != "" && r.Kind != state.RuleKindPreset && r.Name == sel.Name,
			sel.Ref != "" && r.Kind == state.RuleKindPreset && r.Ref == sel.Ref:
			hits = append(hits, i)
		}
	}
	switch len(hits) {
	case 0:
		return state.Rule{}, fmt.Errorf("rule: %w", ErrNotFound)
	case 1:
	default:
		nums := make([]int, 0, len(hits))
		for _, i := range hits {
			nums = append(nums, ruleNum(st.Rules[i]))
		}
		return state.Rule{}, &AmbiguousError{Msg: fmt.Sprintf("%d rules match; delete by num", len(hits)), Nums: nums}
	}
	i := hits[0]
	r := st.Rules[i]
	if r.Kind == state.RuleKindPreset {
		head := ruleNum(r) < state.MinSortableRuleNum
		if td != nil {
			if spec, ok := template.RuleOrderSpecs(td.Presets)[r.Ref]; ok && !spec.Sortable {
				head = true
			}
		}
		if head {
			return state.Rule{}, fieldErr("selector", "preset %q is a fixed head of the rule axis; the loader seeds it back", r.Ref)
		}
	}
	st.Rules = append(st.Rules[:i], st.Rules[i+1:]...)
	if r.Kind == state.RuleKindPreset {
		syncPresets(st, td)
	}
	return r, nil
}

// syncPresets — то, что Конфигуратор делает при сохранении после смены
// набора пресетов: DNS-записи пресетов и Направления пресетов следуют за
// активными правилами. Без шаблона синхронизировать не с чем.
func syncPresets(st *state.State, td *template.TemplateData) {
	if td == nil {
		return
	}
	state.SyncDNSOptionsWithActivePresets(st.Rules, &st.DNS, template.PresetLiteMap(td.Presets))
	build.SyncOutboundsWithTemplate(st.Rules, &st.Directions, td.Presets, build.TemplateOutboundTags(td), build.TargetSpecFromState(st))
}

func ruleNum(r state.Rule) int {
	if r.Num == nil {
		return state.DefaultRuleNum
	}
	return *r.Num
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
