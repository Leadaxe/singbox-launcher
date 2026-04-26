package events

import (
	"sync"
	"sync/atomic"

	"singbox-launcher/internal/debuglog"
)

// MemoryBus — in-memory реализация Bus с синхронной диспетчеризацией.
//
// Внутренняя структура:
//   - handlers[kind] — список подписчиков на конкретный Kind;
//   - all — список подписчиков SubscribeAll;
//   - каждый handler хранится с уникальным id для O(N) удаления при Cancel.
//
// Производительность: подходит для типичной нагрузки (десятки подписчиков,
// сотни Publish'ов в секунду). Не оптимизирован под тысячи подписчиков —
// если когда-нибудь понадобится, заменить linear-scan на map[id]entry.
type MemoryBus struct {
	mu       sync.RWMutex
	nextID   atomic.Uint64
	handlers map[EventKind][]handlerEntry
	all      []handlerEntry
}

type handlerEntry struct {
	id      uint64
	handler Handler
}

// NewMemoryBus создаёт новый bus с пустыми списками подписчиков.
func NewMemoryBus() *MemoryBus {
	return &MemoryBus{
		handlers: make(map[EventKind][]handlerEntry),
	}
}

// Publish доставляет событие всем подписчикам синхронно.
//
// Под RLock'ом снимается копия слайсов handlers[kind] и all,
// после чего lock отпускается. Это позволяет:
//   - handler'у вызвать Subscribe / Cancel без deadlock'а;
//   - параллельным Publish из других goroutine не ждать друг друга.
//
// Каждый handler оборачивается в defer-recover: panic не ломает доставку.
func (b *MemoryBus) Publish(ev Event) {
	b.mu.RLock()
	kindHandlers := append([]handlerEntry(nil), b.handlers[ev.Kind]...)
	allHandlers := append([]handlerEntry(nil), b.all...)
	b.mu.RUnlock()

	for _, h := range kindHandlers {
		safeCall(h.handler, ev)
	}
	for _, h := range allHandlers {
		safeCall(h.handler, ev)
	}
}

// Subscribe регистрирует handler на конкретный Kind.
// Возвращает Cancel-функцию; вызов после первого Cancel — no-op.
func (b *MemoryBus) Subscribe(kind EventKind, h Handler) Cancel {
	if h == nil {
		// Передавать nil-handler — программная ошибка вызывающего, но
		// не валим работающий процесс — возвращаем no-op Cancel.
		debuglog.WarnLog("events.Subscribe called with nil handler for kind=%s", kind)
		return func() {}
	}
	id := b.nextID.Add(1)
	entry := handlerEntry{id: id, handler: h}

	b.mu.Lock()
	b.handlers[kind] = append(b.handlers[kind], entry)
	b.mu.Unlock()

	var cancelled atomic.Bool
	return func() {
		if !cancelled.CompareAndSwap(false, true) {
			return
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		list := b.handlers[kind]
		for i, e := range list {
			if e.id == id {
				b.handlers[kind] = append(list[:i], list[i+1:]...)
				return
			}
		}
	}
}

// SubscribeAll регистрирует handler на все Kind'ы.
// Полезно для отладочного логирования; в прод-коде предпочитайте Subscribe.
func (b *MemoryBus) SubscribeAll(h Handler) Cancel {
	if h == nil {
		debuglog.WarnLog("events.SubscribeAll called with nil handler")
		return func() {}
	}
	id := b.nextID.Add(1)
	entry := handlerEntry{id: id, handler: h}

	b.mu.Lock()
	b.all = append(b.all, entry)
	b.mu.Unlock()

	var cancelled atomic.Bool
	return func() {
		if !cancelled.CompareAndSwap(false, true) {
			return
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		for i, e := range b.all {
			if e.id == id {
				b.all = append(b.all[:i], b.all[i+1:]...)
				return
			}
		}
	}
}

// safeCall вызывает handler с recover'ом, чтобы panic'и не ломали доставку.
func safeCall(h Handler, ev Event) {
	defer func() {
		if r := recover(); r != nil {
			debuglog.WarnLog("event handler panicked on kind=%s: %v", ev.Kind, r)
		}
	}()
	h(ev)
}

// Compile-time проверка, что MemoryBus реализует интерфейс Bus.
var _ Bus = (*MemoryBus)(nil)
