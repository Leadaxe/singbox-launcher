# Upcoming release — черновик

Сюда складываем пункты, которые войдут в следующий релиз. Перед релизом переносим в `X-Y-Z.md` и очищаем этот файл.

**Не добавлять** сюда мелкие правки **только UI** (порядок виджетов, выравнивание, стиль кнопок без смены действия и т.п.). Писать **новое поведение**: данные, форматы, сохранение, заметные для пользователя возможности.

---

## EN

### Highlights

- Subscriptions: **Xray/V2Ray JSON array** body (`[ { full config }, … ]`) — one logical node per element; **`dialerProxy`** (or **`dialer`**) to a **SOCKS** or **VLESS** outbound → sing-box **`detour`** (jump outbound emitted first). Non-empty **`remarks`** → **`Label`** (full text) and tags **`{slug}`** / **`{slug}_jump_server`** for main vs jump (else `xray-{i}` / `xray-{i}_jump_server`); slug keeps letters/digits and **Unicode regional indicators** (UTF flag emoji), then usual prefix/unique rules. Example: `docs/examples/xray_subscription_array_sample.json`. Share URI: outbounds with **`detour`** are not encodable (**`ErrShareURINotSupported`**).
- **VLESS:** no longer auto-fills **`flow: xtls-rprx-vision`** when **`flow`** is missing in the URI or in Xray JSON — add **`flow`** in the subscription if the server requires Vision.

### Technical / Internal

- **Licensing:** the project is now under **GPL-3.0** with optional commercial licensing from Leadaxe (see `LICENSING.md`); previously MIT.

### Added — Mobile parity pass (2026-04-22)

- **Debug API (localhost-only):** optional HTTP server on `127.0.0.1:9269`. Off by default. Endpoints `/ping`, `/version`, `/state`, `/proxies`, `/action/{start,stop,update-subs,ping-all}`. Bearer-token auth (32-byte `crypto/rand`, constant-time compare), generated on first enable, surfaced via Copy-token button on the Diagnostics tab (with confirmation toast). Ported from LxBox spec 031, trimmed for desktop (no rules/subs CRUD, no `/config` dump, no `/logs` tail). Use-case — backing service for MCP wrappers so AI agents can check / drive the launcher. See `SPECS/038-F-C-DEBUG_API/`.
- **Settings tab:** new `⚙️ Settings` tab between Servers and Diagnostics. Consolidates language selector (moved from Help), subscription auto-update toggle, auto-ping-on-connect toggle. Also fixes a latent data-loss bug in the old language handler: `SaveSettings({Lang: code})` used to wipe every other settings field; now load-mutate-save. See `SPECS/039-F-C-SETTINGS_TAB_PREFERENCES/`.
- **Subscription auto-update global toggle** (default ON). Off skips scheduled refreshes; manual Update always works. Re-enabling from UI also resets the auto-disable-on-failure counter. Persisted as `subscription_auto_update_disabled` in `settings.json`.
- **Auto-ping after VPN connect** (default ON). 5 s after sing-box enters the running state, the Servers tab auto-runs Test on all proxies. Timer is cancelled if user stops sing-box inside the window. Persisted as `auto_ping_after_connect_disabled`.
- **Subscription source toggle:** per-row on/off checkbox in the Wizard → Sources list. Disabled sources stay in the file (URL / skip / tag prefix preserved) but are skipped by the parser entirely. `ProxySource.Disabled bool json:"disabled,omitempty"`; legacy configs stay compatible. See `SPECS/037-F-C-SUBSCRIPTION_SOURCE_TOGGLE/`.
- **URLTest parameters as template vars with preset dropdowns:** `auto-proxy-out.url / interval / tolerance` hoisted to `@urltest_url / @urltest_interval / @urltest_tolerance`. See `SPECS/040-F-C-WIZARD_TEMPLATE_OPTION_TITLES/` (note: widget rendering has a known gap and will be reworked before the next release — see the spec).
- **Template `vars[].options` accepts `{title, value}` form:** user-visible labels can differ from substitution values. Legacy string-list form still works (title == value). See `SPECS/040-F-C-WIZARD_TEMPLATE_OPTION_TITLES/`.
- **Keyboard shortcuts:** `Cmd/Ctrl+R` reconnect, `Cmd/Ctrl+U` update-subs, `Cmd/Ctrl+P` ping-all. Tooltip hints on the affected buttons are a TODO for the next pass. See `SPECS/042-F-C-KEYBOARD_SHORTCUTS/`.
- **Power-resume hooks (Windows + Linux):** on wake, the launcher resets the Clash API HTTP transport, then refreshes the proxies list and (if the tunnel is up) re-pings nodes so latency numbers aren't stuck pre-sleep. Linux gained native support via **systemd-logind `PrepareForSleep`** on system DBus (reuses the existing `github.com/godbus/dbus/v5` indirect dep, no new deps). macOS still stubbed — IOKit cgo hook is a tracked follow-up. See `SPECS/011-B-C-launcher-freeze-after-sleep/` (extension section).

