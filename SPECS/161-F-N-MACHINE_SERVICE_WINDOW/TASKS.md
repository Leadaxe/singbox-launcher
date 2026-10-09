# TASKS 161 — окно Service

Три исполнителя, файлы не пересекаются. Порядок:

```
A1─A6 (core) ──┬──▶ B1 (модель окна) ──┬──▶ B2─B8 (ui: окно, удалённо)
               │                        └──▶ C2─C3 (ui: Local)
               └──▶ C1 (Debug API), C4─C6 (доки) — сразу после A
                                            C7 (ru.json, закрытие) — последней, после B и C
```

Правила для всех волн:
- ветка `develop`, без переключений веток, коммит своими путями:
  `git commit -m "…" -- <пути>`; чужие правки не трогать;
- локально только `go build` затронутых пакетов (и `go build ./...` в конце
  волны) и один именованный тест A7; **запрещено** `go test ./...`, прогон
  пакета целиком, `go vet ./...`, Win7-сборка;
- общий код — без `min`/`max`/`clear`, `slices`, `maps`, `range` по int
  (go1.20); страж `go run ./tools/win7guard`;
- каждый новый `locale.T` — с переводом; `bin/locale/ru.json` правит **только
  C7** (сводно, по выводу `go run ./tools/l10n/l10n_check`), остальные
  исполнители перечисляют свои новые/удалённые ключи в отчёте;
- UI тестами не покрывается; вёрстку смотрит владелец.

Якоря — [CODEMAP.md](CODEMAP.md), решения — [PLAN.md](PLAN.md).

---

## Волна A — core (исполнитель A)

### A1. Версии ядра без тегов
- [x] Новый `core/core_build.go` (без build-тега): перенести из
  `core/daemon_service_state.go:205–304` `coreBuild`, `parseCoreBuild`,
  `leadingDigits`, `compareCoreBuilds`, `compareInts` и `sha256File` (`:560–571`);
  в `daemon_service_state.go` удалить (импорты `strconv`/`crypto/sha256`/`hex`/`io`
  подчистить, если стали лишними).
- [x] Добавить `CoreVersionVerdict`, `CompareCoreVersion(running, required)`,
  `CoreBuildShort(v)`, `CoreVersionPairLabels(running, required)` (PLAN §2).
- Проверка: `go build ./core/...`.

### A2. Рецепты и классификатор
- [x] Новый `core/service_recipes.go` (без тега): `ServiceInit`, `ServiceName`,
  `ServiceLaunchdLabel`, `ServicePlatform`, `DefaultServiceInit`,
  `NormalizeInitChoice`, `ServicePath`, `ServicePaths`, `DefaultServicePaths`,
  `ServicePassport`, `MergeServicePaths`, `ServiceStep`, `ServiceRecipeInput`,
  `ServiceRecipes`, `BuildServiceRecipes`, `WrapSSH` (над
  `services.SSHTarget` из A4), `PosixQuote`, `DaemonReachKind`,
  `ClassifyDaemonReachError` — по таблицам PLAN §3.
- [x] `core/backend_daemon.go:214` `diagnoseReachError`: ветвление через
  `ClassifyDaemonReachError` (тексты советов и `followDaemonPlainChannel` без изменений).
- [x] `core/daemon_manager_darwin.go:31`, `core/daemon_manager_windows.go:37`:
  константы метки/имени службы ссылаются на `ServiceLaunchdLabel`/`ServiceName`
  (один источник).
- Проверка: `go build ./core/...`.

### A3. Ядро под платформу машины
- [x] `core/core_downloader.go:304`: `SingboxAssetSuffixFor(goos, goarch)` (+ mips/mipsle
  softfloat), `SingboxAssetSuffix()` — обёртка; `:206`/`:216` — `directAssetNameFor`,
  `DirectAssetURLFor`, старые — обёртки; `:574–700` — `extractArchiveNamed(archive, dest, binName)`,
  `extractZip`/`extractTarGz` с параметром имени, `extractArchive` — обёртка.
