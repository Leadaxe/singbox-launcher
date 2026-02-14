# Отчёт: привилегированный запуск sing-box на macOS

## Описание задачи

На macOS создание TUN-интерфейса требует прав администратора (root). При обычном запуске лаунчера (двойной клик по `.app` или `open singbox-launcher.app`) процесс sing-box запускается от имени пользователя и не может поднять TUN.

**Цель:** дать пользователю возможность запускать sing-box с повышенными правами из лаунчера без ручного `sudo` в терминале — по нажатию Start показывать системный запрос пароля и запускать sing-box с правами root (TUN и другие привилегированные функции работают).

**Ограничения:**
- Не запускать весь лаунчер под `sudo` (неудобно и может ломать GUI).
- Сохранить единый UX: логи в тот же `sing-box.log`, мониторинг, авторестарт и кнопка Stop.

---

## Концепция решения

Использовать **macOS Security framework** и устаревший, но поддерживаемый API **AuthorizationExecuteWithPrivileges**: он показывает системный диалог «приложение запрашивает права администратора», пользователь вводит пароль, после чего указанная программа выполняется с повышенными правами.

API **синхронный** — возвращает управление после завершения запущенной программы. Поэтому нельзя «просто» запустить sing-box и сразу вернуться в лаунчер. Схема:

