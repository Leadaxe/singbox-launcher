# IMPLEMENTATION_REPORT 161

**Дата:** 10.10.2026. **Статус:** реализовано в ветке `spec-161-service-window`, ждёт
показа владельцу и приёмки (папка остаётся `-F-N-`). Полный прогон тестов — CI
после слияния в `develop`.

## Что сделано

### Волна A — core (1ddaa0e4)
- `core/core_build.go`: разбор и сравнение версий ядра без build-тегов
  (`CompareCoreVersion`, `CoreVersionPairLabels`, перенос `parseCoreBuild` и `sha256File`).
- `core/service_recipes.go`: рецепты службы по procd / systemd / launchd / SCM
  (`BuildServiceRecipes`), пути паспорта поверх дефолтов (`MergeServicePaths`),
  ssh-обёртка (`WrapSSH`, `PosixQuote`), классификатор ошибки связи
  (`ClassifyDaemonReachError`, на нём `diagnoseReachError`).
- `core/core_download_target.go`: ядро под платформу машины в `~/Downloads` со сверкой
  `SHA256SUMS`; `SingboxAssetSuffixFor` с mips/mipsle softfloat.
- Запись машины: `ssh`, `init_system`, `core_warn_ack`, `passport`; паспорт в `RemoteHealth`.
- Локальный демон: `ReachErr` и кэш паспорта в `DaemonUIStatus`, restart службы
  (macOS kickstart, Windows `Restart-Service`), `DaemonServicePaths`.
- Тест `TestServiceRecipes`.

### Волна B — окно Service и удалённые машины (df50a782, 498d1a87)
- Модель (`ui/service_model.go`), окно и шапка (`ui/service_window.go`), вкладки
  (`ui/service_tabs.go`), строка шага (`ui/service_step_row.go`), гайды, источник
  «машина», скачивание ядра, окно живого лога машины.
- Строка машины: ⚙ с точкой и строка предупреждения, диалог Deploy на старое ядро;
  heartbeat: `FailSince`, `healthChanged`; поле SSH в окне Edit.

### Волна C — Debug API, документация, Local, переводы (629e6f36, 2ab70446, коммит C2/C3/C7)
- C1: поля окна Service в `/daemon/status`, `/daemon/commands`, `/remote/machines/*`.
- C4–C6: TROUBLESHOOTING «Daemon/Демон», DAEMON_AND_REMOTE, API, ARCHITECTURE,
  релизные заметки.
- C2: `ui/service_source_local.go` — источник «локальный демон»: опрос
  `DaemonStatusSnapshot` раз в 5 с (без наложения опросов), серия отказов, пути,
  плашка службы в шапке, строки операций `daemonOps`.
- C3: панель Local = подсказка + «Stop VPN when quitting» + окно Service
  (`buildServiceView`), опрос останавливается при закрытии окна подключения; слой
  тултипов в окне подключения.
- C7: `bin/locale/ru.json` — +152 перевода, −26 orphan; все проверки чистые.

## Что куда переехало в Local

| Было | Стало |
|---|---|
| Status: строки статуса (`renderDaemonStatusText`) | шапка (версия ядра, отвечает ли демон, uptime, серия отказов); функция удалена |
| плашка службы SPEC 136 §6 | строка в шапке (красная/жёлтая) |
| «служба не установлена», «движок ещё не активен», «ядро без lxd» | строки в шапке |
| Install: install + подсказка обновить ядро | Core |
| bootstrap (NotRunning) | Not running, «Load the service» |
| — | Not running: status, **restart**, log, last-good |
| приглашение + Pair, «Need a fresh invite» | Pairing («Mint an invite», «Paste what it printed») |
| адрес демона, Bearer-секрет | Pairing (адрес — отдельной строкой, секрет — секция Plain mode) |
| ↻ Refresh daemon status | удалён: вид опрашивает демон сам |
| Uninstall | вкладка Uninstall без изменений |
| радио движка, Stop VPN when quitting | на месте, над окном Service |