- [x] Новый `core/core_download_target.go` (без тега): `TargetCoreDownload`,
  `TargetCoreFileName`, `DownloadsDir`, `CheckTargetCore`,
  `(*AppController).DownloadCoreForTarget` (PLAN §4: архив → SHA256SUMS →
  сверка → распаковка → `~/Downloads` → chmod 0755 → сайдкар `.sha256`).
- Проверка: `go build ./core/...`.

### A4. Запись машины и здоровье
- [x] `core/services/lxd_remote_registry.go:37–73`: поля `SSH`, `InitSystem`,
  `CoreWarnAck`, `Passport *RemotePassport`; тип `RemotePassport` (PLAN §5.1).
- [x] Новый `core/services/ssh_target.go` (без тега): `SSHTarget`,
  `ParseSSHTarget`, `DefaultSSHTarget` (PLAN §3: `core/services` не может
  импортировать `core`, а `SetSSH` валидирует; `core/service_recipes.go`
  использует их напрямую из `services`). Делается до A2.
- [x] Сеттеры рядом с `SetPlatform` (`:547`): `SetSSH` (валидация
  `ParseSSHTarget`, пустое — сброс), `SetInitSystem`, `SetCoreWarnAck`,
  `SetSecret`, `SetPassport` (no-op без изменений и при `SeenAt` моложе 10 мин).
- [x] `RemoteHealth` (`:809`): `Executable`, `LogPath`, `Listen`, `TLS *bool`,
  `UptimeSeconds`; `healthCtx` (`:874–881`) заполняет их и зовёт `SetPassport`.
- Проверка: `go build ./core/... && GOOS=linux go build ./core/services/... ./internal/...`.

### A5. Локальный демон: паспорт, restart, пути
- [x] `core/daemon_manager.go:261`: `DaemonUIStatus` += `ReachErr`, `Passport`,
  `PassportSeenAt`, `PassportCached`; кэш `daemonPassportCache`; заполнение в
  `DaemonStatusSnapshot` (`:291–334`: ошибка `Status()` → `ReachErr`, при
  ошибке — паспорт из кэша).
- [x] `core/daemon_manager.go`: `DaemonOpRestart` (в список `:746–763`),
  `DaemonServicePaths()`, `DaemonClientListCommand()`, `DaemonClientRemoveCommand(name)`.
- [x] `core/daemon_manager_darwin.go`: `DaemonRestartCommand` (= kickstart),
  `DaemonRestartService` (`runDaemonOpInTerminal`), `daemonServicePathsPlatform`.
- [x] `core/daemon_manager_windows.go`: `DaemonRestartCommand`
  (`Restart-Service -Name sing-box-lxd -Force`), `DaemonRestartService`
  (`runDaemonCommandElevated` с `powershell.exe`, затем `waitDaemonServiceRunning`,
  `r.Service = ac.daemonServiceCheck(nil, "")`), `daemonServicePathsPlatform`.
- Проверка: `go build ./core/...` (macOS); Windows-файл — CI.

### A6. Константы URL
- [x] `internal/constants/constants.go:142–166`: `LauncherDocsBaseURL`,
  `CoreDocsBaseURL` (PLAN §9) с комментарием «зачем».
- Проверка: `go build ./internal/constants/`.

### A7. Тест волны
- [x] Новый `core/service_recipes_test.go`: `TestServiceRecipes` — таблица из PLAN §12.
- Проверка: `go test ./core -run '^TestServiceRecipes$' -count=1`; затем `go build ./...`;
  `go run ./tools/win7guard`.
- [x] Коммит волны A: `core/core_build.go core/service_recipes.go core/service_recipes_test.go
  core/core_download_target.go core/core_downloader.go core/daemon_service_state.go
  core/backend_daemon.go core/daemon_manager.go core/daemon_manager_darwin.go
  core/daemon_manager_windows.go core/services/lxd_remote_registry.go
  core/services/ssh_target.go internal/constants/constants.go`.

---

## Волна B — ui: окно Service и удалённые машины (исполнитель B, после A)

### B1. Модель окна (первой — от неё зависит C2)
- [x] Новый `ui/service_model.go`: `serviceTab`, `serviceSnapshot`,
  `localServiceExtras`, `localServiceRows`, `serviceSource`, `serviceLevel`,
  `serviceDiagnosis(snapshot) (glyphs map[serviceTab]string, first serviceTab)`,
  `serviceSnapshotEqual` (PLAN §6.1).
