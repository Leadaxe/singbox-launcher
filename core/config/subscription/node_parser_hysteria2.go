package subscription

import (
	"singbox-launcher/core/config/configtypes"
)

// buildHysteria2Outbound — карта outbound'а hysteria2 из разобранной ссылки.
//
// Маппер: имена параметров и форма значений, без решений. Тип обфускации вне
// набора ядра, обфускация без пароля, нечисловая полоса — всё это снимет
// санитайзер по реестру (obfs_unknown, obfs_password_missing, type_invalid).
func buildHysteria2Outbound(node *configtypes.ParsedNode, outbound map[string]interface{}) {
	q := node.Query
	const scheme = "hysteria2"
	// Пароль лежит в userinfo (ParseNode кладёт его в UUID). Пустой = узла
	// нет, и скажет об этом реестр: password у hysteria2 обязателен.
	if node.UUID != "" {
		outbound["password"] = node.UUID
	}

	// mport / ports — многопортовость Hysteria2: в ссылке диапазоны пишут
	// через дефис и запятую, в теле это список "low:high" (форма записи).
	// https://v2.hysteria.network/docs/advanced/Port-Hopping/
	if sp := hysteria2MportSpecToSingBoxServerPorts(queryParam(q, scheme, "mport")); len(sp) > 0 {
		outbound["server_ports"] = sp
	}

	// obfs: в ссылке плоские параметры, в теле — объект.
	if obfs := queryParam(q, scheme, "obfs"); obfs != "" {
		obfsConfig := map[string]interface{}{"type": obfs}
		if pw := queryParamRaw(q, scheme, "obfs-password"); pw != "" {
			obfsConfig["password"] = pw
		}
		// Границы размера пакета есть только у gecko (option/hysteria2.go
		// Hysteria2ObfsGecko); ядро их у salamander игнорирует, а реестр
		// описывает как поля того же объекта.
		if v := queryParam(q, scheme, "obfs-min-packet-size"); v != "" {
			obfsConfig["min_packet_size"] = v
		}
		if v := queryParam(q, scheme, "obfs-max-packet-size"); v != "" {
			obfsConfig["max_packet_size"] = v
		}
		outbound["obfs"] = obfsConfig
	}

	// Полоса: имена upmbps/downmbps и их написания — из реестра; строку в
	// число приведёт санитайзер (у ядра это int, и строка даёт вердикт A).
	if up := queryParam(q, scheme, "upmbps"); up != "" {
		outbound["up_mbps"] = up
	}
	if down := queryParam(q, scheme, "downmbps"); down != "" {
		outbound["down_mbps"] = down
	}

	buildHysteria2TLS(node, outbound)
}

// buildHysteria2TLS собирает TLS-блок hysteria2 (протокол поверх QUIC — TLS
// включён всегда).
//
// uTLS здесь сознательно не читается: ядро не применяет отпечаток поверх
// QUIC, а лишний блок менял бы identity-хеш узла между путями (D-033).
func buildHysteria2TLS(node *configtypes.ParsedNode, outbound map[string]interface{}) {
	q := node.Query
	tlsData := map[string]interface{}{"enabled": true}

	if sni := tlsServerNameFromQuery(q, "hysteria2", node.Server); sni != "" {
		tlsData["server_name"] = sni
	}
	if pin := queryParam(q, "hysteria2", "pinSHA256"); pin != "" {
		tlsData["certificate_public_key_sha256"] = []string{pin}
	}
	applyTLSQueryExtras(q, "hysteria2", tlsData)

	outbound["tls"] = tlsData
}
