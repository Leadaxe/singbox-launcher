# SPEC: Dirty-config маркер на кнопке Update

Задача: показывать пользователю, что в визарде были сохранены изменения, которые **парсер ещё не применил** к работающему `config.json`. Маркер `*` на кнопке Update + повышенный Importance.

**Статус:** реализовано в минимальной версии (коммит `92697c7`), **принято с TODO на редизайн** — сейчас семантика «dirty» размазана: маркер загорается на любом wizard Save, включая случаи когда парсер не нужен (tun/dns/rules/log_level правки уже в config.json).

**Спека написана ретроспективно.**

---

## 1. Проблема

### 1.1 До изменений

Пользователь открывает Wizard, что-то меняет, нажимает Save → возвращается в главное окно → `config.json` обновлён, но:

- **sing-box** по-прежнему работает со старой копией (если running).
- Плановый парсер отстаёт от сохранённого шаблона до следующего Update.
- Если были правки `ParserConfig.Proxies` (список URL, skip, tag_prefix), то outbound-ноды в `config.json` — старые (с прошлого парсинга).

Сигнала для пользователя нет. Частая жалоба: «я нажал Save, почему ничего не поменялось?».

### 1.2 Цель

Визуальный маркер на кнопке Update, который говорит:

- «Ты сохранил что-то, парсер не прогнался с тех пор».
- Сбрасывается автоматически при успешном Update.

---

## 2. Требования (минимальная версия — что реализовано)

### 2.1 Флаг на `StateService`

```go
TemplateDirty      bool
TemplateDirtyMutex sync.RWMutex

func (s *StateService) IsTemplateDirty() bool { ... RLock ... }
func (s *StateService) SetTemplateDirty(dirty bool) { ... Lock ... }
```

### 2.2 Writers

- `ui/wizard/presentation/presenter_save.go saveStateAndShowSuccessDialog` — ставит `TemplateDirty = true` после успешной записи.
- `core/config_service.go RunParserProcess` — на успехе ставит `TemplateDirty = false` + дёргает `UpdateConfigStatusFunc`.

### 2.3 Renderer

- `ui/core_dashboard_tab.go updateConfigInfo`:
  ```go
  base := locale.T("core.button_update")
  if ac.StateService.IsTemplateDirty() {
      tab.updateConfigButton.SetText("* " + base)
      tab.updateConfigButton.Importance = widget.HighImportance
  } else {
      tab.updateConfigButton.SetText(base)
      tab.updateConfigButton.Importance = widget.MediumImportance
  }
  ```

---

## 3. **Критический дизайн-изъян и редизайн**

### 3.1 Семантика «dirty» размазана

`config.json` **перезаписывается всегда** при wizard Save (в `presenter_save.go` комментарий прямо пишет: «Config.json already contains outbounds populated via PopulateParserMarkers — no immediate parser run needed. Subscriptions will refresh on the next auto-update cycle.»).

Подписки **НЕ рефетчатся** при Save.

Значит dirty-marker семантически имеет разные смыслы:

| Что поменял пользователь   | Что фактически устарело | Что надо сделать                |
|----------------------------|-------------------------|----------------------------------|
| URL подписки / skip        | outbounds в config.json | Update (re-fetch)                |
| Добавил источник           | outbounds в config.json | Update                           |
| tun / dns / log_level      | работающий sing-box     | Restart sing-box (если running)  |
| Routing rules / selectors  | работающий sing-box     | Restart sing-box                 |
| Template vars (@something) | работающий sing-box     | Restart sing-box                 |

Текущий маркер `*` на **Update** загорается во всех случаях — включая те, где Update бесполезен (4-я и 5-я строки). Пользователь видит `* Update`, жмёт, парсер отрабатывает, правки tun-режима не применяются потому что sing-box не перезапустили.

### 3.2 Правильная модель — из LxBox мобильного

Мобильный клиент разделяет:

- **`state.json`** — декларативное состояние пользователя (vars, sources, rules, DNS settings).
- **`config.json`** — сгенерированный рантайм-конфиг, собирается из state + subscription cache.

Wizard Save пишет ТОЛЬКО `state.json`. Отдельный шаг **Build Config** читает state + cache → собирает `config.json`. Build запускается:
- По Update (ручной или плановый).
- Автоматически при старте sing-box (если state изменился).

Два разных сигнала в UI:

- **`*` на Update** — когда `state.json` имеет изменения в `Proxies` (sources), которые не попали в последний build.
- **`*` на Restart (или на status-chip)** — когда state меняли template-level настройки (tun/dns/rules), а sing-box работает со старым config.json в памяти.

### 3.3 Что сделать для редизайна

1. **Разделить state ↔ config** (крупный архитектурный рефактор):
   - Новый формат `state.json` — declarative.
   - Переход от прямой мутации `config.json` в wizard saver'е к записи `state.json` + последующему build-шагу.
   - Build step: `ConfigService.BuildFromState(state, cache) → config.json`.
   - Startup: если state есть, а config.json нет или устарел — автоbuild.
2. **Два типа dirty-signal'ов** (после 1):
   - `StateService.SourcesDirty` — sources/subscriptions menjали, нужен Update.
   - `StateService.TemplateDirty` (переименовать, current уезжает) — template менял, нужен Restart (если running).
3. **Event-bus редизайн** (спека на `f28d8db` уровень) — `StateChanged{kind}` событие, подписчики на Core Dashboard рендерят правильный маркер.

Это — работа следующего серьёзного релиза, не one-night pass.

### 3.4 Минимальные улучшения до большого редизайна

Даже без разделения state/config можно сделать лучше:

- Различать типы изменений при Save — wizard знает, менялись ли `Proxies` vs vars vs rules.
- `StateService.SetTemplateDirty` превратить в два:
  - `SetSourcesDirty()` — при мутации `Proxies`.
  - `SetRuntimeDirty()` — при мутации template vars / rules / dns.
- Маркер `*` на Update — только для sources-dirty.
- Маркер `*` (или подсветка) на **Restart-button** / status-chip — для runtime-dirty.

Это ~100 LOC изменений в `presenter_save.go` + `updateConfigInfo`. Можно выкатить до полного state/config разделения.

---

## 4. Инварианты

1. **Маркер `*` не должен показывать "dirty" когда ничего не устарело.** Если пользователь открыл wizard и сразу Close без правок — marker не должен загореться.
2. **Маркер синхронен с реальным состоянием парсинга.** RunParserProcess успех → маркер гаснет. Failure → маркер остаётся.
3. **Perist'а маркера между запусками лаунчера нет** — при старте всегда clean. Основано на том что `config.json` уже на диске соответствует state'у.

---

## 5. Тесты

- **TODO:** тест на `SetTemplateDirty`/`IsTemplateDirty` round-trip — частично есть в `0c15fd8`.
- **TODO:** integration: wizard Save → проверка что Marker установлен в UI; Update → проверка что сброшен.

---

## 6. Открытые вопросы

- Нужен ли маркер на **Restart**-кнопке тоже? Или достаточно на Update? Или на status-chip?
- Как обозначить partial-dirty (sources OK, template надо Restart)? Два маркера? Один с разным цветом?
- Если авто-update отрабатывает сам — маркер гаснет сам, но пользователь мог не заметить. Нужен ли лог / тост «config refreshed in background»?