- Проверка: `go build ./ui/`. Коммит сразу (C2 ждёт его).

### B2. Окно и шапка
- [x] Новый `ui/service_window.go`: `OpenServiceWindow`, `CloseServiceWindow`,
  `buildServiceView` (шапка, `Select` init, ярлыки с глифами, стартовая вкладка,
  ре-рендер без потери ввода, VScroll вкладок с `serviceTabScrollMinHeight = 160`,
  слой тултипов) — PLAN §6.2.
- [x] Новый `ui/service_guides.go`: таблица гайдов EN/RU-якорей, `guideURL`,
  `guideLink` (PLAN §9).
- Проверка: `go build ./ui/`.

### B3. Строка шага
- [x] Новый `ui/service_step_row.go`: `serviceStepRow`, `serviceRunner` по таблице PLAN §6.3
  (ssh-обёртка + `-t`, `RunsLocally`, scm без ▶, `openTerminal == nil` → только ⧉,
  подпись `default`, подтверждение для опасных шагов).
- Проверка: `go build ./ui/`.

### B4. Вкладки
- [x] Новый `ui/service_tabs.go`: `buildNotRunningTab`, `buildCoreTab`,
  `buildPairingTab`, `buildReferenceTab` по SPEC §5.2–§5.5 и PLAN §6–7
  (общие для local/remote; локальные строки — из `src.LocalRows`).
- Проверка: `go build ./ui/`.

### B5. Источник «машина», вердикт строки, живой лог
- [x] Новый `ui/service_source_remote.go`: `newRemoteServiceSource(p, d)`,
  тикер `serviceRemoteRefresh`, `machineServiceVerdict`, `rePairMachine`
  (вынести из `ui/machine_edit_window.go:165–200`).
- [x] Новый `ui/service_core_download.go`: шаг 1 Core удалённо (PLAN §6.6).
- [x] Новый `ui/machine_core_log_window.go`: `OpenMachineCoreLogWindow`,
  `CloseMachineCoreLogWindow` (PLAN §6.7).
- Проверка: `go build ./ui/`.

### B6. Строка машины и Deploy
- [x] `ui/machine_list_panel.go`: ⚙ в `statusRow` (`:434`) и до Connect в `metaRow`
  (`:301`); точка `withCornerDot` по `machineServiceVerdict`; строка
  предупреждения после `statusRow` (`:470`); `deployTo` (`:972`) — диалог
  старого ядра (`dialogs.NewCustom`, чекбокс → `SetCoreWarnAck`, How to update →
  Service/Core); `CloseMachineCoreLogWindow` в `connectMachine` (`:499–506`),
  `disconnectMachine` (`:798`), `removeMachine` (`:852–855`); `CloseServiceWindow`
  в `removeMachine`.
- [x] `ui/machine_heartbeat.go`: `machineLiveness.FailSince` (`:59`), отметка в
  `pollActive` (`:114–118`) и в `connectMachine` (`:575–578`); `healthChanged`
  вместо `prev != h` (`:146`).
- Проверка: `go build ./ui/`.

### B7. Окно Edit машины
- [x] `ui/machine_edit_window.go`: поле SSH в `machineEditPassport` (`:79–118`,
  после Address; плейсхолдер `DefaultSSHTarget(addr)`; Save → `SetSSH`);
  `machineEditRePair` — через `rePairMachine` (B5); закрытие
  `CloseMachineCoreLogWindow` при re-pair.
- Проверка: `go build ./ui/`.

### B8. Сдача волны B
- [x] `go build ./...`; `go run ./tools/win7guard`; `go run ./tools/l10n/hardcoded_check --strict`.
- [x] Список новых ключей `locale.T` волны — в сообщение для C7
  (`go run ./tools/l10n/l10n_check` покажет их как `missing`).
- [ ] Показать владельцу (за владельцем: лаунчер исполнитель не запускает): строка роутера (⚙•, строка ⚠/✖), окно Service на всех
  вкладках, скачивание под linux/arm64, ▶ с `ssh root@…`, диалог Deploy.
