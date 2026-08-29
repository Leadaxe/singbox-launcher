package backup

// SPEC 117 §5.B / риск Р3 — импорт бэкапа тоже создатель Source. Обратный
// синк Save, доминтовывавший пустые ID, упразднён: чужой/рукописный бэкап без
// id обязан получить ULID прямо на импорте, а приехавший id — сохраниться.

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/core/state"
)

func TestImport_MintsULIDsForSourcesWithoutID(t *testing.T) {
	s := state.New()
	b := &Backup{
		LxBackup: FormatVersion,
		Subscriptions: []Subscription{
			{URL: "https://example.invalid/no-id"},
		},
		Servers: []Server{
			{URI: "vless://u@h.example:443#srv"},
		},
		Chains: []Chain{
			{Tag: "chain-1", Chain: &configtypes.SourceChain{Hops: []string{"a", "b"}}},
		},
	}

	if _, err := Import(s, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(s.Connections.Sources) != 3 {
		t.Fatalf("sources = %d, want 3", len(s.Connections.Sources))
	}
	seen := map[string]bool{}
	for _, src := range s.Connections.Sources {
		if len(src.ID) != 26 {
			t.Errorf("source %s imported without ULID: id=%q", src.Type, src.ID)
		}
		if seen[src.ID] {
			t.Errorf("duplicate ULID %q", src.ID)
		}
		seen[src.ID] = true
	}
}

func TestImport_KeepsExistingIDs(t *testing.T) {
	s := state.New()
	b := &Backup{
		LxBackup: FormatVersion,
		Subscriptions: []Subscription{
			{ID: "01J00000000000000000000KEEP", URL: "https://example.invalid/with-id"},
		},
	}
	if _, err := Import(s, b, ImportOptions{}); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(s.Connections.Sources) != 1 || s.Connections.Sources[0].ID != "01J00000000000000000000KEEP" {
		t.Fatalf("imported ID was not preserved: %+v", s.Connections.Sources)
	}
}
