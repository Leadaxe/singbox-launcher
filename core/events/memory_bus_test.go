package events

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestPublishSubscribeBasic — один handler получает одно событие.
func TestPublishSubscribeBasic(t *testing.T) {
	bus := NewMemoryBus()
	var got Event
	cancel := bus.Subscribe(ConfigBuilt, func(ev Event) {
		got = ev
	})
	defer cancel()

	want := Event{Kind: ConfigBuilt, Payload: ConfigBuiltPayload{OK: true}}
	bus.Publish(want)

	if got.Kind != ConfigBuilt {
		t.Fatalf("kind: want %s, got %s", ConfigBuilt, got.Kind)
	}
	p, ok := got.Payload.(ConfigBuiltPayload)
	if !ok {
		t.Fatalf("payload type: want ConfigBuiltPayload, got %T", got.Payload)
	}
	if !p.OK {
		t.Fatalf("payload.OK: want true")
	}
}

// TestPublishMultipleHandlers — N handlers, все получают событие.
func TestPublishMultipleHandlers(t *testing.T) {
	bus := NewMemoryBus()
	var calls atomic.Int32
	const n = 5
	cancels := make([]Cancel, n)
	for i := 0; i < n; i++ {
		cancels[i] = bus.Subscribe(StateChanged, func(_ Event) {
			calls.Add(1)
		})
	}
	defer func() {
		for _, c := range cancels {
			c()
		}
	}()

	bus.Publish(Event{Kind: StateChanged})

	if got := calls.Load(); got != n {
		t.Fatalf("calls: want %d, got %d", n, got)
	}
}

// TestSubscribeAll — handler на ВСЕ kind'ы получает события любого вида.
func TestSubscribeAll(t *testing.T) {
	bus := NewMemoryBus()
	var got []EventKind
	cancel := bus.SubscribeAll(func(ev Event) {
		got = append(got, ev.Kind)
	})
	defer cancel()

	bus.Publish(Event{Kind: StateChanged})
	bus.Publish(Event{Kind: ConfigBuilt})
	bus.Publish(Event{Kind: PowerResume})

	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	want := []EventKind{StateChanged, ConfigBuilt, PowerResume}
	for i, k := range want {
		if got[i] != k {
			t.Fatalf("got[%d]: want %s, got %s", i, k, got[i])
		}
	}
}

// TestSubscribeAllAndKindSpecific — точечная подписка не блокирует SubscribeAll.
func TestSubscribeAllAndKindSpecific(t *testing.T) {
	bus := NewMemoryBus()
	var allCalls, kindCalls atomic.Int32

	cancelAll := bus.SubscribeAll(func(_ Event) { allCalls.Add(1) })
	defer cancelAll()
	cancelKind := bus.Subscribe(ConfigBuilt, func(_ Event) { kindCalls.Add(1) })
	defer cancelKind()

	bus.Publish(Event{Kind: ConfigBuilt})
	bus.Publish(Event{Kind: StateChanged})

	if got := allCalls.Load(); got != 2 {
		t.Fatalf("allCalls: want 2, got %d", got)
	}
	if got := kindCalls.Load(); got != 1 {
		t.Fatalf("kindCalls: want 1, got %d", got)
	}
}

// TestCancelStopsDelivery — после Cancel handler больше не вызывается.
func TestCancelStopsDelivery(t *testing.T) {
	bus := NewMemoryBus()
	var calls atomic.Int32
	cancel := bus.Subscribe(StateChanged, func(_ Event) {
		calls.Add(1)
	})

	bus.Publish(Event{Kind: StateChanged})
	cancel()
	bus.Publish(Event{Kind: StateChanged})

	if got := calls.Load(); got != 1 {
		t.Fatalf("calls: want 1 (only first publish), got %d", got)
	}
}

// TestCancelIdempotent — повторный Cancel безопасен.
func TestCancelIdempotent(t *testing.T) {
	bus := NewMemoryBus()
	cancel := bus.Subscribe(ConfigBuilt, func(_ Event) {})

	cancel()
	cancel() // не должно паниковать
	cancel() // и так далее
}

