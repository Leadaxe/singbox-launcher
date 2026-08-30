package subscription

import (
	"errors"
	"fmt"
	"strings"

	"singbox-launcher/core/config/configtypes"
)

// xrayBuildHysteriaFromOutbound — Xray-диалект протокола "hysteria".
//
// Это НЕ отдельный протокол, а обёртка: форки Xray (в том числе сборки с
// «finalmask») отдают и Hysteria v1, и Hysteria 2 под одним именем
// protocol:"hysteria", различая их полем version. Реальная запись публичной
// подписки выглядит так:
//
//	{"protocol":"hysteria",
//	 "settings":{"address":"1.2.3.4","port":8449,"version":2},
//	 "streamSettings":{"hysteriaSettings":{"auth":"...","version":2},
//	                   "network":"hysteria","security":"tls",
//	                   "tlsSettings":{"alpn":["h3"],"serverName":"..."}}}
//
// Ядро умеет оба типа (protocol/hysteria, protocol/hysteria2), поэтому узел
// разбирается в свой sing-box type, а не отбраковывается целиком. Адрес здесь
// лежит ПЛОСКО в settings, а не в settings.servers[] — общий xrayServerEndpoint
// такую форму не берёт, отсюда собственная выемка.
func xrayBuildHysteriaFromOutbound(ob map[string]interface{}, label string) (*configtypes.ParsedNode, error) {
	settings, _ := ob["settings"].(map[string]interface{})
	streamSettings, _ := ob["streamSettings"].(map[string]interface{})
	hySettings, _ := streamSettings["hysteriaSettings"].(map[string]interface{})

	addr, port, err := xrayHysteriaEndpoint(settings)
	if err != nil {
		return nil, err
	}

	// Версия: сначала транспортная секция (она ближе к протоколу), затем
	// settings. Отсутствие поля означает Hysteria 1.x — версия появилась
	// именно тогда, когда понадобилось отличать вторую.
	version := xrayJSONInt(hySettings["version"])
	if version == 0 {
		version = xrayJSONInt(settings["version"])
	}
	if version == 0 {
		version = 1
	}
	if version != 1 && version != 2 {
		return nil, fmt.Errorf("unsupported hysteria version %d", version)
	}

	auth := xrayHysteriaAuth(hySettings, settings)

	scheme := "hysteria"
	if version == 2 {
		scheme = "hysteria2"
	}

	outbound := map[string]interface{}{
		"tag":         xrayMapString(ob, "tag"),
		"type":        scheme,
		"server":      addr,
		"server_port": port,
	}
	if auth != "" {
		if version == 2 {
			outbound["password"] = auth
		} else {
			outbound["auth_str"] = auth
		}
	}

	// obfs: у v1 — плоская строка-секрет, у v2 — объект {type,password}.
	// Xray-диалект несёт её одним полем, поэтому раскладываем по форме типа;
	// у v2 без явного типа берём salamander — единственную обфускацию,
	// которую понимает и сам Hysteria 2 при строковой записи.
	if obfs := xrayHysteriaObfs(hySettings, settings); obfs != "" {
		if version == 2 {
			outbound["obfs"] = map[string]interface{}{
				"type":     "salamander",
				"password": obfs,
			}
		} else {
			outbound["obfs"] = obfs
		}
	}

	if up := xrayHysteriaMbps(hySettings, settings, "up_mbps", "upMbps", "up"); up > 0 {
		outbound["up_mbps"] = up
	}
	if down := xrayHysteriaMbps(hySettings, settings, "down_mbps", "downMbps", "down"); down > 0 {
		outbound["down_mbps"] = down
	}

	// TLS обязателен: оба протокола живут поверх QUIC. Общая выемка не читает
	// alpn (у TCP-протоколов он берётся из транспорта), а на QUIC alpn=h3 —
	// рабочая часть рукопожатия, поэтому дочитываем его здесь.
	if tls, _ := xrayStreamParts(ob); tls != nil {
		outbound["tls"] = tls
	} else {
		outbound["tls"] = map[string]interface{}{"enabled": true}
	}
	xrayHysteriaApplyTLSExtras(outbound, streamSettings, addr)

	// uTLS/REALITY на QUIC ядро не примет — тот же санитайз, что у hysteria2.
	sanitizeCodes := SanitizeSingboxOutboundMap(outbound, xrayMapString(ob, "tag"))

	node := &configtypes.ParsedNode{
		Tag:      xrayTagOrDefault(ob, scheme),
		Scheme:   scheme,
		Server:   addr,
		Port:     port,
		UUID:     auth,
		Label:    label,
		Outbound: outbound,
	}
	for _, code := range sanitizeCodes {
		node.AddWarning(code)
	}
	return node, nil
}

