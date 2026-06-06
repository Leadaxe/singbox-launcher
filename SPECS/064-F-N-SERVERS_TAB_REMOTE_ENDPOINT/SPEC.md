# SPEC 064-F-N — SERVERS_TAB_REMOTE_ENDPOINT

**Status:** New (N)
**Type:** Feature (F) — превратить вкладку **Servers** (Clash API client) из view-только-локального-sing-box в полноценный generic Clash-API client с возможностью подключиться к **произвольному** endpoint'у (другой инстанс лаунчера на той же машине, sing-box на VPS через ssh-tunnel, mihomo на роутере типа RouteRich на `192.168.10.1`).
**Depends on:** ничего (UI-only feature на верх существующей `api.ClashAPIClient` инфраструктуры).
**Не меняет:** Configurator, Wizard, Traffic Profiler, Logs view, Tray menu — они остаются строго локальными. См. §6 «Out of scope».

---

## Проблема

Вкладка **Servers** сейчас является клиентом Clash API одного-единственного источника: локального sing-box, запущенного самим лаунчером. URL и secret берутся из `bin/config.json::experimental.clash_api`. Если sing-box локально **не запущен**, вкладка **disabled** (`ui/app.go::updateClashAPITabState` зовёт `tabs.DisableItem`), и juzер не может посмотреть прокси-листы, переключить группу селектора, пинговать ноды.

