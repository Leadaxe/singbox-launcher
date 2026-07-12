package warp

import (
	"context"
	"encoding/base64"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// fakeDoer returns a canned response and records the last request.
type fakeDoer struct {
	status   int
	body     string
	lastReq  *http.Request
	lastBody string
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	f.lastReq = req
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		f.lastBody = string(b)
	}
	return &http.Response{
		StatusCode: f.status,
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Header:     make(http.Header),
	}, nil
}

const regResponse = `{
  "id": "dev-123",
  "token": "tok-abc",
  "account": {"id": "acc-1", "account_type": "free"},
  "config": {
    "client_id": "Haxc",
    "interface": {"addresses": {"v4": "172.16.0.2", "v6": "2606:4700:110:8db9:ab57:dc55:6975:e1d"}},
    "peers": [{"public_key": "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=", "endpoint": {"host": "engage.cloudflareclient.com:2408"}}]
  }
}`

func TestGenerateKeypair(t *testing.T) {
	priv, pub, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	for name, k := range map[string]string{"priv": priv, "pub": pub} {
		raw, err := base64.StdEncoding.DecodeString(k)
		if err != nil {
			t.Fatalf("%s not std base64: %v", name, err)
		}
		if len(raw) != 32 {
			t.Fatalf("%s len = %d, want 32", name, len(raw))
		}
	}
	if priv == pub {
		t.Fatal("priv == pub")
	}
}

func TestRegisterParsesResponse(t *testing.T) {
	f := &fakeDoer{status: 200, body: regResponse}
	c := NewClient(f)
	acc, err := c.Register(context.Background(), RegisterOptions{Now: time.Unix(0, 0).UTC()})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if acc.PeerPublic != "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=" {
		t.Errorf("peer pub = %q", acc.PeerPublic)
	}
	if acc.ClientV4 != "172.16.0.2" {
		t.Errorf("v4 = %q", acc.ClientV4)
	}
	if acc.DeviceID != "dev-123" || acc.Token != "tok-abc" {
		t.Errorf("device/token = %q/%q", acc.DeviceID, acc.Token)
	}
	if acc.Endpoint != DefaultEndpoint {
		t.Errorf("endpoint = %q, want default", acc.Endpoint)
	}
	// request shape
	if f.lastReq.Header.Get("CF-Client-Version") != clientVersionHdr {
		t.Errorf("missing CF-Client-Version header")
	}
	if !strings.Contains(f.lastBody, `"key":`) || strings.Contains(f.lastBody, acc.PrivateKey) {
		t.Errorf("body must send public key, never the private key: %s", f.lastBody)
	}
}

func TestRegisterHTTPError(t *testing.T) {
	f := &fakeDoer{status: 400, body: `{}`}
	c := NewClient(f)
	if _, err := c.Register(context.Background(), RegisterOptions{}); err == nil {
		t.Fatal("expected error on HTTP 400")
	}
}

func TestReservedFromClientID(t *testing.T) {
	// "Haxc" base64 → bytes 0x1d,0xac,0x5c → 29,172,92
	acc := &Account{ClientID: "Haxc"}
	r := acc.Reserved()
	if len(r) != 3 || r[0] != 29 || r[1] != 172 || r[2] != 92 {
		t.Fatalf("reserved = %v, want [29 172 92]", r)
	}
	if got := (&Account{ClientID: ""}).Reserved(); got != nil {
		t.Errorf("empty client_id should give nil, got %v", got)
	}
	if got := (&Account{ClientID: "!!"}).Reserved(); got != nil {
		t.Errorf("bad base64 should give nil, got %v", got)
	}
}

func TestToWireguardURI(t *testing.T) {
	acc := &Account{
		PrivateKey: "8HwVtYZaYEecHZ6MqEzHDaOySDpOEDqDEGk0XjFGNHc=",
		PeerPublic: "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
		ClientV4:   "172.16.0.2",
		ClientV6:   "2606:4700:110:8db9:ab57:dc55:6975:e1d",
		ClientID:   "Haxc",
		Endpoint:   "162.159.192.10:2408",
	}
	uri, err := acc.ToWireguardURI(true)
	if err != nil {
		t.Fatalf("uri: %v", err)
	}
	if !strings.HasPrefix(uri, "wireguard://") {
		t.Fatalf("bad scheme: %s", uri)
	}
	u, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	q := u.Query()
	if q.Get("reserved") != "29,172,92" {
		t.Errorf("reserved = %q", q.Get("reserved"))
	}
	if q.Get("mtu") != "1280" {
		t.Errorf("mtu = %q, want 1280", q.Get("mtu"))
	}
	if q.Get("allowedips") != "0.0.0.0/0,::/0" {
		t.Errorf("allowedips = %q", q.Get("allowedips"))
	}
	// without reserved
	uri2, _ := acc.ToWireguardURI(false)
	if strings.Contains(uri2, "reserved=") {
		t.Errorf("reserved should be omitted: %s", uri2)
	}
}

func TestObfuscateAddsAWG(t *testing.T) {
	f := &fakeDoer{status: 200, body: regResponse}
	c := NewClient(f).WithRand(rand.New(rand.NewSource(1)))
	acc, err := c.Register(context.Background(), RegisterOptions{
		Obfuscate:      true,
		RandomEndpoint: true,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if len(acc.AWG) == 0 {
		t.Fatal("AWG fields missing")
	}
	// s1=s2=0 and h1..h4 preset present
	for _, k := range []string{"jc", "s1", "s2", "h1", "h4", "ip", "id"} {
		if _, ok := acc.AWG[k]; !ok {
			t.Errorf("AWG missing %q", k)
		}
	}
	if acc.Endpoint == DefaultEndpoint {
		t.Errorf("random endpoint not applied: %s", acc.Endpoint)
	}
	uri, err := acc.ToWireguardURI(true)
	if err != nil {
		t.Fatalf("uri: %v", err)
	}
	if !strings.Contains(uri, "jc=") || !strings.Contains(uri, "ip=quic") {
		t.Errorf("AWG params not in URI: %s", uri)
	}
}

func TestRandomEndpointDeterministic(t *testing.T) {
	e1 := RandomEndpoint(rand.New(rand.NewSource(42)))
	e2 := RandomEndpoint(rand.New(rand.NewSource(42)))
	if e1 != e2 {
		t.Errorf("seeded rng must be deterministic: %s != %s", e1, e2)
	}
	host, _, err := splitHostPort(e1)
	if err != nil {
		t.Fatalf("bad endpoint %q: %v", e1, err)
	}
	if !strings.HasPrefix(host, "162.159.") && !strings.HasPrefix(host, "188.114.") {
		t.Errorf("endpoint not from pool: %s", e1)
	}
}

func TestSplitHostPortIPv6(t *testing.T) {
	host, port, err := splitHostPort("[2606:4700:d0::a29f:c00a]:2408")
	if err != nil || host != "2606:4700:d0::a29f:c00a" || port != 2408 {
		t.Fatalf("ipv6 split = %q/%d err=%v", host, port, err)
	}
}
