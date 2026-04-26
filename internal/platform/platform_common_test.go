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
