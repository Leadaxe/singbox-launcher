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
		if got := GetWizardStatesDirFor(execDir, target, ""); got != want {
			t.Errorf("dir for %q: want %s, got %s", target, want, got)
		}
	}
	if got := GetWizardStatePathFor(execDir, constants.ConfigTargetLocal, ""); got != GetWizardStatePath(execDir) {
		t.Errorf("local state path diverged from canonical: %s vs %s", got, GetWizardStatePath(execDir))
	}
}

// SPEC 098: id для local игнорируется. Иначе достаточно было бы забыть
// обнулить id при переключении на local, чтобы локальное состояние уехало в
// папку машины и «пропало» для всех остальных читателей.
func TestLocalTargetIgnoresMachineID(t *testing.T) {
	const execDir = "/opt/sbl"
	want := filepath.Join(execDir, "bin", "wizard_states")
	if got := GetWizardStatesDirFor(execDir, constants.ConfigTargetLocal, "routerich"); got != want {
		t.Errorf("local dir must ignore machine id: want %s, got %s", want, got)
	}
	if got := GetRuleSetsDirFor(execDir, constants.ConfigTargetLocal, "routerich"); got != GetRuleSetsDir(execDir) {
		t.Errorf("local rule-sets must ignore machine id: got %s", got)
	}
	if got := GetSubscriptionsDirFor(execDir, constants.ConfigTargetLocal, "routerich"); got != GetSubscriptionsDir(execDir) {
		t.Errorf("local subscriptions must ignore machine id: got %s", got)
	}
}

func TestRemoteTargetUsesMachineSubdirectory(t *testing.T) {
	const execDir = "/opt/sbl"
	const id = "routerich"
	wantDir := filepath.Join(execDir, "bin", "wizard_states", "remote", id)
	if got := GetWizardStatesDirFor(execDir, constants.ConfigTargetRemote, id); got != wantDir {
		t.Errorf("remote dir: want %s, got %s", wantDir, got)
	}
	wantFile := filepath.Join(wantDir, "state.json")
	if got := GetWizardStatePathFor(execDir, constants.ConfigTargetRemote, id); got != wantFile {
		t.Errorf("remote state: want %s, got %s", wantFile, got)
	}
	// Case-insensitive: значение приходит из state.meta.target, где регистр
	// не гарантирован.
	if got := GetWizardStatesDirFor(execDir, "REMOTE", id); got != wantDir {
		t.Errorf("uppercase remote: want %s, got %s", wantDir, got)
	}
}

// Пустой id = плоский remote/ — pre-098 layout, из которого читает миграция.
func TestRemoteWithoutIDKeepsLegacyFlatLayout(t *testing.T) {
	const execDir = "/opt/sbl"
	want := filepath.Join(execDir, "bin", "wizard_states", "remote")
	if got := GetRemoteMachineDir(execDir, ""); got != want {
		t.Errorf("legacy remote dir: want %s, got %s", want, got)
	}
	if got := GetWizardStatesDirFor(execDir, constants.ConfigTargetRemote, "  "); got != want {
		t.Errorf("blank id must resolve to legacy dir, got %s", got)
	}
}

// SPEC 098 §5.7: у каждой машины свои .srs, свои тела подписок и свой
// config.json. Пересечение путей двух машин = нарушение инварианта «машины не
// делят файлы», и обнаружить его на живом стенде куда дороже, чем здесь.
func TestMachinesDoNotShareArtifacts(t *testing.T) {
	const execDir = "/opt/sbl"
	const a, b = "routerich", "home-vps"

	pathsA := []string{
		GetWizardStatePathFor(execDir, constants.ConfigTargetRemote, a),
		GetRemoteConfigPathFor(execDir, a),
		GetRuleSetsDirFor(execDir, constants.ConfigTargetRemote, a),
		GetSubscriptionsDirFor(execDir, constants.ConfigTargetRemote, a),
	}
	pathsB := []string{
		GetWizardStatePathFor(execDir, constants.ConfigTargetRemote, b),
		GetRemoteConfigPathFor(execDir, b),
		GetRuleSetsDirFor(execDir, constants.ConfigTargetRemote, b),
		GetSubscriptionsDirFor(execDir, constants.ConfigTargetRemote, b),
	}
	for i := range pathsA {
		if pathsA[i] == pathsB[i] {
			t.Errorf("machines share path %s", pathsA[i])
		}
	}

	// Всё имущество машины — под её директорией (§5.6): удаление машины
	// должно быть rmdir, а не поиском следов по bin/.
	machineDir := GetRemoteMachineDir(execDir, a)
	for _, p := range pathsA {
		if rel, err := filepath.Rel(machineDir, p); err != nil || rel == ".." || filepath.IsAbs(rel) ||
			len(rel) >= 2 && rel[:2] == ".." {
			t.Errorf("%s escapes machine dir %s", p, machineDir)
		}
	}

	// Локальные пути не должны попасть внутрь директории машины (§5.5).
	remoteRoot := filepath.Join(execDir, "bin", "wizard_states", "remote")
	for _, local := range []string{GetRuleSetsDir(execDir), GetSubscriptionsDir(execDir), GetConfigPath(execDir)} {
		if rel, err := filepath.Rel(remoteRoot, local); err == nil && rel != ".." &&
			!(len(rel) >= 2 && rel[:2] == "..") {
			t.Errorf("local path %s must not live under %s", local, remoteRoot)
		}
	}
}

// Опечатка в таргете не должна ронять state в директорию, куда никто не
// смотрит: неизвестное значение = local, а не новая папка.
func TestUnknownTargetFallsBackToLocal(t *testing.T) {
	const execDir = "/opt/sbl"
	want := filepath.Join(execDir, "bin", "wizard_states")
	for _, bogus := range []string{"gateway", "server", "rmote", "  "} {
		if got := GetWizardStatesDirFor(execDir, bogus, "routerich"); got != want {
			t.Errorf("unknown target %q must fall back to local dir, got %s", bogus, got)
		}
	}
}
