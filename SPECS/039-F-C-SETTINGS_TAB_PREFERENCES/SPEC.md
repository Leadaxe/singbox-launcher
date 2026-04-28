# SPEC: Settings-таб — консолидация launcher-wide preferences

Задача: собрать в одном месте все пользовательские настройки, которые раньше были разбросаны по Core Dashboard и Help, и добавить новые переключатели (автообновление подписок, авто-пинг).

**Статус:** реализовано. Коммиты: `500322c` (auto-update toggle), `f04b0e9` (auto-ping after connect), `e7c46f9` (Settings tab + reorg, утренний коммит). Ретроспективная спека.

**Дополнения:**
- Field-report 2026-04-26 — soft cap для авто-пинга по числу нод (см. §1.3, §2.5, §2.7).

**Связанные спеки:**
- `SPECS/032-F-C-WIZARD_SETTINGS_TAB/` — **другой** Settings-tab, внутри wizard'а. Этот 039 — главное окно лаунчера.

---

## 1. Проблема

### 1.1 До изменений

- Launcher-wide preferences разбросаны:
  - Language-селектор + Download-locales висят в `Help` табе (случайно — «куда ещё воткнуть?»).
  - Ping-test-URL + parallelism живут в диалоге gear-кнопки на вкладке Servers.
  - Нет единой точки «открыть настройки приложения».
- Нет явного **выключателя** для фоновых автоматизаций:
  - Плановые обновления подписок — единственный способ остановить их был допустить две ошибки подряд (auto-disable через failed-attempts counter), что неявно и зависит от провайдера.
  - Auto-ping после connect — хочется выключить на flaky-сетях или при ручном режиме.
- В help-handler'е language-селектора есть **data-loss баг**: `locale.SaveSettings(binDir, Settings{Lang: code})` создаёт свежий `Settings{}` с только полем `Lang` — остальные поля (ping URL, concurrency, любые будущие) затираются.

### 1.2 Цель

1. Создать отдельный **⚙️ Settings** таб в главном окне (между Servers и Diagnostics), содержащий:
   - Секция **Subscriptions**: «Автообновление подписок» + «Автопинг после подключения».
   - Секция **Language**: селектор + «Download locales».
2. Унести эти элементы из Help / Core Dashboard.
3. Добавить соответствующие флаги в `bin/settings.json`, учитывать на старте.
4. Починить data-loss баг language-handler'а — load-mutate-save.

### 1.3 Field-report 2026-04-26 — auto-ping storm на больших списках

**Симптом (Win10):** на 0.8.7+ игры (Where Winds Meet, Solo Leveling, Steam-игры) зависают на экране логина клиента. Возврат на 0.8.6 (где автопинга нет) — починка. Конфиг и состояние идентичны (state.json v4 с ~500 нод от CIDR-парсеров `WHITE-CIDR-RU-all.txt` и др.).

**Корень:** `f04b0e9` армит таймер на `5s` → `pingAllProxies` → Clash API `/proxies/{name}/delay` для **каждой** ноды. Воркеров `pingTestAllConcurrency` штук (default `20`, max `100`); каждый воркер открывает TCP+TLS handshake к удалённому прокси-серверу. На ~500 нодах это ~25 батчей по 20 параллельных хэндшейков, всего >1 минуты непрерывного шторма исходящих соединений через TUN. В этом окне игровой клиент не может протолкнуть свои HTTPS-запросы (login/auth/CDN), Windows TCP-стек захлёбывается одновременными SYN'ами.

**Решение (минимальное):** soft-cap по числу нод. Если `len(GetProxiesList()) > AutoPingMaxProxies` — авто-пинг **молча пропускается** (debug-лог пишется), пользователь по-прежнему может ткнуть «Test» в Servers вручную. Дефолт **`150`** — эмпирически подтверждённый порог пользователем («если очень много серверов, давай скажем более 150, то авто-пинг надо отключать»). Override через `bin/settings.json` → `auto_ping_after_connect_max_proxies`.

