package subscription

import (
	"encoding/json"
	"testing"
)

// TestXrayHysteriaDialectVersionSplit — диалект Xray отдаёт ОБА протокола под
// одним protocol:"hysteria", и версия решает, каким типом узел станет.
// Перепутать их нельзя: у v2 секрет в password, у v1 — в auth_str, и узел с
// чужим полем ядро отвергает вместе со всем конфигом.
func TestXrayHysteriaDialectVersionSplit(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantScheme  string
		wantSecret  string
		secretField string
	}{
		{
			name:        "version 2 → hysteria2",
			body:        `[{"outbounds":[{"protocol":"hysteria","settings":{"address":"45.195.228.220","port":8449,"version":2},"streamSettings":{"hysteriaSettings":{"auth":"a581e923","version":2},"network":"hysteria","security":"tls","tlsSettings":{"alpn":["h3"],"fingerprint":"firefox","serverName":"sni.example.com"}},"tag":"proxy"}]}]`,
			wantScheme:  "hysteria2",
			wantSecret:  "a581e923",
			secretField: "password",
		},
		{
			name:        "без version → hysteria v1",
			body:        `[{"outbounds":[{"protocol":"hysteria","settings":{"address":"1.2.3.4","port":36712},"streamSettings":{"hysteriaSettings":{"auth":"pw","up_mbps":100,"down_mbps":200,"obfs":"s3cr3t"},"network":"hysteria","security":"tls","tlsSettings":{"alpn":["h3"],"serverName":"a.example.com"}},"tag":"hy1"}]}]`,
			wantScheme:  "hysteria",
			wantSecret:  "pw",
			secretField: "auth_str",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := ParseSubscriptionBody([]byte(tc.body), nil, 100)
			if err != nil {
				t.Fatalf("разбор тела: %v", err)
			}
			if len(res.Entries) != 1 {
				t.Fatalf("узлов %d, ожидался 1 (rejected=%d)", len(res.Entries), len(res.Rejected))
			}
			node := res.Entries[0].Node
			if node.Scheme != tc.wantScheme {
				t.Fatalf("схема %q, ожидалась %q", node.Scheme, tc.wantScheme)
			}
			if got, _ := node.Outbound[tc.secretField].(string); got != tc.wantSecret {
				t.Fatalf("%s = %q, ожидалось %q", tc.secretField, got, tc.wantSecret)
			}
			// uTLS на QUIC ядро не примет — fingerprint обязан быть снят.
			if tls, ok := node.Outbound["tls"].(map[string]interface{}); ok {
				if _, has := tls["utls"]; has {
					t.Fatalf("utls остался в TLS-блоке QUIC-протокола: %v", tls)
				}
			}
		})
	}
}

// TestHysteriaV1BandwidthAlwaysEmitted — ядро отказывается инициализировать
// outbound v1 без up_mbps/down_mbps («missing upload speed»), и это fatal для
// ВСЕГО config.json. Полоса обязана появиться на каждом пути разбора.
func TestHysteriaV1BandwidthAlwaysEmitted(t *testing.T) {
	t.Run("uri", func(t *testing.T) {
		node, err := ParseNode("hysteria://host.example.com:36712?auth=a", nil)
		if err != nil {
			t.Fatalf("разбор URI: %v", err)
		}
		assertHysteriaBandwidth(t, node.Outbound)
	})

	t.Run("singbox", func(t *testing.T) {
		body := `{"outbounds":[{"type":"hysteria","tag":"n","server":"1.2.3.4","server_port":443,"auth_str":"pw","tls":{"enabled":true,"server_name":"a.b"}}]}`
		res, err := ParseSubscriptionBody([]byte(body), nil, 100)
		if err != nil {
			t.Fatalf("разбор тела: %v", err)
		}
		if len(res.Entries) != 1 {
			t.Fatalf("узлов %d, ожидался 1", len(res.Entries))
		}
		assertHysteriaBandwidth(t, res.Entries[0].Node.Outbound)
	})

	t.Run("xray", func(t *testing.T) {
		body := `[{"outbounds":[{"protocol":"hysteria","settings":{"address":"1.2.3.4","port":36712},"streamSettings":{"hysteriaSettings":{"auth":"pw"},"network":"hysteria","security":"tls","tlsSettings":{"serverName":"a.b"}},"tag":"t"}]}]`
		res, err := ParseSubscriptionBody([]byte(body), nil, 100)
		if err != nil {
			t.Fatalf("разбор тела: %v", err)
		}
		if len(res.Entries) != 1 {
			t.Fatalf("узлов %d, ожидался 1", len(res.Entries))
		}
		assertHysteriaBandwidth(t, res.Entries[0].Node.Outbound)
	})
}

