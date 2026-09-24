# SPEC 135 · CODEMAP обращений к путям

Карта для этапов 3–5 и 7–9. §1 и §3 — по дереву после этапа 4 (новые
сигнатуры и швы); §2 и §5 — снимок до этапа 3 (коммит `32c21ec9`): строки
`ExecDir` оттуда все переведены, `ExecDir` в `.go` больше нет. Тестовые
файлы — только в §5.

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

Сигнатуры после этапа 3. `GetBinDir` и `GetLogsDir` удалены: `bin/` —
`DataDir.Bin()` / `AppDir.Bin()`, каталог логов — `string(Layout.Logs)`.

| Функция | Файл:строка | Строит | Сигнатура |
|---|---|---|---|
| `GetConfigPath` | platform_common.go:37 | `bin/config.json` | `(DataDir)` |
| `GetRemoteMachineDir` | platform_common.go:51 | `bin/wizard_states/remote/<id>` | `(DataDir, id)` |
| `GetRemoteConfigPathFor` | platform_common.go:69 | `…/remote/<id>/config.json` | `(DataDir, id)` |
| `GetRuleSetsDir` | platform_common.go:74 | `bin/rule-sets` | `(DataDir)` |
| `GetRuleSetPath` | platform_common.go:81 | `bin/rule-sets/<tag>.srs` | `(DataDir, tag)` |
| `TailscaleDirName`, `TempDirName`, `DaemonIdentityDirName`, `RemoteDaemonsDirName` | internal/constants/constants.go:68-75 | константы | перенесены на этапе 3 |
| `GetTailscaleStateDir` | platform_common.go:87 | `bin/tailscale` | `(DataDir)` |
| `GetTempDir` | platform_common.go:93 | `temp` (НЕ под `bin/`) | `(DataDir)` |
| `GetDaemonIdentityDir` | platform_common.go:99 | `bin/daemon` | `(DataDir)` |
| `GetRemoteDaemonIdentityDir` | platform_common.go:105 | `bin/remote-daemons/<id>` | `(DataDir, id)` |
| `GetRuleSetsDirFor` | platform_common.go:119 | local → `bin/rule-sets`, remote → `…/remote/<id>/srs` | `(DataDir, target, id)` |
| `GetSubscriptionsDirFor` | platform_common.go:136 | local → `bin/subscriptions`, remote → `…/<id>/subscriptions` | `(DataDir, target, id)` |
| `GetWizardTemplatePath` | platform_common.go:148 | `<Data>/bin/wizard_template.json` — цель скачивания; читать через `template.ResolveTemplate` (этап 5) | `(DataDir)` |
| `GetShippedTemplatePath` | platform_common.go:154 | `<App>/bin/wizard_template.json` — поставляемый seed; зовёт только `template.ResolveTemplate` | `(AppDir)` |
| `GetWizardStatesDir` | platform_common.go:161 | `bin/wizard_states` | `(DataDir)` |
| `GetWizardStatePath` | platform_common.go:171 | `bin/wizard_states/state.json` | `(DataDir)` |
| `GetWizardStatesDirFor` | platform_common.go:194 | local/remote каталог состояний | `(DataDir, target, id)` |
| `GetWizardStatePathFor` | platform_common.go:204 | `…/state.json` для таргета | `(DataDir, target, id)` |
| `GetOutboundsCachePath` | platform_common.go:225 | `bin/outbounds.cache.json` (legacy, только удаляется) | `(DataDir)` |
| `GetSubscriptionsDir` | platform_common.go:233 | `bin/subscriptions` | `(DataDir)` |
| `EnsureDirectories` | platform_common.go:239 | MkdirAll `Logs`, `Data/bin`, `Data/bin/rule-sets`; AppDir не трогает | `(Layout)` |
| `GLStatePath` | glstate.go:85 | `<Data>/bin/gl-state.json` | `(DataDir)` |
| `LoadGLState` / `SaveGLState` | glstate.go:91 / :107 | чтение/запись gl-state (`SaveGLState` берёт путь из `GLStatePath`) | `(DataDir, …)` |
| `MarkGLStarting` / `UpdateGLState` / `MarkGLRendered` | glstate.go:132 / :142 / :157 | мутация gl-state | `(DataDir, …)` |
| `IsMesaInstalled` / `IsMesaDisabled` / `HasMesaBundle` / `HasForeignOpenGL` | glstate.go:198 / :204 / :210 / :215 | `opengl32.dll`, `libgallium_wgl.dll`, `*.off`, `mesa3d/` рядом с exe | `(AppDir)` |
| `DisableMesa` / `EnableMesa` / `copyMesaFromBundle` / `removeMesaFiles` | glstate.go:227 / :254 / :286 / :330 | переименование/копирование DLL рядом с exe | `(AppDir)` — **исключение §3/§5**, пишет в AppDir; комментарий над группой glstate.go:187 |
| `EnsureDesktopOpenGL` | glprobe_windows.go:320 (стаб glprobe_stub.go:23) | gl-state (Data) + Mesa (App) | `(Layout, interactive)` |
| `restartToApply` | glprobe_windows.go:531 | `UpdateGLState` | `(DataDir, …)` |
| `installAndVerifyMesa` | glprobe_windows.go:599 | копия/скачивание Mesa + gl-state | `(Layout, interactive)` |
| `startBackgroundHardwareProbe` | glprobe_windows.go:666 | проба железа | `(AppDir, renderer)` |
| `probeHardware` | glprobe_windows.go:703 | `opengl32.dll` рядом с exe | `(AppDir)` |
| `downloadMesa` | glprobe_windows.go:795 | распаковывает DLL в AppDir | `(AppDir)` — **исключение** |
| `RunGLProbeChild` / `probeDesktopOpenGL` (`-gl-probe-local`) | glprobe_windows.go:119 / :170 (стаб glprobe_stub.go:16) | `opengl32.dll` из AppDir вместо `os.Executable` | `(AppDir, local)` |
| `GetWintunPathFor` | platform_windows.go:24 (стабы platform_darwin.go:20, platform_linux.go:23 → `""`) | `<coreDir>/wintun.dll` | `(coreDir string)`; зовёт `FileService.ResolveCore` с `Dir(SingboxPath)` (этап 5; `GetWintunPath(DataDir)` удалён) |
| `ResolveSingboxExecPath` | singbox_exec_path.go:45 (один файл без build-тегов; `singbox_exec_path_linux.go` удалён) | путь ядра | `(Layout, env func(string) string) CoreResolution{Path, Source, Shadowed}`; цепочка `SINGBOX_LAUNCHER_CORE` → Data → App → PATH на всех ОС (этап 5) |
| `WritePrivilegedStartScript` | privileged_darwin.go:115 (стаб privileged_stub.go:27) | скрипт/pid в `binDir`, лог в `logPath` | строки: `binDir` = `Data.Bin()`, `logPath` = `FileService.ChildLogPath` (process_service.go) |

