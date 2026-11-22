# PLAN 187

> Реализация упростила план в четырёх точках — см. IMPLEMENTATION_REPORT.md, «Отступления». Главное: тег `.json`-набора несёт расширение, поэтому имя копии и формат выводятся из тега (`srstag.FileName` / `srstag.Format`), а поля `Format` у `RuleSetFetch` и смены сигнатур путей нет.

## Общие примитивы (одно место на всё)

| Файл | Изменение |
|---|---|
| `internal/srstag/srstag.go` | `FormatFromRef(ref)` → `binary`/`source`; `FileName(ref)` → `<тег>.<расширение>`; `TagFromURL` срезает и хвост `.json`; `FileRefFromPath(path)` / `PathFromFileRef(ref)` — строковая логика без `runtime.GOOS` (буква диска, слэши, `%20`), UNC → ошибка; `IsFileRef(ref)` |

Win7/go1.20: только `net/url`, `strings`, `path`; без `slices`/`min`/`max`.

## Загрузчик и цикл

| Файл | Изменение |
|---|---|
| `core/services/srs_downloader.go` | `fetchSRS`: ветка `file://` — открыть исходник вместо HTTP, общий хвост (tmp → проверка → rename), `Chtimes` копии во время исходника; «не изменился» = равны размер и mtime; `validateSRSFile(path, format)`; экспорт проверки для диалога; пути копий через `srstag.FileName`; `DeleteOrphanRuleSetsFor` — имя файла без расширения = тег, для `.srs` и `.json` |
| `internal/platform/platform_common.go` | `GetRuleSetPath` принимает имя файла (или парная функция по ref) |
| `core/build/rule_set_policy.go` | `RuleSetFetch`: поле `FileTag` → имя файла с расширением (`FileName(ref)`), новое поле `Format` (у пресета — явный `format` шаблона, иначе по расширению) — им же проверяется содержимое; `file://` — «срок истёк» всегда (кроме интервала 0); режим remote решается **по ссылке**: в правиле с Mode=Remote `https://`-ссылки уходят remote, `file://` остаются local и попадают в список загрузок |
| `core/srs_refresh.go` | путь копии по имени из `RuleSetFetch` (сейчас `FileTag+".srs"`); пропавший исходник при живой копии — предупреждение, не сбой с паузой |

## Сборка

| Файл | Изменение |
|---|---|
| `core/build/resolve_route.go` | `format` из `srstag.FormatFromRef` (local и remote ветки `kind: srs`); remote-режим не применяется к `file://` |
| `core/build/preset_merge.go` | `convertPresetRuleSetRemoteToLocal`, `CollectSrsCachedPaths`: имя файла по ref; умолчание `format` по расширению, явный `format` пресета сильнее |
| `core/build/route_merge.go` | `ResourceNameForSRS` → имя по ref; `format` по расширению |

`CollectDeployResources` не меняется (имя берётся из `path`).

Все места, где имя копии собирается как `тег + ".srs"` (на 22.11 их восемь: `srs_downloader.go` ×4, `srs_refresh.go`, `rules_tab.go`, `platform_common.go`, `route_merge.go`), переходят на `srstag.FileName`; после правки `grep '+".srs"'` по коду пуст.

Время изменения копии `file://`-набора равно времени исходника, а не моменту загрузки: подсказка кнопки для таких наборов показывает «файл изменён», интервал SPEC 185 к ним не применяется, поэтому расхождения с её учётом свежести нет.

## UI

| Файл | Изменение |
|---|---|
| `ui/configurator/dialogs/add_rule_dialog.go` | подписи типа и поля; кнопка «Add file…» (`dialog.NewFileOpen`, фильтр `.srs`/`.json`); нормализация голого пути в `file://`; проверка файла при сохранении; `format` в `buildSRSRuleSetsAndTags` по расширению |
| `ui/configurator/tabs/rules_tab.go` | `srs` → `upd` в трёх подписях кнопки; подсказка о пропавшем исходнике; путь копии в `srsLastUpdatedHint` по имени от ссылки (сейчас `e.Tag+".srs"`) |
| `ui/configurator/dialogs/rule_set_options.go` | Mode для правила из одних `file://` — недоступен |
| `bin/locale/ru.json` | переводы новых и переименованных ключей |

## Контракт и доки

| Файл | Изменение |
|---|---|
| `contract/VERSION`, `contract/README.md` | 1.1.178, строка changelog |
| `contract/docs/BACKUP.md`, `contract/schema/backup.schema.json` | `refs[]`: `file://`, формат по расширению |
| `contract/registry/backup_warnings.json` | `backup_rule_ref_local_file` |
| `contract/corpus/backup/` | кейс `srs_file_ref` (backup + expected) |
| `contract/TASKS_LXBOX.md` | параграф: формат по расширению, ввоз `file://` выключенным |
| `docs/ARCHITECTURE.md`, `docs/WIZARD_STATE.md`, `docs/release_notes/upcoming.md` | по факту |

## Тест (один, data-критичный)

`core/services`: `TestRefreshSRSFileRef` — копия создаётся; изменённый
исходник заменяет копию; неизменённый не трогает; пропавший исходник копию не
портит; JSON под именем `.srs`-ссылки отвергается проверкой формата. Плюс
раннер корпуса бэкапа по имени.

## Проверка владельцем

Диалог и кнопка — на своей сборке; подхват правки файла работающим ядром —
там же (sing-box агент не запускает).
