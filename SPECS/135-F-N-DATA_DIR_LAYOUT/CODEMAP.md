# SPEC 135 · CODEMAP обращений к путям

Карта для этапов 3–5 и 7–9. Строки — по дереву после этапа 2 (коммит
`32c21ec9`, литералы уже на хелперах). Тестовые файлы — только в §5.

Классы:

- **D** — Data: запись и состояние (`DataDir`);
- **L** — Log: лог-файлы (`LogDir`);
- **A** — App: только поставляемое (`AppDir`);
- **M** — Mixed: нужны два корня; как решать — в пояснении;
- **—** — комментарий или объявление, кода нет.

Объём: 190 строк `ExecDir` в 62 файлах (`grep -rn ExecDir --include=*.go . | grep -v _test.go`),
плюс ~55 вызовов `platform.*` от производной переменной (`execDir`,
`binDir`, `r.execDir`) и ~45 функций с параметром `execDir`/`binDir` (§1.2).

Важно для этапа 3: `paths.Layout.Logs` — это уже сам каталог логов
(`…/logs`, `internal/paths/paths.go:141,199,233,252`), поэтому
`GetLogsDir(LogDir)` вырождается в `string(l)`. `paths.DataDir.Bin()` и
`paths.AppDir.Bin()` (`paths.go:54,57`) заменяют `GetBinDir` там, где нужен
сам `bin/`.

---

## 1. Хелперы путей

### 1.1 `internal/platform`

| Функция | Файл:строка | Строит | Тип после рефакторинга |
|---|---|---|---|
| `GetConfigPath` | platform_common.go:32 | `bin/config.json` | `DataDir` |
| `GetRemoteMachineDir` | platform_common.go:46 | `bin/wizard_states/remote/<id>` | `DataDir` |
| `GetRemoteConfigPathFor` | platform_common.go:64 | `…/remote/<id>/config.json` | `DataDir` |
| `GetBinDir` | platform_common.go:69 | `bin` | `DataDir`; для поставляемого — `AppDir.Bin()` |
| `GetRuleSetsDir` | platform_common.go:74 | `bin/rule-sets` | `DataDir` |
| `GetRuleSetPath` (этап 2) | platform_common.go:81 | `bin/rule-sets/<tag>.srs` | `DataDir` |
| `TailscaleDirName`, `TempDirName`, `DaemonIdentityDirName`, `RemoteDaemonsDirName` | platform_common.go:86-96 | константы | перенести в `internal/constants` на этапе 3 |
| `GetTailscaleStateDir` (этап 2) | platform_common.go:100 | `bin/tailscale` | `DataDir` |
| `GetTempDir` (этап 2) | platform_common.go:106 | `temp` (НЕ под `bin/`) | `DataDir` |
| `GetDaemonIdentityDir` (этап 2) | platform_common.go:112 | `bin/daemon` | `DataDir` |
| `GetRemoteDaemonIdentityDir` (этап 2) | platform_common.go:118 | `bin/remote-daemons/<id>` | `DataDir` |
| `GetRuleSetsDirFor` | platform_common.go:132 | local → `bin/rule-sets`, remote → `…/remote/<id>/srs` | `DataDir` |
| `GetSubscriptionsDirFor` | platform_common.go:149 | local → `bin/subscriptions`, remote → `…/<id>/subscriptions` | `DataDir` |
| `GetWizardTemplatePath` | platform_common.go:160 | `bin/wizard_template.json` | **два варианта**: `GetWizardTemplatePath(DataDir)` — цель скачивания; `AppDir.Bin()+имя` — поставляемый seed; чтение через резолвер §3.3 (этап 5) |
| `GetWizardStatesDir` | platform_common.go:167 | `bin/wizard_states` | `DataDir` |
| `GetWizardStatePath` | platform_common.go:177 | `bin/wizard_states/state.json` | `DataDir` |
| `GetWizardStatesDirFor` | platform_common.go:200 | local/remote каталог состояний | `DataDir` |
| `GetWizardStatePathFor` | platform_common.go:210 | `…/state.json` для таргета | `DataDir` |
| `GetOutboundsCachePath` | platform_common.go:231 | `bin/outbounds.cache.json` (legacy, только удаляется) | `DataDir` |
| `GetSubscriptionsDir` | platform_common.go:239 | `bin/subscriptions` | `DataDir` |
| `GetLogsDir` | platform_common.go:244 | `logs` | `LogDir` (вырождается, см. выше) |
| `EnsureDirectories` | platform_common.go:249 | MkdirAll `logs`, `bin`, `bin/rule-sets` | `Layout`: только `Data/bin`, `Data/bin/rule-sets`, `Logs` |
| `GLStatePath` | glstate.go:84 | `bin/gl-state.json` | `DataDir` |
| `LoadGLState` / `SaveGLState` | glstate.go:90 / :106 | чтение/запись gl-state (`SaveGLState` сам строит путь на :111, дубль `GLStatePath`) | `DataDir` |
| `MarkGLStarting` / `UpdateGLState` / `MarkGLRendered` | glstate.go:132 / :142 / :157 | мутация gl-state | `DataDir` |
| `IsMesaInstalled` / `IsMesaDisabled` / `HasMesaBundle` / `HasForeignOpenGL` | glstate.go:193 / :199 / :205 / :210 | `opengl32.dll`, `libgallium_wgl.dll`, `*.off`, `mesa3d/` рядом с exe | `AppDir` (чтение) |
| `DisableMesa` / `EnableMesa` / `copyMesaFromBundle` / `removeMesaFiles` | glstate.go:222 / :249 / :281 / :325 | переименование/копирование DLL рядом с exe | `AppDir` — **исключение §3/§5**, пишет в AppDir |
| `EnsureDesktopOpenGL` | glprobe_windows.go:323 (стаб glprobe_stub.go:19) | gl-state + Mesa | **два корня**: `(AppDir, DataDir)` или `Layout` |
| `restartToApply` | glprobe_windows.go:534 | `UpdateGLState` | `DataDir` |
| `installAndVerifyMesa` | glprobe_windows.go:602 | копия/скачивание Mesa + gl-state | `(AppDir, DataDir)` |
| `startBackgroundHardwareProbe` | glprobe_windows.go:669 | проба + gl-state | `(AppDir, DataDir)` |
| `probeHardware` | glprobe_windows.go:706 | читает `opengl32.dll` рядом с exe | `AppDir` |
| `downloadMesa` | glprobe_windows.go:798 | распаковывает DLL в каталог exe (tmp — `os.CreateTemp("")`) | `AppDir` — **исключение**, пишет в AppDir |
| `probeDesktopOpenGL` (`-gl-probe-local`) | glprobe_windows.go:189-193, :742 | `os.Executable()` → `opengl32.dll`, перезапуск себя с флагом | `AppDir` (по TASKS: `RunGLProbeChild` получает `AppDir`) |
| `GetWintunPath` | platform_windows.go:23 (стабы platform_darwin.go:19, platform_linux.go:22 → `""`) | `bin/wintun.dll` | этап 3: `DataDir`; этап 5: `Dir(SingboxPath)/wintun.dll` |
| `ResolveSingboxExecPath` | singbox_exec_path.go:7 (non-linux: bundled), singbox_exec_path_linux.go:14 (PATH первым) | путь ядра | этап 5: цепочка `SINGBOX_LAUNCHER_CORE` → Data → App → PATH, возвращать и источник |
| `WritePrivilegedStartScript` | privileged_darwin.go:115 (стаб privileged_stub.go:27) | скрипт/pid в `binDir`, лог в `logPath` | `binDir` ← `DataDir`, `logPath` ← `LogDir` |

