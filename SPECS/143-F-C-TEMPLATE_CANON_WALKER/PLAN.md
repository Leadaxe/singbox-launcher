# PLAN 143 — порядок работ

Пять волн, каждая изолируема и собирается отдельно. Порядок важен: корпус и контракт
раньше кода, снятие зашитых имён последним.

## Волна 0. Норматив и корпус (контракт 1.1.68)

Файлы: `contract/docs/TEMPLATE_LANG.md`, `contract/corpus/template/**`, `contract/registry/vars.json`,
`contract/registry/warnings.json` (поле `go`), `contract/VERSION`, changelog, `contract/TASKS_LXBOX.md` (§64),
`SPECS/103-F-O-LX_SHARED_CONTRACT/DECISIONS.md`.

1. TEMPLATE_LANG: §2.1 (`options` любой тип кроме `bool`, `options_open`), §2.2 (`enum` синоним, снять
   упоминание fallback-списка имён), §3 (запрет коллапса), §4.4 (сплайс на один уровень, двойные скобки, пример
   из SPEC §3.2), §7.2 (`@runtime.*` в значениях; на mobile unresolved → Dropped), §9 (C5 и N5 закрыты, новые
   строки для Dart: сплайс, `options_open`, `text_list`+`options`).
2. Фикстуры корпуса по SPEC §5 (8 кейсов). `expected.json` пишется руками, не `-update`.
   Прогнать `TestContractCorpusTemplate` до правки канона: новые кейсы 17/19 обязаны упасть на текущем
   каноне (вложение вместо сплайса), это подтверждает, что фикстура ловит расхождение.
3. DECISIONS: D-123…D-127 для R2–R6.
4. VERSION 1.1.68, строка changelog в `contract/README.md`, TASKS_LXBOX §64 = SPEC §7 дословно,
   `go generate ./contract/...` (иначе contract.yml падает). Ссылки на код в реестре — `путь.go:Имя`, без
   номеров строк. Поле `go` у кодов шаблона в `warnings.json` можно честно переписать только после волны 2:
   страж `TestRegistryWarningCodesAreActuallySet` требует, чтобы код реально ставился.

## Волна 1. Канон: сплайс, предупреждения с параметрами, `@runtime.*` в значениях

Файлы: `core/template/substitute_canon.go`, `core/template/substitute.go` (только общий движок предикатов и
`wantsIntCast`), `core/template/contract_template_test.go`.

1. `substituteWalkCanon`: ветка массива в позиции условного элемента вливается всегда; `isSpliceable` удалить.
2. `canonCtx`: вместо `[]string` кодов накопитель `[]TemplateWarning{Code string; Params map[string]string}`,
   дедуп по (код, отсортированные параметры). Точки постановки: `replacementCanon` (undeclared: `name`),
   `intCastCanon` (clamped/invalid: `name`, `value`), `handleIfMapSpreadCanon`/`selectIfBranchCanon`/обход
   директив (unknown_directive: `key`).
3. Публичная точка входа `SubstituteVarsInJSONCanonWarnings(...) (json.RawMessage, []TemplateWarning)`;
   старый `SubstituteVarsInJSONCanon` остаётся тонкой обёрткой для корпуса (коды без параметров).
4. `replacementCanon`: `@runtime.*` уже подставляется; убедиться, что неизвестное поле даёт undeclared с `name`.
5. Тест корпуса: `warnings` сравниваются по кодам (формат корпуса не меняется), параметры проверяет
   отдельный Go-тест на трёх кейсах (`int_invalid`, `undeclared`, `unknown_directive`).

## Волна 2. Главный конфиг и `on_change.set` на канон, канал в отчёт

Файлы: `core/template/loader.go` (`ApplyTemplateWithVarsFor`, `GetEffectiveConfigFor`),
`core/template/substitute.go` (`EvalIfScalar`), `core/template/template_validate.go` (Т16),
`core/build/build.go` (`effectiveConfig`, `Result`), `core/config/build_report.go` (вид `template_degraded`),
`core/build_report_feed.go` (`FeedBuildReportFromTemplate`), `core/rebuild.go`,
`ui/configurator/business/create_config.go`, `ui/configurator/tabs/final_report_model.go` (порядок и рендер),
`ui/configurator/business/template_helpers.go` (превью: предупреждения игнорируются).

1. `GetEffectiveConfigFor` возвращает `[]TemplateWarning` дополнительно (или вариант `…WithWarnings`, чтобы
   не трогать все вызовы превью).
2. `build.Result.TemplateWarnings []TemplateWarning` рядом с `ExcludedSources`.
3. `BuildReportTemplateDegraded BuildReportKind = "template_degraded"`; в `finalReportLines` порядок 0,
   остальные сдвигаются. Subject = `Params["name"]` или `Params["key"]`; текст по коду из реестра.
4. `FeedBuildReportFromTemplate(gen, warnings)` вызывается в обоих местах сборки рядом с Sanitizer.
5. Валидатор: снять отказ по `@runtime.*` в значениях `config` (`template_validate.go:152-160` окрестность),
   оставить запрет bare-формы в предикатах.
6. Страж `walker_parity_test.go`: корпус через `ApplyTemplateWithVarsFor`, `knownWalkerDivergences` удалить.
   Ожидание: на этом шаге страж зелёный без исключений.

## Волна 3. Пресеты и DNS-серверы на канон

