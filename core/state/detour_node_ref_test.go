package state

import (
	"encoding/json"
	"testing"
)

// SPEC 112-A — ссылка на узел как объект «source_id + identity-тег»: она
// обязана целиком переживать и запись state.json, и оба адаптера
// (Connections ↔ legacy ProxySource), иначе сборка получит половину ссылки и
// уронит зависимый источник fail-closed.

func TestDetourNodeRefSurvivesJSONRoundTrip(t *testing.T) {
	src := Source{
		ID:                 "01PROTON0000000000000000",
		Node:               Node{Kind: SourceKindSubscription, Enabled: true},
		Label:              "Proton NL",
		URL:                "https://example.invalid/proton",
		DetourNodeSourceID: "01WARP00000000000000000",
		DetourNodeTag:      "🔥🎭 WARP (MASQUE)",
		DetourNodeLabel:    "WARP hop",
	}

	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var restored Source
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if restored.DetourNodeSourceID != src.DetourNodeSourceID || restored.DetourNodeTag != src.DetourNodeTag {
		t.Fatalf("ссылка потеряна на round-trip: %+v", restored)
	}
}

// Обе половины ссылки и ULID самого источника доезжают до сборочной формы:
// без ID резолвить ссылку не по чему, без тега — нечего искать.
func TestDetourNodeRefReachesProxySource(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  Source
	}{
		{"subscription", Source{
			ID: "01PROTON", Node: Node{Kind: SourceKindSubscription, Enabled: true}, Label: "Proton NL",
			URL:                "https://example.invalid/proton",
			DetourNodeSourceID: "01WARP", DetourNodeTag: "hop",
		}},
		{"server", Source{
			ID: "01SRV", Node: Node{Kind: SourceKindServer, Enabled: true}, Label: "Tokyo",
			URI: "vless://u@h:443", NodeTag: "🇯🇵 Tokyo",
			DetourNodeSourceID: "01WARP", DetourNodeTag: "hop",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ps := tc.src.ToProxySourceV4()
			if ps.ID != tc.src.ID {
				t.Errorf("ID = %q, ожидался %q — резолвить ссылку не по чему", ps.ID, tc.src.ID)
			}
			if ps.DetourNodeSourceID != "01WARP" || ps.DetourNodeTag != "hop" {
				t.Errorf("ссылка не доехала: source_id=%q tag=%q", ps.DetourNodeSourceID, ps.DetourNodeTag)
			}
			// Подпись едет только ради текстов диагностики (SPEC 112-A).
			if ps.Label != tc.src.Label {
				t.Errorf("Label = %q, ожидался %q", ps.Label, tc.src.Label)
			}
		})
	}
}

// SPEC 117 (W4): TestSyncConnectionsFromLegacy_KeepsRefAndID удалён — предмет
// теста (обратный синк syncConnectionsFromLegacy) упразднён этим этапом. ID
// живёт в canonical и не пересоздаётся вовсе; инвариант ID-стабильности
// закрывает canonical_roundtrip_test.go (TestCanonical_IDStability).
