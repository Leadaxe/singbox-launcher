package config

import (
	"encoding/json"
	"testing"
)

// Ручной JSON-объект идёт тем же конвейером, что и ссылка (SPEC 131 W2c,
// ловушка Л2).
//
// До этой волны ветка config_json делала stripTagAndDetour и возвращала вход
// дословно: ни санитайзера, ни эмиттера. Опечатка в имени ключа уезжала в
// state.Node.Body как есть и валила `sing-box check` на ВСЁМ конфиге —
// человек оставался без VPN, не зная, какой из узлов виноват.
//
// Тест интеграционный намеренно: дефект жил на стыке воронки материализации и
// эмиттера, и юнит на любой половине был бы зелёным и на сломанном коде.
func TestManualJSONGoesThroughPipeline(t *testing.T) {
	raw := json.RawMessage(`{"type":"vless","tag":"x","detour":"y","server":"e.com","server_port":443,"uuid":"11111111-1111-1111-1111-111111111111","bogus_key":1,"flow":"xtls-rprx-direct"}`)
	res, err := MaterializeServerNode("", raw)
	if err != nil {
		t.Fatalf("MaterializeServerNode: %v", err)
	}
	t.Logf("body=%s", res.Body)
	t.Logf("warnings=%+v", res.Warnings)
	var m map[string]interface{}
	if err := json.Unmarshal(res.Body, &m); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	for _, k := range []string{"bogus_key", "flow", "tag", "detour"} {
		if _, bad := m[k]; bad {
			t.Errorf("%q must not survive in the body", k)
		}
	}
	if m["type"] != "vless" {
		t.Errorf("type lost: %v", m["type"])
	}
	if len(res.Warnings) == 0 {
		t.Error("junk must produce warnings")
	}
	// Тип вне реестра остаётся passthrough: правил для него нет, и выдумывать
	// их конвейер не вправе — ради таких узлов вкладка JSON и существует.
	raw2 := json.RawMessage(`{"type":"some-exotic","tag":"z","server":"e.com","custom":"keepme"}`)
	res2, err := MaterializeServerNode("", raw2)
	if err != nil {
		t.Fatalf("exotic: %v", err)
	}
	t.Logf("exotic body=%s", res2.Body)
	var m2 map[string]interface{}
	if err := json.Unmarshal(res2.Body, &m2); err != nil {
		t.Fatalf("unmarshal exotic body: %v", err)
	}
	if m2["custom"] != "keepme" {
		t.Errorf("exotic type must stay passthrough, got %v", m2)
	}
}
