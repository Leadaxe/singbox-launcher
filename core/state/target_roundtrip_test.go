package state

import (
	"os"
	"path/filepath"
	"testing"
)

// Регрессия ревью-проходки №3 (найдена в рантайме, не тестами).
//
// SwitchConfigTarget читал remote/state.json, LoadState восстанавливал Target
// ИЗ ФАЙЛА, и если файл был legacy/без meta.target — модель тихо становилась
// local, оставаясь показанной как remote. Следующий флаш писал remote-данные
// в local/state.json, затирая локальную конфигурацию.
//
// Здесь фиксируется свойство файлового слоя, на которое опирается фикс:
// meta.target прочитанного файла — единственный источник правды о том, чей
// это state, и он не должен «протекать» между директориями.
func TestTargetIsNotInheritedAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "state.json")
	remoteDir := filepath.Join(dir, "remote")
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	remotePath := filepath.Join(remoteDir, "state.json")

	local := &State{Version: SchemaVersionV6, Comment: "local"}
	if err := local.Save(localPath); err != nil {
		t.Fatalf("save local: %v", err)
	}
	remote := &State{
		Version: SchemaVersionV6, Comment: "remote",
		Target: "remote", TargetPlatform: "linux", TargetArch: "arm64",
	}
	if err := remote.Save(remotePath); err != nil {
		t.Fatalf("save remote: %v", err)
	}

	gotLocal, err := Load(localPath)
	if err != nil {
		t.Fatalf("load local: %v", err)
	}
	if gotLocal.Target != "" {
		t.Errorf("local state must stay untargeted, got %q", gotLocal.Target)
	}
	if gotLocal.Comment != "local" {
		t.Errorf("local state content clobbered: comment=%q", gotLocal.Comment)
	}

	gotRemote, err := Load(remotePath)
	if err != nil {
		t.Fatalf("load remote: %v", err)
	}
	if gotRemote.Target != "remote" || gotRemote.TargetPlatform != "linux" {
		t.Errorf("remote meta lost: %+v", gotRemote)
	}
	if gotRemote.Comment != "remote" {
		t.Errorf("remote state content clobbered: comment=%q", gotRemote.Comment)
	}
}
