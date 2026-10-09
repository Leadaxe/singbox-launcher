package config

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"singbox-launcher/core/config/subscription"
)

// sing-box-lx issue #36 / SPEC 119: per Xray (GetNormalizedPath /
// GetNormalizedQuery) everything after the first "?" of an XHTTP path is the
// request query and is sent verbatim; edgetunnel-style Cloudflare Worker
// relays read proxyip from it. The launcher must carry transport.path through
// every input, the stored body and the generated config unchanged, and the
// share link must percent-encode the "?" inside the path= value. WebSocket
// keeps splitting "?ed=N" into max_early_data.
func TestXHTTPPathQueryVerbatim(t *testing.T) {
	paths := []string{
		"/?proxyip=192.0.2.10",
		"/base?x=1",
		"/proxyip=192.0.2.10",
	}
	const uuid = "c59eb5ed-6324-4d53-ad4f-8cda48b30811"

	for _, p := range paths {
		t.Run("uri "+p, func(t *testing.T) {
			uri := "vless://" + uuid + "@h.test:443?type=xhttp&mode=stream-one&security=tls&sni=h.test&host=h.test&path=" + url.QueryEscape(p) + "#x"
			node, err := subscription.ParseNode(uri, nil)
			if err != nil || node == nil {
				t.Fatalf("parse: %v", err)
			}
			if got := xhttpEmittedPath(t, node); got != p {
				t.Fatalf("generated transport.path = %q, want %q", got, p)
			}

			// State round-trip: the materialized body is what state.json keeps.
			body, _, drop := materializeParsedNodeBody(node)
			if drop != nil {
				t.Fatalf("materialize dropped the node: %s", drop.Code)
			}
			var stored map[string]interface{}
			if err := json.Unmarshal(body, &stored); err != nil {
				t.Fatalf("stored body: %v", err)
			}
			stored["type"] = "vless"
			reloaded := &ParsedNode{Tag: "x", Scheme: "vless", Server: "h.test", Port: 443, Outbound: stored}
			if got := xhttpEmittedPath(t, reloaded); got != p {
				t.Fatalf("after state round-trip transport.path = %q, want %q", got, p)
			}

			// Share link: "?" stays inside the path= value, the link parses back.
			share, err := subscription.ShareURIFromOutbound(node.Outbound)
			if err != nil {
				t.Fatalf("share: %v", err)
			}
			q := share[strings.Index(share, "?")+1:]
			if i := strings.Index(q, "#"); i >= 0 {
				q = q[:i]
			}
			if strings.Contains(q, "?") {
				t.Fatalf("raw '?' in the link query: %s", share)
			}
			back, err := subscription.ParseNode(share, nil)
			if err != nil || back == nil {
				t.Fatalf("re-parse %s: %v", share, err)
			}
			if got := back.Outbound["transport"].(map[string]interface{})["path"]; got != p {
				t.Fatalf("share round-trip path = %v, want %q (link %s)", got, p, share)
			}
		})

		t.Run("xray "+p, func(t *testing.T) {
			pj, _ := json.Marshal(p)
			body := `[{"remarks":"x","outbounds":[{"tag":"proxy","protocol":"vless","settings":{"vnext":[{"address":"h.test","port":443,"users":[{"id":"` + uuid + `"}]}]},"streamSettings":{"network":"xhttp","security":"tls","tlsSettings":{"serverName":"h.test"},"xhttpSettings":{"path":` + string(pj) + `,"host":"h.test","mode":"stream-one"}}}]}]`
			if got := xhttpBodyPath(t, body); got != p {
				t.Fatalf("Xray-JSON transport.path = %q, want %q", got, p)
			}
		})

		t.Run("singbox "+p, func(t *testing.T) {
			pj, _ := json.Marshal(p)
			body := `{"outbounds":[{"type":"vless","tag":"x","server":"h.test","server_port":443,"uuid":"` + uuid + `","tls":{"enabled":true,"server_name":"h.test"},"transport":{"type":"xhttp","mode":"stream-one","host":"h.test","path":` + string(pj) + `}}]}`
			if got := xhttpBodyPath(t, body); got != p {
				t.Fatalf("sing-box JSON transport.path = %q, want %q", got, p)
			}
		})
	}

	t.Run("ws ed tail still split", func(t *testing.T) {
		uri := "vless://" + uuid + "@h.test:443?type=ws&security=tls&sni=h.test&host=h.test&path=" + url.QueryEscape("/ws?ed=2560") + "#ws"
		node, err := subscription.ParseNode(uri, nil)
		if err != nil || node == nil {
			t.Fatalf("parse: %v", err)
		}
		tr := node.Outbound["transport"].(map[string]interface{})
		if tr["path"] != "/ws" {
			t.Fatalf("ws path = %v, want /ws", tr["path"])
		}
		if ed, _ := tr["max_early_data"].(int); ed != 2560 {
			t.Fatalf("ws max_early_data = %v, want 2560", tr["max_early_data"])
		}
	})
}

// xhttpEmittedPath — transport.path of the outbound GenerateNodeJSON writes.
func xhttpEmittedPath(t *testing.T, node *ParsedNode) string {
	t.Helper()
	js, err := GenerateNodeJSON(node)
	if err != nil {
		t.Fatalf("gen: %v", err)
	}
	obj := extractFirstJSONObject(js)
	var out struct {
		Transport struct {
			Type string `json:"type"`
			Path string `json:"path"`
		} `json:"transport"`
	}
	if err := json.Unmarshal([]byte(obj), &out); err != nil {
		t.Fatalf("generated outbound is not JSON: %v\n%s", err, js)
	}
	if out.Transport.Type != "xhttp" {
		t.Fatalf("transport.type = %q, want xhttp\n%s", out.Transport.Type, js)
	}
	return out.Transport.Path
}

// xhttpBodyPath — transport.path of the single node a subscription body yields.
func xhttpBodyPath(t *testing.T, body string) string {
	t.Helper()
	res, err := subscription.ParseSubscriptionBody([]byte(body), nil, 0)
	if err != nil {
		t.Fatalf("parse body: %v", err)
	}
	if len(res.Entries) != 1 || res.Entries[0].Node == nil {
		t.Fatalf("want one node, got %d entries (rejected %v)", len(res.Entries), res.Rejected)
	}
	tr, _ := res.Entries[0].Node.Outbound["transport"].(map[string]interface{})
	p, _ := tr["path"].(string)
	return p
}
