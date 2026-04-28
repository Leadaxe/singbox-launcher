package services

import (
	"sync"
	"testing"
)

func TestAutoPingAfterConnectDefaultEnabled(t *testing.T) {
	s := NewStateService()
	if !s.IsAutoPingAfterConnectEnabled() {
		t.Error("auto-ping should default to enabled")
	}
	s.SetAutoPingAfterConnectEnabled(false)
	if s.IsAutoPingAfterConnectEnabled() {
		t.Error("auto-ping should be disabled after Set(false)")
	}
}

func TestAutoPingMaxProxiesDefault(t *testing.T) {
	s := NewStateService()
	if got := s.GetAutoPingMaxProxies(); got != DefaultAutoPingMaxProxies {
		t.Errorf("default max-proxies = %d, want %d", got, DefaultAutoPingMaxProxies)
	}
}

func TestAutoPingMaxProxiesOverride(t *testing.T) {
	s := NewStateService()
	s.SetAutoPingMaxProxies(300)
	if got := s.GetAutoPingMaxProxies(); got != 300 {
		t.Errorf("after Set(300), got %d, want 300", got)
	}
	s.SetAutoPingMaxProxies(0)
	if got := s.GetAutoPingMaxProxies(); got != 0 {
		t.Errorf("after Set(0) (no cap), got %d, want 0", got)
	}
	// Negative clamps to 0 — defensive guard so a malformed settings.json
	// can't put the field in a confusing state.
	s.SetAutoPingMaxProxies(-1)
	if got := s.GetAutoPingMaxProxies(); got != 0 {
		t.Errorf("after Set(-1), got %d, want 0 (clamped)", got)
	}
}

func TestRecordUpdateSuccess(t *testing.T) {
	s := NewStateService()

	s.LastUpdateMutex.RLock()
	succ := s.LastUpdateSucceededAt
	s.LastUpdateMutex.RUnlock()
	if !succ.IsZero() {
		t.Errorf("fresh state should have zero LastUpdateSucceededAt, got %v", succ)
	}

	s.RecordUpdateSuccess()
	s.LastUpdateMutex.RLock()
	succ = s.LastUpdateSucceededAt
	s.LastUpdateMutex.RUnlock()
	if succ.IsZero() {
		t.Error("LastUpdateSucceededAt should be non-zero after RecordUpdateSuccess")
	}
}

// Stress the success / concurrent-flag-update surface: if any mutex is wrong,
// -race will bark. The real defence is go test -race on CI.
func TestStateServiceConcurrency(t *testing.T) {
	s := NewStateService()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				s.RecordUpdateSuccess()
			}
			s.SetAutoPingAfterConnectEnabled(i%2 == 0)
			_ = s.IsAutoPingAfterConnectEnabled()
			if i%2 == 0 {
				s.MarkCacheStale()
			} else {
				s.ClearCacheStale()
			}
			_ = s.IsCacheStale()
		}(i)
	}
	wg.Wait()
}
