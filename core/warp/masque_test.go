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
