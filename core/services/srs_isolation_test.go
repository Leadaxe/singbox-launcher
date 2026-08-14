package services

import (
	"os"
	"path/filepath"
	"testing"

	"singbox-launcher/internal/constants"
	"singbox-launcher/internal/platform"
)

// SPEC 098 §5.7: операция над одной машиной не может удалить файл другой.
//
// До SPEC 098 все .srs лежали в общем bin/rule-sets/, поэтому GC был обязан
// считать живой набор объединением по ВСЕМ состояниям на диске. Стоило
// пропустить чужое состояние — и rebuild сносил чужой живой файл. Здесь
// проверяется, что раздельные каталоги эту связность убрали.
func TestOrphanGCStaysWithinMachineBoundaries(t *testing.T) {
	execDir := t.TempDir()

	seed := func(target, id, tag string) string {
		dir := platform.GetRuleSetsDirFor(execDir, target, id)
		if err := os.MkdirAll(dir, platform.DefaultDirMode); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, tag+".srs")
		if err := os.WriteFile(path, []byte("srs"), platform.DefaultFileMode); err != nil {
			t.Fatal(err)
		}
		return path
	}

	// Один и тот же тег у трёх владельцев — именно тот случай, где раньше
	// один GC решал судьбу чужих файлов.
	localFile := seed(constants.ConfigTargetLocal, "", "geosite-ru")
	routerFile := seed(constants.ConfigTargetRemote, "routerich", "geosite-ru")
	vpsFile := seed(constants.ConfigTargetRemote, "home-vps", "geosite-ru")

	// У роутера тег больше не живой — чистим ЕГО каталог с пустым набором.
	deleted, err := DeleteOrphanRuleSetsFor(execDir, constants.ConfigTargetRemote, "routerich", nil)
	if err != nil {
		t.Fatalf("GC failed: %v", err)
	}
	if len(deleted) != 1 {
		t.Errorf("expected exactly one deletion, got %v", deleted)
	}

	if _, err := os.Stat(routerFile); !os.IsNotExist(err) {
		t.Errorf("router's orphan must be deleted, err=%v", err)
	}
	for name, path := range map[string]string{"local": localFile, "vps": vpsFile} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s file must survive another machine's GC: %v", name, err)
		}
	}
}

// Живой тег машины не удаляется, а чужой живой тег её не удерживает:
// набор считается строго по её состояниям.
func TestOrphanGCKeepsOwnLiveTags(t *testing.T) {
	execDir := t.TempDir()
	dir := platform.GetRuleSetsDirFor(execDir, constants.ConfigTargetRemote, "routerich")
	if err := os.MkdirAll(dir, platform.DefaultDirMode); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(dir, "keep-me.srs")
	dead := filepath.Join(dir, "drop-me.srs")
	for _, p := range []string{live, dead} {
		if err := os.WriteFile(p, []byte("srs"), platform.DefaultFileMode); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := DeleteOrphanRuleSetsFor(execDir, constants.ConfigTargetRemote, "routerich",
		[]string{"keep-me"}); err != nil {
		t.Fatalf("GC failed: %v", err)
	}

	if _, err := os.Stat(live); err != nil {
		t.Errorf("live tag must survive: %v", err)
	}
	if _, err := os.Stat(dead); !os.IsNotExist(err) {
		t.Errorf("orphan must be deleted, err=%v", err)
	}
}

// Отсутствующий каталог — обычное дело для только что добавленной машины,
// а не ошибка: чистить там нечего.
func TestOrphanGCOnMissingDirectoryIsNoop(t *testing.T) {
	execDir := t.TempDir()
	deleted, err := DeleteOrphanRuleSetsFor(execDir, constants.ConfigTargetRemote, "never-configured", nil)
	if err != nil {
		t.Fatalf("missing dir must not be an error: %v", err)
	}
	if len(deleted) != 0 {
		t.Errorf("nothing to delete, got %v", deleted)
	}
}

// Локальный GC (обёртка без target) обязан работать ровно как раньше —
// инвариант §5.5 «local-путь генерации не меняется».
func TestLocalOrphanGCUnchanged(t *testing.T) {
	execDir := t.TempDir()
	dir := platform.GetRuleSetsDir(execDir)
	if err := os.MkdirAll(dir, platform.DefaultDirMode); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(dir, "stale.srs")
	if err := os.WriteFile(orphan, []byte("srs"), platform.DefaultFileMode); err != nil {
		t.Fatal(err)
	}

	deleted, err := DeleteOrphanRuleSets(execDir, []string{"other"})
	if err != nil {
		t.Fatalf("local GC failed: %v", err)
	}
	if len(deleted) != 1 || deleted[0] != "stale.srs" {
		t.Errorf("expected stale.srs deleted, got %v", deleted)
	}
}
