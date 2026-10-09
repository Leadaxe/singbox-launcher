# PLAN 160

## Ограничение, задающее архитектуру

`core/debugapi` не может импортировать `ui/configurator/business` (цикл: business → core → debugapi), а UI-хелперы работают с `WizardModel`, не со `state.State`. Поэтому операции над состоянием живут в новом core-пакете **`core/stateedit`** (зависимости: `core/state`, `core/config`, `core/config/subscription`, `core/build`, `core/template`; НЕ `core/backup` и НЕ `ui/*`). Debug API — тонкие обработчики над ним. Конфигуратор на этот пакет в этой волне не переводится (его путь остаётся на модели), но функции пишутся так, чтобы это было возможно.

## Карта повторного использования (из разведки, файл:строка на 09.10.2026)

| Задача | Что брать | Где |
|---|---|---|
| ссылка/JSON → узел | `config.MaterializeServerNode(uri, configJSON)`; `config.MaterializeVPNLinkNodes(link)` | `core/config/migrate_materialize.go:429`, `:497` |
| классификация строк | `subscription.IsDirectLink`, `IsSubscriptionURL`, `IsAmneziaVPNLink`, `ExtractWGConfBlocks`/`ConvertWGConfText`, `ClassifySubscriptionBody`, `NodeFromManualConfigJSON` | `core/config/subscription/{node_parser_core.go:28, source_loader.go:134, node_parser_amnezia.go:322, wgconf_text.go:25/63, manual_config.go:36}` |
| образец разбора ввода (UI) | `parseSourceInput` — читать как спецификацию веток, не импортировать | `ui/configurator/business/source_input.go:78` |
| запись источника | `state.NewServerSource`, `state.Source`, `state.Node`, `state.Origin{Kind,Raw}` | `core/state/sources_v7.go:511, :360, :199, :48` |
| занятые корневые имена и суффикс `-N` | `takenRootTags`, `uniqueTag` — **перенести в `core/state`** как `TakenRootTags(s)`, `UniqueTag(taken, tag)`; `core/backup/merge.go` переключить на них | `core/backup/merge.go:338, :384` |
| полный набор известных целей | как `knownOutboundsFor` + узлы/цепочки/системные теги (образец — `importKnownTags`) | `core/debugapi/backup_endpoints.go:365`, `core/backup/import.go:622` |
| ссылки на узел в источниках | форма `NodeLink{FolderID, Tag}`: `Detour`, `Hops`, `Group.Members/Default`; образец обхода — `editNodeLinks` (UI, порт на `[]state.Source`) | `ui/configurator/business/node_move.go:516` |
| ссылки по имени (только отчёт) | `Rules[].Body.outbound`, `Vars["route_final"]`, `Directions[].AddOutbounds`, `DNS.Servers[].Body["detour"]`, `DNS.Servers[].Vars`, `Rules[].Vars` | образец `editRootNameRefs` `ui/configurator/business/root_name_refs.go:89` |
| каталог Tailscale | `config.RemoveTailscaleStateDir(name)`, `config.TailscaleStateDirName(finalTag)` | `core/config/tailscale_state_dir.go:95, :55` |
| правила | `state.Rule`, `DecodeBody`, `NewInlineRule/NewSrsRule/NewPresetRule`, `NextUserRuleNum`, `SortRulesByNum`, `IsSortable`/голова оси | `core/state/rule_types.go`, `rule_order.go:267, :118` |
| синк пресетов | `state.SyncDNSOptionsWithActivePresets(rules, &dns, template.PresetLiteMap(td.Presets))`; `build.SyncOutboundsWithTemplate(rules, &st.Directions, td.Presets, build.TemplateOutboundTags(td), build.TargetSpecFromState(st))` | `core/state/sync_dns.go:51`, `core/build/sync_outbounds.go:66` |
| DNS | `state.DNSServer/DNSRule/DNSOptions`, `FindServerByTag/ByRef`, `FindRuleByRef`; `dns_final`/`dns_default_domain_resolver` в `state.Vars`; теги шаблона — разбор `td.DNSOptionsRaw` (образец `ExtractTemplateDNSTags`, `ui/configurator/business/dns_helpers.go:42`) | `core/state/dns_options.go` |
| нормы записи | `state.ApplyRecordVars(st, decls)`; `recordVarDeclsFor` | `core/debugapi/backup_endpoints.go:114` |
| API-конвенции | `stateAccess`, `guardStateSchema`, `writeJSON`, `decodeJSONBody`, реестр `apiEndpoint`, зеркала `remote_state_endpoints.go` | `core/debugapi/state_endpoints.go`, `server.go:244`, `remote_endpoints.go:62` |
| тесты API | `fakeFacade`, `newTestServer`, `authedReq`, `doJSON`; `TestHelpMethodsMatchHandlers` | `core/debugapi/server_test.go:18`, `traffic_endpoints_test.go:29`, `dns_patch_guard_test.go:69` |

