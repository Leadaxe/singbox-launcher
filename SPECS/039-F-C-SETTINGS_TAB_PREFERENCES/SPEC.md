# SPEC: Settings-таб — консолидация launcher-wide preferences

Задача: собрать в одном месте все пользовательские настройки, которые раньше были разбросаны по Core Dashboard и Help, и добавить новые переключатели (автообновление подписок, авто-пинг).

**Статус:** реализовано. Коммиты: `500322c` (auto-update toggle), `f04b0e9` (auto-ping after connect), `e7c46f9` (Settings tab + reorg, утренний коммит). Ретроспективная спека.

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
type StateService struct {
    // ... existing fields ...

    AutoPingAfterConnect      bool
    AutoPingAfterConnectMutex sync.RWMutex
}

func (s *StateService) IsAutoPingAfterConnectEnabled() bool   // RLock
func (s *StateService) SetAutoPingAfterConnectEnabled(b bool) // Lock
```

- `AutoUpdateEnabled` / `AutoUpdateFailedAttempts` / `AutoUpdateMutex` — **уже** существуют (spec на auto-update loop, документирован в `auto_update.go`). Добавлять не надо, только дёргать setter.

### 2.5 `Settings` (persist)

Два новых поля в `internal/locale/settings.go`:

```go
SubscriptionAutoUpdateDisabled bool `json:"subscription_auto_update_disabled,omitempty"`
AutoPingAfterConnectDisabled   bool `json:"auto_ping_after_connect_disabled,omitempty"`
```

Инвертированный default (disabled=false → enabled) — чтобы существующие settings.json без поля работали как раньше.

### 2.6 `main.go` startup

После `LoadSettings`:

```go
if settings.SubscriptionAutoUpdateDisabled {
    controller.StateService.SetAutoUpdateEnabled(false)
}
if settings.AutoPingAfterConnectDisabled {
    controller.StateService.SetAutoPingAfterConnectEnabled(false)
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
2. **Default — всегда opt-in к `omitempty`.** Новое булево поле формулируется так, чтобы zero-value (`false`) означало прежнее поведение (обычно enabled).
3. **`Set*Enabled` не пишет в settings.json сам.** Запись делает UI-handler после вызова setter'а. Core-код настройки не персистит.
4. **Auto-ping таймер принадлежит `RunningState`** — не `StateService` и не UI. Отменяется на false-transition.

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
