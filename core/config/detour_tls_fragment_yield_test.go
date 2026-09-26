package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// TestDetourYieldsTLSFragmentOnEmit — контракт 1.1.84: узел, которому сборка
// проставила detour, эмитится БЕЗ tls.fragment (связь реестра
// tls.fragment conflicts {with: detour}). Материализованный узел эмитится из
// замороженного EmitBody, поэтому правило обязано действовать на границе
// «тело → outbound», а не только на карте Outbound. Звено цепочки без
// detour фрагментацию сохраняет; тело состояния не меняется.
func TestDetourYieldsTLSFragmentOnEmit(t *testing.T) {
	mk := func(tag, server string) *ParsedNode {
		body := map[string]interface{}{
			"type": "vless", "server": server, "server_port": 443,
			"uuid": "11111111-1111-1111-1111-111111111111",
			"tls":  map[string]interface{}{"enabled": true, "server_name": server, "fragment": true},
		}
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var outbound map[string]interface{}
		if err := json.Unmarshal(raw, &outbound); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return &ParsedNode{
			Tag: tag, Scheme: "vless", Outbound: outbound,
			EmitBody:    json.RawMessage(raw),
			SourceIndex: configtypes.UnsetSourceIndex,
		}
	}
	owner := mk("owner", "a.example")
	hop := mk("hop", "r.example")
	owner.Chain = []*ParsedNode{hop}

	outs, _, err := EmitNodeJSONs(owner)
	if err != nil {
		t.Fatalf("EmitNodeJSONs: %v", err)
	}
	if len(outs) != 2 {
		t.Fatalf("ожидалось 2 outbound (хоп + владелец), got %d: %v", len(outs), outs)
	}
	hopJSON, ownerJSON := outs[0], outs[1]
	if !strings.Contains(hopJSON, `"fragment":true`) {
		t.Errorf("хоп ходит наружу напрямую — fragment обязан остаться:\n%s", hopJSON)
	}
	if !strings.Contains(ownerJSON, `"detour":"hop"`) {
		t.Fatalf("владелец цепочки без detour:\n%s", ownerJSON)
	}
	if strings.Contains(ownerJSON, `"fragment"`) {
		t.Errorf("владелец под detour эмитирован с tls.fragment:\n%s", ownerJSON)
	}
	if !strings.Contains(ownerJSON, `"server_name":"a.example"`) {
		t.Errorf("снятие fragment задело соседние поля tls:\n%s", ownerJSON)
	}
	tls, _ := owner.Outbound["tls"].(map[string]interface{})
	if tls["fragment"] != true {
		t.Errorf("эмит изменил тело состояния: tls = %v", tls)
	}
}