## Файлы

**Новые**
- `core/stateedit/servers.go` — `ParseServerInput`, `AddServers`, `DeleteServer`, типы результатов.
- `core/stateedit/rules.go` — `AddRule`, `DeleteRule`, `KnownRuleTargets`.
- `core/stateedit/dns.go` — `AddDNSServer`, `DeleteDNSServer`, `AddDNSRule`, `DeleteDNSRule`.
- `core/stateedit/stateedit_test.go` — один интеграционный тест: сценарий инцидента (узел добавлен — правила/DNS целы) + удаление с чисткой ссылок.
- `core/debugapi/state_crud_endpoints.go` — обработчики B1–B4 (local + `…With(acc)`), регистрация в `endpoints()` и зеркала в `remoteEndpoints()`.
- `core/debugapi/state_crud_endpoints_test.go` — API-тест по `fakeFacade` (один файл).
- `core/debugapi/mutation_log.go` — middleware журнала (раздел C).

**Изменяемые**
- `core/state/tags.go` (новый) ← перенос `TakenRootTags`/`UniqueTag`; `core/backup/merge.go` — вызовы.
- `core/debugapi/state_endpoints.go` — гард `PATCH /state/dns` (сделано), `guardStateSchema`+`ApplyRecordVars` у `PATCH /state/dns/rules`.
- `core/debugapi/server.go` — строки реестра, обёртка middleware в `routes()`.
- `core/debugapi/remote_endpoints.go`, `remote_state_endpoints.go` — зеркала.
- `core/debugapi_wiring.go` — если фасаду нужен `RebuildConfigIfDirty` (уже есть) и шаблон (`LoadTemplate` есть).
- `docs/API.md`, `docs/API.ru.md`, `docs/ARCHITECTURE.md`, `docs/release_notes/upcoming.md`, `SPECS/README.md`.
- Раздел A — уже изменённые файлы: `core/backup/{types,backup10,file,decoded,import,import10,legacy_read_0x}.go`, `merge_test.go`; `contract/{VERSION,README.md,TASKS_LXBOX.md,registry/backup_warnings.json,schema/backup.schema.json,docs/BACKUP.md,docs/generated/index.md}`; `ui/configurator/tabs/settings_backup.go`; `bin/locale/ru.json`; `core/debugapi/backup_endpoints.go`.

## Ловушки

- Win7 = go1.20: без `min`/`max`, `slices`, `maps`, `http.PathValue`; идентификаторы — query, не `{}` в пути.
- `state.Rules` всегда отсортирован по `Num`; `NextUserRuleNum` клампится на 1100 (дубли номеров допустимы).
- Preset-правило без тела; `DecodeBody` цель не проверяет.
- `state.Node.Label` — `json:"-"`, в файл не идёт; тег узла — `Tag`.
- `takenRootTags` уже ставит `-auto`-двойники у свёрток, но не у Направлений и не системные теги шаблона — при переносе расширить до полноты UI-версии (`KnownRuleTargetTags`) ровно настолько, насколько доступно из состояния + шаблона.
- `decodeJSONBody` — `DisallowUnknownFields`: тело `state.Rule` с лишним ключом даст 400, это норма.
- Логи: без секретов тела запроса; ответ ограничить ~400 символами и схлопнуть переводы строк.
- Новые ключи `locale.T` не ожидаются (API без UI-строк); если появятся — сразу в `bin/locale/ru.json`.

## Раздел E — попап в окне Конфигуратора

| Что | Где |
|---|---|
| payload события | `core/events/payloads.go` — `StateChangedPayload` + поля `Source`, `Target`, `MachineID` |
| публикация из API | `core/debugapi/state_endpoints.go` `localStateAccess` / `remote_state_endpoints.go` `machineStateAccess` — обёртка `save`: после успешной записи `facade.NotifyStateChanged(target, machineID)`; метод в `ControllerFacade` (`server.go`), реализация в `core/debugapi_wiring.go` (публикует в `ac.EventBus`), no-op в `fakeFacade` (`server_test.go`) |
| подписка окна | `ui/configurator/configurator.go` `buildWizardWindow`: `ac.EventBus.Subscribe(events.StateChanged, …)` → `Cancel` в `SetOnClosed` |
| фильтр адресата | `presenter.ConfigTarget()`, `presenter.ConfigMachineID()` (`presenter_target.go`) |
| попап и перечитывание | образец перечитывания — `loadStateFromRead` (`configurator.go:879`); confirm — `dialog.NewCustomConfirm`/`ShowConfirm` как в `configurator.go:851`; UI-поток — `fyne.Do` (образец `ui/machine_list_panel.go:487`) |
