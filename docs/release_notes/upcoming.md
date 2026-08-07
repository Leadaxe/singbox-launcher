# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
-

### Fixed

- **Quit from the tray actually terminates the process.** Quitting via the tray menu (or the Exit button) with the main window hidden or unfocused left the process alive: on Windows the tray icon stayed behind as a ghost, and relaunching the `.exe` reported "already running" because the dead-looking process still held the single-instance lock. Root cause is in Fyne's glfw driver: `Quit()` runs its tray teardown only when one of our windows currently holds focus — exactly the opposite of the quit-from-tray situation. The launcher now tears the tray icon down explicitly (`systray.Quit()`, the same call the driver skips: on Windows it deletes the notification-area icon immediately and stops the systray message pump), and a shutdown watchdog force-exits the process within 3 seconds if the Fyne event loop still refuses to unwind — by that point sing-box is already stopped and log files are closed, so nothing is lost.

### Technical / Internal

- `GracefulExit` is now idempotent (`sync.Once`): it is reachable both from the tray/Exit button and from `main()` after `app.Run()` returns, and used to run the whole teardown twice.
- `fyne.io/systray` promoted from indirect to direct dependency in `go.win7.mod` (v1.12.0, version unchanged); `go.mod` already had it direct (v1.12.2).

## RU
### Основное
-

### Исправлено

- **Quit из трея действительно завершает процесс.** Выход через меню трея (или кнопку Exit) при скрытом или расфокусированном главном окне оставлял процесс живым: на Windows иконка в трее висела «призраком», а повторный запуск `.exe` отвечал «процесс уже запущен» — внешне закрытое приложение продолжало держать single-instance lock. Корень — в glfw-драйвере Fyne: `Quit()` выполняет снятие трея только когда одно из окон приложения в фокусе, то есть ровно наоборот к ситуации «выход из трея». Теперь лаунчер снимает иконку явно (`systray.Quit()` — тот самый вызов, который драйвер пропускает: на Windows он немедленно удаляет иконку из области уведомлений и останавливает message pump систрея), а сторожевой таймер завершает процесс принудительно в пределах 3 секунд, если event loop Fyne так и не размотался — к этому моменту sing-box уже остановлен и лог-файлы закрыты, потерь нет.

### Техническое / Внутреннее

- `GracefulExit` теперь идемпотентен (`sync.Once`): он достижим и из трея/кнопки Exit, и из `main()` после возврата `app.Run()`, и раньше прогонял весь teardown дважды.
- `fyne.io/systray` переведён из indirect в прямую зависимость в `go.win7.mod` (v1.12.0, версия не менялась); в `go.mod` он уже был прямым (v1.12.2).