### 1.2 Хелперы вне `internal/platform` (принимают `execDir`/`binDir`)

| Функция | Файл:строка | Корень | Тип |
|---|---|---|---|
| `locale.GetLocaleDir(binDir)` | internal/locale/locale.go:384 (литерал `"locale"` на :385) | `bin/locale` | **два**: App (поставляемые) + Data (скачанные) |
| `locale.LoadExternalLocales(dir)` | locale.go:255 | каталог | вызывать дважды: App, затем Data (§3.3) |
| `locale.DownloadAllRemoteLocales(dir)` | locale.go:367 | запись | `DataDir` |
| `locale.LoadSettings` / `SaveSettings` | internal/locale/settings.go:236 / :259 (литерал `"settings.json"`) | `bin/settings.json` | `DataDir` |
| `locale.MarkTemplateInstalled` / `MarkLocalesRefreshed` | settings.go:214 / :225 | штампы в settings.json | `DataDir` |
| `template.LoadTemplateData` | core/template/loader.go:254 | чтение шаблона | **M** → резолвер App/Data (этап 5) |
| `template.DownloadTemplate` | core/template/download.go:52 | запись шаблона + `MarkTemplateInstalled` (:90) | `DataDir` |
| `template.EnsureTemplate` | core/template/download.go:136 | чтение, при провале скачивание | **M** (`Layout`) |
| `wizardbusiness.DefaultTemplateLoader.LoadTemplateData` | ui/configurator/business/template_loader.go:26 (интерфейс :19) | обёртка | **M** |
| `core.RefreshTemplateIfStale` | core/template_migration.go:69 | маркер `wizard_template.version` (:99), штамп, скачивание | **M** (`Layout`: маркер из App, штамп/скачивание в Data) |
| `core.stampTemplateCheck(binDir)` | template_migration.go:136 | штамп | `DataDir` |
| `config.BuildVarSubstituterFromDisk` | core/config/varsubst.go:125 | шаблон + state | **M** |
| `config.loadTemplateVarDefaults` / `loadStateSettingsVars` | varsubst.go:156 / :248 | шаблон / state | M / D |
| `config.SetTailscaleStateDirRoot` | вызов core/controller.go:305 | `bin/tailscale` | `DataDir` |
| `snapshot.Build` | core/snapshot/snapshot.go:60 (пути :62-65) | template/state/cache/config | **M** (template из резолвера, остальное Data) |
| `core.DaemonIdentityDir` | core/backend_daemon_darwin.go:122 | `bin/daemon` | `DataDir` |
| `services.NewRemoteRegistry` | core/services/lxd_remote_registry.go:87 (пути :92, :100, :128, :370-392, :529) | `bin/remote-daemons.json`, `bin/remote-daemons/<id>`, remote-машины | `DataDir` |
| `services.MigrateLegacyRemoteProfile` | core/services/lxd_remote_migration.go:34 (:39-86) | remote/ + `bin/remote-config.json` | `DataDir` |
| `services.RuleSRSPath` / `RuleSRSPathFor` / `SRSFileExists*` / `AllSRSDownloaded*` / `DownloadSRSGroup*` / `DeleteOrphanRuleSets*` | core/services/srs_downloader.go:29-319 | `.srs` | `DataDir` |
| `services.localResourceFiles` / `CollectDeployResources` / методы `r.execDir` | core/services/lxd_remote_resources.go:125, :160, :178, :203, :228; lxd_remote_deploy.go:48 | remote `srs/`, config | `DataDir` |
| `build.convertPresetRuleSetRemoteToLocal` / `CollectSrsCachedPaths` | core/build/preset_merge.go:44 / :730 | абсолютный путь `.srs` в config.json | `DataDir` |
| `build.convertRuleSetToLocalRequired` | core/build/route_merge.go:189 | то же через `services.RuleSRSPath` | `DataDir` |
| `state.MigrationReportPath` / `PersistMigrationReport` / `ReadMigrationReport` / `ClearMigrationReport` | core/state/migration_report.go:35 / :106 / :127 / :143 | `bin/…` отчёт миграции | `DataDir` |
| `state` load-router | core/state/load_router.go:203-217 | выводит `bin/subscriptions` из пути state.json (относительно) | не трогать: работает под любым корнем |
| `core.directionBuildOptions` | core/config_service.go:560 | шаблон | **M** |
| `core.collectAllStageRuleSetTags` | config_service.go:409 | states | `DataDir` |
| `core.refreshSubscriptionsMetaAndCache` / `persistFetchResultForSource` | config_service_subscriptions.go:43 / :353 | settings + state | `DataDir` |
| `core.loadTemplateForBuild` | core/rebuild.go:388 | шаблон с докачкой | **M** |
| `core.cleanupLegacyOutboundsCache` | rebuild.go:458 | legacy cache | `DataDir` |
| `core.buildSnapshotFromState` | core/rebuild_snapshot.go:39 (execDir → varsubst :72) | M через varsubst | **M** |
| `main.rememberOfferedRenderer` | main.go:57 | gl-state | `DataDir` |
| `wizardbusiness.ListCloneSources` / `stateExistsFor` / `LoadCloneState` | ui/configurator/business/clone_source.go:116 / :149 / :175 | states | `DataDir` |
| `tabs.currentIdentityDefaults` | ui/configurator/tabs/source_identity_block.go:338 | settings.json | `DataDir` |
| `dialogs.fetchOrLoadGetFree` | ui/configurator/dialogs/get_free_dialog.go:67 | `bin/get_free.json` (в дистрибутив не кладётся) | `DataDir` |
| `configurator.maybeShowMigrationReport` / `migrationReportBody` | ui/configurator/migration_report_dialog.go:49 / :93 | binDir | `DataDir` |
| `ui.buildSubscriptionDefaultsBlock` / `buildSubscriptionIdentificationBlock` | ui/settings_tab.go:285 / :478 | binDir → settings | `DataDir` |

