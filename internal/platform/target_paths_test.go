package platform

import (
	"path/filepath"
	"testing"

	"singbox-launcher/internal/constants"
)

// SPEC 097: local сохраняет исторический плоский layout — иначе все читатели
// state.json (varsubst, snapshot, config_service) и уже сохранённые снапшоты
// пользователей разъедутся с новым кодом.
func TestLocalTargetKeepsFlatLayout(t *testing.T) {
	const execDir = "/opt/sbl"
	want := filepath.Join(execDir, "bin", "wizard_states")
	for _, target := range []string{constants.ConfigTargetLocal, ""} {
		if got := GetWizardStatesDirFor(execDir, target); got != want {
			t.Errorf("dir for %q: want %s, got %s", target, want, got)
		}
	}
	if got := GetWizardStatePathFor(execDir, constants.ConfigTargetLocal); got != GetWizardStatePath(execDir) {
		t.Errorf("local state path diverged from canonical: %s vs %s", got, GetWizardStatePath(execDir))
	}
}

func TestRemoteTargetUsesSubdirectory(t *testing.T) {
	const execDir = "/opt/sbl"
	wantDir := filepath.Join(execDir, "bin", "wizard_states", "remote")
	if got := GetWizardStatesDirFor(execDir, constants.ConfigTargetRemote); got != wantDir {
		t.Errorf("remote dir: want %s, got %s", wantDir, got)
	}
	wantFile := filepath.Join(wantDir, "state.json")
	if got := GetWizardStatePathFor(execDir, constants.ConfigTargetRemote); got != wantFile {
		t.Errorf("remote state: want %s, got %s", wantFile, got)
	}
	// Case-insensitive: значение приходит из state.meta.target, где регистр
	// не гарантирован.
	if got := GetWizardStatesDirFor(execDir, "REMOTE"); got != wantDir {
		t.Errorf("uppercase remote: want %s, got %s", wantDir, got)
	}
}

// Опечатка в таргете не должна ронять state в директорию, куда никто не
// смотрит: неизвестное значение = local, а не новая папка.
func TestUnknownTargetFallsBackToLocal(t *testing.T) {
	const execDir = "/opt/sbl"
	want := filepath.Join(execDir, "bin", "wizard_states")
	for _, bogus := range []string{"gateway", "server", "rmote", "  "} {
		if got := GetWizardStatesDirFor(execDir, bogus); got != want {
			t.Errorf("unknown target %q must fall back to local dir, got %s", bogus, got)
		}
	}
}
