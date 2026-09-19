package subscription

import (
	"testing"
)

func TestParseNodesFromXrayJSONArray_DialerProxyFreedomFragment(t *testing.T) {
	const baseOutbounds = `
		  {"tag": "fragment", "protocol": "freedom", "settings": {"fragment": {"packets": "tlshello", "length": "100-200", "interval": "10-20"}}},
		  {"tag": "direct", "protocol": "freedom"},
		  {"tag": "block", "protocol": "blackhole"}`

	tests := []struct {
		name         string
		proxyStream  string
		wantNodes    int
		wantFragment bool
		wantChain    int
	}{
		{
			name: "tls with fragment freedom",
			proxyStream: `{
			  "network": "tcp",
			  "security": "tls",
			  "tlsSettings": { "serverName": "sni.example" },
			  "sockopt": { "dialerProxy": "fragment" }
			}`,
			wantNodes:    1,
			wantFragment: true,
			wantChain:    0,
		},
		{
			name: "reality with fragment freedom",
			proxyStream: `{
			  "network": "tcp",
			  "security": "reality",
			  "realitySettings": { "publicKey": "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX", "serverName": "sni.example", "shortId": "01" },
			  "sockopt": { "dialerProxy": "fragment" }
			}`,
			wantNodes:    1,
			wantFragment: true,
			wantChain:    0,
		},
		{
			name: "plain freedom relay ignored",
			proxyStream: `{
			  "network": "tcp",
			  "security": "tls",
			  "tlsSettings": { "serverName": "sni.example" },
			  "sockopt": { "dialerProxy": "direct" }
			}`,
			wantNodes:    1,
			wantFragment: false,
			wantChain:    0,
		},
		{
			name: "fragment freedom without tls",
			proxyStream: `{
			  "network": "tcp",
			  "security": "none",
			  "sockopt": { "dialerProxy": "fragment" }
			}`,
			wantNodes:    1,
			wantFragment: false,
			wantChain:    0,
		},
		{
			name: "blackhole dialer rejected",
			proxyStream: `{
			  "network": "tcp",
			  "security": "tls",
			  "tlsSettings": { "serverName": "sni.example" },
			  "sockopt": { "dialerProxy": "block" }
			}`,
			wantNodes: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := `[{
			  "remarks": "frag-test",
			  "outbounds": [
				{
				  "protocol": "vless",
				  "tag": "proxy",
				  "settings": {
					"vnext": [{ "address": "node.example", "port": 443, "users": [{ "id": "11111111-1111-1111-1111-111111111111", "encryption": "none" }] }]
				  },
				  "streamSettings": ` + tc.proxyStream + `
				},` + baseOutbounds + `
			  ]
			}]`
			nodes, err := ParseNodesFromXrayJSONArray(raw, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(nodes) != tc.wantNodes {
				t.Fatalf("nodes: got %d want %d", len(nodes), tc.wantNodes)
			}
			if tc.wantNodes == 0 {
				return
			}
			n := nodes[0]
			if len(n.Chain) != tc.wantChain {
				t.Fatalf("chain len: got %d want %d", len(n.Chain), tc.wantChain)
			}
			if n.Jump != nil {
				t.Fatalf("unexpected jump: %+v", n.Jump)
			}
			tls, _ := n.Outbound["tls"].(map[string]interface{})
			gotFrag, _ := tls["fragment"].(bool)
			if gotFrag != tc.wantFragment {
				t.Fatalf("tls.fragment: got %v want %v", gotFrag, tc.wantFragment)
			}
			if len(n.Warnings) != 0 {
				t.Fatalf("unexpected warnings: %v", n.Warnings)
			}
		})
	}
}