Это **архитектурное ограничение, не техническое**: Clash API — это стандартный HTTP-протокол ([mihomo doc](https://wiki.metacubex.one/api/), [clash doc](https://github.com/Dreamacro/clash/wiki/external-controller)), который реализуют **mihomo, clash, clash-meta, sing-box** и совместимые форки. На стороне юзера типовой scenario:

- На **роутере** (OpenWrt / GL-iNet / RouteRich) крутится sing-box или mihomo с открытым Clash API на LAN-интерфейсе (`192.168.x.x:9090`). Юзер хочет с десктопа выбрать proxy-группу, не лезя в роутерский web-UI.
- На **VPS** крутится sing-box, ssh-tunnel пробрасывает Clash API локально (`127.0.0.1:9091`). Юзер хочет посмотреть состояние с десктопа.
- На той же машине у юзера запущены **два инстанса** лаунчера (dev/stable, или test-stage) на разных портах. Хочет переключиться между ними одной кнопкой.
- Юзер просто использует mihomo / clash-meta **без** лаунчера, а наш лаунчер — как «удобный Servers-UI».

Локальный launcher как требование для просмотра Servers — это **необоснованная привязка**. Сам tab уже HTTP-клиент; точка взятия `baseURL / token` — единственное препятствие.

---

## Целевая модель

### UI

В шапке вкладки Servers — **status badge** + **gear ⚙ button**:

```
[ApiStatusLabel "Clash API online"]     [🏠 Local] [⚙]
[Selector group: <dropdown>] [⇄ map]
─────────────────────────────────────────────────────
[buttons row: sorts, ping all, ping-settings ⚙]
```

- **Badge** показывает текущий источник:
  - `🏠 Local` — берём конфиг из локального `config.json`. Дефолт. Префикс 🏠 = «домашний» = launcher-owned.
  - `🌐 host:port` — активен remote override. Префикс 🌐 = «глобальный/сетевой» = внешний endpoint.
- **⚙ Gear** открывает модальное окно `Remote endpoint` (см. ниже). Существующий ⚙ для ping-settings остаётся в нижней `buttonsRow` — другой concern, не путать.

### Gear dialog

```
┌──────────────────────────────────────────┐
│ Remote endpoint                          │
├──────────────────────────────────────────┤
│ Host:    [192.168.10.1____________]      │
│ Port:    [9090__________________]        │
│ Secret:  [••••••••••••••••••••]    [👁]  │
│                                          │
│ ⓘ HTTP only. Loopback / LAN only —       │
│   нет TLS, нет CORS.                     │
├──────────────────────────────────────────┤
│ [Reset to local config]  [Use Local]     │
│                            [Cancel] [Connect] │
└──────────────────────────────────────────┘
```

- **Host** — hostname или IP. Нормализация в `NormalizeHost`: strip `http://`/`https://` префикса, trim whitespace и trailing `/`, reject `/` и `:` в результате (юзер должен использовать поля Host и Port отдельно). IPv6 brackets в MVP **не** поддерживаются — reject с inline-error.
- **Port** — `1..65535`, парсится как int.
- **Secret** — empty allowed (Clash API можно без secret'а). Поле в password-mode по дефолту, кнопка-eye для reveal.
- **Reset to local config** — заполняет три поля из `LoadClashAPIConfig(config.json)`. Если local config отсутствует (cold start) — кнопка disabled, tooltip «No local config available».
- **Use Local** — `ClearRemoteOverride()` + закрытие окна. Badge возвращается на `🏠 Local`.
- **Cancel** — закрытие без изменений.
- **Connect** — валидация → 3-секундный reachability probe `GET /version` → на failure модал «Endpoint unreachable. Connect anyway? [Yes/No]» → на Yes (или success) `SetRemoteOverride(...)` + closure + tab refresh.

### Data flow

```
RemoteOverride (RAM, sync.RWMutex)
       │
       ▼
EffectiveClashAPIConfig(ac) returns:
   - if override active → (http://<host>:<port>, secret, enabled=true, remote=true)
   - else               → ac.APIService.GetClashAPIConfig() (existing path)
       │
       ▼
all UI callsites in clash_api_tab.go
```

**Storage scope:** RAM only. После рестарта лаунчера override сбрасывается, badge возвращается на `🏠 Local`. Никакой persistence в `bin/settings.json` (см. §6 «Out of scope»).

### Tab availability

Сейчас (`ui/app.go::updateClashAPITabState`):

```go
if !isRunning { DisableItem } else { EnableItem }
```

После SPEC 064:

```go
if !isRunning && !hasActiveRemoteOverride { DisableItem } else { EnableItem }
```

Tab **enabled** если **либо** локальный sing-box запущен, **либо** активен remote override. Если юзер на Servers tab и стопит локальный sing-box, и при этом нет override — tab disable'ится и фокус уходит на Core (как сейчас). Если override active — tab остаётся открытым, продолжает рендерить remote данные.

### Сохранение поведения при старте/стопе local sing-box

- **Local start / stop** при активном override → **остаёмся на remote**. Юзер сделал явный выбор; auto-switch ломал бы его намерение. (Альтернатива auto-switch обсуждалась и отклонена.)
- Override очищается **только** через UI (`Use Local` button или manual close через закрытие лаунчера).

---

## Что меняется по компонентам

### `ui/clash_remote.go` (NEW, ~110 LOC)

Singleton override-storage + resolver + helpers.

```go
package ui

import (
    "fmt"
    "net/url"
    "strings"
    "sync"
    "sync/atomic"

    "singbox-launcher/core"
)

// RemoteOverride — ephemeral remote Clash-API endpoint, RAM-only.
// SPEC 064.
type RemoteOverride struct {
    Host   string
    Port   int
    Secret string
}

var (
    remoteOverrideMu     sync.RWMutex
    remoteOverrideActive bool
    remoteOverrideValue  RemoteOverride

    // clashConfigGeneration — bump on каждом изменении override'а
    // (set/clear). Используется для drop-stale в timer-based refresh
    // loop'ах вкладки. См. §«Concurrency» в SPEC.
    clashConfigGeneration uint64

    // overrideChangedListeners — callbacks для UI rebind'а (status
    // badge update, tab enable, force-refresh).
    overrideChangedListeners []func()
)

// GetRemoteOverride — snapshot текущего override'а. ok=false если override не active.
func GetRemoteOverride() (RemoteOverride, bool) { /* … */ }

// SetRemoteOverride — atomic activate + bump generation + notify.
func SetRemoteOverride(ov RemoteOverride) { /* … */ }

// ClearRemoteOverride — atomic deactivate + bump generation + notify.
func ClearRemoteOverride() { /* … */ }

// OnOverrideChanged — register callback. Goroutine-safe.
func OnOverrideChanged(cb func()) { /* … */ }

// EffectiveClashAPIConfig — single resolver consulted by every callsite.
// Returns (baseURL, token, enabled, remote).
//   - enabled=true когда либо local config валиден, либо override active.
//   - remote=true только когда override active.
func EffectiveClashAPIConfig(ac *core.AppController) (baseURL, token string, enabled, remote bool) {
    if ov, ok := GetRemoteOverride(); ok {
        return fmt.Sprintf("http://%s:%d", ov.Host, ov.Port), ov.Secret, true, true
    }
    base, tok, en := ac.APIService.GetClashAPIConfig()
    return base, tok, en, false
}

// CurrentGeneration — atomic load для timer-loop drop-stale checks.
func CurrentGeneration() uint64 { return atomic.LoadUint64(&clashConfigGeneration) }

// NormalizeHost — trim http(s):// prefix, trailing /, whitespace.
// Reject if contains '/' or ':' in result (юзер должен использовать
// отдельные поля Host и Port). IPv6 literals reject'ятся в MVP.
func NormalizeHost(raw string) (string, error) { /* … */ }
```

### `ui/clash_api_tab.go` (EDIT, ~+200 / -25 LOC)

1. **Replace 9 callsites:** `ac.APIService.GetClashAPIConfig()` → `EffectiveClashAPIConfig(ac)`.
2. **Add status badge** в новой top-row рядом с `ApiStatusLabel`. Содержимое биндится через `OnOverrideChanged`.
3. **Add gear button** ⚙ в той же top-row, справа от badge. `widget.LowImportance` чтобы соответствовать `mapButton`'у. Tooltip: «Remote endpoint settings».
4. **`showRemoteEndpointDialog(ac, parentWin)`** — модальное окно с тремя entry, reveal-toggle для secret'а, четырьмя кнопками. Включает inline-validation, reachability probe и confirmation modal на unreachable.
5. **`probeRemote(host, port, secret) error`** — 3s context-timeout запрос на `http://host:port/version`. Использует `api.TestAPIConnection`-style helper, но с per-call timeout (3s вместо глобального 20s).
6. **Generation guard** во всех refresh goroutine'ах (`onLoadAndRefreshProxies`, `pingAllProxies`, `pingProxy`, `mapButton`, switch-proxy callback): capture `gen := CurrentGeneration()` на старте, в `fyne.Do` callback'е check `if gen != CurrentGeneration() return` чтобы не писать stale данные после Connect.

### `ui/app.go::updateClashAPITabState` (EDIT, ~+15 LOC)

```go
hasOverride := ui.GetRemoteOverride() != (RemoteOverride{}) /* or use _, ok := … */
isActive := isRunning || hasOverride
if !isActive { tabs.DisableItem(clashAPITab) } else { tabs.EnableItem(clashAPITab) }
```

Плюс подписка на `OnOverrideChanged` в `NewApp`, чтобы tab enable/disable реагировал на Connect/Use-Local немедленно.

### `internal/locale/*.json` (EDIT, ~15 keys)

```
servers.endpoint.dialog.title           "Remote endpoint" / "Удалённый endpoint"
servers.endpoint.host                   "Host" / "Хост"
servers.endpoint.port                   "Port" / "Порт"
servers.endpoint.secret                 "Secret" / "Secret"
servers.endpoint.reset_to_local         "Reset to local config" / "Загрузить из локального конфига"
servers.endpoint.use_local              "Use Local" / "Использовать Local"
servers.endpoint.connect                "Connect" / "Подключиться"
servers.endpoint.cancel                 "Cancel" / "Отмена"
servers.endpoint.invalid_port           "Port must be 1..65535"
servers.endpoint.empty_host             "Host is required"
servers.endpoint.host_has_slash         "Host must not contain '/' — use Port field"
servers.endpoint.no_local_config        "No local config available yet"
servers.endpoint.badge_local            "🏠 Local"
servers.endpoint.badge_remote_format    "🌐 %s:%d"
servers.endpoint.tooltip_settings       "Remote endpoint settings"
servers.endpoint.unreachable_title      "Endpoint unreachable"
servers.endpoint.unreachable_body       "GET /version timed out / failed. Connect anyway?"
servers.endpoint.note_http_only         "ⓘ HTTP only. Loopback / LAN only — no TLS, no CORS."
```

Для **EN + RU** — обязательно. Остальные локали (де/es/fr/it/ja/ko/pt-BR/tr/zh) — могут получить EN fallback (точно как другие новые ключи; runtime fallback на en.json уже работает).

### Тесты

- `ui/clash_remote_test.go` — unit tests:
  - `TestEffectiveClashAPIConfig_LocalDefault` — нет override → return из local config
  - `TestEffectiveClashAPIConfig_RemoteOverride` — set → return remote
  - `TestNormalizeHost` — `http://x` → `x`, `host:8080` → reject, `/path` → reject, whitespace trim, IPv6 brackets reject
  - `TestGenerationBumpOnSetClear` — gen counter монотонно растёт
  - `TestOverrideChangedFiresCallback` — listener вызывается на Set/Clear
- Manual test matrix (см. §«Acceptance»):
  - local-only flow (override never active) — поведение идентично до SPEC 064
  - remote-only flow (local sing-box не запущен, override active)
  - override-while-running (start local, потом set override → tab показывает remote)
  - override-then-stop-local (active override, stop local → tab остаётся на remote, не disable)
  - unreachable remote (Connect на не-listening port → 3s timeout + confirm modal)

---

## Concurrency

Главная race-точка: timer-based refresh goroutine'ы вкладки (`onLoadAndRefreshProxies`, `pingAllProxies`, etc.). Юзер кликает Connect → новый baseURL/token, **но in-flight goroutine** уже capture'нула старые значения и пишет результат в UI model. Без защиты — UI показывает stale данные от прежнего endpoint'а поверх свежего refresh'а.

**Решение — generation counter** (тот же паттерн что `pingAllGeneration` в текущем коде):

```go
// Package-level в clash_remote.go
var clashConfigGeneration uint64

// SetRemoteOverride / ClearRemoteOverride bump'ают:
atomic.AddUint64(&clashConfigGeneration, 1)
```

Каждая refresh-goroutine:
```go
go func() {
    gen := ui.CurrentGeneration()
    baseURL, token, _, _ := ui.EffectiveClashAPIConfig(ac)
    result, err := fetch(baseURL, token)
    fyne.Do(func() {
        if gen != ui.CurrentGeneration() { return } // drop stale
        renderResult(result, err)
    })
}()
```

**Switch-proxy в полёте.** PUT `/proxies/{group}` запущенный до Connect может завершиться против **старого** endpoint'а — это harmless (no-op для нового endpoint), но UI callback `SetActiveProxyName` guard'ится тем же gen-check'ом, так что label не обновится на stale.

**Atomic swap.** `Set/ClearRemoteOverride` под `sync.RWMutex.Lock`: меняют value+active flag, bump'ают generation, копируют listener-slice (для notify за пределами lock'а). Чтение в `GetRemoteOverride` под `RLock`.

**Listener invocation** — за пределами write-lock'а, чтобы avoid deadlock'а если listener в свою очередь зовёт `Get`. Listener-функции должны быть тонкими (типа «trigger Fyne refresh»), heavy work делать в spawn'нутой goroutine.

---

## Edge cases

1. **Cold start, нет `bin/config.json`.** `LoadClashAPIConfig` возвращает error. Кнопка `Reset to local config` disabled с tooltip'ом. Pre-fill полей пустой. Юзер может всё ввести вручную.
2. **Юзер вставил `http://192.168.1.1:9090` в Host.** `NormalizeHost` strip'ает `http://`. Если после strip'а остался `192.168.1.1:9090` — inline-error «Host must not contain ':' — use Port field». Не silent-парсим, чтобы избежать сюрпризов.
3. **Empty Secret.** Allowed (Clash API без secret'а валиден). Request будет без `Authorization` header'а.
4. **Empty Host или Port.** Connect disabled, inline-error.
5. **IPv6 literal `[::1]`.** Reject в MVP с inline-error «IPv6 not supported yet». Реализация в follow-up SPEC при request'е.
6. **Remote стал unreachable после Connect.** Next refresh свалится → red status `ApiStatusLabel` + error toast. Tab НЕ auto-disable, НЕ auto-fallback на local — юзерское явное решение Use Local.
7. **Несколько листенеров `OnOverrideChanged` зарегистрированы.** Все вызываются sequentially после lock-release. Порядок не гарантирован semantically — listener'ы должны быть idempotent.
8. **Юзер открыл dialog при активном override.** Поля prepopulate'ятся из current override (не из local config). Reset-to-local явно перезаписывает.
9. **`fyne.Do` после tab unmount.** Standard Fyne race; уже handled через generation-check (gen-bump'аем на tab destroy если что; но в MVP unmount Servers tab не реалистичен — tab persistent весь runtime).
10. **Local sing-box ребилдится (config.json меняется) при активном override.** Override живёт независимо — local config-change не задевает remote-режим. Когда юзер кликает Use Local — берётся свежий local config.

---

## Tests (acceptance criteria)

### Unit (`ui/clash_remote_test.go`)

1. `TestEffectiveClashAPIConfig_LocalDefault` — без override возвращает local config.
2. `TestEffectiveClashAPIConfig_RemoteOverride` — после `SetRemoteOverride({Host:"1.2.3.4",Port:9090,Secret:"x"})` возвращает `("http://1.2.3.4:9090", "x", true, true)`.
3. `TestClearRemoteOverride_ResetsToLocal` — `Set → Clear → Effective` возвращает local config.
4. `TestNormalizeHost_StripsScheme` — `http://x`, `https://x`, `  x  ` → `x`.
5. `TestNormalizeHost_RejectsPathOrPort` — `host/path`, `host:port` → error.
6. `TestNormalizeHost_RejectsIPv6` — `[::1]`, `[fe80::1]` → error.
7. `TestGenerationMonotonic` — `Set → Clear → Set` → каждый раз gen++ (monotonic, never reused).
8. `TestOverrideChangedFiresCallback` — register listener, set, assert called; clear, assert called again.

### Manual (matrix)

| Сценарий | Ожидание |
|---|---|
| Default (override never touched), local running | Tab enabled, badge 🏠 Local, всё работает как до SPEC 064 |
| Local не запущен, нет override | Tab disabled (как сейчас) |
| Local не запущен, Set override on `127.0.0.1:9091` (mock mihomo) | Tab enabled, badge 🌐 127.0.0.1:9091, proxy list загружается с mock'а |
| Local running, Set override | Tab показывает remote, не local; badge 🌐 |
| Local stop при active override | Tab НЕ disable'ится, продолжает показывать remote |
| Click Use Local при active override | Tab refresh'ится на local, badge 🏠 |
| Connect на не-listening port | 3s timeout → confirm modal «Connect anyway?» |
| Connect anyway (даже на dead endpoint) | Override active, badge 🌐, в tab'е error-toast от первого refresh'а |
| Открыть dialog при active override | Поля prepopulate'ены из override (не из local) |
| Reset-to-local при no local config | Кнопка disabled, tooltip визибл |
| `http://1.2.3.4` в Host | После Connect — нормализуется до `1.2.3.4` (no scheme prefix) |
| `host:8080` в Host | Inline error «use Port field» |
| `[::1]` в Host | Inline error «IPv6 not supported» |
| Empty Secret + remote без auth | OK, работает |
| Connect → Refresh → Connect другой endpoint в первые 100ms | Refresh старого endpoint'а не пишет в UI (generation-guard) |

---

## Phases (implementation order)

### Phase 1 — Resolver scaffold (no UI change)
- Create `ui/clash_remote.go` со всем listed выше API.
- Unit tests (`ui/clash_remote_test.go`).
- **Acceptance:** `go test ./ui/...` green; ничего в behavior'е не меняется (override никогда не set'ится).

### Phase 2 — Callsite migration
- В `clash_api_tab.go` все 9 `ac.APIService.GetClashAPIConfig()` → `ui.EffectiveClashAPIConfig(ac)`. Mechanical refactor.
- **Acceptance:** build + manual smoke test (Servers tab работает как до миграции).

### Phase 3 — Tab gating + static badge
- `ui/app.go::updateClashAPITabState`: enable если `isRunning || hasOverride`.
- В `clash_api_tab.go`: добавить badge widget в top-row, всегда рендерит 🏠 Local (override flag в Phase 3 ещё не set'ится никем).
- Подписка на `OnOverrideChanged` в `NewApp`.
- **Acceptance:** tab по-прежнему disable'ится при stopped local; badge видим.

### Phase 4 — Dialog + Connect + probe + generation counter (heavy)
- Gear button в top-row.
- `showRemoteEndpointDialog` — entry + validation + buttons.
- `probeRemote` (3s context timeout).
- Set/Clear через override API.
- Generation-guard во всех refresh goroutine'ах вкладки.
- **Acceptance:** manual test matrix (см. §«Tests» → Manual) полностью green.

### Phase 5 — Locale + polish + comments
- Update EN+RU locale files с ~15 ключами.
- Comments в `ui/traffic_bootstrap.go` и `core/tray_menu.go` объясняют почему **они** остаются local-only.
- Release notes entry в `docs/release_notes/upcoming.md`.
- **Acceptance:** все строки локализованы (no hardcoded English в production-build); tooltips visible.

### Phase 6 — Build + reinstall + commit
- `./build/build_darwin.sh -i arm64`
- Один conventional commit: `feat(servers): remote Clash API endpoint override (SPEC 064)`.

---

## Out of scope

**MVP-cutoff. Не делаем:**
- **Profile list / MRU** — single endpoint в RAM. Хочется хранить несколько — отдельный SPEC, отдельная UI-сложность.
- **Persistence** в `bin/settings.json` — explicit-ephemeral по решению пользователя. Если хочется persistence — отдельный SPEC.
- **HTTPS / WSS** — explicit. Reverse-proxy через nginx с TLS — есть, но за пределами MVP.
- **Basic Auth** — только Clash Bearer token (через secret).
- **IPv6 literal** (`[::1]`) — explicit reject. Follow-up при request'е.
- **mDNS / network discovery** endpoint'ов — нет.
- **Auto-fallback** на local при unreachable remote — explicit. Юзер решает Use Local.
- **Connection-status indicator** beyond badge — нет 🟢/🔴 pulse, нет per-refresh latency.
- **Import endpoint from clipboard / QR / config-file** — explicit нет.
- **Edit-endpoint hotkey** — нет.

**Не меняем (остаются local-only):**
- **Traffic Profiler** (`ui/traffic_bootstrap.go`). Зависит от child.log tailing'а sing-box процесса — фундаментально локальная фича.
- **Tray menu proxy submenu** (`core/tray_menu.go`). Отражает state launcher'ского sing-box'а, не remote-endpoint'а.
- **Configurator / Wizard** — редактирует local `state.json` / `config.json`. Notion remote-config'а отсутствует.
- **Logs view** (Diagnostics tab) — local `bin/logs/` only.
- **Subscriptions update** — запускает local rebuild. К remote не относится.
- **AutoLoadProxies** на старте sing-box (controller.go:668). Wired на local-process lifecycle event — silent-skip если override active, refresh из manual reload / selector-change.

---

## Кредит

Идея — пользователь: «сделать чтобы если не запущен вкладка сервис все равно была доступна и там рядом с Clash API была шестеренка которая бы позволяла указать ключ/порт/ip и подключиться к другому экземпляру или даже к другой системе. Sing-box на роутериче».

Use case-фокус: home router (RouteRich на `192.168.10.1`) — см. memory `home_infra_project`.
