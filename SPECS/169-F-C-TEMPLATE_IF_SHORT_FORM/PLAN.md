# PLAN 169

Принцип: **один разбор формы `#if` на весь движок.** Сегодня каждая точка (map-spread, array-element, `EvalIfScalar`, `default_value`, валидатор) сама лезет в тело `#if` за `and`/`or`/`value`/`else`. Вводится один нормализатор, который по типу значения `#if` отдаёт `(cond, value, hasValue, else, hasElse, form)`, и все точки переводятся на него. Форма — только через нормализатор; тогда короткая и длинная формы не могут разойтись.

## Этап 1 — норма и корпус (контракт 1.1.125)

1. `contract/docs/TEMPLATE_LANG.md` — по SPEC §3: §4.1 (три формы по типу значения, `cond` без обёртки, позиционное чтение, запрет сахар-списка, длины 2–3, `null`), §4.3–4.5 (примеры в обеих формах; упоминания «тело `#if` — объект» заменить на «тело `#if` в любой из форм»), §4.5.1 (канон коротко; `#value`/`#else` — слова длинной формы), §4.6 (абзац D-133: плоский гейт `#if` отвергнут, канон гейта фрагмента — array-element; `#enable` у объявлений и `params` незаменим), §6.7.1 (операция с `#if`), §7.4, §9 (строка разрыва для Dart). Упоминание «легаси-обходчик» в §4.1 снять: обходчик в Go один (`substituteWalkCanon`).
2. Корпус `contract/corpus/template/` — кейсы по списку SPEC §3, парные с длинной формой (одинаковый `expected`). Разделы: `grammar/`, `map_spread/`, `array_element/`, `on_change/`, `for_each_node_do/`. `deps/` без изменений: раннер читает `cond`, а не `#if`-узел, грамматика `cond` не менялась. README корпуса: в таблице разделов `grammar/` — «три формы `#if`», `for_each_node_do/` — «операция с `#if`».
3. `contract/VERSION` → 1.1.125; `contract/README.md` — строка changelog; `contract/TASKS_LXBOX.md` §122 (формат соседних параграфов: что, зачем, норма, кейсы-критерий, что у LxBox трогать — `if_engine.dart`, валидатор загрузки, `evalIfScalar`, `default_value`, `preset_expand.dart` `#do`).

## Этап 2 — движок (`core/template`, `core/build`)

4. **Нормализатор формы** — новый файл `core/template/if_form.go`:
   - `type ifForm struct { Cond interface{}; Value interface{}; HasValue bool; Else interface{}; HasElse bool; Short bool }`;
   - `func parseIfForm(raw interface{}) (ifForm, string)` — по типу `raw`: `[]interface{}` длины 2–3 → `Short` (cond = [0], проверка «cond не массив», `[1]`/`[2]` не `nil`); `map[string]interface{}` с `condKey(m,"value")` → длинная (cond = cond-obj из `and`/`or` по сегодняшней проверке `hasAnd != hasOr`); иначе — сообщение об ошибке формы (строка для валидатора и для `warnDirective`);
   - `func evalIfForm(f ifForm, …) (branch interface{}, take bool)` — общий выбор ветки: `evaluateCond(f.Cond, …)` → `Value` / `Else` / «не выбрана». Для длинной формы `Cond` собирается как `{"#and": list}` / `{"#or": list}` из тела, чтобы `evaluateCond` работал одинаково.
