# 139 · Windows: asInvoker и права по требованию

Закрывает issue #99 («Windows: Запуск без флага requireAdministrator»).
Решения владельца — 24.09.2026, в тексте закреплены и не пересматриваются.
Опирается на SPEC 135 (в `develop` с `eeae31c8`): её §3.5 «Связь с #99»
требует от этой спеки миграции и ручной проверки (§7). Смежные: SPEC 137 §8
п. 5 (дыра остаётся, §10), SPEC 140 (установщик: требования к 139 — его §8),
SPEC 141 (служба `sing-box-lxd` на Windows: кнопка «Install service»,
`ShellExecuteExW`, гейт копии под правами).

Статус: **реализована в ветке `spec-139-asinvoker`**, ждёт ручной приёмки
§11 на Windows; отступления — `IMPLEMENTATION_REPORT.md`. Строки — на `38550b5c`.

## 1. Дефект и мотивация

**#99.** Proxy-only (issue #86) прав не требует, но `app.manifest:7` —
`requireAdministrator` (вшивает `rsrc`: `build/build_windows.bat:153,160`,
для win7-32 `.github/workflows/ci.yml:555`), и каждый старт идёт через UAC.
Автозапуск из `HKCU\…\Run` невозможен: такие записи Windows при входе не
запускает. Тело релиза (`ci.yml:874`) и `README.md:275` советуют распаковать
zip в `C:\Program Files\singbox-launcher` — это работает, только пока лаунчер
всегда администратор.

**Безопасность.** С высокой целостностью работает весь GUI: разбор подписок,
шаблона и локалей из DataDir, куда пишет любой процесс пользователя;
PowerShell-диалоги (SPEC 137 §12); браузер из `OpenURL`; ядро из
`%LOCALAPPDATA%` (SPEC 137 §8 п. 5). После задачи proxy-only не повышается
никогда, TUN — только по явному действию.

**Код рассчитывает на админа.** Проверки прав нет (grep `IsElevated`,
`TokenElevation`, `IsUserAnAdmin` — пусто); места — §6.

## 2. Нормы

1. Манифест `asInvoker` в обеих сборках. Манифест остаётся вшитым:
   32-битный процесс без `requestedExecutionLevel` попадает под
   UAC-виртуализацию, и запись в Program Files уходит в `VirtualStore` — проба
   записи SPEC 135 соврёт (§9).
2. `platform.IsElevated()` — один раз на процесс. Windows: `TokenElevation`
   (`windows.Token.IsElevated`) **или** членство токена в `BUILTIN\Administrators`
   (`CreateWellKnownSid` + `Token.IsMember`) — второе покрывает машины с
   выключенным UAC. Не-Windows: `os.Geteuid() == 0` (в гейтах не участвует).
3. TUN без прав не запускается: вместо ядра — диалог §4.
4. Всё, что требует админа, идёт через явный гейт `IsElevated` (§6). Без прав —
   пропуск и одна строка INFO с перечнем пропущенного, не WARN на каждое место.
5. Где лежат данные, не зависит от прав (§7): повышенный и обычный экземпляры
   видят один DataDir.
6. Дефолт `tun=true` для Windows в шаблоне не меняется: первый старт без прав
   с TUN ведёт в диалог §4.
7. Classic + TUN под правами запускает ядро из `<Data>\bin`, как сейчас (§10).

## 3. Матрица

| Конфиг / операция | Без прав | С правами |
|---|---|---|
| proxy-only (`tun=false`) | старт как сейчас; `set_system_proxy` пишет прокси пользователя (HKCU), права не нужны | как сейчас |
| TUN | ядро не стартует, диалог §4 | как сейчас |
| Очистка Wintun / NLA / правил брандмауэра (старт, Stop, очистка данных) | пропуск (§6) | как сейчас |
| Kill ядра, запущенного повышенным экземпляром | сообщение и «Restart as administrator» | как сейчас |
| Core → Download, wintun, шаблон, локали | пишутся в `<Data>\bin`, работает | как сейчас |
| «Start with Windows» (§8) | работает | чекбокс недоступен |
| Portable (135 §4.2) | по предикату §7 | недоступен |
| Remove all data / `-purge-data -yes` | данные удаляются; сеть и остатки в защищённом AppDir — пропуск с подсказкой | всё |

