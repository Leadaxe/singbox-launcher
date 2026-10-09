package services

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// SSH-цель удалённой машины (SPEC 161): куда окно Service ведёт команды
// обслуживания демона (`ssh <цель> '<команда>'`). Живёт здесь, а не в core:
// её валидирует RemoteRegistry.SetSSH, а core/services не может
// импортировать core. Обёртка команды — core.WrapSSH.

// defaultSSHUser — пользователь цели по умолчанию: на OpenWrt и в типичном
// VPS-образе управляют от root.
const defaultSSHUser = "root"

// SSHTarget — пользователь, хост и порт ssh. Port 0 — порт ssh по умолчанию;
// User "" — пользователь ssh по умолчанию (локальный или из ~/.ssh/config).
type SSHTarget struct {
	User string
	Host string
	Port int
}

// ParseSSHTarget разбирает «user@host», «user@host:port», «host»,
// «[v6]:port», «user@[v6]:port» и голый IPv6 без порта. Символы
// ограничены: цель уходит в командную строку Terminal, и хост с «-» в начале
// ssh принял бы за ключ.
func ParseSSHTarget(s string) (SSHTarget, error) {
	var t SSHTarget
	s = strings.TrimSpace(s)
	if s == "" {
		return t, fmt.Errorf("ssh target is empty")
	}
	hostPart := s
	if i := strings.LastIndex(s, "@"); i >= 0 {
		t.User = s[:i]
		hostPart = s[i+1:]
		if t.User == "" {
			return t, fmt.Errorf("ssh target %q: empty user before @", s)
		}
		if !validSSHUser(t.User) {
			return t, fmt.Errorf("ssh target %q: invalid user %q", s, t.User)
		}
	}
	switch {
	case strings.HasPrefix(hostPart, "["):
		end := strings.Index(hostPart, "]")
		if end < 0 {
			return t, fmt.Errorf("ssh target %q: missing ] after IPv6 address", s)
		}
		t.Host = hostPart[1:end]
		rest := hostPart[end+1:]
		if rest != "" {
			if !strings.HasPrefix(rest, ":") {
				return t, fmt.Errorf("ssh target %q: unexpected %q after ]", s, rest)
			}
			port, err := parseSSHPort(rest[1:])
			if err != nil {
				return t, fmt.Errorf("ssh target %q: %w", s, err)
			}
			t.Port = port
		}
	case strings.Count(hostPart, ":") == 1:
		host, portText, _ := strings.Cut(hostPart, ":")
		port, err := parseSSHPort(portText)
		if err != nil {
			return t, fmt.Errorf("ssh target %q: %w", s, err)
		}
		t.Host, t.Port = host, port
	default:
		// Без двоеточия — имя/IPv4; два и больше — голый IPv6 без порта.
		t.Host = hostPart
	}
	if t.Host == "" {
		return t, fmt.Errorf("ssh target %q: empty host", s)
	}
	if !validSSHHost(t.Host) {
		return t, fmt.Errorf("ssh target %q: invalid host %q", s, t.Host)
	}
	return t, nil
}

// DefaultSSHTarget — цель по умолчанию для машины с адресом демона
// daemonAddr: root@<хост этого адреса>.
func DefaultSSHTarget(daemonAddr string) SSHTarget {
	addr := strings.TrimSpace(daemonAddr)
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = strings.Trim(addr, "[]")
	}
	return SSHTarget{User: defaultSSHUser, Host: host}
}

// String — цель так, как её ввёл бы человек: root@192.168.10.1,
// admin@vps:2222, root@[fe80::1]:2222. ParseSSHTarget(t.String()) == t.
func (t SSHTarget) String() string {
	host := t.Host
	if t.Port != 0 && strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if t.Port != 0 {
		host += ":" + strconv.Itoa(t.Port)
	}
	if t.User != "" {
		return t.User + "@" + host
	}
	return host
}

// IsRoot — команды на машине выполняются от root (пользователь root или
// не указан — тогда решает ssh, и лаунчер sudo не добавляет).
func (t SSHTarget) IsRoot() bool {
	return t.User == "" || t.User == defaultSSHUser
}

func parseSSHPort(s string) (int, error) {
	port, err := strconv.Atoi(s)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	return port, nil
}

// validSSHUser — имя пользователя POSIX: буквы, цифры, «._-», не с «-».
func validSSHUser(u string) bool {
	if strings.HasPrefix(u, "-") {
		return false
	}
	for _, r := range u {
		if !isSSHNameRune(r) {
			return false
		}
	}
	return true
}

// validSSHHost — DNS-имя, IPv4 или IPv6 (с зоной «%en0»), не с «-».
func validSSHHost(h string) bool {
	if strings.HasPrefix(h, "-") {
		return false
	}
	for _, r := range h {
		if !isSSHNameRune(r) && r != ':' && r != '%' {
			return false
		}
	}
	return true
}

func isSSHNameRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	}
	return r == '.' || r == '-' || r == '_'
}
