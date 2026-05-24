package outboundutil

import "testing"

func TestApplyOutboundToRule_Reject(t *testing.T) {
	r := map[string]interface{}{"outbound": "old", "domain_suffix": []interface{}{"x"}}
	got := ApplyOutboundToRule(r, "reject")
	if got["action"] != "reject" {
		t.Errorf("action: %v", got["action"])
	}
	if _, has := got["outbound"]; has {
		t.Errorf("outbound should be removed: %v", got)
	}
	if _, has := got["method"]; has {
		t.Errorf("method should not exist: %v", got)
	}
}

func TestApplyOutboundToRule_Drop(t *testing.T) {
	r := map[string]interface{}{"outbound": "old"}
	got := ApplyOutboundToRule(r, "drop")
	if got["action"] != "reject" || got["method"] != "drop" {
		t.Errorf("expected action=reject + method=drop, got %v", got)
	}
	if _, has := got["outbound"]; has {
		t.Errorf("outbound should be removed")
	}
}

func TestApplyOutboundToRule_Regular(t *testing.T) {
	r := map[string]interface{}{"action": "stale", "method": "stale"}
	got := ApplyOutboundToRule(r, "proxy-out")
	if got["outbound"] != "proxy-out" {
		t.Errorf("outbound: %v", got["outbound"])
	}
	if _, has := got["action"]; has {
		t.Errorf("action should be removed: %v", got)
	}
	if _, has := got["method"]; has {
		t.Errorf("method should be removed: %v", got)
	}
}

func TestApplyOutboundToRule_EmptyOutboundNoop(t *testing.T) {
	r := map[string]interface{}{"outbound": "existing"}
	got := ApplyOutboundToRule(r, "")
	if got["outbound"] != "existing" {
		t.Errorf("empty outbound should not modify: %v", got)
	}
}

func TestApplyOutboundToRule_Nil(t *testing.T) {
	got := ApplyOutboundToRule(nil, "reject")
	if got != nil {
		t.Errorf("nil input → nil output: %v", got)
	}
}
