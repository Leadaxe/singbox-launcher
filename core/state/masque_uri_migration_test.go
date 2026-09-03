package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Legacy masque-URI (`?network=h2` из диалога WARP до контракта 0.8.0)
// переписывается в `?vhttp=h2` при загрузке, результат персистится, второй
// Load ничего не трогает; URI с каноническим `vhttp=` и чужие значения
// `network=` остаются байт в байт.
func TestLoad_RewritesLegacyMasqueNetworkParam(t *testing.T) {
	const legacy = "masque://PRIV@162.159.198.2:443?address=172.16.0.2%2F32&mtu=1280&network=h2&profile=cloudflare&publickey=PUB&sni=www.apple.com#%F0%9F%94%A5%20WARP"
	const canonical = "masque://PRIV@162.159.198.2:443?address=172.16.0.2%2F32&network=h2&vhttp=h3#x"
	const foreign = "masque://PRIV@1.2.3.4:443?address=172.16.0.2%2F32&network=tcp#x"
	const wg = "wireguard://PRIV@1.2.3.4:51820?publickey=PUB&address=10.0.0.2%2F32&network=h2#x"

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s := &State{Version: 7, Sources: []Source{
		{Node: Node{Kind: SourceKindServer, Tag: "🔥 WARP", Enabled: true, Origin: &Origin{Kind: OriginKindURI, Raw: legacy}}},
		{Node: Node{Kind: SourceKindServer, Tag: "canon", Enabled: true, Origin: &Origin{Kind: OriginKindURI, Raw: canonical}}},
		{Node: Node{Kind: SourceKindServer, Tag: "foreign", Enabled: true, Origin: &Origin{Kind: OriginKindURI, Raw: foreign}}},
		{Node: Node{Kind: SourceKindServer, Tag: "wg", Enabled: true, Origin: &Origin{Kind: OriginKindURI, Raw: wg}}},
		{Node: Node{Kind: SourceKindFolder, Tag: "folder"}, Nodes: []Node{
			{Kind: SourceKindServer, Tag: "inner", Enabled: true, Origin: &Origin{Kind: OriginKindURI, Raw: legacy}},
		}},
	}}
	if err := s.Save(path); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := "masque://PRIV@162.159.198.2:443?address=172.16.0.2%2F32&mtu=1280&vhttp=h2&profile=cloudflare&publickey=PUB&sni=www.apple.com#%F0%9F%94%A5%20WARP"
	if got := loaded.Sources[0].Origin.Raw; got != want {
		t.Errorf("root legacy uri:\n got %s\nwant %s", got, want)
	}
	if got := loaded.Sources[4].Nodes[0].Origin.Raw; got != want {
		t.Errorf("folder node legacy uri:\n got %s\nwant %s", got, want)
	}
	for i, exp := range map[int]string{1: canonical, 2: foreign, 3: wg} {
		if got := loaded.Sources[i].Origin.Raw; got != exp {
			t.Errorf("source %d must stay untouched:\n got %s\nwant %s", i, got, exp)
		}
	}

	// Персистировано: файл больше не несёт legacy-форму.
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "network=h2&profile") {
		t.Error("legacy ?network=h2 survived on disk after Load")
	}
	if !strings.Contains(string(data), "vhttp=h2") {
		t.Error("rewritten ?vhttp=h2 not persisted")
	}

	// Идемпотентность: второй Load ничего не переписывает.
	again, err := Load(path)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if rewriteLegacyMasqueURIs(again) != 0 {
		t.Error("second pass must be a no-op")
	}
}
