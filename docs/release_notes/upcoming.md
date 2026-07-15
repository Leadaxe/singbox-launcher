# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

## EN
### Highlights
- **Core bumped to sing-box-lx `1.14.0-lx.5`** (from `1.14.0-lx.4`). Same build tags; the fork's "energy revision" lands: suspended WireGuard/AmneziaWG tunnels are no longer resurrected by a screen-off/on or a network change, idle-suspend no longer cuts live transfers, and an abandoned urltest group stops probing (and waking) its members. Two new opt-in knobs: `route.lx_idle_suspend_reachable` and `urltest.passive_check`. Update via Core Dashboard → Download/Reinstall; no config migration needed. (SPEC 072)
- **Subscription credentials are no longer corrupted.** The parser double-decoded userinfo: a `+` in a trojan / hysteria2 / tuic / ssh / socks / naive password became a space, and literal `%XX` sequences were decoded twice — those nodes silently failed to authenticate. SSH keys were hit hardest (a PEM body is full of `+`).
- **One bad node no longer breaks the whole config.** A port outside 1..65535 now degrades just that node instead of reaching config.json and failing `sing-box check` for everything. Same for junk REALITY `public_key` coming from Xray-JSON subscriptions (the URI path already guarded this) — the node falls back to plain TLS with a warning.
- **Auto-update opt-out is now respected on startup.** With auto-update disabled, the 30-second startup sweep still fetched every subscription once per launch.
- **A failed core update no longer destroys the working core.** If the copy fails mid-way, the previous binary is restored from `.old` instead of leaving a truncated file; a partially extracted `wintun.dll` is no longer installed as valid. After a successful core install the dashboard shows the new version without restarting the launcher.

