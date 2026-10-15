# TASKS 169

## Этап 1 — норма и корпус (контракт 1.1.125)
- [x] T1 `contract/docs/TEMPLATE_LANG.md` §4.1, §4.3–4.5, §4.5.1, §4.6, §6.7.1, §7.4, §9 — по SPEC §3; решение D-133
- [x] T2 Корпус: `grammar/short_*` (10 кейсов SPEC §3, включая `load: reject`), `map_spread/short_*` (3), `array_element/short_*` (4), `on_change/short_set_target`, `for_each_node_do/do_if_*` (9); парные `expected` равны длинной форме; README корпуса
- [x] T3 `contract/VERSION` → 1.1.125; `contract/README.md` changelog; `contract/TASKS_LXBOX.md` §122

## Этап 2 — движок
- [x] T4 `core/template/if_form.go` — `ifForm`, `parseIfForm`, `evalIfForm` (единственное место, знающее обе формы)
- [x] T5 `core/template/substitute_canon.go` — map-spread, array-element, `selectIfBranchCanon`, `notePredicateVars` → нормализатор; массив как тело в обходе массива
- [x] T6 `core/template/substitute.go` (`selectIfBranch`, `evaluateIfCondition`, `EvalIfScalar`), `core/template/vars_default.go` (`resolveDefaultNode`) → нормализатор
- [x] T7 `core/template/template_validate.go` — `validateIfBody`, `walkValidateIf`, `validateDefaultValueIf`: короткая форма, отказы по SPEC §2.1 с контекстом
- [x] T8 `core/template/node_loop.go` — `NodeDoOp.Raw`/ветки, `parseNodeDo` с `#if…`, `ValidateNodeLoops` по веткам, `NodeDoResolve`
- [x] T9 `core/build/preset_expand.go` — `NodeDoResolve` перед `NodeAppendValue`; пропуск узла без предупреждения
- [x] T10 Раннеры корпуса (`contract_template_test.go`, `contract_node_loop_test.go`) — новые кейсы зелёные; `deps/` без изменений зелёный

## Этап 3 — шаблон, доки
- [x] T11 `bin/wizard_template.json` — 20 `#if` → короткая форма; `TestWizardTemplateConfigUnchanged` и golden байт в байт (этап снимается решением владельца)
- [x] T12 `docs/TEMPLATE_REFERENCE.md` + `.ru.md` §9; `docs/WIZARD_TEMPLATE*.md` при упоминании; `docs/release_notes/upcoming.md` (EN/RU); `IMPLEMENTATION_REPORT.md`
- [x] T13 `go build ./...`; раннеры по имени; CI `run_mode=tests` зелёный; папка → `169-F-C-…`
