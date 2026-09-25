# Разведка 2026-09-25: старый обходчик против канона (материал для SPEC 143)

Только чтение, репозиторий не менялся. Сравнение прогонялось тестами на **копии** репо в scratchpad
(файлы сохранены в `scratchpad/recon_tests/`, копия удалена).

## 0. Где что и откуда канон

| Что | Где |
|---|---|
| Норматив языка | `contract/docs/TEMPLATE_LANG.md` §2.2 (типы/int), §3 (`@var`), §4 (`#if`/`#enable`), §5 (Dropped/unresolved), §9 (разрывы C3–C5) |
| Решения | `SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md`: D-011 (модель Dart для unresolved), **D-054** (канон — отдельной точкой входа, legacy живут «до миграции вызывающих»; preset-путь держался за strict «выбросить пресет» — замена отнесена в SPEC 106), **D-055** (int по типу, имена — fallback), D-058 (невалидный `#if` → false) |
| Решение «не унифицировать сейчас» | `SPECS/107-F-O-ENABLE_REACTIVE_CONDITIONS/SPEC.md` п.11 (стр. ~336-366): «Унификация обходчиков — отдельная задача»; аудит 2026-08-24: из 314 фикстур корпуса значение расходится в 5 |
| Страж расхождений | `core/template/walker_parity_test.go:29-42` (`knownWalkerDivergences`, 5 кейсов) |
| Старая | `core/template/substitute.go`: `SubstituteVarsInJSON` :79, `…Strict` :88, walker `substituteWalkCtx` :240, `replacementForPlaceholderCtx` :412 |
| Канон | `core/template/substitute.go:104` (`SubstituteVarsInJSONCanon`) + `core/template/substitute_canon.go` (walker :54, `replacementCanon` :187, `intCastCanon` :232, `selectIfBranchCanon` :309) |
| Общее у обоих | движок предикатов `selectIfBranch/evaluateCond…` (substitute.go:575-873) и **`wantsIntCast` → `isIntCastVar`** (substitute.go:20-44) — канон тоже использует список имён |

### Места вызова в сборке

| Путь | Вызов | Режим |
|---|---|---|
| Главный конфиг (все секции шаблона после `params`) | `core/template/loader.go:409` `ApplyTemplateWithVarsFor` → `GetEffectiveConfigFor` (:550) ← `core/build/build.go:222` `effectiveConfig`; также UI-превью `ui/configurator/business/template_helpers.go:49`, загрузка шаблона `loader.go:334` | `SubstituteVarsInJSON` (lenient) |
| Тела пресетов (rule_set, rules, dns_rule(s), dns_servers, outbounds) | `core/build/preset_expand.go:402` `substitutePresetBody` (вызовы :188 :229 :285 :606 + dns-rules) | `…Strict`; ошибка → выпадает **фрагмент** (не пресет — SPEC 106 уже сузил) + `ExpandWarning` только в лог (`resolve_route.go:195-197`) |
| Тела шаблонных DNS-серверов (`dns_options.servers`) | `core/build/resolve_dns.go:586` `substituteTemplateDNSServer` | `…Strict`, перед ним самодельный Dropped `dropUnsetServerPlaceholders` (:605) |
| `on_change.set` | `EvalIfScalar` substitute.go:526 → legacy walker | lenient |
| Канон | только `core/template/contract_template_test.go:140` | — |

Рядом, но **другой** механизм: `core/config/varsubst.go` — подстановка `@var` в `parser_config.outbounds[].options`
(urltest auto-группы), со **своей копией** списка int-имён `intCastVarNames()` :288 (без clamp, без учёта `type`, коммент ссылается на несуществующий `ui/wizard/template/substitute.go`).

## 1. Таблица расхождений

Проверено мини-тестом (`recon_tests/zz_edge_test.go`) на всех трёх входах. «Прод» — достижимо ли в боевой сборке при текущем загрузочном валидаторе `ValidateWizardTemplate` (`core/template/template_validate.go:83`).