Start по-прежнему требует `wintun.dll` и в proxy-only (`core/controller.go:930, 959`) —
не меняем. Заголовок окна повышенного экземпляра — `Singbox Launcher
(Administrator)` (`main.go:616`). Первая WARN-строка старта (`main.go:279`)
получает `elevated=yes|no`: релизный лог пишет только WARN и выше
(`internal/debuglog/debuglog.go:57`), INFO из нормы 4 видна в dev-сборках.

## 4. Диалог «TUN без прав»

**Где.** `ProcessService.Start` (`core/process_service.go:165`) после пересборки
(`:208`), до ветки darwin и `exec` (`:241-260`): Windows, `!IsElevated()`,
`config.ConfigHasTun(ConfigPath)` (`core/config/config_loader.go:98`, сейчас
зовётся только под darwin; ошибка чтения — как на darwin: WARN, «TUN нет»).
Через `Start` идут все входы: кнопка (`ui/core_dashboard_tab.go:247,341`), трей
(`core/tray_menu.go:59`), `-start` (`main.go:596`), Debug API `/action/start`
(`core/debugapi_wiring.go:83`), авто-рестарт. Окно скрыто (трей, `-tray`) —
сначала `ShowMainWindowOrFocusWizard`, затем диалог. Без UI — WARN и выход.

**Кнопки** (порядок): [SPEC 141: **Install service** — первой; до 141 кнопки
нет, на Win7 нет никогда] · **Restart as administrator** (§5) · **Switch to
proxy mode** · **Cancel**.

**Switch to proxy mode** — путь `ApplyLogLevelAndReloadCore`
(`core/log_level.go:58`): в `state.Vars` локального профиля `tun=false`,
`enable_proxy_in=true`, `proxy_in_set_system_proxy=true` (шаблон: `tun`
`bin/wizard_template.json:57-69`, `enable_proxy_in` `:251-261`,
`proxy_in_set_system_proxy` `:295-318`, inbound `proxy-in` `:1980-2010`) →
`Save` → `RebuildConfigIfDirty(true)` → `Start`. Ошибка сборки —
`ShowRebuildError`. Открыт конфигуратор (`UIService.WizardWindow != nil`) —
кнопка недоступна с подсказкой: его Save перезаписал бы state.

**Тексты** — `locale.T`, перевод сразу в `bin/locale/ru.json`:

| Ключ (EN) | RU |
|---|---|
| `TUN needs administrator rights` | Для TUN нужны права администратора |
| `TUN mode creates a network adapter and changes routes, which Windows allows only to administrators. The launcher is running without administrator rights.` | Режим TUN создаёт сетевой адаптер и меняет маршруты — Windows разрешает это только администраторам. Лаунчер запущен без прав администратора. |
| `Proxy mode needs no rights: programs that use the system proxy (browsers and most apps) go through the local proxy on port %s, the rest connect directly.` | Режиму прокси права не нужны: программы, которые используют системный прокси (браузеры и большинство приложений), пойдут через локальный прокси на порту %s, остальные — напрямую. |
| `Windows will ask for an administrator account, and the launcher will run under that account.` | Windows спросит учётную запись администратора, и лаунчер будет работать под ней. |
| `Restart as administrator` | Перезапустить от имени администратора |
| `Switch to proxy mode` | Перейти в режим прокси |
| `Close the configurator first` | Сначала закройте конфигуратор |
| `The administrator prompt was cancelled.` | Запрос прав администратора отменён. |
| `Could not restart as administrator: %s` | Не удалось перезапустить от имени администратора: %s |
| `sing-box was started with administrator rights, and this launcher cannot stop it without them. Restart the launcher as administrator to stop it.` | sing-box запущен с правами администратора, и лаунчер без прав не может его остановить. Перезапустите лаунчер от имени администратора. |
| `Administrator` | Администратор |
| `Start with Windows` | Запускать вместе с Windows |
| `Connect VPN at sign-in` | Подключать VPN при входе |
| `Windows starts another copy: %s` | Windows запускает другую копию: %s |
| `Change this in a normal start, not as administrator` | Меняется при обычном запуске, не от имени администратора |
| `Requires administrator rights` | Нужны права администратора |
| `Autostart entry (Start with Windows)` | Запись автозапуска (Запускать вместе с Windows) |

Порт в тексте — `proxy_in_listen_port` так, как его видит сборщик (state или
дефолт шаблона 7890). Фраза про учётную запись — только когда повышение
спросит **другую** учётную запись: `TokenElevationType == Default` при
`!IsElevated()` (обычный пользователь). Строка про отмену UAC — та же, что в
SPEC 141 §5.3.

