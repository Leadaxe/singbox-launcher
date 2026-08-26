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
		Type:               SourceTypeSubscription,
		Enabled:            true,
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
			ID: "01PROTON", Type: SourceTypeSubscription, Enabled: true, Label: "Proton NL",
			URL:                "https://example.invalid/proton",
			DetourNodeSourceID: "01WARP", DetourNodeTag: "hop",
		}},
		{"server", Source{
			ID: "01SRV", Type: SourceTypeServer, Enabled: true, Label: "Tokyo",
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

// Обратный синк (Save): ссылка и ULID возвращаются из legacy-формы в
// Connections. ID теперь приезжает полем, а не восстанавливается матчингом по
// URL — иначе правка адреса подписки выдавала бы ей новый ULID, и все ссылки
// на её узлы обрывались бы разом.
func TestSyncConnectionsFromLegacy_KeepsRefAndID(t *testing.T) {
	s := &State{}
	s.Connections.Sources = []Source{{
		ID: "01PROTON", Type: SourceTypeSubscription, Enabled: true, Label: "Proton NL",
		URL: "https://example.invalid/proton",
	}}
	syncLegacyFromConnections(s)

	// Пользователь поменял адрес подписки и выбрал хоп.
	s.ParserConfig.ParserConfig.Proxies[0].Source = "https://example.invalid/proton-v2"
	s.ParserConfig.ParserConfig.Proxies[0].DetourNodeSourceID = "01WARP"
	s.ParserConfig.ParserConfig.Proxies[0].DetourNodeTag = "hop"

	syncConnectionsFromLegacy(s)

	if n := len(s.Connections.Sources); n != 1 {
		t.Fatalf("источников %d, ожидался 1", n)
	}
	got := s.Connections.Sources[0]
	if got.ID != "01PROTON" {
		t.Errorf("ULID = %q — смена URL не должна выдавать источнику новый id", got.ID)
	}
	if got.Label != "Proton NL" {
		t.Errorf("подпись потеряна: %q", got.Label)
	}
	if got.DetourNodeSourceID != "01WARP" || got.DetourNodeTag != "hop" {
		t.Errorf("ссылка не вернулась: source_id=%q tag=%q", got.DetourNodeSourceID, got.DetourNodeTag)
	}
}
