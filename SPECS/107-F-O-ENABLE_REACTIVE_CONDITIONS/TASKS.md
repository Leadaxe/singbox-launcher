# TASKS 107 — `#enable` + рекурсия условий + реактивный пересчёт

Порядок — §16 SPEC.md. Каждая галка = зелёные тесты обеих сторон на этом шаге.
Развилки — в `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md` (D-067+).

## 1. Движок условий — рекурсия
- [x] Go `core/template/substitute.go`: `evaluatePredicate` — объект с ключом `and`/`or` → вложенный cond; `evaluateIfCondition` — элемент списка = предикат или cond-obj; `#not` над cond
- [x] Go `template_validate.go`: снять запрет вложенности, валидировать рекурсивно; bare без `@` — по-прежнему ошибка
- [x] Dart `if_engine.dart`: `_evalPredicate`/`_evalCondition` — рекурсия; валидатор — снять `TemplateIfError` на вложенном and/or
- [x] Go `EvalIfScalar` (on_change) и Dart `evalIfScalar` — через общий движок (проверить, не копия ли)
- [x] Фикстуры `corpus/template/predicates/nested_*` (глубина 2–3, `#not` над and/or, пустые вложенные, невалидный вложенный → false + warning)
- [x] Обе стороны зелёные; DECISIONS: D-018 снят

## 2. Зависимости (контракт)
- [x] `deps(cond)` в Go и Dart — по всему дереву, RHS/`#in`/`#matches`, невычисленные ветки, `@runtime.*` как псевдо-имена
- [x] Раздел `corpus/template/deps/` (`<case>.cond.json` → `{"deps":[...]}`), раннеры обеих сторон
- [x] `contract/registry/warnings.json`: внести существующие `template_*` коды + `template_enable_invalid`

## 3. `#enable` в дереве конфига
- [x] Go legacy-обходчик `substitute.go` (pre-pass до «unknown control-construct — dropping», L~217): `#enable` первым, false → узел выпадает (map-ключ / array-элемент)
- [x] Go канонический `substitute_canon.go` (pre-pass до `warnUnknownDirective`, L~75): то же, через `droppedValue`
- [x] Dart `_walkMap` (до `unknownBang`, L~171) и `_walkList`: `#enable` первым, false → `Dropped.instance`
- [x] Ключ всегда вырезан; при false ничего внутри не вычисляется
- [x] Фикстуры `corpus/template/enable/` (значение ключа / элемент массива / сахар / cond-obj / рекурсия / `#enable`+`#if` порядок / внутри ветки `#if` / невалидный → нет узла + warning)

## 4. Носители и легаси-алиасы
- [x] Go: `PresetRuleSet`/`PresetDNSServer`/`PresetOutbound`/`PresetVar`/`TemplateVar`/`TemplateParam` — читать `#enable`; нормализация `if`/`if_or`/строк-JSON (SPEC 097) в один cond на входе
- [x] Go `ifexpr.go`: переписать черновик (`ParseEnableGate`/`EvalEnable`/`MergeEnableGateJSON` — форма `["or",…]` отвергнута)
- [x] Go `VarUISatisfiedFor`, `applyParamsFiltered`, `filterActiveVars`, все `evalIf(...)` в `preset_expand.go` — через движок по cond
- [x] Go `preset_expand.go`: `extractIfFromMap` читает `#enable`; удалить `#enable` из копии после `deepCopyMap` до подстановки; `delete(m,"#enable")`
- [x] Dart `preset_expand.dart`: `enabled:"@var"` → cond; гейт на `rules[]`/`dns_servers[]`/`dns_rules[]`; `remove('enabled')` только для строковой формы; `remove('#enable')`
- [x] Dart `parser_config.dart`: `#enable` у `WizardVar` (новое для мобилы) и переменных пресета; `TemplateDnsServerEntry.enabled` не трогать
- [x] Семантики сохранены: vars шаблона — UI-гейт (значение подставляется); vars пресета — удаление из varsMap
- [x] Тест байт-идентичности: конфиг из шаблона с легаси `if` == конфиг из того же шаблона с `#enable`

## 5. Шаблоны
- [x] `bin/wizard_template.json`: 29 × `"if"` → `"#enable"` скриптом; JSON валиден; `go test ./...` зелёный
- [x] LxBox `assets/wizard_template.json`: `/selectable_rules[2]/rule_set[2].enabled:"@geoip_enabled"` → `#enable`; 15 bool-`enabled` не трогать
- [x] Копия шаблона в бандл; перезапуск лаунчера **по согласованию с владельцем**; живой `config.json`: `route.rules`/`inbounds` без изменений, `sing-box check` OK

## 6. Реактивный UI-драйвер
- [x] Go `settings_tab.go`: индекс deps → строки; на изменение — батч после on_change, пересчёт затронутых, сравнение с кэшем, точечный Enable/Disable + приглушение подписи; полная пересборка только при смене шаблона/таргета
- [x] Go `preset_ref_edit_dialog.go`: тот же драйвер для полей пресета
- [x] Go `target_tab.go`: смена платформы → батч `runtime.platform`/`runtime.arch`
- [~] Dart `settings_screen.dart`: индекс deps → поля поверх существующего per-key emit `model.set`; гейт активности строки по `#enable` — реактивность уже была на `model.set`; `#enable` у мобильных переменных отложен (гейтов в шаблоне LxBox сегодня нет) → SPEC 106
- [x] Проверка приёмки §15.5

## 7. Документация и закрытие
- [x] `contract/docs/TEMPLATE_LANG.md` §4.1/§4.2/§4.6/§8/§9
- [x] `docs/TEMPLATE_REFERENCE.md` + `.ru.md`; LxBox `docs/TEMPLATE.md`
- [x] `docs/release_notes/upcoming.md` EN+RU
- [x] DECISIONS (D-067+), этот TASKS; `tool/sync_contract.sh` + `contract.lock`
- [x] Грep на `min(`/`max(`/`slices.`/`maps.` в не-test Go (Win7 go1.20)
- [x] Версию не бампать

