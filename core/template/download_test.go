package template

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// minimalTemplate — самый маленький шаблон, который проходит LoadTemplateData.
// Нужен, чтобы отличить «скачали и прочитали» от «скачали мусор».
const minimalTemplate = `{
  "parser_config": {},
  "config": {"outbounds": [], "route": {"final": "direct"}},
  "params": [],
  "vars": []
}`

func fetcherReturning(body string, status int, err error) (URLFetcher, *int) {
	calls := 0
	return func(ctx context.Context, url string, timeout time.Duration) ([]byte, int, error) {
		calls++
		if err != nil {
			return nil, 0, err
		}
		return []byte(body), status, nil
	}, &calls
}

func templatePath(t *testing.T, execDir string) string {
	t.Helper()
	return filepath.Join(execDir, "bin", GetTemplateFileName())
}

func TestDownloadTemplate_WritesFileAndMarksInstall(t *testing.T) {
	execDir := t.TempDir()
	fetch, calls := fetcherReturning(minimalTemplate, http.StatusOK, nil)

	got, err := DownloadTemplate(context.Background(), execDir, fetch)
	if err != nil {
		t.Fatalf("DownloadTemplate: %v", err)
	}
	if *calls != 1 {
		t.Fatalf("fetcher called %d times, want 1", *calls)
	}
	if want := templatePath(t, execDir); got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
	data, readErr := os.ReadFile(got)
	if readErr != nil {
		t.Fatalf("template not written: %v", readErr)
	}
	if string(data) != minimalTemplate {
		t.Fatalf("written body mismatch: %q", string(data))
	}
}

func TestDownloadTemplate_HTTPErrorCarriesReason(t *testing.T) {
	execDir := t.TempDir()
	netErr := errors.New("dial tcp: connection refused")
	fetch, _ := fetcherReturning("", 0, netErr)

	_, err := DownloadTemplate(context.Background(), execDir, fetch)
	if err == nil {
		t.Fatal("expected an error")
	}
	// Именно это и было сломано в баге: диалог показывался с пустой причиной.
	if !errors.Is(err, netErr) {
		t.Fatalf("error must wrap the transport failure, got %v", err)
	}
	if _, statErr := os.Stat(templatePath(t, execDir)); !os.IsNotExist(statErr) {
		t.Fatal("failed download must not leave a file behind")
	}
}

func TestDownloadTemplate_NonOKStatusIsReported(t *testing.T) {
	execDir := t.TempDir()
	fetch, _ := fetcherReturning("not found", http.StatusNotFound, nil)

	_, err := DownloadTemplate(context.Background(), execDir, fetch)
	if err == nil {
		t.Fatal("expected an error on HTTP 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("reason must name the status, got %q", err.Error())
	}
}

func TestDownloadTemplate_EmptyBodyIsRejected(t *testing.T) {
	execDir := t.TempDir()
	fetch, _ := fetcherReturning("", http.StatusOK, nil)

	if _, err := DownloadTemplate(context.Background(), execDir, fetch); err == nil {
		t.Fatal("empty body must not be installed as a template")
	}
	if _, statErr := os.Stat(templatePath(t, execDir)); !os.IsNotExist(statErr) {
		t.Fatal("empty body must not create a file")
	}
}

func TestDownloadTemplate_NoFetcher(t *testing.T) {
	if _, err := DownloadTemplate(context.Background(), t.TempDir(), nil); err == nil {
		t.Fatal("nil fetcher must be an error, not a panic")
	}
}

// Файл на месте → сети не касаемся вовсе (быстрый путь открытия Мастера).
func TestEnsureTemplate_PresentFileSkipsDownload(t *testing.T) {
	execDir := t.TempDir()
	path := templatePath(t, execDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(minimalTemplate), 0o644); err != nil {
		t.Fatal(err)
	}
	fetch, calls := fetcherReturning(minimalTemplate, http.StatusOK, nil)

	data, downloaded, err := EnsureTemplate(context.Background(), execDir, fetch)
	if err != nil {
		t.Fatalf("EnsureTemplate: %v", err)
	}
	if downloaded {
		t.Fatal("must not download when the template is already readable")
	}
	if *calls != 0 {
		t.Fatalf("fetcher called %d times, want 0", *calls)
	}
	if data == nil {
		t.Fatal("template data must be returned")
	}
}

// Регрессия бага: файла нет → СНАЧАЛА попытка скачать, и только её провал —
// это отказ. Раньше отсутствие файла сразу давало «Download failed».
func TestEnsureTemplate_MissingFileDownloadsThenLoads(t *testing.T) {
	execDir := t.TempDir()
	fetch, calls := fetcherReturning(minimalTemplate, http.StatusOK, nil)

	data, downloaded, err := EnsureTemplate(context.Background(), execDir, fetch)
	if err != nil {
		t.Fatalf("EnsureTemplate: %v", err)
	}
	if !downloaded {
		t.Fatal("downloaded flag must be set when the file was fetched")
	}
	if *calls != 1 {
		t.Fatalf("fetcher called %d times, want 1", *calls)
	}
	if data == nil {
		t.Fatal("template data must be returned after a successful download")
	}
	if _, statErr := os.Stat(templatePath(t, execDir)); statErr != nil {
		t.Fatalf("template must be on disk after EnsureTemplate: %v", statErr)
	}
}

func TestEnsureTemplate_MissingFileDownloadFailureIsReported(t *testing.T) {
	execDir := t.TempDir()
	netErr := errors.New("i/o timeout")
	fetch, calls := fetcherReturning("", 0, netErr)

	_, downloaded, err := EnsureTemplate(context.Background(), execDir, fetch)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !downloaded {
		t.Fatal("a download was attempted — the flag must say so")
	}
	if *calls != 1 {
		t.Fatalf("fetcher called %d times, want 1", *calls)
	}
	if !errors.Is(err, netErr) {
		t.Fatalf("failure reason must survive to the caller, got %v", err)
	}
}

// Скачали, но приехал не шаблон: причина обязана указывать на содержимое, а
// не на сеть — иначе пользователь чинит не то.
func TestEnsureTemplate_DownloadedGarbageReportsParseError(t *testing.T) {
	execDir := t.TempDir()
	fetch, _ := fetcherReturning("<html>404</html>", http.StatusOK, nil)

	_, downloaded, err := EnsureTemplate(context.Background(), execDir, fetch)
	if err == nil {
		t.Fatal("unparseable template must be an error")
	}
	if !downloaded {
		t.Fatal("downloaded flag must be set")
	}
	if err.Error() == "" {
		t.Fatal("reason must not be empty")
	}
}
