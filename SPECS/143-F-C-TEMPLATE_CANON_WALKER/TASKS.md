# TASKS 143

Порядок волн по PLAN.md. Каждая волна: `go build ./...` + только свой именованный тест; полный прогон в CI
после волны 5.

## Волна 0. Норматив и корпус (контракт 1.1.68)

- [x] TEMPLATE_LANG §2.1: `options` при любом типе кроме `bool`, объектная форма не меняет тип, флаг `options_open`
- [x] TEMPLATE_LANG §2.2: `enum` = `text` + закрытые `options`; снять фразу про fallback-список имён
- [x] TEMPLATE_LANG §3: коллапс `["@name"]` запрещён, скобки автора сохраняются
- [x] TEMPLATE_LANG §4.4: сплайс на один уровень, двойные скобки, примеры из SPEC §3.2
- [x] TEMPLATE_LANG §7.2: `@runtime.*` в позиции значения; на mobile unresolved → Dropped
- [x] TEMPLATE_LANG §9: C5 и N5 закрыты; строки для Dart: сплайс литерала, `options_open`, `text_list`+`options`
- [x] Фикстуры корпуса (SPEC §5, 8 кейсов), `expected.json` руками; кейсы сплайса падают на текущем каноне
- [x] `contract/registry/vars.json`: снять пометки C5 у `urltest_tolerance`, `tun_mtu`, `proxy_in_listen_port`
- [x] `contract/registry/warnings.json`: поле `go` у четырёх кодов шаблона переписать на точки постановки после волны 2 (сделано в волне 2, 1.1.69)
- [x] DECISIONS D-123…D-127 (R2–R6)
- [x] `contract/VERSION` 1.1.68, changelog, `TASKS_LXBOX.md` §64 = SPEC §7

## Волна 1. Канон

- [x] Сплайс любого массива в ветке условного элемента; удалить `isSpliceable`
- [x] `TemplateWarning{Code, Params}`, дедуп по (код, параметры); параметры во всех четырёх точках постановки
- [x] `SubstituteVarsInJSONCanonWarnings`; `SubstituteVarsInJSONCanon` — обёртка для корпуса
- [x] Go-тест параметров предупреждений на трёх кейсах (один файл)
- [x] `TestContractCorpusTemplate` зелёный с новыми фикстурами

## Волна 2. Главный конфиг, `on_change.set`, отчёт

- [x] `GetEffectiveConfigFor`/`ApplyTemplateWithVarsFor` возвращают предупреждения (новые `…ForWarnings`, старые сигнатуры — обёртки)
- [x] `EvalIfScalar` на канонический walker
- [x] Валидатор: `@runtime.*` в значениях `config` разрешён; bare-форма в предикатах по-прежнему запрещена
- [x] `build.Result.TemplateWarnings`
- [x] `BuildReportTemplateDegraded`, порядок 0 в `finalReportLines` (рендер — текст `Reason` без субъекта, чтобы имя не дублировалось)
- [x] `FeedBuildReportFromTemplate` в `core/rebuild.go` и `create_config.go`
- [x] Страж корпуса через боевой путь, `knownWalkerDivergences` удалён

## Волна 3. Пресеты и DNS

- [x] `substitutePresetBody` объявляет все `td.Vars` + переменные пресета
- [x] Гейты валидности после Dropped: rule, dns_rule, rule_set, dns_server (+ правило без условий); выпадение → код `template_fragment_dropped` (1.1.70) + отчёт
- [x] `substituteTemplateDNSServer` на канон; `dropUnsetServerPlaceholders` удалён
- [x] Удалены `SubstituteVarsInJSON`, `SubstituteVarsInJSONStrict`, `UnresolvedVarError`, legacy walker
- [x] Интеграционный тест «config.json на штатном шаблоне не изменился» (наборы RECON §2, golden от develop)

## Волна 4. `options` и зашитые имена

- [x] `vars_resolve.go`: объектные `options` не меняют тип; `OptionsOpen`; `enum` → `text` + closed на загрузке
- [x] Валидатор: `bool` + `options` → reject
- [x] Settings: список по `Options`, комбобокс по `OptionsOpen`, `int` + `OptionsOpen` проверяет число
- [x] `bin/wizard_template.json`: `tun_mtu`, `proxy_in_listen_port` → `int`; `urltest_tolerance` → `int` + options
- [x] `wantsIntCast` только по типу; `isIntCastVar` удалён (фактически `template.IsIntVarType` + `CastIntValue`, `wantsIntCast` удалён)
- [x] `core/config/varsubst.go`: `intCastVarNames` удалён, тип из объявления + clamp
- [x] Строки `mixed_listen_port` в репозитории нет
- [x] Тест волны 3 по-прежнему зелёный

## Волна 5. Закрытие

- [x] `docs/release_notes/upcoming.md` (EN + RU)
- [x] `docs/ARCHITECTURE.md`: один обходчик, канал предупреждений шаблона
- [x] `RELEASE_NOTES.md`
- [x] `IMPLEMENTATION_REPORT.md`
- [x] CI: `gh workflow run ci.yml --ref develop -f run_mode=tests` (волны 0–4 зелёные; прогон после слияния волны 5 — оркестратор)
- [x] Папка → `143-F-C-TEMPLATE_CANON_WALKER` (контракт 1.1.71, TASKS_LXBOX §67)

## Подзадача 143.1 (вне этой задачи)

- [ ] UI множественного выбора для `text_list` + `options`
- [x] Числовая валидация поля `int` в Settings (сделана в волне 4: `numberEntryValidator` в поле и комбобоксе)