## 5. Перезапуск с повышением

**В коде сейчас.** Single-instance защиты нет: `CheckIfLauncherAlreadyRunningUtil`
(`core/controller.go:870`, вызов `main.go:633`) по имени процесса показывает
«already running» и **не выходит**; мьютекс и событие Quit вводит SPEC 140 §4.
Миграция держит `.migrating.lock` (`internal/paths/migrate.go:113`). Порт Debug
API занят со старта (`main.go:394`) до выхода процесса — `GracefulExit` сервер
не останавливает. Трей снимает `UIService.QuitApplication` (`systray.Quit`).
Бюджет `GracefulExit` — 15 + 3 с (`core/controller.go:448-449`). `RestartSelf`
(Mesa, Portable; `internal/platform/restart_windows.go:32`) наследует токен.

**Порядок: сначала новый, потом выход старого.** Отказ в UAC не оставляет
пользователя без лаунчера; новый ничего не трогает, пока жив старый.

1. Кнопка → горутина с `runtime.LockOSThread` и
   `CoInitializeEx(APARTMENTTHREADED|DISABLE_OLE1DDE)`; UI не блокируется.
2. Аргументы собираются из разобранных флагов (`flag.Visit`), а не из сырой
   строки: всё заданное, кроме `-tray` (действие идёт из открытого окна) и
   прежнего `-handoff`; плюс `-start` (пользователь нажимал Start) и
   `-handoff=<PID>|<Mode>|<DataDir>|<LogDir>` (`|` в путях Windows недопустим).
   Сборка строки — `windows.ComposeCommandLine`.
3. `platform.RunElevated(exe, args, AppDir, SW_SHOWNORMAL)` — `ShellExecuteExW`
   из `shell32.dll` (в `x/sys/windows` v0.25.0 и v0.47.0 есть только
   `ShellExecute`), `lpVerb="runas"`, `SEE_MASK_NOCLOSEPROCESS|SEE_MASK_NOASYNC`,
   `hwnd = GetForegroundWindow()` — окно UAC не прячется за лаунчером. Тот же
   примитив берёт SPEC 141 §5.2 (с `SW_HIDE` и ожиданием).
4. `ERROR_CANCELLED` (1223) — INFO, диалог остаётся открытым со строкой отмены.
   Иная ошибка — WARN и текст в диалоге.
5. Успех — WARN `restart as administrator: started pid N, exiting`, дескриптор
   закрывается, `GracefulExit` — как Quit в трее. `RequestRestartAfterExit`
   не взводится.
6. Новый экземпляр: `flag.Parse` переезжает до `paths.Resolve`. С `-handoff`
   раскладка берётся из флага (App — каталог своего exe; Mode — родителя;
   невалидное значение — stderr и обычный `Resolve`), затем **до** crash-лога,
   GL-пробы, контроллера и мьютекса SPEC 140 — ожидание родителя:
   `OpenProcess(SYNCHRONIZE|PROCESS_QUERY_LIMITED_INFORMATION)`; путь образа
   (`QueryFullProcessImageName`) не равен своему exe — не ждать (PID занят
   другим процессом); иначе `WaitForSingleObject` до 25 с. `OpenProcess`
   отказал (родитель другой учётной записи) — опрос списка процессов по PID
   раз в 250 мс, те же 25 с. Таймаут — старт продолжается, WARN после
   открытия логов.
7. Дальше обычный старт; `-start` → `Start` через `autoStartDelay` → TUN.

Ожидание закрывает занятый порт Debug API, открытые логи (`.old` на Windows не
переименовать), две иконки трея, ложное «already running», гонку с миграцией.
`RestartSelf` повышенного передаёт `-handoff` дальше: раскладка та же,
устаревший PID отсеивает проверка образа.

## 6. Гейты