- [x] Коммит: `ui/service_model.go ui/service_window.go ui/service_guides.go
  ui/service_step_row.go ui/service_tabs.go ui/service_source_remote.go
  ui/service_core_download.go ui/machine_core_log_window.go ui/machine_list_panel.go
  ui/machine_heartbeat.go ui/machine_edit_window.go`.

---

## Волна C — Local, Debug API, документация (исполнитель C)

### C1. Debug API (после A)
- [x] `core/debugapi/daemon_endpoints.go:19–51`: `DaemonStatus` += `log_path`,
  `executable`, `listen`, `tls`, `uptime_seconds`, `reach_error`, `passport_cached`;
  `DaemonCommands` += `restart`, `client_list`.
- [x] `core/debugapi_wiring_daemon.go:32–76`: заполнение из `DaemonStatusSnapshot`,
  `DaemonRestartCommand`, `DaemonClientListCommand`.
- [x] `core/debugapi/remote_endpoints.go`: `machineView` (`:211–230`) += `ssh`,
  `init_system`, `core_warn_ack`, `passport`; PATCH (`:300–350`) += `ssh`,
  `init_system` (через `SetSSH`/`SetInitSystem`, текст ошибки «nothing to update»
  дополнить); `handleRemoteHealth` (`:492`) += `core_required`, `core_outdated`.
  **Δ** `core_outdated` — замыканием `RemoteAPI.CoreOutdated`, проведённым в
  `core/debugapi_wiring.go` (debugapi не импортирует core); health отдаёт и остальной
  паспорт (`executable`, `log_path`, `listen`, `tls`, `uptime_seconds`), `/daemon/status` —
  ещё `passport_seen_at`.
- Проверка: `go build ./core/...`. Коммит этими тремя путями (+ `core/debugapi_wiring.go`).

### C2. Источник «локальный демон» (после B1)
- [ ] Новый `ui/service_source_local.go` (тег `darwin || (windows && !386)`):
  `newLocalServiceSource(ac, win, onPaired)`, тикер `serviceLocalRefresh = 5 s`,
  `DownSince`/`Attempts`, `MergeServicePaths(passport, ac.DaemonServicePaths())`,
  `LocalRows` (строки `daemonOps`: install, bootstrap, restart, fresh invite,
  Pair-поле, адрес, секрет + справка, Uninstall-вкладка), `OpenLiveLog` →
  `OpenLogViewerWindow`.
- Проверка: `go build ./ui/`.

### C3. Панель Local → окно Service
- [ ] `ui/connection_local_daemon.go`: `buildDaemonPanel` (`:61–411`) — подсказка,
  `Stop VPN when quitting`, затем `buildServiceView(local source)` с вкладкой
  Uninstall; код Uninstall (`:241–313`), install/bootstrap/pair/secret/address
  переезжает в `LocalRows` (C2); удалить `refreshBtn` (`:344`), `statusTab`,
  `installTab`, `AppTabs` (`:368–402`), `renderDaemonStatusText` (`:416–469`);
  `daemonServiceNoticeText`, `coreBuildLabel`, `daemonOps`, `showCommandHelpDialog`,
  `daemonPurgeRow` — остаются.
- [ ] `ui/command_row_darwin.go`, `ui/command_row_windows.go`: подписи новых
  локальных шагов (restart, status) с `// l10n-key`; удалить подписи, ставшие
  ненужными (если `daemonInstallStepLabel`/`daemonPairStepLabel` с нумерацией
  «1.»/«2.» больше не используются — заменить на подписи шагов окна).
- [ ] `ui/connection_window.go:53–75`: `fynetooltip.AddWindowToolTipLayer` на
  контент, `DestroyWindowToolTipLayer` в `SetOnClosed`.
- [ ] `ui/connection_local.go` — **не меняется** (радио движка); если сигнатура
  `buildDaemonPanel` останется прежней — файл не трогать.
- Проверка: `go build ./ui/` и `go build ./...`. Показать владельцу окно
  Connection settings (macOS): шапка, пять вкладок, все прежние действия
  (install, bootstrap, pair, fresh invite, secret, address, unpair, uninstall,
  purge) и новый restart.

