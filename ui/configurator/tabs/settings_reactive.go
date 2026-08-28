package tabs

// Реактивный пересчёт гейтов вкладки Settings (SPEC 107 §8, D-066).
//
// Зачем: раньше изменение любой переменной пересобирало вкладку целиком
// (box.RemoveAll + заново все строки). На каждый клик по галке — полная
// перерисовка: лишняя работа Fyne, мигание, риск потери фокуса и позиции
// прокрутки.
//
// Теперь узел знает, от чего зависит: гейт каждой строки компилируется в
// множество имён (template.CondDeps), из них строится инвертированный индекс
// `имя → строки`. При изменении переменной пересчитываются ТОЛЬКО подписанные
// строки, и только те из них, у кого результат гейта реально изменился,
// обновляют свой виджет.
//
// Зависимости извлекаются СТАТИЧЕСКИ — язык условий декларативен и полностью
// анализируем. Динамический трекинг («что прочитали при вычислении») тут был
// бы не только лишним, но и неверным: он пропустил бы ветки, которые
// short-circuit не вычислил, а завтра они решают исход.

import (
	wizardtemplate "singbox-launcher/core/template"
	wizardmodels "singbox-launcher/ui/configurator/models"
	wizardpresentation "singbox-launcher/ui/configurator/presentation"
)

// gateSubscriber — строка Settings, состояние которой зависит от гейта.
type gateSubscriber struct {
	// varName — переменная, чью строку представляет подписчик.
	varName string
	// apply включает/выключает виджеты строки. Вызывается только когда
	// значение гейта изменилось.
	apply func(enabled bool)
	// last — последнее применённое значение гейта (кэш): без него каждый
	// батч дёргал бы Enable/Disable вхолостую.
	last bool
	// recalc — своя формула состояния вместо гейта переменной. Непусто только
	// у подписок subscribeOn, где условие шире шаблонного гейта (поведение
	// ядра). nil = считать по varName, как обычно.
	recalc func(resolved map[string]wizardtemplate.ResolvedVar) bool
}

// gateIndex — инвертированный индекс `имя переменной → подписчики`.
//
// Производная величина: строится при сборке вкладки, не сериализуется и в
// state не попадает. Пересобирается при каждой пересборке вкладки.
type gateIndex struct {
	byDep map[string][]*gateSubscriber
	all   []*gateSubscriber
}

func newGateIndex() *gateIndex {
	return &gateIndex{byDep: make(map[string][]*gateSubscriber)}
}

// subscribe регистрирует строку переменной vd с её текущим состоянием.
//
// Строка без гейта не подписывается ни на что: её состояние не меняется от
// значений других переменных.
func (idx *gateIndex) subscribe(vd wizardtemplate.TemplateVar, enabled bool, apply func(bool)) {
	if idx == nil || apply == nil {
		return
	}
	deps := vd.GateDeps()
	if len(deps) == 0 {
		return
	}
	sub := &gateSubscriber{varName: vd.Name, apply: apply, last: enabled}
	idx.all = append(idx.all, sub)
	for _, dep := range deps {
		idx.byDep[dep] = append(idx.byDep[dep], sub)
	}
}

// subscribeOn регистрирует подписчика на ЯВНО заданные имена, минуя гейт
// переменной, и сам считает своё состояние переданным recalc.
//
// Нужно там, где зависимость диктует не шаблон, а поведение ядра: у
// bind_interface гейта в шаблоне нет вовсе (и быть не обязано — шаблон
// пользовательский), но auto_detect_interface перебивает его в самом sing-box.
// Через subscribe такая строка не подписалась бы ни на что, и клик по галке
// оставлял бы её в прежнем состоянии до полной пересборки вкладки.
//
// recalc, а не общий VarUISatisfiedFor: состояние такой строки — конъюнкция
// гейта шаблона и внешнего условия, и знает о ней только вызывающий.
func (idx *gateIndex) subscribeOn(deps []string, enabled bool, recalc func(resolved map[string]wizardtemplate.ResolvedVar) bool, apply func(bool)) {
	if idx == nil || apply == nil || recalc == nil || len(deps) == 0 {
		return
	}
	sub := &gateSubscriber{apply: apply, last: enabled, recalc: recalc}
	idx.all = append(idx.all, sub)
	for _, dep := range deps {
		idx.byDep[dep] = append(idx.byDep[dep], sub)
	}
}

// affected возвращает подписчиков, затронутых батчем изменённых имён.
//
// Батч, а не имя по одному: каскад on_change меняет несколько переменных за
// один клик, и строка, зависящая от двух из них, должна пересчитаться ОДИН
// раз (SPEC 107 §8.3).
func (idx *gateIndex) affected(changed []string) []*gateSubscriber {
	if idx == nil || len(changed) == 0 {
		return nil
	}
	seen := make(map[*gateSubscriber]bool)
	var out []*gateSubscriber
	for _, name := range changed {
		for _, sub := range idx.byDep[name] {
			if seen[sub] {
				continue
			}
			seen[sub] = true
			out = append(out, sub)
		}
	}
	return out
}

// recompute пересчитывает гейты затронутых строк и обновляет только те, у
// которых результат изменился.
//
// Возвращает число фактически обновлённых строк — по нему видно, была ли
// работа вообще (используется тестами и отладкой).
func (idx *gateIndex) recompute(
	changed []string,
	varByName map[string]wizardtemplate.TemplateVar,
	resolved map[string]wizardtemplate.ResolvedVar,
	target wizardtemplate.TargetSpec,
) int {
	updated := 0
	for _, sub := range idx.affected(changed) {
		var now bool
		if sub.recalc != nil {
			now = sub.recalc(resolved)
		} else {
			vd, ok := varByName[sub.varName]
			if !ok {
				continue
			}
			now = wizardtemplate.VarUISatisfiedFor(vd, varByName, resolved, target)
		}
		if now == sub.last {
			continue // состояние не изменилось — виджет не трогаем
		}
		sub.last = now
		sub.apply(now)
		updated++
	}
	return updated
}

// recomputeSettingsGates — точечный пересчёт гейтов после изменения
// переменных. Возвращает false, если индекса ещё нет: вызывающий тогда падает
// на полную пересборку.
func recomputeSettingsGates(
	gs *wizardpresentation.GUIState,
	td *wizardtemplate.TemplateData,
	model *wizardmodels.WizardModel,
	changed []string,
) bool {
	if gs == nil || td == nil || model == nil {
		return false
	}
	idx, ok := gs.SettingsGates.(*gateIndex)
	if !ok || idx == nil {
		return false
	}
	tgt := model.Target.Normalized()
	vi := wizardtemplate.VarIndex(td.Vars)
	resolved := wizardtemplate.ResolveTemplateVarsFor(td.Vars, model.SettingsVars, td.RawTemplate, tgt)
	idx.recompute(changed, vi, resolved, tgt)
	return true
}