**Что НЕ делаем (open-questions внизу):**
- Не вводим UI-поле для порога — settings.json достаточно.
- Не урезаем concurrency на лету — это глобальная настройка пользователя для ручного «Test», менять её под автопинг хитро и непрозрачно.
- Не реализуем «poll-by-batch» с паузами между батчами — сложнее, и при наличии cap'а не нужно.

---

## 2. Требования

### 2.1 Новый таб «⚙️ Settings»

- Позиция: **между Servers и Diagnostics** (индекс 2 в `AppTabs`).
- Иконка: `⚙️` (совпадает с Core Dashboard — sic! — возможно поменять на другой символ в следующем проходе).
- Файл: новый `ui/settings_tab.go`, функция `CreateSettingsTab(ac *core.AppController) fyne.CanvasObject`.
- Layout: `container.NewPadded(container.NewVBox(subsTitle, autoUpdateCheck, autoPingCheck, separator, langTitle, langRow))`.

### 2.2 Секция Subscriptions

**Чекбокс 1 — «Автообновление подписок»** (key `core.auto_update_subs_label`):

- Читает `ac.StateService.IsAutoUpdateEnabled()`.
- `OnChanged(enabled)`:
  1. `SetAutoUpdateEnabled(enabled)`.
  2. Если `enabled == true` → `ResetAutoUpdateFailedAttempts()` — сбросить счётчик, чтобы цикл не отрубил себя повторно.
  3. `LoadSettings` → `SubscriptionAutoUpdateDisabled = !enabled` → `SaveSettings` (**load-mutate-save**, не свежий struct).
- Default: ON (отсутствие флага в settings.json = enabled).

**Чекбокс 2 — «Автопинг после подключения»** (key `core.auto_ping_label`):

- Читает `ac.StateService.IsAutoPingAfterConnectEnabled()`.
- `OnChanged`: `SetAutoPingAfterConnectEnabled` + load-mutate-save `AutoPingAfterConnectDisabled`.
- Default: ON.

### 2.3 Секция Language

- Селектор `widget.NewSelect(locale.LangDisplayNames())`:
  - `OnChanged`: `locale.SetLang(code)` + **load-mutate-save** `Settings.Lang = code`. **Не `Settings{Lang: code}`** — это и есть исправление data-loss бага.
  - После смены — `ShowInfo(...)` с инструкцией «перезапустить для полного применения».
- Кнопка «Download locales» (ttwidget с tooltip):
  - `DownloadAllRemoteLocales` в goroutine.
  - По успеху — обновить `langSelect.Options` через `LangDisplayNames()`.
  - По ошибке — `ShowDownloadFailedManual` с ссылкой и путём папки.
- Layout: `container.NewBorder(nil, nil, langLabel, downloadLocalesBtn, langSelect)` — селектор растягивается, кнопка справа компактная.

### 2.4 `StateService` — новые флаги

```go
const DefaultAutoPingMaxProxies = 150 // см. §1.3

type StateService struct {
    // ... existing fields ...

    AutoPingAfterConnect      bool
    AutoPingAfterConnectMutex sync.RWMutex // охраняет и AutoPingAfterConnect, и AutoPingMaxProxies

    AutoPingMaxProxies int // 0 = no cap; init = DefaultAutoPingMaxProxies
}

func (s *StateService) IsAutoPingAfterConnectEnabled() bool   // RLock
func (s *StateService) SetAutoPingAfterConnectEnabled(b bool) // Lock
func (s *StateService) GetAutoPingMaxProxies() int            // RLock
func (s *StateService) SetAutoPingMaxProxies(n int)           // Lock; n<0 → 0
```

- `AutoUpdateEnabled` / `AutoUpdateFailedAttempts` / `AutoUpdateMutex` — **уже** существуют (spec на auto-update loop, документирован в `auto_update.go`). Добавлять не надо, только дёргать setter.
- `AutoPingMaxProxies` шарит мьютекс с `AutoPingAfterConnect` намеренно — оба относятся к одному и тому же event'у (timer fired); split-mutex не оправдан.

### 2.5 `Settings` (persist)

Поля в `internal/locale/settings.go`:

