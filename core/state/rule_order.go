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
//
// ЗАКОН ОСИ (SPEC 113-C): массив правил ВСЕГДА отсортирован по оси. Двойного
// порядка — файлового и осевого — не существует, и ни один потребитель не
// вправе читать позицию в слайсе как самостоятельную информацию: она только
// тай-брейк при равных номерах. Поэтому всякая перенумерация здесь
// заканчивается стабильной пересортировкой, а сохранение эмитит в порядке оси
// (ui/configurator/models/rule_order_axis.go ссылается на этот закон).
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
	// MinSortableRuleNum — минимальный номер, доступный сортируемому правилу.
	//
	// Единственная жёсткая граница оси (решение пользователя 28.08.2026):
	// системная голова стоит на 0 и обязана оставаться первой (D-050 — sniff
	// до матчинга роутинга), поэтому ниже 1 пользовательскому правилу нельзя.
	// Всё, что ВЫШЕ этой границы, — свободная территория: правило вправе встать
	// между шаблонными якорями (950/960) или над ними.
	MinSortableRuleNum = 1
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
//
// Разметка заканчивается пересортировкой на месте: закон оси не терпит
// состояния «номера розданы, а массив ещё в старом порядке» — потребитель,
// прочитавший такой слайс между разметкой и сортировкой, увидел бы другую
// раскладку.
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
	sortRulesByNumInPlace(rules)
	return changed
}

// sortRulesByNumInPlace — та же стабильная сортировка, что и SortRulesByNum, но
// без копии: применяется там, где массив уже наш и его надо привести к оси.
func sortRulesByNumInPlace(rules []Rule) {
	sort.SliceStable(rules, func(i, j int) bool {
		return ruleNum(rules[i]) < ruleNum(rules[j])
	})
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
// target == nil → правило уезжает в начало пользовательской зоны.
// Несортируемые не двигаются и не сдвигаются: их номера часть инварианта.
//
// M7 (SPEC 113-C §3, ПЕРЕСМОТРЕН решением пользователя 28.08.2026): клэмп
// оставлен ТОЛЬКО как защита от провала под системную голову (want <= 0).
// Прежняя редакция клэмпила want к UserRuleNumStart, объявляя зону 1..949
// несуществующей — и тем самым запрещала пользователю ставить правило между
// шаблонными якорями. Кейс-эталон: якоря «локальная сеть» (950) и «русские
// домены» (960), правило для 4pda обязано встать МЕЖДУ ними и там остаться
// после save→load. Клэмп выдавал ему 1000, и оно уезжало под оба якоря.
//
// Почему это безопасно для сборки: core/build/preset_merge.go делит правила на
// «голову» (num < UserRuleNumStart) и хвост, сохраняя относительный порядок
// внутри каждой группы. Якоря 950..990 и сами живут в голове, так что правило
// на 951 встаёт между ними ровно там, где его бросили. Единственное, что
// действительно нельзя — уйти на номер несортируемой головы (0) или ниже: там
// правило обогнало бы traffic-processing и sniff перестал бы быть первым.
func PlaceRuleAfter(rules []Rule, movedIdx int, targetIdx int, sortable func(Rule) bool) {
	want := UserRuleNumStart
	if targetIdx >= 0 && targetIdx < len(rules) {
		want = ruleNum(rules[targetIdx]) + 1
	}
	if want < MinSortableRuleNum {
		want = MinSortableRuleNum
	}
	placeRuleAt(rules, movedIdx, want, sortable)
}

// PlaceRuleBefore ставит правило moved НЕПОСРЕДСТВЕННО перед target: moved
// забирает номер target'а, а сам target вытесняется тем же ленивым сдвигом.
//
// Нужен ТОЛЬКО для вырожденного случая «брошено в самую первую строку, а списка
// выше нет вовсе»: в LxBox (§370, rule_order.dart) такой функции нет — там драг
// всегда считает `target.num + 1` от соседа СВЕРХУ. У нас соседа сверху в этой
// точке не существует, а раздать перетащенному UserRuleNumStart нельзя: над ним
// стоят сортируемые якоря 950..990, и правило уехало бы визуально вверх, а по
// маршрутизации вниз.
//
// Почему не «свободный номер под target'ом» (949 под якорем 950): это была бы
// СВОЯ семантика, а лаунчер зеркалит LxBox один в один. Там вставка всегда идёт
// сверху вниз с ленивым вытеснением, и правило, вставшее над якорем, законно
// сдвигает его на +1 — зазор шага 10 ровно для этого и оставлен, а каскад
// останавливается на первой дырке, так что дальше по оси ничего не едет.
//
// Практически эта ветка почти недостижима: при наличии системной головы
// MoveRuleSlot (гард `to < firstSortableSlotIndex()`) не пускает правило выше
// неё, и драг идёт обычным PlaceRuleAfter с головой в качестве target'а —
// то есть даёт num = 1 (см. пункт 2 нормы: дроп в самый верх = сосед сверху 0).
//
// КОНТРАКТ: target ОБЯЗАН быть сортируемым (sortable(rules[targetIdx]) == true);
// за проверку отвечает ВЫЗЫВАЮЩИЙ. Вытеснение внутри placeRuleAt собирает блок
// занятых номеров только из сортируемых, поэтому несортируемый target в блок не
// попадает и с места не уходит — moved получил бы номер, РАВНЫЙ его номеру, то
// есть ровно тот дубль на оси, ради устранения которого функция и написана
// (SPEC 113-C §4).
func PlaceRuleBefore(rules []Rule, movedIdx int, targetIdx int, sortable func(Rule) bool) {
	if targetIdx < 0 || targetIdx >= len(rules) {
		PlaceRuleAfter(rules, movedIdx, -1, sortable)
		return
	}
	placeRuleAt(rules, movedIdx, ruleNum(rules[targetIdx]), sortable)
}

// placeRuleAt — общее тело обоих Place*: занять номер want, вытеснив сплошной
// занятый блок от want вверх.
func placeRuleAt(rules []Rule, movedIdx int, want int, sortable func(Rule) bool) {
	if movedIdx < 0 || movedIdx >= len(rules) {
		return
	}
	if !sortable(rules[movedIdx]) {
		return
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
	// Блок конечен и сдвиг взаимно однозначен — значит равных номеров после
	// него не появляется, пока в зоне есть хоть одна дырка (SPEC 113-C §4).
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
