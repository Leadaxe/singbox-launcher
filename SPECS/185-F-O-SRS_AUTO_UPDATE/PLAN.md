# PLAN 185

## Этап 1 — режим local

| Файл | Изменение |
|---|---|
| `core/services/srs_downloader.go` | `RefreshSRS` (условный GET, 304 → touch), проверка содержимого в общем теле загрузки (`DownloadSRS` тоже), `SRSFileModTime` |
| `core/build/rule_set_policy.go` (новый) | `RuleSetPolicy`, резолв правило → переменная → умолчание; `PresetMergeContext.RuleSetFetches()` — список (URL, файл, интервал) local-наборов включённых правил; `DropUncachedSrsPaths` |
| `core/srs_refresh.go` (новый) | `runScheduledSRSRefresh`, `ensureMissingRuleSets`, пауза после сбоя, повтор 15 с и по VPN-событию, `configReferencesMissingRuleSet`, `RefreshMachineRuleSets` |
| `core/auto_update.go` | вызовы цикла и VPN-события |
| `core/rebuild.go` | докачка отсутствующих перед сборкой; pre-start: принудительная сборка при отсутствующем файле |
| `core/config_service_context.go` | `DropUncachedSrsPaths` для локальной сборки |
| `core/services/lxd_remote_deploy.go` | хук `PreDeploy` перед сбором ресурсов |
| `core/controller.go` | проводка хука |

## Этап 2 — режим remote и настройки

| Файл | Изменение |
|---|---|
| `core/state/rule_types.go`, `record_clone.go` | поля записи `srs_mode`, `update_interval_hours`, `download_detour`; `RuleSetOptions` вид |
| `core/state/load_v6.go`, `ui/configurator/models/*` | провоз полей state ↔ модель визарда (`RuleState`, `PresetRefState`) |
| `core/build/resolve_route.go`, `preset_merge.go` | ветка эмиссии remote для пресета и `kind: srs`; проверка `download_detour` по `FinalOutboundTags` |
| `core/build/build.go` | `ctx.Preset.FinalOutboundTags` до подсчёта эмитируемых тегов |
| `ui/configurator/business/root_name_refs.go` | 8-й вид ссылки — `download_detour` правила |
| `ui/configurator/dialogs/add_rule_dialog.go`, `tabs/preset_ref_edit_dialog.go`, `dialogs/rule_set_options.go` (новый) | блок «Rule-set updates» |
| `ui/configurator/tabs/rules_tab.go`, `rules_unified_rows.go` | дата загрузки в подсказке; кнопка srs не требует файла у remote |
| `bin/wizard_template.json` | три переменные (раздел `general`) |
| `bin/locale/ru.json` | переводы |
| `core/backup/file_keys_10.go`, `lxbox_fields_test.go`, `portable_vars`/`registry/vars.json` | поле общее; переменные |
| `contract/**` | VERSION, README changelog, BACKUP.md, schema, корпус, TASKS_LXBOX |

## Тест (один, data-критичный)

`core/services`: `TestRefreshSRS` — истёкший файл заменяется; HTML-ответ
прежний файл не портит; 304 продлевает свежесть. Плюс раннеры контракта по
имени (корпус бэкапа, sync переменных).

## Проверка на живом ядре

fswatch и замена через rename — проверяет владелец на своей сборке: подменить
файл, в логе ядра ждать перечитывание набора (сам sing-box агент не запускает).
