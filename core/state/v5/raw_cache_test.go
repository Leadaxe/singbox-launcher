package v5

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestWriteReadRawBody — round-trip + atomic semantics.
func TestWriteReadRawBody(t *testing.T) {
	dir := t.TempDir()
	id := "01TESTABC"
	body := []byte("vless://uuid@host:443#node\n")

	if err := WriteRawBody(dir, id, body); err != nil {
		t.Fatalf("WriteRawBody: %v", err)
	}

	got, err := ReadRawBody(dir, id)
	if err != nil {
		t.Fatalf("ReadRawBody: %v", err)
	}
	if string(got) != string(body) {
		t.Errorf("body mismatch: got %q, want %q", got, body)
	}

	// Файл существует с ожидаемым именем.
	target := filepath.Join(dir, id+".raw")
	if _, err := os.Stat(target); err != nil {
		t.Errorf("expected %s to exist, got %v", target, err)
	}

	// .tmp убран после rename.
	tmp := filepath.Join(dir, id+".raw.tmp")
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Errorf(".tmp should be removed, stat=%v", err)
	}
}

// TestReadRawBody_NotFound — чёткая ошибка при отсутствии.
func TestReadRawBody_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadRawBody(dir, "01ABSENT")
	if !errors.Is(err, ErrRawNotFound) {
		t.Errorf("expected ErrRawNotFound, got %v", err)
	}
}

// TestWriteRawBody_OverwriteAtomic — повторный write не повреждает старый
// файл если что-то пошло не так. Проверяем нормальный overwrite path.
func TestWriteRawBody_OverwriteAtomic(t *testing.T) {
	dir := t.TempDir()
	id := "01OVERWRITE"
	if err := WriteRawBody(dir, id, []byte("OLD")); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	if err := WriteRawBody(dir, id, []byte("NEW")); err != nil {
		t.Fatalf("write 2: %v", err)
	}
	got, err := ReadRawBody(dir, id)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "NEW" {
		t.Errorf("got %q, want NEW", got)
	}
}

// TestDeleteRawBody — отсутствующий файл не ошибка.
func TestDeleteRawBody(t *testing.T) {
	dir := t.TempDir()
	id := "01DELME"
	_ = WriteRawBody(dir, id, []byte("x"))
	if err := DeleteRawBody(dir, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, id+".raw")); !os.IsNotExist(err) {
		t.Errorf("file still exists")
	}
	// idempotent — удаление несуществующего ОК
	if err := DeleteRawBody(dir, id); err != nil {
		t.Errorf("delete idempotent: %v", err)
	}
}

// TestListRawBodyIDs — возвращает только .raw файлы с валидными id'ами.
func TestListRawBodyIDs(t *testing.T) {
	dir := t.TempDir()
	_ = WriteRawBody(dir, "01ID-A", []byte("a"))
	_ = WriteRawBody(dir, "01ID-B", []byte("b"))
	// Шум: не-.raw, .tmp, поддиректория, .raw с инвалидным id.
	_ = os.WriteFile(filepath.Join(dir, "garbage.txt"), []byte("noise"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "01XYZ.raw.tmp"), []byte("tmp"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "../suspicious.raw"), []byte("evil"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "has space.raw"), []byte("evil"), 0o644)
	_ = os.MkdirAll(filepath.Join(dir, "subdir"), 0o755)

	ids, err := ListRawBodyIDs(dir)
	if err != nil {
		t.Fatalf("ListRawBodyIDs: %v", err)
	}
	sort.Strings(ids)
	want := []string{"01ID-A", "01ID-B"}
	if len(ids) != len(want) {
		t.Errorf("got %v, want %v", ids, want)
	}
	for i := range want {
		if i >= len(ids) || ids[i] != want[i] {
			t.Errorf("ids[%d]: got %v, want %s", i, ids, want[i])
		}
	}
}

// TestListRawBodyIDs_MissingDir — отсутствующий каталог не ошибка.
func TestListRawBodyIDs_MissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nope")
	ids, err := ListRawBodyIDs(dir)
	if err != nil {
		t.Errorf("got err: %v, want nil", err)
	}
	if len(ids) != 0 {
		t.Errorf("got %v, want empty", ids)
	}
}

// TestDeleteOrphans — удаляет id'ы которых нет в knownIDs + .tmp мусор.
func TestDeleteOrphans(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"01KEEP-A", "01KEEP-B", "01ORPHAN-A", "01ORPHAN-B"} {
		_ = WriteRawBody(dir, id, []byte(id))
	}
	_ = os.WriteFile(filepath.Join(dir, "01STALE.raw.tmp"), []byte("tmp"), 0o644)

	deleted, err := DeleteOrphans(dir, []string{"01KEEP-A", "01KEEP-B"})
	if err != nil {
		t.Fatalf("DeleteOrphans: %v", err)
	}

	sort.Strings(deleted)
	wantDel := []string{"01ORPHAN-A", "01ORPHAN-B"}
	if len(deleted) != len(wantDel) {
		t.Errorf("deleted = %v, want %v", deleted, wantDel)
	}

	// .tmp мусор тоже убран.
	if _, err := os.Stat(filepath.Join(dir, "01STALE.raw.tmp")); !os.IsNotExist(err) {
		t.Errorf(".tmp not cleaned: %v", err)
	}

	// Известные id остались.
	for _, id := range []string{"01KEEP-A", "01KEEP-B"} {
		if _, err := os.Stat(filepath.Join(dir, id+".raw")); err != nil {
			t.Errorf("%s missing after orphan-delete: %v", id, err)
		}
	}
}

// TestValidateID_Rejects — id с path separator / spaces / unicode → error.
func TestValidateID_Rejects(t *testing.T) {
	cases := []string{
		"",
		"foo bar",
		"foo/bar",
		"../../etc/passwd",
		strings.Repeat("a", 200), // too long
		"привет",
	}
	for _, id := range cases {
		if err := validateID(id); err == nil {
			t.Errorf("validateID(%q) accepted, want reject", id)
		}
	}
}

// TestValidateID_Accepts — стандартные ULID-формы и тестовые "id-N" формы.
func TestValidateID_Accepts(t *testing.T) {
	cases := []string{
		"01HQRX9P3DQM2BR4Y8WG3SE0XK",
		"id-1",
		"id-100",
		"foo_bar",
	}
	for _, id := range cases {
		if err := validateID(id); err != nil {
			t.Errorf("validateID(%q) rejected: %v", id, err)
		}
	}
}
