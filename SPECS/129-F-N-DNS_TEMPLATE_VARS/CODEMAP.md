# SPEC 129 · CODEMAP — значения переменных записи

Карта мест кода после волны лаунчера. Адреса — `файл:строка` на ветке
`feat/spec129-record-vars`; правка кода — правка карты тем же куском.

## 1. Реестр типов и функций

| Что | Где | Роль |
|---|---|---|
| `TemplateData.DNSServerVars` | `core/template/loader.go:92` | объявления переменных шаблонных DNS-серверов по тегу, локальные имена |
| `NormalizeDNSOptions` | `core/template/dns_server_form.go:47` | вложенная запись → плоская; возвращает объявления, плейсхолдеры не переписывает |
| `validateDNSServerVars` | `core/template/dns_server_form.go:261`, вызов `loader.go:306` | лексика/дубли/if/#if объявлений сервера; Н11 — `@name` тела объявлен сервером или шаблоном |
| `DNSServerVarScope` | `core/template/dns_server_form.go:154` | глобальные переменные шаблона + объявления сервера (локальное затеняет) |
| `ResolveDNSServerVars` | `core/template/dns_server_form.go:188` | значения для тела: запись → умолчание для цели; чужое имя записи не видно |
| `DNSServerVarValues` | `core/template/dns_server_form.go:214` | те же значения строками — подпись строки DNS и форма окна |
| `DNSServerVarDefault` | `core/template/dns_server_form.go:235` | умолчание переменной сервера для цели — хранилище окна, root_name_refs |
| `RecordVarDeclsFor` | `core/template/record_var_decls.go:22` | мост шаблон → `state.RecordVarDecls` (умолчания для цели, `enabled` по умолчанию) |
| `PresetRecordVarDecls` | `core/template/record_var_decls.go:73` | объявления одного пресета (`Ref` у ref-переменной) |
| `DNSServer.Vars` | `core/state/dns_options.go:99` | поле записи `vars,omitempty`, только `kind: template` |
| `CloneDNSServer` | `core/state/node_sections.go:153` | копия с `Vars`; `NodeSections.Clone` — через неё (`:132`) |
| миграция v7→v8 | `core/state/migration_v7_to_v8.go:376` | `vars` плоской template-записи провозятся |
| `RecordVarDecl` / `RecordVarDecls` | `core/state/record_vars.go:37`, `:52` | объявления без типов шаблона |
| `MatchRootDNSVar` | `core/state/record_vars.go:120` | Н8: кандидаты — объявленные пары, самый длинный тег |
| `ApplyRecordVars` | `core/state/record_vars.go:162` | своё хранение: Н8 (запись создаётся) + Н2–Н4 обоих носителей + сироты |
| `MoveRootDNSVars` / `…Map` | `core/state/record_vars.go:217`, `:279` | Н8 со снимком «у записи свои vars»; файл — `superseded`/`no_record` |
| `NormalizeDNSServerVars` | `core/state/record_vars.go:312` | Н2–Н4 записей серверов, снятие сирот |
| `NormalizePresetRuleVars` | `core/state/record_vars.go:360` | Н2–Н4 `rules[kind=preset].vars` |
| `NormalizeRecordVarMap` | `core/state/record_vars.go:389` | одна карта против объявлений носителя |
| `UndeclaredRecordVars` | `core/state/record_vars.go:443` | необъявленные имена записи файла без правки (отчёт импорта) |

## 2. Сборка

| Что | Где |
|---|---|
| подстановка тела шаблонного сервера по записи | `core/build/resolve_dns.go:191` (вызов), `:566` `substituteTemplateDNSServer`, `:607` `dropUnsetServerPlaceholders`, `:447` `stateTemplateVars` |
| объявления в контексте слияния | `core/build/preset_merge.go:193` `PresetMergeContext.DNSServerVars`, `:558` `templateLikeFromCtx` |
| перенос в памяти при сборке из файла | `core/config_service_context.go:51` → `:104` `stateWithRecordVars`; `DNSServerVars` — `:96` |
| превью / «Итог» / remote | `ui/configurator/business/create_config.go:163` (`ApplyDNSTemplateVarsToState`), `:200` |
| вторая линия fail-closed (Н10) | `core/build/dns_detour_sanitize.go:84` `sanitizeDNSSection`, `:166` `repairAfterServerDrop`, `:278` reject-правила, `:301` выбор резолвера, `:333` резолвер DNS-сервера, `:352`/`:406`/`:453` резолверы route и узлов |
| dns собирается первой, итог линии — секциям | `core/build/build.go:103` поле `dnsFailClosed`, `:287` пред-сборка, `:317` `buildDNSSection`, `:379`–`:397` outbounds/endpoints, `:431` route |

## 3. Бэкап и debug API

