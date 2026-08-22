package state

import (
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// USER-патч из одних пустых значений снимается при загрузке: он не правка, а
// артефакт формы, и он маскирует пресетные патчи (живой state.json:
// `{"addOutbounds":[],"comment":"","filters":{}}` на proxy-out прятал
// `!/(🇷🇺)/i` от пресета russian).
func TestLoadDropsEmptyUserPatchButKeepsPresetAndRealOnes(t *testing.T) {
	raw := `{
	  "meta": {"version": 6, "schema": "` + SchemaName + `", "created_at": "2026-08-22T00:00:00Z", "updated_at": "2026-08-22T00:00:00Z"},
	  "connections": {"sources": [], "defaults": {},
	    "direction_outbounds": [
	      {"tag": "proxy-out", "ref": "#TEMPLATE#", "updates": [
	        {"ref": "russian", "patch": {"filters": {"tag": "!/(🇷🇺)/i"}}},
	        {"ref": "#USER#", "patch": {"addOutbounds": [], "comment": "", "filters": {}}}
	      ]},
	      {"tag": "vpn ②", "ref": "#TEMPLATE#", "updates": [
	        {"ref": "#USER#", "patch": {"addOutbounds": ["direct-out"], "filters": {"tag": "/🔥/i"}}}
	      ]},
	      {"tag": "vpn-3", "ref": "#TEMPLATE#", "updates": [
	        {"ref": "#USER#", "patch": {"disabled": false}}
	      ]}
	    ]},
	  "rules": [], "dns_options": {}
	}`
	s, err := parseCurrent([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	byTag := map[string]configtypes.Direction{}
	for _, o := range s.Connections.Outbounds {
		byTag[o.Tag] = o
	}
	if got := byTag["proxy-out"].Updates; len(got) != 1 || got[0].Ref != "russian" {
		t.Fatalf("пустой USER-патч должен уйти, пресетный остаться: %+v", got)
	}
	if got := byTag["vpn ②"].Updates; len(got) != 1 || got[0].Ref != configtypes.RefUser {
		t.Fatalf("настоящий USER-патч потерян: %+v", got)
	}
	if got := byTag["vpn-3"].Updates; len(got) != 1 {
		t.Fatalf("disabled:false — осмысленное значение, патч должен остаться: %+v", got)
	}
}