## Решения, принятые по ходу

1. `buildDaemonPanel` и `buildLocalEngineTab` возвращают `(obj, dispose)` — иначе опрос
   демона не остановить при закрытии окна (у Fyne нет цепочки `SetOnClosed`).
2. Окно подключения больше не оборачивает всё в одну прокрутку: панель Local
   встаёт в `Border`, вкладки Service прокручиваются сами. Во вложенной прокрутке
   вкладки сжались бы до минимума 160 px. Ширина окна = ширина окна Service (640).
3. Нет службы (`NotInstalled`) → `NeedsInstall` → окно открывается на Core с ⚠ (как
   прежний автовыбор Install), а не на Pairing.
4. Plain-демон (`tls:false`, без пина) считается сопряжённым, когда отвечает.
5. Строка Install на Core — только когда ядро лаунчера умеет root-owned копию;
   иначе подсказка обновить ядро (как прежняя `installCoreHint`). Правка одной
   строки в `buildLocalCore` (`ui/service_tabs.go`).
6. Подписи строк операций — «где и как исполняется» под заголовком шага окна
   (`daemonOpRowLabel` и др.); нумерованные подписи «1.»/«2.» удалены.
7. У полей адреса и секрета добавлена кнопка Save (Enter работает как раньше).
8. Справка «?» у секрета (команда показа секрета) осталась рядом с полем; в
   Reference — шаг «Show it» по `daemon.json`.
9. `localServiceSource.Pair` игнорирует адрес: его несёт приглашение
   (локально вкладка Pairing всё равно берёт готовую строку `LocalRows().Pair`).

## Проверки

- `go build ./...` (macOS) — ок; `go run ./tools/win7guard` — чисто;
  `go run ./tools/l10n/l10n_check --strict` — 0 missing/orphan;
  `go run ./tools/l10n/hardcoded_check --strict` — 0.
- Типовая проверка `ui` под windows/amd64, windows/arm64, windows/386 и linux/amd64
  (go/types с FakeImportC — кросс-сборка пакета невозможна из-за CGO) — 0 ошибок.
- Тест волны — только `TestServiceRecipes` (волна A). UI тестами не покрыт.
- CI (`ci.yml`, `run_mode=tests`) — после слияния ветки в `develop`.

## Проверить владельцу вживую

1. Servers → ⚙ → Local, Daemon: подсказка, «Stop VPN when quitting», шапка
   (macOS · darwin/arm64 · служба · launchd, ядро, «✅ Отвечает · работает …»),
   пять вкладок; вкладки прокручиваются, окно не распирает по высоте и ширине.
2. Not running: status, restart (kickstart в Terminal), log, last-good; после
   restart шапка зеленеет без кликов (≤5 с).
3. Core: версия ядра лаунчера, Install or update the service (Terminal / UAC),
   переход «→ Uninstall tab».
4. Pairing: Mint an invite, вставка приглашения + Pair (+ «?»), адрес демона
   (Enter / Save), Plain mode — секрет (+ «?»), Who is trusted, Revoke.
5. Reference: пути (паспорт или `default`), ключи `daemon.json`, Live log window
   (окно Logs), Follow the log, Who holds the port.
6. Uninstall: Unpair, `--purge`, удаление службы; на Windows — Keep the core copy /
   Remove all.
7. Без службы: окно открывается на ⚠ Core, в шапке «служба не установлена»; не
   сопряжено — ✖ на Pairing; служба лежит — ✖ Not running и «не отвечает уже …».
8. Радио Process ↔ Daemon переключается как раньше; при Process панель скрыта.
9. Удалённо — экраны волны B (TASKS B8): строка роутера (⚙•, строка ⚠/✖), окно
   Service на всех вкладках, скачивание под linux/arm64, ▶ с `ssh root@…`, диалог Deploy.
10. Русская локаль: тексты окна Service и строк машины.
