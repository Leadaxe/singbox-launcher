# PLAN 046 — TYPED_EVENT_BUS

## Структура пакета

```
core/events/
├── events.go         — типы (EventKind константы, Event, Handler, Cancel, Bus интерфейс)
├── payloads.go       — типизированные payload-структуры (StateChangedPayload и т.д.)
├── memory_bus.go     — MemoryBus (sync.RWMutex реализация)
└── memory_bus_test.go
```

`payloads.go` отделён, чтобы при добавлении новых типов событий не разрастался основной файл. Payload-типы могут импортировать другие пакеты (например, `core/state.Diff`) — событийный пакет от них зависит.

## Контракт `MemoryBus`

```go
type handlerEntry struct {
    id      uint64
    handler Handler
}

type MemoryBus struct {
    mu       sync.RWMutex
    nextID   atomic.Uint64
    handlers map[EventKind][]handlerEntry
    all      []handlerEntry
}

func NewMemoryBus() *MemoryBus
func (b *MemoryBus) Publish(ev Event)
func (b *MemoryBus) Subscribe(kind EventKind, h Handler) Cancel
func (b *MemoryBus) SubscribeAll(h Handler) Cancel
```

### Поведение

- **`Publish`** — захватывает `RLock`, копирует слайс handlers (чтобы не держать lock'и во время вызова и не блокировать Subscribe из handler'а), отпускает lock, итерируется и вызывает каждый handler в `defer recover`-блоке.
- **`Subscribe(kind, h)`** — захватывает Lock, добавляет в `handlers[kind]` запись с `id = nextID.Add(1)`. Возвращает `Cancel`, который захватывает Lock и удаляет запись по id (linear scan; ок при низком числе подписчиков).
- **`SubscribeAll(h)`** — то же самое, но в отдельный список `all`. На Publish — handlers по kind + all.
- **Cancel idempotent** — повторный вызов после удаления не паникует.

### Panic-isolation

```go
func safeCall(h Handler, ev Event) {
    defer func() {
        if r := recover(); r != nil {
            debuglog.WarnLog("event handler panicked on kind=%d: %v", ev.Kind, r)
        }
    }()
    h(ev)
}
```

## Тесты

```go
func TestPublishSubscribeBasic(t *testing.T)               // один handler получает событие
func TestPublishMultipleHandlers(t *testing.T)             // N handlers — все получают
func TestSubscribeAll(t *testing.T)                        // SubscribeAll получает все Kind'ы
func TestCancelStopsDelivery(t *testing.T)                 // Cancel — handler больше не вызывается
func TestCancelIdempotent(t *testing.T)                    // Cancel дважды — не падает
func TestCancelInsideHandler(t *testing.T)                 // handler вызывает Cancel сам — следующий Publish не вызывает
func TestPanicInHandlerDoesNotBreakOtherHandlers(t *testing.T)
func TestConcurrentSubscribePublish(t *testing.T)          // race-detector smoke
```

## Зависимости

- `singbox-launcher/internal/debuglog` (для WARN при panic).

Никаких других внешних. `payloads.go` импортирует `core/state` и `core/build` — **но** только когда эти пакеты появятся (фазы 3.2 и 3.4 SPEC 045). До тех пор payload-типы можно объявить как `any` и заполнить позже без breaking change.

## Порядок реализации

1. `events.go` — типы + интерфейс. Без payload-структур (используем `any`).
2. `memory_bus.go` — реализация.
3. `memory_bus_test.go` — все тесты, `go test -race`.
4. (Позже, при появлении `core/state` + `core/build`) — `payloads.go` с типизированными структурами.
