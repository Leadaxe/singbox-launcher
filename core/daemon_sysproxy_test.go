//go:build darwin || (windows && !386)

package core

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// TestPrepareConfigForDaemonSystemProxy — данные, уходящие демону (SPEC
// 141 §7): при «прокси ставит лаунчер» каждый set_system_proxy уходит как
// false, лаунчеру — адрес первого так, как его строит ядро; state_directory
// tailscale из DataDir переезжает в <StateDir>/tailscale/<тег>, явный
// каталог вне корня остаётся. Без платформенных шагов конфиг тот же.
func TestPrepareConfigForDaemonSystemProxy(t *testing.T) {
	localRoot := filepath.Join(testRuntimeDir, "..", "user-data", "tailscale")
	own, _ := json.Marshal(filepath.Join(localRoot, "ts-home"))
	outside, _ := json.Marshal(filepath.Join(testRuntimeDir, "..", "custom", "ts"))
	in := []byte(`{
  // JSONC
  "inbounds": [
    {"type": "mixed", "tag": "proxy-in", "listen": "0.0.0.0", "listen_port": 7890, "set_system_proxy": true},
    {"type": "http", "tag": "second", "listen": "::1", "listen_port": 8080, "set_system_proxy": true},
    {"type": "tun", "tag": "tun-in"}
  ],
  "endpoints": [
    {"type": "tailscale", "tag": "ts-home", "state_directory": ` + string(own) + `},
    {"type": "tailscale", "tag": "ts-custom", "state_directory": ` + string(outside) + `}
  ]
}`)

	out, server, err := prepareConfigForDaemonWith(in, testRuntimeDir, daemonPrepOptions{launcherSetsProxy: true, tailscaleLocalRoot: localRoot})
	if err != nil {
		t.Fatal(err)
	}
	if server != "http://127.0.0.1:7890" {
		t.Fatalf("proxy server = %q, want the first inbound as the core builds it", server)
	}
	var cfg struct {
		Inbounds []struct {
			Tag            string `json:"tag"`
			SetSystemProxy *bool  `json:"set_system_proxy"`
		} `json:"inbounds"`
		Endpoints []struct {
			Tag            string `json:"tag"`
			StateDirectory string `json:"state_directory"`
		} `json:"endpoints"`
	}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatalf("result not valid JSON: %v", err)
	}
	for _, ib := range cfg.Inbounds {
		if ib.SetSystemProxy != nil && *ib.SetSystemProxy {
			t.Fatalf("inbound %s still asks the daemon for the system proxy", ib.Tag)
		}
	}
	if got, want := cfg.Endpoints[0].StateDirectory, filepath.Join(testRuntimeDir, "tailscale", "ts-home"); got != want {
		t.Fatalf("tailscale state_directory = %q, want %q", got, want)
	}
	var wantOutside string
	_ = json.Unmarshal(outside, &wantOutside)
	if got := cfg.Endpoints[1].StateDirectory; got != wantOutside {
		t.Fatalf("explicit state_directory moved: %q", got)
	}

	same, server, err := prepareConfigForDaemonWith(in, testRuntimeDir, daemonPrepOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if server != "" || string(same) != string(in) {
		t.Fatalf("without platform steps the config must stay as is (server %q)", server)
	}
}
