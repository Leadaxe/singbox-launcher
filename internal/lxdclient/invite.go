//go:build darwin

package lxdclient

import (
	"fmt"
	"net"
	"strings"
)

// Invite — распарсенное приглашение сопряжения, которое демон печатает в
// консоль/лог при старте с --tls (пока нет зарегистрированных клиентов) или
// выдаёт по `sing-box lxd client add`.
//
// Формат: `адрес#отпечаток-сервера#одноразовый-код`, например
// `127.0.0.1:9091#3f2a…9c#K7QM-XXNP-2RTD`.
type Invite struct {
	// Addr — host:port управляющего канала.
	Addr string
	// ServerFingerprint — SHA-256 отпечаток серверного сертификата
	// (lowercase hex), по нему лаунчер пинит демона.
	ServerFingerprint string
	// Code — одноразовый код регистрации (сгорает после первого enroll).
	Code string
}

// ParseInvite разбирает строку приглашения. Терпим к пробелам по краям и
// регистру отпечатка.
func ParseInvite(raw string) (Invite, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return Invite{}, fmt.Errorf("invite is empty")
	}
	parts := strings.Split(s, "#")
	if len(parts) != 3 {
		return Invite{}, fmt.Errorf("expected format address#fingerprint#code (3 parts, got %d)", len(parts))
	}
	addr := strings.TrimSpace(parts[0])
	fingerprint := strings.ToLower(strings.TrimSpace(parts[1]))
	code := strings.TrimSpace(parts[2])
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return Invite{}, fmt.Errorf("address %q is not a valid host:port: %w", addr, err)
	}
	if len(fingerprint) != 64 || !isHex(fingerprint) {
		return Invite{}, fmt.Errorf("fingerprint must be 64 hex chars (sha-256)")
	}
	if code == "" {
		return Invite{}, fmt.Errorf("one-time code is empty")
	}
	return Invite{Addr: addr, ServerFingerprint: fingerprint, Code: code}, nil
}

// IsLoopbackAddr сообщает, указывает ли host:port на loopback-интерфейс.
// Используется как гейт для bootstrap без пина (AllowUnpinnedTLS): отправлять
// Bearer-секрет по неаутентифицированному TLS можно только на loopback, где
// MITM невозможен без локального root.
func IsLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isHex(s string) bool {
	for _, r := range s {
		ok := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		if !ok {
			return false
		}
	}
	return true
}
