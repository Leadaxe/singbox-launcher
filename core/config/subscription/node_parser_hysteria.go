package subscription

import (
	"strconv"
	"strings"

	"singbox-launcher/core/config/configtypes"
	"singbox-launcher/internal/debuglog"
)

// buildHysteriaOutbound строит outbound для Hysteria v1 (sing-box type "hysteria").
//
// Протокол не путать с hysteria2: у v1 другая форма учётных данных
// (auth_str / auth вместо password), obfs — ПЛОСКАЯ строка, а не объект
// {type,password} (option/hysteria.go, HysteriaOutboundOptions.Obfs), и
// bandwidth обязателен на практике: сервер v1 согласует скорость по up/down.
//
// URI-схема у v1 своя де-факто (клиенты Hysteria 1.x):
//
//	hysteria://host:port?auth=...&peer=sni&upmbps=100&downmbps=100&obfs=xplus&obfsParam=...&alpn=h3&insecure=1#label
//
// Учётные данные тут в query (auth / auth_str), а НЕ в userinfo — этим v1
// отличается от hysteria2, где пароль стоит перед '@'. Userinfo всё же
// принимается как запасной вариант: часть панелей выписывает ссылки в форме
// hysteria://auth@host:port.
func buildHysteriaOutbound(node *configtypes.ParsedNode, outbound map[string]interface{}) {
	if auth := hysteriaAuthFromNode(node); auth != "" {
		outbound["auth_str"] = auth
	} else {
		debuglog.WarnLog("Parser: Hysteria link missing auth. URI might be invalid.")
	}

	// Multi-port / port hopping: тот же синтаксис списка и диапазонов, что у
	// hysteria2 (ядро принимает server_ports у обоих типов — см. option).
	mport := strings.TrimSpace(queryGetFold(node.Query, "mport"))
	if mport == "" {
		mport = strings.TrimSpace(queryGetFold(node.Query, "ports"))
	}
	if sp := hysteria2MportSpecToSingBoxServerPorts(mport); len(sp) > 0 {
		outbound["server_ports"] = sp
	}

	// obfs у v1 — строка-секрет («obfs string»), а не тип обфускации.
	// Клиенты пишут её то как obfs=, то как obfsParam= (в оригинальном
	// Hysteria 1.x obfs задавал тип xplus, а сам секрет ехал в obfsParam).
	// Ядру нужна ровно одна строка, поэтому берём секрет, если он есть,
	// и только иначе — значение obfs.
	obfsParam := strings.TrimSpace(queryGetFold(node.Query, "obfsParam"))
	if obfsParam == "" {
		obfsParam = strings.TrimSpace(queryGetFold(node.Query, "obfs-password"))
	}
	obfs := strings.TrimSpace(queryGetFold(node.Query, "obfs"))
	switch {
	case obfsParam != "":
		outbound["obfs"] = obfsParam
	case obfs != "" && !strings.EqualFold(obfs, "xplus"):
		// Голый obfs=xplus без секрета обфускацией не является — это только
		// объявление типа; ядру такая строка сказала бы шифровать по слову
		// «xplus», и узел молча перестал бы соединяться.
		outbound["obfs"] = obfs
	}

	if up := hysteriaMbps(node, "upmbps", "up_mbps", "up"); up > 0 {
		outbound["up_mbps"] = up
	}
	if down := hysteriaMbps(node, "downmbps", "down_mbps", "down"); down > 0 {
		outbound["down_mbps"] = down
	}

	buildHysteriaTLS(node, outbound)
}

// hysteriaAuthFromNode достаёт секрет v1 из query (auth/auth_str) или userinfo.
func hysteriaAuthFromNode(node *configtypes.ParsedNode) string {
	for _, key := range []string{"auth", "auth_str", "authStr", "password"} {
		if v := strings.TrimSpace(queryGetFold(node.Query, key)); v != "" {
			return v
		}
	}
	return strings.TrimSpace(node.UUID)
}

// hysteriaMbps читает пропускную способность по любому из принятых написаний.
//
// Значение может нести суффикс единицы («100 mbps», «50m») — панели пишут
// по-разному, а ядру нужно целое число Мбит/с.
func hysteriaMbps(node *configtypes.ParsedNode, keys ...string) int {
	for _, key := range keys {
		raw := strings.TrimSpace(queryGetFold(node.Query, key))
		if raw == "" {
			continue
		}
		digits := raw
		if i := strings.IndexFunc(raw, func(r rune) bool { return r < '0' || r > '9' }); i > 0 {
			digits = raw[:i]
		}
		if n, err := strconv.Atoi(strings.TrimSpace(digits)); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// buildHysteriaTLS собирает TLS-блок v1.
//
// Как и hysteria2, протокол живёт поверх QUIC — TLS всегда включён, а uTLS и
// REALITY ядро на QUIC не примет (см. quicOutboundTypes), поэтому fp здесь
// сознательно не читается.
func buildHysteriaTLS(node *configtypes.ParsedNode, outbound map[string]interface{}) {
	q := node.Query
	tlsData := map[string]interface{}{"enabled": true}

	// SNI у v1 исторически зовут peer= (клиенты Hysteria 1.x), sni= — общее.
	sni := queryGetFold(q, "sni")
	if sni == "" {
		sni = queryGetFold(q, "peer")
	}
	if sni != "" && sni != "🔒" && (strings.Contains(sni, ".") || strings.Contains(sni, ":")) {
		tlsData["server_name"] = sni
	} else if node.Server != "" {
		tlsData["server_name"] = node.Server
	}

	if tlsInsecureTrue(q) {
		tlsData["insecure"] = true
	} else if v := queryGetFold(q, "skip-cert-verify"); v == "true" || v == "1" {
		tlsData["insecure"] = true
	}

	if pin := strings.TrimSpace(queryGetFold(q, "pinSHA256")); pin != "" {
		tlsData["certificate_public_key_sha256"] = []string{pin}
	}

	if alpn := queryGetFold(q, "alpn"); alpn != "" {
		alpnList := strings.Split(alpn, ",")
		for i := range alpnList {
			alpnList[i] = strings.TrimSpace(alpnList[i])
		}
		tlsData["alpn"] = alpnList
	}

	outbound["tls"] = tlsData
}