func assertHysteriaBandwidth(t *testing.T, ob map[string]interface{}) {
	t.Helper()
	for _, key := range []string{"up_mbps", "down_mbps"} {
		switch v := ob[key].(type) {
		case int:
			if v <= 0 {
				t.Fatalf("%s = %d, ядро отвергнет конфиг", key, v)
			}
		case float64:
			if v <= 0 {
				t.Fatalf("%s = %v, ядро отвергнет конфиг", key, v)
			}
		default:
			t.Fatalf("%s отсутствует (%T) — ядро отвергнет ВЕСЬ конфиг", key, ob[key])
		}
	}
}

// TestSingboxHysteriaObfsObjectFlattened — провайдеры кладут в v1 obfs-объект
// от v2; ядро ждёт строку и на объекте роняет разбор всего конфига.
func TestSingboxHysteriaObfsObjectFlattened(t *testing.T) {
	body := `{"outbounds":[{"type":"hysteria","tag":"n","server":"1.2.3.4","server_port":443,"auth_str":"pw","up_mbps":50,"down_mbps":50,"obfs":{"type":"salamander","password":"zzz"},"tls":{"enabled":true,"server_name":"a.b"}}]}`
	res, err := ParseSubscriptionBody([]byte(body), nil, 100)
	if err != nil {
		t.Fatalf("разбор тела: %v", err)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("узлов %d, ожидался 1", len(res.Entries))
	}
	obfs, ok := res.Entries[0].Node.Outbound["obfs"].(string)
	if !ok || obfs != "zzz" {
		t.Fatalf("obfs = %#v, ожидалась плоская строка \"zzz\"", res.Entries[0].Node.Outbound["obfs"])
	}
}

// TestHysteriaShareURIRoundTrip — ссылка, выданная наружу, обязана вернуться
// тем же узлом: секрет в auth=, обфускация парой obfs=xplus&obfsParam=.
func TestHysteriaShareURIRoundTrip(t *testing.T) {
	const uri = "hysteria://host.example.com:36712?auth=pass123&obfs=xplus&obfsParam=s3cr3t&upmbps=100&downmbps=200&sni=host.example.com&alpn=h3#HY1"

	node, err := ParseNode(uri, nil)
	if err != nil {
		t.Fatalf("разбор URI: %v", err)
	}
	raw, err := json.Marshal(node.Outbound)
	if err != nil {
		t.Fatalf("маршал outbound: %v", err)
	}
	var ob map[string]interface{}
	if err := json.Unmarshal(raw, &ob); err != nil {
		t.Fatalf("демаршал outbound: %v", err)
	}
	ob["type"] = "hysteria"

	share, err := ShareURIFromOutbound(ob)
	if err != nil {
		t.Fatalf("эмиссия share-URI: %v", err)
	}
	back, err := ParseNode(share, nil)
	if err != nil {
		t.Fatalf("повторный разбор %q: %v", share, err)
	}

	if back.Server != node.Server || back.Port != node.Port {
		t.Fatalf("endpoint разъехался: %s:%d → %s:%d", node.Server, node.Port, back.Server, back.Port)
	}
	for _, key := range []string{"auth_str", "obfs", "up_mbps", "down_mbps"} {
		if !jsonEqual(node.Outbound[key], back.Outbound[key]) {
			t.Fatalf("%s разъехался: %#v → %#v", key, node.Outbound[key], back.Outbound[key])
		}
	}
}

func jsonEqual(a, b interface{}) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	return errA == nil && errB == nil && string(ab) == string(bb)
}
