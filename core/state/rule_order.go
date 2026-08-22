// File rule_order.go — разреженная ось порядка правил (SPEC 106, D-051/D-053).
//
// Порт модели LxBox (`lib/services/builder/rule_order.dart`, §370/§398).
// До SPEC 106 приоритет правила в лаунчере задавался позицией в слайсе
// `state.Rules[]` — этого недостаточно, когда часть правил обязана стоять на
// закреплённых местах (D-050: `traffic-processing` первым, иначе sniff не
// успевает извлечь домен до матчинга роутинга).
//
// Ось — сквозная нумерация всех правил:
//
//	0          голова (traffic-processing, несортируемый)
//	950..990   специфичные пресеты шаблона
//	1000..1100 зона пользовательских правил
//	1110..1150 широкие перехватчики
//
// Шаг 10 между шаблонными якорями оставлен НАМЕРЕННО: в зазор вписывается
// новый пресет, не переделывая раскладку и не трогая чужие номера.
package state

import "sort"

// Границы пользовательской зоны оси.
const (
	// UserRuleNumStart — начало зоны пользовательских правил.
	UserRuleNumStart = 1000
	// UserRuleNumEnd — конец зоны; сто слотов, раздвигать не нужно.
	UserRuleNumEnd = 1100
	// DefaultRuleNum — номер неразмеченного правила при сортировке.
	DefaultRuleNum = UserRuleNumStart
)

// RuleOrderSpec — то, что ось знает о правиле из шаблона: стартовый номер и
// признак сортируемости. Заполняется вызывающим из template.Preset, чтобы
// пакет state не зависел от пакета template.
type RuleOrderSpec struct {
	// Num — стартовая позиция на оси (`presets[].num`).
	Num int
	// Sortable — можно ли двигать правило пользователю. false у
	// неотчуждаемых пресетов: их номера часть инварианта.
	Sortable bool
	// DefaultEnabled — состояние при seed'е отсутствующего правила.
	DefaultEnabled bool
}

// MarkRuleOrder проставляет OrderNum правилам, у которых его ещё нет.
//
// Отдельного версионированного шага миграции НЕТ: state, записанный до
// SPEC 106, приезжает с nil и размечается при первой загрузке.
//
//   - preset-правило, чей ref есть в шаблоне → номер из шаблона;
//   - всё прочее (inline/srs + пресеты, которых в шаблоне уже нет) → подряд
//     от UserRuleNumStart в текущем порядке списка.
//
// Принятое следствие: правила из старого state садятся в начало
// пользовательской зоны и оказываются приоритетнее добавленных после
// обновления. Возвращает true, если что-то размечено — вызывающий сохраняет.
func MarkRuleOrder(rules []Rule, specs map[string]RuleOrderSpec) bool {
	changed := false
	nextUser := UserRuleNumStart
	for i := range rules {
		if rules[i].OrderNum != nil {
			continue
		}
		num := 0
		if rules[i].Kind == RuleKindPreset {
			if spec, ok := specs[rules[i].Ref]; ok {
				num = spec.Num
			} else {
				num = nextUser
				nextUser++
			}
		} else {
			num = nextUser
			nextUser++
		}
		n := num
		rules[i].OrderNum = &n
		changed = true
	}
	return changed
}

// SortRulesByNum сортирует правила по возрастанию OrderNum.
//
// Сортировка СТАБИЛЬНАЯ: при равных номерах сохраняется взаимный порядок —
// равенство возможно после исчерпания пользовательской зоны (NextUserRuleNum).
func SortRulesByNum(rules []Rule) []Rule {
	out := make([]Rule, len(rules))
	copy(out, rules)
	sort.SliceStable(out, func(i, j int) bool {
		return ruleNum(out[i]) < ruleNum(out[j])
	})
	return out
}

func ruleNum(r Rule) int {
	if r.OrderNum == nil {
		return DefaultRuleNum
	}
	return *r.OrderNum
}

// SeedRequiredRules добавляет отсутствующие несортируемые пресеты шаблона.
//
// Продуктовый инвариант D-050: неотчуждаемый пресет обязан присутствовать
// независимо от того, что лежит в state (чистая установка, восстановление из
// бэкапа, ручная правка через Debug API, апгрейд со старого state). Именно
// re-seed, а не флаг в файле, делает правило неотчуждаемым.
//
// Сортируемые пресеты здесь НЕ сидятся: их состав — выбор пользователя.
func SeedRequiredRules(rules []Rule, specs map[string]RuleOrderSpec) []Rule {
	present := make(map[string]bool, len(rules))
	for _, r := range rules {
		if r.Kind == RuleKindPreset {
			present[r.Ref] = true
		}
	}

	// Детерминированный порядок обхода: map в Go не упорядочен, а от порядка
	// зависит содержимое state.
	missing := make([]string, 0)
	for ref, spec := range specs {
		if !spec.Sortable && !present[ref] {
			missing = append(missing, ref)
		}
	}
	if len(missing) == 0 {
		return rules
	}
	sort.Strings(missing)

	out := make([]Rule, len(rules), len(rules)+len(missing))
	copy(out, rules)
	for _, ref := range missing {
		spec := specs[ref]
		num := spec.Num
		out = append(out, Rule{
			Kind:     RuleKindPreset,
			Ref:      ref,
			Enabled:  spec.DefaultEnabled,
			OrderNum: &num,
			Body:     []byte(`{"vars":{}}`),
		})
	}
	return out
}

