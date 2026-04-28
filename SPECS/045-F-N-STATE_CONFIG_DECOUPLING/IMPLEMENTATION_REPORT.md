# Реализация: 045 — STATE_CONFIG_DECOUPLING

**Статус:** in-progress (cutover phase). Working tree, не закоммичено. Ветка `develop`.

**Дата старта:** 2026-04-26 (после v0.8.7 пререлиза). **Текущая дата:** 2026-04-27.

---

## Сводка по фазам

| Фаза | Описание | Статус |
|---|---|---|
| 0 | LxBox deep-dive (`LXBOX_NOTES.md`) | ✅ |
| 1 | Карта текущей архитектуры лаунчера (`CURRENT_ARCH_NOTES.md`) | ✅ |
| 2 | Финализация PLAN.md / TASKS.md | ✅ |
| 3.1 | `core/events` — typed event-bus (SPEC 047) | ✅ |
| 3.2 | `core/state` — pure state model + Diff + миграции v3/v4 | ✅ |
| 3.3 | `core/outboundscache` — `bin/outbounds.cache.json` snapshot | ✅ |
| 3.4 | `core/build` — orchestrator + все pure-data функции | ✅ |
| 4.1 | `StateService` — UpdateDirty / RestartDirty + `ApplyDiff` + EventBus | ✅ |
| 4.2 | EventBus передаётся через AppController в StateService | ✅ |
| **5.A** | **Update path** через `BuildConfig` (cutover Update) | ✅ |
| **5.B** | **Wizard Save → state-only** | ✅ |
| 5.C | Restart pre-rebuild (компенсация UX из 5.B) | ✅ |
| 5.D | Снос legacy: `WriteToConfig`, `BuildTemplateConfig`, `SaveConfigWithBackup`, шимы | ✅ |
| 6.1/6.2 | StateService UpdateDirty/RestartDirty + EventBus | ✅ |
| 6.3 | Снос legacy `TemplateDirty` после миграции UI | ✅ |
| **E.1** | **UI два маркера в Core Dashboard (Update `*` / Restart `*`)** | ✅ |
| **E.2** | **Аггрессивный снос шимов (formatting, MergeDNSSection, ParseDNSRulesText)** | ✅ |
| 7.1 | Rename `ui/wizard` → `ui/configurator` (Go-уровень) | ✅ |
| 7.2 | Переименование i18n-ключей `wizard.*` → `configurator.*` | ⏸ (отложено: 11 locale-файлов × 21 callsite, чисто косметика) |
| 7.3 | Переименование `docs/CREATE_WIZARD_TEMPLATE.md` etc. | ⏸ |
| 8 | Финал docs / release notes / переименование папки SPEC | 🟡 финал-полировка |

---

## Архитектурные решения

1. **Имя UI-контейнера** — `Configurator` (не `Wizard`); вкладки внутри сохраняют свои имена.
2. **Кэш outbounds** — один общий файл `bin/outbounds.cache.json`, scope = последний активный state. На переключении state'а cache не инвалидируется (advisory snapshot предыдущего state); StateService поднимает `UpdateDirty`, BuildConfig собирает из stale-cache не падая.
3. **Переименование Wizard→Configurator** — делается в этом же SPEC'е (фаза 7), не отдельным.
4. **Typed events** — выделены в зависимый SPEC 047 (`SPECS/047-F-N-TYPED_EVENT_BUS/`). Реализованы как `core/events.MemoryBus` с sync dispatch + panic-isolation.
5. **State-файл schema** — пока `v4` (без bump в `v5`). Поведение меняется без изменения формата на диске.
6. **`core/build` — leaf-пакет**: ничего не импортирует из `ui/`. Wizard-side готовит `BuildContext` из своей модели и зовёт `BuildConfig`.

---

## Что физически сделано в коде

### Новые пакеты (clean leaf-пакеты в `core/`)

| Пакет | Файлы | LOC | Тесты | Назначение |
|---|---|---|---|---|
| `core/events` | events.go, payloads.go, memory_bus.go, _test.go | ~300 | 11 | Typed event bus (SPEC 047) |
| `core/state` | state.go, load.go, save.go, diff.go, _test.go, testdata/ | ~450 | 18 | Pure state model + миграции v3/v4 + Diff |
| `core/outboundscache` | snapshot.go, _test.go | ~190 | 8 | `bin/outbounds.cache.json` snapshot |
| `core/snapshot` | snapshot.go, _test.go | ~190 | 7 | `Build(execDir, ver, ver)` — общий backend для `/debug/snapshot` и UI Copy snapshot |
| `core/template` | (move from `ui/wizard/template/`, 26 callsites обновлены) | ~700 | существующие | Pure-data template loader, GetEffectiveConfig, SubstituteVarsInJSON |
| `core/build` | build.go, format.go, parser_config.go, clash_secret.go, sections.go, dns_merge.go, route_merge.go, sync_dns.go, golden_test.go | ~1500 + ~1500 тестов | 75+ + golden | **Orchestrator** `BuildConfig(BuildContext) → Result`; pure-data порт всех `BuildTemplateConfig` модулей |

