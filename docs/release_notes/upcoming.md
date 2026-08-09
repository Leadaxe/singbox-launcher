# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights

- **Daemon mode for the core (macOS).** The launcher can now run the VPN core inside a long-lived system service (`sing-box lxd`) instead of spawning `sing-box run` itself — the same in-process, reload-surviving model the Android app uses. Turn it on in **Settings → Core engine (lxd daemon)**. What it buys you:
  - **Password once.** Installing the service asks for an administrator password a single time (to register the LaunchDaemon). After that, starting/stopping the VPN and applying config changes need no password at all.
  - **Quitting the launcher can keep the VPN up.** By default, closing the launcher leaves the core running in the daemon; a "Stop VPN when quitting" toggle restores the classic behavior.
  - **In-process config swaps.** Applying a new config no longer kills and restarts a process: the daemon swaps the core in-place, validates the config in a subprocess before touching the running instance, and auto-rolls-back to the last working config if the new one fails to start.
  - **Richer observability over gRPC.** Proxy groups, node selection, latency tests, live status/traffic, connections, and core logs all flow over the daemon's gRPC channel (the CommandClient protocol shared with the Android line) — including a new **balancer pool** view on the Servers tab showing each urltest slot and its delay.
  - **Pairing.** The launcher pairs with the daemon over mTLS using a one-time invite (`address#fingerprint#code`). For a locally-installed service this happens automatically; for any daemon you can paste an invite in Settings (mint one on the daemon's host with `sing-box lxd client add`).

  The classic engine remains the default and is unchanged; daemon mode is opt-in and requires a core build with the `lxd` subcommand (sing-box-lx 1.14.0-lx.23 or newer).

### Fixed

- **Quit from the tray actually terminates the process.** Quitting via the tray menu (or the Exit button) with the main window hidden or unfocused left the process alive: on Windows the tray icon stayed behind as a ghost, and relaunching the `.exe` reported "already running" because the dead-looking process still held the single-instance lock. Root cause is in Fyne's glfw driver: `Quit()` runs its tray teardown only when one of our windows currently holds focus — exactly the opposite of the quit-from-tray situation. The launcher now tears the tray icon down explicitly (`systray.Quit()`, the same call the driver skips: on Windows it deletes the notification-area icon immediately and stops the systray message pump), and a shutdown watchdog force-exits the process within 3 seconds if the Fyne event loop still refuses to unwind — by that point sing-box is already stopped and log files are closed, so nothing is lost.

### Technical / Internal

- `GracefulExit` is now idempotent (`sync.Once`): it is reachable both from the tray/Exit button and from `main()` after `app.Run()` returns, and used to run the whole teardown twice.
- `fyne.io/systray` promoted from indirect to direct dependency in `go.win7.mod` (v1.12.0, version unchanged); `go.mod` already had it direct (v1.12.2).
- **Core engine abstraction (`CoreBackend`).** The UI, tray, shortcuts, and debug-API no longer call the process manager or Clash API directly; everything routes through the active backend (`LegacyBackend` = classic spawn, `DaemonBackend` = lxd). Proxy-group operations go through a `ProxyTransport` seam (Clash HTTP for classic, gRPC for daemon), so the Servers tab is engine-agnostic. Daemon mode adds `google.golang.org/grpc` + `protobuf` (darwin-only build tags — the win7 build is untouched); the daemon protobuf stubs are vendored from the fork via `scripts/sync_daemonpb.sh`.
- Daemon mode requires a core built with `with_lx_command` (the `lxd` subcommand). `RequiredCoreVersion` must be bumped to a fork release that ships it (1.14.0-lx.23+) before the feature is usable by end users; until then it is developed against a locally built core.

## RU
### Основное

- **Daemon-режим ядра (macOS).** Лаунчер теперь умеет запускать ядро VPN внутри долгоживущей системной службы (`sing-box lxd`), а не спавнить `sing-box run` сам — та же модель «ядро внутри процесса, канал управления переживает перезагрузку конфига», что и в Android-приложении. Включается в **Настройках → Движок ядра (демон lxd)**. Что это даёт:
  - **Пароль — один раз.** Установка службы спрашивает пароль администратора единожды (чтобы зарегистрировать LaunchDaemon). Дальше запуск/остановка VPN и смена конфига идут вообще без пароля.
  - **Выход из лаунчера может оставлять VPN работать.** По умолчанию закрытие лаунчера не выключает ядро в демоне; галочка «Останавливать VPN при выходе» возвращает классическое поведение.
  - **Смена конфига без убийства процесса.** Применение нового конфига больше не перезапускает процесс: демон подменяет ядро на месте, валидирует конфиг сабпроцессом до того, как тронуть работающий инстанс, и автоматически откатывается на последний рабочий конфиг, если новый не стартовал.
  - **Богатая наблюдаемость по gRPC.** Группы прокси, выбор узла, тесты задержки, живой статус/трафик, соединения и логи ядра — всё идёт по gRPC-каналу демона (протокол CommandClient, общий с Android-линией), включая новый экран **пула балансировщика** на вкладке Servers: каждый слот urltest-группы и его задержка.
  - **Сопряжение.** Лаунчер сопрягается с демоном по mTLS через одноразовое приглашение (`адрес#отпечаток#код`). Для локально установленной службы это происходит автоматически; для любого демона можно вставить приглашение в Настройках (выпустить его на хосте демона командой `sing-box lxd client add`).

  Классический движок остаётся по умолчанию и не меняется; daemon-режим включается вручную и требует сборки ядра с сабкомандой `lxd` (sing-box-lx 1.14.0-lx.23 или новее).

### Исправлено

- **Quit из трея действительно завершает процесс.** Выход через меню трея (или кнопку Exit) при скрытом или расфокусированном главном окне оставлял процесс живым: на Windows иконка в трее висела «призраком», а повторный запуск `.exe` отвечал «процесс уже запущен» — внешне закрытое приложение продолжало держать single-instance lock. Корень — в glfw-драйвере Fyne: `Quit()` выполняет снятие трея только когда одно из окон приложения в фокусе, то есть ровно наоборот к ситуации «выход из трея». Теперь лаунчер снимает иконку явно (`systray.Quit()` — тот самый вызов, который драйвер пропускает: на Windows он немедленно удаляет иконку из области уведомлений и останавливает message pump систрея), а сторожевой таймер завершает процесс принудительно в пределах 3 секунд, если event loop Fyne так и не размотался — к этому моменту sing-box уже остановлен и лог-файлы закрыты, потерь нет.

### Техническое / Внутреннее

- `GracefulExit` теперь идемпотентен (`sync.Once`): он достижим и из трея/кнопки Exit, и из `main()` после возврата `app.Run()`, и раньше прогонял весь teardown дважды.
- `fyne.io/systray` переведён из indirect в прямую зависимость в `go.win7.mod` (v1.12.0, версия не менялась); в `go.mod` он уже был прямым (v1.12.2).
- **Абстракция движка ядра (`CoreBackend`).** UI, трей, горячие клавиши и debug-API больше не зовут процесс-менеджер или Clash API напрямую — всё идёт через активный движок (`LegacyBackend` = классический spawn, `DaemonBackend` = lxd). Операции с группами прокси проходят через шов `ProxyTransport` (Clash HTTP для classic, gRPC для daemon), поэтому вкладка Servers не зависит от движка. Daemon-режим добавляет `google.golang.org/grpc` + `protobuf` (build-tag только darwin — win7-сборка не затронута); protobuf-стабы демона вендорятся из форка через `scripts/sync_daemonpb.sh`.
- Daemon-режим требует ядра, собранного с `with_lx_command` (сабкоманда `lxd`). Перед тем как фича станет доступна конечным пользователям, `RequiredCoreVersion` нужно поднять до релиза форка с этим тегом (1.14.0-lx.23+); до этого разработка идёт против локально собранного ядра.