| Что | Где |
|---|---|
| `Warning.Record`, `WarnBackupDNSEntrySkipped`, `VarSkippedNotPortable` | `core/backup/import.go:64`, `:97`, `:1072` |
| `ImportOptions.RecordVars` | `core/backup/import.go:272` |
| нормы своего хранения приёмника до слияния | `core/backup/import.go:391` |
| Н2/Н4 пресетов после замены `rules[]` | `core/backup/import.go:496` |
| `importDNS` §5.2 + Н9 | `core/backup/import.go:850`, `:972` `overlayRecordVars`, `:995` `checkImportedDNSTargets`, `:1040` `recordVarWarnings` |
| Н8 в файле до слияния | `core/backup/decoded.go:151`; вызовы `import10.go:94`, `legacy_read_0x.go:119` |
| `vars` только у template (Л5) | `core/backup/import10.go:641`, `node_sections.go:173`; ключи — `file_keys_10.go:82`, `:134` |
| 0.x `DNSRef.Vars` → запись | `core/backup/legacy_read_0x.go:154` |
| экспорт нормализует копию | `core/backup/export.go:54`, `export10.go:46`, `:146` |
| пять имён сняты из переносимых | `core/backup/portable_vars.go` (комментарий на месте списка) |
| debug API: объявления, поля вида | `core/debugapi/backup_endpoints.go:95`, `:114`, `:241` (экспорт), `:313` (импорт) |
| debug API: PATCH `/state/rules`, `/state/dns` | `core/debugapi/state_endpoints.go:223`, `:308` (вид `vars`), `:335` |

## 4. UI

| Что | Где |
|---|---|
| модель `DNSTemplateVars` | `ui/configurator/models/wizard_model.go:135` |
| state ↔ модель | `ui/configurator/models/preset_ref_sync.go:700`, `:724` |
| загрузка: нормы ДО фильтра сирот (Л1) | `ui/configurator/presentation/presenter_state.go:349`; модель — `presenter_state_helpers.go:103` |
| сохранение | `presenter_state.go:173` (`vars` в записи), `:237` (`ApplyRecordVars`), `:246` объявления |
| импорт из UI | `presenter_backup_import.go:93`; экспорт — `tabs/settings_backup.go:146`; тексты — `:370` |
| строка переменной над хранилищем (Л3/Л4) | `tabs/settings_tab.go:534` `varRowStore`, `:557` Settings, `:592` `buildVarRow`; хранилище записи — `tabs/dns_template_vars.go:53`, строки окна — `:141` |
| подпись строки и форма по записи | `tabs/dns_tab.go:519` `dnsVarValuesFor` |
| пресеты: сброс умолчанием, общая норма | `tabs/rules_unified_rows.go:256`, `tabs/preset_ref_edit_dialog.go:358` |
| D-113/D-114 пункт 7 | `business/root_name_refs.go:214`, `:227` `editDNSTemplateVarRefs` |

## 5. Связи конвейеров

```
шаблон ─ NormalizeDNSOptions ─▶ DNSServerVars ─┬─ ResolveDNSServerVars ─▶ ResolveDNS (сборка, превью)
                                               └─ RecordVarDeclsFor ─▶ state.RecordVarDecls
state.json ─ Load ─▶ ApplyRecordVars ◀─ decls ─┬─ сборка из файла (копия в памяти)
                                               ├─ UI: LoadState (до фильтра сирот) и CreateStateFromModel
                                               ├─ экспорт (копия), импорт (приёмник до слияния, файл — Н8/Н2/Н4/Н9)
                                               └─ debug API: PATCH /state/rules, /state/dns
BuildConfig: dns (линия 2 → dnsFailClosed) ─▶ endpoints / outbounds / route (замена резолвера)
```

## 6. Тесты волны (интеграционные)

| Тест | Что ловит |
|---|---|
| `core/spec129_record_vars_test.go` `TestSpec129RecordVarsKeepConfigByteIdentical` | старая форма → config; писатель → Save → config байт-в-байт; записи без умолчаний, сирота-запись создана |
| `core/spec129_record_vars_test.go` `TestSpec129DanglingDNSRouteFailsClosed` | висячий канал: сервер не эмитится, правило → reject, заглушка final, замена резолверов DNS/route/узла, `sing-box check` установленного ядра |
| `ui/configurator/presentation/backup_restore_dns_route_test.go` | экспорт из старой формы → файл без корневых `dns_*`; маршрут и `dns_ip` через debug API, UI новой машины и UI в настроенное (Л1, Л13) |
| корпус `contract/corpus/backup/v10_dns_template_vars` (+ фикстура), `contract/corpus/dns/nested_vars_substituted` | нормы Н2/Н4/Н8/Н9 и §5.2 против фикстуры объявлений; подстановка по `state[].vars` |
