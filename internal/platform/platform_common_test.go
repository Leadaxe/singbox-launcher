package platform

import (
	"path/filepath"
	"testing"
)

func TestGetWizardTemplatePath(t *testing.T) {
	got := GetWizardTemplatePath("/opt/sbl")
	want := filepath.Join("/opt/sbl", "bin", "wizard_template.json")
	if got != want {
		t.Errorf("GetWizardTemplatePath = %q, want %q", got, want)
	}
}

func TestGetWizardStatesDir(t *testing.T) {
	got := GetWizardStatesDir("/opt/sbl")
	want := filepath.Join("/opt/sbl", "bin", "wizard_states")
	if got != want {
		t.Errorf("GetWizardStatesDir = %q, want %q", got, want)
	}
}

func TestGetWizardStatePath(t *testing.T) {
	got := GetWizardStatePath("/opt/sbl")
	want := filepath.Join("/opt/sbl", "bin", "wizard_states", "state.json")
	if got != want {
		t.Errorf("GetWizardStatePath = %q, want %q", got, want)
	}
}

func TestGetWizardStatePathConsistentWithGetWizardStatesDir(t *testing.T) {
	// Invariant: state.json lives directly inside wizard_states/.
	dir := GetWizardStatesDir("/x")
	state := GetWizardStatePath("/x")
	if filepath.Dir(state) != dir {
		t.Errorf("state path %q must live inside states dir %q", state, dir)
	}
}

func TestGetConfigPath(t *testing.T) {
	got := GetConfigPath("/opt/sbl")
	want := filepath.Join("/opt/sbl", "bin", "config.json")
	if got != want {
		t.Errorf("GetConfigPath = %q, want %q", got, want)
	}
}

func TestGetOutboundsCachePath(t *testing.T) {
	got := GetOutboundsCachePath("/opt/sbl")
	want := filepath.Join("/opt/sbl", "bin", "outbounds.cache.json")
	if got != want {
		t.Errorf("GetOutboundsCachePath = %q, want %q", got, want)
	}
}

// TestOutboundsCachePathSiblingOfConfig — инвариант: outbounds.cache.json
// лежит в той же директории, что и config.json (а не в wizard_states/).
// Cache — общий артефакт парсера, не часть state'а (SPEC 045 PLAN.md).
func TestOutboundsCachePathSiblingOfConfig(t *testing.T) {
	cfg := GetConfigPath("/x")
	cache := GetOutboundsCachePath("/x")
	if filepath.Dir(cfg) != filepath.Dir(cache) {
		t.Errorf("config and outbounds-cache must share dir; got config=%q cache=%q", cfg, cache)
	}
}
