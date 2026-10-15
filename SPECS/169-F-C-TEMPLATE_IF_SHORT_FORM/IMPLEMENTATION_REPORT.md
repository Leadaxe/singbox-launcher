# IMPLEMENTATION REPORT 169

**Итог:** короткая тернарная форма `{"#if": [cond, V, E]}` — канон языка шаблонов; длинная форма читается бессрочно; операция `#do` может быть условной. Контракт 1.1.125, решение D-133. Шаблон лаунчера переписан на короткую форму, golden-конфиги байт в байт прежние.

## Что сделано
- **Норма:** `contract/docs/TEMPLATE_LANG.md` §4.1 переписан (три формы по типу значения, `cond` без обёртки, запрет сахар-списка, позиционное чтение, длины 2–3, `null`), §4.3–4.5 и §4.7 — примеры в обеих формах, §4.5.1 — канон коротко, §4.6 — абзац D-133 (плоский гейт отвергнут, `#enable` незаменим у объявлений и `params`), §6.7.1 — операция с условием, §7.4 — `default_value` коротко, §9 — строка D-133. Упоминание «легаси-обходчика» снято.
- **Контракт:** `VERSION` → 1.1.125, строка changelog в `contract/README.md`, `contract/TASKS_LXBOX.md` §122 (самодостаточный для агента LxBox: грамматика, разбор по типу, отказы, `#do` с `#if`, какие файлы Dart трогать, список кейсов-критерия), `go generate ./contract/...` (index.md).
- **Корпус** (`contract/corpus/template/`): `grammar/short_*` (10), `map_spread/short_*` (3) + длинный близнец `else_merge`, `array_element/short_*` (4), `on_change/short_set_target`, `for_each_node_do/do_if_*` (9, из них 4 `load: reject`). README корпуса обновлён.
- **Движок:** новый `core/template/if_form.go` — `ifForm`, `parseIfForm` (единственное место разбора формы), `evalIfForm`. На него переведены `selectIfBranchCanon`/map-spread/array-element (`substitute_canon.go`), `EvalIfScalar` и `selectIfBranch` (`substitute.go`, `evaluateIfCondition` удалена), `resolveDefaultNode` и `UnmarshalJSON` (`vars_default.go`: массив под `#if` сохраняется), `validateIfBody`/`validateDefaultValueIf` (`template_validate.go`). `node_loop.go`: `NodeDoOp{If, Cond, Value, Else, HasElse}`, `parseNodeDo` с `#if…` (один ключ, ничего рядом, глаголы в обеих ветках), новая `NodeDoValue`; `preset_expand.go` зовёт её вместо `NodeAppendValue`.
- **Шаблон:** 21 конструкция `#if` в `bin/wizard_template.json` → короткая форма (`#enable` не тронут); `TestWizardTemplateConfigUnchanged` и `TestGoldenScenarios` зелёные без правки golden.
- **Доки:** `docs/TEMPLATE_REFERENCE.md` + `.ru.md` §9.2–9.3, §9.8, §10.4; `docs/WIZARD_TEMPLATE.md` + `.ru.md` §4.1–4.2, §8.3, таблица ошибок; `docs/release_notes/upcoming.md` (EN/RU).

## Пример из шаблона (длинная → короткая)

```json
"#if": {"#and": [{"@runtime.platform": "windows"}, {"@runtime.arch": "386"}], "#value": "gvisor", "#else": "system"}
"#if": [{"#and": [{"@runtime.platform": "windows"}, {"@runtime.arch": "386"}]}, "gvisor", "system"]

"#if": {"#and": ["@ipv6_enabled"], "#value": "prefer_ipv4", "#else": "ipv4_only"}
"#if": ["@ipv6_enabled", "prefer_ipv4", "ipv4_only"]
```

## Фрагмент `#do` с `#if` (кейс `for_each_node_do/do_if_else_other_value`)

```json
"#do": {
  "@list": { "#if": [{ "@node.body.server": "#notEmpty" }, { "#append": "@node.body.server" }, { "#append": "@node" }] }
}
```
Узлы `a` (server `a.example`), `home-ts` (Tailscale, без server), `e` (server `b.example`) → `["a.example", "home-ts", "b.example"]`.

## Решения по умолчанию
- **Поле `@node.allow_lan` в примерах** SPEC §2.3 заменено в норме, §122 и changelog на существующие имена узла (`@node.body.type`, `@node.skip_presets`): поля записи `allow_lan` в контракте нет (оно придёт с SPEC 170), а норма для LxBox не должна ссылаться на несуществующее поле. Язык от этого не меняется.
- **Невалидная форма в толерантном рантайме.** Длинная форма с невалидным `and`/`or` ведёт себя как прежде (условие ложно → `else`, если есть; кейсы `if_without_and_or_is_false`, `if_with_both_and_or_is_false` не тронуты). У короткой формы невалидность — свойство всего массива: ветка не выбирается вовсе, даже при истинном условии (`short_null_else_rejected`). Оба пути — `template_unknown_directive {key}`.
- **`NodeDoValue` вместо пары `NodeDoResolve` + `NodeAppendValue`** (PLAN п. 8–9): одна функция даёт текст значения на узле для обеих операций. Условная прогоняется map-spread'ом объекта `{"#if": [cond, {"append": V'}, {"append": E'}]}` с пространством узла (ветка-глагол под нейтральным ключом — ключ на `#` обходчик выбросил бы как неизвестную директиву; правые части веток заранее сведены к вставке `#tpl`, как у безусловной операции). `{}` → `""` → узел пропущен без предупреждения.
- **Условие операции на загрузке не валидируется глубже формы** (как и `#filter`): имена `@node.*` динамические.
- **`default_value` в корпусе не закреплён:** §7.4 — desktop-расширение, Dart схлопывает объект в `default`; кейс сломал бы паритет. Покрыт `vars_default_test` через `ForTargetIn` косвенно (шаблон лаунчера с короткой формой в `default_value` проходит golden).
- **`docs/TEMPLATE_REFERENCE.md` §9.7** («реальный пример TUN») оставлен в длинной форме — она законна, раздел иллюстрирует структуру, а не форму.
- Лишний пробел в одной строке шаблона (`"#tpl"` у `preferred_by`) выровнен пересборкой файла `json.dumps(indent=1)` — иного диффа форматирования нет.
- `docs/ARCHITECTURE.md` не менялся: потоки прежние, добавилась одна функция разбора формы.

## Проверки
`go build ./...`; `gofmt -l` пусто; `go test ./core/template -run 'TestContractCorpusTemplate|TestContractCorpusDeps'`; `go test ./core/build -run 'TestContractCorpusNodeLoops|TestWizardTemplateConfigUnchanged|TestGoldenScenarios'`; по имени — `TestIf_*`, `TestValidate*`, `TestOnChange*`, `TestDefault*`, `TestNodeLoop*`, `TestPreset*`, `TestLoad*`, `TestRefreshTemplateIfStale*`; `go generate ./contract/...`. В новом коде нет builtin `min`/`max`, `slices`, `maps` (Win7). Полный прогон — CI `run_mode=tests` (см. ниже).
