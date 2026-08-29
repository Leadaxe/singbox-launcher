package state

import "testing"

// Ручной config_json (Source.ConfigJSON) обязан доезжать до прямой проекции
// Source → ProxySource: сборка вставляет его passthrough, и без прокидки
// узел молча терял бы тело.
//
// SPEC 117 (W4): TestConfigJSON_LegacyRoundTrip удалён — предмет теста
// (обратный синк syncConnectionsFromLegacy на Save) упразднён этим этапом.
// Canonical roundtrip и ID-стабильность закрывает canonical_roundtrip_test.go.
func TestConfigJSON_ToProxySourceV4(t *testing.T) {
	raw := []byte(`{"type":"someproto","server":"10.0.0.1"}`)
	srv := Source{Node: Node{Kind: SourceKindServer, Enabled: true}, URI: "x://y", ConfigJSON: raw}
	if got := string(srv.ToProxySourceV4().ConfigJSON); got != string(raw) {
		t.Errorf("ConfigJSON = %q, want %q", got, raw)
	}
}