| # | Случай | Старая lenient (главный конфиг) | Старая Strict (пресеты/DNS) | Канон | Код канона | Прод |
|---|---|---|---|---|---|---|
| 1 | `@name` **не объявлено** в vars (значение) | `""` + WarnLog (substitute.go:413-419) | ошибка → фрагмент выпадает | `"@name"` остаётся (canon.go:200-205) | `template_var_undeclared` | Главный конфиг: **нет** — валидатор отвергает шаблон (template_validate.go:152-160). Пресеты: **да**, см. п.13 |
| 2 | Объявлено, значения нет (ключа нет в `resolved`) | `""` | ошибка → фрагмент выпадает | Dropped: ключ удаляется из объекта / элемент из массива (canon.go:207-208, 101-107, 152-154); родитель-объект остаётся `{}` | — | Главный конфиг: **нет** — `ResolveTemplateVarsFor` кладёт в `resolved` КАЖДУЮ объявленную var (vars_resolve.go:351-356). Пресеты: да (varsMap без пустых `ref`/`required`/неактивных) |
| 3 | Необъявленное имя **в предикате** `#if`/`#enable` | false, WarnLog | false | false | `template_var_undeclared` (canon.go:342-414) | нет — валидатор отвергает (template_validate.go:494, 553) |
| 4 | Неизвестная директива `#foo` | удаляется, WarnLog (:271) | то же | удаляется (canon.go:95-98) | `template_unknown_directive` | **да** — валидатор лишь WarnLog (template_validate.go:341): шаблон новой версии |
| 5 | `#if` без `and`/`or` | **TRUE** (:604-606) | TRUE | false (+ else) | `template_unknown_directive` | нет — валидатор (template_validate.go:415) |
| 6 | `#if` с `and` и `or` сразу | false → else | false → else | false → else | `template_unknown_directive` | нет (валидатор :412) |
| 7 | `#if` без `value`, но с `else` | cond=true → пропуск; cond=false → **берёт else** (:578-587) | = | пропуск **всегда**, else не берётся (canon.go:331-335) | `template_unknown_directive` | нет (валидатор :451) |
| 8 | Ветка map-spread `#if` — не объект (скаляр) | пропуск, WarnLog | = | пропуск | `template_unknown_directive` | валидатор такое пропускает → **да**, но значение одинаковое |
| 9 | `#enable` невалидной формы (число и т.п.) | узел выпадает (fail-closed) | = | узел выпадает | `template_unknown_directive` | нет (валидатор `validateCondNode`) |
| 10 | int (`type:int/number` **или имя из списка**) вне [0,65535] | clamp + WarnLog | = | clamp | `template_int_clamped` | **да** — пользователь вводит MTU/порт руками |
| 11 | int-мусор (`abc`) | строкой как есть + WarnLog | = | строкой как есть | `template_int_invalid` | **да**; ядро потом отвергает весь конфиг — сейчас пользователь видит только ошибку ядра |
| 12 | int пустой | `0` | = | `0` | — | одинаково |
| 13 | Переменная без `type:int`, но в legacy-списке | каст по имени | = | каст по имени (тот же `wantsIntCast`, canon.go:224) | как у int | одинаково — **миграция walker'а список не снимает** |
| 14 | bool | trim + EqualFold("true") | = | = | — | одинаково |
| 15 | `text_list` пустой | `[]` + WarnLog | = | `[]` | — | одинаково |
| 16 | text/enum/secret/outbound пустой | `""` + WarnLog | = | `""` | — | одинаково |
| 17 | Одноэлементный массив `["@text"]` / `["@bool"]` / `["@int"]` (не text_list) | **коллапс в скаляр**: `"a"`, `true` (:358-368) | = | массив `["a"]`, `[true]` (§3: коллапс — desktop-legacy, не ядро) | — | bin-шаблон: нет. Старые шаблоны (golden `real-v088`): `"address": ["@tun_address"]` → строка vs массив; для ядра эквивалентно (`address` listable), но это расхождение **не покрыто корпусом** и `knownWalkerDivergences` |
| 18 | `["@text_list", …]` в массиве | splice | = | splice | — | одинаково |
| 19 | Элемент-`#if` с веткой — **литеральный массив** `{"#if":{…,"value":["p","q"]}}` | **splice** `["p","q",…]` (:322-326) | = | **вложение** `[["p","q"],…]` — сплайс только для ветки-ссылки на text_list (canon.go:127, 171-184) | — | bin-шаблон: нет. Корпусом не покрыто — **новое, незафиксированное расхождение** |
| 20 | `"@runtime.platform"` в позиции **значения** | `""` + WarnLog | ошибка → фрагмент выпадает | `"darwin"` (canon.go:190-196) | undeclared, если поле неизвестно | главный конфиг: валидатор для `config` отвергает, но для `params[].value` runtime-ссылки **пропускает** (template_validate.go:143-145) → достижимо в пользовательском шаблоне |
| 21 | `@` в ключах объекта (`{"@txt":"v"}`) | не подставляется | = | не подставляется | — | одинаково |
| 22 | Интерполяция `"x @a y"`, `" @a"`, `"@a@b"` | не подставляется | = | = | — | одинаково (валидатор тримит `" @a"` и считает ссылкой — мелкая несостыковка валидатора, не walker'а) |
| 23 | Вложенные `#if` в ветках, суффиксы `#if1`, порядок нескольких `#if…`, `#not`, `#in`, `#matches`, @var в RHS | общий движок | = | тот же движок | — | одинаково |
| 24 | Значение **Strict-режима** в целом | — | любой unresolved (п.1, 2, 20) → `UnresolvedVarError` → вызывающий бросает фрагмент | ошибок нет никогда; деградация по ключу | — | см. §2 про пресеты |
| 25 | Канал warning'ов | только WarnLog | список имён unresolved | `[]string` кодов **без параметров**, дедуп по коду (canon.go:36-43) | — | реестр требует `params` (`name`/`key`/`value`) — сейчас их нет |

Корпусные 5 известных расхождений (`walker_parity_test.go:29-42`) = строки 1, 2 (×3 фикстуры) и 5. Строки **17 и 19** в корпусе не представлены.

## 2. Реальные шаблоны

### Главный конфиг
Прогон `recon_tests/zz_recon_test.go` (template): `bin/wizard_template.json` × 6 таргетов (darwin/arm64, windows/amd64, windows/386, linux local, linux remote, darwin remote) × 6 наборов состояния (умолчания; tun off + proxy-in; ipv6+gateway+text_list; auth proxy + tls_fragment; мусорные int `tun_mtu=abc`, `proxy_in_listen_port=70000`, `urltest_tolerance=-5`; пустой mtu), с одинаковым `resolved` и `params`:

- **36/36 — побайтно одинаковый JSON.** Коды канона: пусто на всех нормальных наборах; на «мусорных int» — `[template_int_clamped template_int_invalid]`.
- Golden-шаблоны `core/build/testdata/golden/real-v088*/template.json`: одно расхождение везде, где tun включён — `inbounds[N].address` `"172.16.0.1/30"` vs `["172.16.0.1/30"]` (строка 17). Эти шаблоны **не проходят** нынешний `ValidateWizardTemplate` (голый `"tun"` в `if[]`), т.е. в проде не загрузятся; расхождение иллюстрирует риск для старых пользовательских шаблонов.

### Пресеты и DNS-серверы
Инструментирован `substitutePresetBody` и `substituteTemplateDNSServer`: Strict и канон на тех же входах, все 16 пресетов bin-шаблона × 3 таргета × 3 состояния × 3 набора пользовательских vars пресета (включая `use_dns_override=false`, выключенные sniff/resolve): **954 вызова пресетов + 117 DNS — всё совпало, кодов нет.** Условные фрагменты гейтятся до подстановки, поэтому strict-падения не возникают.

**Скрытая ловушка (п.1 в пресетах).** `presetVarsToTemplateVarsWithExtras` (preset_expand.go:440) объявляет walker'у только vars пресета + имена, попавшие в `varsMap`; а `VarValuesFor` (vars_resolve.go:400) **выкидывает глобали с пустым значением**. В `traffic-processing` есть `"strategy": "@resolve_strategy"`. Если глобаль пуста:
- Strict: правило `resolve` выпадает целиком (+ строка в лог);
- канон как есть: имя не объявлено → в конфиг уезжает литерал `"@resolve_strategy"` → **ядро отвергает конфиг**.
При переводе пресетов на канон walker'у надо объявлять **все** `td.Vars` (тогда пустая глобаль → Dropped ключа → валидное правило без `strategy`), плюс нужны гейты валидности фрагмента после каскада (§5.1: правило без `outbound`/`action` выпадает) — в Go есть лишь `isRuleEmpty` по rule_set.

### Четыре legacy-имени в `bin/wizard_template.json`

| Имя | Объявленный `type` | Где подставляется | Снять из списка без изменения поведения? |
|---|---|---|---|
| `tun_mtu` | `text` (стр. ~186) | `config` `"mtu"` (стр. 1958) | **нет** — уедет строкой `"1492"`, ядро отвергнет |
| `proxy_in_listen_port` | `text` (~285) | `"listen_port"` (1990) | **нет** |
| `urltest_tolerance` | `enum` с объектными `options` (499-520) | `config` `"tolerance"` (2042) **и** `parser_config` auto-группы (стр. 20, через `core/config/varsubst.go`) | **нет**, и даже `type:int` не поможет: объектные `options` насильно превращают тип в `enum` (`vars_resolve.go:169-179`, разрыв N5) |
| `mixed_listen_port` | не объявлена вовсе | нигде | да — мёртвое имя из старых шаблонов |

`contract/registry/vars.json` уже числит все три живых как `type:int` с пометкой «desktop сегодня text/enum + каст по имени (C5)».

**Вывод:** `isIntCastVar` сегодня снять нельзя — ни одна из трёх живых переменных не объявлена `int`.

## 3. Куда отдавать коды пользователю

Механизм есть: `config.BuildReportEntry{Kind, Subject, Reason, Code, Params}` (`core/config/build_report.go:103`), UI берёт текст по `Code` из реестра на языке интерфейса (`ui/configurator/tabs/final_report_model.go:134-137, 250`). Все четыре кода в `warnings.json` уже есть с `title/text_{en,ru}`, в поле `go` честно записано «в сборке не ставится».

Что надо дотянуть:
1. **Параметры.** `canonCtx.warn(code)` хранит только код и дедупит по нему. Реестр ждёт `name` (undeclared, int_*), `value` (int_*), `key` (unknown_directive). Нужен накопитель `[]{Code, Params}` с дедупом по (код, параметры) — иначе текст отчёта выйдет с `{name}` или про «какую-то» переменную.
2. **Канал из сборки.** `ApplyTemplateWithVarsFor`/`GetEffectiveConfigFor` возвращают только JSON → добавить warnings в возврат (или вариант функции), `build.effectiveConfig` (build.go:216) кладёт их в `Result` (новое поле рядом с `ExcludedSources`), а пресетный/DNS-путь — туда же.
3. **Запись в отчёт.** Рядом с `FeedBuildReportFromSanitizer` (`core/build_report_feed.go:280`) — подача template-кодов в ту же попытку; вызывается в обоих местах сборки: `core/rebuild.go:207` и `ui/configurator/business/create_config.go:251`.
4. **Вид записи.** Подходящего `BuildReportKind` нет (все про источники/узлы). Нужен новый, например `template_degraded`, + место в порядке `finalReportLines` (final_report_model.go:90) и строка рендера (сейчас упадёт в общий `"%s: %s"` — годится, если Subject = имя переменной/директивы). Реестровый код уже даёт русский/английский текст, новые `locale.T` почти не нужны.

## 4. Рекомендация и план

### Что реально меняется для пользователя
- **Главный конфиг на bin-шаблоне — ничего** по значению (36/36). Появятся строки в «Итоге» только когда пользователь ввёл мусор в MTU/порт/tolerance (`template_int_invalid`/`clamped`) или шаблон новее приложения (`template_unknown_directive`). Остальные коды в главном пути недостижимы из-за загрузочного валидатора.
- **Пользовательские шаблоны**: могут поменяться значения по строкам 17 (`["@text"]` → массив) и 19 (литеральный массив в `#if`-элементе перестанет сплайситься) и 20 (`@runtime.*` в `params[].value` начнёт подставляться).
- **Пресеты/DNS** (если переводить и их): вместо «выпал фрагмент, запись в логе» — «выпал ключ»; нужны гейты валидности фрагмента и полный список объявлений, иначе регресс (см. `@resolve_strategy`).

### План (по шагам, каждый изолируем)
1. **Закрыть незафиксированные расхождения корпусом**: фикстуры для строк 17 и 19 (и 20 через params, если это desktop-расширение). По 17 канон уже высказался (§3: коллапс — legacy); по 19 норматив §4.4 «элемент заменяется веткой» = вложение, но решение нужно явное (ниже вопрос 2).
2. **Канал warning'ов с параметрами** в каноне (`canonCtx` → `[]TemplateWarning{Code, Params}`), без смены walker'а в сборке.
3. **Главный конфиг → канон**: `ApplyTemplateWithVarsFor` зовёт `SubstituteVarsInJSONCanon`, warnings едут в `build.Result` → `BuildReportEntry` (новый вид). Страж `walker_parity_test.go` после этого пересобрать: lenient остаётся только у `EvalIfScalar` (или перевести и его). Риск низкий: на bin-шаблоне значения идентичны, проверено.
4. **Пресеты/DNS → канон** отдельной волной: объявлять walker'у все `td.Vars` + vars пресета; гейты валидности фрагмента после Dropped (rule без `outbound`/`action`, dns_rule без `server`); `ExpandWarning` тоже в отчёт. После этого удалить `…Strict`, `UnresolvedVarError`, `dropUnsetServerPlaceholders` (канон сам делает Dropped).
5. **Снять `isIntCastVar`** — отдельно и только после правки шаблона:
   - `tun_mtu`, `proxy_in_listen_port` → `type:"int"` в `bin/wizard_template.json` (UI рендерит `int` как текстовое поле — ветка `default` в `settings_tab.go:886`; хорошо бы валидатор числа, как у `number` в `preset_ref_edit_dialog.go:217`);
   - `urltest_tolerance` — требует решения по N5 (int + объектные options сейчас насильно `enum`);
   - `core/config/varsubst.go:288` — вторая копия списка для parser_config: перевести на объявленный `type` из шаблона (он уже читает типы для bool, `loadTemplateVarDefaults`) + clamp;
   - `mixed_listen_port` можно выбросить из обоих списков сразу;
   - `contract/registry/vars.json` и §2.2/§9 C5 TEMPLATE_LANG — снять пометку «fallback».
   - Контракт: правка `bin`-шаблона и `vars.json` → бамп `contract/VERSION` + changelog + TASKS_LXBOX по правилам репо.

## Вопросы владельцу
1. Переводить на канон **только главный конфиг** (низкий риск, почти без видимых изменений) или сразу и пресеты/DNS (меняется семантика «выпал фрагмент» → «выпал ключ»)?
2. Строка 19: литеральный массив в ветке `#if`-элемента — **сплайсить** (как сейчас в проде; тогда поправить канон и §4.4) или **вкладывать** (канон/§4.4; тогда пользовательские шаблоны с таким приёмом меняют вывод)?
3. Строка 17: одноэлементный коллапс `["@text"]` → скаляр — окончательно хороним (канон §3) или оставляем desktop-расширением?
4. `urltest_tolerance`: как совместить выпадающий список с подписями и число в JSON — разрешить `type:int` + объектные `options` (снять насильный `enum`, разрыв N5) или другой путь?
5. Нужен ли отдельный вид записи отчёта (`template_degraded`) и где он в порядке «Итога» (предлагается — первым: причина в шаблоне/настройках объясняет всё ниже)? Блокирует ли `template_int_invalid` кнопку Save (ядро такой конфиг всё равно отвергнет)?