### 1.2 Хелперы вне `internal/platform`

Правило этапа 4: чистые Data-функции принимают `paths.DataDir` (параметр
переименован `execDir` → `dataDir`); функции, которым нужен шаблон (сейчас из
Data, с этапа 5 — резолвер App/Data), принимают `paths.Layout` целиком, чтобы
этап 5 менял только тела. Строковый `binDir` оставлен там, где пакет-лист
(`internal/locale`) не должен знать `paths`.

| Функция | Файл:строка | Сигнатура после этапа 4 |
|---|---|---|
| `locale.GetLocaleDir(binDir)` / `LoadExternalLocales(dir)` | internal/locale/locale.go:384 / :255 | строки; main.go:170-173 грузит `App.Bin()/locale`, затем `Data.Bin()/locale` (поздний перекрывает; в portable один каталог — второй вызов пропускается) |
| `locale.DownloadAllRemoteLocales(dir)` | locale.go:367 | строка; вызывается с `Data.Bin()/locale` (main.go, settings_tab.go) |
| `locale.LoadSettings` / `SaveSettings` / `MarkTemplateInstalled` / `MarkLocalesRefreshed` | internal/locale/settings.go | строковый `binDir` = `layout.Data.Bin()` |
| `template.LoadTemplateData` | core/template/loader.go:256 | `(Layout)`; читает `ResolveTemplate(l).Path` (этап 5) |
| `template.ResolveTemplate` / `ReadTemplateMarker` | core/template/resolve.go:45 / :83 | `(Layout) TemplateResolution{Path, Source, ShippedCurrent}` — единственное правило §3.3; `ReadTemplateMarker(binDir)` |
| `template.DownloadTemplate` | core/template/download.go:53 | `(ctx, DataDir, fetch)` |
| `template.EnsureTemplate` | core/template/download.go:137 | `(ctx, Layout, fetch)` |
| `wizardbusiness.TemplateLoader.LoadTemplateData` | ui/configurator/business/template_loader.go:20 | `(Layout)` |
| `core.RefreshTemplateIfStale` | core/template_migration.go:82 | `(ctx, Layout, fetch)`; маркер из App (нет в App — из Data), штамп и скачивание в Data; `keptShippedTemplate` :163; проверка `config_data_root` :89 (этап 5) |
| `config.BuildVarSubstituterFromDisk` | core/config/varsubst.go:127 | `(Layout)`; `loadTemplateVarDefaults(Layout)` :158 читает `template.ResolveTemplate(l).Path` :163, `loadStateSettingsVars(DataDir)` |
| `config.SetTailscaleStateDirRoot` | вызов core/controller.go:300 | `GetTailscaleStateDir(Layout.Data)` |
| `snapshot.Build` | core/snapshot/snapshot.go:61 | `(Layout, launcherVersion, singboxVersion)` |
| `core.DaemonIdentityDir` | core/backend_daemon_darwin.go:123 | `(DataDir)` |
| `services.NewRemoteRegistry` | core/services/lxd_remote_registry.go:88 | `(DataDir)`; поле `RemoteRegistry.dataDir` |
| `services.MigrateLegacyRemoteProfile` | core/services/lxd_remote_migration.go:35 | `(DataDir, *RemoteRegistry)` |
| `services.RuleSRSPath` / `RuleSRSPathFor` / `SRSFileExists*` / `AllSRSDownloaded*` / `DownloadSRSGroup*` / `DeleteOrphanRuleSets*` | core/services/srs_downloader.go:30… | первый аргумент `DataDir` |
| `services.localResourceFiles` / `CollectDeployResources` | core/services/lxd_remote_resources.go:125 / :161 | `(DataDir, …)` |
| `build.convertPresetRuleSetRemoteToLocal` / `CollectSrsCachedPaths` / `convertRuleSetToLocalRequired` / `ResolveRoute*` | core/build/preset_merge.go, route_merge.go, resolve_route.go | `DataDir`; поля `PresetMergeContext.DataDir` (preset_merge.go:164), `RouteConfig.DataDir` (route_merge.go:46) |
| `state` load-router | core/state/load_router.go:203-217 | не тронут: выводит пути из пути state.json |
| `core.directionBuildOptions` | core/config_service.go:561 | `(Layout)` |
| `core.collectAllStageRuleSetTags` | config_service.go:410 | `(DataDir, …)` |
| `core.refreshSubscriptionsMetaAndCache` / `persistFetchResultForSource` | config_service_subscriptions.go | `(…, DataDir)` |
| `core.loadTemplateForBuild` | core/rebuild.go:389 | `(Layout)` |
| `core.cleanupLegacyOutboundsCache` | rebuild.go:459 | `(DataDir)` |
| `core.buildSnapshotFromState` | core/rebuild_snapshot.go:40 | `(state, Layout, subst, td)` |
| `main.rememberOfferedRenderer` | main.go | `(DataDir, renderer)` |
| `wizardbusiness.ListCloneSources` / `stateExistsFor` / `LoadCloneState` | ui/configurator/business/clone_source.go | `(DataDir, …)` |
| `tabs.currentIdentityDefaults` | ui/configurator/tabs/source_identity_block.go | `(DataDir)` |
| `dialogs.fetchOrLoadGetFree` | ui/configurator/dialogs/get_free_dialog.go | `(DataDir)` |

