package debugapi

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fakeUI struct {
	cleared int
	err     error
}

func (f *fakeUI) Snapshot() (any, error) {
	if f.err != nil {
		return nil, f.err
	}
	return []map[string]any{{"title": "Main", "overlays": []string{"*fyne.Container"}}}, nil
}

func (f *fakeUI) ClearOverlays() (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.cleared++
	return 2, nil
}

func uiDo(t *testing.T, method, url, tok string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, url, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body
}

// TestUIEndpoints — группа /debug/ui/* существует только после EnableUI:
// capability "ui" в манифесте, snapshot отдаёт окна, clear возвращает счётчик,
// зависший цикл Fyne (ErrUILoopUnresponsive) — 504.
func TestUIEndpoints(t *testing.T) {
	execDir := snapshotLayout(t)

	// Без EnableUI — capability false и эндпоинта нет в /help.
	base, tok := snapshotServer(t, execDir)
	st, body := uiDo(t, http.MethodGet, base+"/", tok)
	if st != http.StatusOK || !strings.Contains(string(body), `"ui":false`) {
		t.Fatalf("манифест без EnableUI: status=%d body=%s", st, body)
	}
	if strings.Contains(string(body), "/debug/ui") {
		t.Errorf("эндпоинт /debug/ui заявлен в манифесте без EnableUI")
	}

	// С EnableUI.
	port := freeLocalPort(t)
	s, err := New(&fakeFacade{dataDir: execDir}, port, tok)
	if err != nil {
		t.Fatal(err)
	}
	ui := &fakeUI{}
	s.EnableUI(ui)
	s.Start()
	t.Cleanup(s.Stop)
	base2 := "http://127.0.0.1:" + itoa(port)

	st, body = uiDo(t, http.MethodGet, base2+"/", tok)
	if st != http.StatusOK || !strings.Contains(string(body), `"ui":true`) || !strings.Contains(string(body), "/debug/ui/overlays/clear") {
		t.Fatalf("манифест с EnableUI: status=%d body=%s", st, body)
	}

	st, body = uiDo(t, http.MethodGet, base2+"/debug/ui", tok)
	if st != http.StatusOK {
		t.Fatalf("GET /debug/ui: %d %s", st, body)
	}
	var snap struct {
		Windows []map[string]any `json:"windows"`
	}
	if err := json.Unmarshal(body, &snap); err != nil || len(snap.Windows) != 1 || snap.Windows[0]["title"] != "Main" {
		t.Fatalf("тело snapshot: err=%v body=%s", err, body)
	}

	st, body = uiDo(t, http.MethodPost, base2+"/debug/ui/overlays/clear", tok)
	if st != http.StatusOK || !strings.Contains(string(body), `"removed":2`) || ui.cleared != 1 {
		t.Fatalf("POST clear: %d %s cleared=%d", st, body, ui.cleared)
	}
	if st, _ := uiDo(t, http.MethodGet, base2+"/debug/ui/overlays/clear", tok); st != http.StatusMethodNotAllowed {
		t.Errorf("GET на clear: %d, ожидалось 405", st)
	}
	if st, _ := uiDo(t, http.MethodGet, base2+"/debug/ui", ""); st != http.StatusUnauthorized {
		t.Errorf("без токена: %d, ожидалось 401", st)
	}

	ui.err = ErrUILoopUnresponsive
	if st, _ := uiDo(t, http.MethodGet, base2+"/debug/ui", tok); st != http.StatusGatewayTimeout {
		t.Errorf("зависший цикл: %d, ожидалось 504", st)
	}
}