```go
SubscriptionAutoUpdateDisabled bool `json:"subscription_auto_update_disabled,omitempty"`
AutoPingAfterConnectDisabled   bool `json:"auto_ping_after_connect_disabled,omitempty"`
AutoPingAfterConnectMaxProxies int  `json:"auto_ping_after_connect_max_proxies,omitempty"` // см. §1.3
```

Инвертированный default (disabled=false → enabled) — чтобы существующие settings.json без поля работали как раньше.

Семантика `AutoPingAfterConnectMaxProxies`:
- `0` / поле отсутствует → использовать `services.DefaultAutoPingMaxProxies` (=150).
- `>0` → переопределить дефолт. Например, `300` для мощного железа / маленького количества соединений на ноду.
- Поле UI не имеет — это "power-user" override через прямое редактирование settings.json. UI-выключатель (§2.2 чекбокс 2) сильнее: если он `false`, авто-пинг отключён вообще, max не проверяется.

### 2.6 `main.go` startup

После `LoadSettings`:

```go
if settings.SubscriptionAutoUpdateDisabled {
    controller.StateService.SetAutoUpdateEnabled(false)
}
if settings.AutoPingAfterConnectDisabled {
    controller.StateService.SetAutoPingAfterConnectEnabled(false)
}
if settings.AutoPingAfterConnectMaxProxies > 0 {
    controller.StateService.SetAutoPingMaxProxies(settings.AutoPingAfterConnectMaxProxies)
}
```

### 2.7 Auto-ping implementation (в `RunningState.Set`)

```go
func (r *RunningState) Set(value bool) {
    r.Lock()
    if r.running == value { r.Unlock(); return }
    r.running = value
    ac := r.controller
    if value {
        if r.autoPingTimer != nil { r.autoPingTimer.Stop() }
        if ac != nil && ac.StateService != nil && ac.StateService.IsAutoPingAfterConnectEnabled() {
            r.autoPingTimer = time.AfterFunc(5*time.Second, func() {
                if !r.IsRunning() { return }       // user might Stop in the 5s window
                if ac.UIService != nil && ac.UIService.AutoPingAfterConnectFunc != nil {
                    ac.UIService.AutoPingAfterConnectFunc()
                }
            })
        }
    } else if r.autoPingTimer != nil {
        r.autoPingTimer.Stop()
        r.autoPingTimer = nil
    }
    r.Unlock()
    // ... rest as was ...
}
```

Hook `UIService.AutoPingAfterConnectFunc func()` — регистрируется в `clash_api_tab.go` так:

```go
ac.UIService.AutoPingAfterConnectFunc = func() {
    fyne.Do(pingAllProxies)
}
```

`AutoPingAfterConnectFunc` остаётся **uncapped**. Это намеренно: тот же hook биндится к Cmd/Ctrl+P (`ui/app.go`) и `/action/ping-all` debug-API endpoint (`core/debugapi_wiring.go`) — оба эксплицитные пользовательские триггеры. Cap применяется **только на timer-driven сайтах**:

1. **`core/controller.go` `RunningState.Set`** — таймер на 5с после `running=true`:

```go
if max := ac.StateService.GetAutoPingMaxProxies(); max > 0 {
    if count := len(ac.GetProxiesList()); count > max {
        debuglog.InfoLog("auto-ping: skipped on connect (proxies=%d > cap=%d); use Servers → Test or Cmd/Ctrl+P for manual ping", count, max)
        return
    }
}
ac.UIService.AutoPingAfterConnectFunc()
```

2. **`main.go` `RegisterPowerResumeCallback`** — таймер на 5с после wake (см. SPEC 011): тот же guard перед вызовом `AutoPingAfterConnectFunc`.

Cap дублирован между двумя сайтами намеренно — это две короткие 4-строчные проверки, абстракция через метод `AppController.AutoPingAllowedByCap()` экономит одну строку и теряет grep'абельность. Если появится третий timer-driven caller, выносить в helper.

### 2.8 Data-loss bug fix (из старого help-handler'а)

Старый код (до `e7c46f9`) в `ui/help_tab.go`:

```go
if err := locale.SaveSettings(binDir, locale.Settings{Lang: code}); err != nil { ... }
```