### Technical / Internal
- `core/services/file_service.go`: log rotation renamed `sing-box.log` out from under the open `ChildLogFile` descriptor — after the first mid-session rotation all sing-box output went to `.old`, which never rotates again (unbounded growth). The fd is now closed before the rename and reopened after.
- `core/process_service.go`: `Start()` re-checks `RunningState` under `CmdMutex` — two concurrent starts could launch two sing-box processes and orphan the first Cmd handle.
- `core/debugapi`: `PATCH /state/*` and `/settings/user-agent` load-modify-save cycles are serialized (concurrent patches lost one side's edit); the Serve goroutine snapshots `httpSrv`/`listener` under the mutex (`Stop()` nils them → nil-deref crash); GET `/state/*` maps `state.ErrNotFound` to 404 like `/state/full`; `DELETE /traffic/sessions/{id}` reports deletion from `DeleteSession` instead of a count-delta heuristic that raced concurrent stop/clear.
- `api/`: `pingTestURL` / `pingTestAllConcurrency` are mutex-guarded (UI writes, ping workers read); api-log writes happen under the read lock instead of after it.
- `core/warp`: share-URI hosts are built with `net.JoinHostPort` — IPv6 endpoints lost their brackets and produced unparseable URIs.
- `shareuri_{hysteria2,tuic,anytls}.go`: emit `fp=` from `tls.utls.fingerprint` — the parsers read it but the emitters dropped it on round-trip.
- Lint sweep: staticcheck 49 → 5 findings (gofmt, S1000/S1003/S1011/S1021, ST1005/ST1006/ST1019, dead code in `clash_remote_ui.go`, SA4000/SA4023), deprecated Fyne APIs replaced (`SelectTab`→`Select`, `OnChanged`→`OnSelected`, `Wrapping = TextTruncate`→`Truncation`, `PlaceHolderColor`, `Window.Clipboard`→`App.Clipboard`).
- `docs/API.md` synced with the code: examples on v1.2.1/lx.4, the non-existent "Copy snapshot" button removed, Debug API UI references point at Settings (not Diagnostics), `/debug/snapshot` shows `errors` as an object with an omitempty note, the traffic rolling buffer is documented as 60s (`last=` clamps at 10 min), wiring path corrected. `docs/TRAFFIC_PROFILER.md`: `router.find_process` → `route.find_process`.

## RU
### Основное
- **Ядро обновлено до sing-box-lx `1.14.0-lx.5`** (с `1.14.0-lx.4`). Те же build-теги; приезжает «энергетическая ревизия» форка: приостановленные WireGuard/AmneziaWG-туннели больше не «воскресают» от выключения/включения экрана или смены сети, idle-suspend больше не рвёт активные передачи, а покинутая urltest-группа перестаёт пинговать (и будить) свои ноды. Две новые опциональные настройки: `route.lx_idle_suspend_reachable` и `urltest.passive_check`. Обновить: Core Dashboard → Download/Reinstall; миграция конфигов не нужна. (SPEC 072)
- **Пароли из подписок больше не портятся.** Парсер дважды декодировал userinfo: `+` в пароле trojan / hysteria2 / tuic / ssh / socks / naive превращался в пробел, а последовательности `%XX` декодировались повторно — такие ноды молча не проходили аутентификацию. Сильнее всего страдали SSH-ключи (в теле PEM полно `+`).
- **Одна битая нода больше не ломает весь конфиг.** Порт вне диапазона 1..65535 теперь деградирует только свою ноду, а не доезжает до config.json и не валит `sing-box check` целиком. То же для мусорного REALITY `public_key` из Xray-JSON подписок (в URI-пути защита уже была) — нода откатывается на обычный TLS с предупреждением.
- **Выключенное автообновление теперь уважается при старте.** При отключённом автообновлении стартовый проход (через 30 секунд после запуска) всё равно фетчил все подписки один раз за запуск.
- **Неудачное обновление ядра больше не уничтожает рабочее.** Если копирование обрывается, прежний бинарь восстанавливается из `.old`, а не остаётся обрезанный файл; частично распакованный `wintun.dll` больше не ставится как валидный. После успешной установки ядра дашборд показывает новую версию без перезапуска лаунчера.

### Техническое / Внутреннее
- `core/services/file_service.go`: ротация переименовывала `sing-box.log` из-под открытого дескриптора `ChildLogFile` — после первой ротации в течение сессии весь вывод sing-box уходил в `.old`, который больше никогда не ротируется (неограниченный рост). Теперь дескриптор закрывается до rename и переоткрывается после.
- `core/process_service.go`: `Start()` перепроверяет `RunningState` под `CmdMutex` — два конкурентных старта могли запустить два процесса sing-box и потерять трекинг первого.
- `core/debugapi`: циклы load-modify-save в `PATCH /state/*` и `/settings/user-agent` сериализованы (конкурентные патчи теряли одну из правок); goroutine `Serve` снимает `httpSrv`/`listener` под мьютексом (`Stop()` обнуляет их → nil-deref и падение); GET `/state/*` маппит `state.ErrNotFound` в 404, как `/state/full`; `DELETE /traffic/sessions/{id}` берёт факт удаления из `DeleteSession`, а не из разницы счётчиков, которая гонялась с конкурентным stop/clear.
- `api/`: `pingTestURL` / `pingTestAllConcurrency` под мьютексом (пишет UI, читают ping-воркеры); запись в api-лог идёт под read-локом, а не после него.
- `core/warp`: хосты share-URI собираются через `net.JoinHostPort` — IPv6-endpoint'ы теряли квадратные скобки и давали непарсибельные URI.
- `shareuri_{hysteria2,tuic,anytls}.go`: эмитят `fp=` из `tls.utls.fingerprint` — парсеры его читали, а эмиттеры теряли на round-trip.
- Прогон линтеров: staticcheck 49 → 5 находок (gofmt, S1000/S1003/S1011/S1021, ST1005/ST1006/ST1019, мёртвый код в `clash_remote_ui.go`, SA4000/SA4023), заменены deprecated Fyne API (`SelectTab`→`Select`, `OnChanged`→`OnSelected`, `Wrapping = TextTruncate`→`Truncation`, `PlaceHolderColor`, `Window.Clipboard`→`App.Clipboard`).
- `docs/API.md` синхронизирован с кодом: примеры на v1.2.1/lx.4, убрана несуществующая кнопка «Copy snapshot», UI-ссылки Debug API указывают на Settings (а не Diagnostics), `/debug/snapshot` показывает `errors` как объект с пометкой про omitempty, rolling buffer профайлера описан как 60 секунд (`last=` клампится до 10 минут), исправлен путь wiring-файла. `docs/TRAFFIC_PROFILER.md`: `router.find_process` → `route.find_process`.
