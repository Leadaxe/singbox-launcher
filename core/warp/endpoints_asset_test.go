package warp

import (
	"math/rand"
	"testing"
)

// Пулы загружаются из embedded assets/warp_endpoints.json в init(). Эти тесты
// лочат контракт: непустые пулы, masque-fallback, fail-loud на битом JSON,
// формат эндпоинта (prefix + 1..10 : port из пула).

func TestWarpPoolsLoadedNonEmpty(t *testing.T) {
	if len(endpointPrefixes) == 0 {
		t.Error("endpointPrefixes empty — asset not loaded")
	}
	if len(endpointPorts) == 0 {
		t.Error("endpointPorts empty — asset not loaded")
	}
	if len(SNIPool) == 0 {
		t.Error("SNIPool empty — asset not loaded")
	}
	if len(MasqueSNIPool) == 0 {
		t.Error("MasqueSNIPool empty — asset not loaded / fallback failed")
	}
}

// Свободный лоуэр-баунд, чтобы не ломаться когда LxBox дольёт домены/порты.
func TestWarpPoolsMinimumCounts(t *testing.T) {
	if len(endpointPrefixes) < 5 {
		t.Errorf("prefixes = %d, want >= 5", len(endpointPrefixes))
	}
	if len(endpointPorts) < 50 {
		t.Errorf("ports = %d, want >= 50", len(endpointPorts))
	}
	if len(SNIPool) < 12 {
		t.Errorf("sni_pool = %d, want >= 12", len(SNIPool))
	}
	if len(MasqueSNIPool) < 13 {
		t.Errorf("masque_sni_pool = %d, want >= 13", len(MasqueSNIPool))
	}
}

func TestResolveMasquePool_FallbackToSNI(t *testing.T) {
	sni := []string{"a.example", "b.example"}
	if got := resolveMasquePool(nil, sni); !sameSlice(got, sni) {
		t.Errorf("nil masque must fall back to sni, got %v", got)
	}
	if got := resolveMasquePool([]string{}, sni); !sameSlice(got, sni) {
		t.Errorf("empty masque must fall back to sni, got %v", got)
	}
	masque := []string{"m.example"}
	if got := resolveMasquePool(masque, sni); !sameSlice(got, masque) {
		t.Errorf("non-empty masque must be used, got %v", got)
	}
}

func TestMustParseWarpPools_PanicsOnGarbage(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("mustParseWarpPools must panic on malformed JSON")
		}
	}()
	mustParseWarpPools([]byte("{not json"))
}

// Порт из RandomEndpoint должен принадлежать пулу; хост — один из prefixes + 1..10.
func TestRandomEndpointPortInPool(t *testing.T) {
	portSet := make(map[int]bool, len(endpointPorts))
	for _, p := range endpointPorts {
		portSet[p] = true
	}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 50; i++ {
		e := RandomEndpoint(rng)
		_, port, err := splitHostPort(e)
		if err != nil {
			t.Fatalf("bad endpoint %q: %v", e, err)
		}
		if !portSet[port] {
			t.Errorf("endpoint %q port %d not in pool", e, port)
		}
	}
}

func sameSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