Правило для **M**: функции, которым нужны шаблон и state одновременно
(`BuildVarSubstituterFromDisk`, `snapshot.Build`, `EnsureTemplate`,
`loadTemplateForBuild`, `buildSnapshotFromState`, `RefreshTemplateIfStale`),
получают `paths.Layout` (или пару `AppDir, DataDir`); чисто шаблонные
(`LoadTemplateData`, `directionBuildOptions`) — тот же резолвер шаблона
этапа 5. До этапа 5 резолвер может смотреть только в Data, но сигнатуру
лучше сразу завести двухкорневой, чтобы не перекраивать вызовы дважды.

---

## 2. Обращения к `ExecDir` по файлам (190 строк)

### main.go (12) — порядок старта см. §3.4

| Строки | Класс | Что |
|---|---|---|
| :57-60 (`rememberOfferedRenderer`) | D | `UpdateGLState` → gl-state.json |
| :77 | — | комментарий |
| :78-93 (не `ExecDir`, `os.Executable`) | L | `crash.log` (:80) и `native-stderr.log` (:90) в `GetLogsDir(Dir(exe))` **до** контроллера; этап 3 — `Layout.Logs` |
| :100 | A | `RunGLProbeChild(local)` — этап 3 даёт `AppDir` |
| :114 | — | первая WARN-строка `exec=%s`; этап 3 — `layout.LogLine()` |
| :124 | M | `EnsureDesktopOpenGL(execDir)` → App (Mesa) + Data (gl-state) |
| :143-144 | D | `MigrateLegacyRemoteProfile`, `NewRemoteRegistry` |
| :149-151 | M | `binDir` → `LoadExternalLocales(GetLocaleDir)` (App+Data, §3.3) и `LoadSettings` (D) |
| :158-179 | D | докачка локалей после апдейта: `MarkLocalesRefreshed`, `DownloadAllRemoteLocales` в Data |
| :328 | M | `glExecDir` — ниже им пользуются Data- и App-вызовы (строки :330-:359 попадают в grep по подстроке) |
| :330 | D | `MarkGLRendered` |
| :346 | D | `rememberOfferedRenderer` |
| :350 | A | `DisableMesa` — исключение |
| :359 | D | `UpdateGLState` |
| :379 | D | `GetWizardStatePath` (информационное чтение state) |
| :620 | — | `RestartSelf()` в конце `main()` |

### core/services/file_service.go (14)