Решения по **M** на этапах 3–4: шаблон читается из Data; маркер шаблона —
из Data; `GetCoreBinaryPath` (core_version.go:57) показывает путь ядра
относительно Data (ядро качается в `Data/bin`; в portable Data = App);
`SingboxCmd.Dir` = `Data.Bin()`; privileged-скрипт macOS — `Data.Bin()`,
его лог — `ChildLogPath`; Mesa-кнопка Diagnostics — `App`, её
`UpdateGLState` — `Data`.

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
`ProgramArguments[0]` по §5.1, этап 10). Этап 10 сделан: `fillServiceCorePath`
(:217) читает plist через `readPlistProgramPath` (:248, `encoding/xml`) и
ставит `ServiceCorePath` / `ServiceCoreMismatch`; предупреждение и команда
установки — `coreMismatchBox` на вкладке Status
(ui/connection_local_daemon_darwin.go:75). `build/build_darwin.sh -i`:
вынос `bin`/`logs` ради печати только при старых данных в бандле
(`LEGACY_DATA`), чистый бандл проверяется `codesign --verify` на месте.

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

### 3.1 Поля `FileService` (core/services/file_service.go:44-74)

После этапа 3: `ExecDir` удалён, вместо него `Layout paths.Layout`
(file_service.go:47); `NewFileService(layout)` (:79) не зовёт
`os.Executable`; `ChildLogRelativePath` заменён абсолютным `ChildLogPath`
(:73, `<Logs>/sing-box.log`), его читают controller.go (`RunHidden`),
process_service.go (ротация, privileged-старт), ui/log_viewer_window.go,
ui/traffic_bootstrap.go, settings_tun_darwin.go. `OpenLogFiles()` (:99) без
аргументов: имена логов — `constants.*LogFileName` под `Layout.Logs`;
константы `"logs/…"` в controller.go удалены.