// DedupePresetRules схлопывает повторы preset-правил: из группы с одинаковым
// Ref остаётся ПОСЛЕДНЕЕ в порядке списка.
//
// Зачем: SeedRequiredRules проверяет наличие по Ref, и на задвоенном списке он
// доволен — тогда удаление одной копии не даёт видимого эффекта («удалить
// невозможно»). Поэтому дедуп обязан идти ПЕРЕД seed'ом (D-053в).
func DedupePresetRules(rules []Rule) []Rule {
	lastIdx := make(map[string]int)
	presetCount := 0
	for i, r := range rules {
		if r.Kind != RuleKindPreset {
			continue
		}
		presetCount++
		lastIdx[r.Ref] = i
	}
	if len(lastIdx) == presetCount {
		return rules // повторов нет — список не пересобираем
	}
	out := make([]Rule, 0, len(rules))
	for i, r := range rules {
		if r.Kind == RuleKindPreset && lastIdx[r.Ref] != i {
			continue
		}
		out = append(out, r)
	}
	return out
}

// NormalizeRuleOrder — полный проход: дедуп → seed → разметка → сортировка.
// Идемпотентен: повторный вызов на нормализованном списке ничего не меняет.
// Значения переменных существующих пресетов сохраняются (seed срабатывает,
// только если правила нет вовсе).
func NormalizeRuleOrder(rules []Rule, specs map[string]RuleOrderSpec) []Rule {
	out := DedupePresetRules(rules)
	out = SeedRequiredRules(out, specs)
	MarkRuleOrder(out, specs)
	return SortRulesByNum(out)
}

// NextUserRuleNum возвращает номер для нового пользовательского правила —
// конец занятой части зоны [UserRuleNumStart, UserRuleNumEnd].
//
// Зона исчерпана (максимум уже на границе) → возвращается та же граница:
// равенство номеров допустимо, порядок доопределяется позицией в списке.
func NextUserRuleNum(rules []Rule) int {
	maxInZone := UserRuleNumStart - 1
	for _, r := range rules {
		if r.OrderNum == nil {
			continue
		}
		n := *r.OrderNum
		if n < UserRuleNumStart || n > UserRuleNumEnd {
			continue
		}
		if n > maxInZone {
			maxInZone = n
		}
	}
	next := maxInZone + 1
	if next > UserRuleNumEnd {
		return UserRuleNumEnd
	}
	return next
}

// PlaceRuleAfter ставит правило moved сразу за правилом target («ленивый сдвиг»).
//
//	want = target.num + 1
//	want свободен → moved.num = want            (соседи не трогаются)
//	want занят    → сдвигаем СПЛОШНОЙ занятый блок от want вверх на +1,
//	                останавливаясь на ПЕРВОЙ дырке; moved.num = want
//
// Почему сдвиг ленивый, а не безусловный (D-053а — не «упрощать»): каскад +1
// на каждом перетаскивании съедал бы зазоры и двигал шаблонные якоря — тогда
// номер из шаблона перестал бы что-либо гарантировать, и вписать новый пресет
// между двумя соседями стало бы невозможно. Каскад обязан останавливаться на
// первой дырке: сдвигать всех с num >= want недостаточно лениво — правило,
// стоящее вплотную, вытесняется законно, а якорь за сотней свободных номеров
// вытеснять некуда, но он всё равно уезжал бы на +1.
//
// target == nil → правило уезжает в начало сортируемой части.
// Несортируемые не двигаются и не сдвигаются: их номера часть инварианта.
func PlaceRuleAfter(rules []Rule, movedIdx int, targetIdx int, sortable func(Rule) bool) {
	if movedIdx < 0 || movedIdx >= len(rules) {
		return
	}
	if !sortable(rules[movedIdx]) {
		return
	}

	want := UserRuleNumStart
	if targetIdx >= 0 && targetIdx < len(rules) {
		want = ruleNum(rules[targetIdx]) + 1
	}

	// Занятые номера от want вверх — без самого moved и без несортируемых
	// (их номера часть инварианта, вытеснять их нельзя).
	occupied := make(map[int][]int)
	for i := range rules {
		if i == movedIdx || !sortable(rules[i]) {
			continue
		}
		if rules[i].OrderNum == nil {
			continue
		}
		if n := *rules[i].OrderNum; n >= want {
			occupied[n] = append(occupied[n], i)
		}
	}

	// Сплошной занятый блок от want: 1001,1002,1003 — сдвигаем; на первой
	// дырке останавливаемся, всё что за ней (включая якоря) не трогаем.
	var block []int
	for n := want; ; n++ {
		idxs, ok := occupied[n]
		if !ok {
			break
		}
		block = append(block, idxs...)
	}
	// Сдвигаем сверху вниз — иначе +1 наложился бы на ещё не сдвинутого соседа.
	for i := len(block) - 1; i >= 0; i-- {
		idx := block[i]
		n := *rules[idx].OrderNum + 1
		rules[idx].OrderNum = &n
	}

	w := want
	rules[movedIdx].OrderNum = &w
}