### Изменения в существующих пакетах

| Файл | Изменение |
|---|---|
| `internal/constants/constants.go` | + `OutboundsCacheFileName` |
| `internal/platform/platform_common.go` | + `GetOutboundsCachePath` (+ test) |
| `core/services/state_service.go` | + `UpdateDirty` / `RestartDirty` + `ApplyDiff` + `EventBus` |
| `core/services/state_service_dirty_test.go` | новый файл (9 тестов) |
| `core/controller.go` | + `EventBus events.Bus` поле; инициализация в обоих конструкторах StateService |
| `core/debugapi/server.go` | + `GetExecDir` в `ControllerFacade`; route `/debug/snapshot` |
| `core/debugapi/snapshot.go` | новый — тонкая HTTP-обёртка над `core/snapshot.Build` |
| `core/debugapi/snapshot_test.go` | 8 тестов |
| `core/debugapi_wiring.go` | + `GetExecDir()` реализация |
| `core/config_service.go` | **CUTOVER (фаза 5.A):** `UpdateConfigFromSubscriptions` переписан на новый pipeline (`state.Load → parser.Generate → outboundscache.Save → BuildConfig → atomic-write`); legacy `config.UpdateConfigFromSubscriptions + WriteToConfig` выпилен из этого callsite |
| `ui/wizard/business/create_config.go` | 521 → 250 LOC (-50%); все pure-data функции стали тонкими шимами над `core/build.*` |
| `ui/wizard/business/wizard_dns.go` | `MergeDNSSection`, `ParseDNSRulesText` — шимы; удалён мёртвый `firstEnabledDNSServerTag` |
| `ui/wizard/business/dns_settings_vars.go` | `SyncDNSModelToSettingsVars` — шим |
| `ui/wizard/business/formatting.go` | шим над `core/build.{Indent,IndentMultiline,FormatSectionJSON}` |
| `ui/wizard/presentation/presenter_save.go` | **CUTOVER (фаза 5.B, в процессе):** `executeSaveOperation` — state-only; убраны `ensureOutboundsParsed` (60-сек poll), `buildConfigForSave`, `saveConfigFile`. Save теперь: `state.Save + MarkUpdate/RestartDirty + EventBus.Publish + success dialog` |

### CI / runtime / docs (попутное)

- **`.github/workflows/ci.yml`** — переход с `RELEASE_NOTES.md` (агрегат) на `docs/release_notes/<slug>.md` (per-version) + auto-banner для prerelease (отдельная задача из ранней сессии, не SPEC 045).
- **`docs/RELEASE_PROCESS.md`** — runbook stable + prerelease (в той же ранней сессии).
- **SPEC 047** — `SPECS/047-F-N-TYPED_EVENT_BUS/SPEC.md, PLAN.md, TASKS.md` — типизированный event-bus.
- **SUB_SPEC_SNAPSHOT** — `SPECS/038-F-C-DEBUG_API/SUB_SPEC_SNAPSHOT.md` — endpoint `GET /debug/snapshot`.
- **`docs/release_notes/upcoming.md`** — пункт про `/debug/snapshot` добавлен.

---

## Тесты

- **75+ unit-тестов** в `core/build/*` + 35+ в других новых пакетах (`events`, `state`, `outboundscache`, `snapshot`).
- **Golden-test `real-v088`** — реальный production-сценарий с dev-машины (29 outbounds, 1 WireGuard endpoint, custom DNS rules, custom route rules, 7 top-level секций). **Byte-equal parity** между `core/build.BuildConfig` и legacy `BuildTemplateConfig + populateCheckText + WriteToConfig`. Запускается через `GOLDEN_RUN_REAL=1 go test ./core/build`.
- **Race detector** — clean на новых пакетах.
- **Все существующие тесты** (`go test ./...`) — green после рефакторинга. Не сломан ни один.

---

## Что РЕАЛЬНО изменилось в production-флоу

После следующего билда лаунчера:

1. **Update / auto-update** — идёт через новый pipeline `core/build.BuildConfig`. Output байт-в-байт совпадает с legacy на golden test'е.
2. **Wizard Save** — после фазы 5.B пишет ТОЛЬКО `state.json`, поднимает оба dirty-маркера, `config.json` остаётся прежним до Update/Restart. Это поведенческий сдвиг — пользователь должен явно нажимать Update / Restart.
3. **`bin/outbounds.cache.json`** — новый файл; пишется при Update; не используется legacy-флоу, но используется новым Update path и golden-тестами.
4. **`StateService` имеет два независимых dirty-флага** + публикует `StateChanged` события.
5. **Debug API** — новый endpoint `GET /debug/snapshot` возвращает четвёрку файлов wizard-pipeline'а как один JSON.

