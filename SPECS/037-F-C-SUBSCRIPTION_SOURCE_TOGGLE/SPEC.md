# SPEC: Wizard — переключатель ON/OFF для каждой подписки

Задача: в визарде на вкладке **Sources** дать пользователю возможность **быстро выключить и включить** отдельный источник подписок без его удаления / повторного ввода URL.

**Статус:** реализовано (commit `996dbec`, ночь 2026-04-22). Детали — **PLAN.md**, **IMPLEMENTATION_REPORT.md**. Спека написана **ретроспективно** — фича шипилась без предварительного SPEC, процессный долг зафиксирован в `docs/night-reports/2026-04-22.md`.

---

## 1. Проблема

### 1.1 До изменений

- В `ParserConfig.Proxies` каждый элемент — активный источник; единственный способ «временно не использовать» — удалить его из списка (с потерей URL / skip-правил / tag-prefix).
- Частые сценарии:
  - Провайдер A молча лёг на ночь; хочется отключить до утра, а завтра включить.
  - A/B сравнение: «какие ноды живут только у провайдера B» — отключить A, прогнать Update, посмотреть node list.
  - Отладка: «парсер падает на этом источнике», временно убрать его без удаления.
- Пользователь вынужден держать URL / skip-rules / tag-prefix в буфере обмена или в заметках на время отладки.

### 1.2 Цель

- Один клик в UI выключает источник — он перестаёт участвовать в парсинге, но все его настройки остаются на месте.
- Второй клик возвращает в работу.
- В парсере выключенный источник пропускается **полностью** (нет fetch'а, нет parse'а, нет вклада в node list).
- В сводке «N of M succeeded» (если будет реализована, см. SPEC для 84698fd редизайна) выключенные не считаются ни успехами, ни провалами — их попросту нет в пуле.

---

## 2. Требования

### 2.1 Модель данных

- Новое поле `ProxySource.Disabled bool json:"disabled,omitempty"`.
- **Обязательно `omitempty`** — legacy `ParserConfig.json` без поля интерпретируется как `Disabled == false` (т.е. enabled, прежнее поведение).
- Сериализация: после save визард пишет поле только если оно `true` (лишний шум в JSON нежелателен).

### 2.2 Парсер

- `core/config/outbound_generator.go GenerateOutboundsFromParserConfig`:
  - Источники с `Disabled == true` **пропускаются до** вызова `loadNodesFunc` (нет fetch, нет parse).
  - `debuglog.DebugLog` строка про skip — чтобы в логах было видно.
  - `TotalSources` в результирующей структуре `OutboundGenerationResult` считает **только enabled** источники. Disabled не считаются ни `Succeeded`, ни `Failed`.
- Если **все** источники выключены — `GenerateOutboundsFromParserConfig` возвращает явную ошибку `"no enabled sources (all subscriptions disabled in wizard)"` (отличная от `"no nodes parsed from any source"`).

### 2.3 UI — Sources tab визарда

- В строке каждого источника (`refreshSourcesList` в `ui/wizard/tabs/source_tab.go`) добавляется **чекбокс слева** от label'а.
- Чекбокс отражает `!proxy.Disabled` (checked = enabled).
- На toggle:
  1. Пишет в `m.ParserConfig.ParserConfig.Proxies[sourceIndex].Disabled`.
  2. Вызывает `wizardbusiness.SerializeParserConfig` + сохраняет в `m.ParserConfigJSON`.
  3. Ставит `m.PreviewNeedsParse = true`, вызывает `wizardbusiness.InvalidatePreviewCache(m)`.
  4. Дёргает `presenter.UpdateParserConfig(serialized)` + `guiState.RefreshSourcesList()`.
- **Визуальное подавление** disabled-строки: `sourceLabel.Importance = widget.LowImportance`; если есть `prefixLabel` — тоже LowImportance. Чтобы в списке disabled-строки явно отличались от активных.
- Поведение drag-reorder / edit / delete не меняется (disabled-строка остаётся полноценной по составу действий).

### 2.4 Совместимость

- Старые state.json / ParserConfig.json без `disabled` продолжают работать (`omitempty` → zero value `false` → enabled).
- Добавление фичи не требует миграции файлов.

### 2.5 Локализация и документация

- **Новых локалей не требуется.** Чекбокс без label'а, tooltip — через сам контекст строки (tooltip уже висит на source_label через `sourceLabel.SetToolTip(tooltipText)`).
- В `docs/release_notes/upcoming.md` — запись в секции «Added».
- В `docs/ARCHITECTURE.md` — при следующем sync обновить описание Sources tab.

---

## 3. Не-цели

- Не решает проблему **partial failure** — если три источника из пяти временно лежат, они **ошибочно** считаются failed. Управлять через Disabled — это ручной шаг, не автоматический retry.
- Не предоставляет scheduler'а «отключить на N часов». Toggle бинарный, без времени жизни.
- Не персистит «причину» выключения. Если важно — пользователь пишет в `comment` исходника.
- Не даёт разным группам различные правила включения/выключения источников.

---

## 4. Открытые вопросы

- Будет ли `Disabled` учитываться в `IMPLEMENTATION_REPORT.md` коммита `84698fd` (per-source summary), когда тот переделается? Ответ: **нет**, disabled просто не в пуле — ни в знаменателе «N/M», ни в счётчике failed.
- Нужна ли отдельная иконка (🚫 / ⏸) рядом с disabled-строкой помимо LowImportance? Решение: пока нет — LowImportance достаточен для сканируемости.
