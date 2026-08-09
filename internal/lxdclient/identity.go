//go:build darwin

// Package lxdclient — клиент управляющего канала демона `sing-box lxd`:
// mTLS-сопряжение по одноразовому приглашению, admin REST (apply / start /
// stop / status) и gRPC-дайл для протокола daemon.StartedService.
//
// Модель доверия зеркалит WireGuard-пиров (дизайн задачи 057 форка):
// приватные ключи не покидают свои хосты, обмениваются только отпечатки.
// Лаунчер пинит сервер по отпечатку из приглашения `адрес#отпечаток#код`,
// демон пинит клиента по отпечатку, зарегистрированному на enroll.
package lxdclient

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

const (
	clientCertFile = "client_cert.pem"
	clientKeyFile  = "client_key.pem"
	certValidity   = 10 * 365 * 24 * time.Hour
)

// Identity — клиентская ключевая пара лаунчера для mTLS.
type Identity struct {
	CertPEM []byte
	KeyPEM  []byte
	TLSCert tls.Certificate
	// Fingerprint — SHA-256 от DER сертификата, lowercase hex. Именно его
	// демон пинит при enroll.
	Fingerprint string
}

// FingerprintOf возвращает SHA-256 отпечаток DER-представления сертификата
// в lowercase hex — формат, единый с lxd/tlsca.go форка.
func FingerprintOf(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

// LoadOrCreateIdentity читает клиентскую пару из dir или создаёт новую при
// первом обращении (ECDSA P-256, self-signed, 10 лет — как у демона).
// Отпечаток стабилен между запусками: на нём держится доверие демона.
func LoadOrCreateIdentity(dir string) (*Identity, error) {
	certPath := filepath.Join(dir, clientCertFile)
	keyPath := filepath.Join(dir, clientKeyFile)
	certPEM, err := os.ReadFile(certPath)
	if err == nil {
		keyPEM, keyErr := os.ReadFile(keyPath)
		if keyErr != nil {
			return nil, fmt.Errorf("lxdclient: read client key: %w", keyErr)
		}
		return identityFromPEM(certPEM, keyPEM)
	}
	// 0700: каталог держит приватный ключ и (в соседнем файле) Bearer-секрет
	// службы — материал сопряжения не для чужих пользователей машины.
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("lxdclient: create identity dir: %w", err)
	}
	created, err := generateIdentity("singbox-launcher")
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(keyPath, created.KeyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("lxdclient: persist client key: %w", err)
	}
	if err = os.WriteFile(certPath, created.CertPEM, 0o600); err != nil {
		return nil, fmt.Errorf("lxdclient: persist client cert: %w", err)
	}
	return created, nil
}

// HasIdentity сообщает, существует ли сохранённая клиентская пара в dir.
func HasIdentity(dir string) bool {
	_, certErr := os.Stat(filepath.Join(dir, clientCertFile))
	_, keyErr := os.Stat(filepath.Join(dir, clientKeyFile))
	return certErr == nil && keyErr == nil
}

// RemoveIdentity удаляет клиентскую пару (полный сброс сопряжения).
func RemoveIdentity(dir string) error {
	var firstErr error
	for _, name := range []string{clientCertFile, clientKeyFile} {
		if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func generateIdentity(commonName string) (*Identity, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(certValidity),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &Identity{CertPEM: certPEM, KeyPEM: keyPEM, TLSCert: tlsCert, Fingerprint: FingerprintOf(der)}, nil
}

func identityFromPEM(certPEM, keyPEM []byte) (*Identity, error) {
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("lxdclient: parse client keypair: %w", err)
	}
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("lxdclient: invalid client certificate PEM")
	}
	return &Identity{CertPEM: certPEM, KeyPEM: keyPEM, TLSCert: tlsCert, Fingerprint: FingerprintOf(block.Bytes)}, nil
}
