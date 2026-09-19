package linkmap

import (
	"strings"
	"testing"
)

func TestEmitWireGuardRefusesMultiplePeers(t *testing.T) {
	h := newEmitHarness(t)
	body := map[string]interface{}{
		"private_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"address":     []interface{}{"10.0.0.2/32"},
		"peers": []interface{}{
			map[string]interface{}{
				"address": "1.1.1.1", "port": 51820,
				"public_key": "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=",
				"allowed_ips": []interface{}{"0.0.0.0/0"},
			},
			map[string]interface{}{
				"address": "2.2.2.2", "port": 51820,
				"public_key": "AQIDAQIDAQIDAQIDAQIDAQIDAQIDAQIDAQIDAQIDAQIDAQI=",
				"allowed_ips": []interface{}{"0.0.0.0/0"},
			},
		},
	}
	_, err := h.emitOne("wireguard", body, "wg-multi")
	assertRefuseMultiPeer(t, err)

	bodyNative := map[string]interface{}{
		"private_key": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"address":     []string{"10.0.0.2/32"},
		"peers": []map[string]interface{}{
			{
				"address": "1.1.1.1", "port": 51820,
				"public_key": "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE=",
				"allowed_ips": []string{"0.0.0.0/0"},
			},
			{
				"address": "2.2.2.2", "port": 51820,
				"public_key": "AQIDAQIDAQIDAQIDAQIDAQIDAQIDAQIDAQIDAQIDAQIDAQI=",
				"allowed_ips": []string{"0.0.0.0/0"},
			},
		},
	}
	_, err = h.emitOne("wireguard", bodyNative, "wg-multi-native")
	assertRefuseMultiPeer(t, err)
}

func assertRefuseMultiPeer(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected refuse error, got nil")
	}
	want := "ОДИН удалённый сервер"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}
