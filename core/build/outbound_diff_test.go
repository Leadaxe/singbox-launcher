package build

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// === mapDiff ===============================================================

func TestMapDiff_ChangedAndNewKeys(t *testing.T) {
	form := map[string]interface{}{"a": "2", "c": "new"}
	base := map[string]interface{}{"a": "1", "b": "keep"}
	out := mapDiff(form, base)
	if out["a"] != "2" {
		t.Fatalf("expected changed a=2, got %#v", out["a"])
	}
	if out["c"] != "new" {
		t.Fatalf("expected new c=new, got %#v", out["c"])
	}
	if _, ok := out["b"]; ok {
		t.Fatalf("unchanged key b must not be in patch, got %#v", out)
	}
}

func TestMapDiff_EqualMapsProduceNil(t *testing.T) {
	m := map[string]interface{}{"a": "1", "n": 100}
	if out := mapDiff(m, map[string]interface{}{"a": "1", "n": 100}); out != nil {
		t.Fatalf("expected nil diff for equal maps, got %#v", out)
	}
}

// Регрессия issue #91: ключ, отсутствующий в form, НЕ пишется в patch как nil.
// Раньше это был tombstone "ключ удалён", который merge не понимал, а emitter
// превращал в "interval":null → sing-box: invalid duration "".
func TestMapDiff_MissingKeyNotTombstoned(t *testing.T) {
	form := map[string]interface{}{"url": "https://example.com"}
	base := map[string]interface{}{
		"url":       "https://cp.cloudflare.com/generate_204",
		"interval":  "5m",
		"tolerance": 100,
	}
	out := mapDiff(form, base)
	for _, k := range []string{"interval", "tolerance"} {
		if v, ok := out[k]; ok {
			t.Fatalf("key %q missing in form must not appear in patch, got %#v", k, v)
		}
	}
	if out["url"] != "https://example.com" {
		t.Fatalf("expected url override, got %#v", out["url"])
	}
}

// Форма, которая только убрала ключи, не даёт patch'а вообще.
func TestMapDiff_OnlyRemovedKeysProduceNil(t *testing.T) {
	form := map[string]interface{}{"interval": "5m"}
	base := map[string]interface{}{"interval": "5m", "tolerance": 100}
	if out := mapDiff(form, base); out != nil {
		t.Fatalf("expected nil diff, got %#v", out)
	}
}

// === OutboundFieldDiff =====================================================

// urltest → selector: diff не должен рождать null'ы для urltest-only ключей.
// Их отсутствие в selector'е обеспечивает edit_dialog (он их не копирует),
// а не tombstone в patch'е.
func TestOutboundFieldDiff_UrltestToSelectorNoNullOptions(t *testing.T) {
	base := configtypes.OutboundConfig{
		Tag: "auto", Type: "urltest",
		Options: map[string]interface{}{
			"url":                         "https://cp.cloudflare.com/generate_204",
			"interval":                    "5m",
			"tolerance":                   100,
			"interrupt_exist_connections": true,
		},
	}
	form := configtypes.OutboundConfig{
		Tag: "auto", Type: "selector",
		Options: map[string]interface{}{"interrupt_exist_connections": true},
	}
	patch := OutboundFieldDiff(form, base)
	opts, _ := patch["options"].(map[string]interface{})
	for k, v := range opts {
		if v == nil {
			t.Fatalf("patch option %q is nil — tombstone regressed (issue #91): %#v", k, opts)
		}
	}
}

func TestOutboundFieldDiff_NoChangesProducesNil(t *testing.T) {
	ob := configtypes.OutboundConfig{
		Tag: "auto", Type: "urltest",
		Options: map[string]interface{}{"interval": "5m"},
		Comment: "same",
	}
	if patch := OutboundFieldDiff(ob, ob); patch != nil {
		t.Fatalf("expected nil patch for identical configs, got %#v", patch)
	}
}