1. **Старт:** лаунчер записывает в **bin/** постоянный скрипт `start-singbox-privileged.sh`, который:
   - в фоне запускает sing-box с нужными аргументами и перенаправлением вывода в `sing-box.log`;
   - записывает PID дочернего процесса в **bin/singbox.pid** и делает `chmod 644`, чтобы лаунчер (пользователь) мог прочитать файл после записи от root;
   - сразу завершается.
2. Скрипт запускается через **RunWithPrivileges** → запрос пароля, скрипт выполняется с правами root, sing-box тоже получает root.
3. После выхода скрипта лаунчер ждёт 250 ms и один раз читает PID из **bin/singbox.pid**, дальше работает с этим PID: мониторинг, авторестарт, Stop.
4. **Остановка:** процесс sing-box принадлежит root, поэтому Stop при привилегированном режиме снова вызывает **RunWithPrivileges** с командой `kill -TERM $(cat bin/singbox.pid); rm -f bin/singbox.pid` — пользователь вводит пароль для остановки.

Скрипт и PID-файл лежат в **bin/** для удобной диагностики (можно открыть скрипт и посмотреть PID при проблемах).

---

## Логика

### Когда включается привилегированный режим

- **Платформа:** только `runtime.GOOS == "darwin"`.
- **Условие:** на macOS при нажатии Start всегда используется привилегированный путь (проверка TUN в конфиге не выполняется).

При darwin вызывается **startSingBoxPrivileged()**, иначе — обычный путь через **exec.Command** и **Monitor**.

### Старт (startSingBoxPrivileged)

1. Подготовка путей: `binDir`, имя конфига, путь к `sing-box.log`; при необходимости — ротация лог-файла.
2. Пути скрипта и PID-файла: **bin/start-singbox-privileged.sh**, **bin/singbox.pid** (в каталоге приложения, не во временной директории).
3. Запись скрипта в **bin/** (`os.WriteFile`, режим 0700):
   - `cd binDir && singboxPath run -c configName >> logPath 2>&1 &`
   - `echo $! > pidFilePath && chmod 644 pidFilePath`
   Все пути в скрипте экранированы через `strconv.Quote`.
4. Вызов **platform.RunWithPrivileges("/bin/sh", []string{scriptPath})** — блокируется до выхода скрипта, пользователь видит запрос пароля.
5. В C-коде (privileged_darwin.go) pipe от дочернего процесса **не читается** — только **fclose(pipe)**. Иначе лаунчер зависал бы: sing-box запускается в фоне и наследует этот pipe, EOF не приходит.
6. Задержка **250 ms**, затем **одно** чтение PID из **bin/singbox.pid** и проверка валидности.
7. Установка в контроллере: **SingboxPrivilegedMode = true**, **SingboxPrivilegedPID**, **SingboxPrivilegedPIDFile**; **SingboxCmd = nil**.
8. **RunningState.Set(true)**, отложенная загрузка прокси, запуск **MonitorPrivileged(pid, pidFilePath)**.
9. Скрипт в bin/ не удаляется (остаётся для диагностики); **singbox.pid** удаляется при Stop.

### Мониторинг (MonitorPrivileged)

- Раз в секунду проверяется наличие процесса по PID (**process.FindProcess**).
- Если процесс исчез:
  - под блокировкой проверяется, что это всё ещё «наш» привилегированный запуск (по **SingboxPrivilegedMode** и **SingboxPrivilegedPID**);
  - если перед выходом был выставлен **StoppedByUser** — штатная остановка, сброс состояния и счётчика крашей;
  - иначе — краш: сброс привилегированного состояния, увеличение **ConsecutiveCrashAttempts**, при превышении лимита — сообщение и выход;
  - иначе — вызов **svc.Start(true)** для авторестарта (снова запрос пароля) и таймер стабильности (сброс счётчика через 3 минуты), как в обычном **Monitor**.

### Остановка (Stop при привилегированном режиме)

1. Под блокировкой проверяется **SingboxPrivilegedMode** и наличие **SingboxPrivilegedPIDFile**.
2. Сохраняются **pidFile** и **pid** в локальные переменные, мьютекс отпускается.
3. Вызов **RunWithPrivileges("/bin/sh", []string{"-c", "kill -TERM $(cat pidFile) 2>/dev/null; rm -f pidFile"})** — запрос пароля, SIGTERM, удаление pid-файла.
4. Под блокировкой сбрасываются **SingboxPrivilegedMode**, **SingboxPrivilegedPID**, **SingboxPrivilegedPIDFile**, **RunningState**, **StoppedByUser**.

### Идентификация процесса

- **getTrackedPID()**: если **SingboxPrivilegedMode** и **SingboxPrivilegedPID != 0**, возвращается этот PID; иначе — PID из **SingboxCmd**. Так проверка «уже запущен sing-box» и остальная логика работают и для привилегированного режима.

---

## Архитектура

### Новые/изменённые компоненты

| Компонент | Назначение |
|-----------|------------|
| **internal/platform.RunWithPrivileges(toolPath, args)** | Запуск указанной программы с повышенными правами (darwin: Security framework; не-darwin: заглушка с ошибкой). |
| **internal/platform/privileged_darwin.go** | CGO-обёртка над AuthorizationCreate + AuthorizationExecuteWithPrivileges; pipe только закрывается (fclose), не читается, чтобы не зависать. |
| **internal/platform/privileged_stub.go** | Заглушка для не-darwin (build tag `!darwin`). |
| **core/controller (AppController)** | Поля: **SingboxPrivilegedMode**, **SingboxPrivilegedPID**, **SingboxPrivilegedPIDFile**. |
| **core/process_service** | Ветвление Start (darwin → startSingBoxPrivileged); **startSingBoxPrivileged** (скрипт и PID в bin/, одна попытка чтения PID после 250 ms), **MonitorPrivileged**; в **Stop** — ветка привилегированного kill; в **getTrackedPID** — учёт привилегированного PID. |

(Функция **core/config.ConfigHasTun** в config_loader по желанию можно использовать для других целей; для решения «когда включать привилегированный режим» она не используется — на darwin всегда привилегированный старт.)

### Поток данных

- **Обычный режим (не macOS):** **exec.Command** → **SingboxCmd** → **Monitor(cmd)** → **cmd.Wait()**; Stop — сигнал по **SingboxCmd.Process**.
- **Привилегированный режим (darwin):** скрипт **bin/start-singbox-privileged.sh** через **RunWithPrivileges** → PID в **bin/singbox.pid** → **SingboxPrivilegedPID** + **SingboxPrivilegedPIDFile** → **MonitorPrivileged** (опрос по PID); Stop — **RunWithPrivileges** с `kill -TERM $(cat pidFile); rm -f pidFile`.

Оба пути приводят к одному **RunningState**, одному лог-файлу и одной кнопке Stop.

### Зависимости

- **CGO** на darwin: для **privileged_darwin.go** нужны `-framework Security` и `-framework Foundation`. Сборка macOS уже использует CGO.

---

## Главные особенности

1. **На macOS всегда запрос пароля при Start** — привилегированный путь используется при каждом запуске на darwin, без проверки наличия TUN в конфиге.
2. **Один API для старта и остановки** — и запуск скрипта, и `kill` выполняются через **RunWithPrivileges** (пароль при Start и при Stop, или при авторестарте после краша).
3. **Устаревший, но рабочий API** — **AuthorizationExecuteWithPrivileges** помечен как deprecated; предупреждение компилятора подавлено через `#pragma clang diagnostic` в CGO.
4. **Скрипт и PID в bin/** — скрипт **bin/start-singbox-privileged.sh** перезаписывается при каждом старте, не удаляется; **bin/singbox.pid** создаётся скриптом с `chmod 644` для чтения лаунчером, удаляется при Stop. Удобно для диагностики.
5. **Нет зависания из‑за pipe** — в C после **AuthorizationExecuteWithPrivileges** pipe только закрывается (fclose), не читается до EOF, иначе лаунчер зависал бы (sing-box в фоне наследует pipe).
6. **Одна попытка чтения PID** — после выхода из RunWithPrivileges пауза 250 ms, затем одно чтение **bin/singbox.pid**; при ошибке или невалидном PID — сообщение с путём к файлу и отсылкой к скрипту и логу sing-box.
7. **Паритет с обычным режимом** — авторестарт (до 3 попыток), таймер стабильности (3 минуты), логи в **sing-box.log**, автозагрузка прокси, **getTrackedPID** и проверка «sing-box уже запущен».
8. **Платформенная изоляция** — darwin-специфика в **internal/platform** (CGO + stub); в **core** только вызов **RunWithPrivileges** и ветвление по **SingboxPrivilegedMode** в Stop/getTrackedPID.

---

## Файлы изменений (кратко)

- **core/controller.go** — поля **SingboxPrivilegedMode**, **SingboxPrivilegedPID**, **SingboxPrivilegedPIDFile**.
- **core/process_service.go** — ветвление Start (darwin → startSingBoxPrivileged); **startSingBoxPrivileged** (скрипт и PID в bin/, 250 ms + одно чтение PID), **MonitorPrivileged**; ветка Stop для привилегированного kill; учёт привилегированного PID в **getTrackedPID**.
- **internal/platform/privileged_darwin.go** — **RunWithPrivileges** (CGO + Security.framework), pipe только закрывается.
- **internal/platform/privileged_stub.go** — заглушка для не-darwin.

Опционально: **core/config/config_loader.go** — функция **ConfigHasTun** (сейчас не используется для выбора режима запуска).

Сборка: `CGO_ENABLED=1 GOOS=darwin go build .` проходит успешно; возможны предупреждения линкера (например, дубликат `-lobjc`), на работу не влияют.
