//go:build darwin

package core

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// testRuntimeDir имитирует state_dir из /admin/info.
const testRuntimeDir = "/Library/Application Support/sing-box-lxd/state"

func TestPrepareConfigForDaemon(t *testing.T) {
	t.Run("relative cache_file → absolute in support dir", func(t *testing.T) {
		in := []byte(`{
  // комментарий JSONC
  "log": {"level": "info"},
  "experimental": {
    "cache_file": {"enabled": true, "path": "cache.db"}
  }
}`)
		out, err := prepareConfigForDaemon(in, testRuntimeDir)
		if err != nil {
			t.Fatal(err)
		}
		var root map[string]json.RawMessage
		if err := json.Unmarshal(out, &root); err != nil {
			t.Fatalf("result not valid JSON: %v", err)
		}
		var exp map[string]json.RawMessage
		if err := json.Unmarshal(root["experimental"], &exp); err != nil {
			t.Fatalf("experimental parse: %v", err)
		}
		var cf map[string]json.RawMessage
		if err := json.Unmarshal(exp["cache_file"], &cf); err != nil {
			t.Fatalf("cache_file parse: %v", err)
		}
		var path string
		if err := json.Unmarshal(cf["path"], &path); err != nil {
			t.Fatalf("path parse: %v", err)
		}
		if !filepath.IsAbs(path) {
			t.Fatalf("path not absolutized: %q", path)
		}
		if !strings.HasSuffix(path, "cache.db") {
			t.Fatalf("basename lost: %q", path)
		}
		if !strings.Contains(path, "sing-box-lxd") {
			t.Fatalf("not in daemon support dir: %q", path)
		}
	})

	t.Run("already absolute → untouched", func(t *testing.T) {
		in := []byte(`{"experimental":{"cache_file":{"path":"/var/db/cache.db"}}}`)
		out, err := prepareConfigForDaemon(in, testRuntimeDir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), "/var/db/cache.db") {
			t.Fatalf("absolute path was altered: %s", out)
		}
	})

	t.Run("no cache_file → unchanged", func(t *testing.T) {
		in := []byte(`{"log":{"level":"info"}}`)
		out, err := prepareConfigForDaemon(in, testRuntimeDir)
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != string(in) {
			t.Fatalf("config without cache_file was modified: %s", out)
		}
	})

	t.Run("no path (default) → absolutized cache.db", func(t *testing.T) {
		in := []byte(`{"experimental":{"cache_file":{"enabled":true}}}`)
		out, err := prepareConfigForDaemon(in, testRuntimeDir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), "sing-box-lxd") || !strings.Contains(string(out), "cache.db") {
			t.Fatalf("default cache.db not absolutized: %s", out)
		}
	})

	t.Run("clash_api removed entirely (daemon uses gRPC)", func(t *testing.T) {
		in := []byte(`{"experimental":{"clash_api":{"external_controller":"127.0.0.1:9090","secret":"s"}}}`)
		out, err := prepareConfigForDaemon(in, testRuntimeDir)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "clash_api") {
			t.Fatalf("clash_api not removed: %s", out)
		}
		if strings.Contains(string(out), "9090") {
			t.Fatalf("clash_api port leaked: %s", out)
		}
	})

	t.Run("cache_file absolutized AND clash_api removed together", func(t *testing.T) {
		in := []byte(`{"experimental":{"cache_file":{"path":"cache.db"},"clash_api":{"external_controller":"127.0.0.1:9090"}}}`)
		out, err := prepareConfigForDaemon(in, testRuntimeDir)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(out), "clash_api") {
			t.Fatalf("clash_api not removed: %s", out)
		}
		if strings.Contains(string(out), `"path":"cache.db"`) {
			t.Fatalf("cache_file not absolutized: %s", out)
		}
	})
}