| # | Место | Что требует прав | Без прав |
|---|---|---|---|
| 1 | `core/process_service.go:165` `Start` | TUN ядра (адаптер, маршруты, правило `sing-tun (<путь>)` ставит само ядро) | диалог §4 |
| 2 | `core/controller.go:821-828` → `ProcessService.CleanupStaleTunAtStart` (`core/process_service.go:87`) → `platform.CleanupGhostSingboxTunAdapters`: NLA в HKLM (`internal/platform/wintun_cleanup_windows_nla_profiles.go:144,201`, все Windows), `DIF_REMOVE` (`wintun_cleanup_windows_device.go:147`, Win7) | запись HKLM, SetupAPI | пропуск |
| 3 | `core/controller.go:835-852` → `platform.CleanupOrphanSingTunFirewallRules` (`netsh … delete rule`, `internal/platform/singtun_fwrules_windows.go:60`) | брандмауэр | пропуск |
| 4 | `core/process_service.go:52` `runGhostTunCleanup` (вызовы `:412`, `:540`, `:612` — Stop, рестарт) | как п. 2 | пропуск молча (Debug): без прав ядро TUN не поднимало |
| 5 | `core/purge.go:50` `networkCleanup` — диалог Remove all data (`ui/settings_purge.go:96-103`) и `-purge-data -yes` | п. 2 + п. 3 | UI: пункт снят и недоступен, подсказка `Requires administrator rights`; CLI: `Network cleanup: skipped (needs administrator). To finish, run from an administrator command prompt: "<exe>" -purge-data -yes` |
| 6 | `internal/paths/purge.go:186-226` — остатки под AppDir (`.migrated_from`-источник, `AppDir\logs`, `bin.moved-*`), когда процесс не проходит пробу записи AppDir (обычная проба, не предикат §7: повышенный удалить может) | запись в Program Files | пункт снят с той же подсказкой; пропуск, а не неудачная попытка: код выхода 0 |
| 7 | `core/process_service.go:710-738` Kill в «already running»; `ui/diagnostics_tab.go:337` → `killSingBoxPanic` (`:49-59`) | завершить повышенный процесс | `taskkill` не смог и процесс жив → сообщение §4 с «Restart as administrator»; `RunningState` не сбрасывается вслепую |
| 8 | `core/storage_switch.go:46` Portable | запись AppDir | предикат §7 вместо голой пробы; повышенный экземпляр — недоступен |
| 9 | Mesa (`ui/diagnostics_tab.go:483`), wintun в AppDir (`core/wintun_downloader.go:67`) | запись рядом с exe | уже гейт пробой SPEC 135; GL-гейт старта — правка SPEC 140 §8 |
| 10 | Тексты, исходившие из «всегда админ»: `internal/platform/file_dialog_ps.go:12`, `file_dialog_windows.go:14`, `singtun_fwrules_windows.go:18-19`, `wintun_cleanup_windows_device.go:155-159`, `ui/diagnostics_tab.go:46-48`, `.github/workflows/ci.yml:548-553` | — | переписать: «права по требованию»; флаг `-manifest` в CI остаётся |

Строка INFO старта без прав (п. 2–3), одна:
`elevation: not elevated, skipped: NLA profile cleanup, orphan firewall rule cleanup[, ghost adapter cleanup (Win7)]; they run on the next start as administrator`.

## 7. Раскладка данных и миграция 135

**Проблема.** Правило 2 (`portable.txt`, `internal/paths/paths.go:175`)
пробу записи не делает, правило 3 (Legacy, `:178-180`) делает её от лица
процесса. В Program Files обычный экземпляр пробу не проходит, повышенный —
проходит: без правки они выберут разные DataDir, и повышенный будет писать в
устаревшую копию.

**Правило.** «AppDir пишется пользователем» = проба записи **и** (не Windows
**или** AppDir не лежит под `%ProgramFiles%`, `%ProgramFiles(x86)%`,
`%ProgramW6432%`, `%SystemRoot%`; переменные — из `env` резолвера, сравнение
без регистра по границе каталога). Этим предикатом пользуется всё, что
выбирает раскладку: правило 2 (новое), правило 3 и Windows-фоллбэк без
`LOCALAPPDATA` в `Resolve` и `SystemDefault` (план очистки, переключатель), а
также блокировка переключателя Portable. Маркер при непишущемся AppDir игнорируется:
WARN при старте, суффикс `portable.txt ignored` в `Layout.LogLine` и строке
Mode (Storage, `-paths`, `/debug/paths`). Раскладка из `-handoff` (§5) идёт
мимо `Resolve`. Правило 2 общее для всех ОС: `portable.txt` в read-only
каталоге на Linux сейчас роняет старт так же, как #85.

