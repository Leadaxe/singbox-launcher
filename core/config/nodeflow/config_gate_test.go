package nodeflow

import (
	"encoding/json"
	"testing"
)

// Ключ корневого lx новее ядра снимается, известный остаётся; опустевший
// блок уходит целиком; неизвестная версия конфиг не трогает.
func TestGateConfigForCore(t *testing.T) {
	const src = `{"log":{"level":"warn"},"lx":{"mtu_align":"fill","sniff_guard":{"tls":{"method":"record_fragment","persist":true}}}}`
	cases := []struct {
		version string
		want    string
		dropped int
	}{
		{"", src, 0},
		{"1.14.4-lx.34", src, 0},
		{"1.14.4-lx.33", `{"log":{"level":"warn"},"lx":{"mtu_align":"fill"}}`, 1},
		{"1.14.3-lx.21", `{"log":{"level":"warn"},"lx":{"mtu_align":"fill"}}`, 1},
		{"1.14.3-lx.13", `{"log":{"level":"warn"}}`, 2},
		{"1.14.0-lx.32", `{"log":{"level":"warn"}}`, 1},
	}
	for _, c := range cases {
		var root map[string]json.RawMessage
		if err := json.Unmarshal([]byte(src), &root); err != nil {
			t.Fatal(err)
		}
		dropped, err := GateConfigForCore(root, c.version)
		if err != nil {
			t.Fatalf("%s: %v", c.version, err)
		}
		got, _ := json.Marshal(root)
		if string(got) != c.want || len(dropped) != c.dropped {
			t.Errorf("core %q: got %s (dropped %v), want %s (%d dropped)", c.version, got, dropped, c.want, c.dropped)
		}
	}
	// Снятый в lx.34 quic_sniff_decoy (forbidden, контракт 1.1.170) уходит и у
	// ядра lx.33, которое его ещё знает, и при неизвестной версии; блок
	// sniff_guard рядом с ним живёт по своей границе.
	const decoy = `{"lx":{"quic_sniff_decoy":{"mode":"detour","send":"retry"},"sniff_guard":{"quic":{"method":"decoy_first_byte"}}}}`
	for version, want := range map[string]string{
		"":             `{"lx":{"sniff_guard":{"quic":{"method":"decoy_first_byte"}}}}`,
		"1.14.4-lx.34": `{"lx":{"sniff_guard":{"quic":{"method":"decoy_first_byte"}}}}`,
		"1.14.4-lx.33": `{}`,
	} {
		var root map[string]json.RawMessage
		if err := json.Unmarshal([]byte(decoy), &root); err != nil {
			t.Fatal(err)
		}
		if _, err := GateConfigForCore(root, version); err != nil {
			t.Fatalf("%s: %v", version, err)
		}
		if got, _ := json.Marshal(root); string(got) != want {
			t.Errorf("decoy, core %q: got %s, want %s", version, got, want)
		}
	}
	// Запрещённый ключ (forbidden, контракт 1.1.164) снимается при любой
	// версии, включая неизвестную; соседний ключ остаётся.
	const forb = `{"lx":{"awg_masquerade":"direct","mtu_align":"fill"}}`
	for _, version := range []string{"", "1.14.4-lx.31", "1.14.4-lx.34"} {
		var root map[string]json.RawMessage
		if err := json.Unmarshal([]byte(forb), &root); err != nil {
			t.Fatal(err)
		}
		dropped, err := GateConfigForCore(root, version)
		if err != nil {
			t.Fatalf("%s: %v", version, err)
		}
		got, _ := json.Marshal(root)
		if want := `{"lx":{"mtu_align":"fill"}}`; string(got) != want || len(dropped) != 1 {
			t.Errorf("forbidden, core %q: got %s (dropped %v), want %s", version, got, dropped, want)
		}
	}
}
