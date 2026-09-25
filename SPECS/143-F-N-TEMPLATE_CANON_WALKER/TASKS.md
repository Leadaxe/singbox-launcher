# TASKS 143

Порядок волн по PLAN.md. Каждая волна: `go build ./...` + только свой именованный тест; полный прогон в CI
после волны 5.

## Волна 0. Норматив и корпус (контракт 1.1.68)

- [ ] TEMPLATE_LANG §2.1: `options` при любом типе кроме `bool`, объектная форма не меняет тип, флаг `options_open`
- [ ] TEMPLATE_LANG §2.2: `enum` = `text` + закрытые `options`; снять фразу про fallback-список имён
- [ ] TEMPLATE_LANG §3: коллапс `["@name"]` запрещён, скобки автора сохраняются
- [ ] TEMPLATE_LANG §4.4: сплайс на один уровень, двойные скобки, примеры из SPEC §3.2
- [ ] TEMPLATE_LANG §7.2: `@runtime.*` в позиции значения; на mobile unresolved → Dropped
- [ ] TEMPLATE_LANG §9: C5 и N5 закрыты; строки для Dart: сплайс литерала, `options_open`, `text_list`+`options`
- [ ] Фикстуры корпуса (SPEC §5, 8 кейсов), `expected.json` руками; кейсы сплайса падают на текущем каноне
- [ ] `contract/registry/vars.json`: снять пометки C5 у `urltest_tolerance`, `tun_mtu`, `proxy_in_listen_port`
- [ ] `contract/registry/warnings.json`: поле `go` у четырёх кодов шаблона переписать на точки постановки после волны 2
- [ ] DECISIONS D-123…D-127 (R2–R6)
- [ ] `contract/VERSION` 1.1.68, changelog, `TASKS_LXBOX.md` §64 = SPEC §7

## Волна 1. Канон

- [ ] Сплайс любого массива в ветке условного элемента; удалить `isSpliceable`
- [ ] `TemplateWarning{Code, Params}`, дедуп по (код, параметры); параметры во всех четырёх точках постановки
- [ ] `SubstituteVarsInJSONCanonWarnings`; `SubstituteVarsInJSONCanon` — обёртка для корпуса
- [ ] Go-тест параметров предупреждений на трёх кейсах (один файл)
- [ ] `TestContractCorpusTemplate` зелёный с новыми фикстурами

## Волна 2. Главный конфиг, `on_change.set`, отчёт

- [ ] `GetEffectiveConfigFor`/`ApplyTemplateWithVarsFor` возвращают предупреждения
- [ ] `EvalIfScalar` на канонический walker
- [ ] Валидатор: `@runtime.*` в значениях `config` разрешён; bare-форма в предикатах по-прежнему запрещена
- [ ] `build.Result.TemplateWarnings`
- [ ] `BuildReportTemplateDegraded`, порядок 0 в `finalReportLines`, рендер Subject из `name`/`key`
- [ ] `FeedBuildReportFromTemplate` в `core/rebuild.go` и `create_config.go`
- [ ] Страж корпуса через боевой путь, `knownWalkerDivergences` удалён

## Волна 3. Пресеты и DNS

- [ ] `substitutePresetBody` объявляет все `td.Vars` + переменные пресета
- [ ] Гейты валидности после Dropped: rule, dns_rule, rule_set, dns_server; выпадение → `ExpandWarning` + отчёт
- [ ] `substituteTemplateDNSServer` на канон; `dropUnsetServerPlaceholders` удалён
- [ ] Удалены `SubstituteVarsInJSON`, `SubstituteVarsInJSONStrict`, `UnresolvedVarError`, legacy walker
- [ ] Интеграционный тест «config.json на штатном шаблоне не изменился» (наборы RECON §2, golden от develop)

## Волна 4. `options` и зашитые имена

- [ ] `vars_resolve.go`: объектные `options` не меняют тип; `OptionsOpen`; `enum` → `text` + closed на загрузке
- [ ] Валидатор: `bool` + `options` → reject
- [ ] Settings: список по `Options`, комбобокс по `OptionsOpen`, `int` + `OptionsOpen` проверяет число
- [ ] `bin/wizard_template.json`: `tun_mtu`, `proxy_in_listen_port` → `int`; `urltest_tolerance` → `int` + options
- [ ] `wantsIntCast` только по типу; `isIntCastVar` удалён
- [ ] `core/config/varsubst.go`: `intCastVarNames` удалён, тип из объявления + clamp
- [ ] Строки `mixed_listen_port` в репозитории нет
- [ ] Тест волны 3 по-прежнему зелёный

## Волна 5. Закрытие

- [ ] `docs/release_notes/upcoming.md` (EN + RU)
- [ ] `docs/ARCHITECTURE.md`: один обходчик, канал предупреждений шаблона
- [ ] `RELEASE_NOTES.md`
- [ ] `IMPLEMENTATION_REPORT.md`
- [ ] CI: `gh workflow run ci.yml --ref develop -f run_mode=tests`
- [ ] Папка → `143-F-C-TEMPLATE_CANON_WALKER`

## Подзадача 143.1 (вне этой задачи)

- [ ] UI множественного выбора для `text_list` + `options`
- [ ] Числовая валидация поля `int` в Settings