| Установка | До 139 (всегда админ) | Обычный старт после 139 | Повышенный после 139 |
|---|---|---|---|
| zip (`portable.txt`) в пишущейся папке (`D:\Tools`, профиль) | Portable | Portable | Portable |
| zip (`portable.txt`) в Program Files — совет README и тела релиза | Portable | System + миграция 135 §3.4 из `…\bin` | System, те же данные |
| распаковка без маркера (до 2.1.0) в Program Files с `bin\wizard_states\state.json` | Legacy | System + миграция (135 §3.4, второй случай) | System |
| то же в пишущейся папке | Legacy | Legacy | Legacy |
| установщик SPEC 140 (без маркера, данных рядом нет) | — | System | System |
| `SINGBOX_LAUNCHER_DATA_DIR` | Env | Env | Env через `-handoff` (окружение сессии под `runas` не рассчитываем, §12) |

Миграция копирует `bin\` целиком, с ядром, `wintun.dll`, `libcronet.dll`:
дальше ядро берётся из `<Data>\bin` (135 §3.3), Core → Download и wintun
пишутся туда же без прав. Источник в Program Files не трогается (откат на
прежнюю версию работает); удалить его может только админ — гейт §6 п. 6,
удаление установщиком — SPEC 140 §6. `config_data_root` форсирует пересборку
`config.json` (135 §3.5). Файлы, созданные повышенным экземпляром в
`%LOCALAPPDATA%`, наследуют DACL профиля — обычный экземпляр пишет их дальше
(проверка §11 п. 6).

## 8. Автозапуск

- Settings → раздел Connection (`ui/settings_tab.go:77`), только Windows: чекбокс
  **Start with Windows** и вложенный **Connect VPN at sign-in** (доступен при
  отмеченном первом).
- Значение `singbox-launcher` в `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
  `REG_SZ` `"<exe>" -tray`, со вторым чекбоксом — `"<exe>" -tray -start`.
  `-start` отдельно, потому что с TUN без прав он на каждом входе откроет окно
  с диалогом §4; в proxy-only это и есть «VPN при входе». `exe` —
  `paths.Executable()`.
- Состояние — при открытии Settings: отмечен, если значение есть и путь в
  кавычках совпадает с этим exe без учёта регистра; другой exe — не отмечен,
  подсказка `Windows starts another copy: %s`, установка перезаписывает.
  `StartupApproved` (отключение в диспетчере задач) не трогаем.
- Повышенный экземпляр: оба недоступны с подсказкой — запись
  пользовательская, а повышение могло идти под другой учётной записью.
- CLI для установщика (SPEC 140 §3.3, §6): `-autostart=on|off` до GUI, как
  `-paths`; печать итога, код 0/1; `on` пишет `"<exe>" -tray`, `off` удаляет
  значение, только если оно указывает на этот exe. По тому же правилу его
  удаляют Remove all data (строка `Autostart entry (Start with Windows)`) и
  `-purge-data -yes` (строка плана CLI, без перевода, как весь вывод CLI).
- Лог: INFO старт/успех записи, WARN ошибка.

## 9. Windows 7

`asInvoker` и в `win7-32` (флаг `-manifest` в `ci.yml:555` остаётся). В
диалоге — Restart as administrator и Switch to proxy mode. Go 1.20: без
`min`/`max`, `slices`, `maps`. `x/sys` v0.25.0 (`go.win7.mod:57`) содержит всё
нужное — `Token.IsElevated`, `GetTokenInformation`, `CreateWellKnownSid`,
`Token.IsMember`, `OpenProcess`, `WaitForSingleObject`,
`QueryFullProcessImageName`, `CoInitializeEx`, `GetForegroundWindow`,
`ComposeCommandLine`, `registry` (проверено по module cache); `ShellExecuteExW`
— через `NewLazySystemDLL("shell32.dll")`. `DIF_REMOVE` (только Win7) — только
в повышенном экземпляре.

## 10. Границы

- **SPEC 140** — установщик, мьютекс и событие Quit, GL-гейт Mesa в
  непишущемся AppDir, удаление. Берёт из 139 `-autostart`, `-purge-data` и
  раскладку через `-handoff`.
- **SPEC 141** — служба и кнопка **Install service**, гейт «под правами
  classic исполняет только защищённую копию» (`lxd --service=copy`). **Дыра
  SPEC 137 §8 п. 5 здесь не закрывается:** classic + TUN под правами
  исполняет `<Data>\bin\sing-box.exe`, как сейчас. Закрыта для proxy-only:
  они не повышаются.
- **Принятый риск, окно до выхода 141.** Сценарий §11 п. 9: обычный
  пользователь, пароль вводит администратор — повышенный лаунчер и ядро с TUN
  работают под учётной записью администратора, а ядро исполняется из
  `<Data>\bin\sing-box.exe`, то есть из каталога, в который пишет обычный
  пользователь. Это дыра SPEC 137 §8 п. 5 в худшей форме (повышение не только
  целостности, но и учётной записи). Закрывается SPEC 141 §8: classic под
  правами исполняет только копию из Program Files. В 139 код не меняется.
