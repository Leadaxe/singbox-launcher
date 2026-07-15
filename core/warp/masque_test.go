package warp

import (
	"crypto/x509"
	"encoding/base64"
	"testing"
)

func TestGenerateECDSAKeypair_DERValid(t *testing.T) {
	priv, pub, err := GenerateECDSAKeypair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	// private must decode as SEC1 EC private key (x509.ParseECPrivateKey) —
	// exactly what the sing-box masque outbound expects.
	pb, err := base64.StdEncoding.DecodeString(priv)
	if err != nil {
		t.Fatalf("priv not std base64: %v", err)
	}
	if _, err := x509.ParseECPrivateKey(pb); err != nil {
		t.Fatalf("priv not SEC1 DER: %v", err)
	}
	// public must decode as PKIX (x509.ParsePKIXPublicKey).
	kb, err := base64.StdEncoding.DecodeString(pub)
	if err != nil {
		t.Fatalf("pub not std base64: %v", err)
	}
	if _, err := x509.ParsePKIXPublicKey(kb); err != nil {
		t.Fatalf("pub not PKIX DER: %v", err)
	}
	if priv == pub {
		t.Fatal("priv == pub")
	}
}

func TestToMasqueOutbound(t *testing.T) {
	acc := &MasqueAccount{
		PrivateKeyDER: "cHJpdg==",
		ServerPubDER:  "cHVi",
		ClientV4:      "172.16.0.2",
		ClientV6:      "2606:4700:110:8db9::",
		Server:        "162.159.198.1",
		Network:       "h3",
		SNI:           "consumer-masque.cloudflareclient.com",
		IdleTimeout:   "5m",
		KeepAlive:     "30s",
	}
	ob, err := acc.ToMasqueOutbound()
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	assert := func(k string, want interface{}) {
		if ob[k] != want {
			t.Errorf("%s = %v, want %v", k, ob[k], want)
		}
	}
	assert("type", "masque")
	assert("profile", "cloudflare")
	assert("network", "h3")
	assert("server_port", 443)
	assert("ip", "172.16.0.2/32") // bare address gets /32
	assert("ipv6", "2606:4700:110:8db9::/128")
	assert("mtu", warpMTU)
	assert("sni", "consumer-masque.cloudflareclient.com")
	assert("idle_timeout", "5m")
	assert("keep_alive_period", "30s")
}

func TestToMasqueOutbound_RequiresKeys(t *testing.T) {
	if _, err := (&MasqueAccount{ClientV4: "172.16.0.2"}).ToMasqueOutbound(); err == nil {
		t.Fatal("expected error without key material")
	}
	if _, err := (&MasqueAccount{PrivateKeyDER: "x", ServerPubDER: "y"}).ToMasqueOutbound(); err == nil {
		t.Fatal("expected error without interface address")
	}
}

func TestEnsureCIDR(t *testing.T) {
	if ensureCIDR("1.2.3.4", false) != "1.2.3.4/32" {
		t.Error("v4 mask")
	}
	if ensureCIDR("fe80::1", true) != "fe80::1/128" {
		t.Error("v6 mask")
	}
	if ensureCIDR("1.2.3.4/24", false) != "1.2.3.4/24" {
		t.Error("existing mask must be preserved")
	}
}