| Строки | Класс | Что |
|---|---|---|
| :6, :44-46 | — | док и поле `ExecDir` (удаляется на этапе 3) |
| :82-86 | — | `os.Executable` → `ExecDir`; этап 3 — берётся из `Layout` |
| :88 | D+L | `EnsureDirectories` → `EnsureDirectories(Layout)` |
| :92 | D | `ConfigPath` |
| :94 | D | `SingboxBundledPath = GetBinDir/…` — цель скачивания ядра (этап 5: Data) |
| :95 | M | `SingboxPath = ResolveSingboxExecPath(…)` — этап 5: цепочка Data → App → PATH |
| :96 | D | `WintunPath` (этап 5: от каталога ядра) |
| :107, :114, :122 | L | `OpenLogFiles`: `Join(ExecDir, "logs/…")` |
| :143-145 | L | `ReopenChildLogFile` (`rel` по умолчанию `logs/sing-box.log`) |
| :204 | L | сравнение пути в `CheckAndRotateLogFile` |

Относительные имена логов — `core/controller.go:39-41`: `"logs/" + constants.*LogFileName`
(литерал `"logs/"` оставлен этапом 2, см. отчёт); на этапе 3 `OpenLogFiles`
должен принимать `LogDir` и голые имена файлов, а `ChildLogRelativePath`
заменить абсолютным `ChildLogPath` (им пользуются controller.go:561,
process_service.go:228/:265, log_viewer_window.go:105).

### core/controller.go (3)

| Строки | Класс | Что |
|---|---|---|
| :304-305 | D | `SetTailscaleStateDirRoot(GetTailscaleStateDir)` |
| :561 | L | `RunHidden`: сравнение `logPath` с child-логом |

### core/process_service.go (4)

| Строки | Класс | Что |
|---|---|---|
| :225 | D | `SingboxCmd.Dir = bin` — **cwd ядра**: относительные пути конфига (`cache_file` и т. п.) резолвятся от DataDir/bin |
| :228 | L | ротация child-лога |
| :263 | D | `binDir` для privileged-скрипта macOS (скрипт и pid в bin) |
| :265 | L | `logPath` child-лога для privileged-старта |

### core/config_service.go (4), config_service_context.go (6), config_service_subscriptions.go (2)

| Строки | Класс | Что |
|---|---|---|
| config_service.go:49 | D | `LoadSubscriptionSettingsFunc` → settings.json (HWID) |
| config_service.go:184 | M | `execDir` → `BuildVarSubstituterFromDisk` + `directionBuildOptions` (шаблон) |
| config_service.go:223 | M | то же + `refreshSubscriptionsMetaAndCache` (D) + `LoadTemplateData` (:249) |
| config_service.go:364 | D | state path |
| config_service_context.go:7-9 | — | комментарий |
| config_service_context.go:74-87 | D | `Route.ExecDir`, `PresetMergeContext.ExecDir`, `CollectSrsCachedPaths` (`.srs`) |
| config_service_subscriptions.go:327, :404 | D | settings + state |

### core/rebuild.go (2), rebuild_corereject.go (3)

| Строки | Класс | Что |
|---|---|---|
| rebuild.go:108 | M | `execDir`: state (:119, D), legacy cache (:116, D), `loadTemplateForBuild` (:129, M), `buildSnapshotFromState` (:135/:161/:243, M), GC `.srs` (:317-318, D) |
| rebuild.go:445 | M | `CleanOrphanRuleSets`: шаблон (:446) + GC (D) |
| rebuild_corereject.go:636 | D | state |
| rebuild_corereject.go:649, :654 | M | шаблон и снапшот |

### core/template_migration.go (1)

| Строки | Класс | Что |
|---|---|---|
| :156 | M | `StartTemplateRefresh` → `RefreshTemplateIfStale(execDir)`; внутри :76 settings (D), :86 state (D), :90 шаблон, :99 маркер `wizard_template.version` (A по §3.3), :126 скачивание (D) |

### core/debugapi_wiring.go (7), debugapi_wiring_daemon_darwin.go (1)

| Строки | Класс | Что |
|---|---|---|
| :57-61 | — | `GetExecDir()` фасада (шов, §3.2) |
| :107, :120 | D | state load/save |
| :141 | M | `LoadTemplateData` |
| :190 | D | `NewRemoteRegistry` |
| :197 | D | `RemoteAPI.ExecDir` |
| debugapi_wiring_daemon_darwin.go:90 | D | settings (backend mode) |

### core/debugapi/* (9)

| Строки | Класс | Что |
|---|---|---|
| server.go:55-57 | — | `ControllerFacade.GetExecDir()` |
| settings_endpoints.go:11 | — | комментарий |
| settings_endpoints.go:52 | D | settings.json (User-Agent) |
| snapshot.go:22 | M | `snapshot.Build(GetExecDir())` |
| state_endpoints.go:62 | D | state |
| remote_endpoints.go:37-39 | — | поле `RemoteAPI.ExecDir` → `DataDir` |
| remote_endpoints.go:577 | D | remote config |
| remote_resources_endpoints.go:66, :77 | D | remote config, `CollectDeployResources` |
| remote_state_endpoints.go:22 | D | remote state |

### core/daemon_manager_darwin.go (12), backend_daemon_darwin.go (3), backend.go (1)

