package build

import (
	"encoding/json"
	"strings"
	"testing"
)

// SPEC 063: для удалённой машины путь .srs должен указывать в ЕЁ ресурс-стор.
//
// Раньше сюда уезжал путь файловой системы ЛАУНЧЕРА — на роутере такого нет,
// ядро не находило набор и не поднималось: apply проходил, инстанс падал.
func TestRemoteRuleSetPointsAtResourceStore(t *testing.T) {
	execDir := t.TempDir()
	stubSRSFile(t, execDir, "geosite-ru")

	const resourceDir = "/var/lib/sing-box-lxd/state/resources"
	in := json.RawMessage(`{"tag":"geosite-ru","type":"remote","url":"https://example/geosite-ru.srs"}`)

	got, err := convertRuleSetToLocalRequired(in, execDir, resourceDir)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	m := got.(map[string]interface{})
	if m["type"] != "local" {
		t.Errorf("type must be local, got %v", m["type"])
	}
	want := resourceDir + "/" + ResourceNameForSRS("geosite-ru")
	if m["path"] != want {
		t.Errorf("path: want %s, got %v", want, m["path"])
	}
	if strings.Contains(m["path"].(string), execDir) {
		t.Errorf("remote config must not carry the launcher's own path: %v", m["path"])
	}
}

// Локальная машина не меняется: путь по-прежнему свой, из bin/rule-sets/.
func TestLocalRuleSetKeepsOwnPath(t *testing.T) {
	execDir := t.TempDir()
	want := stubSRSFile(t, execDir, "geosite-ru")

	in := json.RawMessage(`{"tag":"geosite-ru","type":"remote","url":"https://example/geosite-ru.srs"}`)
	got, err := convertRuleSetToLocalRequired(in, execDir, "")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if path := got.(map[string]interface{})["path"]; path != want {
		t.Errorf("local path: want %s, got %v", want, path)
	}
}

// Для чужой машины уже-local запись не проверяется на НАШЕЙ файловой системе:
// файла тут нет по определению, и os.Stat завалил бы сборку исправного конфига.
func TestRemoteLocalEntryNotStatedLocally(t *testing.T) {
	in := json.RawMessage(`{"tag":"x","type":"local","format":"binary","path":"/var/lib/sing-box-lxd/state/resources/x.srs"}`)
	if _, err := convertRuleSetToLocalRequired(in, t.TempDir(), "/var/lib/sing-box-lxd/state/resources"); err != nil {
		t.Errorf("remote local entry must not be checked on this machine: %v", err)
	}
	// А для локальной машины проверка остаётся: там путь наш и файл обязан быть.
	if _, err := convertRuleSetToLocalRequired(in, t.TempDir(), ""); err == nil {
		t.Error("local target must still verify the file exists")
	}
}
