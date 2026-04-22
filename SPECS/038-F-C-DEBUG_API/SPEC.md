# SPEC: Debug API — локальный HTTP-сервер для introspection и control

Задача: предоставить **стабильную программную поверхность** для внешних инструментов (скрипты, CI, мониторинг, **MCP-обёртки для AI-агентов**), чтобы читать состояние лаунчера и управлять им без shell-хаков (`pkill sing-box`, curl Clash API напрямую, парсинг UI).

**Статус:** реализовано. Детали — **PLAN.md**, **IMPLEMENTATION_REPORT.md**. Коммиты: `220256c` (базовый сервер), `154a1e7` (constant-time auth), `ba15424` (`/version` + Copy toast), `264ffaf` (`/action/ping-all` + `Cmd/Ctrl+P`). Ретроспективная спека — все коммиты шипились ночью 2026-04-22 без предварительного SPEC.

Прообраз в мобильном: **LxBox spec 031**. На десктопе поверхность урезана по сравнению с мобильным (см. §5 «Не-цели»).

---

## 1. Проблема

### 1.1 До изменений

- У скриптов и автоматизации нет стабильного API к лаунчеру. Единственное доступное:
  - Clash API напрямую — нужно найти его host / token в `config.json`, парсить пути вручную.
  - Сигналы процессу (`kill -HUP` и т.п.) — не предусмотрены.
  - Парсинг UI — никак.
- Нет способа из внешнего процесса попросить лаунчер:
  - «запусти sing-box»
  - «перезагрузи подписки»
  - «пропингуй все ноды сейчас»
  - «скажи, в каком ты состоянии»
- Для **MCP-обёрток** (AI-агенты через Claude/Cursor/etc.) особенно важен стабильный HTTP + auth — это общепринятый backing-service-интерфейс для MCP-серверов.

### 1.2 Цель

Локальный HTTP-сервер на `127.0.0.1:<port>`, защищённый Bearer-токеном, с узким набором `GET` endpoints для чтения состояния и `POST` endpoints для безопасных действий. **Off by default**; включается пользователем явно через UI на вкладке Diagnostics. Токен генерируется при первом enable, показывается через кнопку Copy.

---

## 2. Требования

### 2.1 Безопасность

**Непреложные правила:**

1. **Listener строго на `127.0.0.1:<port>`.** Никакого `0.0.0.0`, никакого LAN. Удалённый доступ — только через `adb forward` / ssh-tunnel.
2. **Каждый protected endpoint требует `Authorization: Bearer <token>`.** Исключение — `/ping` (без auth — health-check допустим без токена).
3. **Сравнение токенов — constant-time** через `crypto/subtle.ConstantTimeCompare`. На loopback'е теоретическая защита, но бесплатная.
4. **Токен генерируется с `crypto/rand`**, 32 байта, `base64.RawURLEncoding`. Длина в base64 — 43 символа.
5. **Mutating endpoints — только `POST`.** GET на `/action/*` → 405 Method Not Allowed. Защита от drive-by триггеров через открытые вкладки / ложные `<img>` теги.
6. **Токен НЕ попадает в debug-логи.** Уже есть `urlredact.RedactToken` (spec: commit `3889852`), и в самом debug-API логировании токен не используется.
7. **Off by default.** Новый пользователь не получит открытого порта без явного действия.

### 2.2 Конфигурация

- Новые поля в `bin/settings.json`:
  - `debug_api_enabled bool omitempty` — пользовательский выключатель.
  - `debug_api_token string omitempty` — Bearer-токен. Генерируется при первом `debug_api_enabled = true`, сохраняется между off/on (чтобы скрипты не ломались при rotate). Ротация — только через ручное удаление поля в `settings.json`.
  - `debug_api_port int omitempty` — порт. `0` / отсутствует → `DefaultPort = 9269` (совпадает с LxBox мобильным).

### 2.3 Endpoints (v1)

| Method | Path                      | Auth | Описание                                                                              |
|--------|---------------------------|------|---------------------------------------------------------------------------------------|
| GET    | `/ping`                   | —    | `{"ok": true}`. Health-check.                                                         |
| GET    | `/version`                | ✓    | `{"launcher", "singbox", "api": "debugapi/v1"}`.                                      |
| GET    | `/state`                  | ✓    | `{running, active_proxy, selected_group, singbox_version, subs_last_updated_unix}`.   |
| GET    | `/proxies`                | ✓    | Текущий список прокси в том же формате что `api.ProxyInfo`.                           |
| POST   | `/action/start`           | ✓    | `StartSingBoxProcess()`. Ответ `{"ok": true}` или 500 `{"error": "..."}`.             |
| POST   | `/action/stop`            | ✓    | `StopSingBoxProcess()`.                                                               |
| POST   | `/action/update-subs`     | ✓    | `ConfigService.UpdateConfigFromSubscriptions()` **синхронно** (ответ после прогона).  |
| POST   | `/action/ping-all`        | ✓    | Триггерит `UIService.AutoPingAfterConnectFunc` (та же функция что Servers «test»).    |

