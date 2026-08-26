package business

import (
	"encoding/json"
	"strings"
	"testing"
)

// Одиночный outbound — самый частый случай: человек скопировал объект из
// чужого конфига и вставил в поле Add.
func TestCarveSingboxJSON_SingleOutbound(t *testing.T) {
	in := `{"type":"vless","tag":"my-node","server":"1.2.3.4","server_port":443,"uuid":"u-1"}`
	nodes, isJSON, err := carveSingboxJSON(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isJSON {
		t.Fatal("single outbound must be recognized as JSON")
	}
	if len(nodes) != 1 {
		t.Fatalf("want 1 node, got %d", len(nodes))
	}
	if nodes[0].Label != "my-node" {
		t.Errorf("label: want my-node, got %q", nodes[0].Label)
	}
	// Тело должно доехать целиком, а не быть урезанным до опорных полей.
	var ob map[string]interface{}
	if err := json.Unmarshal(nodes[0].ConfigJSON, &ob); err != nil {
		t.Fatalf("config json invalid: %v", err)
	}
	if ob["uuid"] != "u-1" {
		t.Errorf("uuid lost in round-trip: %v", ob["uuid"])
	}
}

// Массив outbound'ов раскладывается по одному Source на узел — так же, как
// список share-URI.
func TestCarveSingboxJSON_OutboundArray(t *testing.T) {
	in := `[{"type":"socks","tag":"a","server":"10.0.0.1","server_port":1080},
	        {"type":"http","tag":"b","server":"10.0.0.2","server_port":8080}]`
	nodes, isJSON, err := carveSingboxJSON(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isJSON {
		t.Fatal("array must be recognized as JSON")
	}
	if len(nodes) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(nodes))
	}
	if nodes[0].Label != "a" || nodes[1].Label != "b" {
		t.Errorf("labels: got %q, %q", nodes[0].Label, nodes[1].Label)
	}
}

// Целый конфиг: берём outbounds, служебные секции игнорируются импортом.
func TestCarveSingboxJSON_FullConfig(t *testing.T) {
	in := `{"log":{"level":"info"},"outbounds":[
	         {"type":"trojan","tag":"t1","server":"h.example","server_port":443,"password":"p"}]}`
	nodes, isJSON, err := carveSingboxJSON(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isJSON || len(nodes) != 1 {
		t.Fatalf("want 1 node from full config, got %d (isJSON=%v)", len(nodes), isJSON)
	}
	if nodes[0].Label != "t1" {
		t.Errorf("label: want t1, got %q", nodes[0].Label)
	}
}

// Не-JSON вход обязан пройти мимо: ссылки разбирает построчный классификатор.
func TestCarveSingboxJSON_PassesThroughURIs(t *testing.T) {
	for _, in := range []string{
		"vless://u@h:443#node",
		"https://example.com/sub",
		"",
		"  ",
	} {
		nodes, isJSON, err := carveSingboxJSON(in)
		if err != nil || isJSON || nodes != nil {
			t.Errorf("input %q: want pass-through, got isJSON=%v nodes=%d err=%v",
				in, isJSON, len(nodes), err)
		}
	}
}

// Битый JSON не должен молча превращаться в «no valid URLs to add»: человек
// вставил документ и обязан узнать, что с ним не так.
func TestCarveSingboxJSON_BrokenReportsError(t *testing.T) {
	// Валидный объект, но без "type" — классификатор его за outbound не
	// примет, а как конфиг он пуст.
	nodes, isJSON, err := carveSingboxJSON(`{"server":"1.2.3.4"}`)
	if isJSON && err == nil && len(nodes) > 0 {
		t.Fatalf("typeless object must not produce nodes: %d", len(nodes))
	}

	// Оборванный документ.
	if _, jsonish, err := carveSingboxJSON(`{"type":"vless","tag":`); jsonish && err == nil {
		t.Error("truncated JSON classified as valid")
	}
}

// Неизвестный sing-box тип в одиночном объекте сохраняется passthrough —
// лаунчер не обязан знать каждый тип, чтобы дать его ядру.
func TestCarveSingboxJSON_UnknownTypePassthrough(t *testing.T) {
	in := `{"type":"brand-new-proto","tag":"x","server":"h","server_port":1}`
	nodes, _, err := carveSingboxJSON(in)
	if err != nil {
		t.Fatalf("unknown type must survive: %v", err)
	}
	if len(nodes) != 1 || !strings.Contains(string(nodes[0].ConfigJSON), "brand-new-proto") {
		t.Errorf("unknown type not preserved: %+v", nodes)
	}
}