Всё **D**: settings.json (`binDir` на daemon_manager_darwin.go:161, :263,
:291, :302, :319, :342; backend_daemon_darwin.go:130, :440; backend.go:471),
клиентская пара `DaemonIdentityDir` (daemon_manager_darwin.go:173, :232,
:254, :282, :287 — legacy secret; backend_daemon_darwin.go:141), реестр
remote (daemon_manager_darwin.go:251). `SingboxPath` — в sudo-командах
службы (:204 `CoreSupportsLxd`, :402, :421, :427) и в plist
(`daemonSystemPlistPath` :59, `DaemonStatusSnapshot` :160 — точка проверки
`ProgramArguments[0]` по §5.1, этап 10).

### Прочие core

| Файл:строка | Класс | Что |
|---|---|---|
| core/auto_update.go:119, :273 | D | state |
| core/auto_update.go:128 | D | settings |
| core/log_level.go:33, :62 | D | state |
| core/tray_menu.go:203 | D | settings (`HideAppFromDock`) |
| core/version_marks.go:35 | D | settings (штампы версий) |
| core/core_downloader.go:92 | D | `GetTempDir` для распаковки ядра |
| core/core_downloader.go:121, :131 (не `ExecDir`) | D | установка ядра в `SingboxBundledPath` и спутников (`libcronet`) в его каталог |
| core/wintun_downloader.go:58 | D | `GetTempDir`; цель — `WintunPath` (:37, :179, :203) |
| core/core_version.go:57 | A | `GetCoreBinaryPath`: путь ядра относительно `ExecDir` для показа; этап 5 — показывать источник (`data`/`app`/`PATH`/`env`) |
| core/services/srs_downloader.go:25 | — | комментарий |
| core/build/preset_merge.go:162-163, :353, :632 | D | поле `PresetMergeContext.ExecDir` → `ResolveRouteWithGlobals` |
| core/build/route_merge.go:43-52, :117 | D | поле `RouteConfig.ExecDir` → `convertRuleSetToLocalRequired` |
| core/config/endpoint_schemes.go:49 | — | комментарий |

### ui/configurator (52)

| Файл:строки | Класс | Что |
|---|---|---|
| models/wizard_model.go:223-224 | — | поле `WizardModel.ExecDir` → `DataDir` (шов, §3.3) |
| models/wizard_model.go:230 | — | комментарий |
| models/wizard_model.go:316 | D | `SrsDir()` → `GetRuleSetsDirFor` |
| configurator.go:100 | D | `SrsLocalDir` для remote |
| configurator.go:140, :821 | M | `LoadTemplateData` |
| configurator.go:147 | M | путь шаблона в логе |
| configurator.go:156 | M | `EnsureTemplate` |
| configurator.go:164, :380, :828 | D | каталог для диалога ручного скачивания шаблона |
| configurator.go:224 | D | `model.ExecDir = …` — единственное место, где модель получает корень |
| configurator.go:309, :879 | D | `maybeShowMigrationReport(bin)` |
| configurator.go:897 | D | клон: `ListCloneSources`, `LoadCloneState` |
| business/interfaces.go:37 | — | `FileServiceInterface.ExecDir()` (шов) |
| business/file_service_adapter.go:22-23 | — | реализация |
| business/state_store.go:45, :59 | D | `NewStateStore`, `NewStateStoreFor` |
| business/create_config.go:189-190, :316 | D | `CollectSrsCachedPaths`, `PresetMergeContext.ExecDir`, `RouteConfig.ExecDir` |
| presentation/presenter_save.go:253, :267 | D | state для таргета |
| presentation/presenter_save.go:408 | D | remote config |
| presentation/presenter_state.go:268 | D | state |
| tabs/rules_tab.go:114, :126, :131, :304, :369, :506, :599 | D | скачивание и проверка `.srs`, подсказка каталога |
| tabs/rules_unified_rows.go:314-315, :399, :407, :483 | D | проверка `.srs` |
| tabs/source_edit_window.go:1020 | D | `currentIdentityDefaults(m.ExecDir)` → settings |
| tabs/source_tab.go:646 | D | settings (`DefaultSubscriptionReload`) |
| tabs/settings_tun_darwin.go:89-90 | M | `binDir` (D: `cache.db` из experimental) и `execDir` как корень для `logs/sing-box.log` (:114, L) и страж `pathUnderRoot(execDir, …)` — на этапе 4 страж для логов должен смотреть на `LogDir` |
| dialogs/get_free_dialog.go:153 | D | `fetchOrLoadGetFree` |

Все читатели `model.ExecDir` (20): create_config.go ×3, rules_tab.go ×7,
rules_unified_rows.go ×5, source_edit_window.go, source_tab.go,
configurator.go:879, wizard_model.go:316; писатель — configurator.go:224.

### ui (прочее, 40)

