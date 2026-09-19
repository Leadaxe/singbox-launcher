package subscription

import (
	"encoding/base64"
	"testing"
)

func TestVMessJSONFractionalPortRejected(t *testing.T) {
	vmessJSON := `{"v":"2","ps":"t","add":"vm.example.com","port":443.9,"id":"b831381d-6324-4d53-ad4f-8cda48b30811","aid":"0","net":"tcp","type":"none","host":"","path":"","tls":""}`
	uri := "vmess://" + base64.StdEncoding.EncodeToString([]byte(vmessJSON))
	_, err := ParseNode(uri, nil)
	if err == nil {
		t.Fatal("ParseNode: expected error for fractional port, got success")
	}
}

func TestVMessJSONWholeFloatPortAccepted(t *testing.T) {
	vmessJSON := `{"v":"2","ps":"t","add":"vm.example.com","port":443.0,"id":"b831381d-6324-4d53-ad4f-8cda48b30811","aid":"0","net":"tcp","type":"none","host":"","path":"","tls":""}`
	uri := "vmess://" + base64.StdEncoding.EncodeToString([]byte(vmessJSON))
	node, err := ParseNode(uri, nil)
	if err != nil {
		t.Fatalf("ParseNode: %v", err)
	}
	if node.Outbound["server_port"] != 443 {
		t.Fatalf("server_port=%v, want 443", node.Outbound["server_port"])
	}
}

func TestXrayJSONFractionalPortRejected(t *testing.T) {
	body := `[{"remarks":"frac port","outbounds":[{"tag":"proxy","protocol":"vmess","settings":{"vnext":[{"address":"v.example","port":443.9,"users":[{"id":"11111111-2222-3333-4444-555555555555","alterId":0}]}]},"streamSettings":{"network":"tcp","security":"none"}}]}]`
	nodes, err := ParseNodesFromXrayJSONArray(body, nil)
	if err != nil {
		t.Fatalf("ParseNodesFromXrayJSONArray: %v", err)
	}
	if len(nodes) != 0 {
		t.Fatalf("nodes=%d, want 0 (fractional port must not truncate to 443)", len(nodes))
	}
}
