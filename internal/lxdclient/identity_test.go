package lxdclient

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestLoadOrCreateIdentityPersists(t *testing.T) {
	dir := t.TempDir()

	first, err := LoadOrCreateIdentity(dir)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(first.Fingerprint) != 64 {
		t.Fatalf("fingerprint length = %d, want 64", len(first.Fingerprint))
	}
	if !HasIdentity(dir) {
		t.Fatal("HasIdentity = false after create")
	}

	// Повторная загрузка — тот же отпечаток (доверие демона держится на нём).
	second, err := LoadOrCreateIdentity(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if second.Fingerprint != first.Fingerprint {
		t.Fatalf("fingerprint changed across reload: %s → %s", first.Fingerprint, second.Fingerprint)
	}

	// Сертификат — валидный DER с ExtKeyUsageClientAuth.
	block, _ := pem.Decode(first.CertPEM)
	if block == nil {
		t.Fatal("cert PEM did not decode")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	if FingerprintOf(block.Bytes) != first.Fingerprint {
		t.Fatal("FingerprintOf(DER) != Identity.Fingerprint")
	}
	hasClientAuth := false
	for _, u := range cert.ExtKeyUsage {
		if u == x509.ExtKeyUsageClientAuth {
			hasClientAuth = true
		}
	}
	if !hasClientAuth {
		t.Error("client cert missing ExtKeyUsageClientAuth")
	}

	// RemoveIdentity стирает пару.
	if err := RemoveIdentity(dir); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if HasIdentity(dir) {
		t.Fatal("HasIdentity = true after remove")
	}
}

func TestFingerprintOfStable(t *testing.T) {
	der := []byte("some-der-bytes")
	if FingerprintOf(der) != FingerprintOf(der) {
		t.Fatal("FingerprintOf not deterministic")
	}
	if len(FingerprintOf(der)) != 64 {
		t.Fatal("fingerprint not 64 hex chars")
	}
}