| Файл:строки | Класс | Что |
|---|---|---|
| core_dashboard_tab.go:524 | D | «Open bin folder» в подсказке про ядро (куда класть скачанное) |
| core_dashboard_tab.go:589, :626, :646, :800, :882-883, :907-909 | D | снапшоты состояний (`GetWizardStatesDir`/`GetWizardStatePath`) |
| core_dashboard_tab.go:941 | D | `DownloadTemplate` |
| core_dashboard_tab.go:947, :1001, :1130 | D | каталог в диалоге провала скачивания (шаблон / ядро / wintun) |
| core_dashboard_tab.go:1046 | D | «Open bin folder» в подсказке wintun (этап 5: каталог ядра) |
| core_dashboard_tab_status.go:180, :291 | D | наличие state.json |
| core_dashboard_tab_status.go:252 | M | наличие шаблона → кнопка Download; этап 5 — тот же резолвер (найден в App или Data) |
| diagnostics_tab.go:320 | L | «Logs folder» |
| diagnostics_tab.go:331 | D | «Config folder» (bin) |
| diagnostics_tab.go:411 | M | `buildMesaToggleButton` (:407): Mesa-проверки :414-433 и `Disable/EnableMesa` :453/:456 — **A**, `UpdateGLState` :467 — **D** |
| log_viewer_window.go:105 | L | путь `sing-box.log` |
| traffic_bootstrap.go:63 | — | комментарий |
| traffic_bootstrap.go:95 | L | `sing-box.log` для профайлера |
| settings_tab.go:54 | D | `binDir` на всю вкладку (settings, локали :204) |
| settings_tab.go:615 | D | `buildDebugAPIRow` settings |
| connection_local.go:62 | D | settings (backend mode) |
| connection_local_daemon_darwin.go:50 | D | settings (панель демона, :104-231) |
| clash_api_tab.go:1751 | D | settings (ping) |
| clash_api_tab_render.go:38, :59, :62 | D | remote config / state |
| lxd_remote_override.go:44, machine_add_window.go:136, machine_list_panel.go:126, machine_resources_window.go:35 | D | `NewRemoteRegistry` |
| machine_edit_window.go:325, machine_list_panel.go:930 | D | remote state / config |

Итог по классам (190 строк `ExecDir`; `glExecDir` в main.go тоже попадает в grep):
D ≈ 121, M ≈ 24 (включая file_service.go:88 D+L), L ≈ 11, A = 2
(core_version.go:57, main.go:350), комментарии и объявления ≈ 32.

---

## 3. Швы

### 3.1 Поля `FileService` (core/services/file_service.go:43-74) и их читатели

| Поле | Читатели |
|---|---|
| `ExecDir` | 190 строк §2 |
| `ConfigPath` | controller.go:322, :385-386, :850-851, :916; process_service.go:208, :223, :264; rebuild.go:233; own_tun_names.go:28; backend_daemon_darwin.go:327; debugapi_wiring.go:54; ui/clash_api_tab.go:172, :387, :1451, :1486, :1810; clash_api_tab_render.go:41; traffic_bootstrap.go:202; core_dashboard_tab_status.go:227; configurator/presentation/draft_reject.go:58; FileServiceAdapter.ConfigPath (file_service_adapter.go:18) |
| `SingboxBundledPath` | core_downloader.go:121 (установка), :131 (спутники) |
| `SingboxPath` | controller.go:270 (→ `uiservice.SingboxPath`, ui_service.go:90/146/254), :590, :594; process_service.go:181, :184, :223, :273; core_version.go:30-34, :56; core_capabilities.go:45, :137, :223; core_chain_capability.go:51; rebuild.go:232; daemon_manager_darwin.go:204, :402, :421, :427; configurator/presentation/draft_reject.go:86; FileServiceAdapter.SingboxPath (:26) |
| `WintunPath` | wintun_downloader.go:37 (`CheckWintunDLL`), :179, :203, :213 (`DownloadWintunDLL`) |
| `ChildLogRelativePath` | file_service.go:106, :141, :203-204; log_viewer_window.go:105 |

### 3.2 Debug API

- `ControllerFacade.GetExecDir()` — core/debugapi/server.go:57; реализация
  core/debugapi_wiring.go:57-62; фейк в тестах server_test.go:54.
  Читатели: settings_endpoints.go:52 (D), snapshot.go:22 (M), state_endpoints.go:62 (D).
  TASKS: → `GetDataDir()` (+ `GetAppDir()`/`Layout` для снапшота, где шаблон M).
- `RemoteAPI.ExecDir` — remote_endpoints.go:39; заполняется debugapi_wiring.go:197;
  читатели remote_endpoints.go:577, remote_resources_endpoints.go:66/:77,
  remote_state_endpoints.go:22. Всё D.

### 3.3 Мастер

- `WizardModel.ExecDir` — models/wizard_model.go:224, пишется configurator.go:224.
- `FileServiceInterface.ExecDir()` — business/interfaces.go:37; реализация
  file_service_adapter.go:22; создаётся configurator.go:291, :816,
  presentation/presenter_state_helpers.go:246; потребители
  state_store.go:45 (`NewStateStore`), :59 (`NewStateStoreFor`,
  presenter_state_helpers.go:258). Всё D → `DataDir()`.

### 3.4 Порядок старта

1. main.go:64 `main()`; флаги :66-70 (`-start`, `-tray`, `-gl-probe`, `-gl-probe-local`; сюда же `-paths`, `-purge-data`, `-yes`).
2. main.go:78-93 — `os.Executable`, `crash.log` + `native-stderr.log` в `Dir(exe)/logs`.
3. main.go:99-101 — `RunGLProbeChild` (выход, если флаг).
4. main.go:105 → `core.NewAppController` (core/controller.go:245):
   - :250 `services.NewFileService()` (file_service.go:79: `os.Executable` :82, `EnsureDirectories` :88, пути :92-96);
   - :257 `OpenLogFiles(logFileName, childLogFileName, apiLogFileName)` (константы :39-41);
   - :260 `api.SetAPILogFile`; :267 `NewUIService(…, SingboxPath)`;
   - :305 `SetTailscaleStateDirRoot`; :376 `initBackendFromSettings` (backend.go:471).