- Не делаем: автоповышение при старте; смену дефолта `tun`; требование
  `wintun.dll` для Start в proxy-only; снятие системного прокси после падения
  ядра (SPEC 140 §2); вывод `-paths`/`-purge-data`/`-autostart` в консоль
  GUI-сборки (`-H windowsgui`) — виден при перенаправлении; поведение Linux и
  macOS, кроме общего правила 2 (§7).

## 11. Приёмка (Windows 10/11, сборка из CI `run_mode=build`)

1. **Манифест.** Ресурс exe win64 и win7-32 — `level="asInvoker"`. Двойной
   клик — без UAC; в Диспетчере задач «С повышенными правами» — «Нет»; в
   логе `elevated=no`.
2. **Без прав, proxy-only:** Start → VPN работает, «Параметры → Прокси» —
   `127.0.0.1:7890`; Stop → прокси снят. В логе нет WARN от netsh, NLA,
   DIF_REMOVE.
3. **Без прав, TUN** (дефолт): Start → диалог Restart / Switch / Cancel, без
   Install service. Cancel → ничего не запущено. Restart → UAC «Нет» → диалог
   открыт со строкой отмены. Restart → UAC «Да» → старое окно закрылось, новое
   открыто (и при исходном `-tray`), заголовок `(Administrator)`, TUN поднят;
   один `singbox-launcher.exe`, `sing-box.exe` повышен, одна иконка трея, Debug
   API на том же порту, в Storage тот же Data. Switch → VPN работает, в
   конфигураторе TUN снят, proxy-in и system proxy отмечены, в `config.json`
   `mixed` с `set_system_proxy: true` и без `tun`. При открытом конфигураторе
   Switch недоступен.
4. **Повышенный:** Stop → очистка адаптеров и правил в логе, как до 139.
5. **Сирота:** снять повышенный лаунчер в Диспетчере задач (ядро живо) →
   обычный старт → «already running» → Kill → сообщение про права → Restart →
   повышенный снимает ядро.
6. **Program Files.** Распаковка 2.0.x без маркера с данными, exe заменён на
   139 → в логе `migration: … -> …`, подписки на месте, Storage — System,
   `.migrated_from` есть; TUN → Restart → тот же Data. zip 2.1.0+ с маркером
   туда же — то же, в Mode `portable.txt ignored`. ПКМ → «Запуск от имени
   администратора» на обоих — System. После повышенного обычный экземпляр
   сохраняет state без «Access denied». Remove all data без прав — данные
   удалены, сеть и остаток помечены `Requires administrator rights`;
   `-purge-data -yes > purge.txt` в обычной консоли — строка skipped с
   командой, та же команда в консоли администратора доделывает.
7. **Пишущаяся папка** (`D:\singbox`, zip с маркером): обычный и повышенный —
   Portable.
8. **Автозапуск:** Start with Windows →
   `reg query HKCU\Software\Microsoft\Windows\CurrentVersion\Run /v singbox-launcher`
   = `"<exe>" -tray`; выход и вход → лаунчер в трее без UAC. Connect VPN at
   sign-in → `-tray -start`: proxy-only — VPN при входе, TUN — окно с диалогом.
   Снять → значения нет; папка перенесена → «another copy»; в повышенном —
   недоступны. `-autostart=on|off` → код 0; `-purge-data -yes` значение удаляет.
9. **Обычный пользователь:** в диалоге фраза про учётную запись; Restart →
   пароль администратора → лаунчер под ним с тем же Data.
10. **Win7 SP1** (win7-32 на x86 и x64): п. 1–3 и 8; нет
    `%LOCALAPPDATA%\VirtualStore\Program Files*\singbox-launcher`.

## 12. Проверить при реализации

Вне кода не подтверждено: `TokenElevation` при выключенном UAC (поэтому
норма 2 берёт и членство в Administrators); что окружение родителя не
доходит до процесса под `runas` (поэтому раскладка — флагом, а не
`SINGBOX_LAUNCHER_*`, как предлагает SPEC 140 §8); `OpenProcess` родителя из
повышенного процесса другой учётной записи; наследование DACL файлов,
созданных повышенным процессом в профиле. Каждое — пункт §11.