Файлы: `core/build/preset_expand.go` (`substitutePresetBody`, `presetVarsToTemplateVarsWithExtras`, вызовы
:188 :229 :285 :606 и dns-rules), `core/build/resolve_dns.go` (`substituteTemplateDNSServer`,
`dropUnsetServerPlaceholders` удалить), `core/build/resolve_route.go` (`ExpandWarning` → отчёт),
`core/template/substitute.go` (удалить `SubstituteVarsInJSON`, `…Strict`, `UnresolvedVarError`, legacy walker).

1. `substitutePresetBody` получает `td.Vars` целиком: объявления = все переменные шаблона + переменные пресета
   (переменная пресета при совпадении имени побеждает, как сегодня). `varsMap` по-прежнему источник значений.
2. Гейты валидности после Dropped (SPEC Т3) в одном месте, рядом с `isRuleEmpty`: `isRuleUnusable`,
   `isDNSRuleUnusable`, `isRuleSetUnusable`, `isDNSServerUnusable`. Выпадение фрагмента даёт `ExpandWarning`
   с кодом реестра (проверить наличие подходящего кода в `warnings.json`, иначе новый код в контракт этой же
   версии) и запись `template_degraded`.
3. Предупреждения подстановки пресета и DNS собираются в тот же `Result.TemplateWarnings`.
4. Удалить legacy-обходчик и Strict целиком; `go build ./...`; корпус и страж по имени.
5. Прогон наборов из RECON §2 (главный конфиг × 6 таргетов × 6 состояний, пресеты × 3 × 3 × 3): `config.json`
   побайтно равен сборке до задачи. Скрипт разведки в `RECON.md` §2 воспроизводится Go-тестом в
   `core/build` (один интеграционный тест на волну по политике репо), фиксирует golden от текущего develop.

## Волна 4. `options` ортогонально типу, снятие зашитых имён

Файлы: `core/template/vars_resolve.go` (снять `v.Type = "enum"`, `options_open`), `core/template/types.go`
(поле `OptionsOpen`), `core/template/template_validate.go` (`bool` + `options` → reject),
`ui/configurator/tabs/settings_tab.go` (рендер по `options`/`options_open`, а не по `enum`; `int` с
`options_open` проверяет число), `ui/wizard/**` где читается `OptionTitles`, `bin/wizard_template.json`
(Т13), `core/template/substitute.go` (`isIntCastVar`, `wantsIntCast` → только по типу),
`core/config/varsubst.go` (`intCastVarNames` удалить, тип из объявления + clamp), `bin/locale/ru.json`
(если появятся строки).

1. Резолвер: объектные `options` не меняют тип; `enum` нормализуется в `text` + closed options на загрузке
   (`canonicalVarType`), остальной код видит только `text`/`int`/`text_list` с `Options`/`OptionsOpen`.
2. UI: выпадающий список при `len(Options) > 0`; комбобокс при `OptionsOpen`; `text_list` + `options` пока
   многострочное поле (143.1).
3. Шаблон: три объявления по Т13; `mixed_listen_port` нигде не остаётся.
4. `wantsIntCast` = `type == int`; `isIntCastVar` удалить. `varsubst.go`: `coerceVarValue` по типу из
   `loadTemplateVarDefaults`-подобного чтения плюс clamp [0, 65535] как в каноне.
5. Проверка: `config.json` на штатном шаблоне не изменился (тест волны 3); Settings показывает те же контролы.

## Волна 5. Закрытие

`docs/release_notes/upcoming.md` (EN + RU), `docs/ARCHITECTURE.md` (потоки: один обходчик, канал
предупреждений шаблона в отчёт), `RELEASE_NOTES.md` (строки в «Итоге» при мусоре в настройках),
`IMPLEMENTATION_REPORT.md`, переименование папки в `…-C-…`, CI `gh workflow run ci.yml --ref develop -f run_mode=tests`.

## Правила кампании, которые здесь особенно важны

- Тихих отбраковок нет: каждый гейт волны 3 ставит код из `warnings.json` с параметрами, и код доходит до
  «Итога». Если подходящего кода нет, он добавляется в контракт той же версии, а не логом.
- Локальные копии правил (`isIntCastVar`, `intCastVarNames`, `isSpliceable`, `knownWalkerDivergences`,
  `dropUnsetServerPlaceholders`) удаляются вместе с тестами, которые проверяли именно их.
- Каждая волна — свой агент на Opus в своём worktree (`git worktree add .claude/worktrees/<имя> -b <имя> develop`),
  после волны `git -C <основная> merge --ff-only <имя>` и удаление worktree; develop пушит только оркестратор.
  Волны 0 и 1 независимы и идут параллельно; 2 после 1; 3 после 2; 4 после 3 (тест волны 3 её страхует).
- Локально: `go build ./...`, свой тест по имени, `l10n_check --strict`. CI после каждой волны, ожидание
  отдельным агентом на Sonnet через `gh run watch --exit-status`.

## Риски

- **Пресеты после Dropped.** Самый рискованный шаг. Проверять на всех 16 пресетах штатного шаблона с
  выключенными глобалями (`use_dns_override=false`, пустой `resolve_strategy`, выключенные sniff/resolve).
- **Порядок «Итога».** Сдвиг индексов у восьми существующих видов; проверить, что снапшот-тесты на порядок
  (если есть) обновлены осознанно.
- **Win7 (go1.20).** Никаких `slices`/`maps`/`min`/`max` в новом коде канона и отчёта.
- **Второй обходчик в `varsubst.go`.** Это отдельный движок для `parser_config.outbounds[].options`; его не
  переводим на канон (другая модель данных), только снимаем список имён и берём тип из объявления.
