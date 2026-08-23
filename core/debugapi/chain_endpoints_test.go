package debugapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

// TestChainDeltasCost — цена хопа есть разность соседних префиксов, и
// отрицательный шум сводится к нулю только в clamped-поле: сырая цифра
// обязана доезжать до клиента как есть.
func TestChainDeltasCost(t *testing.T) {
	run := probeRun{Layers: []probeLayer{
		{Pos: 0, Tag: "warp-awg", DelayMs: 1224},
		{Pos: 1, Tag: "vpn2", DelayMs: 162},
		{Pos: 2, Tag: "warp-masque", DelayMs: 7617},
	}}
	got := runDeltas(run).Deltas
	if len(got) != 3 {
		t.Fatalf("deltas: want 3, got %d", len(got))
	}
	if got[0].CostMs != 1224 {
		t.Errorf("pos0: want 1224, got %d", got[0].CostMs)
	}
	// 162-1224 — отрицательная: сырая сохраняется, показ обнуляется.
	if got[1].CostMs != -1062 || got[1].CostClamped != 0 {
		t.Errorf("pos1: want raw -1062 clamped 0, got %d/%d", got[1].CostMs, got[1].CostClamped)
	}
	if got[2].CostMs != 7455 || got[2].CostClamped != 7455 {
		t.Errorf("pos2: want 7455, got %d/%d", got[2].CostMs, got[2].CostClamped)
	}
	w := worstLayer(runDeltas(run))
	if w == nil || w.Pos != 2 {
		t.Fatalf("worst: want pos 2, got %+v", w)
	}
}

// TestChainDeltasUnmeasured — упавшая проба не превращается в «бесплатный
// хоп»: ни у самой позиции, ни у следующей, потерявшей опорную точку.
func TestChainDeltasUnmeasured(t *testing.T) {
	run := runDeltas(probeRun{Layers: []probeLayer{
		{Pos: 0, DelayMs: 100},
		{Pos: 1, Error: "chain[c] #1: handshake timeout"},
		{Pos: 2, DelayMs: 900},
	}})
	if !run.Deltas[1].Unmeasured {
		t.Errorf("failed layer must be unmeasured: %+v", run.Deltas[1])
	}
	if !run.Deltas[2].Unmeasured {
		t.Errorf("layer after a failed one has no baseline: %+v", run.Deltas[2])
	}
	if w := worstLayer(run); w != nil && w.Unmeasured {
		t.Errorf("worst must not be an unmeasured layer: %+v", w)
	}
}

// TestChainWorstSkipsTransparent — в схлопнутой позиции выбран direct, цены у
// неё нет по построению; обвинять её нельзя.
func TestChainWorstSkipsTransparent(t *testing.T) {
	run := runDeltas(probeRun{Layers: []probeLayer{
		{Pos: 0, DelayMs: 50},
		{Pos: 1, Transparent: true, DelayMs: 3000},
		{Pos: 2, DelayMs: 3100},
	}})
	w := worstLayer(run)
	if w == nil || w.Pos != 2 {
		t.Fatalf("worst: want pos 2 (transparent skipped), got %+v", w)
	}
}

// TestLayerTag — тег префикса собирается ровно как ждёт ядро, включая теги с
// эмодзи и пробелами.
func TestLayerTag(t *testing.T) {
	if got := layerTag("chain-1", 2); got != "chain-1#2" {
		t.Errorf("layerTag: %q", got)
	}
	if got := layerTag("🔥 цепь ②", 0); got != "🔥 цепь ②#0" {
		t.Errorf("layerTag unicode: %q", got)
	}
}

// TestChainProbeRoutesHardTag — тег с эмодзи, пробелом и решёткой доезжает до
// обработчика через path-параметр. Без демона ответ 503, но 404 здесь означал
// бы, что маршрут не сматчился, — а это и есть проверяемое.
func TestChainProbeRoutesHardTag(t *testing.T) {
	s, base := newChainTestServer(t)
	_ = s
	tag := "🔥 цепь ② #x"
	u := base + "/chains/" + url.PathEscape(tag) + "/probe"
	resp, err := http.DefaultClient.Do(authedReq(t, "POST", u, []byte(`{"repeat":1}`)))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("route did not match escaped tag")
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("want 503 (no daemon in tests), got %d", resp.StatusCode)
	}
}

// TestChainsEndpointRegistered — /chains виден в discovery ровно тогда, когда
// включена группа демона: без неё данных о цепочках взять неоткуда.
func TestChainsEndpointRegistered(t *testing.T) {
	s, base := newChainTestServer(t)
	_ = s
	resp, err := http.DefaultClient.Do(authedReq(t, "GET", base+"/help", nil))
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var help struct {
		Endpoints []struct {
			Path string `json:"path"`
		} `json:"endpoints"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&help); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := map[string]bool{"/chains": false, "/chains/{tag}/probe": false}
	for _, e := range help.Endpoints {
		if _, ok := want[e.Path]; ok {
			want[e.Path] = true
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("%s missing from /help", path)
		}
	}
}

func newChainTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	port := freeLocalPort(t)
	s, err := New(&fakeFacade{}, port, "tok")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.EnableDaemon(&fakeDaemonFacade{mode: "daemon"})
	s.Start()
	t.Cleanup(s.Stop)
	return s, "http://127.0.0.1:" + itoa(port)
}
