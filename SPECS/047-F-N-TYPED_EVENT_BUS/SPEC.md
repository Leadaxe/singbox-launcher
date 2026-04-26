# SPEC 047 — TYPED_EVENT_BUS

**Тип:** F (Feature) · **Статус:** N (New) · **Сопутствует:** SPEC 045.

## Контекст и проблема

В лаунчере сейчас единая coarse-grained модель реактивности через прямые callback'и в `UIService`:

```go
UpdateConfigStatusFunc   func()                    // «что-то с конфигом»
UpdateCoreStatusFunc     func()                    // «что-то с процессом»
UpdateTrayMenuFunc       func()                    // «обнови трей»
RefreshAPIFunc           func()                    // «перечитай API»
UpdateParserProgressFunc func(float64, string)
AutoPingAfterConnectFunc func()
```

Каждый — функция-broadcast, которую регистрирует один подписчик и дёргают все, кому надо «обновить что-нибудь по теме». Подписчик не знает причины вызова, поэтому **перечитывает всё** (например, `updateConfigInfo` читает `config.json` с диска при каждом дёрге), что неэффективно и шумит.

Аналогичная проблема есть в LxBox — у них тоже broadcast через ChangeNotifier (см. `LXBOX_NOTES.md` в SPEC 045). При рефакторинге к разделению state/config мы хотим сразу ввести **типизированные события**, чтобы не унаследовать coarse-grained долг.

## Цель

Ввести в `core/events/` типизированный event-bus, через который компоненты:
1. **Публикуют** события с конкретным типом и payload'ом.
2. **Подписываются** на конкретный тип (или конкретные типы) и получают только релевантные события.
3. Обработчики выполняются **синхронно** в той же goroutine, что публикует — для предсказуемости порядка и отсутствия гонок UI vs core.

## Контракт

```go
package events

type EventKind int

const (
    StateChanged EventKind = iota
    ConfigBuilt
    SubscriptionUpdated
    VpnStateChanged
    ProxyActiveChanged
    PowerResume
    AutoUpdateStatus
)

type Event struct {
    Kind    EventKind
    Payload any   // конкретный тип см. ниже
}

// Payload-структуры — типизированные:
type StateChangedPayload struct {
    Diff state.Diff   // (из core/state)
}
type ConfigBuiltPayload struct {
    OK        bool
    Validation build.ValidationResult
}
type SubscriptionUpdatedPayload struct {
    SourceTag    string
    Succeeded    int
    Failed       int
}
type VpnStateChangedPayload struct {
    Running bool
}
// ... и т.д.

type Handler func(Event)
type Cancel func()

type Bus interface {
    Publish(Event)
    Subscribe(kind EventKind, h Handler) Cancel
    SubscribeAll(h Handler) Cancel
}
```

### Реализация: `MemoryBus`

- `sync.RWMutex` на map[EventKind][]handlerEntry.
- `Publish` — under RLock, итерируется по handlers и вызывает их. **Sync dispatch.**
- `Subscribe` — under Lock, добавляет handler с уникальным id; возвращает Cancel-замыкание, которое removes by id.
- Panic в handler'е **не должен** отвалить весь Publish: каждый handler оборачивается в `defer recover` с warning в `debuglog`.

### Что это **не** делает

- Не гарантирует асинхронную доставку — это синхронный bus, по контракту.
- Не сериализует publish'ы между goroutines (если две goroutine публикуют одновременно, handler'ы могут видеть события в любом порядке per-горутине).
- Не персистит события (не история, не лог).
- Не делает «request/reply» — только fire-and-forget.

## Критерии приёмки

1. `core/events/` существует, тестируется в изоляции (zero deps на остальной проект).
2. Subscribe + Publish + Cancel работают, юнит-тестами покрыты edge-cases (cancel inside handler, panic inside handler).
3. После реализации SPEC 045: все callsites `UIService.UpdateConfigStatusFunc` / `UpdateCoreStatusFunc` / `UpdateTrayMenuFunc` переведены на `Subscribe(...)` к конкретным `EventKind`.
4. `RefreshAPIFunc`, `AutoPingAfterConnectFunc`, `UpdateParserProgressFunc` остаются как direct callbacks (это про конкретные точечные операции, не broadcast — события для них не нужны).
5. `go test -race ./core/events` зелёный.

## Связи

- Напрямую востребован SPEC 045 (StateService публикует события Save'а; UI слушает).
- После реализации может породить дочерние мини-задачи: «Tray menu подписывается на VpnStateChanged + ConfigBuilt вместо callback».

## Не входит

- Замена ВСЕХ callback'ов в проекте на события — только тех, что описаны в SPEC 045 + явно перечислены выше.
- Cross-process events (для Debug API нужно отдельное решение, не это).
- Persisted event log / undo-redo через события.