## UX-регрессия от фазы 5.B → закрыта в 5.C

5.C **реализован** в `core/rebuild.go::AppController.RebuildConfigIfDirty()` и
вызывается из `ProcessService.Start` ДО kill/start sing-box. Алгоритм:

1. Если оба маркера чистые — выходим (no-op).
2. Загрузить `state.json` + `bin/outbounds.cache.json` + template.
3. Собрать `BuildContext` через `buildContextFromState`.
4. `core/build.BuildConfig` → `Result`.
5. Атомарная запись `config.json` (`.tmp` → `os.Rename`).
6. `ClearUpdateDirty` + `ClearRestartDirty`.
7. Публикация `events.ConfigBuilt`.
8. Coarse callbacks `UpdateConfigStatusFunc` + `UpdateCoreStatusFunc` для refresh маркеров в UI.

Best-effort: если `BuildConfig` падает, sing-box запускается со старым `config.json`
(не блокируем запуск VPN ради config-rebuild ошибки). Ошибка логируется в `debuglog`.

---

## Метрики

| Метрика | Значение |
|---|---|
| LOC pure business в `core/build/` | ~1500 |
| LOC тестов в `core/build/` | ~1500 |
| `ui/wizard/business/create_config.go` | 521 → 250 (-52%) |
| Новых pure-leaf пакетов в core/ | 6 (events, state, outboundscache, snapshot, template переехал, build) |
| Затронутых файлов | ~25 |
| Удалено LOC из ui/wizard/business/ | ~300 (помимо шимов) |
| Sing-box runtime | не тронут — все изменения в working tree, не коммитились |

---

## Что осталось сделать (детально)

### Фаза 5.B — завершено

`executeSaveOperation` переписан на state-only (272 LOC, было 580). Save пишет
ТОЛЬКО `state.json`, поднимает `UpdateDirty`+`RestartDirty`, публикует
`events.StateChanged`. UI получает refresh через `UpdateConfigStatusFunc` +
`UpdateCoreStatusFunc`. Dead helpers (`ensureOutboundsParsed`, `buildConfigForSave`,
`saveConfigFile`, `saveStateAndShowSuccessDialog`, `showSaveErrorDialog`,
`showValidationErrorDialog`) удалены.

### Фаза 5.C — завершено

`core/rebuild.go::AppController.RebuildConfigIfDirty()` вызывается из
`ProcessService.Start`. Подробности — выше в разделе «UX-регрессия».

### Фаза 5.D — завершено (через `git rm`)

Удалены:
- `core/config/updater.go` (`WriteToConfig`, `PopulateParserMarkers`,
  `UpdateConfigFromSubscriptions`, `indentEndpointsBlock`).
- `ui/configurator/business/saver.go` (`SaveConfigWithBackup`, `prepareConfigText`,
  `generateRandomSecret`, `ValidateConfigWithSingBox`).
- `ui/configurator/business/validation_error.go`.
- `ui/configurator/business/formatting.go` (шим над `core/build.Indent*`).
- `MergeDNSSection` wizard-обёртка из `wizard_dns.go`.
- `ParseDNSRulesText` wizard-обёртка (callsites инлайнены на `build.ParseDNSRulesText`).

Оставшиеся легитимные UI-адаптеры (`MaterializeClashSecretIfNeeded`,
`SyncDNSModelToSettingsVars`, `MergeRouteSection`, `EffectiveConfigSection`):
конвертят `WizardModel` → `core/build` структуры; не шимы, а UI-side adapters.
Полное устранение требует refactor wizard-модели в DI-стиле — out of scope для 045.

### Фаза 6 — завершено

- 6.1/6.2: `StateService.UpdateDirty` / `RestartDirty` + `EventBus` — реализовано в
  фазе 4.1.
- 6.3: `TemplateDirty` поле, `IsTemplateDirty`, `SetTemplateDirty`, `TemplateDirtyMutex`,
  одноимённый тест — удалены. Все callsites (`core/rebuild.go`, `core/config_service.go`,
  `presenter_save.go`) обновлены.

### Фаза E.1 — завершено (UI два маркера)

- `ui/core_dashboard_tab.go::updateConfigInfo`: `IsTemplateDirty()` →
  `IsUpdateDirty()` (Update marker `* 🔄 Update`).
- `updateRunningStatus`: добавлен Restart-marker `*🔄` на `restartButton` когда
  `IsRunning && IsRestartDirty`. Tooltip переключается на
  `core.restart_dirty_tooltip`.