5. main.go:113 — WARN-строка старта (`exec=`), место для `layout: …`.
6. main.go:124 `EnsureDesktopOpenGL`; :134 `StartTemplateRefresh` (template_migration.go:150; горутины `CheckVersionMarks` и `RefreshTemplateIfStale` :185).
7. main.go:143 `MigrateLegacyRemoteProfile`; :149-180 локали и settings; :214 `StartDebugAPI` (debugapi_wiring.go:178).
8. main.go:328-365 — GL после старта цикла событий; :379 чтение state.
9. main.go:437 `CheckIfLauncherAlreadyRunningUtil` (controller.go:860, `os.Executable` :866).
10. Выход: `GracefulExit` (controller.go:437) → `CloseLogFiles` (:523); main.go:619-620 `RestartSelf`.

`os.Executable()` вне `internal/paths`: main.go:78, controller.go:866 (имя
процесса, не путь данных — оставить), file_service.go:82,
platform/restart_windows.go:33, glprobe_windows.go:189, :742.

---

## 4. Поставляемое vs скачанное

| Что | Чтение | Запись/скачивание | Этап 5 |
|---|---|---|---|
| Шаблон `wizard_template.json` | `template.LoadTemplateData` (loader.go:254) ← config_service.go:249, :561; rebuild.go:389, :446; debugapi_wiring.go:141; configurator.go:140, :821 (через template_loader.go:26); `EnsureTemplate` (download.go:137, :146); напрямую по пути: varsubst.go:161, snapshot.go:62, template_migration.go:90 (stat), core_dashboard_tab_status.go:252 (stat) | `DownloadTemplate` (download.go:52) ← template_migration.go:126, core_dashboard_tab.go:941, `EnsureTemplate` ← rebuild.go:396, configurator.go:156 | один резолвер для всех 13 точек чтения; запись только Data |
| Маркер `wizard_template.version` | template_migration.go:99 | CI `win64-full` (.github/workflows/ci.yml:721) | читать из App |
| Штамп `LastTemplateLauncherVersion` | template_migration.go:77 | `MarkTemplateInstalled` (settings.go:214) ← download.go:90, template_migration.go:137 | Data |
| Локали `bin/locale/*.json` | main.go:150 (`LoadExternalLocales`) | main.go:171, settings_tab.go:205 (`DownloadAllRemoteLocales`) | загрузка App → Data; скачивание в Data |
| Штамп `LastLocaleLauncherVersion` | main.go:158 | `MarkLocalesRefreshed` ← main.go:165, :175 | Data |
| Ядро | `SingboxPath` (file_service.go:95; `ResolveSingboxExecPath`) | `SingboxBundledPath` (file_service.go:94) ← core_downloader.go:121; спутники :131-137 | `SINGBOX_LAUNCHER_CORE` → Data → App → PATH |
| wintun.dll | `CheckWintunDLL` (wintun_downloader.go:30-40) | `DownloadWintunDLL` (:44; запись :179-213) | `Dir(SingboxPath)/wintun.dll` |
| libcronet | загрузчик ОС рядом с ядром | core_downloader.go:131-137 | от каталога ядра |
| Mesa3D (`mesa3d/`, `opengl32.dll`) | glstate.go:193-212, glprobe_windows.go:193 | glstate.go:222-330, glprobe_windows.go:798 | остаётся App (исключение) |
| `get_free.json` | get_free_dialog.go:67-80 | `downloadGetFree` в тот же путь | Data (в zip не входит) |

---

## 5. Тесты, которые сломаются при смене сигнатур

