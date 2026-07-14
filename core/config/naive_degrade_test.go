package config

import (
	"strings"
	"testing"
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

func naiveDegradeLoadNodes(nodes []*ParsedNode) func(ProxySource, map[string]int, func(float64, string), int, int) ([]*ParsedNode, error) {
	return func(ProxySource, map[string]int, func(float64, string), int, int) ([]*ParsedNode, error) {
		return nodes, nil
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
	result, err := GenerateOutboundsFromParserConfig(
		naiveDegradeParserConfig(), map[string]int{}, nil, naiveDegradeLoadNodes(nodes))
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
	result, err := GenerateOutboundsFromParserConfig(
		naiveDegradeParserConfig(), map[string]int{}, nil, naiveDegradeLoadNodes(nodes))
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
	result, err := GenerateOutboundsFromParserConfig(
		naiveDegradeParserConfig(), map[string]int{}, nil, naiveDegradeLoadNodes(nodes))
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
	_, err := GenerateOutboundsFromParserConfig(
		naiveDegradeParserConfig(), map[string]int{}, nil, naiveDegradeLoadNodes(nodes))
	if err == nil {
		t.Fatal("want error when every node degraded, got nil")
	}
	if !strings.Contains(err.Error(), "naive") {
		t.Errorf("error = %q, want it to name the naive degradation", err)
	}
}
