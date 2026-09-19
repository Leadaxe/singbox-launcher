package nodeflow

import (
	"encoding/json"
	"testing"
)

// Стражи зеркальных дефектов LxBox (SPEC 134): CIDR, дробные JSON-числа.

func TestCIDRFormatRejectsInvalidPrefixes(t *testing.T) {
	cases := []string{
		"1.2.3.4/64",
		"::::/128",
		"10.0.0.1/33",
		"fe80::1/129",
		"10.0.0.1/-1",
		" 10.0.0.1/24",
	}
	for _, cidr := range cases {
		if formatOK("cidr", cidr) {
			t.Errorf("formatOK(cidr, %q) = true, want false", cidr)
		}
	}
	if !formatOK("cidr", "10.0.0.1/24") {
		t.Error("valid cidr 10.0.0.1/24 rejected")
	}
}

func TestFractionalPortRejectedBySanitize(t *testing.T) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(`{"server":"a.com","server_port":443.9,"uuid":"b831381d-6324-4d53-ad4f-8cda48b30811"}`), &m); err != nil {
		t.Fatal(err)
	}
	res := Sanitize("vless", m)
	if res.Clean["server_port"] != nil {
		t.Fatalf("server_port=%v, want dropped", res.Clean["server_port"])
	}
}

func TestFractionalMTUAndReservedRejectedBySanitize(t *testing.T) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(`{
		"type":"wireguard","private_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"address":["10.0.0.2/32"],"mtu":1420.5,
		"peers":[{"address":"1.1.1.1","port":51820,"public_key":"AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=","allowed_ips":["0.0.0.0/0"],"reserved":[1,2.5]}]
	}`), &m); err != nil {
		t.Fatal(err)
	}
	res := Sanitize("wireguard", m)
	if res.Clean["mtu"] != nil {
		t.Fatalf("mtu=%v, want dropped", res.Clean["mtu"])
	}
	peers, _ := res.Clean["peers"].([]interface{})
	if len(peers) == 0 {
		t.Fatal("no peers in clean body")
	}
	peer, _ := peers[0].(map[string]interface{})
	if peer["reserved"] != nil {
		t.Fatalf("reserved=%v, want dropped", peer["reserved"])
	}
}

func TestWholeFloatPortAcceptedBySanitize(t *testing.T) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(`{"server":"a.com","server_port":443.0,"uuid":"b831381d-6324-4d53-ad4f-8cda48b30811"}`), &m); err != nil {
		t.Fatal(err)
	}
	res := Sanitize("vless", m)
	if res.Clean["server_port"] != 443 {
		t.Fatalf("server_port=%v, want 443", res.Clean["server_port"])
	}
}
