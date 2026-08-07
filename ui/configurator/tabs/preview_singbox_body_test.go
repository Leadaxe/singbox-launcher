package tabs

import "testing"

// SPEC 094 — вкладка Preview обязана показывать те же ноды, которые реально
// импортируются. До SPEC 094 тело, начинающееся с '{', давало 0 нод и в
// превью, и в пайплайне; теперь пайплайн его импортирует, и превью не должно
// расходиться с ним.

func TestParsePreviewNodesFromSingboxBody(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantTags  []string
		wantCount int
	}{
		{
			name:      "single outbound",
			body:      `{"type":"vless","tag":"solo","server":"e.com","server_port":443,"uuid":"u1"}`,
			wantTags:  []string{"solo"},
			wantCount: 1,
		},
		{
			name: "outbound array",
			body: `[{"type":"vless","tag":"a","server":"e1.com","server_port":443,"uuid":"u1"},
			        {"type":"trojan","tag":"b","server":"e2.com","server_port":443,"password":"p"}]`,
			wantTags:  []string{"a", "b"},
			wantCount: 2,
		},
		{
			// Служебные типы узлами не становятся, а группа — становится:
			// SPEC 094 A5, она рядовой узел списка, идущий после обычных.
			name: "whole config: service types dropped, group is an ordinary node",
			body: `{"outbounds":[
			         {"type":"vless","tag":"a","server":"e1.com","server_port":443,"uuid":"u1"},
			         {"type":"direct","tag":"direct"},
			         {"type":"selector","tag":"sel","outbounds":["a"]}
			       ],"route":{"final":"sel"}}`,
			wantTags:  []string{"a", "sel"},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodes := parsePreviewNodesFromBody([]byte(tt.body), nil)
			if len(nodes) != tt.wantCount {
				t.Fatalf("got %d nodes, want %d", len(nodes), tt.wantCount)
			}
			for i, want := range tt.wantTags {
				if nodes[i].Tag != want {
					t.Fatalf("node[%d] tag = %q, want %q", i, nodes[i].Tag, want)
				}
			}
		})
	}
}

// URI-подписки и Xray-массив разбираются как раньше — диспатч sing-box не
// должен их перехватывать.
func TestParsePreviewNodesFromBodyKeepsOtherFormats(t *testing.T) {
	t.Run("uri list", func(t *testing.T) {
		body := "vless://b831381d-6324-4d53-ad4f-8cda48b30811@e.com:443?security=tls&sni=e.com#uri-node"
		nodes := parsePreviewNodesFromBody([]byte(body), nil)
		if len(nodes) != 1 {
			t.Fatalf("got %d nodes, want 1", len(nodes))
		}
		if nodes[0].Server != "e.com" {
			t.Fatalf("server = %q, want e.com", nodes[0].Server)
		}
	})

	t.Run("xray json array", func(t *testing.T) {
		body := `[{"remarks":"xray-node","outbounds":[{"protocol":"vless","tag":"proxy",
		           "settings":{"vnext":[{"address":"x.com","port":443,
		           "users":[{"id":"b831381d-6324-4d53-ad4f-8cda48b30811"}]}]}}]}]`
		nodes := parsePreviewNodesFromBody([]byte(body), nil)
		if len(nodes) != 1 {
			t.Fatalf("got %d nodes, want 1", len(nodes))
		}
		if nodes[0].Server != "x.com" {
			t.Fatalf("server = %q, want x.com", nodes[0].Server)
		}
	})

	t.Run("garbage body yields nothing, without panicking", func(t *testing.T) {
		if nodes := parsePreviewNodesFromBody([]byte("not a subscription"), nil); len(nodes) != 0 {
			t.Fatalf("got %d nodes, want 0", len(nodes))
		}
	})
}
