package subscription

import (
	"fmt"
	"net/url"
)

// --- VLESS ---

func vlessTLSToQuery(q url.Values, tls map[string]interface{}, server string, port int) {
	// Блока tls нет — узел работает БЕЗ шифрования, и ссылка обязана сказать
	// это явно. Раньше здесь угадывалось security=tls по номеру порта
	// (эвристика plaintextVLESSPorts), и узел с security=none на 443 после
	// round-trip приезжал уже с TLS: пользователь делился ссылкой на узел,
	// который не подключается. Эвристика уместна на ВХОДЕ, где выбора нет
	// (провайдер не написал security), но не на выходе, где мы точно знаем,
	// что TLS выключен (SPEC 103, фаза 2).
	if tls == nil {
		q.Set("security", "none")
		return
	}
	en, hasEn := tls["enabled"].(bool)
	if hasEn && !en {
		q.Set("security", "none")
		return
	}
	if reality, ok := tls["reality"].(map[string]interface{}); ok {
		pbk := mapGetString(reality, "public_key")
		if pbk != "" {
			q.Set("pbk", pbk)
			if sid := mapGetString(reality, "short_id"); sid != "" {
				q.Set("sid", sid)
			}
			sni := mapGetString(tls, "server_name")
			if sni == "" {
				sni = server
			}
			if sni != "" {
				q.Set("sni", sni)
			}
			if utls, ok := tls["utls"].(map[string]interface{}); ok {
				if fp := mapGetString(utls, "fingerprint"); fp != "" && fp != "random" {
					q.Set("fp", fp)
				}
			}
			shareAppendALPNInsecure(q, tls)
			return
		}
	}
	// Plain TLS
	q.Set("security", "tls")
	if sni := mapGetString(tls, "server_name"); sni != "" {
		q.Set("sni", sni)
	} else if server != "" {
		q.Set("sni", server)
	}
	if utls, ok := tls["utls"].(map[string]interface{}); ok {
		if fp := mapGetString(utls, "fingerprint"); fp != "" && fp != "random" {
			q.Set("fp", fp)
		}
	}
	shareAppendALPNInsecure(q, tls)
}

func shareURIFromVLESS(out map[string]interface{}) (string, error) {
	uuid := mapGetString(out, "uuid")
	server := mapGetString(out, "server")
	port := mapGetInt(out, "server_port")
	if uuid == "" || server == "" || port <= 0 {
		return "", fmt.Errorf("%w: vless needs uuid, server, server_port", ErrShareURINotSupported)
	}
	q := url.Values{}
	// encryption: у VLESS почти всегда "none", но post-quantum узлы несут в
	// этом поле длинный ключ (mlkem768x25519plus…). Жёсткое "none" затирало
	// его, и поделившийся ссылкой отдавал узел без ключа обмена.
	if enc := mapGetString(out, "encryption"); enc != "" {
		q.Set("encryption", enc)
	} else {
		q.Set("encryption", "none")
	}
	if tr, ok := out["transport"].(map[string]interface{}); ok {
		transportToQuery(q, tr)
	}
	// Эвристика порта здесь не нужна: отсутствие блока tls означает, что TLS
	// выключен, и vlessTLSToQuery скажет об этом явным security=none.
	if tls, ok := out["tls"].(map[string]interface{}); ok {
		vlessTLSToQuery(q, tls, server, port)
	} else {
		vlessTLSToQuery(q, nil, server, port)
	}
	if f := mapGetString(out, "flow"); f != "" {
		q.Set("flow", f)
	}
	if pe := mapGetString(out, "packet_encoding"); pe != "" {
		q.Set("packetEncoding", pe)
	}
	shareAppendDetourLiteral(q, out)
	hp := hostPort(server, port)
	u := &url.URL{
		Scheme:   "vless",
		User:     url.User(url.PathEscape(uuid)),
		Host:     hp,
		RawQuery: q.Encode(),
		Fragment: fragmentFromTag(out),
	}
	return u.String(), nil
}
