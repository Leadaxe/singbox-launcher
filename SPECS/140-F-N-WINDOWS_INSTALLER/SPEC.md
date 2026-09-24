# SPEC 140 · Windows: установщик (Inno Setup)

Статус: **N**, спека. Решения владельца от 24.09.2026 закреплены в тексте. Опирается на
SPEC 135 (раскладка данных) и SPEC 139 (манифест `asInvoker`, автозапуск, issue #99): без 139
не выпускается (§8). Под службу SPEC 141 оставлен зарезервированный шаг.

## 1. Мотивация

Установщика нет ни в каком виде: Inno, NSIS, MSI/WiX, msix, winget, scoop, choco в репозитории
не встречаются. Заделов два: хук `-purge-data -yes` в `main.go:229-233` («uninstall-хук
установщика #99») и строка в `docs/release_notes/2-1-0.md:16`.

Сейчас пользователь Windows получает zip и сам решает, куда его положить. README и тело релиза
(`ci.yml:874`) советуют `C:\Program Files\singbox-launcher`. С `asInvoker` (139) писать в этот
каталог нельзя, а zip несёт `portable.txt`, который включает portable без пробы записи (§2).
Нужна поставка в Program Files с данными в `%LOCALAPPDATA%` (135 §3.1): ярлык в «Пуске»,
строка в «Приложениях», обновление поверх и удаление с очисткой одной программой.

## 2. Что проверено в коде

| Факт | Где | Следствие |
|---|---|---|
| Мьютекса и канала «закройся» нет. «Уже запущен» — скан имён процессов и окно | `core/controller.go:870`, `main.go:633` | `AppMutex` не на что повесить, извне лаунчер не закрыть |
| Крестик прячет окно в трей | `main.go:637` | `taskkill` без `/F` (`WM_CLOSE`) лаунчер не завершает |
| Ядро classic — дочерний процесс без Job Object (только `HideWindow`) | `internal/platform/platform_windows.go:88` | `taskkill /F` без `/T` оставляет `sing-box.exe` сиротой; с `/T` ядро снимается аварийно |
| Шаблон умеет `set_system_proxy` | `bin/wizard_template.json:1991` | аварийное снятие ядра оставляет системный прокси на мёртвом порту |
| `GracefulExit` останавливает ядро, сторож разборки 15 с | `core/controller.go:448` | ждать закрытия ≥ 20 с |
| `portable.txt` включает Portable без пробы записи | `internal/paths/paths.go:175` | маркер в Program Files при `asInvoker` = данные в каталоге без права записи |
| Legacy требует пробы записи AppDir | `internal/paths/paths.go:178-180` | повышенный деинсталлятор видит Legacy там, где лаунчер видит System (§6) |
| `-purge-data -yes` отказывает при живом лаунчере или ядре, коды 0/1 | `core/purge.go:116,132-139` | закрывать до очистки, код проверять |
| Mesa включается копированием DLL рядом с exe, GL-гейт запись не проверяет | `internal/platform/glstate.go:254`, `glprobe_windows.go:320` | в Program Files гейт Mesa не поставит |
| Манифест `requireAdministrator` | `app.manifest` | меняет 139 |
| Полный набор собирается только в `release` (ubuntu), только для тега и prerelease | `ci.yml:589,698-737` | в режиме `build` набора нет, установщику его собирать самому |

Inno (jrsoftware.org/ishelp): `ExecAsOriginalUser` при удалении не поддерживается,
`runasoriginaluser` допустим только в `[Run]`; «файлы и настройки пользователя ведёт само
приложение, а не установщик в admin-режиме». `windows-latest` = Windows Server 2025 с
InnoSetup 6.7.1 (runner-images, 24.09.2026).

## 3. Решение

### 3.1 Поставка

Inno Setup 6, скрипт `build/installer/singbox-launcher.iss`. Установка на машину:
`{autopf}\singbox-launcher`, `PrivilegesRequired=admin`, один UAC. Содержимое — `win64-full`
**без `portable.txt`**: `singbox-launcher.exe`; `bin\` с `sing-box.exe`, `libcronet.dll` (если
есть в архиве ядра), `wintun.dll`, `wizard_template.json` и `wizard_template.version`
(= AppVersion); `mesa3d\*.dll`. Только amd64, `ArchitecturesAllowed=x64os`: Inno велит брать
`os`-вариант для драйверов, а wintun под эмуляцией на ARM64 не работает. Минимум — Windows 10,
как у Go 1.25. Win7 без установщика, все три zip остаются как есть.

### 3.2 Ключевые директивы

```iss
; AppVersion, AppVersionNumeric, StageDir — из ISCC /D (§7). Комментарии в Inno — только
; целой строкой: «;» после значения стал бы частью значения или параметром.
[Setup]
; AppId не меняется никогда
AppId={{9D2B9E7B-73EA-49AC-A77F-43DE503552B0}
AppName=singbox-launcher
AppVersion={#AppVersion}
VersionInfoVersion={#AppVersionNumeric}
DefaultDirName={autopf}\singbox-launcher
PrivilegesRequired=admin
ArchitecturesAllowed=x64os
ArchitecturesInstallIn64BitMode=x64os
MinVersion=10.0
CloseApplications=force
RestartApplications=no
SetupLogging=yes
UninstallLogging=yes
OutputBaseFilename=singbox-launcher-{#AppVersion}-win64-setup
; SignTool=... — §9, выключено

[InstallDelete]
; §5 (б); DLL Mesa (список mesaDLLs, glstate.go:194) — три строки, здесь одна
Type: files; Name: "{app}\portable.txt"
Type: files; Name: "{app}\opengl32.dll"; Tasks: not mesa

[Files]
Source: "{#StageDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "{#StageDir}\mesa3d\opengl32.dll"; DestDir: "{app}"; Tasks: mesa; Flags: ignoreversion

[UninstallDelete]
; следы Disable Mesa из повышенного запуска
Type: files; Name: "{app}\*.dll.off"
```

Языки — `english` и `russian` (оба `.isl` идут с Inno), свои строки — `[CustomMessages]` на
двух языках, как английский ключ + `ru.json` в лаунчере. Лицензионной страницы нет: GPL не
требует принятия.

### 3.3 Задачи, ярлыки, запуск

| Задача | Умолч. | Что делает | Видна |
|---|---|---|---|
| `desktopicon` | выкл. | ярлык на общем рабочем столе | всегда |
| `autostart` «Launch at startup» | выкл. | `[Run]` `singbox-launcher.exe -autostart=on`, `runasoriginaluser` | только при первой установке (`Check: not IsUpgrade`) |
| `mesa` «Software OpenGL (Mesa3D) for RDP / VM without GPU» | выкл. | три DLL из `mesa3d\` рядом с exe | всегда |
| `daemonservice` «Install sing-box-lxd service» | вкл. | `[Run]` `{app}\bin\sing-box.exe lxd --service=install`, повышенно, `runhidden` | только в сборке с `/DDaemonService` (после 141) |

Ярлык в «Пуске» ставится всегда. «Launch after install» — стандартный флажок последней
страницы: `[Run]` с `postinstall nowait skipifsilent runasoriginaluser`. Лаунчер стартует
**неповышенным**, иначе увидит не ту раскладку (§8).

**Автозапуск пишет сам лаунчер** (флаг из 139, то же значение `HKCU\...\CurrentVersion\Run`,
что у чекбокса «Start with Windows»): при повышении под чужой учётной записью `HKCU`
установщика — куст администратора. При обновлении задача скрыта: состоянием владеет Settings.

## 4. Закрытие лаунчера

Restart Manager (`CloseApplications`) шлёт окну `WM_CLOSE`/`WM_ENDSESSION`: окно уходит в трей,
с `force` процесс убивается. `taskkill /F` убивает сразу. Ядро при этом остаётся сиротой или
(с `/T`) снимается аварийно (§2), а `AppMutex` лишь просит закрыть руками. Поэтому лаунчер
закрывается сам.

**Лаунчер (правка в 140).** На старте создаются мьютексы `Local\SingboxLauncher.Instance` и
`Global\SingboxLauncher.Instance` (если Global не создался, это не ошибка) и событие
`Local\SingboxLauncher.Quit`. Горутина ждёт событие и вызывает `GracefulExit` на UI-потоке
без перезапуска.

**Установщик.** Процедура `CloseLauncher` в `[Code]` вызывается из `PrepareToInstall` и
`InitializeUninstall`:

1. `CheckForMutexes` мьютекса не видит — готово.
2. `OpenEventW` + `SetEvent` (`kernel32` через `external`), опрос мьютекса до 20 с.
3. Мьютекс остался: экземпляр в чужом сеансе (туда `Local\`-событие не доходит) или завис.
   Сообщение Retry / Cancel / Ignore. Ignore — `taskkill /F /T /IM singbox-launcher.exe` с
   предупреждением про ядро и прокси. В тихом режиме — сразу Ignore.

`CloseApplications=force` — страховка для того, что держит файлы `{app}`: копия до 140 без
мьютекса в той же папке, осиротевший `sing-box.exe` из `{app}\bin`. Служба `sing-box-lxd`
работает из своей копии вне `{app}` (141), процедура её не трогает, VPN в daemon-режиме при
обновлении не рвётся. **Инвариант для 141:** служба не запускает ничего из `{app}`.

## 5. Обновление поверх

Тот же `AppId` и каталог (`UsePreviousAppDir`), задачи запоминаются. Порядок: `CloseLauncher` →
`[InstallDelete]` → файлы → запуск, если отмечен. `%LOCALAPPDATA%` установщик не трогает. Новый
поставляемый шаблон побеждает скачанный по маркеру (135 §3.3), ядро — нет: скачанное в DataDir
затеняет поставляемое всегда (135, решение Г), и тогда вкладка Local предложит Download при
новом ядре в `{app}\bin`. Осознанное следствие 135, здесь не меняется. Ловушки:

- **(а) Поверх распаковки без маркера** (zip до 2.1.0) в тот же `C:\Program Files\singbox-launcher`
  с данными в `bin\`. Данные остаются; неповышенный лаунчер не проходит пробу записи → System →
  миграция 135 §3.4 (второй случай) в `%LOCALAPPDATA%`, `.migrated_from`, пересборка
  `config.json`. Старую копию в `{app}\bin` лаунчер удалить не может — её уберёт удаление (§6).
- **(б) Поверх распаковки с `portable.txt`** (zip 2.1.0+): маркер включил бы Portable в каталоге
  без права записи, `[InstallDelete]` его удаляет, дальше как (а). В пишущейся папке (`D:\Tools`)
  после этого срабатывает Legacy, данные на месте — это верно.
- **(д) Portable-копия в другой папке.** Установленный лаунчер её не видит и покажет «данных не
  найдено» (135 §3.4). Переход: в старой копии снять Settings → Storage → Portable (данные
  уедут в `%LOCALAPPDATA%`), затем ставить. Либо LX Backup.

Если есть `{app}\bin\wizard_states\state.json`, страница Ready (`UpdateReadyMemo`) добавляет
строку: найдены данные portable-копии; если папка программы закрыта для записи (Program Files),
при первом старте они будут скопированы в `%LOCALAPPDATA%\singbox-launcher`.

## 6. Удаление

1. `InitializeUninstall` → `CloseLauncher` (§4). Отказ — удаление отменяется.
2. Вопрос «Удалить данные и настройки?» (`SuppressibleMsgBox`; по умолчанию и в тихом режиме —
   **Нет**). В тексте сказано, что удаляются данные только текущего пользователя Windows.
3. Служба (после 141). Если есть `HKLM\SYSTEM\CurrentControlSet\Services\sing-box-lxd`, путь к
   копии берётся из `ImagePath`, а не угадывается. Выполняется `<копия> lxd --service=uninstall`,
   при «Да» — с `--purge`. Служба снимается при любом ответе: без лаунчера управлять ею некому.
4. `singbox-launcher.exe -autostart=off` — значение `Run` удаляет код лаунчера (139).
5. При «Да»: `DelTree({app}\bin\wizard_states)`, **затем** `singbox-launcher.exe -purge-data -yes`.
   Порядок важен: повышенный деинсталлятор проходит пробу записи Program Files, и с `state.json`
   из ловушки (а) очистка выбрала бы Legacy — удалила бы устаревшую копию, а настоящие данные в
   `%LOCALAPPDATA%` сохранила (135 §11 з). Сетевая очистка (wintun, `sing-tun`) — внутри.
6. Inno удаляет свои файлы и ярлыки.
7. `usPostUninstall`, при «Да»: `DelTree` только `{app}\bin` и `{app}\logs` (остатки старых
   распаковок), затем `RemoveDir({app})`, если пуст. Каталог целиком не сносится: пользователь
   мог выбрать общую папку. При «Нет» остатки не трогаются: в ловушке (а) до первого старта
   нового лаунчера это единственная копия данных.

**(г) `-H windowsgui` и ожидание.** `Exec(…, ewWaitUntilTerminated)` ждёт дескриптор процесса
независимо от подсистемы exe; `shellexec` и `cmd /c start` не используются. Консоли нет, поэтому
план и отчёт `-purge-data` снимаются `ExecAndCaptureOutput` в лог деинсталлятора. Путь
`-purge-data` в `main.go` идёт до GUI и GL-пробы, окна нет. Код ≠ 0 (лаунчер в чужом сеансе,
живое ядро) — сообщение с выводом и подсказкой Remove all data / `-purge-data -yes`; удаление
программы продолжается.

**(в) При работающей службе** файлы `{app}` не заняты (инвариант §4), шаг 3 снимает службу, VPN
останавливается. До 141 шага 3 нет, как нет и службы.

**Ограничения.** Установка общая, данные у каждого пользователя свои: чистятся данные и
автозапуск учётной записи деинсталлятора (под чужим администратором — его, где данных нет).
Остальным — Remove all data до удаления. Чужое значение `Run` на удалённый exe Windows пропускает.

## 7. Версия и CI

**Версия.** `AppVersion` в Inno — версия `meta` как есть (`v2.1.0`, `v2.1.0-11-g38550b5c-prerelease`,
`dev.develop.38550b5`). Числовая `VersionInfoVersion` (`X.Y.Z.N`) считается отдельно из
`git describe --tags --long --match "v[0-9]*" --exclude "*-prerelease"` (`--match` отсекает
`mesa3d-26.2.0` и `v.dev.*`) разбором `^v(\d+)\.(\d+)\.(\d+)-(\d+)-g`: `v2.1.0-0-g…` → `2.1.0.0`,
`v2.0.2-63-g…` → `2.0.2.63`; режим `build` получает число ближайшего тега. Не разобралось — job
падает, `0.0.0.0` не подставляется.

**Job `build-windows-installer`**: `windows-latest`, `needs: [meta, build-windows]`, условие как
у `build-windows` (тег или `build`/`prerelease` с целью `Win64`).

checkout с `fetch-depth: 0` (нужны `describe`, шаблон, `assets/wintun-*.zip`) → скачать
`artifacts-windows` → `bash build/installer/stage_win64_full.sh <exe> <version> stage` (блок
`ci.yml:698-737`, вынесенный в скрипт без изменений, но без маркера) → число версии → найти
`ISCC.exe` в `%ProgramFiles(x86)%\Inno Setup 6`, иначе `choco install innosetup -y --no-progress` →
`ISCC /DAppVersion=… /DAppVersionNumeric=… /DStageDir=… /Odist build\installer\singbox-launcher.iss`
→ артефакт `artifacts-windows-installer`.

**Job `release`.** Новая job добавляется в `needs` и в условие (`success`/`skipped`).
`win64-full` зовёт тот же скрипт и сам кладёт `portable.txt`, состав zip не меняется.
`…-win64-setup.exe` переносится в корень `release-artifacts`, попадает в `find` для
`checksums.txt` и в `files:`. В разделе Windows тела релиза установщик идёт первым вариантом,
совет распаковывать zip в Program Files убирается.

| `run_mode` / событие | Установщик |
|---|---|
| `tests`, push, PR | не собирается |
| `build` | собирается, артефакт на 30 дней, не публикуется |
| `prerelease`, тег `v*` | собирается, публикуется в релиз, строка в `checksums.txt` |

## 8. Связь с 135 / 139 / 141 / 125

- **135.** AppDir без маркера (135 §3.2), старый маркер удаляется (§5 б); опора на миграцию §3.4,
  правило шаблона §3.3, `-purge-data` (§4.3, Е). Запись в AppDir (Mesa; wintun, §11 ж) в Program
  Files не работает: wintun поставляется рядом с ядром, Mesa — задачей §3.3.
- **139 — обязательная предпосылка.** С `requireAdministrator` «Launch after install» от
  исходного пользователя не стартует, автозапуск из `Run` Windows блокирует, каждый старт — UAC.
  Требования к 139: (1) CLI-вход в код автозапуска, выходящий без GUI (здесь условно
  `-autostart=on|off`, имя за 139); (2) повышенный по требованию процесс получает раскладку от
  неповышенного родителя флагом `-handoff` (139 §5; окружение под `runas` до процесса не
  доходит), а не вычисляет заново — иначе проба записи Program Files вернёт Legacy на остатке
  из ловушки (а), а повышение под чужой учётной записью — её `%LOCALAPPDATA%`.
- **141.** Задача `daemonservice` и шаг 3 удаления включаются, когда лаунчер умеет daemon-режим
  на Windows. Команды, путь копии, обновление службы при обновлении лаунчера и снятие без
  `--purge` определяет 141. Инвариант со стороны 140: служба не запускает ничего из `{app}`.
- **125.** GL-гейт в непишущемся AppDir не предлагает поставить Mesa, а подсказывает задачу
  установщика. Небольшая правка, делается в 140 вместе с мьютексом.

## 9. Подпись кода

Пока не подписываем. Последствия те же, что у exe из zip сегодня: SmartScreen «Windows защитила
компьютер» (More info → Run anyway), в UAC «Неизвестный издатель», ложные срабатывания
антивирусов. Плюс: файлы, записанные установщиком, без Mark-of-the-Web, и установленный лаунчер
SmartScreen не вызывает. Заготовка: закомментированные `SignTool=` и `SignedUninstaller=yes`; в
CI секрет (PFX base64 + пароль) или Azure Trusted Signing; сначала подписать exe, затем Inno
подпишет setup и деинсталлятор. `sing-box.exe` подписывает форк.

## 10. Границы

Не делаем: winget-манифест (отдельная задача: `InstallerType: inno`, URL и sha256 из релиза),
scoop, choco, MSI/MSIX; установку без UAC для одного пользователя, ARM64, Win7; самообновление
(`CheckLauncherVersionOnStartup`, `core/core_version.go:121`, только сообщает версию, кнопка
открывает `releases/latest`, `ui/core_dashboard_tab.go:1157`); подпись (§9); Job Object для ядра
(сирота после `/F` — отдельная задача); замену скана процессов в
`CheckIfLauncherAlreadyRunningUtil` на мьютекс.

## 11. Приёмка (вручную, Windows 10 и 11 x64)

1. **Чистая установка:** один UAC, ярлык в «Пуске», запись в «Приложениях» с версией, нет
   `portable.txt`; лаунчер неповышенный; Storage — Mode System, Data в `%LOCALAPPDATA%`, ядро
   и шаблон из `app`, первый старт ничего не качает.
2. **Задачи:** ярлык на столе; значение `Run` совпадает с чекбоксом 139, после входа лаунчер в
   трее; Mesa — три DLL рядом с exe, на RDP без GPU окно рисуется.
3. **Обновление с VPN** (classic, TUN, `set_system_proxy`): лаунчер закрылся сам за ≤ 20 с,
   `sing-box.exe` нет, прокси снят; данные и автозапуск прежние, шаблон новый.
4. **Лаунчер в чужом сеансе или завис:** Retry/Cancel/Ignore, Ignore завершает процесс.
5. **Удаление, «Нет»:** `{app}` удалён, данные целы, `Run` нет; повторная установка их видит.
6. **Удаление, «Да»:** данные, логи, призрачные адаптеры и правила `sing-tun` удалены, вывод
   `-purge-data` в логе деинсталлятора; с лаунчером в чужом сеансе — сообщение, данные на месте.
7. **Тихий режим:** `/VERYSILENT /TASKS=…` ставит; `unins000.exe /VERYSILENT` данные оставляет.
8. **(а)** Распаковка без маркера в Program Files с данными: строка на Ready, первый старт
   мигрирует (`.migrated_from`, Mode System), удаление с «Да» убирает обе копии.
9. **(б)** С `portable.txt`: маркер удалён, дальше как в п. 8; в `D:\Tools` — Legacy.
10. **(д)** После снятия Portable в старой копии установленный лаунчер видит данные.
11. **Служба (после 141):** удаление при работающей службе снимает её, `{app}` удаляется.
12. **Отказы:** ARM64 — установка не начинается; setup.exe — «Неизвестный издатель», установленный
    exe стартует без SmartScreen.