---

## Журнал выполнения

**Этап 1 — рекурсия (Go) ✅ 2026-08-21**
`evaluateCond`/`evaluateCondObj` в `substitute.go` — единая точка для #if,
#enable и гейтов; элемент списка = предикат ИЛИ вложенный cond-obj; `#not` над
любым cond. `evaluateIfCondition` стал тонкой обёрткой. Фикстуры
`predicates/nested_*`, `not_over_and` (5 шт.) — зелёные.

**Этап 3 — `#enable` в дереве конфига (Go) ✅ 2026-08-21**
Оба обходчика: `substitute.go` (legacy, боевой путь конфига) и
`substitute_canon.go`. Гейт вычисляется ДО pre-pass ctrl-ключей — иначе ключ
выбрасывался бы как неизвестная директива, а узел оставался в конфиге всегда
(ловушка §11.1). При false — `droppedValue`, узел не обходится.
В legacy-обходчик добавлены: отсев Dropped в полях map, в фильтрующей ветке
массива и в обходе «на месте», а также страховка на корне дерева.
Условие включения фильтрующей ветки массива расширено: раньше она срабатывала
только при наличии `#if`-обёртки, и элемент с одним лишь `#enable` проходил
мимо. Фикстуры `enable/` (13 шт.) — зелёные, включая порядок вычисления
(опечатка внутри выключенного узла не даёт warning) и fail-closed.

**Этап 2 — зависимости ✅ (Go) 2026-08-21**
`core/template/cond_deps.go`: `CondDeps`/`CondDepsJSON`. Раздел корпуса
`deps/` (14 кейсов) + раннер `TestContractCorpusDeps`. Попутно `notePredicate`
в каноническом обходчике сделан рекурсивным (вложенные cond-obj не давали
warning на необъявленное имя) и добавлен `noteCondVars` для гейта.

Итого корпус шаблонов: **90 фикстур + 14 deps**, Go зелёный; `go test ./core/...`
— 12 пакетов.

**Этапы 1–5 завершены 2026-08-22 (Go + Dart, кроме UI-драйвера)**

Go: рекурсия, `#enable` в обоих обходчиках, `deps()`, нормализация гейта на
всех носителях (фрагменты/переменные пресета/переменные шаблона/params),
шаблон переведён (29 гейтов), тест байт-идентичности конфига.
Dart: рекурсия, `#enable` в `_walkMap`, `deps()` + раннер, гейт фрагментов,
шаблон переведён (1 гейт; 15 bool-`enabled` не тронуты).

Найдено и исправлено по ходу — 6 регрессий, все в DECISIONS (D-067…D-072):
голые имена в легаси-списках, потеря учёта платформы операнда, JSON-предикат
строкой внутри `#enable`, «if+if_or = ошибка», сплайсинг списка в Dart,
удаление `enabled` у вложенных объектов.

Корпус: **90 фикстур + 15 deps**, 100 % на обеих сторонах.
Тесты: лаунчер 33 пакета, LxBox 3454 — зелёные.

Осталось: **этап 6** (реактивный UI-драйвер) и **этап 7** (документация,
release notes).

**Этап 6 — реактивный UI-драйвер ✅ 2026-08-22 (Go)**
`ui/configurator/tabs/settings_reactive.go`: `gateIndex` (инвертированный
индекс `имя → строки`), подписка при сборке каждой из 7 веток строки
(`bindRowGate`), `setRowEnabled` в обе стороны + приглушение подписи,
`recomputeSettingsGates` вместо полной пересборки в `applyOnChangeAndRefresh`.
Батч = изменённая переменная + цели каскада (D-075 — поймано на живом UI).
7 тестов `TestGateIndex*`/`TestBundledTemplateGateDeps`/`TestGateBatchIncludesChangedVarItself`.
Dart: реактивная часть на модели `model.set` per-key emit уже была;
`#enable` на объявлениях var в Settings-экране — оставлено на следующую
итерацию (гейтов у мобильных переменных в шаблоне сегодня нет).

**Помеченные ключевые слова (D-073) ✅ 2026-08-22**
`condKey` в обоих движках: канон `#and/#or/#value/#else/#on_change/#set`,
легаси без `#` читается бессрочно. Валидаторы загрузки обеих сторон, closed-
схема тела `#if` в Dart. Оба шаблона переведены (лаунчер: 21 `#and`, 21
`#value`, 10 `#else`, 1 `#on_change`; LxBox: 22/22/4/3). `options[].value` —
поля данных — не тронуты. Фикстуры `grammar/hashed_keywords_*`,
`legacy_keywords_still_read`, `mixed_hashed_and_legacy`, `deps/hashed_keywords`.

**Issue #106 ✅ 2026-08-22** — `auto_route` стал переменной (D-074), тест
`TestAutoRouteIsUserControlled`.

**Этап 7 — документация ✅ 2026-08-22**
`contract/docs/TEMPLATE_LANG.md` (§4.2 рекурсия, §4.5.1 пометка ключевых
слов, §9 D-018/D-073), `docs/TEMPLATE_REFERENCE.md` + `.ru.md` (§9.1
naming discipline переписан под одно правило, §9.2 форма с рекурсией,
§9.2.1 `#enable`), LxBox `docs/TEMPLATE.md` (раздел SPEC 107 + канон в
примере on_change), `docs/release_notes/upcoming.md` EN+RU (4 пункта
Highlights + 1 Technical).

Итого корпус: **94 фикстуры + 16 deps**, 100 % на обеих сторонах.