Это затирает **ВСЕ** остальные поля `Settings` (ping URL, concurrency, auto-update флаг, debug-API token+port и т.д.).

Новый код в `ui/settings_tab.go`:

```go
st := locale.LoadSettings(binDir)
st.Lang = code
locale.SaveSettings(binDir, st)
```

**Паттерн «load-mutate-save» — обязателен для ВСЕХ будущих editor'ов `settings.json`.**

### 2.9 Локализация

Новые ключи:

- `app.tab.settings` — «⚙️ Settings» / «⚙️ Настройки»
- `settings.section_subscriptions` — «Subscriptions» / «Подписки»
- `settings.section_language` — «Language» / «Язык»
- `core.auto_update_subs_label` — «Auto-update subscriptions» / «Автообновление подписок»
- `core.auto_ping_label` — «Auto-ping on connect» / «Автопинг после подключения»

`help.language_label` / `help.download_locales*` / `help.language_changed` — переиспользуются из старого help-handler'а.

---

## 3. Инварианты

1. **Settings.json пишется только через load-mutate-save.** Любой handler, который создаёт свежий `Settings{...}` и сохраняет — баг.
2. **Default — всегда opt-in к `omitempty`.** Новое булево поле формулируется так, чтобы zero-value (`false`) означало прежнее поведение (обычно enabled). Для int-полей (как `AutoPingAfterConnectMaxProxies`) — zero-value = "use built-in default".
3. **`Set*Enabled` не пишет в settings.json сам.** Запись делает UI-handler после вызова setter'а. Core-код настройки не персистит.
4. **Auto-ping таймер принадлежит `RunningState`** — не `StateService` и не UI. Отменяется на false-transition.
5. **Soft-cap для авто-пинга применяется только на timer-driven сайтах** (`RunningState.Set` 5s timer + `power.OnResume` 5s timer). `AutoPingAfterConnectFunc` сам по себе **uncapped**, потому что тот же hook биндится к манульным триггерам (Cmd/Ctrl+P, `/action/ping-all`). Manual «Test» button через `pingAllProxies` тоже cap'ом не ограничен — все эксплицитные действия пользователя проходят без проверки.

---

## 4. Совместимость

- Старые `settings.json` без новых полей → `omitempty` zero → enabled → прежнее поведение.
- Language-селектор перенесён из Help → Settings. Пользователь, привыкший искать в Help, не найдёт сразу; в release-notes упомянуть.

---

## 5. Не-цели

- Не добавляем «scheduled auto-update» (раз в сутки / по пятницам etc.) — есть `reload` interval в парсер-конфиге, это отдельный механизм.
- Не делаем profile-switching (несколько настроек для разных сред) — одна settings.json на всё.
- Не синхронизируем настройки между устройствами — никаких облачных sync.

---

## 6. Открытые вопросы

- Иконка таба: сейчас `⚙️`, такая же как у Core. Может поменять на `🎛️` / `🛠`? — в release-polish.
- Нужны ли нам **под-вкладки** внутри Settings (Subscriptions / Language / Advanced)? — пока секций 2, компактно; когда секций станет больше 5 — рефакторить в tab'ы или accordion.
- Event-bus редизайн (см. `docs/night-reports/2026-04-22.md` под `f28d8db`): после него `OnChanged`-handlers могут публиковать типизированные события вместо прямого вызова `UIService.UpdateXxxFunc`.
- **Auto-ping max-proxies UI**: сейчас power-user only через settings.json. Если жалобы повторятся у других пользователей с большими списками — поднять в Settings tab как numeric-entry с tooltip-объяснением. Альтернатива: показывать notification «auto-ping skipped (N nodes > K cap)» в Servers tab, чтобы было заметнее, что cap сработал.
- **Дробление ping-batch'ей с паузами** вместо cap'а — позволит держать актуальную latency и для больших списков, ценой больших задержек до полной картины. Реализуется как отдельный backoff в `pingAllProxies` (`time.Sleep(N)` между батчами при `len(proxies) > threshold`). Не делаем сейчас — cap решает 90% случаев.
