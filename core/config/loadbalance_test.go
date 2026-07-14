package config

import (
	"strings"
	"testing"
)

// The generator emits urltest mode/balancer options via the generic Options
// loop; these tests lock the sing-box-lx load-balancing contract (SPEC 088 /
// core SPEC 019): round_robin + balancer pass through, the sticky_hash sentinel
// is respected, and a plain urltest stays clean.
func genSelector(t *testing.T, cfg OutboundConfig) string {
	t.Helper()
	out, err := GenerateSelectorWithFilteredAddOutbounds(
		[]*ParsedNode{{Tag: "n1"}, {Tag: "n2"}},
		cfg,
		map[string]*outboundInfo{},
		false,
		nil,
	)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	return out
}

func TestGenerator_RoundRobinBalancer(t *testing.T) {
	cfg := OutboundConfig{
		Tag:  "auto",
		Type: "urltest",
		Options: map[string]interface{}{
			"url":  "https://cp.cloudflare.com/generate_204",
			"mode": "round_robin",
			"balancer": map[string]interface{}{
				"pool":           3,
				"pool_tolerance": 30,
				"sticky_hash":    []interface{}{"process", "domain"},
			},
		},
	}
	out := genSelector(t, cfg)
	for _, want := range []string{`"mode":"round_robin"`, `"balancer":`, `"pool":3`, `"sticky_hash":["process","domain"]`} {
		if !strings.Contains(out, want) {
			t.Errorf("emit missing %s\ngot: %s", want, out)
		}
	}
}

// The core treats an empty sticky_hash as "use default", NOT "off"; the
// generator must never emit a bare []  (only the explicit ["none"] disables it).
func TestGenerator_EmptyStickyHashDropped(t *testing.T) {
	cfg := OutboundConfig{
		Tag:  "auto",
		Type: "urltest",
		Options: map[string]interface{}{
			"mode": "round_robin",
			"balancer": map[string]interface{}{
				"pool":        3,
				"sticky_hash": []interface{}{},
			},
		},
	}
	out := genSelector(t, cfg)
	if strings.Contains(out, `"sticky_hash":[]`) {
		t.Errorf("bare empty sticky_hash must be dropped (sentinel contract)\ngot: %s", out)
	}
	if !strings.Contains(out, `"pool":3`) {
		t.Errorf("balancer body should survive with sticky_hash removed\ngot: %s", out)
	}
}

func TestGenerator_ExplicitNoneSentinelKept(t *testing.T) {
	cfg := OutboundConfig{
		Tag:  "auto",
		Type: "urltest",
		Options: map[string]interface{}{
			"mode":     "round_robin",
			"balancer": map[string]interface{}{"sticky_hash": []interface{}{"none"}},
		},
	}
	out := genSelector(t, cfg)
	if !strings.Contains(out, `"sticky_hash":["none"]`) {
		t.Errorf("explicit [none] sentinel must be preserved\ngot: %s", out)
	}
}

// A plain urltest (no mode/balancer) must not gain any load-balancing keys.
func TestGenerator_PlainUrltestUnchanged(t *testing.T) {
	cfg := OutboundConfig{
		Tag:     "auto",
		Type:    "urltest",
		Options: map[string]interface{}{"url": "https://x/y"},
	}
	out := genSelector(t, cfg)
	if strings.Contains(out, "mode") || strings.Contains(out, "balancer") {
		t.Errorf("plain urltest must not emit mode/balancer\ngot: %s", out)
	}
}

// Options keys emit in deterministic (sorted) order — guards against flaky
// byte-exact golden fixtures from Go's randomized map range.
func TestGenerator_DeterministicOptionOrder(t *testing.T) {
	cfg := OutboundConfig{
		Tag:  "auto",
		Type: "urltest",
		Options: map[string]interface{}{
			"mode":     "round_robin",
			"balancer": map[string]interface{}{"pool": 3},
			"url":      "https://x/y",
			"interval": "5m",
		},
	}
	first := genSelector(t, cfg)
	for i := 0; i < 20; i++ {
		if got := genSelector(t, cfg); got != first {
			t.Fatalf("non-deterministic emit at iter %d:\n%s\nvs\n%s", i, first, got)
		}
	}
}