### Added — Resilience & observability

- **Atomic writes** for `config.json` (scheduled parser + wizard save) and `settings.json` — stage to `.tmp` / `.swap` then rename. Crash / kill -9 / power-loss mid-write can no longer truncate the live file. See `SPECS/041-F-C-ATOMIC_FILE_WRITES/`.
- **100 MB download cap** on sing-box core downloads (pre-flight Content-Length + mid-stream cumulative) — contains a compromised or misconfigured mirror from filling the user's drive.
- **HTTP status humanization** in subscription-fetch errors: `401 → "unauthorized — subscription token may have expired"`, `429 → "rate limited — try again later"`, `5xx → "server error — provider is having issues"`, etc. Helps users triage without googling.
- **"(subs: Xh ago)"** hint next to the config file row on Core Dashboard — freshness of the last successful subscription update. Session-scoped.
- **Dirty-config marker (`*`)** on the Update button when wizard has saved changes not yet applied by the parser. See `SPECS/043-F-C-DIRTY_CONFIG_MARKER/` — current impl is the minimal version; redesign (separate sources-dirty vs runtime-dirty signals, long-term: state/config separation as in LxBox mobile) is tracked in the spec.

### Added — Security hygiene

- **Clash API token redaction** in debug logs — `token=ab***89` instead of full secret. Safe to paste logs into GitHub issues.

### Fixed