// TestCancelInsideHandler — handler может вызвать собственный Cancel,
// и следующий Publish уже не доставит.
func TestCancelInsideHandler(t *testing.T) {
	bus := NewMemoryBus()
	var calls atomic.Int32
	var cancel Cancel
	cancel = bus.Subscribe(StateChanged, func(_ Event) {
		calls.Add(1)
		cancel()
	})

	bus.Publish(Event{Kind: StateChanged}) // вызовет handler один раз и сразу отпишет
	bus.Publish(Event{Kind: StateChanged}) // никого

	if got := calls.Load(); got != 1 {
		t.Fatalf("calls: want 1, got %d", got)
	}
}

// TestPanicInHandlerDoesNotBreakOtherHandlers — panic в одном не ломает остальных.
func TestPanicInHandlerDoesNotBreakOtherHandlers(t *testing.T) {
	bus := NewMemoryBus()
	var goodCalls atomic.Int32

	cancelBad := bus.Subscribe(StateChanged, func(_ Event) {
		panic("boom")
	})
	defer cancelBad()
	cancelGood := bus.Subscribe(StateChanged, func(_ Event) {
		goodCalls.Add(1)
	})
	defer cancelGood()

	// Publish не должен паниковать
	bus.Publish(Event{Kind: StateChanged})
	bus.Publish(Event{Kind: StateChanged})

	if got := goodCalls.Load(); got != 2 {
		t.Fatalf("goodCalls: want 2, got %d", got)
	}
}

// TestNilHandlerSubscribeReturnsNoopCancel — nil handler не валит, возвращает безопасный Cancel.
func TestNilHandlerSubscribeReturnsNoopCancel(t *testing.T) {
	bus := NewMemoryBus()

	cancel1 := bus.Subscribe(ConfigBuilt, nil)
	cancel2 := bus.SubscribeAll(nil)

	// Publish не должен паниковать
	bus.Publish(Event{Kind: ConfigBuilt})

	cancel1()
	cancel2()
}

// TestConcurrentSubscribePublish — race-detector smoke на параллельной работе.
func TestConcurrentSubscribePublish(t *testing.T) {
	bus := NewMemoryBus()
	var wg sync.WaitGroup
	const writers, subscribers = 10, 10

	// subscribers
	for i := 0; i < subscribers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				cancel := bus.Subscribe(StateChanged, func(_ Event) {})
				cancel()
			}
		}()
	}
	// publishers
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				bus.Publish(Event{Kind: StateChanged})
			}
		}()
	}
	wg.Wait()
}

// TestPublishWithoutSubscribers — не паникует, ничего не делает.
func TestPublishWithoutSubscribers(t *testing.T) {
	bus := NewMemoryBus()
	bus.Publish(Event{Kind: PowerResume})
}

// TestPayloadTypeAssertion — payload приводится к ожидаемому типу.
func TestPayloadTypeAssertion(t *testing.T) {
	bus := NewMemoryBus()
	var p SubscriptionUpdatedPayload
	cancel := bus.Subscribe(SubscriptionUpdated, func(ev Event) {
		p, _ = ev.Payload.(SubscriptionUpdatedPayload)
	})
	defer cancel()

	bus.Publish(Event{
		Kind: SubscriptionUpdated,
		Payload: SubscriptionUpdatedPayload{
			SourceTag: "test-source",
			Succeeded: 42,
			Failed:    1,
		},
	})

	if p.SourceTag != "test-source" || p.Succeeded != 42 || p.Failed != 1 {
		t.Fatalf("payload not delivered correctly: %+v", p)
	}
}

// TestEventKindString — все известные Kind имеют человекочитаемые имена.
func TestEventKindString(t *testing.T) {
	cases := []struct {
		k    EventKind
		want string
	}{
		{StateChanged, "StateChanged"},
		{ConfigBuilt, "ConfigBuilt"},
		{SubscriptionUpdated, "SubscriptionUpdated"},
		{VpnStateChanged, "VpnStateChanged"},
		{ProxyActiveChanged, "ProxyActiveChanged"},
		{PowerResume, "PowerResume"},
		{AutoUpdateStatus, "AutoUpdateStatus"},
		{EventKind(9999), "Unknown"},
	}
	for _, c := range cases {
		if got := c.k.String(); got != c.want {
			t.Fatalf("Kind(%d).String(): want %q, got %q", c.k, c.want, got)
		}
	}
}