| Поле | Читатели |
|---|---|
| `Layout` | все бывшие читатели `ExecDir` (§2): `.Data` — состояние/кэши, `.Logs` — логи, `.App` — Mesa |
| `ConfigPath` | без изменений (`GetConfigPath(Layout.Data)`) |
| `SingboxBundledPath` | `Data/bin/sing-box` — цель скачивания (core_downloader.go) |
| `SingboxPath`, `CoreSource`, `ShadowedCorePath` | `ResolveCore()` (file_service.go:111) = `ResolveSingboxExecPath(Layout, os.Getenv)`; зовут `NewFileService` (:99) и `DownloadCore` после установки (core_downloader.go:141) |
| `WintunPath` | `GetWintunPathFor(Dir(SingboxPath))` там же, в `ResolveCore` |
| `ChildLogPath` | см. выше |
| `Migration`, `MigrationErr` | этап 6: `paths.MigrateLegacyData(layout, nil)` в `NewFileService` (file_service.go:109) до `EnsureDirectories`; ошибка старт не прерывает. Читает main.go: строка WARN после `layout.LogLine()` (:190, `MigrationResult.Summary()` migrate.go:78 / «migration failed: …») и `firstRunNoticeDue` (:73) → `scheduleFirstRunNotice` (:88) из `OnStarted` (:480); флаг `first_run_notice_shown` (`locale.MarkFirstRunNoticeShown`, settings.go:250) |

### 3.2 Debug API

- `ControllerFacade.GetExecDir() string` → `GetLayout() paths.Layout`
  (core/debugapi/server.go:59); реализация core/debugapi_wiring.go:58;
  фейк в тестах server_test.go (`fakeFacade.dataDir` → portable-раскладка).
  Читатели: settings_endpoints.go (`GetLayout().Data.Bin()`), snapshot.go
  (`snapshot.Build(GetLayout(), …)`), state_endpoints.go (`GetLayout().Data`).