// xrayHysteriaEndpoint достаёт адрес и порт из плоской settings-секции, а при
// её отсутствии — из привычной settings.servers[0].
func xrayHysteriaEndpoint(settings map[string]interface{}) (string, int, error) {
	if settings == nil {
		return "", 0, errors.New(xrayReasonNoSettings)
	}
	addr := xrayMapString(settings, "address")
	port := xrayJSONInt(settings["port"])
	if addr == "" {
		if serversRaw, ok := settings["servers"].([]interface{}); ok && len(serversRaw) > 0 {
			if s0, ok := serversRaw[0].(map[string]interface{}); ok {
				addr = xrayMapString(s0, "address")
				if p := xrayJSONInt(s0["port"]); p > 0 {
					port = p
				}
			}
		}
	}
	if addr == "" {
		return "", 0, errors.New(xrayReasonNoAddress)
	}
	if port <= 0 || port > 65535 {
		return "", 0, errors.New(xrayReasonBadPort)
	}
	return addr, port, nil
}

// xrayHysteriaAuth ищет секрет по всем написаниям, что встречаются в диалекте.
func xrayHysteriaAuth(sections ...map[string]interface{}) string {
	for _, sec := range sections {
		if sec == nil {
			continue
		}
		for _, key := range []string{"auth", "auth_str", "authStr", "password", "obfsPassword"} {
			if v := strings.TrimSpace(xrayMapString(sec, key)); v != "" && key != "obfsPassword" {
				return v
			}
		}
	}
	return ""
}

// xrayHysteriaObfs достаёт строку обфускации (не путать с auth).
func xrayHysteriaObfs(sections ...map[string]interface{}) string {
	for _, sec := range sections {
		if sec == nil {
			continue
		}
		for _, key := range []string{"obfs", "obfsParam", "obfs_password"} {
			if v := strings.TrimSpace(xrayMapString(sec, key)); v != "" {
				return v
			}
		}
	}
	return ""
}

// xrayHysteriaMbps читает пропускную способность по любому из написаний.
func xrayHysteriaMbps(a, b map[string]interface{}, keys ...string) int {
	for _, sec := range []map[string]interface{}{a, b} {
		if sec == nil {
			continue
		}
		for _, key := range keys {
			if n := xrayJSONInt(sec[key]); n > 0 {
				return n
			}
		}
	}
	return 0
}

// xrayHysteriaApplyTLSExtras дочитывает alpn и подставляет server_name.
//
// server_name на QUIC обязателен по смыслу: без него ядро проверяет
// сертификат по IP-адресу и рукопожатие падает на каждом дозвоне.
func xrayHysteriaApplyTLSExtras(outbound map[string]interface{}, streamSettings map[string]interface{}, addr string) {
	tls, _ := outbound["tls"].(map[string]interface{})
	if tls == nil {
		return
	}
	tlsSettings, _ := streamSettings["tlsSettings"].(map[string]interface{})
	if tlsSettings != nil {
		if _, has := tls["alpn"]; !has {
			if alpnRaw, ok := tlsSettings["alpn"].([]interface{}); ok && len(alpnRaw) > 0 {
				alpn := make([]string, 0, len(alpnRaw))
				for _, v := range alpnRaw {
					if s := strings.TrimSpace(xrayMapString(map[string]interface{}{"v": v}, "v")); s != "" {
						alpn = append(alpn, s)
					}
				}
				if len(alpn) > 0 {
					tls["alpn"] = alpn
				}
			}
		}
		if _, has := tls["server_name"]; !has {
			if sni := xrayMapString(tlsSettings, "server_name"); sni != "" {
				tls["server_name"] = sni
			}
		}
	}
	if sn, _ := tls["server_name"].(string); strings.TrimSpace(sn) == "" && addr != "" {
		tls["server_name"] = addr
	}
}
