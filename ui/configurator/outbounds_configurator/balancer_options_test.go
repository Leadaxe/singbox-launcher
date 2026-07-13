package outbounds_configurator

import (
	"reflect"
	"testing"
)

// SPEC 088: balancer options round-trip between Options map and the flat UI state.

func TestBuildBalancerOptions_LeastTestStripsBoth(t *testing.T) {
	opts := map[string]interface{}{"mode": "round_robin", "balancer": map[string]interface{}{"pool": 3}, "url": "x"}
	buildBalancerOptions(opts, balancerFormState{RoundRobin: false})
	if _, ok := opts["mode"]; ok {
		t.Error("least_test must strip mode")
	}
	if _, ok := opts["balancer"]; ok {
		t.Error("least_test must strip balancer")
	}
	if opts["url"] != "x" {
		t.Error("must not touch unrelated keys")
	}
}

func TestBuildBalancerOptions_RoundRobinEmits(t *testing.T) {
	opts := map[string]interface{}{}
	buildBalancerOptions(opts, balancerFormState{
		RoundRobin: true, Pool: "3", PoolTolerance: "30",
		Sticky: map[string]bool{"process": true, "domain": true},
	})
	if opts["mode"] != "round_robin" {
		t.Fatalf("mode = %v, want round_robin", opts["mode"])
	}
	bal, ok := opts["balancer"].(map[string]interface{})
	if !ok {
		t.Fatal("balancer missing")
	}
	if bal["pool"] != 3 {
		t.Errorf("pool = %v, want 3", bal["pool"])
	}
	if bal["pool_tolerance"] != 30 {
		t.Errorf("pool_tolerance = %v, want 30", bal["pool_tolerance"])
	}
	// sticky_hash preserves UI order (process, domain).
	if got := bal["sticky_hash"].([]interface{}); !reflect.DeepEqual(got, []interface{}{"process", "domain"}) {
		t.Errorf("sticky_hash = %v, want [process domain]", got)
	}
}

func TestBuildBalancerOptions_NoStickyEmitsNoneSentinel(t *testing.T) {
	opts := map[string]interface{}{}
	buildBalancerOptions(opts, balancerFormState{RoundRobin: true, Pool: "2"})
	bal := opts["balancer"].(map[string]interface{})
	if got := bal["sticky_hash"].([]interface{}); !reflect.DeepEqual(got, []interface{}{"none"}) {
		t.Errorf("empty sticky must emit [none] (core drops bare-empty), got %v", got)
	}
}

func TestBuildBalancerOptions_PoolToleranceClampedToUint16(t *testing.T) {
	opts := map[string]interface{}{}
	buildBalancerOptions(opts, balancerFormState{RoundRobin: true, PoolTolerance: "99999"})
	bal := opts["balancer"].(map[string]interface{})
	if bal["pool_tolerance"] != 65535 {
		t.Errorf("pool_tolerance = %v, want clamped 65535", bal["pool_tolerance"])
	}
}

func TestBuildBalancerOptions_InvalidPoolOmitted(t *testing.T) {
	opts := map[string]interface{}{}
	buildBalancerOptions(opts, balancerFormState{RoundRobin: true, Pool: "abc", PoolTolerance: ""})
	bal := opts["balancer"].(map[string]interface{})
	if _, ok := bal["pool"]; ok {
		t.Error("non-numeric pool must be omitted")
	}
	if _, ok := bal["pool_tolerance"]; ok {
		t.Error("empty pool_tolerance must be omitted")
	}
}

func TestParseBalancerFromOptions_RoundTrip(t *testing.T) {
	orig := map[string]interface{}{
		"mode": "round_robin",
		"balancer": map[string]interface{}{
			"pool":           5,
			"pool_tolerance": 40,
			"sticky_hash":    []interface{}{"source_ip", "dest_port"},
		},
	}
	st := parseBalancerFromOptions(orig)
	if !st.RoundRobin || st.Pool != "5" || st.PoolTolerance != "40" {
		t.Fatalf("parse mismatch: %+v", st)
	}
	if !st.Sticky["source_ip"] || !st.Sticky["dest_port"] || st.Sticky["process"] {
		t.Errorf("sticky mismatch: %v", st.Sticky)
	}
	// Re-emit and compare the meaningful fields.
	out := map[string]interface{}{}
	buildBalancerOptions(out, st)
	bal := out["balancer"].(map[string]interface{})
	if bal["pool"] != 5 || bal["pool_tolerance"] != 40 {
		t.Errorf("round-trip lost pool/tolerance: %v", bal)
	}
	if got := bal["sticky_hash"].([]interface{}); !reflect.DeepEqual(got, []interface{}{"source_ip", "dest_port"}) {
		t.Errorf("round-trip sticky = %v", got)
	}
}

func TestParseBalancerFromOptions_NoneSentinelUnchecksAll(t *testing.T) {
	st := parseBalancerFromOptions(map[string]interface{}{
		"mode":     "round_robin",
		"balancer": map[string]interface{}{"sticky_hash": []interface{}{"none"}},
	})
	if len(st.Sticky) != 0 {
		t.Errorf("[none] sentinel must leave all unchecked, got %v", st.Sticky)
	}
}

func TestParseBalancerFromOptions_LeastTestOrNil(t *testing.T) {
	if st := parseBalancerFromOptions(nil); st.RoundRobin {
		t.Error("nil options must be least_test")
	}
	if st := parseBalancerFromOptions(map[string]interface{}{"mode": "least_test"}); st.RoundRobin {
		t.Error("least_test mode must not be round_robin")
	}
}
