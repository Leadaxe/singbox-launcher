package state

import "testing"

// Ручной config_json (Source.ConfigJSON) обязан переживать оба направления
// Source ↔ ProxySource: syncConnectionsFromLegacy пересобирает
// Connections.Sources на каждом Save, и без прокидки поле молча терялось бы
// при первом же сохранении.
func TestConfigJSON_ToProxySourceV4(t *testing.T) {
	raw := []byte(`{"type":"someproto","server":"10.0.0.1"}`)
	srv := Source{Type: SourceTypeServer, Enabled: true, URI: "x://y", ConfigJSON: raw}
	if got := string(srv.ToProxySourceV4().ConfigJSON); got != string(raw) {
		t.Errorf("ConfigJSON = %q, want %q", got, raw)
	}
}

func TestConfigJSON_LegacyRoundTrip(t *testing.T) {
	raw := `{"type":"someproto","server":"10.0.0.1"}`
	s := &State{}
	s.Connections.Sources = []Source{
		{ID: "with-uri", Type: SourceTypeServer, Enabled: true, URI: "vless://u@h:443#a", ConfigJSON: []byte(raw)},
		// Главный кейс фичи: протокол без URI-схемы — только ручной JSON.
		{ID: "json-only", Type: SourceTypeServer, Enabled: true, ConfigJSON: []byte(raw)},
	}

	syncLegacyFromConnections(s)
	if n := len(s.ParserConfig.ParserConfig.Proxies); n != 2 {
		t.Fatalf("proxies = %d, want 2", n)
	}
	for i, p := range s.ParserConfig.ParserConfig.Proxies {
		if string(p.ConfigJSON) != raw {
			t.Errorf("legacy proxy %d ConfigJSON = %q, want %q", i, p.ConfigJSON, raw)
		}
	}

	syncConnectionsFromLegacy(s)
	if n := len(s.Connections.Sources); n != 2 {
		t.Fatalf("sources after round-trip = %d, want 2", n)
	}
	for _, src := range s.Connections.Sources {
		if string(src.ConfigJSON) != raw {
			t.Errorf("round-trip source %q ConfigJSON = %q, want %q", src.ID, src.ConfigJSON, raw)
		}
	}
	// ID стабильны: URI-source матчится по URI, URI-less — по телу JSON.
	// Без матчинга json-only источник получал бы новый ULID на каждом Save.
	ids := map[string]bool{}
	for _, src := range s.Connections.Sources {
		ids[src.ID] = true
	}
	if !ids["with-uri"] || !ids["json-only"] {
		t.Errorf("IDs must survive the round-trip, got %v", ids)
	}
}
