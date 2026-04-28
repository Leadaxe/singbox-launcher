package services

import (
	"sync"
	"sync/atomic"
	"testing"

	"singbox-launcher/core/events"
	"singbox-launcher/core/state"
)

// TestCacheStale_SetClearReadback — базовая жизнь маркера.
func TestCacheStale_SetClearReadback(t *testing.T) {
	s := NewStateService()
	if s.IsCacheStale() {
		t.Fatalf("default CacheStale must be false")
	}
	s.MarkCacheStale()
	if !s.IsCacheStale() {
		t.Fatalf("after Mark, CacheStale must be true")
	}
	s.ClearCacheStale()
	if s.IsCacheStale() {
		t.Fatalf("after Clear, CacheStale must be false")
	}
}

// TestConfigStale_SetClearReadback — то же для ConfigStale.
func TestConfigStale_SetClearReadback(t *testing.T) {
	s := NewStateService()
	if s.IsConfigStale() {
		t.Fatalf("default ConfigStale must be false")
	}
	s.MarkConfigStale()
	if !s.IsConfigStale() {
		t.Fatalf("after Mark, ConfigStale must be true")
	}
	s.ClearConfigStale()
	if s.IsConfigStale() {
		t.Fatalf("after Clear, ConfigStale must be false")
	}
}

// TestApplyDiff_NoOpOnEmpty — пустой Diff не поднимает флагов.
func TestApplyDiff_NoOpOnEmpty(t *testing.T) {
	s := NewStateService()
	s.ApplyDiff(state.Diff{})
	if s.IsCacheStale() || s.IsConfigStale() {
		t.Fatalf("empty diff must not raise flags")
	}
}

// TestApplyDiff_ParserOnly — изменения только в parser-конфиге → CacheStale.
func TestApplyDiff_ParserOnly(t *testing.T) {
	s := NewStateService()
	s.ApplyDiff(state.Diff{ProxiesChanged: true})
	if !s.IsCacheStale() {
		t.Fatalf("ProxiesChanged must raise CacheStale")
	}
	if s.IsConfigStale() {
		t.Fatalf("ProxiesChanged must NOT raise ConfigStale")
	}
}

// TestApplyDiff_TemplateOnly — изменения шаблона → ConfigStale.
func TestApplyDiff_TemplateOnly(t *testing.T) {
	s := NewStateService()
	s.ApplyDiff(state.Diff{VarsChanged: true})
	if !s.IsConfigStale() {
		t.Fatalf("VarsChanged must raise ConfigStale")
	}
	if s.IsCacheStale() {
		t.Fatalf("VarsChanged must NOT raise CacheStale")
	}
}

// TestApplyDiff_BothFlags — одновременное изменение поднимает оба.
func TestApplyDiff_BothFlags(t *testing.T) {
	s := NewStateService()
	s.ApplyDiff(state.Diff{ProxiesChanged: true, DNSOptionsChanged: true})
	if !s.IsCacheStale() {
		t.Fatalf("CacheStale expected")
	}
	if !s.IsConfigStale() {
		t.Fatalf("ConfigStale expected")
	}
}

// TestCacheStale_PublishesEventOnTransition — переход false→true публикует
// StateChanged ровно один раз, повторный Mark — не публикует.
func TestCacheStale_PublishesEventOnTransition(t *testing.T) {
	bus := events.NewMemoryBus()
	var calls atomic.Int32
	cancel := bus.Subscribe(events.StateChanged, func(_ events.Event) {
		calls.Add(1)
	})
	defer cancel()

	s := NewStateService()
	s.EventBus = bus

	s.MarkCacheStale()
	s.MarkCacheStale() // повторно — не должен публиковаться
	s.MarkCacheStale()
	if got := calls.Load(); got != 1 {
		t.Fatalf("StateChanged calls: want 1, got %d", got)
	}

	s.ClearCacheStale()
	if got := calls.Load(); got != 2 {
		t.Fatalf("after Clear: want 2 (set + clear), got %d", got)
	}
}

// TestConfigStale_PublishesEventOnTransition — параллельная проверка.
func TestConfigStale_PublishesEventOnTransition(t *testing.T) {
	bus := events.NewMemoryBus()
	var calls atomic.Int32
	cancel := bus.Subscribe(events.StateChanged, func(_ events.Event) {
		calls.Add(1)
	})
	defer cancel()

	s := NewStateService()
	s.EventBus = bus

	s.MarkConfigStale()
	s.ClearConfigStale()
	if got := calls.Load(); got != 2 {
		t.Fatalf("want 2 events (mark + clear), got %d", got)
	}
}

// TestNilEventBus_NoPanic — отсутствующий bus не валит маркеры.
func TestNilEventBus_NoPanic(t *testing.T) {
	s := NewStateService()
	s.EventBus = nil

	s.MarkCacheStale()
	s.ClearCacheStale()
	s.MarkConfigStale()
	s.ClearConfigStale()
	// если дошли — ОК
}

// TestDirtyFlags_Concurrency — race-detector smoke.
func TestDirtyFlags_Concurrency(t *testing.T) {
	s := NewStateService()
	bus := events.NewMemoryBus()
	s.EventBus = bus

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			s.MarkCacheStale()
			_ = s.IsCacheStale()
			s.ClearCacheStale()
		}()
		go func() {
			defer wg.Done()
			s.MarkConfigStale()
			_ = s.IsConfigStale()
			s.ClearConfigStale()
		}()
	}
	wg.Wait()
}