- `RemoteAPI.ExecDir string` → `RemoteAPI.DataDir paths.DataDir`
  (remote_endpoints.go:40); заполняется debugapi_wiring.go; читатели
  remote_endpoints.go, remote_resources_endpoints.go, remote_state_endpoints.go.

### 3.3 Мастер

- `WizardModel.ExecDir string` → `WizardModel.DataDir paths.DataDir`
  (models/wizard_model.go:225); пишется configurator.go:224
  (`ac.FileService.Layout.Data`). AppDir модели не понадобился: все 20
  читателей — Data.
- `FileServiceInterface.ExecDir() string` → `Layout() paths.Layout`
  (business/interfaces.go:38; адаптер file_service_adapter.go:23);
  потребители state_store.go (`fileService.Layout().Data`).

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

После этапа 5. Чтение — по цепочке Data → App (для шаблона с правилом
маркера), запись — только в Data.

| Что | Чтение | Запись/скачивание | Как решено (этап 5) |
|---|---|---|---|
| Шаблон `wizard_template.json` | **`template.ResolveTemplate(Layout)`** (core/template/resolve.go:45) — одно правило §3.3 для всех точек: `LoadTemplateData` (loader.go:256) ← config_service.go:250, :562; rebuild.go:395 (`loadTemplateForBuild`), :452; debugapi_wiring.go:142; configurator.go:140, :821 (через template_loader.go:28); `EnsureTemplate` (download.go:137); напрямую по пути: varsubst.go:163, snapshot.go:64, core_dashboard_tab_status.go:254 (есть ли шаблон — `Source != ""`), configurator.go:147 (строка лога) | `DownloadTemplate(DataDir)` (download.go:53) ← template_migration.go (`RefreshTemplateIfStale`), core_dashboard_tab.go:941, `EnsureTemplate` ← rebuild.go:402, configurator.go:156 | A = App/bin, D = Data/bin: A с маркером == `AppVersion` и (нет D, штамп пуст или < `AppVersion`) → A; иначе D; иначе A; иначе путь D. `template_migration.go:91` (stat D) остаётся на Data намеренно: это проверка цели скачивания |
| Маркер `wizard_template.version` | `template.ReadTemplateMarker` (resolve.go:83) ← `ResolveTemplate` (App), `keptShippedTemplate` (template_migration.go:163; App, при отсутствии — Data) | CI `win64-full` (.github/workflows/ci.yml:721) | читается из App |
| Штамп `LastTemplateLauncherVersion` | `ResolveTemplate` (правило 1), `RefreshTemplateIfStale` | `MarkTemplateInstalled` (settings.go:222) ← download.go:91, `stampTemplateCheck` | Data. Ветка «поставляемый под эту версию» удаляет устаревший D (иначе после штампа резолвер вернулся бы к нему); не удалился — штамп не ставится |
| Штамп `config_data_root` | `RefreshTemplateIfStale` (template_migration.go:89) → `RebuildConfig` → `MarkConfigStale` (:275) | `stampConfigDataRoot` (template_migration.go:196) → `locale.MarkConfigDataRoot` ← rebuild.go:307 (единственная успешная запись локального `config.json`, после `promoteCandidate`); после успешной миграции — СТАРЫЙ корень, если штампа нет: `stampPreMigrationDataRoot` (core/services/file_service.go:139 ← :112), тест `TestMigrationStampsPreviousDataRoot` (core/template_migration_test.go:306) | §3.5; проверка работает и на dev-сборках; пересборка после миграции переживает перезапуск (SPEC §11 к) |
| Локали `bin/locale/*.json` | main.go (`LoadExternalLocales` App, затем Data) | main.go, settings_tab.go (`DownloadAllRemoteLocales` в Data) | этап 4 |
| Штамп `LastLocaleLauncherVersion` | main.go | `MarkLocalesRefreshed` | Data |
| Ядро | `FileService.SingboxPath` ← `ResolveCore` (file_service.go:111) ← `platform.ResolveSingboxExecPath` (singbox_exec_path.go:45) | `SingboxBundledPath` = Data/bin/sing-box ← core_downloader.go:121; после установки `ResolveCore()` (:141) | `SINGBOX_LAUNCHER_CORE` (`constants.EnvCorePath`) → Data → App → PATH. Лог старта `core: <path> (source=…)` и `core: <src> <v> shadows <kind> <v> (<path>)` — `logCoreResolution` (version_marks.go:114) в горутине отметок версий (template_migration.go:241) |
| wintun.dll | `CheckWintunDLL` (wintun_downloader.go:40) по `WintunPath` | `DownloadWintunDLL` (:52) в `Dir(WintunPath)`; каталог не пишется (`paths.ProbeWritable`, :67) → `ErrCoreDirReadOnly` + текст для диалога (core_dashboard_tab.go:1134) | `Dir(SingboxPath)/wintun.dll` |
| libcronet | загрузчик ОС рядом с ядром; проверка `cronetLibAvailable` (core_capabilities.go:342) от `Dir(SingboxPath)` | core_downloader.go:131 рядом со скачанным ядром (Data/bin) | уже от каталога ядра, правка не понадобилась |
| Mesa3D (`mesa3d/`, `opengl32.dll`) | glstate.go:193-212, glprobe_windows.go:193 | glstate.go:222-330, glprobe_windows.go:798 | остаётся App (исключение) |
| `get_free.json` | get_free_dialog.go:67-80 | `downloadGetFree` в тот же путь | Data (в zip не входит) |

