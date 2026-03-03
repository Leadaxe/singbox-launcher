# Отчёт: привилегированный запуск sing-box на macOS

## Описание задачи

На macOS создание TUN-интерфейса требует прав администратора (root). При обычном запуске лаунчера (двойной клик по `.app` или `open singbox-launcher.app`) процесс sing-box запускается от имени пользователя и не может поднять TUN.

**Цель:** дать пользователю возможность запускать sing-box с повышенными правами из лаунчера без ручного `sudo` в терминале — по нажатию Start (при конфиге с TUN) показывать системный запрос пароля и запускать sing-box с правами root (TUN и другие привилегированные функции работают).

**Ограничения:**
- Не запускать весь лаунчер под `sudo` (неудобно и может ломать GUI).
- Сохранить единый UX: логи в тот же `sing-box.log`, мониторинг, авторестарт и кнопка Stop.

---

## Концепция решения

Использовать **macOS Security framework** и устаревший, но поддерживаемый API **AuthorizationExecuteWithPrivileges**: он показывает системный диалог «приложение запрашивает права администратора», пользователь вводит пароль, после чего указанная программа выполняется с повышенными правами.

**Когда включается привилегированный режим:** только если в конфиге есть TUN inbound (**ConfigHasTun**). На darwin без TUN sing-box стартует обычным способом (без пароля).

**Один пароль на сессию:** один **AuthorizationRef** создаётся при первом вызове **RunWithPrivileges** и переиспользуется для всех последующих вызовов (следующий Start, Stop, авторестарт). При выходе из приложения вызывается **FreePrivilegedAuthorization()** (в **GracefulExit**).

Схема старта:

1. **Скрипт** **bin/start-singbox-privileged.sh** перезаписывается при каждом Start. Он:
   - выводит в stdout свой PID (`echo $$`) — первая строка в pipe;
   - делает `cd binDir`;
   - запускает sing-box **в фоне одной командой** (`sing-box run ... >> log 2>&1 &`), чтобы `$!` был PID именно sing-box, а не подоболочки;
   - выводит PID sing-box (`echo $!`) — вторая строка в pipe;
   - перенаправляет свой stdout в лог (`exec 1>>log 2>&1`) и ждёт выхода sing-box (`wait`).
2. Скрипт запускается через **RunWithPrivileges** → запрос пароля (только при первом действии в сессии), скрипт выполняется с правами root.
3. В C из pipe читаются **две строки** (script PID, sing-box PID), **RunWithPrivileges** возвращает `(scriptPID, singboxPID, nil)` и сразу выходит (до завершения скрипта). Горутина в Go сохраняет оба PID, пишет их в **bin/singbox.pid** (две строки), затем вызывает **WaitForPrivilegedExit(scriptPID)** (Wait4) и по возврату — **onPrivilegedScriptExited()** (краш/рестарт или сброс состояния).
4. **Остановка:** Stop вызывает **RunWithPrivileges** с `kill -TERM scriptPID; kill -TERM singboxPID; rm -f pidFile` (оба PID берутся из памяти контроллера). Пароль не запрашивается повторно, если ref уже получен.