- i18n keys `core.restart_dirty_tooltip` (en+ru).
- `presenter_save.go` + `core/rebuild.go` зовут оба callback'а
  (`UpdateConfigStatusFunc` + `UpdateCoreStatusFunc`) для синхронного refresh
  обоих маркеров.

### Фаза E.2 — завершено (аггрессивный снос шимов)

См. фазу 5.D.

### Фаза 7.1 — завершено (Go-уровень)

- `mv ui/wizard ui/configurator`.
- `package wizard` → `package configurator` в `configurator.go`.
- 26+ import-paths обновлены sed'ом.
- Внешний caller `wizard.ShowConfigWizard` → `configurator.ShowConfigWizard`
  обновлён в `ui/core_dashboard_tab.go`.

### Фаза 7.2 / 7.3 — отложено

- **i18n keys `wizard.*` → `configurator.*`**: 11 locale-файлов × ~21 callsite.
  Чисто косметическая операция (ключи внутренние, не видны пользователю).
  Решено отложить до отдельного relabel-PR.
- **`docs/CREATE_WIZARD_TEMPLATE.md` rename**: аналогично, не блокирует SPEC.
- **`WizardModel` → `ConfiguratorModel`**: Go-API rename внутри
  `ui/configurator/models/`. Большой diff на 100+ callsites; не приоритет.
- **`bin/wizard_template.json`** имя файла **остаётся** (legacy migration cost >
  UX win — пришлось бы удалять/переустанавливать у пользователей).

### Фаза 8 — финальная полировка

- `docs/release_notes/upcoming.md` — добавить пункт про SPEC 045.
- `docs/ARCHITECTURE.md` — обновить слой state / BuildConfig / outboundscache /
  events bus.
- Переименовать `SPECS/045-F-N-` → `SPECS/045-F-C-` (close-out).
- Финальный коммит после ручного прогона лаунчера.

---

## Связи

- **SPEC 002 (WIZARD_STATE)** — формат state.json v4, миграции. Используется `core/state` без изменений в формате.
- **SPEC 027** — rules library merger. Логика сохранена через `state.RulesLibraryMerged`.
- **SPEC 032** — vars + DNS migration. `ApplyDNSScalarsToVars` повторяет ту же логику.
- **SPEC 038 (DEBUG_API)** — расширен через `SUB_SPEC_SNAPSHOT.md` + endpoint `/debug/snapshot`.
- **SPEC 043 (DIRTY_CONFIG_MARKER)** — `TemplateDirty` будет удалён в фазе 6.3.
- **SPEC 047 (TYPED_EVENT_BUS)** — реализован как зависимость 045 (`core/events`).

---

## Риски и mitigation

| Риск | Mitigation |
|---|---|
| Расхождение между `core/build.BuildConfig` и legacy на edge-case инсталляциях | Golden-test harness `core/build/testdata/golden/<scenario>/`; добавлять реальные сценарии через `/debug/snapshot` |
| После 5.B без 5.C — Save не применяется на Run | 5.C делается в той же серии, сразу после 5.B |
| Migrate state.json формат сломается | `state.Load` поддерживает v2/v3/v4, миграции тестами покрыты |
| sing-box check регрессия | На Update path сейчас НЕТ sing-box check (как у legacy `WriteToConfig`); добавить в фазу 5.D как enhancement |
| Real-v088 — единственный сценарий | Нужно несколько captured-сценариев для разных кейсов (multi-source, custom rules-only, no DNS overrides и т.д.) |

---

## Открытые вопросы

1. **Sing-box check на Update path** — добавлять ли? Legacy не делал. Предложение: добавить в фазе 5.D как enhancement (validate temp file before atomic-rename; на провале — keep old config.json).
2. **State.Diff как источник dirty markers** — currently Save поднимает оба маркера (UpdateDirty + RestartDirty). Точнее было бы вычислять `Diff(prev_state, new_state)` и поднимать только релевантные. Требует хранить `prev_state` в WizardPresenter с момента load.
3. **Multi-state** (несколько named state'ов) — текущий `core/state` поддерживает `state.ID`, но multi-state UI пока не было. Готов к расширению.

---

## Acceptance criteria (из SPEC.md)

| Критерий | Статус |
|---|---|
| Save в визарде не мутирует config.json | 🟡 в процессе (фаза 5.B) — build зелёный, runtime не tested |
| Два UI-маркера (`*` Update vs Restart) | ⏸ фаза 6 |
| BuildConfig — единственный writer config.json | 🟡 50% (Update done, Restart не done) |
| Migration со старого state.json | ✅ `core/state.Load` поддерживает v2/v3/v4 |
| SPEC на event-bus | ✅ SPEC 047 реализован |
| Конс. terminology UI (если переименовываем) | ⏸ фаза 7 |
