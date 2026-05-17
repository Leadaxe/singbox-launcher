package template

import (
	"strings"
	"testing"
)

// SPEC 055 — preset.outbounds validation tests.

func TestLoadPresets_OutboundsModeAdd(t *testing.T) {
	raw := []byte(`[
		{"id": "p1", "label": "P1",
		 "vars": [{"name": "out", "type": "outbound", "default": "direct-out"}],
		 "rule": {"outbound": "@out"},
		 "outbounds": [
		   {"tag": "my-selector", "type": "selector",
		    "options": {"default": "direct-out"}, "addOutbounds": ["direct-out"]}
		 ]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 1 {
		t.Fatalf("expected 1 preset, got %d (warnings: %+v)", len(ps), ws)
	}
	if len(ps[0].Outbounds) != 1 {
		t.Fatalf("expected 1 outbound, got %d", len(ps[0].Outbounds))
	}
	ob := ps[0].Outbounds[0]
	if ob.Mode != "add" {
		t.Errorf("expected mode normalized to 'add' (was empty), got %q", ob.Mode)
	}
	if ob.Tag != "my-selector" || ob.Type != "selector" {
		t.Errorf("unexpected outbound: %+v", ob)
	}
}

func TestLoadPresets_OutboundsModeUpdate(t *testing.T) {
	raw := []byte(`[
		{"id": "p1", "label": "P1",
		 "rule": {"outbound": "direct-out"},
		 "outbounds": [
		   {"mode": "update", "tag": "proxy-out",
		    "filters": {"tag": "!/RU/i"}, "addOutbounds": ["x"]}
		 ]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 1 || len(ps[0].Outbounds) != 1 {
		t.Fatalf("expected 1 preset+outbound, got ps=%d obs=%d warnings=%+v",
			len(ps), len(ps[0].Outbounds), ws)
	}
	ob := ps[0].Outbounds[0]
	if ob.Mode != "update" || ob.Tag != "proxy-out" {
		t.Errorf("unexpected outbound: %+v", ob)
	}
}

func TestLoadPresets_OutboundsModeAddRequiresType(t *testing.T) {
	raw := []byte(`[
		{"id": "p1", "label": "P1",
		 "rule": {"outbound": "direct-out"},
		 "outbounds": [{"tag": "no-type"}]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 1 {
		t.Fatalf("preset should survive (only entry stripped), got %d (warns: %+v)", len(ps), ws)
	}
	if len(ps[0].Outbounds) != 0 {
		t.Errorf("expected stripped (no type), got %+v", ps[0].Outbounds)
	}
	found := false
	for _, w := range ws {
		if strings.Contains(w.Message, "mode=add requires 'type'") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'mode=add requires type' warning, got: %+v", ws)
	}
}

func TestLoadPresets_OutboundsModeUpdateWithTypeWarning(t *testing.T) {
	raw := []byte(`[
		{"id": "p1", "label": "P1",
		 "rule": {"outbound": "direct-out"},
		 "outbounds": [
		   {"mode": "update", "tag": "proxy-out", "type": "shadowsocks"}
		 ]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 1 || len(ps[0].Outbounds) != 1 {
		t.Fatalf("expected outbound kept with warning, got %+v / warns=%+v", ps, ws)
	}
	// Type field stays in struct; expand-time drop is done in build pipeline.
	found := false
	for _, w := range ws {
		if strings.Contains(w.Message, "cannot change 'type'") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected type-change warning, got: %+v", ws)
	}
}

func TestLoadPresets_OutboundsUnknownMode(t *testing.T) {
	raw := []byte(`[
		{"id": "p1", "label": "P1",
		 "rule": {"outbound": "direct-out"},
		 "outbounds": [{"mode": "replace", "tag": "x", "type": "selector"}]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 1 {
		t.Fatalf("preset should survive, got %d", len(ps))
	}
	if len(ps[0].Outbounds) != 0 {
		t.Errorf("expected unknown-mode entry stripped, got %+v", ps[0].Outbounds)
	}
	found := false
	for _, w := range ws {
		if strings.Contains(w.Message, "unknown mode") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected unknown-mode warning: %+v", ws)
	}
}

func TestLoadPresets_OutboundsEmptyTag(t *testing.T) {
	raw := []byte(`[
		{"id": "p1", "label": "P1",
		 "rule": {"outbound": "direct-out"},
		 "outbounds": [{"type": "selector"}]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 1 || len(ps[0].Outbounds) != 0 {
		t.Errorf("expected empty-tag stripped: %+v / warns=%+v", ps, ws)
	}
	found := false
	for _, w := range ws {
		if strings.Contains(w.Message, "empty tag") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected empty-tag warning: %+v", ws)
	}
}

func TestLoadPresets_OutboundsDuplicateTagInPreset(t *testing.T) {
	raw := []byte(`[
		{"id": "p1", "label": "P1",
		 "rule": {"outbound": "direct-out"},
		 "outbounds": [
		   {"tag": "foo", "type": "selector"},
		   {"tag": "foo", "type": "urltest"}
		 ]}
	]`)
	ps, ws := LoadPresets(raw, nil)
	if len(ps) != 1 || len(ps[0].Outbounds) != 1 {
		t.Errorf("expected dedup to 1 entry, got %d / warns=%+v",
			len(ps[0].Outbounds), ws)
	}
	found := false
	for _, w := range ws {
		if strings.Contains(w.Message, "duplicated within preset") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected duplicate warning: %+v", ws)
	}
}