5. `core/template/substitute_canon.go`: `handleIfMapSpreadCanon`, `handleIfArrayElementCanon` (сигнатура: принимать `rawBody interface{}`, не `map`), `selectIfBranchCanon` → через `parseIfForm`/`evalIfForm`; `notePredicateVars(body)` → `noteCondVars(f.Cond)` (уже есть для `#enable`). Невалидная форма — `warnDirective(key)` + ветка не выбрана, как сегодня. В обходе массива (`substituteWalkCanon`, case `[]interface{}`) снять требование `m[ks[0]].(map)` — тело может быть массивом.
6. `core/template/substitute.go`: `selectIfBranch(body map…)` и `evaluateIfCondition` → обёртки над `evalIfForm`; `EvalIfScalar` — тело через `parseIfForm` (массив строк `["@a", "x", "y"]` даёт скаляр). `core/template/vars_default.go` `resolveDefaultNode`: `body.(map)` → `parseIfForm`.
7. `core/template/template_validate.go`: `validateIfBody(raw)` → по `parseIfForm`: короткая форма — `validateCondNode(cond)` (уже есть для `#enable`), запрет сахар-списка в `cond`, длина, `nil`-ветки, рекурсия `walkValidateIf` в обе ветки; длинная — как сегодня. `walkValidateIf` (case `[]interface{}`): single-key обёртка принимается с телом любого типа. `validateDefaultValueIf` — через ту же `validateIfBody`. Сообщения с контекстом `…#if: …` по SPEC §5.4.
8. `core/template/node_loop.go` — операция с условием (SPEC §2.3):
   - `NodeDoOp` получает `Raw map[string]interface{}` (объект операции как написан) и список разобранных веток `[]doBranch{Append interface{}}` для валидации;
   - `parseNodeDo`: если в объекте операции есть ключ `#if…` — ровно один, других ключей нет, тело через `parseIfForm`, каждая ветка — объект ровно с одним глаголом `#append` и валидной правой частью (`validAppendValue`); иначе — прежний разбор;
   - `ValidateNodeLoops` — типы переменных для всех веток (как сегодня для `Append`);
   - новая `func NodeDoResolve(op NodeDoOp, vars, resolved, target, node) (rhs interface{}, present bool, warnings)`: объект операции прогоняется через `SubstituteVarsInJSONCanonScoped` как `{"op": Raw}` (map-spread сам вырежет `#if` и вольёт ветку), результат `{}` → `present=false`; `{"#append": rhs}` → `rhs`. Без `#if` — прежний путь (`Raw` = `{"#append": …}`), чтобы ветка одна.
9. `core/build/preset_expand.go` (`expandNodeLoopDo`, вызов `template.NodeAppendValue` ~стр. 299): перед `NodeAppendValue` — `NodeDoResolve`; `present=false` → `continue` без предупреждения. Предупреждения подстановки — тем же `substitutionWarnings`.
10. `core/template/cond_deps.go` — без изменений (грамматика `cond` та же); проверить, что `GateCond.Deps` и корпус `deps/` зелёные.
11. Раннеры корпуса: `core/template/contract_template_test.go` (разделы по каталогам — новых не добавляем), `core/build/contract_node_loop_test.go` (`for_each_node_do/` — новые кейсы). Если раннер `for_each_node_do/` разбирает `#do` сам (а не через `parseNodeDo`), перевести на общий разбор.

## Этап 3 — шаблон, доки

12. `bin/wizard_template.json`: 20 конструкций `#if` → короткая форма (`#enable` не трогать). Проверка: `go test ./core/build -run 'TestWizardTemplateConfigUnchanged|TestGoldenScenarios' -count=1` — golden-конфиги байт в байт. Этап снимается решением владельца без последствий для остального (SPEC §4).
13. `docs/TEMPLATE_REFERENCE.md` + `docs/TEMPLATE_REFERENCE.ru.md` §9 (грамматика `#if`, примеры), `docs/WIZARD_TEMPLATE.md` + `.ru.md` при упоминании формы; `docs/release_notes/upcoming.md` (EN/RU); `docs/ARCHITECTURE.md` — только если менялись потоки (не ожидается); `IMPLEMENTATION_REPORT.md`.

## Проверки

- `go build ./...`.
- Локально по имени: `go test ./core/template -run 'TestContractCorpusTemplate|TestContractCorpusDeps' -count=1`; `go test ./core/build -run 'TestContractCorpusNodeLoops|TestWizardTemplateConfigUnchanged|TestGoldenScenarios' -count=1`.
- Полный прогон — CI: `gh workflow run ci.yml --ref develop -f run_mode=tests`; упавшее — `gh run view <ID> --log-failed`.
- Win7: в новом коде без builtin `min`/`max`, `slices`, `maps`.

## Границы

- Файлы: только перечисленные выше и новые `core/template/if_form.go`, кейсы корпуса, папка SPEC.
- Не трогать: `#enable` и его семантику, `#filter`, D-044, реестр протоколов, UI.
