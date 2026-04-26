# TASKS 046 — TYPED_EVENT_BUS

## Реализация

- [ ] Создать пакет `core/events/`.
- [ ] `events.go`: `EventKind` enum, `Event` struct, `Handler`, `Cancel`, `Bus` интерфейс.
- [ ] `memory_bus.go`: `MemoryBus` с `sync.RWMutex` + `atomic.Uint64` ID counter.
- [ ] `Publish` копирует слайс под RLock, итерируется без lock'а; каждый handler в `defer recover`.
- [ ] `Subscribe(kind, h)` под Lock, возвращает Cancel.
- [ ] `SubscribeAll(h)` — отдельный список `all`.
- [ ] Cancel idempotent.
- [ ] `payloads.go` пока пустой — заполняется в фазе 5 SPEC 045.

## Тесты

- [ ] `memory_bus_test.go` со всеми сценариями (см. PLAN.md).
- [ ] `go test -race ./core/events` — зелёный.

## Подключение

- [ ] `AppController` владеет `events.Bus`, передаёт в `StateService` (фаза 4.2 SPEC 045).
- [ ] (Позднее) — UI-подписки в `core_dashboard_tab` и др.

## Закрытие

- [ ] IMPLEMENTATION_REPORT.md с фактической поверхностью API.
- [ ] Папка `046-F-N-` → `046-F-C-`.