Скрипт и PID-файл лежат в **bin/** для диагностики.

---

## Логика

### Когда включается привилегированный режим

- **Платформа:** только `runtime.GOOS == "darwin"`.
- **Условие:** при Start вызывается **config.ConfigHasTun(configPath)**. Если в конфиге есть inbound типа `"tun"` — используется **startSingBoxPrivileged()** (запрос пароля при первом действии). Если TUN нет — обычный **exec.Command** + **Monitor** (без пароля). При ошибке чтения/разбора конфига считается, что TUN нет — обычный запуск без пароля (избегаем лишнего запроса при повреждённом или недоступном конфиге).

### Старт (startSingBoxPrivileged)

1. Подготовка путей: `binDir`, имя конфига, путь к `sing-box.log`; при необходимости — ротация лог-файла.
2. Пути: **bin/start-singbox-privileged.sh**, **bin/singbox.pid**.
3. Запись скрипта в **bin/** (`os.WriteFile`, 0700):
   - `echo $$`
   - `cd binDir`
   - `singboxPath run -c configName >> logPath 2>&1 &`
   - `echo $!`
   - `exec 1>>logPath 2>&1`
   - `wait`
   Все пути в скрипте экранированы через `strconv.Quote`. Редирект вывода sing-box в лог делается в той же команде с `&`, чтобы в pipe попадали только два числа (PID скрипта и PID sing-box).
4. **RunWithPrivileges("/bin/sh", []string{scriptPath})** вызывается в горутине. В C: при первом вызове создаётся и сохраняется **AuthorizationRef** (g_privilegedAuthRef), при последующих — переиспользуется. Из pipe читаются две строки (script PID, sing-box PID), pipe закрывается, функция возвращается (скрипт продолжает работать).
5. Горутина получает `(scriptPID, singboxPID, err)`, отправляет оба в канал; основной поток ждёт канал и при валидном scriptPID возвращает управление из **startSingBoxPrivileged**.
6. В горутине: установка **SingboxPrivilegedMode**, **SingboxPrivilegedPID**, **SingboxPrivilegedSingboxPID**, **SingboxPrivilegedPIDFile**, **RunningState**; запись в **bin/singbox.pid** двух строк (script PID, sing-box PID).
7. **platform.WaitForPrivilegedExit(scriptPID)** — блокируется до выхода процесса скрипта (Wait4). Когда скрипт завершается (sing-box уже завершён), вызывается **onPrivilegedScriptExited()** (сброс состояния, при краше — авторестарт и т.д.).

### Выход скрипта (onPrivilegedScriptExited)

- Вызывается, когда **Wait4(scriptPID)** в горутине возвращается (скрипт завершился).
- Под блокировкой: сброс **SingboxPrivilegedMode**, **SingboxPrivilegedPID**, **SingboxPrivilegedSingboxPID**, **SingboxPrivilegedPIDFile**, **RunningState**. Если **StoppedByUser** — штатная остановка; иначе — краш: **ConsecutiveCrashAttempts++**, при превышении лимита — сообщение, иначе — авторестарт через **Start(true)** и таймер стабильности (как в обычном Monitor).

### Остановка (Stop при привилегированном режиме)

1. Под блокировкой проверяется **SingboxPrivilegedMode**, **SingboxPrivilegedPID**, **SingboxPrivilegedPIDFile**.
2. Сохраняются **pidFile**, **scriptPID**, **singboxPID** (из **SingboxPrivilegedSingboxPID**); мьютекс отпускается.
3. **RunWithPrivileges("/bin/sh", []string{"-c", "kill -TERM scriptPID [; kill -TERM singboxPID] ; rm -f pidFile"})** — оба PID убиваются явно (без process group). singboxPID добавляется в команду только если > 0.
4. Под блокировкой сбрасываются **SingboxPrivilegedMode**, **SingboxPrivilegedPID**, **SingboxPrivilegedSingboxPID**, **SingboxPrivilegedPIDFile**, **RunningState**, **StoppedByUser**.

### Переиспользование AuthorizationRef и выход из приложения

- В C (privileged_darwin.go): статическая переменная **g_privilegedAuthRef**. При первом вызове **runWithPrivileges** выполняется **AuthorizationCreate** и результат сохраняется; при следующих вызовах используется тот же ref. После **AuthorizationExecuteWithPrivileges** ref не освобождается.
- **FreePrivilegedAuthorization()** (C + Go): освобождает **g_privilegedAuthRef**. Вызывается в **GracefulExit()** после **StopSingBoxProcess()** (только на darwin). На не-darwin — заглушка.

### Диалог «Sing-Box already running»

При проверке «уже запущен» (Start или CheckIfRunningAtStart) если найден процесс sing-box, показывается диалог «Would you like to kill the existing process?». При нажатии **Kill Process**:
- **На darwin:** выполняется **RunWithPrivileges** с командой, возвращаемой **buildPrivilegedKillByPatternScript()** — `pkill -TERM -f "sing-box run|start-singbox-privileged"`, чтобы завершить и скрипт, и sing-box даже при запуске от root. Используется тот же кэшированный AuthorizationRef (пароль не запрашивается повторно при уже полученном ref).
- **На остальных ОС:** **platform.KillProcess(processName)** (killall и т.п.).

### Константы и формирование команд (process_service)

В одном месте заданы имена и паттерн для привилегированного режима, чтобы не дублировать «магию» по коду:
- **privilegedScriptName** — `"start-singbox-privileged.sh"` (имя скрипта в bin/).
- **privilegedPidFileName** — `"singbox.pid"` (имя PID-файла в bin/).
- **privilegedPkillPattern** — `"sing-box run|start-singbox-privileged"` (паттерн для pkill при убийстве по имени процессов).
- **buildPrivilegedKillByPatternScript()** — возвращает строку команды для sh -c: `pkill -TERM -f "<pattern>" 2>/dev/null`. Используется в диалоге «already running» при вызове RunWithPrivileges.

Пути скрипта и PID-файла собираются как `filepath.Join(binDir, privilegedScriptName)` и `filepath.Join(binDir, privilegedPidFileName)`.

### Идентификация процесса

- **getTrackedPID()**: при **SingboxPrivilegedMode** и **SingboxPrivilegedPID != 0** возвращается script PID; иначе — PID из **SingboxCmd**.

### Визард и конфиг с TUN на darwin

Конфиг собирается в визарде из шаблона **bin/wizard_template.json**. В шаблоне в **params** заданы платформозависимые подстановки:
- **platforms: ["darwin"]** — подставляет в **inbounds** только mixed (прокси без TUN);
- **platforms: ["darwin-tun"]**, **mode: "prepend"** — при включённой галочке TUN добавляет TUN inbound в начало списка.

При сборке конфига на darwin вызывается **GetEffectiveConfig(rawConfig, params, goos, model.EnableTunForMacOS)**. Параметр **enableTunForDarwin** задаётся галочкой **TUN** на вкладке Rules визарда; при **true** матчится платформа **darwin-tun**, и в конфиг попадает секция TUN. Значение галочки сохраняется в состоянии визарда как **enable_tun_macos** (config_params в state.json). Итог: при сохранении конфига из визарда с включённой галочкой TUN в **config.json** записываются оба inbound (tun и mixed); при следующем Start лаунчер по **ConfigHasTun** запросит пароль.

---

## Архитектура

### Компоненты

| Компонент | Назначение |
|-----------|------------|
| **internal/platform.RunWithPrivileges(toolPath, args)** | Запуск программы с повышенными правами. Возвращает **(scriptPID, singboxPID int, err)**. Darwin: один кэшированный AuthorizationRef, чтение двух строк из pipe (PID скрипта и sing-box). Не-darwin: заглушка (0, 0, err). |
| **internal/platform.WaitForPrivilegedExit(pid)** | Darwin: Wait4(pid). Не-darwin: no-op. Используется в горутине после RunWithPrivileges для ожидания выхода скрипта и вызова onPrivilegedScriptExited. |
| **internal/platform.FreePrivilegedAuthorization()** | Darwin: освобождение кэшированного AuthorizationRef. Не-darwin: no-op. Вызывается в GracefulExit. |
| **internal/platform/privileged_darwin.go** | CGO: g_privilegedAuthRef, runWithPrivileges (создание ref при первом вызове, два fgets из pipe, без AuthorizationFree), freePrivilegedAuthorization. |
| **internal/platform/privileged_stub.go** | Заглушки для не-darwin. |
| **core/controller** | Поля: **SingboxPrivilegedMode**, **SingboxPrivilegedPID**, **SingboxPrivilegedSingboxPID**, **SingboxPrivilegedPIDFile**. В **GracefulExit** — вызов **platform.FreePrivilegedAuthorization()** на darwin. |
| **core/process_service** | Константы **privilegedScriptName**, **privilegedPidFileName**, **privilegedPkillPattern**; **buildPrivilegedKillByPatternScript()** для команды kill по паттерну. Start: на darwin **ConfigHasTun** → при TUN **startSingBoxPrivileged** (скрипт и pid-файл по константам, cd + sing-box &, два PID из RunWithPrivileges, Wait4 + onPrivilegedScriptExited); иначе обычный exec + Monitor. Stop: при привилегированном режиме — kill обоих PID через RunWithPrivileges. Диалог «already running»: на darwin kill через RunWithPrivileges + buildPrivilegedKillByPatternScript(). **getTrackedPID** учитывает SingboxPrivilegedPID. |
| **core/config.ConfigHasTun(configPath)** | Определяет наличие inbound типа `"tun"` в JSON конфиге. Используется для выбора привилегированного старта на darwin. |

### Поток данных

- **Обычный режим (не macOS или darwin без TUN):** **exec.Command** → **SingboxCmd** → **Monitor(cmd)** → **cmd.Wait()**; Stop — сигнал по **SingboxCmd.Process**.
- **Привилегированный режим (darwin + TUN):** скрипт **bin/start-singbox-privileged.sh** через **RunWithPrivileges** → два PID из pipe → **SingboxPrivilegedPID**, **SingboxPrivilegedSingboxPID**, запись в **bin/singbox.pid** (две строки) → горутина ждёт **Wait4(scriptPID)** → **onPrivilegedScriptExited**. Stop — **RunWithPrivileges** с `kill -TERM scriptPID [; kill -TERM singboxPID]; rm -f pidFile`.

Оба пути приводят к одному **RunningState**, одному лог-файлу и одной кнопке Stop.

### Зависимости

- **CGO** на darwin: для **privileged_darwin.go** нужны `-framework Security` и `-framework Foundation**.

---

## Главные особенности

1. **Пароль только когда нужен TUN** — привилегированный старт на darwin только при **ConfigHasTun**; без TUN — обычный запуск без пароля. При ошибке **ConfigHasTun** (файл не найден, не разобран) — тоже обычный запуск без пароля.
2. **Один пароль на сессию** — один **AuthorizationRef** создаётся при первом RunWithPrivileges и переиспользуется; при выходе из приложения вызывается **FreePrivilegedAuthorization()**.
3. **Два PID** — скрипт выводит свой PID и PID sing-box (благодаря запуску sing-box одной командой с `&`, без подоболочки); оба читаются из pipe в C и возвращаются в Go. Stop убивает оба процесса явно (kill -TERM для каждого).
4. **Скрипт без подоболочки для sing-box** — `cd binDir` отдельной строкой, затем `sing-box ... &`, чтобы `$!` был PID sing-box. PID-файл — две строки (script PID, sing-box PID), пишется из Go.
5. **Один API для старта и остановки** — RunWithPrivileges для запуска скрипта и для kill; без повторного запроса пароля в рамках сессии.
6. **Выход по Wait4** — горутина после RunWithPrivileges вызывает **WaitForPrivilegedExit(scriptPID)** и по возврату **onPrivilegedScriptExited** (краш/рестарт или сброс). Отдельного опроса по таймеру нет.
7. **Устаревший, но рабочий API** — **AuthorizationExecuteWithPrivileges** помечен как deprecated; предупреждение подавлено в CGO.
8. **Платформенная изоляция** — darwin-специфика в **internal/platform** (CGO + stub); в **core** — вызов RunWithPrivileges, ConfigHasTun, ветвление по **SingboxPrivilegedMode** в Stop/getTrackedPID и вызов FreePrivilegedAuthorization в GracefulExit.
9. **Один источник имён и kill по паттерну** — константы **privilegedScriptName**, **privilegedPidFileName**, **privilegedPkillPattern** и функция **buildPrivilegedKillByPatternScript()** в process_service; скрипт, pid-файл и команда для диалога «already running» собираются из них.
10. **Диалог «already running» с привилегиями на macOS** — при нажатии «Kill Process» на darwin выполняется RunWithPrivileges с pkill по паттерну (скрипт + sing-box), чтобы можно было завершить процессы, запущенные от root.

---

## Файлы изменений (кратко)

- **core/controller.go** — поля **SingboxPrivilegedMode**, **SingboxPrivilegedPID**, **SingboxPrivilegedSingboxPID**, **SingboxPrivilegedPIDFile**; в **GracefulExit** — **platform.FreePrivilegedAuthorization()** на darwin.
- **core/process_service.go** — константы **privilegedScriptName**, **privilegedPidFileName**, **privilegedPkillPattern**; **buildPrivilegedKillByPatternScript()**. Start: на darwin **ConfigHasTun** → при true startSingBoxPrivileged (пути по константам, cd + sing-box &, два PID из RunWithPrivileges, pid-файл две строки, Wait4 + onPrivilegedScriptExited); при ошибке ConfigHasTun — **hasTun = false** (обычный запуск без пароля); Stop: kill обоих PID через RunWithPrivileges; при диалоге «already running» на darwin — kill через RunWithPrivileges + buildPrivilegedKillByPatternScript(); getTrackedPID с учётом привилегированного режима.
- **core/config/config_loader.go** — **ConfigHasTun(configPath)** для выбора привилегированного старта на darwin.
- **internal/platform/privileged_darwin.go** — кэш **g_privilegedAuthRef**, runWithPrivileges (две строки из pipe, без Free после использования), **freePrivilegedAuthorization**; Go: **FreePrivilegedAuthorization()**, **WaitForPrivilegedExit(pid)**.
- **internal/platform/privileged_stub.go** — заглушки RunWithPrivileges (0, 0, err), WaitForPrivilegedExit, FreePrivilegedAuthorization.
- **ui/wizard** (конфиг с TUN): **template/loader.go** — RawConfig/Params, **GetEffectiveConfig(raw, params, goos, enableTunForDarwin)**, **matchesPlatform** с учётом darwin-tun; **models/wizard_model.go** — поле **EnableTunForMacOS**; **tabs/rules_tab.go** — чекбокс TUN на darwin рядом с Final outbound; **presenter_state.go** — **enable_tun_macos** в config_params при сохранении/загрузке state; **business/create_config.go** — на darwin сборка секций через GetEffectiveConfig с model.EnableTunForMacOS. Шаблон **bin/wizard_template.json** содержит params с platforms **["darwin"]** (mixed) и **["darwin-tun"]** (prepend TUN).

Сборка: `CGO_ENABLED=1 GOOS=darwin go build .` проходит успешно; возможны предупреждения линкера (например, дубликат `-lobjc`), на работу не влияют.