**Расширения в будущем** (см. §6 «Открытые вопросы»):
- `POST /action/switch-proxy {group, name}`
- `GET /logs?tail=N` (когда появится in-memory log ring)
- `GET /config?redacted=true` (когда будет надёжное редактирование секретов)

### 2.4 Архитектура

- Пакет `core/debugapi/`:
  - `server.go` — `Server` struct с `net.Listener`, `http.Server`, middleware pipeline.
  - `server_test.go` — unit-тесты на auth / method-gate / token entropy.
- Пакет `core/` (не `core/debugapi/`, чтобы не порождать cycle):
  - `debugapi_wiring.go` — `debugAPIFacade` реализует `debugapi.ControllerFacade` (узкий интерфейс), адаптируя singleton `AppController`. Методы `StartDebugAPI / StopDebugAPI / DebugAPIAddr` на `*AppController`.
- `ControllerFacade` в `core/debugapi/server.go` — **узкий интерфейс**, не зависящий от типа `AppController` (чтобы пакет тестировался изолированно):
  ```go
  type ControllerFacade interface {
      IsRunning() bool
      GetProxiesList() []api.ProxyInfo
      GetActiveProxyName() string
      GetSelectedClashGroup() string
      GetSingboxVersion() string
      GetConfigPath() string
      GetLauncherVersion() string
      GetLastUpdateSucceededAt() time.Time

      StartSingBox() error
      StopSingBox() error
      UpdateSubscriptions() error
      PingAllProxies() error
  }
  ```
- Middleware: простая цепочка `authMiddleware → handler`. Открытые (без auth) эндпоинты регистрируются отдельным `*http.ServeMux` до обёртки.
- `server.Stop()` — graceful с 5s deadline.
- `GenerateToken()` — helper генерации токена (экспортируется для UI).

### 2.5 UI — Diagnostics tab

- Секция «Debug API (localhost)» внизу вкладки Diagnostics, после списка IP-checker'ов.
- **Заголовок:** `locale.T("diag.debug_api_title")` — жирный.
- **Хинт:** `locale.T("diag.debug_api_hint")` — multi-line, `TextWrapWord` (иначе пинает ширину окна).
- **Строка действий:** `[Enable ☐] [Copy token 📋]` в HBox.
- **Статус-строка:** «Status: Off» / «Status: Listening on 127.0.0.1:9269» — обновляется на toggle.

Toggle:
1. OFF → ON: если `DebugAPIToken == ""`, генерируется через `debugapi.GenerateToken()` и сохраняется. Затем `StartDebugAPI(port, token)`. При ошибке — чекбокс откатывается в OFF, сохраняется OFF в settings.
2. ON → OFF: `StopDebugAPI()`. **Токен сохраняется** в `settings.json` — чтобы при повторном включении скрипты не ломались. Ротация требует ручного удаления поля.

Copy-token:
- Читает актуальный токен из `settings.json` (каждый клик — свежий read; пользователь может в ручную изменить между запусками).
- Копирует в clipboard.
- Показывает `dialogs.ShowAutoHideInfo` toast — «Token copied. Pass as: Authorization: Bearer …».

### 2.6 Локализация

Новые ключи в `en.json` + `ru.json`:

