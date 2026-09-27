package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// SPEC 148 §2 (LxBox §579): отбор узлов NETWORKS из собранного конфига.
func TestNetworksNodeTags(t *testing.T) {
	const cfg = `{
	  "outbounds": [
	    {"type": "tailscale", "tag": "ts-in-outbounds"},
	    {"type": "selector", "tag": "proxy-out", "outbounds": ["ts-exit"]}
	  ],
	  "endpoints": [
	    {"type": "tailscale", "tag": "ts-lan"},
	    {"type": "tailscale", "tag": "ts-exit", "exit_node": "100.64.0.1"},
	    {"type": "tailscale", "tag": "ts-empty-exit", "exit_node": ""},
	    {"type": "wireguard", "tag": "wg-hop"},
	    {"type": "tailscale"}
	  ]
	}`
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(cfg), &m); err != nil {
		t.Fatal(err)
	}
	got := NetworksNodeTags(m)
	// Входят: tailscale без exit_node (пустая строка для ядра = нет ключа).
	// Не входят: с exit_node, WireGuard вне групп, запись в outbounds[],
	// запись без тега.
	want := []string{"ts-lan", "ts-empty-exit"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NetworksNodeTags = %v, want %v", got, want)
	}

	if got := NetworksNodeTags(map[string]interface{}{"outbounds": []interface{}{}}); len(got) != 0 {
		t.Errorf("без endpoints[] NETWORKS обязано быть пустым: %v", got)
	}

	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	fromFile, err := GetNetworksNodeTagsFromConfig(path)
	if err != nil || !reflect.DeepEqual(fromFile, want) {
		t.Errorf("по файлу: %v, %v", fromFile, err)
	}
}