### C4. TROUBLESHOOTING
- [x] `docs/TROUBLESHOOTING.md`: раздел `## Daemon` (якорь `#daemon`) — не
  поднимается / обновить ядро на машине / сопряжение / где что лежит, команды
  по procd/systemd/macOS/Windows как в `core/service_recipes.go`.
- [x] `docs/TROUBLESHOOTING.ru.md`: раздел `## Демон` (якорь `#демон`) — то же.

### C5. DAEMON_AND_REMOTE, API, ARCHITECTURE
- [x] `docs/DAEMON_AND_REMOTE.md` + `.ru.md`: §2 (окно Service локально, что куда
  переехало), §4.1 (⚙ и строка предупреждения), §4.2 (поля `ssh`, `init_system`,
  `core_warn_ack`, `passport`), §4.4 (предупреждение о ядре при Deploy).
- [x] `docs/API.md` + `docs/API.ru.md`: поля C1.
- [x] `docs/ARCHITECTURE.md` (+ `.ru.md` — тот же §11.8): L6 (`:85`) — окно Service, окно лога машины; §11
  (`:1029`) — `core/service_recipes.go`, `core/core_build.go`,
  `core/core_download_target.go`, источники local/remote.

### C6. Релизные заметки
- [x] `docs/release_notes/upcoming.md` — EN Highlights / RU Основное.
- [x] `RELEASE_NOTES.md` — строка в выжимке черновика (значимый UX).

### C7. Переводы и закрытие (последней, после B8 и C3)
- [ ] `bin/locale/ru.json`: переводы всех `missing` и удаление всех `orphan` из
  `go run ./tools/l10n/l10n_check --strict` (ключи волн B и C).
- [ ] `go run ./tools/l10n/l10n_check --strict`, `go run ./tools/l10n/hardcoded_check --strict`,
  `go run ./tools/win7guard`, `go build ./...`.
- [ ] CI: `gh workflow run ci.yml --ref develop -f run_mode=tests`; упавшее —
  `gh run view <ID> --log-failed`.
- [ ] `SPECS/161-F-N-MACHINE_SERVICE_WINDOW/IMPLEMENTATION_REPORT.md`; строка 161 в
  `SPECS/README.md`; папка → `161-F-C-MACHINE_SERVICE_WINDOW` после приёмки владельцем.
- [ ] Коммит C: `core/debugapi/daemon_endpoints.go core/debugapi_wiring_daemon.go
  core/debugapi/remote_endpoints.go ui/service_source_local.go
  ui/connection_local_daemon.go ui/command_row_darwin.go ui/command_row_windows.go
  ui/connection_window.go docs/TROUBLESHOOTING.md docs/TROUBLESHOOTING.ru.md
  docs/DAEMON_AND_REMOTE.md docs/DAEMON_AND_REMOTE.ru.md docs/API.md docs/API.ru.md
  docs/ARCHITECTURE.md docs/release_notes/upcoming.md RELEASE_NOTES.md
  bin/locale/ru.json SPECS/README.md SPECS/161-…/IMPLEMENTATION_REPORT.md`.

---

## Приёмка (SPEC §10)

- [ ] Старое ядро: ⚙• жёлтая, строка `⚠ Core … older than required`, Deploy — диалог
  с «Deploy anyway» и чекбоксом; после смены версии предупреждение вернулось.
- [ ] Демон лёг: ⚙• красная, строка `✖ …`, окно на `✖ Not running`, команды под
  init машины с путями паспорта или `default`.
- [ ] ▶ открывает Terminal с `ssh <target> '<cmd>'`; ⧉ копирует без обёртки.
- [ ] Core скачивает ядро под linux/arm64, sha256 сверен с SHA256SUMS, шаги 2–4 с
  путём файла и путями машины.
- [ ] После `restart` на машине шапка зеленеет без кликов.
- [ ] Local: шапка и вкладки; install, bootstrap, pair, re-pair, secret, address,
  unpair, uninstall, purge работают как раньше; restart (kickstart) появился.
- [ ] Гайды в шапке и каждой вкладке, по локали.
- [ ] `docs/TROUBLESHOOTING` содержит раздел Daemon.
- [ ] Один тест `TestServiceRecipes`; UI без тестов.