| Файл | Что делает с путями |
|---|---|
| internal/platform/platform_common_test.go | строковые хелперы от фиксированного корня |
| internal/platform/target_paths_test.go | `GetWizardStatesDirFor`/`…For` от `/opt/sbl` |
| internal/platform/glstate_test.go | `LoadGLState`/`SaveGLState` в `t.TempDir()` |
| core/build/route_merge_test.go | `stubSRSFile` в `execDir/bin/rule-sets`, `RouteConfig{ExecDir}` (:468, :502) |
| core/build/resource_path_test.go | `convertRuleSetToLocalRequired(in, execDir, …)` |
| core/build/dns_ruleset_dangling_test.go | `PresetMergeContext{ExecDir}` (:69) |
| core/build/srs_filename_test.go | `CollectSrsCachedPaths(rules, "/exec", "")`, ожидание через `filepath.Join` (этап 2) |
| core/build/golden_test.go | `ExecDir` пустой (коммент :407) — сигнатуры не заденут, если поле останется строкой-типом |
| core/services/srs_downloader_test.go, srs_isolation_test.go, lxd_remote_deploy_resources_test.go | `.srs`-каталоги в `t.TempDir()` |
| core/config/varsubst_test.go | `BuildVarSubstituterFromDisk` от temp-макета с шаблоном и state |
| core/config/tailscale_state_dir_test.go | `SetTailscaleStateDirRoot(t.TempDir())` |
| core/snapshot/snapshot_test.go | `snapshot.Build(execDir)` от temp-макета |
| core/template/download_test.go | `DownloadTemplate`/`EnsureTemplate` в `t.TempDir()`, свой `templatePath` с литералом `"bin"` (:36) |
| core/template_migration_test.go | `RefreshTemplateIfStale(d.root)`, `FileService{ExecDir: d.root}` (:272-273) |
| core/spec129_record_vars_test.go | `FileService{ExecDir, ConfigPath}` (:70) |
| core/etalon_v6mig_capture_test.go | temp-макет `bin/subscriptions` |
| core/state/migration_scenarios_test.go | temp-макет `bin/wizard_states`, `bin/subscriptions` литералами |
| core/integration_test.go | `services.NewFileService()` (:73, :255) — сменится на `NewFileService(layout)` |
| core/core_downloader_extract_test.go | только `GetExecutableNames` — вероятно не заденет |
| core/debugapi/server_test.go | `fakeFacade.GetExecDir` (:54) |
| core/debugapi/snapshot_test.go, ui_endpoints_test.go | `snapshotLayout` в temp, фасад с `execDir` |
| core/debugapi/remote_endpoints_test.go, raw_endpoints_test.go, schema_gate_test.go | `RemoteAPI{ExecDir}` (:34), реестр/состояния машин в temp |
| ui/configurator/business/clone_source_test.go | `ListCloneSources`/`LoadCloneState(execDir)` |
| ui/configurator/business/row_refresh_writeback_test.go | `FileService{ExecDir}` (:49, :214) |
| ui/configurator/business/empty_model_gates_test.go, final_build_report_test.go, preview_target_test.go, srs_rule_reopen_test.go, wizard_integration_test.go | `findProjectRoot` как `execDir`: читают **поставляемый** `bin/wizard_template.json` репозитория (по смыслу AppDir) и ставят `model.ExecDir` |
| ui/configurator/presentation/backup_restore_dns_route_test.go, preset_toggle_canonical_test.go | `m.ExecDir = t.TempDir()` / корень |

---

## 6. Прочее (для этапов 7–9)

- **`platform.RestartSelf`** — restart_windows.go:32 (`exec.Command(exe, os.Args[1:]…)`,
  `cmd.Dir = Dir(exe)`, DETACHED); restart_other.go:12 — ошибка «not supported».
  Для переключателя Portable на Linux нужен рабочий вариант вне Windows.
  Вызов — только main.go:620 по флагу `RequestRestartAfterExit`/`RestartRequested`
  (glstate.go:177/:180), после `GracefulExit`.
- **`platform.OpenFolder`** — platform_windows.go:28, platform_darwin.go:24,
  platform_linux.go:27. Вызовы: diagnostics_tab.go:321, :332;
  core_dashboard_tab.go:533, :1055; internal/dialogs/dialogs.go:151 (кнопка в
  `ShowDownloadFailedManual` :91 / `…WithReason` :103).
- **`CleanupGhostSingboxTunAdapters(mode)`** — wintun_cleanup_windows_device.go:34,
  стаб wintun_cleanup_other.go:16; вызовы process_service.go:58
  (`runGhostTunCleanup`), :101 (`CleanupStaleTunAtStart`).
- **`CleanupOrphanSingTunFirewallRules()`** — singtun_fwrules_windows.go:28,
  стаб singtun_fwrules_stub.go:9; вызов controller.go:832
  (`cleanupOrphanSingTunFirewallRulesAtStart` :825).
- **Diagnostics**: «Logs folder» diagnostics_tab.go:319-325 (`GetLogsDir`),
  «Config folder» :330-336 (`GetBinDir`); Mesa-кнопка `buildMesaToggleButton`
  :407 (возвращает nil вне Windows / без Mesa).
- **Settings** (ui/settings_tab.go): `BuildSettingsContent` :53, вызывается из
  ui/app.go:86 в `WrapInScrollWithGutter`. Раздел = жирный заголовок
  `widget.NewLabelWithStyle(locale.T("…"), Leading, Bold)` + строки;
  примеры: Connection :75, Language :175-232 (`container.NewBorder(nil, nil, label, btn, select)` на :232).
  Сборка — `container.NewVBox(…)` на :247-264, разделы через `widget.NewSeparator()`;
  Storage встаёт отдельным блоком перед/после Debug API.
  Кнопка «Copy API info» — :653-676 (`widget.NewButtonWithIcon(…, theme.ContentCopyIcon(), …)`,
  `ac.UIService.Application.Clipboard().SetContent`, подтверждение
  `dialogs.ShowAutoHideInfo`) — образец для «Copy paths». `buildDebugAPIRow` :614.
- **Debug API**: реестр эндпоинтов `(s *Server) endpoints()` server.go:239-304
  (строка `{"GET", "/path", auth, "summary", handler}`; `/debug/snapshot` на :247 —
  рядом место для `/debug/paths`); `routes()` :309-329 регистрирует из реестра,
  манифест `/` и `/help` строятся из него же (`handleManifest` :335,
  `handleHelp` :361) — отдельной записи в манифест не нужно. manifest.go:
  константы :14-23, `DocsURL` :33, `apiEndpoint` :45, `ConnectionCardJSON` :76.
  Проверка манифеста — debugapi/manifest_test.go:49 (`TestManifestAndHelp`);
  документация эндпоинтов — docs/API.md.
- **Закрытие логов для очистки**: `FileService.CloseLogFiles` file_service.go:161;
  `debuglog` crash/native stderr открыты main.go:81/:90 до контроллера — их
  тоже закрывать перед удалением LogDir (Windows держит дескрипторы).
