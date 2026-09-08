package debugapi

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// TestGoroutines_DumpsAllStacks — GET /debug/goroutines отдаёт text/plain
// дамп всех горутин (в нём есть главная goroutine 1 и http-обработчик), без
// токена — 401, POST — 405.
func TestGoroutines_DumpsAllStacks(t *testing.T) {
	base, tok := snapshotServer(t, snapshotLayout(t))

	req, _ := http.NewRequest(http.MethodGet, base+"/debug/goroutines", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: want 200, got %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("content-type: want text/plain, got %q", ct)
	}
	if resp.Header.Get("X-Goroutines") == "" {
		t.Errorf("X-Goroutines header missing")
	}
	dump := string(body)
	if !strings.Contains(dump, "goroutine 1 [") {
		t.Errorf("dump lacks main goroutine:\n%s", dump)
	}
	if !strings.Contains(dump, "handleGoroutines") {
		t.Errorf("dump lacks the serving handler itself (is it really all goroutines?)")
	}

	// Без токена — 401.
	r2, err := http.Get(base + "/debug/goroutines")
	if err != nil {
		t.Fatalf("GET no-auth: %v", err)
	}
	_ = r2.Body.Close()
	if r2.StatusCode != http.StatusUnauthorized {
		t.Errorf("no-auth status: want 401, got %d", r2.StatusCode)
	}

	// POST — 405.
	req3, _ := http.NewRequest(http.MethodPost, base+"/debug/goroutines", nil)
	req3.Header.Set("Authorization", "Bearer "+tok)
	r3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = r3.Body.Close()
	if r3.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST status: want 405, got %d", r3.StatusCode)
	}
}