- `diag.debug_api_title`
- `diag.debug_api_hint`
- `diag.debug_api_enable`
- `diag.debug_api_copy_token`
- `diag.debug_api_off`
- `diag.debug_api_on` (формат с `%s` — адрес listener'а)
- `diag.debug_api_copied_title`
- `diag.debug_api_copied_msg`

### 2.7 Горячая клавиша

`Cmd/Ctrl+P` на главном окне — триггер `/action/ping-all` локально (без HTTP). Биндинг через `desktop.CustomShortcut{KeyName: fyne.KeyP, Modifier: fyne.KeyModifierShortcutDefault}` в `app.registerShortcuts()`.

---

## 3. Инварианты

1. **Listener не может быть запущен с auth-token == "".** `New()` возвращает error.
2. **Пересоздание сервера при повторном `StartDebugAPI()`** — `Stop()` предыдущего + `New()` нового. Никогда не два listener'а параллельно.
3. **`DebugAPIAddr()` возвращает реальный адрес** после `Start()`; `""` если `Stop()` был вызван.
4. **Порт задаётся только через settings.** Runtime-change — требует toggle OFF → change setting → toggle ON.

---

## 4. Совместимость

- Старые `settings.json` без `debug_api_*` полей работают как раньше — debug API просто off.
- Бэкворд-совместимость v1: пока «debugapi/v1». Изменение structure поля ответов — breaking change, версия API должна быть повышена (v2) с сохранением v1 или deprecation policy.

---

## 5. Не-цели

В отличие от LxBox мобильного (spec 031, полный CRUD), десктопная версия **намеренно урезана**:

- **Нет CRUD для rules / subs / settings.** Wizard на десктопе это уже полностью покрывает; дублировать через HTTP — лишняя поверхность.
- **Нет `GET /config`.** config.json содержит секреты (Clash secret, UUID VLESS, Reality private key). Редактирование секретов — отдельная задача; пока безопаснее не выдавать.
- **Нет `GET /logs?tail=N`.** Нет in-memory log ring на десктопе — debug-лог пишется в файл, без буфера. Добавим endpoint, когда появится ring-buffer в `internal/debuglog/`.
- **Нет streaming endpoints.** Никаких SSE / WebSocket / chunked push — HTTP request/response всегда завершается за разумное время.

---

## 6. Открытые вопросы / Будущие расширения

### 6.1 `POST /action/switch-proxy {group, name}`

Нужен для MCP-обёртки («переключи на быстрейшую JP-ноду»). Реализация: через существующий `api.SwitchProxy(baseURL, token, group, proxy)`. Добавить в `ControllerFacade` и в `handlers`.

### 6.2 `GET /logs?tail=N`

Зависит от добавления in-memory log ring в `internal/debuglog/`. **Отдельная спека** (SPEC 039+), не в рамках этой.

### 6.3 `GET /groups`

Список selector-групп с активным прокси в каждой. Сейчас `/state` возвращает только один `selected_group`. Полезно для MCP («покажи все группы и что в них активно»).

### 6.4 Rate limiting

Сейчас нет — loopback, auth required. Но при `/action/update-subs` спам может задолбать провайдера подписок. Возможно стоит добавить дебаунс на стороне сервера.

### 6.5 MCP-сервер (отдельный репо)

См. `docs/night-reports/2026-04-22.md` раздел про Debug API — план по MCP-обёртке: `vpn_status`, `vpn_start`, `vpn_stop`, `update_subscriptions`, `ping_all`, `list_proxies`, плюс `switch_proxy` / `list_groups` / `get_logs` когда соответствующие endpoints появятся.

---

## 7. Влияние на другие компоненты

- **Settings.go**: +3 поля (см. §2.2).
- **`core/uiservice/`**: не трогается.
- **`main.go`**: при старте — если `DebugAPIEnabled && DebugAPIToken != ""` → `controller.StartDebugAPI(port, token)`. При выходе сервер останавливается вместе с процессом (нет отдельного defer).
- **`ui/diagnostics_tab.go`**: новая секция (см. §2.5). Помимо неё, существующие STUN / open-log / IP-services не затрагиваются.
- **`ui/app.go`**: +1 горячая клавиша (Cmd/Ctrl+P).
- **`.gitignore`**: не меняется (settings.json и так ignored? — нет, settings.json коммитить мы НЕ должны; проверить что в gitignore он есть).

---

## 8. Тестирование

### 8.1 Unit (есть в `server_test.go`)

- `TestServerAuthAndState`: fake-facade, реальный `net.Listen` на свободный порт, 4 кейса:
  - `/ping` без auth → 200.
  - `/state` без auth → 401.
  - `/state` с wrong token → 401.
  - `/state` с right token → 200, body содержит `active_proxy`.
  - GET на `/action/start` → 405.
- `TestGenerateTokenUnique`: два вызова `GenerateToken()` дают разные значения + длина ≥ 32.

### 8.2 Интеграционный (TODO)

- Smoke-тест на запуск / остановку через `controller.StartDebugAPI / StopDebugAPI`.
- Тест на rotate токена (stop → change settings → start).
- Тест на двойной start (должен корректно перезапустить).

### 8.3 Ручной

- Включить в UI → `curl http://127.0.0.1:9269/ping` → `{"ok":true}`.
- `curl -H "Authorization: Bearer <token>" http://127.0.0.1:9269/state` → JSON со state'ом.
- `curl -X POST -H "Authorization: Bearer <token>" http://127.0.0.1:9269/action/ping-all` → `{"ok":true}`, в UI отрабатывает ping-all.
