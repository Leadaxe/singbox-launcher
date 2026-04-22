# Реализация: 037 — SUBSCRIPTION_SOURCE_TOGGLE

**Коммит:** `996dbec` (ветка `night-work`, 2026-04-22).
**Спека написана ретроспективно** — фича шипилась без предварительного SPEC (процессный долг зафиксирован).

## Что сделано

- **`core/config/configtypes/types.go`** — поле `Disabled bool json:"disabled,omitempty"` в `ProxySource`.
- **`core/config/outbound_generator.go`:**
  - Pre-scan для подсчёта enabled → `totalSources`.
  - `if proxySource.Disabled { continue }` перед вызовом `loadNodesFunc`.
  - `processedIdx` вместо raw `i` для progress-callback'а (чтобы прогресс шёл по enabled, а не по всем).
  - Early-return `"no enabled sources (all subscriptions disabled in wizard)"` когда `totalSources == 0 && allNodes == 0`.
- **`ui/wizard/tabs/source_tab.go`** — чекбокс в строке Sources-таба:
  - `enableCheck := widget.NewCheck("", nil)`, `SetChecked(!proxyPtr.Disabled)`.
  - OnChanged → мутация модели → serialize → invalidate preview → refresh list.
  - LowImportance на sourceLabel + prefixLabel для disabled-строк.
  - Layout через `container.NewBorder(nil, nil, enableCheck, rightControls, rowCenter)`.

## Что не сделано (TODO)

- **Unit-тесты** парсера на микс enabled/disabled — 3 сценария (всё enabled / один disabled / всё disabled).
- **Unit-тест** сериализации `ProxySource` с/без поля.
- **`docs/ARCHITECTURE.md`** — отдельным проходом.

## Бэкворд-совместимость

- `omitempty` + zero-value bool = legacy файлы без поля продолжают работать как раньше (Disabled = false = enabled).
- Никаких миграций state-формата не требовалось.

## Связанные спеки / задачи

- **`84698fd`** (per-source summary) — когда переделается, должен **не учитывать** disabled-источники ни в `TotalSources`, ни в `Failed`/`Succeeded`. Семантика: disabled просто нет в пуле.
- **`92697c7`** (dirty marker) и его будущий редизайн (separate state+build per mobile LxBox model) — изменения в `Disabled` должны считаться «изменением sources» и триггерить dirty-marker / invalidate outbounds-cache. Сейчас так и работает через `PreviewNeedsParse=true`.
