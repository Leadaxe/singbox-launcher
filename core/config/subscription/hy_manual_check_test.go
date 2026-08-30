package subscription

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestManualHysteriaCheck(t *testing.T) {
	body := `[{"outbounds":[{"protocol":"hysteria","settings":{"address":"45.195.228.220","port":8449,"version":2},"streamSettings":{"finalmask":{"quicParams":{"congestion":"bbr","debug":false}},"hysteriaSettings":{"auth":"a581e923-3e69-4338-86d2-4a9d9e1339fb","version":2},"network":"hysteria","security":"tls","tlsSettings":{"alpn":["h3"],"fingerprint":"firefox","serverName":"snihelsinki02.jasontech.online"}},"tag":"proxy"}]}]`
	res, err := ParseSubscriptionBody([]byte(body), nil, 100)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	fmt.Println("entries:", len(res.Entries), "rejected:", len(res.Rejected))
	for _, e := range res.Entries {
		b, _ := json.Marshal(e.Node.Outbound)
		fmt.Println("  ", e.Node.Scheme, e.Node.Tag, string(b))
	}
	for _, r := range res.Rejected {
		fmt.Println("  REJ:", r.Reason)
	}

	// v1 вариант того же диалекта
	body1 := `[{"outbounds":[{"protocol":"hysteria","settings":{"address":"1.2.3.4","port":36712},"streamSettings":{"hysteriaSettings":{"auth":"pw","up_mbps":100,"down_mbps":200,"obfs":"s3cr3t"},"network":"hysteria","security":"tls","tlsSettings":{"alpn":["h3"],"serverName":"a.example.com"}},"tag":"hy1"}]}]`
	res1, err := ParseSubscriptionBody([]byte(body1), nil, 100)
	if err != nil {
		t.Fatalf("parse1: %v", err)
	}
	for _, e := range res1.Entries {
		b, _ := json.Marshal(e.Node.Outbound)
		fmt.Println("  v1:", e.Node.Scheme, e.Node.Tag, string(b))
	}
	for _, r := range res1.Rejected {
		fmt.Println("  REJ1:", r.Reason)
	}
}

func TestManualHysteriaURI(t *testing.T) {
	for _, u := range []string{
		"hysteria://1.2.3.4:36712?auth=mypass&peer=example.com&upmbps=100&downmbps=200&obfs=xplus&obfsParam=s3cr3t&alpn=h3&insecure=1#HY1",
		"hy://1.2.3.4:20000-30000?auth=x&sni=a.b",
		"hysteria://pw@1.2.3.4:443?upmbps=50",
	} {
		n, err := ParseNode(u, nil)
		if err != nil {
			t.Fatalf("%s: %v", u, err)
		}
		b, _ := json.Marshal(n.Outbound)
		fmt.Println(n.Scheme, n.Tag, string(b))
		var m map[string]interface{}
		_ = json.Unmarshal(b, &m)
		m["type"] = "hysteria"
		back, err := ShareURIFromOutbound(m)
		fmt.Println("   ->", back, err)
	}
}

func TestManualSingboxHysteriaImport(t *testing.T) {
	body := `{"outbounds":[{"type":"hysteria","tag":"n1","server":"1.2.3.4","server_port":443,"auth_str":"pw","up_mbps":50,"down_mbps":100,"obfs":{"type":"salamander","password":"zzz"},"tls":{"enabled":true,"server_name":"a.b","alpn":["h3"],"utls":{"enabled":true,"fingerprint":"chrome"}}}]}`
	res, err := ParseSubscriptionBody([]byte(body), nil, 100)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, e := range res.Entries {
		b, _ := json.Marshal(e.Node.Outbound)
		fmt.Println("  singbox:", e.Node.Scheme, e.Node.Tag, string(b))
	}
	for _, r := range res.Rejected {
		fmt.Println("  REJ:", r.Reason)
	}
}
