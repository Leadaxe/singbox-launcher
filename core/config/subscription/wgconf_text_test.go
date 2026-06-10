package subscription

import (
	"strings"
	"testing"
)

func TestExtractWGConfBlocks(t *testing.T) {
	t.Run("no blocks", func(t *testing.T) {
		input := "vless://uuid@host:443#x\nhttps://example.com/sub"
		rest, blocks := ExtractWGConfBlocks(input)
		if rest != input || len(blocks) != 0 {
			t.Errorf("plain links must pass through untouched: rest=%q blocks=%d", rest, len(blocks))
		}
	})

	t.Run("links plus two blocks", func(t *testing.T) {
		input := "vless://uuid@host:443#x\n" + amneziaAWGIni + "\n" + amneziaPlainWGIni
		rest, blocks := ExtractWGConfBlocks(input)
		if !strings.Contains(rest, "vless://") || strings.Contains(rest, "[Interface]") {
			t.Errorf("rest must keep the link and drop conf text: %q", rest)
		}
		if len(blocks) != 2 {
			t.Fatalf("blocks = %d, want 2", len(blocks))
		}
		if !strings.Contains(blocks[0], "Jc = 4") || !strings.Contains(blocks[1], "MTU = 1380") {
			t.Errorf("blocks split at the wrong boundary")
		}
	})

	t.Run("case-insensitive section", func(t *testing.T) {
		_, blocks := ExtractWGConfBlocks("[interface]\nPrivateKey = x\n[Peer]\nEndpoint = h:1")
		if len(blocks) != 1 {
			t.Errorf("[interface] (lower case) must start a block")
		}
	})
}

func TestConvertWGConfText_AWG(t *testing.T) {
	uri, err := ConvertWGConfText(amneziaAWGIni)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("converted URI must parse: %v (uri=%s)", err, uri)
	}
	if node.Server != "203.0.113.7" || node.Port != 38291 {
		t.Errorf("endpoint = %s:%d, want 203.0.113.7:38291", node.Server, node.Port)
	}
	// Label (and thus tag) comes from the Endpoint host — a pasted .conf has no name.
	if node.Tag != "203.0.113.7" {
		t.Errorf("Tag = %q, want endpoint host", node.Tag)
	}
	if got, _ := node.Outbound["jc"].(int64); got != 4 {
		t.Errorf("jc = %v, want 4", node.Outbound["jc"])
	}
	if got, _ := node.Outbound["mtu"].(int); got != 1280 {
		t.Errorf("mtu = %v, want clamped 1280", node.Outbound["mtu"])
	}
}

func TestConvertWGConfText_IPv6Endpoint(t *testing.T) {
	conf := strings.Replace(amneziaPlainWGIni, "Endpoint = 198.51.100.4:51820", "Endpoint = [2001:db8::1]:51820", 1)
	uri, err := ConvertWGConfText(conf)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	node, err := ParseNode(uri, nil)
	if err != nil || node == nil {
		t.Fatalf("converted URI must parse: %v (uri=%s)", err, uri)
	}
	if node.Server != "2001:db8::1" || node.Port != 51820 {
		t.Errorf("endpoint = %s:%d, want [2001:db8::1]:51820", node.Server, node.Port)
	}
	if node.Tag != "2001:db8::1" {
		t.Errorf("Tag = %q, want bracket-free IPv6 host", node.Tag)
	}
}

func TestConvertWGConfText_Invalid(t *testing.T) {
	for name, conf := range map[string]string{
		"no peer":     "[Interface]\nPrivateKey = x\nAddress = 10.0.0.2/32",
		"no endpoint": "[Interface]\nPrivateKey = x\nAddress = 10.0.0.2/32\n[Peer]\nPublicKey = y",
		"empty":       "",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ConvertWGConfText(conf); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