---

## 5. Тесты, которые сломаются при смене сигнатур

Переведены на этапе 4 минимальной правкой (`paths.DataDir(t.TempDir())`,
`paths.Layout{Data: …}`; где тест читает поставляемый шаблон из корня
репозитория — `paths.Layout{App: root, Data: root}`). Новых тестов нет.

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

### Этап 7 — сделано

- **Блок путей**: `paths.PathsInfo` + `Lines()`/`Text()` — internal/paths/paths.go
  (конец файла); заполняют `(*AppController).PathsInfo()` (из FileService,
  версия ядра только из кэша) и `core.PathsInfoFor(layout)` (лёгкий, для
  `-paths`) — core/paths_info.go.
- **Storage**: `buildStorageSection(ac) (obj, refresh)` — ui/settings_storage.go;
  встаёт последним разделом в `BuildSettingsContent` (теперь возвращает ещё
  `refresh`), refresh зовётся в `app.tabs.OnSelected` на вкладке Settings
  (ui/app.go). Место под этапы 8–9 — пустой `extra` VBox под Copy paths.
- **`-paths`**: main.go сразу после `flag.Parse()`, до crash-лога и GL-пробы.
- **`GET /debug/paths`**: реестр server.go (рядом с `/debug/snapshot`),
  `handlePaths` + `pathsView` — core/debugapi/snapshot.go; фасад
  `GetPathsInfo()` — server.go, core/debugapi_wiring.go, fakeFacade в
  server_test.go. Документация — docs/API.md, docs/API.ru.md.

### Этап 8 — сделано

- **Библиотека**: internal/paths/switch.go — `SystemDefault` (правило 4 через
  выделенный `platformDefault` в paths.go), `SwitchToPortable`,
  `SwitchToSystem`, `SwitchReport{From, To, Copy, Leftover}` + `Summary()`,
  `MovedBinPrefix` («bin.moved-») для очистки §4.3, `ErrEnvLayout`. Тест —
  `TestSwitchPortable` (switch_test.go).
- **Обвязка**: core/storage_switch.go — `PortableToggleAvailable()` (не
  darwin), `PortableToggleState()`, `PortableSwitchTarget(on)`,
  `SwitchPortable(on)` (повторная проверка условий, WARN со сводкой).
- **UI**: `buildPortableToggle` / `confirmPortableSwitch` — ui/settings_storage.go,
  в `extra` раздела Storage; перечитывается общим refresh раздела.
- **Перезапуск**: как у Mesa — `platform.RequestRestartAfterExit()` +
  `GracefulExit()`, `RestartSelf` в конце main(); `RestartSelf` вне Windows
  теперь настоящий (internal/platform/restart_other.go, `Setsid`).

### Этап 9 — очистка

