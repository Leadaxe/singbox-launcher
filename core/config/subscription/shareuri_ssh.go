package subscription

import (
	"fmt"
	"net/url"
	"strings"
)

// --- SSH ---

// sshPrivateKeyLiteral приводит листаемое поле private_key к одной строке для
// ?private_key=. ok=false означает «в URI не кодируется»: несколько ключей в
// одном параметре не разберутся обратно.
func sshPrivateKeyLiteral(v interface{}) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", true
	case string:
		return t, true
	case []string:
		return sshPrivateKeySingle(t)
	case []interface{}:
		parts := make([]string, 0, len(t))
		for _, e := range t {
			s, ok := e.(string)
			if !ok {
				return "", false
			}
			parts = append(parts, s)
		}
		return sshPrivateKeySingle(parts)
	default:
		return "", false
	}
}

func sshPrivateKeySingle(list []string) (string, bool) {
	kept := make([]string, 0, len(list))
	for _, s := range list {
		if strings.TrimSpace(s) != "" {
			kept = append(kept, s)
		}
	}
	switch len(kept) {
	case 0:
		return "", true
	case 1:
		return kept[0], true
	default:
		return "", false
	}
}

func shareURIFromSSH(out map[string]interface{}) (string, error) {
	user := mapGetString(out, "user")
	if user == "" {
		user = "root"
	}
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if server == "" {
		return "", fmt.Errorf("%w: ssh needs server", ErrShareURINotSupported)
	}
	if port <= 0 {
		port = 22
	}
	pass := mapGetString(out, "password")
	q := url.Values{}
	// Inline приватный ключ уезжает в ?private_key= — так его пишет LxBox
	// (node_spec_emit.dart) и так его читает наш же buildSSHOutbound, то
	// есть ссылка round-trip'ится. Раньше здесь стоял отказ
	// ErrShareURINotSupported, и ssh-узел с ключом нельзя было
	// скопировать вовсе; решение владельца 18.09.2026 — отдавать, но
	// только после предупреждения в UI (см. ShareURICarriesPrivateKey).
	//
	// private_key листаемый (строка либо массив): многоэлементную форму не
	// кодируем — склейка в один параметр не round-trip'ится, парсер вернул
	// бы один ключ вместо списка.
	pk, pkOK := sshPrivateKeyLiteral(out["private_key"])
	if !pkOK {
		return "", fmt.Errorf("%w: ssh private_key list with several keys cannot be encoded as URI", ErrShareURINotSupported)
	}
	if pk != "" {
		q.Set("private_key", pk)
	} else if pkp := mapGetString(out, "private_key_path"); pkp != "" {
		// Парсер отдаёт private_key_path только когда private_key пуст
		// (buildSSHOutbound): эмитим той же парой, иначе ссылка описывала
		// бы узел, которого разбор не даст.
		q.Set("private_key_path", pkp)
	}
	if hk, ok := out["host_key"].([]interface{}); ok && len(hk) > 0 {
		parts := make([]string, 0, len(hk))
		for _, x := range hk {
			parts = append(parts, mapGetString(map[string]interface{}{"v": x}, "v"))
		}
		q.Set("host_key", strings.Join(parts, ","))
	} else if hk, ok := out["host_key"].([]string); ok && len(hk) > 0 {
		q.Set("host_key", strings.Join(hk, ","))
	}
	if algs, ok := out["host_key_algorithms"].([]interface{}); ok && len(algs) > 0 {
		parts := make([]string, 0, len(algs))
		for _, x := range algs {
			parts = append(parts, mapGetString(map[string]interface{}{"v": x}, "v"))
		}
		q.Set("host_key_algorithms", strings.Join(parts, ","))
	} else if algs, ok := out["host_key_algorithms"].([]string); ok && len(algs) > 0 {
		q.Set("host_key_algorithms", strings.Join(algs, ","))
	}
	if cv := mapGetString(out, "client_version"); cv != "" {
		q.Set("client_version", cv)
	}
	if pp := mapGetString(out, "private_key_passphrase"); pp != "" {
		q.Set("private_key_passphrase", pp)
	}
	shareAppendDetourLiteral(q, out)
	var ui *url.Userinfo
	if pass != "" {
		ui = url.UserPassword(user, pass)
	} else {
		ui = url.User(url.PathEscape(user))
	}
	u := &url.URL{
		Scheme:   "ssh",
		User:     ui,
		Host:     hostPort(server, port),
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}
