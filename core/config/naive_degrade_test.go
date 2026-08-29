package config

import (
	"encoding/json"
	"strings"
	"testing"

	"singbox-launcher/core/config/configtypes"
)

// SPEC 044 feature-probe: when the running core can't create naive outbounds,
// the generator must degrade naive nodes (drop + count) instead of emitting a
// config that `sing-box check` rejects wholesale.

func naiveDegradeParserConfig() *ParserConfig {
	pc := &ParserConfig{}
	pc.ParserConfig.Version = ParserConfigVersion
	pc.ParserConfig.Proxies = []ProxySource{{Source: "https://example.com/sub"}}
	return pc
}

// seedCanonicalNodes кладёт готовые узлы в КАНОН первого источника — так же,
// как это делает состояние v7 (SPEC 118 Т5: конвейер сборки парсер тел не
// зовёт, узлы приезжают материализованными).
//
// Тело собирается тем же эмиттером, что и в бою (EmitNodeJSONs), и очищается
// от tag/detour: их владелец — модель, и сборка возвращает их на место сама.
func seedCanonicalNodes(t *testing.T, pc *ParserConfig, nodes []*ParsedNode) {
	t.Helper()
	if pc == nil || len(pc.ParserConfig.Proxies) == 0 {
		return
	}
	canon := make([]configtypes.CanonicalNode, 0, len(nodes))
	for _, n := range nodes {
		outs, ep, err := EmitNodeJSONs(n)
		if err != nil {
			t.Fatalf("seedCanonicalNodes: эмиссия узла %q: %v", n.Tag, err)
		}
		raw := ep
		if raw == "" && len(outs) > 0 {
			raw = outs[len(outs)-1]
		}
		// Эмиттер пишет строку узла как элемент массива: перед объектом
		// может стоять комментарий-подпись, после — запятая.
		if i := strings.Index(raw, "{"); i >= 0 {
			raw = raw[i:]
		}
		raw = strings.TrimRight(strings.TrimSpace(raw), ",")
		body, err := stripTagAndDetour(json.RawMessage(raw))
		if err != nil {
			t.Fatalf("seedCanonicalNodes: тело узла %q: %v", n.Tag, err)
		}
		canon = append(canon, configtypes.CanonicalNode{
			Kind: "server", Tag: n.Tag, Enabled: true, Body: body,
		})
	}
	pc.ParserConfig.Proxies[0].Canonical = &configtypes.CanonicalSource{
		FolderID: "F1", IsContainer: true, Nodes: canon,
	}
}

func testSocksNode(tag string) *ParsedNode {
	return &ParsedNode{Tag: tag, Scheme: "socks", Server: "10.0.0.1", Port: 1080}
}

func testNaiveNode(tag string) *ParsedNode {
	return &ParsedNode{
		Tag: tag, Scheme: "naive", Server: "example.com", Port: 443, UUID: "user",
		Outbound: map[string]interface{}{
			"tls": map[string]interface{}{"enabled": true, "server_name": "example.com"},
		},
	}
}

func withNaiveProbe(t *testing.T, probe func() (bool, string)) {
	t.Helper()
	prev := NaiveSupportProbe
	NaiveSupportProbe = probe
	t.Cleanup(func() { NaiveSupportProbe = prev })
}

func TestGenerateOutbounds_NaiveDegradedWhenUnsupported(t *testing.T) {
	withNaiveProbe(t, func() (bool, string) { return false, "core built without with_naive_outbound" })

	nodes := []*ParsedNode{testSocksNode("socks-1"), testNaiveNode("naive-1")}
	result, err := generateWithCanonicalNodes(t, naiveDegradeParserConfig(), nodes, DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("GenerateOutboundsFromParserConfig: %v", err)
	}

	if result.SkippedNaiveNodes != 1 {
		t.Errorf("SkippedNaiveNodes = %d, want 1", result.SkippedNaiveNodes)
	}
	if !strings.Contains(result.SkippedNaiveReason, "with_naive_outbound") {
		t.Errorf("SkippedNaiveReason = %q, want probe reason", result.SkippedNaiveReason)
	}
	all := strings.Join(result.OutboundsJSON, "\n")
	if strings.Contains(all, "naive-1") {
		t.Errorf("naive node leaked into OutboundsJSON:\n%s", all)
	}
	if !strings.Contains(all, "socks-1") {
		t.Errorf("socks node missing from OutboundsJSON:\n%s", all)
	}
}

func TestGenerateOutbounds_NaiveKeptWhenSupported(t *testing.T) {
	withNaiveProbe(t, func() (bool, string) { return true, "" })

	nodes := []*ParsedNode{testSocksNode("socks-1"), testNaiveNode("naive-1")}
	result, err := generateWithCanonicalNodes(t, naiveDegradeParserConfig(), nodes, DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("GenerateOutboundsFromParserConfig: %v", err)
	}

	if result.SkippedNaiveNodes != 0 {
		t.Errorf("SkippedNaiveNodes = %d, want 0", result.SkippedNaiveNodes)
	}
	all := strings.Join(result.OutboundsJSON, "\n")
	if !strings.Contains(all, "naive-1") {
		t.Errorf("naive node missing from OutboundsJSON:\n%s", all)
	}
}

func TestGenerateOutbounds_NilProbeAssumesSupported(t *testing.T) {
	withNaiveProbe(t, nil)

	nodes := []*ParsedNode{testNaiveNode("naive-1")}
	result, err := generateWithCanonicalNodes(t, naiveDegradeParserConfig(), nodes, DirectionBuildOptions{})
	if err != nil {
		t.Fatalf("GenerateOutboundsFromParserConfig: %v", err)
	}
	if result.SkippedNaiveNodes != 0 {
		t.Errorf("SkippedNaiveNodes = %d, want 0 with nil probe", result.SkippedNaiveNodes)
	}
}

// A source whose every node degrades must not be reported as silent-empty
// failure, and an all-naive run must fail with a message naming the cause.
func TestGenerateOutbounds_AllNaiveGivesActionableError(t *testing.T) {
	withNaiveProbe(t, func() (bool, string) { return false, "core built without with_naive_outbound" })

	nodes := []*ParsedNode{testNaiveNode("naive-1"), testNaiveNode("naive-2")}
	_, err := generateWithCanonicalNodes(t, naiveDegradeParserConfig(), nodes, DirectionBuildOptions{})
	if err == nil {
		t.Fatal("want error when every node degraded, got nil")
	}
	if !strings.Contains(err.Error(), "naive") {
		t.Errorf("error = %q, want it to name the naive degradation", err)
	}
}

// generateWithCanonicalNodes — сборка по готовым узлам: узлы кладутся в канон
// первого источника, и конвейер эмитит их оттуда (SPEC 118 Т5).
func generateWithCanonicalNodes(t *testing.T, pc *ParserConfig, nodes []*ParsedNode, opts DirectionBuildOptions) (*OutboundGenerationResult, error) {
	t.Helper()
	seedCanonicalNodes(t, pc, nodes)
	return GenerateOutboundsFromParserConfig(pc, map[string]int{}, nil, opts)
}
