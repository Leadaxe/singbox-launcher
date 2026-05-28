// File clash_remote_test.go — SPEC 064 unit tests.
package ui

import (
	"sync"
	"testing"
)

func resetOverrideForTest(t *testing.T) {
	t.Helper()
	// Reset to clean state. Tests могут запускаться в любом порядке.
	remoteOverrideMu.Lock()
	remoteOverrideActive = false
	remoteOverrideValue = RemoteOverride{}
	remoteOverrideMu.Unlock()
	// Reset listeners — иначе тесты «протекают» друг в друга через listeners.
	overrideListenersMu.Lock()
	overrideListeners = nil
	overrideListenersMu.Unlock()
}

// TestGetRemoteOverride_DefaultInactive — без Set'а GetRemoteOverride
// возвращает (zero, false).
func TestGetRemoteOverride_DefaultInactive(t *testing.T) {
	resetOverrideForTest(t)
	ov, ok := GetRemoteOverride()
	if ok {
		t.Errorf("expected inactive override, got ok=true (value=%+v)", ov)
	}
}

// TestSetThenGetRemoteOverride — Set возвращается из Get.
func TestSetThenGetRemoteOverride(t *testing.T) {
	resetOverrideForTest(t)
	want := RemoteOverride{Host: "192.168.10.1", Port: 9090, Secret: "abc"}
	SetRemoteOverride(want)

	got, ok := GetRemoteOverride()
	if !ok {
		t.Fatalf("expected active override after Set")
	}
	if got != want {
		t.Errorf("Get != Set: got %+v, want %+v", got, want)
	}
}

// TestClearRemoteOverride_ResetsToInactive — Set → Clear → inactive.
func TestClearRemoteOverride_ResetsToInactive(t *testing.T) {
	resetOverrideForTest(t)
	SetRemoteOverride(RemoteOverride{Host: "x", Port: 1, Secret: "y"})
	ClearRemoteOverride()
	if _, ok := GetRemoteOverride(); ok {
		t.Error("expected inactive after Clear")
	}
}

// TestGenerationMonotonic — каждый Set/Clear bump'ает generation, никогда
// не повторяется.
func TestGenerationMonotonic(t *testing.T) {
	resetOverrideForTest(t)
	g0 := CurrentGeneration()

	SetRemoteOverride(RemoteOverride{Host: "a", Port: 1})
	g1 := CurrentGeneration()
	if g1 <= g0 {
		t.Errorf("expected g1 > g0, got g0=%d g1=%d", g0, g1)
	}

	ClearRemoteOverride()
	g2 := CurrentGeneration()
	if g2 <= g1 {
		t.Errorf("expected g2 > g1, got g1=%d g2=%d", g1, g2)
	}

	SetRemoteOverride(RemoteOverride{Host: "b", Port: 2})
	g3 := CurrentGeneration()
	if g3 <= g2 {
		t.Errorf("expected g3 > g2, got g2=%d g3=%d", g2, g3)
	}
}

// TestOnOverrideChanged_FiresOnSetAndClear — listener вызывается на оба события.
func TestOnOverrideChanged_FiresOnSetAndClear(t *testing.T) {
	resetOverrideForTest(t)
	var mu sync.Mutex
	calls := 0
	OnOverrideChanged(func() {
		mu.Lock()
		calls++
		mu.Unlock()
	})
	SetRemoteOverride(RemoteOverride{Host: "x", Port: 1})
	ClearRemoteOverride()

	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Errorf("expected 2 listener calls (Set + Clear), got %d", calls)
	}
}

// TestEffectiveClashAPIConfig_NilAC — defensive: nil controller не паникует.
func TestEffectiveClashAPIConfig_NilAC(t *testing.T) {
	resetOverrideForTest(t)
	base, tok, en, rem := EffectiveClashAPIConfig(nil)
	if base != "" || tok != "" || en || rem {
		t.Errorf("nil AC + no override should return (\"\",\"\",false,false), got (%q,%q,%v,%v)",
			base, tok, en, rem)
	}
}

// TestEffectiveClashAPIConfig_RemoteOverride — override active → возвращает
// remote значения с remote=true.
func TestEffectiveClashAPIConfig_RemoteOverride(t *testing.T) {
	resetOverrideForTest(t)
	SetRemoteOverride(RemoteOverride{Host: "1.2.3.4", Port: 9090, Secret: "s"})

	base, tok, en, rem := EffectiveClashAPIConfig(nil) // nil AC OK because override короткирует
	if base != "http://1.2.3.4:9090" {
		t.Errorf("baseURL: %q", base)
	}
	if tok != "s" {
		t.Errorf("token: %q", tok)
	}
	if !en {
		t.Error("enabled should be true with override")
	}
	if !rem {
		t.Error("remote flag should be true with override")
	}
}

// TestNormalizeHost — happy path + edge cases в одной table-driven форме.
func TestNormalizeHost(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		want      string
		expectErr bool
	}{
		{"plain hostname", "example.com", "example.com", false},
		{"plain IPv4", "192.168.10.1", "192.168.10.1", false},
		{"strip http scheme", "http://192.168.10.1", "192.168.10.1", false},
		{"strip https scheme", "https://router.lan", "router.lan", false},
		{"strip trailing slash", "http://x/", "x", false},
		{"trim whitespace", "  example.com  ", "example.com", false},
		{"trim mixed", "  http://x/  ", "x", false},
		{"empty", "", "", true},
		{"whitespace-only", "   ", "", true},
		{"contains slash (path)", "host/api", "", true},
		{"contains slash after scheme strip", "http://host/api", "", true},
		{"contains colon (port mistake)", "host:9090", "", true},
		{"contains colon after scheme", "http://host:9090", "", true},
		{"IPv6 bracket form", "[::1]", "", true},
		{"IPv6 bracket with port-ish", "[fe80::1]", "", true},
		{"uppercase scheme", "HTTP://X", "X", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeHost(tc.in)
			if tc.expectErr {
				if err == nil {
					t.Errorf("expected error for %q, got nil (result=%q)", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for %q: %v", tc.in, err)
				return
			}
			if got != tc.want {
				t.Errorf("NormalizeHost(%q): got %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
