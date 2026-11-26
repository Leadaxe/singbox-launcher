package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"singbox-launcher/internal/srstag"
)

// TestRefreshSRS — обновление набора (SPEC 185): устаревший файл заменяется,
// битый ответ (страница вместо набора) прежний файл не портит, 304 продлевает
// свежесть, отсутствующий файл скачивается.
func TestRefreshSRS(t *testing.T) {
	prev := CreateHTTPClientFunc
	CreateHTTPClientFunc = nil
	t.Cleanup(func() { CreateHTTPClientFunc = prev })

	body := "SRS\x03fresh"
	status := http.StatusOK
	var sawConditional bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawConditional = r.Header.Get("If-Modified-Since") != ""
		w.WriteHeader(status)
		if status == http.StatusOK {
			_, _ = w.Write([]byte(body))
		}
	}))
	defer srv.Close()

	path := filepath.Join(t.TempDir(), "rule-sets", "list.srs")
	read := func() string {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		return string(b)
	}
	age := func(d time.Duration) {
		t.Helper()
		old := time.Now().Add(-d)
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}

	// Файла нет — обычная загрузка, без условного заголовка.
	changed, err := RefreshSRS(context.Background(), srv.URL, path)
	if err != nil || !changed || sawConditional {
		t.Fatalf("first download: changed=%v conditional=%v err=%v", changed, sawConditional, err)
	}
	if read() != body {
		t.Fatalf("first download: content %q", read())
	}

	// Устаревший файл заменяется новым содержимым.
	age(200 * time.Hour)
	body = "SRS\x03newer"
	changed, err = RefreshSRS(context.Background(), srv.URL, path)
	if err != nil || !changed || !sawConditional {
		t.Fatalf("refresh: changed=%v conditional=%v err=%v", changed, sawConditional, err)
	}
	if read() != "SRS\x03newer" {
		t.Fatalf("refresh: content %q", read())
	}

	// Битый ответ со статусом 200 прежний файл не трогает.
	body = "<html>blocked</html>"
	changed, err = RefreshSRS(context.Background(), srv.URL, path)
	if !errors.Is(err, ErrSRSInvalidContent) || changed {
		t.Fatalf("invalid body: changed=%v err=%v", changed, err)
	}
	if read() != "SRS\x03newer" {
		t.Fatalf("invalid body replaced the file: %q", read())
	}
	if _, statErr := os.Stat(path + ".tmp"); !os.IsNotExist(statErr) {
		t.Fatalf("temp file left behind: %v", statErr)
	}

	// 304 — файл тот же, свежесть продлена.
	age(200 * time.Hour)
	status = http.StatusNotModified
	changed, err = RefreshSRS(context.Background(), srv.URL, path)
	if err != nil || changed {
		t.Fatalf("not modified: changed=%v err=%v", changed, err)
	}
	mod, ok := SRSFileModTime(path)
	if !ok || time.Since(mod) > time.Minute {
		t.Fatalf("not modified: mtime not refreshed (%v)", mod)
	}

	// Сбой сервера прежний файл не трогает.
	status = http.StatusBadGateway
	if _, err = RefreshSRS(context.Background(), srv.URL, path); err == nil {
		t.Fatal("HTTP 502: expected an error")
	}
	if read() != "SRS\x03newer" {
		t.Fatalf("HTTP 502 replaced the file: %q", read())
	}
}

// TestRefreshSRSFileRef — набор из локального файла (file://, SPEC 187):
// копия создаётся под именем от ссылки, правка исходника доезжает до копии,
// неизменённый исходник копию не трогает, пропавший — не портит, а не-JSON
// под ссылкой .json отвергается до замены.
func TestRefreshSRSFileRef(t *testing.T) {
	srcDir, dstDir := t.TempDir(), t.TempDir()
	src := filepath.Join(srcDir, "my rules.json")
	if err := os.WriteFile(src, []byte(`{"version":3,"rules":[{"domain":["a.example"]}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ref, err := srstag.FileRefFromPath(filepath.ToSlash(src))
	if err != nil {
		t.Fatal(err)
	}
	if got := srstag.FormatFromRef(ref); got != srstag.FormatSource {
		t.Fatalf("format = %q, want source", got)
	}
	tag := srstag.TagFromURL(ref)
	name := srstag.FileName(tag)
	if filepath.Ext(name) != ".json" || srstag.TagFromFileName(name) != tag {
		t.Fatalf("file name %q does not round-trip tag %q", name, tag)
	}
	dst := filepath.Join(dstDir, name)
	ctx := context.Background()

	changed, err := RefreshSRS(ctx, ref, dst)
	if err != nil || !changed {
		t.Fatalf("first copy: changed=%v err=%v", changed, err)
	}
	if changed, err = RefreshSRS(ctx, ref, dst); err != nil || changed {
		t.Fatalf("unchanged source: changed=%v err=%v", changed, err)
	}

	body2 := `{"version":3,"rules":[{"domain":["a.example","b.example"]}]}`
	if err := os.WriteFile(src, []byte(body2), 0o644); err != nil {
		t.Fatal(err)
	}
	if changed, err = RefreshSRS(ctx, ref, dst); err != nil || !changed {
		t.Fatalf("edited source: changed=%v err=%v", changed, err)
	}
	if got, _ := os.ReadFile(dst); string(got) != body2 {
		t.Fatalf("copy = %q", got)
	}

	// Не JSON под ссылкой .json: копия остаётся прежней.
	if err := os.WriteFile(src, []byte("SRS\x03binary-under-json-name"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err = RefreshSRS(ctx, ref, dst); !errors.Is(err, ErrSRSInvalidContent) {
		t.Fatalf("binary under .json: err=%v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != body2 {
		t.Fatalf("copy damaged by a rejected source: %q", got)
	}

	// Исходник пропал: ошибка названа, копия цела.
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if _, err = RefreshSRS(ctx, ref, dst); !errors.Is(err, ErrSRSSourceMissing) {
		t.Fatalf("missing source: err=%v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != body2 {
		t.Fatalf("copy damaged by a missing source: %q", got)
	}

	// Путь Windows и сетевой путь — строковая логика, без файловой системы.
	if got, _ := srstag.FileRefFromPath(`C:\Rules\blocked.srs`); got != "file:///C:/Rules/blocked.srs" {
		t.Fatalf("windows path ref = %q", got)
	}
	if got, _ := srstag.PathFromFileRef("file:///C:/Rules/blocked.srs"); got != "C:/Rules/blocked.srs" {
		t.Fatalf("windows ref path = %q", got)
	}
	if _, err := srstag.FileRefFromPath(`\\server\share\x.srs`); !errors.Is(err, srstag.ErrNetworkPath) {
		t.Fatalf("network path: err=%v", err)
	}
	if got := srstag.FileName(srstag.TagFromURL("https://example.com/a/list.srs")); filepath.Ext(got) != ".srs" {
		t.Fatalf("srs url file name = %q", got)
	}
}
