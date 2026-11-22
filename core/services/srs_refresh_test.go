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