- **CI red since 2026-04-16:** `internal/locale.TestAllKeysPresent` missing 4 `ru.json` translations for the jump-outbound share-URI menu items.
- **STUN settings dialog** clipping on Windows (issue **#54**) — force ≥ 520 px width.

### Changed — Template defaults

- **Russian & Cyrillic TLDs expanded** in `ru-domains` rule-set: added `.рус / .москва / moscow / tatar / .дети / .сайт / .орг / .ком`.

### Not in this release (tracked as TODO — see `SPECS/` and per-night retrospective)

These were prototyped during the 2026-04-22 pass but rejected in review for redesign:

- **Right-click menu on Update** — `SecondaryTapWrap` doesn't route secondary taps into a `widget.Button`. Needs a custom `RightClickableButton` that implements `fyne.SecondaryTappable` directly.
- **Per-source parser success/failure summary** — counters land in `OutboundGenerationResult`, but the user-visible toast still says "Config updated successfully" regardless. Needs the breakdown piped into the toast.
- **Last-auto-update failure pill** on Core Dashboard — wired but not visible in practice (layout / flag-path diagnosis pending).
- **Ping-all button lock during test** — disable-button design blocks cancellation. Will rework as click-to-cancel with `pingAllInFlight` / `pingAllCancelled` atomics + button label swap to "Cancel" / "Cancelling…".
- **Wizard URLTest vars widget rendering** — `{title, value}` with editable SelectEntry is semantically broken (user can type arbitrary text into a labeled preset). Rule for next pass: `enum` → pure Select (titles allowed); `text` + options → SelectEntry (plain strings only); `text` + `{title, value}` → validator error.

---

## RU

### Основное

- Подписки: **JSON-массив** полных конфигов **Xray** (`[ {...}, … ]`) — по одной логической ноде на элемент; **`dialerProxy`**/**`dialer`** → hop **SOCKS** или **VLESS**, затем основной outbound с **`detour`**. **`remarks`**: полный текст в **`Label`** и в комментарии к outbound в JSON; теги: основной **`{slug}`**, jump **`{slug}_jump_server`** (или **`xray-{i}`** / **`xray-{i}_jump_server`** без `remarks`); в slug сохраняются буквы/цифры и **символы UTF-флагов** (региональные индикаторы). Пример: **`docs/examples/xray_subscription_array_sample.json`**. «Копировать ссылку» для таких нод с цепочкой — не поддерживается (**`detour`**).
- **VLESS:** больше **не подставляется** автоматически **`flow: xtls-rprx-vision`**, если в ссылке или в JSON Xray **`flow` не задан** — при необходимости Vision укажите **`flow`** в подписке.

### Техническое / Внутреннее

- **Лицензия:** репозиторий под **GPL-3.0**, при необходимости коммерческая лицензия — см. `LICENSING.md` (ранее MIT).

### Добавлено — паритет с мобильным клиентом (2026-04-22)

- **Debug API (локальный HTTP):** опциональный сервер на `127.0.0.1:9269`. По умолчанию выключен. Эндпоинты `/ping`, `/version`, `/state`, `/proxies`, `/action/{start,stop,update-subs,ping-all}`. Bearer-токен (32 байта `crypto/rand`, constant-time compare), генерируется при первом включении, копируется кнопкой с подтверждением-тостом на вкладке Диагностика. Портировано из LxBox spec 031 в урезанном виде (без CRUD правил/подписок, без `/config`, без `/logs`). Основной use-case — backing-service для MCP-обёрток, чтобы AI-агенты могли читать/дёргать лаунчер. Детали — `SPECS/038-F-C-DEBUG_API/`.
- **Вкладка Settings:** новая `⚙️ Settings` между Servers и Diagnostics. Собраны launcher-wide preferences: язык (перенесён из Help), глобальный выключатель автообновления подписок, автопинг-после-подключения. Попутно починен data-loss баг старого language-handler'а: `SaveSettings({Lang: code})` затирал ВСЕ остальные поля; теперь — load-mutate-save. См. `SPECS/039-F-C-SETTINGS_TAB_PREFERENCES/`.
- **Глобальный выключатель автообновления подписок** (по умолчанию ВКЛ). OFF — плановые обновления пропускаются; ручной Update всегда работает. Повторное включение сбрасывает счётчик последовательных ошибок. Хранится как `subscription_auto_update_disabled` в `settings.json`.
- **Автопинг после подключения VPN** (по умолчанию ВКЛ). Через 5 с после перехода sing-box в running вкладка Servers авто-запускает Test. Таймер отменяется, если пользователь нажал Stop внутри окна. Хранится как `auto_ping_after_connect_disabled`.
- **Переключатель подписки:** чекбокс слева в каждой строке Sources-таба визарда. Отключённые источники остаются в файле (URL / skip / tag prefix), но полностью пропускаются парсером. `ProxySource.Disabled bool json:"disabled,omitempty"`; legacy-конфиги совместимы. См. `SPECS/037-F-C-SUBSCRIPTION_SOURCE_TOGGLE/`.
- **Параметры URLTest как шаблонные vars с preset-дропдаунами:** `auto-proxy-out.url / interval / tolerance` вынесены в `@urltest_url / @urltest_interval / @urltest_tolerance`. См. `SPECS/040-F-C-WIZARD_TEMPLATE_OPTION_TITLES/` (известный пробел в рендере виджета — правка перед релизом).
- **`vars[].options` поддерживает форму `{title, value}`:** подписи для дропдаунов отдельно от подставляемых значений. Старая форма (массив строк) работает как раньше (title == value). См. `SPECS/040-F-C-WIZARD_TEMPLATE_OPTION_TITLES/`.
- **Горячие клавиши:** `Cmd/Ctrl+R` — reconnect, `Cmd/Ctrl+U` — update-subs, `Cmd/Ctrl+P` — ping-all. Tooltip-подсказки на кнопках — TODO на следующий pass. См. `SPECS/042-F-C-KEYBOARD_SHORTCUTS/`.
- **Обработка wake-from-sleep (Windows + Linux):** на резюм лаунчер сбрасывает HTTP-соединения Clash API, затем перечитывает список прокси и (если VPN работает) re-пингует ноды — задержки больше не застывают до сна. Linux получил нативную поддержку через **systemd-logind `PrepareForSleep`** на system DBus (используется уже существующий `github.com/godbus/dbus/v5` indirect-dep, новых зависимостей нет). macOS пока stub — IOKit cgo hook в отслеживаемом TODO. См. `SPECS/011-B-C-launcher-freeze-after-sleep/` (раздел Расширение).

### Добавлено — устойчивость и наблюдаемость

- **Атомарные записи** `config.json` (парсер + визард) и `settings.json` — сначала `.tmp` / `.swap`, потом `os.Rename`. Падение / kill -9 / обесточивание посреди записи больше не обнулит живой файл. См. `SPECS/041-F-C-ATOMIC_FILE_WRITES/`.
- **Лимит 100 МБ** на загрузку sing-box core (проверка Content-Length до скачивания + кумулятивный счётчик во время). Скомпрометированное или неправильно настроенное зеркало не может залить диск пользователя.
- **Расшифровка HTTP-статусов** в ошибках fetch'а подписок: `401 → "unauthorized — subscription token may have expired"`, `429 → "rate limited — try again later"`, `5xx → "server error — provider is having issues"` и т.д. Помогает триажить без Google.
- **Подсказка «(подписки: X ч назад)»** рядом с config.json на Core Dashboard — свежесть последнего успешного Update. В рамках сессии.
- **Маркер `*`** на кнопке Update, если визард сохранил изменения, а парсер ещё не прокатал. См. `SPECS/043-F-C-DIRTY_CONFIG_MARKER/` — текущая реализация минимальна; редизайн (отдельные sources-dirty vs runtime-dirty сигналы; в дальней перспективе — разделение state/config как в мобильном LxBox) — в спеке.

### Добавлено — безопасность

- **Редакция токена Clash API** в debug-логах — `token=ab***89` вместо полного секрета. Безопасно вставлять логи в GitHub issues.

### Исправлено

- **CI был красный с 2026-04-16:** `internal/locale.TestAllKeysPresent` — не хватало 4 переводов в `ru.json` для jump-outbound share-URI.
- **STUN settings dialog** обрезался на Windows (**#54**) — теперь принудительно ≥ 520 px.

### Изменено — шаблон

- **Список русских и кириллических TLD расширен** в `ru-domains` rule-set: добавлены `.рус / .москва / moscow / tatar / .дети / .сайт / .орг / .ком`.

### Не в этом релизе (отложено на редизайн — см. `SPECS/` и retrospective-отчёт)

Прототипы были написаны 2026-04-22, но отклонены в ревью и отложены:

- **Меню по правому клику на Update** — `SecondaryTapWrap` не маршрутизирует secondary-тапы в `widget.Button`. Нужен кастомный `RightClickableButton`, реализующий `fyne.SecondaryTappable` напрямую.
- **Сводка по источникам «N из M»** — счётчики доходят до `OutboundGenerationResult`, но user-visible тост по-прежнему пишет «Config updated successfully». Нужно пробросить разбивку в тост.
- **Плитка последней ошибки автообновления** на Core Dashboard — wired, но в UI не видна (диагностика layout / flag-path нужна).
- **Блокировка кнопки Ping-all во время теста** — полное Disable ломает возможность прервать текущий прогон. Будет переделано как click-to-cancel с `pingAllInFlight` / `pingAllCancelled` + swap лейбла на «Cancel» / «Cancelling…».
- **Рендеринг виджетов URLTest-vars в визарде** — `{title, value}` + editable SelectEntry семантически некорректен (пользователь может напечатать произвольный текст в поле с пресет-подписью). Правило для следующего pass'а: `enum` → чистый Select (titles допустимы); `text` + options → SelectEntry (только plain strings); `text` + `{title, value}` → ошибка валидатора.
