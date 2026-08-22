package config

import (
	"strings"
	"testing"
)

func TestFilterNodesExcludeFromGlobal(t *testing.T) {
	proxies := []ProxySource{
		{ExcludeFromGlobal: false},
		{ExcludeFromGlobal: true},
	}
	nodes := []*ParsedNode{
		{Tag: "a", SourceIndex: 0},
		{Tag: "b", SourceIndex: 1},
		{Tag: "c", SourceIndex: UnsetSourceIndex},
	}
	out := FilterNodesExcludeFromGlobal(nodes, proxies)
	if len(out) != 2 {
		t.Fatalf("want 2 nodes, got %d", len(out))
	}
	if out[0].Tag != "a" || out[1].Tag != "c" {
		t.Fatalf("unexpected filter result: %#v", out)
	}
}

func TestCollectExposeTagCandidates(t *testing.T) {
	pc := &ParserConfig{}
	pc.ParserConfig.Proxies = []ProxySource{
		{
			ExposeGroupTagsToGlobal: true,
			Outbounds: []Direction{
				{Tag: "1:auto", Comment: "x WIZARD:auto"},
				{Tag: "noise", Comment: "no marker"},
			},
		},
	}
	cands := collectExposeTagCandidates(pc)
	if len(cands) != 1 || cands[0].Tag != "1:auto" {
		t.Fatalf("candidates: %#v", cands)
	}
}

func TestSelectorFiltersAcceptSyntheticHostRejectsExpose(t *testing.T) {
	filter := map[string]interface{}{"host": "/example/i"}
	syn := ExposeTagSyntheticNode("1:auto", "WIZARD:auto")
	if SelectorFiltersAcceptNode(filter, syn) {
		t.Fatal("synthetic with empty host should not match host filter")
	}
}

func TestSelectorFiltersAcceptSyntheticCommentWIZARD(t *testing.T) {
	filter := map[string]interface{}{"comment": "/WIZARD:auto/i"}
	syn := ExposeTagSyntheticNode("1:auto", "local WIZARD:auto")
	if !SelectorFiltersAcceptNode(filter, syn) {
		t.Fatal("expected WIZARD:auto in comment to match")
	}
}

func TestGenerateSelectorWithExposeGlobalOnly(t *testing.T) {
	pc := &ParserConfig{}
	pc.ParserConfig.Proxies = []ProxySource{
		{
			ExposeGroupTagsToGlobal: true,
			Outbounds: []Direction{
				{Tag: "1:auto", Type: "urltest", Comment: "WIZARD:auto", Filters: map[string]interface{}{}},
			},
		},
	}
	pc.ParserConfig.Outbounds = []Direction{
		{Tag: "g", Type: "selector", Filters: map[string]interface{}{}, AddOutbounds: []string{"direct-out"}},
	}
	nodes := []*ParsedNode{{Tag: "n1", SourceIndex: 0}}
	info := map[string]*outboundInfo{
		"1:auto": {
			config:        pc.ParserConfig.Proxies[0].Outbounds[0],
			filteredNodes: []*ParsedNode{{Tag: "x"}},
			outboundCount: 1,
			isValid:       true,
			isLocal:       true,
		},
		"g": {
			config:        pc.ParserConfig.Outbounds[0],
			filteredNodes: nodes,
			outboundCount: 2,
			isValid:       true,
			isLocal:       false,
		},
	}
	cands := collectExposeTagCandidates(pc)
	json, err := GenerateSelectorWithFilteredAddOutbounds(nodes, pc.ParserConfig.Outbounds[0], info, true, cands)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(json, `"1:auto"`) {
		t.Fatalf("expected expose tag in output: %s", json)
	}
}
