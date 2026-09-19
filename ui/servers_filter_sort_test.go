package ui

import (
	"testing"

	"singbox-launcher/api"
)

func TestVisibleProxiesAfterNameSortWithFilter(t *testing.T) {
	all := []api.ProxyInfo{
		{Name: "node-c", DisplayName: "C"},
		{Name: "node-a", DisplayName: "A"},
		{Name: "node-b", DisplayName: "B"},
		{Name: "node-d", DisplayName: "D"},
	}
	filter := serversFilterState{
		RegexBody: "A|C",
		Protocols: map[string]bool{},
		Variants:  map[string]bool{},
		Sources:   map[string]bool{},
		Test:      serversTestAny,
	}
	keep := func(name string) bool { return name == "node-b" }

	tests := []struct {
		name      string
		ascending bool
		want      []string
		selected  string
		wantIdx   int
	}{
		{
			name:      "ascending filter preserves sorted visible order",
			ascending: true,
			want:      []string{"node-a", "node-b", "node-c"},
			selected:  "node-c",
			wantIdx:   2,
		},
		{
			name:      "descending filter preserves sorted visible order",
			ascending: false,
			want:      []string{"node-c", "node-b", "node-a"},
			selected:  "node-a",
			wantIdx:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vis := visibleProxiesAfterNameSort(all, nil, filter, keep, tt.ascending)
			if got := proxyTagNames(vis); !stringSlicesEqual(got, tt.want) {
				t.Fatalf("visible order = %v, want %v", got, tt.want)
			}
			idx := selectedProxyIndex(vis, tt.selected)
			if idx != tt.wantIdx {
				t.Fatalf("selected %q index = %d, want %d", tt.selected, idx, tt.wantIdx)
			}
		})
	}
}

func proxyTagNames(list []api.ProxyInfo) []string {
	out := make([]string, len(list))
	for i := range list {
		out[i] = list[i].Name
	}
	return out
}

func selectedProxyIndex(visible []api.ProxyInfo, tag string) int {
	for i := range visible {
		if visible[i].Name == tag {
			return i
		}
	}
	return -1
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