- **Библиотека**: internal/paths/purge.go — `BuildPurgePlan(l, exe, env,
  goos, probe) PurgePlan`, `ExecutePurge(PurgePlan) PurgeReport`,
  `PurgePlan.Text()`, `PurgeReport.Text()`, `FormatBytes`. Виды — `PurgeData`,
  `PurgeLogs`, `PurgeLeftover`; пояснения — константы `PurgeNote*`. Системный
  DataDir для остатка при portable — `SystemDefault` (switch.go), остаток
  выключения Portable — `MovedBinPrefix`. Поставляемое в `<App>/bin`
  (шаблон, маркер, locale/, ядро со спутниками) — `shippedBinNames`; при
  Data == App элемент data — `<App>/bin`, и если в нём нет
  `wizard_template.version` (zip только с exe), поставляемыми считаются только
  локали (`alwaysShippedBinNames`): скачанные ядро, спутники и шаблон уходят с
  данными. Тест — `TestPurge` (purge_test.go).
- **Обвязка**: core/purge.go — `(*AppController).PurgePlan()`,
  `NetworkCleanup()` (Windows, `GhostTunCleanupAggressive` + правила
  sing-tun), `DaemonUninstallHint()`, `ExecutePurgeAndExit(plan, network)`;
  `PurgeCLI(layout, exe, yes, out) int` для флага. Подсказка службы демона —
  core/purge_darwin.go (plist → `DaemonUninstallCommand(true)`), заглушка —
  core/purge_other.go.
- **Диалог**: ui/settings_purge.go — `buildPurgeButton(ac)` (в `extra`
  раздела Storage после чекбокса Portable; при запущенном ядре — «Stop the VPN
  first»), `showPurgeDialog(ac, plan, hint)`: `dialog.NewCustomConfirm`,
  список в `VScroll` — пустой Check + Label с `TextWrapBreak`, Data
  отключён; на Windows чекбокс сетевой очистки; команда службы демона —
  общий `CommandRow` (ui/command_row.go). Подтверждение → `Selected` из
  чекбоксов → `ExecutePurgeAndExit` в горутине.
- **`-purge-data [-yes]`**: main.go сразу за `-paths`, до crash-лога.
  С `-yes` отказ с кодом 1, если жив процесс из `<Data>/bin/singbox.pid`
  (только проверка по списку процессов, без сигналов).

### Ревью data-критичного кода — сделано (SPEC §11 з, и)

- **Скрытые данные**: `paths.HiddenSystemData`, `paths.RemovePortableMarker`,
  `paths.ErrTargetHasData` — internal/paths/switch.go; диалог —
  `hiddenDataNoticeDue`/`scheduleHiddenDataNotice` в main.go (общий показ с
  first-run notice — `whenWindowVisible`); флаг `hidden_data_notice_shown` —
  internal/locale/settings.go.
- **Копировщик**: `CopyReport.SkippedPaths` + `SkippedPath(rel)`,
  `ErrStateNotCopied`, разворот корня-симлинка, `wizard_states` последним —
  internal/paths/copytree.go. Lock миграции `Data/.migrating.lock`
  (`MigrationResult.Busy`) — internal/paths/migrate.go.
- **Очистка**: `PurgeItem.Files` (список путей, каталог не трогается; счётчик
  переименован в `FileCount`), Env-режим, `storage_leftover`,
  `PurgeNoteSystemDataHasState` — internal/paths/purge.go;
  `purgeBlockingProcess` (лаунчер по имени, sing-box по имени и пути) —
  core/purge.go; `debuglog.ReleaseLogFiles` — internal/debuglog/close.go.
- **Переезд**: `storageSwitching` на контроллере, отказ в
  `StartSingBoxProcess` (core/controller.go) и в `runScheduledRefresh`/
  `refreshSourceWithRetry` (core/auto_update.go); перезапуск и
  `storage_leftover` — `SwitchPortable` (core/storage_switch.go); строка
  остатка — ui/settings_storage.go.
- **Затенённое ядро из PATH**: `logCoreResolution` — core/version_marks.go.
